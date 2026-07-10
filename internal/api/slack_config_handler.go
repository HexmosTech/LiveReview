package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/slackbot"
)

type SlackConfigHandler struct {
	storage *slackbot.Storage
}

func NewSlackConfigHandler(db *sql.DB) *SlackConfigHandler {
	return &SlackConfigHandler{storage: slackbot.NewStorage(db)}
}

// GetSlackConfig returns the org's slack bot configuration (without secrets).
func (h *SlackConfigHandler) GetSlackConfig(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid org_id")
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}
	if pc.OrgID != orgID {
		return echo.NewHTTPError(http.StatusForbidden, "org mismatch")
	}

	cfg, err := h.storage.GetSlackConfig(c.Request().Context(), orgID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusOK, map[string]any{"configured": false})
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to read slack config")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"configured": true,
		"id":         cfg.ID,
		"org_id":     cfg.OrgID,
		"team_id":    cfg.TeamID,
		"enabled":    cfg.Enabled,
		"created_at": cfg.CreatedAt,
		"updated_at": cfg.UpdatedAt,
	})
}

// PutSlackConfig creates or updates the org's slack bot configuration.
func (h *SlackConfigHandler) PutSlackConfig(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid org_id")
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}
	if !pc.IsSuperAdmin && (pc.OrgID != orgID || pc.Role != "owner") {
		return echo.NewHTTPError(http.StatusForbidden, "owner or super admin privileges required")
	}

	var req struct {
		BotToken string `json:"bot_token"`
		APIKey   string `json:"api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.BotToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_token is required")
	}
	if req.APIKey == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "api_key is required")
	}

	cfg, err := h.storage.UpsertSlackConfig(c.Request().Context(), orgID, req.BotToken, req.APIKey)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save slack config")
	}

	return c.JSON(http.StatusOK, map[string]any{
		"configured": true,
		"id":         cfg.ID,
		"org_id":     cfg.OrgID,
		"team_id":    cfg.TeamID,
		"enabled":    cfg.Enabled,
		"created_at": cfg.CreatedAt,
		"updated_at": cfg.UpdatedAt,
	})
}

// DeleteSlackConfig removes the org's slack bot configuration.
func (h *SlackConfigHandler) DeleteSlackConfig(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid org_id")
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}
	if !pc.IsSuperAdmin && (pc.OrgID != orgID || pc.Role != "owner") {
		return echo.NewHTTPError(http.StatusForbidden, "owner or super admin privileges required")
	}

	if err := h.storage.DeleteSlackConfig(c.Request().Context(), orgID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete slack config")
	}

	return c.NoContent(http.StatusNoContent)
}
