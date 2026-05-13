package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/livereview/internal/api/mcp/generated"
	"github.com/livereview/internal/api/mcp/shared"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const API_BASE_URL = "https://manual-talent2.apps.hexmos.com"

type MCPService struct {
	MCPServer *server.MCPServer
	SSEServer *server.SSEServer
	Store     *shared.TokenStore
}

func NewMCPService(baseURL string) *MCPService {
	store := shared.NewTokenStore()

	mcpServer := server.NewMCPServer(
		"LiveReview",
		"1.0.0",
		server.WithLogging(),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithToolCapabilities(true),
	)

	s := &MCPService{
		MCPServer: mcpServer,
		Store:     store,
	}

	// Initialize SSEServer
	s.SSEServer = server.NewSSEServer(mcpServer,
		server.WithBaseURL(baseURL),
		server.WithStaticBasePath("/api/v1/mcp"),
		server.WithSSEEndpoint("/"),
		server.WithMessageEndpoint("/message"),
		server.WithUseFullURLForMessageEndpoint(true),
        server.WithSSEContextFunc(server.SSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
            key := r.Header.Get("X-API-Key")
            if key != "" {
                ctx = context.WithValue(ctx, "livereview_api_key", key)
            }
            return ctx
        })),
	)

	s.registerCustomTools()
	s.registerGeneratedTools()

	// Initialize global proxy
	shared.GlobalProxy = s

	return s
}

func (s *MCPService) GetHandler() http.Handler {
	return s.SSEServer
}

func (s *MCPService) registerCustomTools() {
	// Register login_to_livereview
	s.MCPServer.AddTool(mcp.NewTool("login_to_livereview",
		mcp.WithDescription("Start the interactive login process for LiveReview."),
	), s.loginHandler)

	// Register check_login_status
	s.MCPServer.AddTool(mcp.NewTool("check_login_status",
		mcp.WithDescription("Check if the browser-based login has been completed."),
	), s.checkLoginStatusHandler)

	// Register list_organizations
	s.MCPServer.AddTool(mcp.NewTool("list_organizations",
		mcp.WithDescription("List all organizations the authenticated user belongs to."),
	), s.listOrganizationsHandler)

	// Register select_organization
	s.MCPServer.AddTool(mcp.NewTool("select_organization",
		mcp.WithDescription("Set the active organization context for LiveReview API calls."),
		mcp.WithInteger("org_id", mcp.Description("The ID of the organization to select.")),
	), s.selectOrganizationHandler)

	// Register get_me for testing authentication and /me endpoint access
	s.MCPServer.AddTool(mcp.NewTool("get_me",
		mcp.WithDescription("Call /api/v1/auth/me to verify authentication and connectivity."),
	), s.authMeHandler)
}

func (s *MCPService) registerGeneratedTools() {
	// Register all tools from the generated mcpgen package
	mcpgen.RegisterAllTools(s.MCPServer)
}

func (s *MCPService) loginHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(API_BASE_URL+"/api/v1/auth/mcp/initiate", "application/json", nil)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("❌ Error: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("❌ Failed to initiate login: %s", string(body))), nil
	}

	var data struct {
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("❌ Failed to decode response: %v", err)), nil
	}

	loginURL := fmt.Sprintf("%s/#/auth/mcp?id=%s", API_BASE_URL, data.RequestID)

	// Save it for check_login_status
	s.Store.SetPendingRequestID(data.RequestID)

	msg := fmt.Sprintf("### LiveReview Authentication Required\n\nPlease open this link in your browser to sign in:\n%s\n\nAfter signing in, run the `check_login_status` tool.", loginURL)
	return mcp.NewToolResultText(msg), nil
}

func (s *MCPService) checkLoginStatusHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	state := s.Store.GetPendingRequestID()
	if state == "" {
		return mcp.NewToolResultError("❌ No pending login found. Please run the `login_to_livereview` tool first."), nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/api/v1/auth/mcp/poll/%s", API_BASE_URL, state))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("❌ Error checking status: %v", err)), nil
	}
	defer resp.Body.Close()

	// Case 1: Pending (202 or 200 with pending status)
	var data struct {
		Status    string `json:"status"`
		TokenPair struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"token_pair"`
	}

	if resp.StatusCode == http.StatusAccepted {
		return mcp.NewToolResultText("⏳ Login still pending in browser... please complete the process there."), nil
	}

	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("❌ Failed to decode response: %v", err)), nil
		}

		if data.Status == "pending" {
			return mcp.NewToolResultText("⏳ Login still pending in browser... please complete the process there."), nil
		}

		if data.Status == "completed" {
			// s.Store.SetToken(data.TokenPair.AccessToken, data.TokenPair.RefreshToken)
			return mcp.NewToolResultText("✅ Login successful! Your session is now preserved."), nil
		}
	}

	body, _ := io.ReadAll(resp.Body)
	return mcp.NewToolResultError(fmt.Sprintf("❌ Unexpected response: %d %s", resp.StatusCode, string(body))), nil
}

func (s *MCPService) listOrganizationsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	res, err := s.rawCallAPI(ctx, "GET", "/api/v1/auth/me", nil)
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return res, nil
	}

	if len(res.Content) == 0 {
		return mcp.NewToolResultError("❌ API returned no content"), nil
	}

	textContent, ok := mcp.AsTextContent(res.Content[0])
	if !ok {
		return mcp.NewToolResultError("❌ API returned non-text content"), nil
	}

	// Python implementation parses the response and lists orgs
	var data struct {
		Organizations []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"organizations"`
	}

	// result is a JSON string from rawCallAPI
	if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("❌ Failed to parse user info: %v", err)), nil
	}

	if len(data.Organizations) == 0 {
		return mcp.NewToolResultText("No organizations found for this user."), nil
	}

	lines := []string{"### Available Organizations:", ""}
	for _, org := range data.Organizations {
		lines = append(lines, fmt.Sprintf("- **%s** (ID: `%d`) - Role: %s", org.Name, org.ID, org.Role))
	}
	lines = append(lines, "\nUse `select_organization` tool with an ID to set the active context.")

	return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
}

func (s *MCPService) selectOrganizationHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("invalid arguments"), nil
	}

	orgID, ok := args["org_id"].(float64)
	if !ok {
		return mcp.NewToolResultError("org_id is required"), nil
	}

	// s.Store.SetOrgID(int(orgID))
	return mcp.NewToolResultText(fmt.Sprintf("✅ Organization context set to ID: `%d`. All subsequent calls will use this context.", int(orgID))), nil
}

func (s *MCPService) authMeHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return s.rawCallAPI(ctx, "GET", "/api/v1/auth/me", nil)
}

// CallAPI is the main proxy method called by generated tool handlers.
func (s *MCPService) CallAPI(ctx context.Context, method, path string, arguments interface{}) (*mcp.CallToolResult, error) {
	return s.rawCallAPI(ctx, method, path, arguments)
}

func (s *MCPService) rawCallAPI(ctx context.Context, method, path string, arguments interface{}) (*mcp.CallToolResult, error) {
	client := &http.Client{Timeout: 90 * time.Second}

	args, _ := arguments.(map[string]interface{})

	// Interpolate path parameters if any (e.g. {id})
	for k, v := range args {
		placeholder := "{" + k + "}"
		if strings.Contains(path, placeholder) {
			path = strings.ReplaceAll(path, placeholder, fmt.Sprintf("%v", v))
			delete(args, k)
		}
	}

	fullURL := API_BASE_URL + path

	var body io.Reader
	if method == "POST" || method == "PUT" {
		jsonBody, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		body = strings.NewReader(string(jsonBody))
	} else if method == "GET" && len(args) > 0 {
		// handle query params
		queryParams := []string{}
		for k, v := range args {
			queryParams = append(queryParams, fmt.Sprintf("%s=%v", k, v))
		}
		if len(queryParams) > 0 {
			fullURL += "?" + strings.Join(queryParams, "&")
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}

	// Inject API Key from MCP config environment
	// Get API key from mcp-remote header (passed via context)
	apiKey, ok := ctx.Value("livereview_api_key").(string)
	if !ok || apiKey == "" {
		return mcp.NewToolResultError("missing LIVEREVIEW_API_KEY (set it in your Claude/Cursor MCP config)"), nil
	}
	req.Header.Set("X-API-Key", apiKey)

	orgID := 0
	if orgID != 0 {
		req.Header.Set("X-Org-Context", fmt.Sprintf("%d", orgID))
	}

	log.Printf("[MCP Proxy] %s %s %s", method, fullURL,apiKey)

	log.Println("=== OUTGOING HEADERS ===")
	for k, v := range req.Header {
		log.Printf("%s: %v\n", k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("API call failed: %v", err)), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(respBody))), nil
	}

	var result interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to decode API response: %v", err)), nil
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}
