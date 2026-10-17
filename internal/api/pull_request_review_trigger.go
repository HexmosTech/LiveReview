package api

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
)

// CreateReviewRequest is the body of POST /api/v1/reviews - trigger a new
// review against a known pull_requests row, as an alternative to
// POST /connectors/trigger-review's raw-URL flow.
type CreateReviewRequest struct {
	PullRequestID int64 `json:"pull_request_id"`
	// FromPRLink is set by the PR comment's "Start a review" link: reuse a review still
	// running from the last 10 minutes, and run the approval policy check when it finishes.
	FromPRLink bool `json:"from_pr_link"`
}

// createReview handles POST /api/v1/reviews. It resolves the PR/MR's web_url
// from the pull_requests table (org-scoped) and reuses TriggerReviewV2's
// phases unchanged (setupReviewContext -> prepareAuthentication ->
// createReviewRequest -> enrichMetadata -> trackActivity ->
// launchBackgroundProcessing), so both trigger entrypoints share identical
// authentication, LOC-quota, and background-processing behavior. The only
// addition is passing the already-known pull_request_id through so setupReviewContext
// links reviews.pull_request_id atomically at insert time.
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

	if req.FromPRLink {
		var ongoingID int64
		err := s.db.QueryRow(
			`SELECT id FROM reviews
			 WHERE pull_request_id = $1 AND org_id = $2
			   AND status IN ('created', 'in_progress')
			   AND created_at > now() - interval '10 minutes'
			 ORDER BY created_at DESC LIMIT 1`,
			req.PullRequestID, orgID,
		).Scan(&ongoingID)
		if err == nil {
			return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
				"message":         "Review already in progress",
				"url":             webURL,
				"reviewId":        fmt.Sprintf("%d", ongoingID),
				"pull_request_id": req.PullRequestID,
				"reused":          true,
			})
		}
		if err != sql.ErrNoRows {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to check ongoing review: %v", err)})
		}
	}

	// LOC Quota preflight check — block before creating any DB records, same
	// as TriggerReviewV2.
	if blocked, pfErr := s.preflightLOCQuota(c); blocked {
		return pfErr
	}

	ctx, err := s.setupReviewContext(c, TriggerReviewRequest{URL: webURL, PullRequestID: &req.PullRequestID})
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if err := s.prepareAuthentication(ctx); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}
	if err := s.createReviewRequest(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}
	s.enrichMetadata(ctx)
	s.markForPolicyCheck(ctx.review.ID, orgID)
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

// getPullRequestOrg handles GET /api/v1/pull-requests/:id/org for the PR comment's
// "Start a review" link, which doesn't know the PR's org yet. Auth-only route, so it
// checks membership itself and returns 404 for both "no such PR" and "not a member".
func (s *Server) getPullRequestOrg(c echo.Context) error {
	user, ok := c.Get(string(auth.UserContextKey)).(*models.User)
	if !ok || user == nil {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "authentication required"})
	}
	prID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || prID <= 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid pull request id"})
	}

	// Same access rule as ValidateOrgAccess: a role in the org, or super admin.
	var orgID int64
	err = s.db.QueryRow(
		`SELECT pr.org_id FROM pull_requests pr
		 WHERE pr.id = $1 AND EXISTS (
			SELECT 1 FROM user_roles ur JOIN roles r ON ur.role_id = r.id
			WHERE ur.user_id = $2 AND (ur.org_id = pr.org_id OR r.name = $3)
		 )`,
		prID, user.ID, auth.RoleSuperAdmin,
	).Scan(&orgID)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, ErrorResponse{Error: "pull request not found"})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "failed to look up pull request"})
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"org_id": orgID})
}

// markForPolicyCheck flags a manual PR review so the worker runs the approval policy check once it completes.
// Reviews not linked to a known PR are left alone.
func (s *Server) markForPolicyCheck(reviewID, orgID int64) {
	if _, err := s.db.Exec(
		`UPDATE reviews SET metadata = COALESCE(metadata, '{}'::jsonb) || '{"pr_link_policy": true}'::jsonb
		 WHERE id = $1 AND org_id = $2 AND pull_request_id IS NOT NULL`,
		reviewID, orgID,
	); err != nil {
		log.Printf("[WARN] failed to mark review %d for policy check: %v", reviewID, err)
	}
}
