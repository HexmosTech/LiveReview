package api

// Handler-level tests for the instance-wide AI review performance ceiling
// (GET/PUT /admin/settings/ai-review-performance) and its interaction with
// an org's own soft preference (GET/PUT /aiconnectors/settings) - the Phase
// 2/3 settings infrastructure for configurable worker count/timeout
// requested alongside the concurrency and Events-tab truth fixes.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/livereview/internal/reviewperf"
	storageaiconnectors "github.com/livereview/storage/aiconnectors"
)

func perfSettingsTestDB(t *testing.T) *sql.DB {
	if testing.Short() {
		t.Skip("skipping database integration test")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	return db
}

func TestAIReviewPerformanceSettings_DefaultsWhenNoRowSaved(t *testing.T) {
	db := perfSettingsTestDB(t)
	// Clean slate - delete any row a prior run left behind.
	if _, err := db.Exec(`DELETE FROM system_settings WHERE name = 'ai_review_performance'`); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	s := &Server{db: db}
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/ai-review-performance", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.GetAIReviewPerformanceSettings(c); err != nil {
		t.Fatalf("GetAIReviewPerformanceSettings: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got reviewperf.Settings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	want := reviewperf.DefaultSettings()
	if got != want {
		t.Fatalf("got %+v, want defaults %+v", got, want)
	}
}

func TestAIReviewPerformanceSettings_UpdateThenGetRoundTrips(t *testing.T) {
	db := perfSettingsTestDB(t)

	s := &Server{db: db}
	e := echo.New()

	newSettings := reviewperf.Settings{
		MaxConcurrentBatchesCeiling: 24,
		MinReviewTimeoutSeconds:     90,
		MaxReviewTimeoutSeconds:     300,
	}
	body, _ := json.Marshal(newSettings)
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/ai-review-performance", bytes.NewReader(body))
	putReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	putRec := httptest.NewRecorder()
	putC := e.NewContext(putReq, putRec)

	if err := s.UpdateAIReviewPerformanceSettings(putC); err != nil {
		t.Fatalf("UpdateAIReviewPerformanceSettings: %v", err)
	}
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings/ai-review-performance", nil)
	getRec := httptest.NewRecorder()
	getC := e.NewContext(getReq, getRec)
	if err := s.GetAIReviewPerformanceSettings(getC); err != nil {
		t.Fatalf("GetAIReviewPerformanceSettings: %v", err)
	}

	var got reviewperf.Settings
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got != newSettings {
		t.Fatalf("got %+v after round-trip, want %+v", got, newSettings)
	}

	// Restore defaults so other tests (and a shared dev DB) aren't left
	// with this test's ceiling.
	if err := reviewperf.Save(putC.Request().Context(), db, reviewperf.DefaultSettings()); err != nil {
		t.Fatalf("restore defaults: %v", err)
	}
}

func TestAIReviewPerformanceSettings_RejectsInvalidPayload(t *testing.T) {
	db := perfSettingsTestDB(t)
	s := &Server{db: db}
	e := echo.New()

	invalid := reviewperf.Settings{MaxConcurrentBatchesCeiling: 0, MinReviewTimeoutSeconds: 60, MaxReviewTimeoutSeconds: 240}
	body, _ := json.Marshal(invalid)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/ai-review-performance", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := s.UpdateAIReviewPerformanceSettings(c); err != nil {
		t.Fatalf("UpdateAIReviewPerformanceSettings: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid ceiling, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestUpsertReviewAISettings_PreferenceClampedToSystemCeiling is the
// end-to-end proof that an org preference above the instance ceiling never
// takes effect: it saves fine (the org is free to ask), but the *effective*
// value returned is clamped - the whole point of the two-tier design in
// internal/reviewperf (an org can turn concurrency down, never up past what
// the instance allows).
func TestUpsertReviewAISettings_PreferenceClampedToSystemCeiling(t *testing.T) {
	db := perfSettingsTestDB(t)
	if err := reviewperf.Save(context.Background(), db, reviewperf.Settings{
		MaxConcurrentBatchesCeiling: 10,
		MinReviewTimeoutSeconds:     60,
		MaxReviewTimeoutSeconds:     240,
	}); err != nil {
		t.Fatalf("seed system ceiling: %v", err)
	}
	t.Cleanup(func() {
		_ = reviewperf.Save(context.Background(), db, reviewperf.DefaultSettings())
	})

	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('ai-perf-handler-test') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM org_review_ai_settings WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	s := &Server{db: db}
	e := echo.New()

	preferred := 999 // way above the system ceiling of 10
	reqBody, _ := json.Marshal(ReviewAISettingsRequest{
		HelperEnabled:                 true,
		HelperMode:                    storageaiconnectors.HelperModeConciseThenExpand,
		PreferredMaxConcurrentBatches: &preferred,
	})
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/aiconnectors/settings", bytes.NewReader(reqBody))
	putReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	putRec := httptest.NewRecorder()
	putC := e.NewContext(putReq, putRec)
	putC.Set("org_id", orgID)

	if err := s.UpsertReviewAISettings(putC); err != nil {
		t.Fatalf("UpsertReviewAISettings: %v", err)
	}
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", putRec.Code, putRec.Body.String())
	}

	var resp ReviewAISettingsResponse
	if err := json.Unmarshal(putRec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.PreferredMaxConcurrentBatches == nil || *resp.PreferredMaxConcurrentBatches != 999 {
		t.Fatalf("saved preference = %v, want the raw 999 the org asked for", resp.PreferredMaxConcurrentBatches)
	}
	if resp.EffectiveMaxConcurrentBatches != 10 {
		t.Fatalf("effective max concurrent batches = %d, want clamped to the system ceiling of 10", resp.EffectiveMaxConcurrentBatches)
	}
}
