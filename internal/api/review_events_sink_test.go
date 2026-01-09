package api

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatabaseEventSink(t *testing.T) {
	// Skip if running in CI without database
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	// Connect to test database
	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	defer db.Close()

	sink := NewDatabaseEventSink(db)
	ctx := context.Background()
	orgID := int64(1)

	// Create a test review record first
	var reviewID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, "test/sink-repo", "main", "ghi789", "https://test.com/pr/3", "created", "manual", "test@example.com", "test", orgID).Scan(&reviewID)
	require.NoError(t, err, "Failed to create test review")

	// Clean up any existing test data
	_, _ = db.ExecContext(ctx, "DELETE FROM public.review_events WHERE review_id = $1", reviewID)

	t.Run("EmitStatusEvent", func(t *testing.T) {
		err := sink.EmitStatusEvent(ctx, reviewID, orgID, "running")
		require.NoError(t, err)

		// Verify event was stored
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.review_events WHERE review_id = $1 AND event_type = 'status'", reviewID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("EmitLogEvent", func(t *testing.T) {
		err := sink.EmitLogEvent(ctx, reviewID, orgID, "info", "Test log message from sink", "test-batch-1")
		require.NoError(t, err)

		// Verify event was stored
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.review_events WHERE review_id = $1 AND event_type = 'log'", reviewID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("EmitBatchEvent", func(t *testing.T) {
		err := sink.EmitBatchEvent(ctx, reviewID, orgID, "test-batch-1", "processing", 600, 4, nil)
		require.NoError(t, err)

		// Verify event was stored
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.review_events WHERE review_id = $1 AND event_type = 'batch'", reviewID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("EmitArtifactEvent", func(t *testing.T) {
		err := sink.EmitArtifactEvent(ctx, reviewID, orgID, "prompt", "/test/prompt.txt", "test-batch-1", 1024, "Sample prompt...", "...end of prompt")
		require.NoError(t, err)

		// Verify event was stored
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.review_events WHERE review_id = $1 AND event_type = 'artifact'", reviewID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("EmitCompletionEvent", func(t *testing.T) {
		err := sink.EmitCompletionEvent(ctx, reviewID, orgID, "Test review completed successfully", 7, "")
		require.NoError(t, err)

		// Verify event was stored
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.review_events WHERE review_id = $1 AND event_type = 'completion'", reviewID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("VerifyAllEvents", func(t *testing.T) {
		// Verify we have all expected event types
		var totalCount int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM public.review_events WHERE review_id = $1", reviewID).Scan(&totalCount)
		require.NoError(t, err)
		assert.Equal(t, 5, totalCount) // status + log + batch + artifact + completion
	})

	// Clean up test data
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM public.review_events WHERE review_id = $1", reviewID)
		_, _ = db.ExecContext(ctx, "DELETE FROM public.reviews WHERE id = $1", reviewID)
	})
}
