package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	feedbackstorage "github.com/livereview/storage/feedback"
)

type FeedbackHandler struct {
	db    *sql.DB
	store *feedbackstorage.FeedbackStore
}

func NewFeedbackHandler(db *sql.DB) *FeedbackHandler {
	return &FeedbackHandler{db: db, store: feedbackstorage.NewFeedbackStore(db)}
}

type SubmitFeedbackRequest struct {
	ReviewID       *int64   `json:"review_id"`
	AICommentID    *int64   `json:"ai_comment_id"`
	VoteType       string   `json:"vote_type"`
	Tags           []string `json:"tags"`
	FeedbackText   *string  `json:"feedback_text"`
	CommentContent *string  `json:"comment_content"`
	CodeExcerpt    *string  `json:"code_excerpt"`
	FilePath       *string  `json:"file_path"`
	Severity       *string  `json:"severity"`
	SourceType     string   `json:"source_type"`
}

func (h *FeedbackHandler) SubmitFeedback(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		orgID = 1
	}
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	var req SubmitFeedbackRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.VoteType != "up" && req.VoteType != "down" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "vote_type must be 'up' or 'down'"})
	}
	if req.SourceType == "" {
		req.SourceType = "comment"
	}
	if req.SourceType != "comment" && req.SourceType != "pr_level" && req.SourceType != "slideshow" && req.SourceType != "general" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "source_type must be 'comment', 'pr_level', 'slideshow', or 'general'"})
	}

	if req.ReviewID != nil {
		var owns int
		err := h.db.QueryRowContext(c.Request().Context(), `
			SELECT 1
			FROM reviews r
			JOIN users u ON u.email = r.user_email
			WHERE r.id = $1
			  AND r.org_id = $2
			  AND u.id = $3
		`, *req.ReviewID, orgID, userID).Scan(&owns)
		if err != nil {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "review not found or access denied"})
		}
	}

	lrcVersion := c.Request().Header.Get("X-LRC-Version")
	var lrcVersionPtr *string
	if lrcVersion != "" {
		lrcVersionPtr = &lrcVersion
	}

	id, createdAt, err := h.store.InsertFeedback(c.Request().Context(), feedbackstorage.InsertFeedbackInput{
		OrgID:          orgID,
		ReviewID:       req.ReviewID,
		AICommentID:    req.AICommentID,
		VoteType:       req.VoteType,
		Tags:           req.Tags,
		FeedbackText:   req.FeedbackText,
		CommentContent: req.CommentContent,
		CodeExcerpt:    req.CodeExcerpt,
		FilePath:       req.FilePath,
		Severity:       req.Severity,
		SourceType:     req.SourceType,
		LRCVersion:     lrcVersionPtr,
	})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save feedback"})
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id":         id,
		"created_at": createdAt,
	})
}

type ImpactStatsResponse struct {
	TotalReviews int64 `json:"total_reviews"`
	IssuesFound  int64 `json:"issues_found"`
	BugsCaught   int64 `json:"bugs_caught"`
	Critical     int64 `json:"critical"`
	Errors       int64 `json:"errors"`
	Warnings     int64 `json:"warnings"`
	Info         int64 `json:"info"`
}

func (h *FeedbackHandler) RetractFeedback(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid id"})
	}

	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		orgID = 1
	}
	userID, ok := c.Get("user_id").(int64)
	if !ok || userID == 0 {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}

	// Only allow retraction if the feedback's review belongs to the requesting user.
	// Floating feedback (review_id IS NULL) cannot be user-attributed so retraction is denied.
	result, err := h.db.ExecContext(c.Request().Context(), `
		UPDATE review_feedback rf
		SET retracted_at = NOW()
		FROM (
			SELECT rf2.id
			FROM review_feedback rf2
			JOIN reviews r ON r.id = rf2.review_id
			JOIN users u ON u.email = r.user_email
			WHERE rf2.id = $1
			  AND rf2.retracted_at IS NULL
			  AND r.org_id = $2
			  AND u.id = $3
		) owned
		WHERE rf.id = owned.id
	`, id, orgID, userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to retract"})
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "feedback not found or access denied"})
	}
	return c.NoContent(http.StatusNoContent)
}

// ImpactStats returns org-scoped review quality stats for the authenticated user's org.
func (h *FeedbackHandler) ImpactStats(c echo.Context) error {
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		orgID = 1
	}

	var stats ImpactStatsResponse
	err := h.db.QueryRowContext(c.Request().Context(), `
		SELECT
			COUNT(DISTINCT r.id),
			COUNT(c),
			COUNT(c) FILTER (WHERE c->>'Severity' ILIKE 'critical' OR c->>'Severity' ILIKE 'error'),
			COUNT(c) FILTER (WHERE c->>'Severity' ILIKE 'critical'),
			COUNT(c) FILTER (WHERE c->>'Severity' ILIKE 'error'),
			COUNT(c) FILTER (WHERE c->>'Severity' ILIKE 'warning'),
			COUNT(c) FILTER (WHERE c->>'Severity' ILIKE 'info')
		FROM reviews r,
		     jsonb_array_elements(r.metadata->'review_result'->'comments') c
		WHERE r.org_id = $1
		  AND r.status = 'completed'
		  AND jsonb_typeof(r.metadata->'review_result'->'comments') = 'array'
	`, orgID).Scan(
		&stats.TotalReviews,
		&stats.IssuesFound,
		&stats.BugsCaught,
		&stats.Critical,
		&stats.Errors,
		&stats.Warnings,
		&stats.Info,
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to fetch stats"})
	}

	return c.JSON(http.StatusOK, stats)
}
