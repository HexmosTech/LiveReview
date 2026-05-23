package feedback

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"
)

type FeedbackStore struct {
	db *sql.DB
}

func NewFeedbackStore(db *sql.DB) *FeedbackStore {
	return &FeedbackStore{db: db}
}

type InsertFeedbackInput struct {
	OrgID          int64
	ReviewID       *int64
	AICommentID    *int64
	VoteType       string
	Tags           []string
	FeedbackText   *string
	CommentContent *string
	CodeExcerpt    *string
	FilePath       *string
	Severity       *string
	SourceType     string
	LRCVersion     *string
}

func (s *FeedbackStore) InsertFeedback(ctx context.Context, in InsertFeedbackInput) (int64, time.Time, error) {
	var id int64
	var createdAt time.Time

	tags := pq.Array(in.Tags)
	if in.Tags == nil {
		tags = pq.Array([]string{})
	}

	err := s.db.QueryRowContext(ctx, `
		INSERT INTO review_feedback (
			org_id, review_id, ai_comment_id,
			vote_type, tags, feedback_text,
			comment_content, code_excerpt, file_path,
			severity, source_type, lrc_version
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9,
			$10, $11, $12
		) RETURNING id, created_at
	`,
		in.OrgID, in.ReviewID, in.AICommentID,
		in.VoteType, tags, in.FeedbackText,
		in.CommentContent, in.CodeExcerpt, in.FilePath,
		in.Severity, in.SourceType, in.LRCVersion,
	).Scan(&id, &createdAt)

	return id, createdAt, err
}
