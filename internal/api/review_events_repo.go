package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ReviewEvent represents a structured event in the review pipeline
type ReviewEvent struct {
	ID        int64           `json:"id" db:"id"`
	ReviewID  int64           `json:"reviewId" db:"review_id"`
	OrgID     int64           `json:"orgId" db:"org_id"`
	Timestamp time.Time       `json:"time" db:"ts"`
	EventType string          `json:"type" db:"event_type"`
	Level     *string         `json:"level,omitempty" db:"level"`
	BatchID   *string         `json:"batchId,omitempty" db:"batch_id"`
	Data      json.RawMessage `json:"data" db:"data"`
}

// EventData represents the common structure for different event types
type EventData struct {
	// For "status" events
	Status     *string `json:"status,omitempty"`
	StartedAt  *string `json:"startedAt,omitempty"`
	FinishedAt *string `json:"finishedAt,omitempty"`
	DurationMs *int64  `json:"durationMs,omitempty"`

	// For "log" events
	Message *string `json:"message,omitempty"`

	// For "batch" events
	TokenEstimate *int        `json:"tokenEstimate,omitempty"`
	FileCount     *int        `json:"fileCount,omitempty"` // Number of files in the batch
	Comments      interface{} `json:"comments,omitempty"`  // Actual comment objects when batch completes

	// For "artifact" events
	Kind        *string `json:"kind,omitempty"`
	SizeBytes   *int64  `json:"sizeBytes,omitempty"`
	PreviewHead *string `json:"previewHead,omitempty"`
	PreviewTail *string `json:"previewTail,omitempty"`
	URL         *string `json:"url,omitempty"`

	// For "completion" events (also used by "batch" events with status="completed")
	ResultSummary *string `json:"resultSummary,omitempty"`
	CommentCount  *int    `json:"commentCount,omitempty"` // Number of comments generated
	ErrorSummary  *string `json:"errorSummary,omitempty"`

	// For "retry" events
	Attempt     *int    `json:"attempt,omitempty"`
	Reason      *string `json:"reason,omitempty"`
	Delay       *string `json:"delay,omitempty"`
	NextAttempt *string `json:"nextAttempt,omitempty"`

	// For "json_repair" events
	OriginalSize     *int      `json:"originalSize,omitempty"`
	RepairedSize     *int      `json:"repairedSize,omitempty"`
	CommentsLost     *int      `json:"commentsLost,omitempty"`
	FieldsRecovered  *int      `json:"fieldsRecovered,omitempty"`
	RepairTime       *string   `json:"repairTime,omitempty"`
	RepairStrategies *[]string `json:"repairStrategies,omitempty"`

	// For "timeout" events
	Operation         *string `json:"operation,omitempty"`
	ConfiguredTimeout *string `json:"configuredTimeout,omitempty"`
	ActualDuration    *string `json:"actualDuration,omitempty"`

	// For "batch_stats" events
	TotalRequests   *int    `json:"totalRequests,omitempty"`
	Successful      *int    `json:"successful,omitempty"`
	Retries         *int    `json:"retries,omitempty"`
	JsonRepairs     *int    `json:"jsonRepairs,omitempty"`
	AvgResponseTime *string `json:"avgResponseTime,omitempty"`
}

// ReviewEventsRepo handles database operations for review events
type ReviewEventsRepo struct {
	db *sql.DB
}

// NewReviewEventsRepo creates a new review events repository
func NewReviewEventsRepo(db *sql.DB) *ReviewEventsRepo {
	return &ReviewEventsRepo{db: db}
}

// InsertEvent inserts a new review event into the database
func (r *ReviewEventsRepo) InsertEvent(ctx context.Context, event *ReviewEvent) error {
	query := `
		INSERT INTO public.review_events (review_id, org_id, ts, event_type, level, batch_id, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id
	`

	err := r.db.QueryRowContext(
		ctx, query,
		event.ReviewID,
		event.OrgID,
		event.Timestamp,
		event.EventType,
		event.Level,
		event.BatchID,
		event.Data,
	).Scan(&event.ID)

	if err != nil {
		return fmt.Errorf("failed to insert review event: %w", err)
	}

	return nil
}

// ListEventsCursor represents pagination cursor for events
type ListEventsCursor struct {
	Since *time.Time `json:"since,omitempty"`
	Limit int        `json:"limit"`
}

// ListEvents retrieves events for a review with optional cursor-based pagination
func (r *ReviewEventsRepo) ListEvents(ctx context.Context, reviewID, orgID int64, cursor *ListEventsCursor) ([]*ReviewEvent, error) {
	var query string
	var args []interface{}

	baseQuery := `
		SELECT id, review_id, org_id, ts, event_type, level, batch_id, data
		FROM public.review_events
		WHERE review_id = $1 AND org_id = $2
	`

	args = append(args, reviewID, orgID)
	argCount := 2

	// Add time filter if cursor provided
	if cursor != nil && cursor.Since != nil {
		argCount++
		baseQuery += fmt.Sprintf(" AND ts > $%d", argCount)
		args = append(args, *cursor.Since)
	}

	// Order by timestamp
	baseQuery += " ORDER BY ts ASC"

	// Add limit
	limit := 100 // default
	if cursor != nil && cursor.Limit > 0 {
		limit = cursor.Limit
	}
	if limit > 1000 {
		limit = 1000 // max limit
	}

	argCount++
	query = baseQuery + fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query review events: %w", err)
	}
	defer rows.Close()

	// Initialize as empty slice so JSON encodes to [] rather than null
	events := make([]*ReviewEvent, 0)
	for rows.Next() {
		event := &ReviewEvent{}
		err := rows.Scan(
			&event.ID,
			&event.ReviewID,
			&event.OrgID,
			&event.Timestamp,
			&event.EventType,
			&event.Level,
			&event.BatchID,
			&event.Data,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan review event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating review events: %w", err)
	}

	return events, nil
}

// GetEventsByType retrieves events of a specific type for a review
func (r *ReviewEventsRepo) GetEventsByType(ctx context.Context, reviewID, orgID int64, eventType string, limit int) ([]*ReviewEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100 // default/max limit
	}

	query := `
		SELECT id, review_id, org_id, ts, event_type, level, batch_id, data
		FROM public.review_events
		WHERE review_id = $1 AND org_id = $2 AND event_type = $3
		ORDER BY ts DESC
		LIMIT $4
	`

	rows, err := r.db.QueryContext(ctx, query, reviewID, orgID, eventType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query review events by type: %w", err)
	}
	defer rows.Close()

	// Initialize as empty slice so JSON encodes to [] rather than null
	events := make([]*ReviewEvent, 0)
	for rows.Next() {
		event := &ReviewEvent{}
		err := rows.Scan(
			&event.ID,
			&event.ReviewID,
			&event.OrgID,
			&event.Timestamp,
			&event.EventType,
			&event.Level,
			&event.BatchID,
			&event.Data,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan review event: %w", err)
		}
		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating review events: %w", err)
	}

	return events, nil
}

// GetLatestStatusEvent gets the most recent status event for a review
func (r *ReviewEventsRepo) GetLatestStatusEvent(ctx context.Context, reviewID, orgID int64) (*ReviewEvent, error) {
	query := `
		SELECT id, review_id, org_id, ts, event_type, level, batch_id, data
		FROM public.review_events
		WHERE review_id = $1 AND org_id = $2 AND event_type = 'status'
		ORDER BY ts DESC
		LIMIT 1
	`

	event := &ReviewEvent{}
	err := r.db.QueryRowContext(ctx, query, reviewID, orgID).Scan(
		&event.ID,
		&event.ReviewID,
		&event.OrgID,
		&event.Timestamp,
		&event.EventType,
		&event.Level,
		&event.BatchID,
		&event.Data,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No status event found
		}
		return nil, fmt.Errorf("failed to get latest status event: %w", err)
	}

	return event, nil
}

// DeleteEventsForReview deletes all events for a review (used when review is deleted due to CASCADE)
func (r *ReviewEventsRepo) DeleteEventsForReview(ctx context.Context, reviewID, orgID int64) error {
	query := `DELETE FROM public.review_events WHERE review_id = $1 AND org_id = $2`

	_, err := r.db.ExecContext(ctx, query, reviewID, orgID)
	if err != nil {
		return fmt.Errorf("failed to delete events for review: %w", err)
	}

	return nil
}

// CountEventsByReview returns the count of events for a review by type
func (r *ReviewEventsRepo) CountEventsByReview(ctx context.Context, reviewID, orgID int64) (map[string]int, error) {
	query := `
		SELECT event_type, COUNT(*) as count
		FROM public.review_events
		WHERE review_id = $1 AND org_id = $2
		GROUP BY event_type
	`

	rows, err := r.db.QueryContext(ctx, query, reviewID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to count events by review: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan event count: %w", err)
		}
		counts[eventType] = count
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating event counts: %w", err)
	}

	return counts, nil
}

// CountDistinctBatchIDs returns the number of unique batch IDs for a review
func (r *ReviewEventsRepo) CountDistinctBatchIDs(ctx context.Context, reviewID, orgID int64) (int, error) {
	query := `
		SELECT COUNT(DISTINCT batch_id)
		FROM public.review_events
		WHERE review_id = $1 AND org_id = $2 AND batch_id IS NOT NULL AND batch_id <> ''
	`

	var count int
	err := r.db.QueryRowContext(ctx, query, reviewID, orgID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count distinct batch IDs: %w", err)
	}

	return count, nil
}

// Helper functions for creating resiliency-specific events

// CreateRetryEvent creates a retry event for a review
func (r *ReviewEventsRepo) CreateRetryEvent(ctx context.Context, reviewID, orgID int64, batchID *string, attempt int, reason, delay, nextAttempt string) error {
	data := EventData{
		Attempt:     &attempt,
		Reason:      &reason,
		Delay:       &delay,
		NextAttempt: &nextAttempt,
	}

	return r.createTypedEvent(ctx, reviewID, orgID, "retry", "warn", batchID, data)
}

// CreateJSONRepairEvent creates a JSON repair event for a review
func (r *ReviewEventsRepo) CreateJSONRepairEvent(ctx context.Context, reviewID, orgID int64, batchID *string,
	originalSize, repairedSize, commentsLost, fieldsRecovered int, repairTime string, strategies []string) error {

	data := EventData{
		OriginalSize:     &originalSize,
		RepairedSize:     &repairedSize,
		CommentsLost:     &commentsLost,
		FieldsRecovered:  &fieldsRecovered,
		RepairTime:       &repairTime,
		RepairStrategies: &strategies,
	}

	return r.createTypedEvent(ctx, reviewID, orgID, "json_repair", "info", batchID, data)
}

// CreateTimeoutEvent creates a timeout event for a review
func (r *ReviewEventsRepo) CreateTimeoutEvent(ctx context.Context, reviewID, orgID int64, batchID *string,
	operation, configuredTimeout, actualDuration string) error {

	data := EventData{
		Operation:         &operation,
		ConfiguredTimeout: &configuredTimeout,
		ActualDuration:    &actualDuration,
	}

	return r.createTypedEvent(ctx, reviewID, orgID, "timeout", "error", batchID, data)
}

// CreateBatchStatsEvent creates a batch statistics event for a review
func (r *ReviewEventsRepo) CreateBatchStatsEvent(ctx context.Context, reviewID, orgID int64, batchID string,
	totalRequests, successful, retries, jsonRepairs int, avgResponseTime string) error {

	data := EventData{
		TotalRequests:   &totalRequests,
		Successful:      &successful,
		Retries:         &retries,
		JsonRepairs:     &jsonRepairs,
		AvgResponseTime: &avgResponseTime,
	}

	return r.createTypedEvent(ctx, reviewID, orgID, "batch_stats", "info", &batchID, data)
}

// createTypedEvent is a helper function to create events with proper JSON marshaling
func (r *ReviewEventsRepo) createTypedEvent(ctx context.Context, reviewID, orgID int64, eventType, level string, batchID *string, data EventData) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	event := ReviewEvent{
		ReviewID:  reviewID,
		OrgID:     orgID,
		Timestamp: time.Now(),
		EventType: eventType,
		Level:     &level,
		BatchID:   batchID,
		Data:      dataJSON,
	}

	return r.InsertEvent(ctx, &event)
}
