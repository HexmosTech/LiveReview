package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	scheduledreviewstore "github.com/livereview/storage/scheduledreview"
)

// defaultScheduledReviewIntervalHours is the only interval offered in this iteration; the
// review runs once every 24 hours from the last checkpoint.
const defaultScheduledReviewIntervalHours = 24

// ScheduledReviewConfigResponse is the wire shape for a single repo's scheduled-review config.
type ScheduledReviewConfigResponse struct {
	ID              int64   `json:"id"`
	ProjectFullName string  `json:"project_full_name"`
	Enabled         bool    `json:"enabled"`
	IntervalHours   int     `json:"interval_hours"`
	LastRunAt       *string `json:"last_run_at,omitempty"`
	NextRunAt       string  `json:"next_run_at"`
}

func toScheduledReviewConfigResponse(cfg *scheduledreviewstore.Config) ScheduledReviewConfigResponse {
	resp := ScheduledReviewConfigResponse{
		ID:              cfg.ID,
		ProjectFullName: cfg.ProjectFullName,
		Enabled:         cfg.Enabled,
		IntervalHours:   cfg.IntervalHours,
		NextRunAt:       cfg.NextRunAt.UTC().Format(time.RFC3339),
	}
	if cfg.LastRunAt.Valid {
		formatted := cfg.LastRunAt.Time.UTC().Format(time.RFC3339)
		resp.LastRunAt = &formatted
	}
	return resp
}

// GetScheduledReviewConfigs lists the scheduled-review configuration for every repo under a
// connector, for display next to each repo's webhook status in the connector detail UI.
func (s *Server) GetScheduledReviewConfigs(c echo.Context) error {
	connectorIDStr := c.Param("connectorId")
	connectorID, err := strconv.Atoi(connectorIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid connector ID"})
	}

	if _, err := s.validateConnectorOwnership(c, connectorID); err != nil {
		return err
	}

	store := scheduledreviewstore.NewStore(s.db)
	configs, err := store.ListByConnector(c.Request().Context(), int64(connectorID))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load scheduled review configs"})
	}

	out := make([]ScheduledReviewConfigResponse, 0, len(configs))
	for _, cfg := range configs {
		out = append(out, toScheduledReviewConfigResponse(cfg))
	}
	return c.JSON(http.StatusOK, out)
}

// SetScheduledReviewRequest is the body for enabling/disabling scheduled review on a repo.
type SetScheduledReviewRequest struct {
	ProjectPath string `json:"project_path"`
	Enabled     bool   `json:"enabled"`
}

// SetScheduledReview enables or disables the periodic default-branch review for a single
// repo under a connector. Interval is fixed at 24 hours for this iteration.
func (s *Server) SetScheduledReview(c echo.Context) error {
	connectorIDStr := c.Param("connectorId")
	connectorID, err := strconv.Atoi(connectorIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid connector ID"})
	}

	orgID, err := s.validateConnectorOwnership(c, connectorID)
	if err != nil {
		return err
	}

	var req SetScheduledReviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	projectPath := strings.TrimSpace(req.ProjectPath)
	if projectPath == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "project_path is required"})
	}

	if _, ok := auth.GetOrgIDFromContext(c); !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Org context not found"})
	}

	store := scheduledreviewstore.NewStore(s.db)
	cfg, err := store.Upsert(c.Request().Context(), orgID, int64(connectorID), projectPath, req.Enabled, defaultScheduledReviewIntervalHours)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update scheduled review config"})
	}

	return c.JSON(http.StatusOK, toScheduledReviewConfigResponse(cfg))
}
