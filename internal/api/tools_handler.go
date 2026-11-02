package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

	err := upsertAvailableTool(c.Request().Context(), s.db, req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok", "name": req.Name})
}

// ListAvailableTools handles GET /api/v1/admin/tools
// Returns all tools in the catalog — used by the Settings UI (Phase 2).
func (s *Server) ListAvailableTools(c echo.Context) error {
	store := tools.NewToolsStore(s.db)
	catalog, err := store.LoadCatalog(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, catalog)
}

func getNextToolID(ctx context.Context, db *sql.DB, catalog []tools.AvailableTool) int64 {
	var seqID int64
	err := db.QueryRowContext(ctx, "SELECT nextval('public.tools_catalog_id_seq')").Scan(&seqID)
	if err == nil {
		var maxID int64
		for _, t := range catalog {
			if t.ID > maxID {
				maxID = t.ID
			}
		}
		if seqID <= maxID {
			return maxID + 1
		}
		return seqID
	}
	var maxID int64
	for _, t := range catalog {
		if t.ID > maxID {
			maxID = t.ID
		}
	}
	return maxID + 1
}

func upsertAvailableTool(ctx context.Context, db *sql.DB, req UpsertToolRequest) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var data []byte
	err = tx.QueryRowContext(ctx, "SELECT data FROM public.system_settings WHERE name = 'tools_catalog' FOR UPDATE").Scan(&data)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("query tools_catalog: %w", err)
	}

	var catalog []tools.AvailableTool
	if len(data) > 0 {
		if err := json.Unmarshal(data, &catalog); err != nil {
			return fmt.Errorf("unmarshal tools_catalog: %w", err)
		}
	}

	found := false
	for i, t := range catalog {
		if strings.EqualFold(t.Name, req.Name) {
			catalog[i].Description = req.Description
			catalog[i].LambdaARN = req.LambdaARN
			catalog[i].Multiplier = req.Multiplier
			catalog[i].UseCase = req.UseCase
			found = true
			break
		}
	}
	if !found {
		newID := getNextToolID(ctx, db, catalog)
		catalog = append(catalog, tools.AvailableTool{
			ID:          newID,
			Name:        req.Name,
			Description: req.Description,
			LambdaARN:   req.LambdaARN,
			Multiplier:  req.Multiplier,
			UseCase:     req.UseCase,
		})
	}

	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO public.system_settings (name, data, updated_at)
		VALUES ('tools_catalog', $1, NOW())
		ON CONFLICT (name) DO UPDATE
		SET data = EXCLUDED.data,
		    updated_at = NOW()
	`, catalogJSON)
	if err != nil {
		return fmt.Errorf("upsert tools_catalog: %w", err)
	}

	return tx.Commit()
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
