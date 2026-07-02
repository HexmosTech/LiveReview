package mcpagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

const defaultMCPTimeout = 120 * time.Second

// jsonrpcMessage is a JSON-RPC 2.0 request/response.
type jsonrpcMessage struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method,omitempty"`
	Params  any         `json:"params,omitempty"`
	Result  any         `json:"result,omitempty"`
	Error   *jsonrpcErr `json:"error,omitempty"`
}

type jsonrpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// callToolParams is the JSON-RPC params for the "tools/call" method.
type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// listToolsResult is the JSON-RPC result for the "tools/list" method.
type listToolsResult struct {
	Tools []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema any    `json:"inputSchema"`
	} `json:"tools"`
}

// callToolResult is the JSON-RPC result for the "tools/call" method.
type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text,omitempty"`
		Mime string `json:"mimeType,omitempty"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// ConnectMCP opens a session with a remote MCP server via Streamable HTTP,
// performs the initialize handshake, and lists available tools.
func ConnectMCP(ctx context.Context, serverURL string, headers map[string]string) (*MCPSession, error) {
	reqID := 1

	// 1. initialize
	initResult, err := doJSONRPC(ctx, serverURL, headers, reqID, "initialize", map[string]any{
		"protocolVersion": "2025-03-26",
		"client": map[string]string{
			"name":    "livereview-mcp-agent",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp initialize: %w", err)
	}
	log.Debug().Str("server", serverURL).Any("result", initResult).Msg("MCP initialized")
	reqID++

	// 2. tools/list
	listResult, err := doJSONRPC(ctx, serverURL, headers, reqID, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list: %w", err)
	}
	reqID++

	b, _ := json.Marshal(listResult)
	var ltr listToolsResult
	if err := json.Unmarshal(b, &ltr); err != nil {
		return nil, fmt.Errorf("mcp tools/list decode: %w", err)
	}

	tools := make([]MCPToolDef, len(ltr.Tools))
	for i, t := range ltr.Tools {
		tools[i] = MCPToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}

	session := &MCPSession{
		ServerURL: serverURL,
		Headers:   headers,
		Tools:     tools,
	}

	log.Info().
		Str("server", serverURL).
		Int("tools", len(tools)).
		Msg("MCP session established")
	return session, nil
}

// CallTool invokes a tool on the remote MCP server.
func CallTool(ctx context.Context, session *MCPSession, name string, args map[string]any) (string, error) {
	reqID := 1

	result, err := doJSONRPC(ctx, session.ServerURL, session.Headers, reqID, "tools/call", callToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("mcp tools/call %s: %w", name, err)
	}

	b, _ := json.Marshal(result)
	var ctr callToolResult
	if err := json.Unmarshal(b, &ctr); err != nil {
		return "", fmt.Errorf("mcp tools/call %s decode: %w", name, err)
	}

	var parts []string
	for _, c := range ctr.Content {
		switch c.Type {
		case "text":
			parts = append(parts, c.Text)
		case "image":
			parts = append(parts, fmt.Sprintf("[image content omitted, mime type: %s]", c.Mime))
		default:
			parts = append(parts, fmt.Sprintf("[%s content]", c.Type))
		}
	}
	text := joinStrings(parts, "\n")
	if ctr.IsError {
		text = "[MCP TOOL ERROR] " + text
	}
	return text, nil
}

// doJSONRPC sends a JSON-RPC request and returns the result.
func doJSONRPC(ctx context.Context, url string, headers map[string]string, id int, method string, params any) (any, error) {
	reqBody := jsonrpcMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: defaultMCPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var jr jsonrpcMessage
	if err := json.Unmarshal(respBody, &jr); err != nil {
		return nil, fmt.Errorf("json decode: %w (body: %s)", err, truncate(string(respBody), 500))
	}
	if jr.Error != nil {
		return nil, fmt.Errorf("json-rpc error %d: %s", jr.Error.Code, jr.Error.Message)
	}

	return jr.Result, nil
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += sep + p
	}
	return result
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
