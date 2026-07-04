package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

type SlackOAuthHandler struct {
	db            *sql.DB
	storage       *slackbot.Storage
	apiKeys       *APIKeyManager
	bot           *slackbot.Bot
	clientID      string
	clientSecret  string
	redirectURL   string
	mcpServerURL  string
	maxSteps      int
	states        map[string]*slackOAuthState
	statesMu      sync.RWMutex
}

func NewSlackOAuthHandler(db *sql.DB, clientID, clientSecret, redirectURL, mcpServerURL string, maxSteps int, bot *slackbot.Bot) *SlackOAuthHandler {
	return &SlackOAuthHandler{
		db:           db,
		storage:      slackbot.NewStorage(db),
		apiKeys:      NewAPIKeyManager(db),
		bot:          bot,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		mcpServerURL: mcpServerURL,
		maxSteps:     maxSteps,
		states:       make(map[string]*slackOAuthState),
	}
}

// InstallSlackBot redirects the user to Slack's OAuth consent screen.
// Requires authentication. Accepts org_id as a query parameter.
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

	// Check if already configured
	existing, err := h.storage.GetSlackConfig(c.Request().Context(), orgID)
	if err == nil && existing != nil && existing.BotToken != "" {
		return c.JSON(http.StatusConflict, map[string]string{
			"error":   "Slack bot already configured for this org",
			"message": "Delete the existing config first, or reconnect from the Slack app settings.",
		})
	}

	// Generate a random state nonce
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate state")
	}
	stateStr := hex.EncodeToString(nonce)

	redirectTo := c.QueryParam("redirect_to")
	if redirectTo == "" {
		redirectTo = "/settings#slack"
	}

	h.statesMu.Lock()
	h.states[stateStr] = &slackOAuthState{
		OrgID:      orgID,
		UserID:     user.ID,
		RedirectTo: redirectTo,
		CreateAt:   time.Now(),
	}
	h.statesMu.Unlock()

	// Build Slack OAuth URL
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

// SlackOAuthCallback handles the OAuth callback from Slack.
func (h *SlackOAuthHandler) SlackOAuthCallback(c echo.Context) error {
	code := c.QueryParam("code")
	stateStr := c.QueryParam("error")
	if stateStr != "" {
		return c.HTML(http.StatusBadRequest, fmt.Sprintf("<h2>Error</h2><p>Slack returned an error: %s. Please try again.</p>", stateStr))
	}
	code = c.QueryParam("code")
	stateStr = c.QueryParam("state")
	if code == "" || stateStr == "" {
		return c.HTML(http.StatusBadRequest, "<h2>Error</h2><p>Missing code or state parameter. Please try again.</p>")
	}

	// Look up and validate state
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

	// Exchange code for token
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

	// Fetch existing config to see if we already have an api_key
	existing, lookupErr := h.storage.GetSlackConfig(c.Request().Context(), state.OrgID)
	apiKey := ""
	if lookupErr == nil && existing != nil {
		apiKey = existing.APIKey
	}

	// Auto-generate an API key if none exists
	if apiKey == "" {
		log.Printf("[SlackOAuth] No API key found for org %d, generating one", state.OrgID)
		_, plainKey, err := h.apiKeys.CreateAPIKey(state.UserID, state.OrgID, "slack-bot", []string{}, nil)
		if err != nil {
			log.Printf("[SlackOAuth] Failed to generate API key for org %d: %s", state.OrgID, err)
		} else {
			apiKey = plainKey
		}
	}

	// Upsert with bot_token and api_key
	_, err = h.storage.UpsertSlackConfig(c.Request().Context(), state.OrgID, botToken, apiKey)
	if err != nil {
		log.Printf("[SlackOAuth] Failed to save config for org %d: %s", state.OrgID, err)
		return c.HTML(http.StatusInternalServerError, "<h2>Error</h2><p>Failed to save configuration. Please try again.</p>")
	}

	if err := h.storage.UpdateTeamID(c.Request().Context(), state.OrgID, teamID); err != nil {
		log.Printf("[SlackOAuth] Failed to update team_id for org %d: %s", state.OrgID, err)
	}

	log.Printf("[SlackOAuth] Org %d: bot installed successfully for workspace %s (%s)", state.OrgID, teamName, teamID)

	// Dynamically register this org on the running bot
	if h.bot != nil {
		go func() {
			connectorStorage := aiconnectors.NewStorage(h.db)
			connectors, err := connectorStorage.GetAllConnectors(context.Background(), state.OrgID)
			if err != nil || len(connectors) == 0 {
				log.Printf("[SlackOAuth] Org %d: no AI connectors found, bot will start on next restart", state.OrgID)
				return
			}

			var connector *aiconnectors.Connector
			for _, record := range connectors {
				options := connectorStorage.GetConnectorOptions(context.Background(), record)
				c, err := aiconnectors.NewConnector(context.Background(), options)
				if err != nil {
					log.Printf("[SlackOAuth] Org %d: connector %q failed: %v", state.OrgID, record.ConnectorName, err)
					continue
				}
				connector = c
				break
			}
			if connector == nil {
				log.Printf("[SlackOAuth] Org %d: no working connector found, bot will start on next restart", state.OrgID)
				return
			}

			mcpHeaders := map[string]string{"X-API-Key": apiKey}
			err = h.bot.AddOrg(slackbot.OrgConfig{
				OrgID:         state.OrgID,
				SlackBotToken: botToken,
				MCPServerURL:  h.mcpServerURL,
				MCPHeaders:    mcpHeaders,
				Connector:     connector,
				MaxAgentSteps: h.maxSteps,
			})
			if err != nil {
				log.Printf("[SlackOAuth] Org %d: failed to add to running bot: %s", state.OrgID, err)
			} else {
				log.Printf("[SlackOAuth] Org %d: bot now live!", state.OrgID)
			}
		}()
	}

	return c.Redirect(http.StatusFound, state.RedirectTo)
}
