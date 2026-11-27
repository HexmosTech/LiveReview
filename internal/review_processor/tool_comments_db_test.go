package reviewprocessor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/livereview/pkg/models"
	"go.uber.org/goleak"
)

func openIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in -short mode")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping database integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not reachable: %v", err)
	}
	return db
}

func seedMetadataTestReview(t *testing.T, db *sql.DB) (orgID, reviewID int64) {
	t.Helper()
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('tool-comments-db-test-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	if err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, metadata)
		VALUES ('test/repo', 'in_progress', $1, '{}'::jsonb) RETURNING id
	`, orgID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })
	return orgID, reviewID
}

// Verifies that concurrent per-comment marker writes across many goroutines
// (both tools_posted_to_provider and tools_skipped_on_provider via leaf jsonb_set)
// survive subsequent atomic finalization (review_result + status='completed') on real Postgres.
func TestDB_PostingMarkersSurviveFinalization(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, baseline) })

	db := openIntegrationDB(t)
	ctx := context.Background()
	orgID, reviewID := seedMetadataTestReview(t, db)

	const numMarkers = 15
	var wg sync.WaitGroup
	errCh := make(chan error, numMarkers)
	postedKeys := make([]string, 0, numMarkers)
	skippedKeys := make([]string, 0, numMarkers)

	for i := 0; i < numMarkers; i++ {
		comment := &models.ReviewComment{
			ToolName: fmt.Sprintf("tool-%02d", i),
			RuleID:   fmt.Sprintf("R%03d", i),
			FilePath: "main.py",
			Line:     i + 1,
		}
		k := commentPostMarkerKey(comment)
		section := "tools_posted_to_provider"
		if i%5 == 0 {
			section = "tools_skipped_on_provider"
			skippedKeys = append(skippedKeys, k)
		} else {
			postedKeys = append(postedKeys, k)
		}
		wg.Add(1)
		go func(sec, key string) {
			defer wg.Done()
			if err := persistCommentMarkerWithRetry(ctx, db, orgID, reviewID, sec, key); err != nil {
				errCh <- fmt.Errorf("persistCommentMarkerWithRetry(%s, %s): %w", sec, key, err)
			}
		}(section, k)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent marker write failed: %v", err)
	}

	mergedComments := []*models.ReviewComment{
		{FilePath: "main.py", Line: 4, Content: "AI comment", Source: "ai"},
		{FilePath: "main.py", Line: 8, Content: "unused import", Source: "tool", ToolName: "ruff", RuleID: "F401"},
	}
	if transitioned := persistManualReviewResultDBWithStatus(ctx, db, reviewID, "Summary", mergedComments, nil, nil, "", "completed"); !transitioned {
		t.Fatalf("expected atomic finalization update to transition review to completed")
	}

	// Verify all concurrent posted + skipped markers AND review_result / status='completed' survived.
	markers := loadPostedToolMarkers(ctx, db, orgID, reviewID)
	if len(markers) != numMarkers {
		t.Fatalf("expected all %d concurrent markers to survive finalization, got %d (%+v)", numMarkers, len(markers), markers)
	}

	var rawMeta []byte
	var status string
	if err := db.QueryRowContext(ctx, `SELECT status, metadata FROM reviews WHERE id = $1 AND org_id = $2`, reviewID, orgID).Scan(&status, &rawMeta); err != nil {
		t.Fatalf("query review after finalization: %v", err)
	}
	if status != "completed" {
		t.Fatalf("expected status 'completed', got %q", status)
	}

	var parsed struct {
		AIDone bool `json:"ai_review_done"`
		Result struct {
			Summary  string                  `json:"summary"`
			Comments []*models.ReviewComment `json:"comments"`
		} `json:"review_result"`
		Posted  map[string]bool `json:"tools_posted_to_provider"`
		Skipped map[string]bool `json:"tools_skipped_on_provider"`
	}
	if err := json.Unmarshal(rawMeta, &parsed); err != nil {
		t.Fatalf("unmarshal finalized metadata: %v", err)
	}
	if !parsed.AIDone || parsed.Result.Summary != "Summary" || len(parsed.Result.Comments) != 2 {
		t.Fatalf("unexpected review_result after finalization: %+v", parsed)
	}
	for _, k := range postedKeys {
		if !parsed.Posted[k] {
			t.Fatalf("expected posted marker %q intact after finalization, got: %+v", k, parsed.Posted)
		}
	}
	for _, k := range skippedKeys {
		if !parsed.Skipped[k] {
			t.Fatalf("expected skipped marker %q intact after finalization, got: %+v", k, parsed.Skipped)
		}
	}
}

// Verifies concurrent JSONB metadata updates (tools_posted_to_provider vs review_result)
// and concurrent review_events inserts do not clobber keys or hit Postgres constraint collisions.
func TestDB_ConcurrentMetadataAndReviewEventWrites(t *testing.T) {
	baseline := goleak.IgnoreCurrent()
	t.Cleanup(func() { goleak.VerifyNone(t, baseline) })

	db := openIntegrationDB(t)
	ctx := context.Background()
	orgID, reviewID := seedMetadataTestReview(t, db)
	sink := NewDatabaseEventSink(db)

	const numTools = 15
	var wg sync.WaitGroup
	errCh := make(chan error, numTools*2+4)

	for i := 0; i < numTools; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			toolName := fmt.Sprintf("tool-%02d", idx)
			if err := markToolPostedToProvider(ctx, db, orgID, reviewID, toolName); err != nil {
				errCh <- fmt.Errorf("markToolPostedToProvider(%s): %w", toolName, err)
			}
			comments := []*models.ReviewComment{
				{FilePath: "app.py", Line: idx + 1, Content: fmt.Sprintf("finding-%d", idx), Source: "tool", ToolName: toolName},
			}
			if err := sink.EmitBatchEvent(ctx, reviewID, orgID, "tool-"+toolName, "completed", 0, len(comments), comments); err != nil {
				errCh <- fmt.Errorf("EmitBatchEvent(%s): %w", toolName, err)
			}
		}(i)
	}

	// Concurrent finalization metadata merge racing with tool markers & event inserts.
	for j := 0; j < 4; j++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			comments := []*models.ReviewComment{
				{FilePath: "main.go", Line: idx + 1, Content: "ai-finding", Source: "ai"},
			}
			persistManualReviewResultDB(ctx, db, reviewID, "Concurrent summary", comments, nil, nil, "")
		}(j)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent DB operation failed: %v", err)
	}

	markers := loadPostedToolMarkers(ctx, db, orgID, reviewID)
	if len(markers) != numTools {
		t.Fatalf("expected all %d concurrent tool markers to survive without JSONB clobbering, got %d (%+v)", numTools, len(markers), markers)
	}

	var batchCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM review_events WHERE review_id = $1 AND org_id = $2 AND event_type = 'batch'`, reviewID, orgID).Scan(&batchCount); err != nil {
		t.Fatalf("count review_events: %v", err)
	}
	if batchCount != numTools {
		t.Fatalf("expected %d concurrent batch events in review_events, got %d", numTools, batchCount)
	}
}
