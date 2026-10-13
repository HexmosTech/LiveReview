package canonicaldoc

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
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

// Build must never return a review that belongs to a different org, even
// when given a syntactically valid review id.
func TestBuildRejectsCrossOrgReview(t *testing.T) {
	db := testDB(t)

	// The live dev server's background dashboard-cache refresh job may insert a
	// row for a new org before the test ends; that FK has no cascade, so it
	// must be cleared before the org delete or the delete silently no-ops.
	cleanupOrg := func(orgID int64) {
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
			_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
		})
	}

	var orgAID, orgBID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('canonicaldoc-org-a') RETURNING id`).Scan(&orgAID); err != nil {
		t.Fatalf("insert org a: %v", err)
	}
	cleanupOrg(orgAID)
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('canonicaldoc-org-b') RETURNING id`).Scan(&orgBID); err != nil {
		t.Fatalf("insert org b: %v", err)
	}
	cleanupOrg(orgBID)

	var reviewID int64
	if err := db.QueryRow(`INSERT INTO reviews (repository, status, org_id, metadata) VALUES ('a/repo', 'completed', $1, '{}'::jsonb) RETURNING id`, orgAID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })

	if _, err := Build(context.Background(), db, orgBID, reviewID); err == nil {
		t.Fatalf("expected Build to fail when orgID doesn't own the review")
	}

	doc, err := Build(context.Background(), db, orgAID, reviewID)
	if err != nil {
		t.Fatalf("Build with the owning org failed: %v", err)
	}
	if doc.ReviewID != reviewID || doc.OrgID != orgAID {
		t.Fatalf("unexpected doc: %+v", doc)
	}
	for _, sev := range KnownSeverities {
		if _, ok := doc.Counts.BySeverity[sev]; !ok {
			t.Fatalf("expected by_severity to be zero-filled for %q", sev)
		}
	}
}

func TestRunJQBoolTruthiness(t *testing.T) {
	doc := &ReviewDoc{Counts: Counts{BySeverity: map[string]int{"critical": 1}}}

	blocked, _, err := RunJQBool(".counts.by_severity.critical > 0", doc)
	if err != nil {
		t.Fatalf("RunJQBool: %v", err)
	}
	if !blocked {
		t.Fatalf("expected blocked=true for a critical finding")
	}

	blocked, _, err = RunJQBool(".counts.by_severity.critical > 5", doc)
	if err != nil {
		t.Fatalf("RunJQBool: %v", err)
	}
	if blocked {
		t.Fatalf("expected blocked=false when the condition doesn't hold")
	}

	if _, _, err := RunJQBool("not valid jq [[", doc); err == nil {
		t.Fatalf("expected an error for an invalid jq expression")
	}
}
