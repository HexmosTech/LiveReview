package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/storage/tools"
)

// ListAvailableTools handles GET /api/v1/admin/tools
// Returns all tools in the catalog — used by the Settings UI.
// Tool catalog additions/modifications are managed strictly via lr-tools deployer manually.
func (s *Server) ListAvailableTools(c echo.Context) error {
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SAST tools are only available in cloud mode"})
	}

	store := tools.NewToolsStore(s.db)
	catalog, err := store.LoadCatalog(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, catalog)
}

// ListOrgTools handles GET /api/v1/orgs/:org_id/tools
// Returns the org's tool configuration views.
func (s *Server) ListOrgTools(c echo.Context) error {
	// Cloud gate check
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SAST tools are only available in cloud mode"})
	}

	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()

	store := tools.NewToolsStore(s.db)
	orgTools, err := store.GetAvailableToolsForOrg(c.Request().Context(), orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{"tools": orgTools})
}

// UpdateOrgTool handles PUT /api/v1/orgs/:org_id/tools/:tool_id
// Updates the enabled state of a specific tool for the organization.
func (s *Server) UpdateOrgTool(c echo.Context) error {
	// Cloud gate check
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SAST tools are only available in cloud mode"})
	}

	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}
	orgID := pc.GetOrgID()

	toolIDStr := c.Param("tool_id")
	toolID, err := strconv.ParseInt(toolIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid tool_id"})
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Enabled == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "enabled field is required"})
	}

	store := tools.NewToolsStore(s.db)
	row, err := store.UpsertOrgTool(c.Request().Context(), orgID, toolID, *req.Enabled)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, row)
}

// GetRepositoryTools handles GET /api/v1/repositories/:repoId/tools
// Returns the repository's PR-only static analysis tool configuration merged with org defaults.
func (s *Server) GetRepositoryTools(c echo.Context) error {
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SAST tools are only available in cloud mode"})
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}
	orgID := pc.GetOrgID()

	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil || repoID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid repoId"})
	}

	store := tools.NewToolsStore(s.db)
	mode, repoTools, err := store.GetAvailableToolsForRepo(c.Request().Context(), orgID, repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Repository not found"})
		}
		log.Printf("[ERROR] GetRepositoryTools(org=%d repo=%d): %v", orgID, repoID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
	}
	if repoTools == nil {
		repoTools = []tools.RepoToolView{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"repository_id": repoID,
		"mode":          mode,
		"tools":         repoTools,
	})
}

// UpdateRepositoryTools handles PUT /api/v1/repositories/:repoId/tools
// Updates a repository's PR-only static analysis tool mode ('inherit' or 'custom') and enabled tools.
func (s *Server) UpdateRepositoryTools(c echo.Context) error {
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SAST tools are only available in cloud mode"})
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required"})
	}
	if err := pc.RequireOrgOwner(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}
	orgID := pc.GetOrgID()

	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil || repoID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid repoId"})
	}

	var req struct {
		Mode        string                              `json:"mode"`
		Tools       map[string]bool                     `json:"tools,omitempty"`
		ToolConfigs map[string]tools.RepoToolRuleConfig `json:"tool_configs,omitempty"`
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != tools.RepoToolModeInherit && mode != tools.RepoToolModeCustom {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "mode must be 'inherit' or 'custom'"})
	}

	store := tools.NewToolsStore(s.db)
	ctx := c.Request().Context()
	if _, err := store.UpsertRepoToolSettingsWithConfigs(ctx, orgID, repoID, mode, req.Tools, req.ToolConfigs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "Repository not found"})
		}
		log.Printf("[ERROR] UpdateRepositoryTools(org=%d repo=%d): %v", orgID, repoID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
	}

	effectiveMode, repoTools, err := store.GetAvailableToolsForRepo(ctx, orgID, repoID)
	if err != nil {
		log.Printf("[ERROR] UpdateRepositoryTools reload(org=%d repo=%d): %v", orgID, repoID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
	}
	if repoTools == nil {
		repoTools = []tools.RepoToolView{}
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"repository_id": repoID,
		"mode":          effectiveMode,
		"tools":         repoTools,
	})
}
