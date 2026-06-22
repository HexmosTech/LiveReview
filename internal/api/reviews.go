package api

import (
	"database/sql"

	reviewprocessor "github.com/livereview/internal/review_processor"
)

// Review represents a code review record, aliased from reviewprocessor
type Review = reviewprocessor.Review

// AIComment represents an AI-generated comment, aliased from reviewprocessor
type AIComment = reviewprocessor.AIComment

// ReviewManager handles review operations, aliased from reviewprocessor
type ReviewManager = reviewprocessor.ReviewManager

// ReviewMetadataUpdate describes optional fields that can be updated, aliased from reviewprocessor
type ReviewMetadataUpdate = reviewprocessor.ReviewMetadataUpdate

// NewReviewManager creates a new review manager using the reviewprocessor implementation
func NewReviewManager(db *sql.DB) *ReviewManager {
	return reviewprocessor.NewReviewManager(db)
}

// TrackAICommentFromURL tracks AI comments based on MR/PR URL
func TrackAICommentFromURL(db *sql.DB, prMrURL, commentType string, content map[string]interface{}, filePath *string, lineNumber *int, orgID int64) error {
	return reviewprocessor.TrackAICommentFromURL(db, prMrURL, commentType, content, filePath, lineNumber, orgID)
}
