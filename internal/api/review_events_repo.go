package api

import (
	"database/sql"

	reviewprocessor "github.com/livereview/internal/review_processor"
)

// ReviewEvent represents a structured event in the review pipeline, aliased from reviewprocessor
type ReviewEvent = reviewprocessor.ReviewEvent

// EventData represents the common structure for different event types, aliased from reviewprocessor
type EventData = reviewprocessor.EventData

// ReviewEventsRepo handles database operations for review events, aliased from reviewprocessor
type ReviewEventsRepo = reviewprocessor.ReviewEventsRepo

// ListEventsCursor represents pagination cursor for events, aliased from reviewprocessor
type ListEventsCursor = reviewprocessor.ListEventsCursor

// SeverityCounts represents counts of events by severity level, aliased from reviewprocessor
type SeverityCounts = reviewprocessor.SeverityCounts

// ToolBreakdownItem represents status and findings for an individual tool, aliased from reviewprocessor
type ToolBreakdownItem = reviewprocessor.ToolBreakdownItem

// ToolSummary represents summary metrics and breakdown for tool execution, aliased from reviewprocessor
type ToolSummary = reviewprocessor.ToolSummary

// NewReviewEventsRepo creates a new review events repository using the reviewprocessor implementation
func NewReviewEventsRepo(db *sql.DB) *ReviewEventsRepo {
	return reviewprocessor.NewReviewEventsRepo(db)
}
