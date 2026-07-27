package api

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// CreateReviewRequest is the body of POST /api/v1/reviews - trigger a new
// review against a known pull_requests row, as an alternative to
// POST /connectors/trigger-review's raw-URL flow.
type CreateReviewRequest struct {
	PullRequestID int64 `json:"pull_request_id"`
}

// createReview handles POST /api/v1/reviews. It resolves the PR/MR's web_url
// from the pull_requests table (org-scoped) and reuses TriggerReviewV2's
// phases unchanged (setupReviewContext -> prepareAuthentication ->
// createReviewRequest -> enrichMetadata -> trackActivity ->
// launchBackgroundProcessing), so both trigger entrypoints share identical
// authentication, LOC-quota, and background-processing behavior. The only
// addition is linking the created review back to its canonical PR/MR row via
// reviews.pull_request_id.
func (s *Server) createReview(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Missing organization context"})
	}

	var req CreateReviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
	}
	if req.PullRequestID <= 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "pull_request_id is required"})
	}

	var webURL string
	err := s.db.QueryRow(
		`SELECT pr.web_url FROM pull_requests pr WHERE pr.id = $1 AND pr.org_id = $2`,
		req.PullRequestID, orgID,
	).Scan(&webURL)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "pull request not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to look up pull request: %v", err)})
	}

	// LOC Quota preflight check — block before creating any DB records, same
	// as TriggerReviewV2.
	if blocked, pfErr := s.preflightLOCQuota(c); blocked {
		return pfErr
	}

	ctx, err := s.setupReviewContext(c, TriggerReviewRequest{URL: webURL})
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if _, err := s.db.Exec(`UPDATE reviews SET pull_request_id = $1 WHERE id = $2`, req.PullRequestID, ctx.review.ID); err != nil {
		// Non-fatal: the review can still proceed without the PR link: it will
		// simply not show up grouped under the PR's review history.
		log.Printf("[WARN] Failed to link review %d to pull_request_id %d: %v", ctx.review.ID, req.PullRequestID, err)
	}

	if err := s.prepareAuthentication(ctx); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.createReviewRequest(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.enrichMetadata(ctx)
	s.trackActivity(ctx)
	if err := s.launchBackgroundProcessing(ctx); err != nil {
		log.Printf("[ERROR] createReview: Failed to queue manual review: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to queue review: %v", err)})
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"message":         "Review triggered successfully",
		"url":             ctx.requestURL,
		"reviewId":        ctx.reviewID,
		"pull_request_id": req.PullRequestID,
	})
}
