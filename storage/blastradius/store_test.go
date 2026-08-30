package blastradius

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/blastradius"
)

// testDB opens the local dev DB (same DATABASE_URL every other package's
// integration test in this repo reads) and skips the test entirely when
// it's not reachable, matching this repo's existing pattern for tests that
// need Postgres.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set, skipping storage/blastradius integration test")
	}
	db, err := sql.Open("postgres", url)
	if err != nil {
		t.Skipf("could not open DATABASE_URL: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("could not ping DATABASE_URL: %v", err)
	}
	return db
}

// firstRealReviewAndOrg picks any existing (review_id, org_id) pair from the
// reviews table, so this test respects the real FK constraints without
// hardcoding an id that may not exist in whichever DB it runs against.
func firstRealReviewAndOrg(t *testing.T, db *sql.DB) (reviewID, orgID int64) {
	t.Helper()
	err := db.QueryRow(`SELECT id, org_id FROM reviews ORDER BY id LIMIT 1`).Scan(&reviewID, &orgID)
	if err != nil {
		t.Skipf("no reviews row available to test against: %v", err)
	}
	return reviewID, orgID
}

func loadGoldenHunks(t *testing.T) []blastradius.HunkReport {
	t.Helper()
	data, err := os.ReadFile("../../internal/blastradius/testdata/review_11632_report.json")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	var r blastradius.Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("unmarshal testdata: %v", err)
	}
	var hunks []blastradius.HunkReport
	for _, f := range r.Files {
		hunks = append(hunks, f.Hunks...)
	}
	return hunks
}

func TestReplaceHunksForReview_RoundTrip(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	reviewID, orgID := firstRealReviewAndOrg(t, db)
	store := NewStore(db)
	ctx := context.Background()

	// Clean slate regardless of what ran here before.
	if err := store.ReplaceHunksForReview(ctx, orgID, reviewID, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}

	allHunks := loadGoldenHunks(t)
	if err := store.ReplaceHunksForReview(ctx, orgID, reviewID, allHunks); err != nil {
		t.Fatalf("replace with %d hunks: %v", len(allHunks), err)
	}

	got, err := store.GetForReview(ctx, orgID, reviewID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != len(allHunks) {
		t.Fatalf("got %d rows, want %d", len(got), len(allHunks))
	}

	// Spot-check the multi-symbol max hunk round-trips its Combined/Tier/MathMode.
	key := HunkKey("ui/src/pages/Chatbot/rebucketChart.ts", 557, 96)
	stored, ok := got[key]
	if !ok {
		t.Fatalf("hunk %q not found in stored rows", key)
	}
	if stored.Combined != 100 {
		t.Errorf("Combined = %v, want 100", stored.Combined)
	}
	if stored.Tier != "blast-radius-high" {
		t.Errorf("Tier = %q, want blast-radius-high", stored.Tier)
	}
	if stored.MathMode.Final < 99.99 || stored.MathMode.Final > 100.01 {
		t.Errorf("MathMode.Final = %v, want ~100", stored.MathMode.Final)
	}

	// The orphan-row regression: re-uploading with FEWER hunks must not
	// leave the vanished ones behind.
	fewerHunks := allHunks[:1]
	if err := store.ReplaceHunksForReview(ctx, orgID, reviewID, fewerHunks); err != nil {
		t.Fatalf("replace with %d hunks: %v", len(fewerHunks), err)
	}
	got2, err := store.GetForReview(ctx, orgID, reviewID)
	if err != nil {
		t.Fatalf("get after shrink: %v", err)
	}
	if len(got2) != len(fewerHunks) {
		t.Fatalf("after re-upload with %d hunks, got %d rows in DB, want %d (orphan rows left behind)",
			len(fewerHunks), len(got2), len(fewerHunks))
	}

	// Clean up after ourselves.
	if err := store.ReplaceHunksForReview(ctx, orgID, reviewID, nil); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
