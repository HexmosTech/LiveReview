package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/slackbot"
	"github.com/slack-go/slack"
)

const (
	slackOAuthAuthorizeURL = "https://slack.com/oauth/v2/authorize"
	slackOAuthScope        = "chat:write,files:write,im:read,im:history,channels:read,channels:history,app_mentions:read,users:read"
	slackUserScope         = ""
	stateTTL               = 10 * time.Minute
)

type slackOAuthState struct {
	OrgID      int64     `json:"org_id"`
	UserID     int64     `json:"user_id"`
	RedirectTo string    `json:"redirect_to"`
	CreateAt   time.Time `json:"created_at"`
}

type proxySetupState struct {
	OrgID    int64     `json:"org_id"`
	CreateAt time.Time `json:"created_at"`
}

type SlackOAuthHandler struct {
	db              *sql.DB
	storage         *slackbot.Storage
	apiKeys         *APIKeyManager
	bot             *slackbot.Bot
	clientID        string
	clientSecret    string
	redirectURL     string
	mcpServerURL    string
	maxSteps        int
	selfURL         string
	isCloud         bool
	states          map[string]*slackOAuthState
	proxySetupStore map[string]*proxySetupState
	statesMu        sync.RWMutex
}

func NewSlackOAuthHandler(db *sql.DB, clientID, clientSecret, redirectURL, mcpServerURL string, maxSteps int, bot *slackbot.Bot, selfURL string, isCloud bool) *SlackOAuthHandler {
	return &SlackOAuthHandler{
		db:              db,
		storage:         slackbot.NewStorage(db),
		apiKeys:         NewAPIKeyManager(db),
		bot:             bot,
		clientID:        clientID,
		clientSecret:    clientSecret,
		redirectURL:     redirectURL,
		mcpServerURL:    mcpServerURL,
		maxSteps:        maxSteps,
		selfURL:         selfURL,
		isCloud:         isCloud,
		states:          make(map[string]*slackOAuthState),
		proxySetupStore: make(map[string]*proxySetupState),
	}
}

func (h *SlackOAuthHandler) InstallSlackBot(c echo.Context) error {
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "authentication required")
	}

	orgIDStr := c.QueryParam("org_id")
	if orgIDStr == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "org_id query parameter is required")
	}

	var orgID int64
	if _, err := fmt.Sscan(orgIDStr, &orgID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid org_id")
	}

	existing, err := h.storage.GetSlackConfig(c.Request().Context(), orgID)
	if err == nil && existing != nil && existing.BotToken != "" {
		return c.JSON(http.StatusConflict, map[string]string{
			"error":   "Slack bot already configured for this org",
			"message": "Delete the existing config first, or reconnect from the Slack app settings.",
		})
	}

	redirectTo := c.QueryParam("redirect_to")
	if redirectTo == "" {
		redirectTo = "/settings#integrations"
	}

	var stateStr string

	if h.selfURL != "" {
		// Proxy flow: state encodes target info for the cloud proxy callback
		setupToken := make([]byte, 32)
		if _, err := rand.Read(setupToken); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate setup token")
		}
		setupTokenStr := hex.EncodeToString(setupToken)

		h.statesMu.Lock()
		h.proxySetupStore[setupTokenStr] = &proxySetupState{
			OrgID:    orgID,
			CreateAt: time.Now(),
		}
		h.statesMu.Unlock()

		statePayload := map[string]string{
			"url":         h.selfURL,
			"org_id":      fmt.Sprintf("%d", orgID),
			"setup_token": setupTokenStr,
		}
		stateJSON, _ := json.Marshal(statePayload)
		stateStr = base64.URLEncoding.EncodeToString(stateJSON)
	} else {
		// Direct flow: state stores org/user for local callback
		stateBytes := make([]byte, 32)
		if _, err := rand.Read(stateBytes); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate state")
		}
		stateStr = hex.EncodeToString(stateBytes)

		h.statesMu.Lock()
		h.states[stateStr] = &slackOAuthState{
			OrgID:      orgID,
			UserID:     user.ID,
			RedirectTo: redirectTo,
			CreateAt:   time.Now(),
		}
		h.statesMu.Unlock()
	}

	// Build Slack OAuth URL pointing to cloud callback
	slackURL, err := url.Parse(slackOAuthAuthorizeURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build auth URL")
	}
	q := slackURL.Query()
	q.Set("client_id", h.clientID)
	q.Set("scope", slackOAuthScope)
	q.Set("user_scope", slackUserScope)
	q.Set("redirect_uri", h.redirectURL)
	q.Set("state", stateStr)
	slackURL.RawQuery = q.Encode()

	return c.JSON(http.StatusOK, map[string]string{
		"url": slackURL.String(),
	})
}

// SlackOAuthCallback handles the OAuth callback from Slack (self-hosted direct mode).
func (h *SlackOAuthHandler) SlackOAuthCallback(c echo.Context) error {
	code := c.QueryParam("code")
	stateStr := c.QueryParam("error")
	if stateStr != "" {
		escapedErr := html.EscapeString(stateStr)
		return c.HTML(http.StatusBadRequest, fmt.Sprintf("<h2>Error</h2><p>Slack returned an error: %s. Please try again.</p>", escapedErr))
	}
	code = c.QueryParam("code")
	stateStr = c.QueryParam("state")
	if code == "" || stateStr == "" {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Missing code or state parameter. Please try again.</p>")
	}

	h.statesMu.Lock()
	state, ok := h.states[stateStr]
	if ok {
		delete(h.states, stateStr)
	}
	h.statesMu.Unlock()

	if !ok {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Invalid or expired state. Please try again.</p>")
	}
	if time.Since(state.CreateAt) > stateTTL {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>State expired. Please try again.</p>")
	}

	return h.completeInstall(c, state.OrgID, state.UserID, state.RedirectTo, code)
}

// SlackOAuthProxyCallback handles the OAuth callback in cloud mode and
// proxies the bot token to the customer's self-hosted server.
func (h *SlackOAuthHandler) SlackOAuthProxyCallback(c echo.Context) error {
	code := c.QueryParam("code")
	stateStr := c.QueryParam("error")
	if stateStr != "" {
		escapedErr := html.EscapeString(stateStr)
		return c.HTML(http.StatusBadRequest, fmt.Sprintf("<h2>Error</h2><p>Slack returned an error: %s. Please try again.</p>", escapedErr))
	}
	code = c.QueryParam("code")
	stateStr = c.QueryParam("state")
	if code == "" || stateStr == "" {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Missing code or state parameter. Please try again.</p>")
	}

	// Decode state to get target server info
	stateJSON, err := base64.URLEncoding.DecodeString(stateStr)
	if err != nil {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Invalid state. Please try again.</p>")
	}
	var statePayload map[string]string
	if err := json.Unmarshal(stateJSON, &statePayload); err != nil {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Invalid state. Please try again.</p>")
	}

	targetURL := statePayload["url"]
	orgID := statePayload["org_id"]
	setupToken := statePayload["setup_token"]
	if targetURL == "" || orgID == "" || setupToken == "" {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Invalid state. Please try again.</p>")
	}

	// Exchange code for bot token
	resp, err := slack.GetOAuthV2Response(
		&http.Client{Timeout: 30 * time.Second},
		h.clientID, h.clientSecret, code, h.redirectURL,
	)
	if err != nil {
		log.Printf("[SlackOAuth] Token exchange failed: %s", err)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Failed to exchange authorization code with Slack. Please try again.</p>")
	}
	if !resp.Ok {
		log.Printf("[SlackOAuth] Token exchange returned error: %s", resp.Error)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Slack returned an error during token exchange. Please try again.</p>")
	}

	botToken := resp.AccessToken

	// Proxy the bot token to the customer's self-hosted server
	proxyBody, _ := json.Marshal(map[string]string{"bot_token": botToken})
	proxyReq, err := http.NewRequestWithContext(c.Request().Context(), http.MethodPost,
		fmt.Sprintf("%s/api/v1/orgs/%s/slack-proxy-callback?setup_token=%s", targetURL, orgID, setupToken),
		bytes.NewReader(proxyBody),
	)
	if err != nil {
		log.Printf("[SlackOAuth] Failed to create proxy request: %s", err)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Failed to forward token. Please try again.</p>")
	}
	proxyReq.Header.Set("Content-Type", "application/json")

	proxyResp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		log.Printf("[SlackOAuth] Proxy to customer server failed: %s", err)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Failed to reach your LiveReview instance. Ensure it is accessible from the cloud.</p>")
	}
	defer proxyResp.Body.Close()

	if proxyResp.StatusCode != http.StatusOK {
		log.Printf("[SlackOAuth] Proxy returned status %d", proxyResp.StatusCode)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Your LiveReview instance rejected the token. Please try again.</p>")
	}

	log.Printf("[SlackOAuth] Bot token proxied successfully to %s org %s", targetURL, orgID)
	return c.Redirect(http.StatusFound, fmt.Sprintf("%s/settings#integrations", targetURL))
}

// SlackProxyCallback receives the bot token from the cloud proxy and completes installation.
func (h *SlackOAuthHandler) SlackProxyCallback(c echo.Context) error {
	orgIDStr := c.Param("org_id")
	orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid org_id")
	}

	setupToken := c.QueryParam("setup_token")
	if setupToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "setup_token is required")
	}

	// Validate setup token
	h.statesMu.Lock()
	state, ok := h.proxySetupStore[setupToken]
	if ok {
		delete(h.proxySetupStore, setupToken)
	}
	h.statesMu.Unlock()

	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid or expired setup_token")
	}
	if state.OrgID != orgID {
		return echo.NewHTTPError(http.StatusBadRequest, "org_id mismatch")
	}
	if time.Since(state.CreateAt) > stateTTL {
		return echo.NewHTTPError(http.StatusBadRequest, "setup_token expired")
	}

	var req struct {
		BotToken string `json:"bot_token"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.BotToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_token is required")
	}

	// Generate an API key for this org
	_, plainKey, err := h.apiKeys.CreateAPIKey(0, orgID, "slack-bot", []string{}, nil)
	if err != nil {
		log.Printf("[SlackOAuth] Failed to generate API key for org %d: %s", orgID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate API key")
	}

	ctx := c.Request().Context()

	// Store config
	_, err = h.storage.UpsertSlackConfig(ctx, orgID, req.BotToken, plainKey)
	if err != nil {
		log.Printf("[SlackOAuth] Failed to save config for org %d: %s", orgID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config")
	}

	log.Printf("[SlackOAuth] Org %d: bot installed via proxy", orgID)

	h.ensureBotRunning(orgID, req.BotToken, plainKey)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *SlackOAuthHandler) completeInstall(c echo.Context, orgID, userID int64, redirectTo, code string) error {
	resp, err := slack.GetOAuthV2Response(
		&http.Client{Timeout: 30 * time.Second},
		h.clientID, h.clientSecret, code, h.redirectURL,
	)
	if err != nil {
		log.Printf("[SlackOAuth] Token exchange failed: %s", err)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Failed to exchange authorization code with Slack. Please try again.</p>")
	}
	if !resp.Ok {
		log.Printf("[SlackOAuth] Token exchange returned error: %s", resp.Error)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Slack returned an error during token exchange. Please try again.</p>")
	}

	botToken := resp.AccessToken
	teamID := resp.Team.ID
	teamName := resp.Team.Name

	existing, lookupErr := h.storage.GetSlackConfig(c.Request().Context(), orgID)
	apiKey := ""
	if lookupErr == nil && existing != nil {
		apiKey = existing.APIKey
	}

	if apiKey == "" {
		log.Printf("[SlackOAuth] No API key found for org %d, generating one", orgID)
		_, plainKey, err := h.apiKeys.CreateAPIKey(userID, orgID, "slack-bot", []string{}, nil)
		if err != nil {
			log.Printf("[SlackOAuth] Failed to generate API key for org %d: %s", orgID, err)
		} else {
			apiKey = plainKey
		}
	}

	_, err = h.storage.UpsertSlackConfig(c.Request().Context(), orgID, botToken, apiKey)
	if err != nil {
		log.Printf("[SlackOAuth] Failed to save config for org %d: %s", orgID, err)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Failed to save configuration. Please try again.</p>")
	}

	if err := h.storage.UpdateTeamID(c.Request().Context(), orgID, teamID); err != nil {
		log.Printf("[SlackOAuth] Failed to update team_id for org %d: %s", orgID, err)
	}

	log.Printf("[SlackOAuth] Org %d: bot installed successfully for workspace %s (%s)", orgID, teamName, teamID)

	h.ensureBotRunning(orgID, botToken, apiKey)

	return c.Redirect(http.StatusFound, redirectTo)
}

// ensureBotRunning makes sure the Slack bot is running and the new org is registered.
// If the bot wasn't started at boot (no configs existed), it lazily creates and starts one.
func (h *SlackOAuthHandler) ensureBotRunning(orgID int64, botToken, apiKey string) {
	if h.bot != nil {
		h.bot.UpdateBotToken(orgID, botToken)
		go h.addOrgToBot(orgID, botToken, apiKey)
		return
	}

	// Bot was nil at startup — try to create it now that we have a config in DB.
	bots, err := startOrgSlackBots(h.db)
	if err != nil {
		log.Printf("[SlackOAuth] Failed to lazily start Slack bot: %s", err)
		return
	}
	if len(bots) == 0 {
		log.Printf("[SlackOAuth] No bots created by startOrgSlackBots")
		return
	}
	bot := bots[0]
	h.bot = bot
	log.Printf("[SlackOAuth] Org %d: bot lazily created and started", orgID)
	go func() {
		if err := bot.Start(context.Background()); err != nil {
			log.Printf("[SlackBot] Lazily-started bot exited: %v", err)
		}
	}()
}

func (h *SlackOAuthHandler) addOrgToBot(orgID int64, botToken, apiKey string) {
	connectorStorage := aiconnectors.NewStorage(h.db)
	connectors, err := connectorStorage.GetAllConnectors(context.Background(), orgID)
	if err != nil || len(connectors) == 0 {
		log.Printf("[SlackOAuth] Org %d: no AI connectors found, bot will start on next restart", orgID)
		return
	}

	var connector *aiconnectors.Connector
	for _, record := range connectors {
		options := connectorStorage.GetConnectorOptions(context.Background(), record)
		c, err := aiconnectors.NewConnector(context.Background(), options)
		if err != nil {
			log.Printf("[SlackOAuth] Org %d: connector %q failed: %v", orgID, record.ConnectorName, err)
			continue
		}
		connector = c
		break
	}
	if connector == nil {
		log.Printf("[SlackOAuth] Org %d: no working connector found, bot will start on next restart", orgID)
		return
	}

	mcpHeaders := map[string]string{"X-API-Key": apiKey}
	err = h.bot.AddOrg(slackbot.OrgConfig{
		OrgID:         orgID,
		SlackBotToken: botToken,
		MCPServerURL:  h.mcpServerURL,
		MCPHeaders:    mcpHeaders,
		Connector:     connector,
		MaxAgentSteps: h.maxSteps,
	})
	if err != nil {
		log.Printf("[SlackOAuth] Org %d: failed to add to running bot: %s", orgID, err)
	} else {
		log.Printf("[SlackOAuth] Org %d: bot now live!", orgID)
	}
}
