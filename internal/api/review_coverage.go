package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// ReviewCoverageRequest is the input to POST /api/v1/review-coverage: a
// single flat array mixing bare commit SHAs and literal range expressions
// (e.g. "a..b"). Matching is exact-string against review_commits.ref; the
// backend never expands a range itself (no repo checkout to walk ancestry
// with) — a caller that wants exact per-commit answers should expand ranges
// client-side (e.g. via `git rev-list`) before calling.
type ReviewCoverageRequest struct {
	Commits []string `json:"commits"`
}

// maxReviewCoverageCommits bounds the size of a single lookup so a caller
// can't force an unbounded `= ANY($2)` scan (or an unbounded response) in
// one request. A CI gate checking a deploy's commit range is expected to
// stay well under this; callers with more should batch.
const maxReviewCoverageCommits = 2000

// ReviewCoverageReport is one database-backed review record matching a
// requested ref. Several reports can (and often will) exist for the same
// ref — e.g. a commit reviewed once via git-lrc's --commit flow and again
// later on its PR — this endpoint does not dedupe or collapse them.
type ReviewCoverageReport struct {
	Ref         string     `json:"ref"`
	Source      string     `json:"source"` // always "database" for this endpoint
	ReviewID    string     `json:"review_id"`
	TriggerType string     `json:"trigger_type"`
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	UserEmail   string     `json:"user_email,omitempty"`
	PrMrURL     string     `json:"pr_mr_url,omitempty"`
}

// ReviewCoverage handles POST /api/v1/review-coverage: given a list of
// commit SHAs and/or literal ranges, return every backend-stored review
// (database source only — see git-lrc's `coverage` command for the merged
// git-log + database picture) that covers any of them.
func (s *Server) ReviewCoverage(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok || orgID == 0 {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "missing org context")
	}

	var req ReviewCoverageRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}

	refs := make([]string, 0, len(req.Commits))
	for _, ref := range req.Commits {
		ref = strings.TrimSpace(ref)
		if ref != "" {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "commits must contain at least one ref")
	}
	if len(refs) > maxReviewCoverageCommits {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("commits must contain at most %d refs (got %d); batch larger lookups", maxReviewCoverageCommits, len(refs)))
	}

	rows, err := s.db.QueryContext(c.Request().Context(), `
		SELECT rc.ref, r.id, r.trigger_type, r.status, r.completed_at, r.user_email, r.pr_mr_url
		FROM review_commits rc
		JOIN reviews r ON r.id = rc.review_id
		WHERE rc.org_id = $1 AND rc.ref = ANY($2)
		ORDER BY r.completed_at DESC NULLS LAST, r.id DESC`,
		orgID, pq.Array(refs))
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to query review coverage")
	}
	defer rows.Close()

	reports := make([]ReviewCoverageReport, 0)
	for rows.Next() {
		var (
			ref, triggerType, status string
			reviewID                 int64
			completedAt              sql.NullTime
			userEmail, prMrURL       sql.NullString
		)
		if err := rows.Scan(&ref, &reviewID, &triggerType, &status, &completedAt, &userEmail, &prMrURL); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to scan review coverage row")
		}
		report := ReviewCoverageReport{
			Ref:         ref,
			Source:      "database",
			ReviewID:    formatReviewID(reviewID),
			TriggerType: triggerType,
			Status:      status,
			UserEmail:   userEmail.String,
			PrMrURL:     prMrURL.String,
		}
		if completedAt.Valid {
			t := completedAt.Time
			report.CompletedAt = &t
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "error iterating review coverage rows")
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"commits": refs,
		"reports": reports,
	})
}

func formatReviewID(id int64) string {
	return strconv.FormatInt(id, 10)
}
