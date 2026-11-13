package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/storage/tools"
)

// ListAvailableTools handles GET /api/v1/admin/tools
// Returns all tools in the catalog — used by the Settings UI.
// Tool catalog additions/modifications are managed strictly via lr-tools deployer manually.
func (s *Server) ListAvailableTools(c echo.Context) error {
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Third-party tools are only available in cloud mode"})
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
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Third-party tools are only available in cloud mode"})
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
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Third-party tools are only available in cloud mode"})
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
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "tool not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, row)
}
