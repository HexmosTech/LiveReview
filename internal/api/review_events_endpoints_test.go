package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewEventsEndpoints(t *testing.T) {
	// Per Phase 0: this integration test relies on a local DB and currently fails in CI.
	// The user requested removing failing tests; skip this entire test so the suite only
	// contains passing tests.
	t.Skip("Skipping DB-dependent integration test per Phase 0 (removed failing cases)")

	// Connect to test database
	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	defer db.Close()

	// Create handler
	handler := NewReviewEventsHandler(db)

	// Setup Echo
	e := echo.New()

	// Create a test review record first
	orgID := int64(1)
	var reviewID int64
	err = db.QueryRow(`
		INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, "test/endpoint-repo", "main", "jkl012", "https://test.com/pr/4", "created", "manual", "test@example.com", "test", orgID).Scan(&reviewID)
	require.NoError(t, err, "Failed to create test review")

	// Clean up any existing test data
	_, _ = db.Exec("DELETE FROM public.review_events WHERE review_id = $1", reviewID)

	// Create some test events
	sink := NewDatabaseEventSink(db)
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()

	_ = sink.EmitStatusEvent(ctx, reviewID, orgID, "running")
	_ = sink.EmitLogEvent(ctx, reviewID, orgID, "info", "Test log message for endpoints", "test-batch-1")
	_ = sink.EmitBatchEvent(ctx, reviewID, orgID, "test-batch-1", "processing", 700, 5, nil)

	t.Run("GetReviewEvents", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews/123/events", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("123") // Use string for path param

		// Override the reviewID extraction for test
		req = httptest.NewRequest(http.MethodGet, "/api/v1/reviews/"+string(rune(reviewID+48))+"/events", nil)
		c = e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(string(rune(reviewID + 48))) // Convert int64 to string roughly

		// Actually, let's just use a proper string conversion
		req = httptest.NewRequest(http.MethodGet, "/api/v1/reviews/1/events", nil)
		rec = httptest.NewRecorder()
		c = e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("1") // Use a simple ID for testing

		// For this test, we'll modify the handler to use reviewID = 1
		// But first let's update our test data to use reviewID = 1
		// Clean up and create with reviewID = 1
		_, _ = db.Exec("DELETE FROM public.review_events WHERE review_id = $1", 1)
		_ = sink.EmitStatusEvent(ctx, 1, orgID, "running")

		err := handler.GetReviewEvents(c)
		if assert.NoError(t, err) {
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "events")
			assert.Contains(t, rec.Body.String(), "meta")
		}
	})

	t.Run("GetReviewSummary", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews/1/summary", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("1")

		err := handler.GetReviewSummary(c)
		if assert.NoError(t, err) {
			assert.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.Contains(t, body, "reviewId")
			assert.Contains(t, body, "eventCounts")
		}
	})

	t.Run("GetReviewEventsByType", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews/1/events/status", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id", "type")
		c.SetParamValues("1", "status")

		err := handler.GetReviewEventsByType(c)
		if assert.NoError(t, err) {
			assert.Equal(t, http.StatusOK, rec.Code)
			body := rec.Body.String()
			assert.Contains(t, body, "events")
			assert.Contains(t, body, "eventType")
			assert.Contains(t, body, "status")
		}
	})

	t.Run("InvalidReviewID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/reviews/invalid/events", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues("invalid")

		err := handler.GetReviewEvents(c)

		// Should return an HTTP error
		if he, ok := err.(*echo.HTTPError); ok {
			assert.Equal(t, http.StatusBadRequest, he.Code)
			assert.Contains(t, he.Message, "Invalid review ID")
		} else {
			t.Errorf("Expected HTTP error, got %v", err)
		}
	})

	// Clean up test data
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM public.review_events WHERE review_id IN ($1, $2)", reviewID, 1)
		_, _ = db.Exec("DELETE FROM public.reviews WHERE id IN ($1, $2)", reviewID, 1)
	})
}
