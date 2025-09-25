package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventSink defines the interface for broadcasting events
type EventSink interface {
	EmitEvent(ctx context.Context, event *ReviewEvent) error
}

// PollingEventService provides event storage and retrieval for polling-based updates
type PollingEventService struct {
	repo *ReviewEventsRepo
}

// NewPollingEventService creates a new polling-based event service
func NewPollingEventService(db *sql.DB) *PollingEventService {
	return &PollingEventService{
		repo: NewReviewEventsRepo(db),
	}
}

// EmitEvent stores an event for later retrieval via polling
func (s *PollingEventService) EmitEvent(ctx context.Context, event *ReviewEvent) error {
	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	return s.repo.InsertEvent(ctx, event)
}

// GetRecentEvents retrieves recent events for a review (used by polling endpoints)
func (s *PollingEventService) GetRecentEvents(ctx context.Context, reviewID, orgID int64, since *time.Time, limit int) ([]*ReviewEvent, error) {
	cursor := &ListEventsCursor{
		Since: since,
		Limit: limit,
	}
	return s.repo.ListEvents(ctx, reviewID, orgID, cursor)
}

// GetEventsByType retrieves events of a specific type
func (s *PollingEventService) GetEventsByType(ctx context.Context, reviewID, orgID int64, eventType string, limit int) ([]*ReviewEvent, error) {
	return s.repo.GetEventsByType(ctx, reviewID, orgID, eventType, limit)
}

// GetLatestStatus gets the most recent status for a review
func (s *PollingEventService) GetLatestStatus(ctx context.Context, reviewID, orgID int64) (*ReviewEvent, error) {
	return s.repo.GetLatestStatusEvent(ctx, reviewID, orgID)
}

// GetEventCounts returns event counts by type for a review
func (s *PollingEventService) GetEventCounts(ctx context.Context, reviewID, orgID int64) (map[string]int, error) {
	return s.repo.CountEventsByReview(ctx, reviewID, orgID)
}

// CreateStatusEvent creates a status change event
func (s *PollingEventService) CreateStatusEvent(ctx context.Context, reviewID, orgID int64, status string, startedAt, finishedAt *time.Time) error {
	data := EventData{
		Status: &status,
	}

	if startedAt != nil {
		startedAtStr := startedAt.Format(time.RFC3339)
		data.StartedAt = &startedAtStr
	}

	if finishedAt != nil {
		finishedAtStr := finishedAt.Format(time.RFC3339)
		data.FinishedAt = &finishedAtStr

		if startedAt != nil {
			durationMs := finishedAt.Sub(*startedAt).Milliseconds()
			data.DurationMs = &durationMs
		}
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal status event data: %w", err)
	}

	event := &ReviewEvent{
		ReviewID:  reviewID,
		OrgID:     orgID,
		EventType: "status",
		Level:     stringPtr("info"),
		Data:      dataJSON,
	}

	return s.EmitEvent(ctx, event)
}

// CreateLogEvent creates a log message event
func (s *PollingEventService) CreateLogEvent(ctx context.Context, reviewID, orgID int64, level, message string, batchID *string) error {
	data := EventData{
		Message: &message,
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal log event data: %w", err)
	}

	event := &ReviewEvent{
		ReviewID:  reviewID,
		OrgID:     orgID,
		EventType: "log",
		Level:     &level,
		BatchID:   batchID,
		Data:      dataJSON,
	}

	return s.EmitEvent(ctx, event)
}

// CreateBatchEvent creates a batch progress event
func (s *PollingEventService) CreateBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount *int, startedAt, finishedAt *time.Time) error {
	data := EventData{
		Status:        &status,
		TokenEstimate: tokenEstimate,
		FileCount:     fileCount,
	}

	if startedAt != nil {
		startedAtStr := startedAt.Format(time.RFC3339)
		data.StartedAt = &startedAtStr
	}

	if finishedAt != nil {
		finishedAtStr := finishedAt.Format(time.RFC3339)
		data.FinishedAt = &finishedAtStr
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal batch event data: %w", err)
	}

	event := &ReviewEvent{
		ReviewID:  reviewID,
		OrgID:     orgID,
		EventType: "batch",
		Level:     stringPtr("info"),
		BatchID:   &batchID,
		Data:      dataJSON,
	}

	return s.EmitEvent(ctx, event)
}

// CreateArtifactEvent creates an artifact reference event
func (s *PollingEventService) CreateArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url string, batchID *string, sizeBytes *int64, previewHead, previewTail *string) error {
	data := EventData{
		Kind:        &kind,
		URL:         &url,
		SizeBytes:   sizeBytes,
		PreviewHead: previewHead,
		PreviewTail: previewTail,
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal artifact event data: %w", err)
	}

	event := &ReviewEvent{
		ReviewID:  reviewID,
		OrgID:     orgID,
		EventType: "artifact",
		Level:     stringPtr("info"),
		BatchID:   batchID,
		Data:      dataJSON,
	}

	return s.EmitEvent(ctx, event)
}

// CreateCompletionEvent creates a review completion event
func (s *PollingEventService) CreateCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount *int, errorSummary *string) error {
	data := EventData{
		ResultSummary: &resultSummary,
		CommentCount:  commentCount,
		ErrorSummary:  errorSummary,
	}

	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal completion event data: %w", err)
	}

	event := &ReviewEvent{
		ReviewID:  reviewID,
		OrgID:     orgID,
		EventType: "completion",
		Level:     stringPtr("info"),
		Data:      dataJSON,
	}

	return s.EmitEvent(ctx, event)
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

// GetReviewSummary creates a summary of recent review activity for display
func (s *PollingEventService) GetReviewSummary(ctx context.Context, reviewID, orgID int64) (*ReviewSummary, error) {
	// Get latest status
	latestStatus, err := s.GetLatestStatus(ctx, reviewID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest status: %w", err)
	}

	// Get event counts
	counts, err := s.GetEventCounts(ctx, reviewID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event counts: %w", err)
	}

	batchCount, err := s.repo.CountDistinctBatchIDs(ctx, reviewID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to count batch IDs: %w", err)
	}
	summary := &ReviewSummary{
		ReviewID:     reviewID,
		LastActivity: time.Now(), // Will be updated with actual latest event
		EventCounts:  counts,
		BatchCount:   batchCount,
	}

	// Parse latest status if available
	if latestStatus != nil {
		summary.LastActivity = latestStatus.Timestamp
		var statusData EventData
		if err := json.Unmarshal(latestStatus.Data, &statusData); err == nil && statusData.Status != nil {
			summary.CurrentStatus = *statusData.Status
		}
	}

	return summary, nil
}

// ReviewSummary provides a quick overview of review progress
type ReviewSummary struct {
	ReviewID      int64          `json:"reviewId"`
	CurrentStatus string         `json:"currentStatus"`
	LastActivity  time.Time      `json:"lastActivity"`
	EventCounts   map[string]int `json:"eventCounts"`
	BatchCount    int            `json:"batchCount"`
}
