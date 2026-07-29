package api

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/discordbot"
)

type DiscordConfigHandler struct {
	storage *discordbot.Storage
	apiKeys *APIKeyManager
}

func NewDiscordConfigHandler(db *sql.DB) *DiscordConfigHandler {
	return &DiscordConfigHandler{
		storage: discordbot.NewStorage(db),
		apiKeys: NewAPIKeyManager(db),
	}
}

type DiscordConfigResponse struct {
	Configured bool   `json:"configured"`
	GuildID    string `json:"guild_id,omitempty"`
}

type DiscordConfigUpdateRequest struct {
	BotToken string `json:"bot_token"`
}

func (h *DiscordConfigHandler) GetDiscordConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	cfg, err := h.storage.GetDiscordConfig(c.Request().Context(), permCtx.OrgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, DiscordConfigResponse{Configured: false})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get Discord config")
	}

	return c.JSON(http.StatusOK, DiscordConfigResponse{
		Configured: true,
		GuildID:    cfg.GuildID,
	})
}

func (h *DiscordConfigHandler) UpdateDiscordConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can configure Discord integration")
	}

	var req DiscordConfigUpdateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.BotToken == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_token is required")
	}

	userID := permCtx.GetUserID()

	apiKey := ""
	existing, err := h.storage.GetDiscordConfig(c.Request().Context(), permCtx.OrgID)
	if err == nil && existing != nil {
		apiKey = existing.APIKey
	}
	if apiKey == "" {
		_, plainKey, err := h.apiKeys.CreateAPIKey(userID, permCtx.OrgID, "discord-bot", []string{}, nil)
		if err != nil {
			log.Printf("[DiscordConfig] Failed to generate API key for org %d: %s", permCtx.OrgID, err)
			return echo.NewHTTPError(http.StatusInternalServerError, "failed to generate API key")
		}
		apiKey = plainKey
	}

	cfg, err := h.storage.UpsertDiscordConfig(c.Request().Context(), permCtx.OrgID, req.BotToken, apiKey)
	if err != nil {
		log.Printf("[DiscordConfig] Failed to save config for org %d: %s", permCtx.OrgID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config")
	}

	log.Printf("[DiscordConfig] Org %d: Discord bot configured", permCtx.OrgID)

	return c.JSON(http.StatusOK, DiscordConfigResponse{
		Configured: true,
		GuildID:    cfg.GuildID,
	})
}

func (h *DiscordConfigHandler) DeleteDiscordConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can delete Discord integration")
	}

	ctx := c.Request().Context()

	existing, err := h.storage.GetDiscordConfig(ctx, permCtx.OrgID)
	if err == nil && existing != nil && existing.APIKey != "" {
		if err := h.apiKeys.RevokeAPIKeyByPlainKey(ctx, existing.APIKey); err != nil {
			log.Printf("[DiscordConfig] Failed to revoke API key for org %d: %s", permCtx.OrgID, err)
		}
	}

	if err := h.storage.DeleteDiscordConfig(ctx, permCtx.OrgID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete Discord config")
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
