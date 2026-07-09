package teamsbot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
)

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

func buildBot(db *sql.DB) (*Bot, error) {
	mcpServerURL := os.Getenv("SLACK_MCP_SERVER_URL")
	if mcpServerURL == "" {
		mcpServerURL = "https://livereview.hexmos.com/api/mcp"
	}
	maxSteps := 20
	if s := os.Getenv("SLACK_MAX_AGENT_STEPS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			maxSteps = n
		}
	}

	configStorage := NewStorage(db)
	configs, err := configStorage.GetAllEnabledConfigs(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to query Teams configs: %w", err)
	}
	if len(configs) == 0 {
		return nil, nil
	}

	connectorStorage := aiconnectors.NewStorage(db)

	var botCfgs []BotConfig
	for _, cfg := range configs {
		connectors, err := connectorStorage.GetAllConnectors(context.Background(), cfg.OrgID)
		if err != nil || len(connectors) == 0 {
			log.Printf("Teams bot org %d: no AI connectors found, skipping", cfg.OrgID)
			continue
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
			log.Printf("Teams bot: all connectors for org %d failed to initialize — skipping", cfg.OrgID)
			continue
		}

		mcpHeaders := map[string]string{"X-API-Key": cfg.APIKey}

		botCfgs = append(botCfgs, BotConfig{
			OrgID:        cfg.OrgID,
			BotAppID:     cfg.BotAppID,
			BotPassword:  cfg.BotPassword,
			MCPServerURL: mcpServerURL,
			MCPHeaders:   mcpHeaders,
			Connector:    connector,
			MaxSteps:     maxSteps,
		})
	}

	if len(botCfgs) == 0 {
		return nil, fmt.Errorf("no orgs could be configured for Teams bot")
	}

	baseURL := os.Getenv("TEAMS_BOT_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8888"
	}

	return NewBot(context.Background(), botCfgs, baseURL), nil
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
		log.Printf("[TeamsBot] Error handling activity: %s", err)
		if errors.Is(err, ErrJWTValidationFailed) {
			return c.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
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
