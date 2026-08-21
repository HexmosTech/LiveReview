package api

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPollingEventService(t *testing.T) {
	// Skip if running in CI without database
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		dbURL = "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
	}

	// Connect to test database
	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Skip("Skipping database integration test: database not accessible")
	}

	service := NewPollingEventService(db)
	orgID := int64(1)

	// Create a test review record first
	var reviewID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, "test/polling-repo", "main", "def456", "https://test.com/pr/2", "created", "manual", "test@example.com", "test", orgID).Scan(&reviewID)
	require.NoError(t, err, "Failed to create test review")

	// Clean up any existing test data
	_, _ = db.ExecContext(ctx, "DELETE FROM public.review_events WHERE review_id = $1", reviewID)

	t.Run("CreateStatusEvent", func(t *testing.T) {
		startTime := time.Now()
		err := service.CreateStatusEvent(ctx, reviewID, orgID, "running", &startTime, nil)
		require.NoError(t, err)

		// Verify the event was created
		status, err := service.GetLatestStatus(ctx, reviewID, orgID)
		require.NoError(t, err)
		require.NotNil(t, status)
		assert.Equal(t, "status", status.EventType)
	})

	t.Run("CreateLogEvent", func(t *testing.T) {
		batchID := "test-batch-1"
		err := service.CreateLogEvent(ctx, reviewID, orgID, "info", "Test log message", &batchID)
		require.NoError(t, err)

		// Get recent events
		since := time.Now().Add(-1 * time.Minute)
		events, err := service.GetRecentEvents(ctx, reviewID, orgID, &since, 10)
		require.NoError(t, err)
		assert.Len(t, events, 2) // status + log events

		// Find the log event
		var logEvent *ReviewEvent
		for _, event := range events {
			if event.EventType == "log" {
				logEvent = event
				break
			}
		}
		require.NotNil(t, logEvent)
		assert.Equal(t, "info", *logEvent.Level)
		assert.Equal(t, "test-batch-1", *logEvent.BatchID)
	})

	t.Run("CreateBatchEvent", func(t *testing.T) {
		batchID := "test-batch-1"
		tokenEstimate := 500
		fileCount := 3
		startTime := time.Now()

		err := service.CreateBatchEvent(ctx, reviewID, orgID, batchID, "processing", &tokenEstimate, &fileCount, &startTime, nil, nil)
		require.NoError(t, err)

		// Get batch events
		batches, err := service.GetEventsByType(ctx, reviewID, orgID, "batch", 5)
		require.NoError(t, err)
		assert.Len(t, batches, 1)
		assert.Equal(t, "batch", batches[0].EventType)
		assert.Equal(t, "test-batch-1", *batches[0].BatchID)
	})

	t.Run("CreateCompletionEvent", func(t *testing.T) {
		commentCount := 5
		err := service.CreateCompletionEvent(ctx, reviewID, orgID, "Review completed successfully", &commentCount, nil)
		require.NoError(t, err)

		// Get completion events
		completions, err := service.GetEventsByType(ctx, reviewID, orgID, "completion", 5)
		require.NoError(t, err)
		assert.Len(t, completions, 1)
	})

	t.Run("GetReviewSummary", func(t *testing.T) {
		// Emit tool_dispatch and tool_result events for testing enriched summary
		_ = service.EmitEvent(ctx, &ReviewEvent{
			ReviewID:  reviewID,
			OrgID:     orgID,
			EventType: "tool_dispatch",
			Data:      []byte(`{"tool_id": 1, "tool_name": "ruff", "status": "pending"}`),
		})
		_ = service.EmitEvent(ctx, &ReviewEvent{
			ReviewID:  reviewID,
			OrgID:     orgID,
			EventType: "tool_result",
			Data:      []byte(`{"tool_id": 1, "tool_name": "ruff", "exit_code": 0, "findings": [{"file": "main.py", "line": 10, "message": "Line too long"}]}`),
		})

		summary, err := service.GetReviewSummary(ctx, reviewID, orgID)
		require.NoError(t, err)
		require.NotNil(t, summary)

		assert.Equal(t, reviewID, summary.ReviewID)
		assert.NotEmpty(t, summary.EventCounts)
		assert.Contains(t, summary.EventCounts, "status")
		assert.Contains(t, summary.EventCounts, "log")
		assert.Contains(t, summary.EventCounts, "batch")
		assert.Contains(t, summary.EventCounts, "completion")
		assert.Equal(t, 1, summary.BatchCount)
		assert.NotNil(t, summary.SeverityCounts)
		assert.NotNil(t, summary.ToolSummary)
		assert.Equal(t, 1, summary.ToolSummary.ToolsExecuted)
		assert.Equal(t, 1, summary.ToolSummary.TotalCommentsGenerated)
		assert.Len(t, summary.ToolSummary.ToolBreakdown, 1)
		assert.Equal(t, "ruff", summary.ToolSummary.ToolBreakdown[0].ToolName)
		assert.Equal(t, "completed", summary.ToolSummary.ToolBreakdown[0].Status)
	})

	t.Run("GetEventCounts", func(t *testing.T) {
		counts, err := service.GetEventCounts(ctx, reviewID, orgID)
		require.NoError(t, err)

		// We should have created: 1 status, 1 log, 1 batch, 1 completion, 1 tool_dispatch, 1 tool_result
		assert.Equal(t, 1, counts["status"])
		assert.Equal(t, 1, counts["log"])
		assert.Equal(t, 1, counts["batch"])
		assert.Equal(t, 1, counts["completion"])
		assert.Equal(t, 1, counts["tool_dispatch"])
		assert.Equal(t, 1, counts["tool_result"])
	})

	// Clean up test data
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM public.review_events WHERE review_id = $1", reviewID)
		_, _ = db.ExecContext(ctx, "DELETE FROM public.reviews WHERE id = $1", reviewID)
	})
}
