package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	scheduledreviewstore "github.com/livereview/storage/scheduledreview"
)

// defaultScheduledReviewCronExpression is used when a repo is enabled without an explicit schedule (UTC).
const defaultScheduledReviewCronExpression = "0 9 * * *"

// ScheduledReviewConfigResponse is the wire shape for a single repo's scheduled-review config.
type ScheduledReviewConfigResponse struct {
	ID              int64   `json:"id"`
	RepositoryID    int64   `json:"repository_id"`
	ProjectFullName string  `json:"project_full_name"`
	Enabled         bool    `json:"enabled"`
	CronExpression  string  `json:"cron_expression"`
	LastRunAt       *string `json:"last_run_at,omitempty"`
	NextRunAt       string  `json:"next_run_at"`
}

func toScheduledReviewConfigResponse(cfg *scheduledreviewstore.Config) ScheduledReviewConfigResponse {
	resp := ScheduledReviewConfigResponse{
		ID:              cfg.ID,
		RepositoryID:    cfg.RepositoryID,
		ProjectFullName: cfg.ProjectFullName,
		Enabled:         cfg.Enabled,
		CronExpression:  cfg.CronExpression,
		NextRunAt:       cfg.NextRunAt.UTC().Format(time.RFC3339),
	}
	if cfg.LastRunAt.Valid {
		formatted := cfg.LastRunAt.Time.UTC().Format(time.RFC3339)
		resp.LastRunAt = &formatted
	}
	return resp
}

// GetScheduledReviewConfigs lists the scheduled-review configuration for every repo under a connector.
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

// SetScheduledReviewRequest is the body for enabling/disabling/(re)scheduling a repo; CronExpression is UTC.
type SetScheduledReviewRequest struct {
	RepositoryID   int64  `json:"repository_id"`
	Enabled        bool   `json:"enabled"`
	CronExpression string `json:"cron_expression"`
}

// SetScheduledReview enables, disables, or reschedules the periodic default-branch review for a repo.
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
	if req.RepositoryID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "repository_id is required"})
	}

	if _, ok := auth.GetOrgIDFromContext(c); !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "Org context not found"})
	}

	cronExpr := strings.TrimSpace(req.CronExpression)
	if cronExpr == "" {
		cronExpr = defaultScheduledReviewCronExpression
	}
	if err := scheduledreviewstore.ParseCronExpression(cronExpr); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	ctx := c.Request().Context()

	// repository_id comes from the client, so this is the authorization boundary (same pattern as GetRepository).
	var repoConnectorID int64
	err = s.db.QueryRowContext(ctx, `SELECT connector_id FROM repositories WHERE id = $1 AND org_id = $2`, req.RepositoryID, orgID).Scan(&repoConnectorID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Repository not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to look up repository"})
	}
	if repoConnectorID != int64(connectorID) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Repository does not belong to this connector"})
	}

	store := scheduledreviewstore.NewStore(s.db)

	existing, err := store.GetByRepository(ctx, req.RepositoryID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to load existing scheduled review config"})
	}

	// next_run_at only needs recomputing on a real create/enable/cron-change; otherwise keep the existing value.
	now := time.Now().UTC()
	nextRunAt := now
	switch {
	case existing == nil:
		nextRunAt = now
	case !existing.Enabled && req.Enabled:
		nextRunAt = now
	case existing.CronExpression != cronExpr && req.Enabled:
		nextRunAt, err = scheduledreviewstore.NextRunAfter(cronExpr, now)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
		}
	default:
		nextRunAt = existing.NextRunAt
	}

	cfg, err := store.Upsert(ctx, orgID, req.RepositoryID, req.Enabled, cronExpr, nextRunAt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update scheduled review config"})
	}

	// Non-blocking nudge so the scheduler rechecks immediately instead of possibly sleeping past this change.
	if s.scheduledReviewWakeCh != nil {
		select {
		case s.scheduledReviewWakeCh <- struct{}{}:
		default:
		}
	}

	return c.JSON(http.StatusOK, toScheduledReviewConfigResponse(cfg))
}
