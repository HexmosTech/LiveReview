package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/storage/tools"
)

// UpsertToolRequest is the payload for POST /api/v1/admin/tools
// Called by `make register-tools` in lr-tools after Lambda deployment.
type UpsertToolRequest struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	LambdaARN   string  `json:"lambda_arn"`
	Multiplier  float64 `json:"multiplier"`
	UseCase     string  `json:"use_case"`
}

// UpsertAvailableTool handles POST /api/v1/admin/tools
// Inserts or updates a tool in the available_tools catalog.
// Super-admin only — called by the lr-tools deployer after Lambda deployment.
func (s *Server) UpsertAvailableTool(c echo.Context) error {
	var req UpsertToolRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.Name == "" || req.LambdaARN == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name and lambda_arn are required"})
	}
	if req.Multiplier <= 0 {
		req.Multiplier = 1.0
	}

	err := upsertAvailableTool(s.db, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "name": req.Name})
}

// ListAvailableTools handles GET /api/v1/admin/tools
// Returns all tools in the catalog — used by the Settings UI (Phase 2).
func (s *Server) ListAvailableTools(c echo.Context) error {
	type ToolRow struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		LambdaARN   string  `json:"lambda_arn"`
		Multiplier  float64 `json:"multiplier"`
		UseCase     string  `json:"use_case"`
	}

	rows, err := s.db.QueryContext(c.Request().Context(),
		`SELECT id, name, description, lambda_arn, multiplier, use_case
		   FROM available_tools
		  ORDER BY name`,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	defer rows.Close()

	tools := make([]ToolRow, 0)
	for rows.Next() {
		var t ToolRow
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.LambdaARN, &t.Multiplier, &t.UseCase); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		tools = append(tools, t)
	}
	return c.JSON(http.StatusOK, tools)
}

func upsertAvailableTool(db *sql.DB, req UpsertToolRequest) error {
	_, err := db.Exec(`
		INSERT INTO available_tools (name, description, lambda_arn, multiplier, use_case)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO UPDATE
		  SET description = EXCLUDED.description,
		      lambda_arn  = EXCLUDED.lambda_arn,
		      multiplier  = EXCLUDED.multiplier,
		      use_case    = EXCLUDED.use_case`,
		req.Name, req.Description, req.LambdaARN, req.Multiplier, req.UseCase,
	)
	return err
}

// ListOrgTools handles GET /api/v1/orgs/:org_id/tools
// Returns the org's tool configuration views.
func (s *Server) ListOrgTools(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()

	// Cloud gate check
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Third-party tools are only available in cloud mode"})
	}

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
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": err.Error()})
	}
	orgID := pc.GetOrgID()

	// Cloud gate check
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Third-party tools are only available in cloud mode"})
	}

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
