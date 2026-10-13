package jobqueue

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	"github.com/riverqueue/river"
)

// Regression test: PRPolicyPublishWorker and PRPolicyForceApproveWorker once
// built their idempotency-claim UPDATE with a malformed string
// (`'{"key":"'" || now() || "'"}'::jsonb`), which is a Postgres syntax error
// on every single invocation — Force Approve and the landing-delay publish
// path never actually posted anything, though the audit trail looked fine.
// This runs the real worker against a real Postgres DB so the claim
// statement is actually executed, not just read.
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

func seedPolicyTestOrgAndReview(t *testing.T, db *sql.DB) (orgID, reviewID int64) {
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('pr-policy-claim-sql-test-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		// The live dev server's background dashboard-cache refresh job may insert
		// a row for this org before cleanup runs; that FK has no cascade, so it
		// must be cleared first or the org delete silently no-ops.
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	if err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, metadata)
		VALUES ('test/repo', 'completed', $1, '{}'::jsonb) RETURNING id`, orgID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })
	return orgID, reviewID
}

func TestPRPolicyPublishWorkerClaimSQLIsValid(t *testing.T) {
	db := testDB(t)
	orgID, reviewID := seedPolicyTestOrgAndReview(t, db)

	w := &PRPolicyPublishWorker{db: db}
	job := &river.Job[PRPolicyPublishJobArgs]{Args: PRPolicyPublishJobArgs{OrgID: orgID, ReviewID: reviewID}}

	// No pull_request_id on this review, so Work runs the claim UPDATE (exactly
	// the statement that used to be a SQL syntax error), then releases the
	// claim and returns nil since there's nothing to publish to.
	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work() returned an error (claim SQL likely broken again): %v", err)
	}
}

func TestPRPolicyForceApproveWorkerClaimSQLIsValid(t *testing.T) {
	db := testDB(t)
	orgID, reviewID := seedPolicyTestOrgAndReview(t, db)

	w := &PRPolicyForceApproveWorker{db: db}
	job := &river.Job[PRPolicyForceApproveJobArgs]{Args: PRPolicyForceApproveJobArgs{OrgID: orgID, ReviewID: reviewID, ActorUserID: 0, Reason: "test"}}

	if err := w.Work(context.Background(), job); err != nil {
		t.Fatalf("Work() returned an error (claim SQL likely broken again): %v", err)
	}
}
