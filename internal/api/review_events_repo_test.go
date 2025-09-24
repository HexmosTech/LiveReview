package api

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"database/sql"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewEventsRepo(t *testing.T) {
	// Skip if running in CI without database
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	// Connect to test database
	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	defer db.Close()

	repo := NewReviewEventsRepo(db)
	ctx := context.Background()

	// Test data
	orgID := int64(1)
	now := time.Now()

	// Create a test review record first
	var reviewID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, "test/repo", "main", "abc123", "https://test.com/pr/1", "created", "manual", "test@example.com", "test", orgID).Scan(&reviewID)
	require.NoError(t, err, "Failed to create test review")

	// Clean up any existing test data
	_, _ = db.ExecContext(ctx, "DELETE FROM public.review_events WHERE review_id = $1", reviewID)

	t.Run("InsertEvent", func(t *testing.T) {
		// Create test event data
		eventData := EventData{
			Status:    stringPtr("running"),
			StartedAt: stringPtr(now.Format(time.RFC3339)),
		}
		dataJSON, err := json.Marshal(eventData)
		require.NoError(t, err)

		event := &ReviewEvent{
			ReviewID:  reviewID,
			OrgID:     orgID,
			Timestamp: now,
			EventType: "status",
			Level:     stringPtr("info"),
			BatchID:   stringPtr("batch-1"),
			Data:      dataJSON,
		}

		// Insert the event
		err = repo.InsertEvent(ctx, event)
		require.NoError(t, err)
		assert.NotZero(t, event.ID, "Event ID should be set after insert")
	})

	t.Run("ListEvents", func(t *testing.T) {
		// Insert another event
		eventData := EventData{
			Message: stringPtr("Test log message"),
		}
		dataJSON, err := json.Marshal(eventData)
		require.NoError(t, err)

		event2 := &ReviewEvent{
			ReviewID:  reviewID,
			OrgID:     orgID,
			Timestamp: now.Add(time.Minute),
			EventType: "log",
			Level:     stringPtr("info"),
			Data:      dataJSON,
		}

		err = repo.InsertEvent(ctx, event2)
		require.NoError(t, err)

		// List all events
		cursor := &ListEventsCursor{Limit: 10}
		events, err := repo.ListEvents(ctx, reviewID, orgID, cursor)
		require.NoError(t, err)
		assert.Len(t, events, 2, "Should retrieve both events")
		assert.Equal(t, "status", events[0].EventType)
		assert.Equal(t, "log", events[1].EventType)
	})

	t.Run("ListEventsWithCursor", func(t *testing.T) {
		// List events after the first timestamp
		since := now.Add(30 * time.Second)
		cursor := &ListEventsCursor{
			Since: &since,
			Limit: 10,
		}
		events, err := repo.ListEvents(ctx, reviewID, orgID, cursor)
		require.NoError(t, err)
		assert.Len(t, events, 1, "Should retrieve only the second event")
		assert.Equal(t, "log", events[0].EventType)
	})

	t.Run("GetEventsByType", func(t *testing.T) {
		events, err := repo.GetEventsByType(ctx, reviewID, orgID, "status", 10)
		require.NoError(t, err)
		assert.Len(t, events, 1, "Should retrieve only status events")
		assert.Equal(t, "status", events[0].EventType)
	})

	t.Run("GetLatestStatusEvent", func(t *testing.T) {
		event, err := repo.GetLatestStatusEvent(ctx, reviewID, orgID)
		require.NoError(t, err)
		require.NotNil(t, event)
		assert.Equal(t, "status", event.EventType)
	})

	t.Run("CountEventsByReview", func(t *testing.T) {
		counts, err := repo.CountEventsByReview(ctx, reviewID, orgID)
		require.NoError(t, err)
		assert.Equal(t, 1, counts["status"])
		assert.Equal(t, 1, counts["log"])
	})

	// Clean up test data
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM public.review_events WHERE review_id = $1", reviewID)
		_, _ = db.ExecContext(ctx, "DELETE FROM public.reviews WHERE id = $1", reviewID)
	})
}
