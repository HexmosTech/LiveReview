package teamsbot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/orgname"
	storageanalytics "github.com/livereview/storage/analytics"
)

// resolveMCPBaseURL derives the MCP endpoint the bots talk to. In cloud mode
// it uses the cloud endpoint; in self-hosted mode it uses the production URL
// set in the instance settings (Settings → Instance → Production URL), falling
// back to the cloud endpoint if none is configured.
func resolveMCPBaseURL(db *sql.DB) string {
	if isCloudMode() {
		return "https://livereview.hexmos.com/api/mcp"
	}
	var prodURL sql.NullString
	if err := db.QueryRow("SELECT livereview_prod_url FROM instance_details LIMIT 1").Scan(&prodURL); err == nil && prodURL.Valid && prodURL.String != "" {
		return strings.TrimSuffix(prodURL.String, "/") + "/api/mcp"
	}
	return "https://livereview.hexmos.com/api/mcp"
}

// isCloudMode reports whether LiveReview is running in cloud mode.
func isCloudMode() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("LIVEREVIEW_IS_CLOUD")), "true")
}

type Handler struct {
	Bot    *Bot
	db     *sql.DB
	cancel context.CancelFunc
}

func NewHandler(db *sql.DB) (*Handler, error) {
	bot, err := buildBot(db)
	if err != nil {
		return nil, err
	}
	if bot == nil {
		return nil, nil
	}
	return &Handler{Bot: bot, db: db}, nil
}

// BaseURL resolves the address the Teams bot uses for anything it needs to
// hand back to Teams itself (e.g. chart image URLs): TEAMS_BOT_BASE_URL if
// explicitly set, else the instance's own configured public URL (Settings ->
// Instance -> Production URL), else http://localhost:8888 as a last-resort
// local-dev default. Exported so a live sync path (adding one org to an
// already-running bot, without a full server restart) can build a BotConfig
// with the same base URL as the boot-time path below.
//
// Chart images embedded in Teams AdaptiveCards are fetched by Microsoft's
// own cloud infrastructure, not the customer's browser - a URL pointing at
// localhost (the old unconditional fallback here) is unreachable from
// Teams and renders as a broken image in every real deployment that hasn't
// separately set TEAMS_BOT_BASE_URL. Falling back to the same public URL
// ResolveInstancePublicURL already resolves for the app manifest fixes that
// for any self-hosted instance with a Production URL configured, with
// localhost remaining the fallback only when neither is set (pure local
// dev/testing, e.g. against the Bot Framework Emulator).
func BaseURL(db *sql.DB) string {
	if v := os.Getenv("TEAMS_BOT_BASE_URL"); v != "" {
		return v
	}
	if db != nil {
		if u, err := ResolveInstancePublicURL(db); err == nil {
			return u
		}
	}
	return "http://localhost:8888"
}

// isPublicHTTPSURL reports whether raw is a well-formed https:// URL with a
// hostname that isn't localhost/loopback - the bar for "safe to ship inside
// a Teams app package a real admin will upload to a real Teams tenant".
func isPublicHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1", "::1":
		return false
	default:
		return true
	}
}

// ResolveInstancePublicURL returns this instance's own public HTTPS base
// URL, for contexts that need a real customer-facing address rather than
// whatever's convenient for local bot-to-Teams traffic (see BaseURL).
// Prefers the admin-configured Production URL (Settings -> Instance ->
// Production URL, stored as instance_details.livereview_prod_url) - that's
// the field customers are actually told to set for exactly this purpose -
// falling back to TEAMS_BOT_BASE_URL only if it happens to already be a
// real public https URL. Returns an error (never a localhost/http URL) if
// neither resolves, so callers can surface an actionable message instead of
// silently shipping a broken URL to a customer.
func ResolveInstancePublicURL(db *sql.DB) (string, error) {
	var prodURL sql.NullString
	if err := db.QueryRow("SELECT livereview_prod_url FROM instance_details LIMIT 1").Scan(&prodURL); err == nil && prodURL.Valid {
		if u := strings.TrimSuffix(strings.TrimSpace(prodURL.String), "/"); isPublicHTTPSURL(u) {
			return u, nil
		}
	}
	if v := strings.TrimSuffix(strings.TrimSpace(os.Getenv("TEAMS_BOT_BASE_URL")), "/"); isPublicHTTPSURL(v) {
		return v, nil
	}
	return "", fmt.Errorf("no public HTTPS instance URL configured - set it under Settings → Instance → Production URL")
}

// BuildOrgConfig resolves the AI connector, MCP URL/headers, and org name
// needed to construct a BotConfig for one org's stored Teams config. Returns
// (nil, err) with a log-ready reason when the org should be skipped (no
// usable AI connector, etc.) - shared by buildBot (boot-time, all orgs) and
// the live-sync path in internal/api/server.go (one org, called right after
// UpdateTeamsConfig saves).
func BuildOrgConfig(db *sql.DB, cfg TeamsConfig) (*BotConfig, error) {
	connectorStorage := aiconnectors.NewStorage(db)
	connectors, err := connectorStorage.GetAllConnectors(context.Background(), cfg.OrgID)
	if err != nil || len(connectors) == 0 {
		return nil, fmt.Errorf("org %d: no AI connectors found", cfg.OrgID)
	}

	var connector *aiconnectors.Connector
	for _, record := range connectors {
		options := connectorStorage.GetConnectorOptions(context.Background(), record)
		c, err := aiconnectors.NewConnector(context.Background(), options)
		if err != nil {
			log.Printf("Teams bot org %d: connector %q failed: %v", cfg.OrgID, record.ConnectorName, err)
			continue
		}
		connector = c
		log.Printf("Teams bot org %d: using connector %q (%s, model=%s)", cfg.OrgID, record.ConnectorName, record.ProviderName, options.ModelConfig.Model)
		break
	}
	if connector == nil {
		return nil, fmt.Errorf("org %d: all connectors failed to initialize", cfg.OrgID)
	}

	orgName, orgNameErr := orgname.OrgNameByID(context.Background(), db, cfg.OrgID)
	if orgNameErr != nil {
		log.Printf("Teams bot: failed to resolve org name for org %d: %v", cfg.OrgID, orgNameErr)
	}

	return &BotConfig{
		OrgID:        cfg.OrgID,
		OrgName:      orgName,
		BotAppID:     cfg.BotAppID,
		BotPassword:  cfg.BotPassword,
		TenantID:     cfg.TenantID,
		MCPServerURL: resolveMCPBaseURL(db),
		MCPHeaders:   map[string]string{"X-API-Key": cfg.APIKey},
		Connector:    connector,
		Analytics:    storageanalytics.NewAdHocStore(db),
		MaxSteps:     20,
	}, nil
}

func buildBot(db *sql.DB) (*Bot, error) {
	configStorage := NewStorage(db)
	configs, err := configStorage.GetAllEnabledConfigs(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to query Teams configs: %w", err)
	}
	if len(configs) == 0 {
		return nil, nil
	}

	var botCfgs []BotConfig
	for _, cfg := range configs {
		botCfg, err := BuildOrgConfig(db, cfg)
		if err != nil {
			log.Printf("Teams bot: %v — skipping", err)
			continue
		}
		botCfgs = append(botCfgs, *botCfg)
	}

	if len(botCfgs) == 0 {
		return nil, fmt.Errorf("no orgs could be configured for Teams bot")
	}

	return NewBot(context.Background(), botCfgs, BaseURL(db)), nil
}

func (h *Handler) Start() {
	if h == nil || h.Bot == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	fmt.Println("Starting Teams bot...")
	go func() {
		if err := h.Bot.Start(ctx); err != nil {
			fmt.Printf("Teams bot failed: %v\n", err)
		}
	}()
}

func (h *Handler) Stop() {
	if h == nil || h.cancel == nil {
		return
	}
	h.cancel()
	fmt.Println("Teams bot stopped")
}

func (h *Handler) HandleMessage(c echo.Context) error {
	if h == nil || h.Bot == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Teams bot not initialized"})
	}

	var activity Activity
	if err := c.Bind(&activity); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid activity"})
	}

	log.Printf("[TeamsBot] Received activity: type=%s text=%q conv=%+v from=%+v recipient=%+v serviceUrl=%s id=%s",
		activity.Type, activity.Text, activity.Conversation, activity.From, activity.Recipient, activity.ServiceURL, activity.ID)

	authHeader := c.Request().Header.Get("Authorization")

	if err := h.Bot.HandleActivity(c.Request().Context(), &activity, authHeader); err != nil {
		if errors.Is(err, ErrJWTValidationFailed) {
			log.Printf("[TeamsBot] JWT validation failed")
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		}
		// Non-JWT errors here come from HandleActivity's message/postReply
		// path (marshal/HTTP/URL errors, agent errors) - none embed authHeader.
		log.Printf("[TeamsBot] Error handling activity")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) ServeChartPNG(c echo.Context) error {
	if h == nil {
		return c.NoContent(http.StatusInternalServerError)
	}
	id := c.Param("id")
	if id == "" {
		return c.NoContent(http.StatusBadRequest)
	}
	path, ok := LookupChartFile(id)
	if !ok {
		return c.NoContent(http.StatusNotFound)
	}
	return c.File(path)
}
