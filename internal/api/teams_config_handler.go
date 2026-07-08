package api

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/teamsbot"
)

// TeamsConfigHandler handles REST CRUD for Teams bot configs.
type TeamsConfigHandler struct {
	storage   *teamsbot.Storage
	apiKeys   *APIKeyManager
}

func NewTeamsConfigHandler(db *sql.DB) *TeamsConfigHandler {
	return &TeamsConfigHandler{
		storage:   teamsbot.NewStorage(db),
		apiKeys:   NewAPIKeyManager(db),
	}
}

type teamsConfigResponse struct {
	Configured bool   `json:"configured"`
	BotAppID   string `json:"bot_app_id,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
}

type teamsConfigUpdateRequest struct {
	BotAppID    string `json:"bot_app_id"`
	BotPassword string `json:"bot_password"`
}

func (h *TeamsConfigHandler) GetTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	cfg, err := h.storage.GetTeamsConfig(c.Request().Context(), permCtx.OrgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, teamsConfigResponse{Configured: false})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get Teams config")
	}

	return c.JSON(http.StatusOK, teamsConfigResponse{
		Configured: true,
		BotAppID:   cfg.BotAppID,
		TenantID:   cfg.TenantID,
	})
}

func (h *TeamsConfigHandler) UpdateTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can configure Teams integration")
	}

	var req teamsConfigUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.BotAppID == "" || req.BotPassword == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_app_id and bot_password are required")
	}

	userID := permCtx.GetUserID()

	apiKey := ""
	existing, err := h.storage.GetTeamsConfig(c.Request().Context(), permCtx.OrgID)
	if err == nil && existing != nil {
		apiKey = existing.APIKey
	}
	if apiKey == "" {
		_, plainKey, err := h.apiKeys.CreateAPIKey(userID, permCtx.OrgID, "teams-bot", []string{}, nil)
		if err != nil {
			log.Printf("[TeamsConfig] Failed to generate API key for org %d: %s", permCtx.OrgID, err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate API key")
		}
		apiKey = plainKey
	}

	cfg, err := h.storage.UpsertTeamsConfig(c.Request().Context(), permCtx.OrgID, req.BotAppID, req.BotPassword, apiKey)
	if err != nil {
		log.Printf("[TeamsConfig] Failed to save config for org %d: %s", permCtx.OrgID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config")
	}

	log.Printf("[TeamsConfig] Org %d: Teams bot configured with app ID %s", permCtx.OrgID, req.BotAppID)

	return c.JSON(http.StatusOK, teamsConfigResponse{
		Configured: true,
		BotAppID:   cfg.BotAppID,
	})
}

func (h *TeamsConfigHandler) DeleteTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can delete Teams integration")
	}

	if err := h.storage.DeleteTeamsConfig(c.Request().Context(), permCtx.OrgID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete Teams config")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
