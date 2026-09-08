package api

import (
	"database/sql"
	"log"
	"net/http"
	"regexp"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/teamsbot"
)

// azureGUIDRe matches Azure App IDs and Tenant IDs, which are always GUIDs.
// Rejecting anything else here (e.g. a browser-autofilled email that made
// it past client-side validation) stops garbage from ever reaching the
// Teams bot's live token acquisition or the generated app manifest.
var azureGUIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// TeamsConfigHandler handles REST CRUD for Teams bot configs.
type TeamsConfigHandler struct {
	db        *sql.DB
	storage   *teamsbot.Storage
	apiKeys   *APIKeyManager
	onSaved   func(orgID int64) // (re)connects the Teams bot for orgID live, no restart needed
	onDeleted func(orgID int64) // disconnects the live Teams bot for orgID, if running
}

// NewTeamsConfigHandler wires onSaved/onDeleted so that saving or deleting an
// org's Teams config takes effect on the running server immediately -
// without these, a config change only takes effect on the next server
// restart (see (*Server).syncTeamsBotForOrg / removeTeamsBotForOrg).
func NewTeamsConfigHandler(db *sql.DB, onSaved, onDeleted func(orgID int64)) *TeamsConfigHandler {
	return &TeamsConfigHandler{
		db:        db,
		storage:   teamsbot.NewStorage(db),
		apiKeys:   NewAPIKeyManager(db),
		onSaved:   onSaved,
		onDeleted: onDeleted,
	}
}

type TeamsConfigResponse struct {
	Configured bool   `json:"configured"`
	BotAppID   string `json:"bot_app_id,omitempty"`
	TenantID   string `json:"tenant_id,omitempty"`
}

type TeamsConfigUpdateRequest struct {
	BotAppID    string `json:"bot_app_id"`
	BotPassword string `json:"bot_password"`
	// TenantID is optional: empty keeps the bot on the legacy fixed
	// multi-tenant token endpoint (see teamsbot.tokenURLFor), so existing
	// configs saved before Single Tenant support was added keep working
	// unchanged. The Settings UI requires it for new Single Tenant setups.
	TenantID string `json:"tenant_id"`
}

func (h *TeamsConfigHandler) GetTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	cfg, err := h.storage.GetTeamsConfig(c.Request().Context(), permCtx.OrgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, TeamsConfigResponse{Configured: false})
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to get Teams config")
	}

	return c.JSON(http.StatusOK, TeamsConfigResponse{
		Configured: true,
		BotAppID:   cfg.BotAppID,
		TenantID:   cfg.TenantID,
	})
}

// bindAndValidateTeamsConfigRequest binds the request body and runs the
// format checks (required fields, GUID shape) shared by UpdateTeamsConfig
// and ValidateTeamsConfig. It does NOT call teamsbot.ValidateCredentials -
// callers do that themselves, since UpdateTeamsConfig treats a credential
// failure as a hard error while ValidateTeamsConfig reports it as a normal
// {valid: false} response instead of an HTTP error.
func bindAndValidateTeamsConfigRequest(c echo.Context) (TeamsConfigUpdateRequest, error) {
	var req TeamsConfigUpdateRequest
	if err := c.Bind(&req); err != nil {
		return req, echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	if req.BotAppID == "" || req.BotPassword == "" {
		return req, echo.NewHTTPError(http.StatusBadRequest, "bot_app_id and bot_password are required")
	}
	if !azureGUIDRe.MatchString(req.BotAppID) {
		return req, echo.NewHTTPError(http.StatusBadRequest, "bot_app_id must be a GUID (the Azure Bot resource's Microsoft App ID)")
	}
	if req.TenantID != "" && !azureGUIDRe.MatchString(req.TenantID) {
		return req, echo.NewHTTPError(http.StatusBadRequest, "tenant_id must be a GUID (the Azure Bot resource's Directory/Tenant ID)")
	}
	return req, nil
}

func (h *TeamsConfigHandler) UpdateTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can configure Teams integration")
	}

	req, err := bindAndValidateTeamsConfigRequest(c)
	if err != nil {
		return err
	}

	if err := teamsbot.ValidateCredentials(c.Request().Context(), req.BotAppID, req.BotPassword, req.TenantID); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Microsoft rejected these credentials: "+err.Error())
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

	cfg, err := h.storage.UpsertTeamsConfig(c.Request().Context(), permCtx.OrgID, req.BotAppID, req.BotPassword, apiKey, req.TenantID)
	if err != nil {
		log.Printf("[TeamsConfig] Failed to save config for org %d: %s", permCtx.OrgID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save config")
	}

	log.Printf("[TeamsConfig] Org %d: Teams bot configured with app ID %s", permCtx.OrgID, req.BotAppID)

	if h.onSaved != nil {
		h.onSaved(permCtx.OrgID)
	}

	return c.JSON(http.StatusOK, TeamsConfigResponse{
		Configured: true,
		BotAppID:   cfg.BotAppID,
		TenantID:   cfg.TenantID,
	})
}

// ValidateTeamsConfig performs a real Entra ID OAuth token request with the
// given credentials, without saving anything, so the UI can show a Verify
// result before Save. UpdateTeamsConfig also runs this same check on Save
// itself, so this endpoint exists purely for earlier UX feedback - it is
// not the only place credentials get validated.
func (h *TeamsConfigHandler) ValidateTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}
	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can configure Teams integration")
	}

	req, err := bindAndValidateTeamsConfigRequest(c)
	if err != nil {
		return err
	}

	if err := teamsbot.ValidateCredentials(c.Request().Context(), req.BotAppID, req.BotPassword, req.TenantID); err != nil {
		return c.JSON(http.StatusOK, map[string]any{"valid": false, "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"valid": true})
}

// DownloadAppPackage generates and streams the Teams app package (a zip
// containing manifest.json + icons) for this org's already-saved bot, so an
// admin can upload it to Teams Admin Center. Requires the Teams config to
// already be saved (the manifest needs a real Bot App ID). Open to any org
// member - the package contains no secrets, only the already-visible Bot
// App ID.
func (h *TeamsConfigHandler) DownloadAppPackage(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	cfg, err := h.storage.GetTeamsConfig(c.Request().Context(), permCtx.OrgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return echo.NewHTTPError(http.StatusBadRequest, "configure the Teams bot before downloading the app package")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to load Teams config")
	}

	publicURL, err := teamsbot.ResolveInstancePublicURL(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	pkg, err := teamsbot.BuildAppPackage(cfg, publicURL)
	if err != nil {
		log.Printf("[TeamsConfig] Failed to build app package for org %d: %s", permCtx.OrgID, err)
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to build Teams app package")
	}

	c.Response().Header().Set(echo.HeaderContentDisposition, `attachment; filename="livi-teams-app.zip"`)
	return c.Blob(http.StatusOK, "application/zip", pkg)
}

func (h *TeamsConfigHandler) DeleteTeamsConfig(c echo.Context) error {
	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}

	if permCtx.Role != "owner" && permCtx.Role != "super_admin" {
		return echo.NewHTTPError(http.StatusForbidden, "only owners can delete Teams integration")
	}

	ctx := c.Request().Context()

	existing, err := h.storage.GetTeamsConfig(ctx, permCtx.OrgID)
	if err == nil && existing != nil && existing.APIKey != "" {
		if err := h.apiKeys.RevokeAPIKeyByPlainKey(ctx, existing.APIKey); err != nil {
			log.Printf("[TeamsConfig] Failed to revoke API key for org %d: %s", permCtx.OrgID, err)
		}
	}

	if err := h.storage.DeleteTeamsConfig(ctx, permCtx.OrgID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to delete Teams config")
	}

	if h.onDeleted != nil {
		h.onDeleted(permCtx.OrgID)
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "deleted"})
}
