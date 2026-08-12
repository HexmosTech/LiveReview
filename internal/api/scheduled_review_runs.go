package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	scheduledreviewstore "github.com/livereview/storage/scheduledreview"
)

// ScheduledReviewRunResponse is one scheduler attempt for a repo's schedule.
type ScheduledReviewRunResponse struct {
	ID           int64   `json:"id"`
	ReviewID     *int64  `json:"review_id,omitempty"`
	Outcome      string  `json:"outcome"`
	Branch       string  `json:"branch,omitempty"`
	BaseSHA      string  `json:"base_sha,omitempty"`
	HeadSHA      string  `json:"head_sha,omitempty"`
	CommitCount  int     `json:"commit_count"`
	ErrorMessage string  `json:"error_message,omitempty"`
	StartedAt    string  `json:"started_at"`
	CompletedAt  *string `json:"completed_at,omitempty"`
}

// ScheduledReviewRunsListResponse wraps a paginated run-history listing.
type ScheduledReviewRunsListResponse struct {
	Runs       []ScheduledReviewRunResponse `json:"runs"`
	Total      int                          `json:"total"`
	Page       int                          `json:"page"`
	PerPage    int                          `json:"per_page"`
	TotalPages int                          `json:"total_pages"`
}

func toScheduledReviewRunResponse(r *scheduledreviewstore.Run) ScheduledReviewRunResponse {
	resp := ScheduledReviewRunResponse{
		ID:           r.ID,
		Outcome:      string(r.Outcome),
		Branch:       r.Branch.String,
		BaseSHA:      r.BaseSHA.String,
		HeadSHA:      r.HeadSHA.String,
		CommitCount:  r.CommitCount,
		ErrorMessage: r.ErrorMessage.String,
		StartedAt:    r.StartedAt.UTC().Format(time.RFC3339),
	}
	if r.ReviewID.Valid {
		id := r.ReviewID.Int64
		resp.ReviewID = &id
	}
	if r.CompletedAt.Valid {
		formatted := r.CompletedAt.Time.UTC().Format(time.RFC3339)
		resp.CompletedAt = &formatted
	}
	return resp
}

// GetScheduledReviewRuns handles GET /api/v1/repositories/:repoId/scheduled-review-runs.
func (s *Server) GetScheduledReviewRuns(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}
	repoID, err := strconv.ParseInt(c.Param("repoId"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid repository ID")
	}

	var exists bool
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM repositories WHERE id = $1 AND org_id = $2)`, repoID, orgID).Scan(&exists); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to look up repository")
	}
	if !exists {
		return echo.NewHTTPError(http.StatusNotFound, "Repository not found")
	}

	page, perPage := parsePageParams(c)
	ctx := c.Request().Context()

	store := scheduledreviewstore.NewStore(s.db)
	cfg, err := store.GetByRepository(ctx, repoID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to load scheduled review config")
	}
	if cfg == nil {
		return c.JSON(http.StatusOK, ScheduledReviewRunsListResponse{Runs: []ScheduledReviewRunResponse{}, Page: page, PerPage: perPage})
	}

	var outcomes []string
	if v := c.QueryParam("outcome"); v != "" {
		for _, o := range strings.Split(v, ",") {
			if o = strings.TrimSpace(o); o != "" {
				outcomes = append(outcomes, o)
			}
		}
	}
	desc := strings.ToLower(c.QueryParam("order")) != "asc"

	runs, total, err := store.ListRuns(ctx, scheduledreviewstore.ListRunsParams{
		ConfigID: cfg.ID,
		Outcomes: outcomes,
		Desc:     desc,
		Limit:    perPage,
		Offset:   (page - 1) * perPage,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to load run history")
	}

	out := make([]ScheduledReviewRunResponse, 0, len(runs))
	for _, r := range runs {
		out = append(out, toScheduledReviewRunResponse(r))
	}

	return c.JSON(http.StatusOK, ScheduledReviewRunsListResponse{
		Runs:       out,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages(total, perPage),
	})
}
