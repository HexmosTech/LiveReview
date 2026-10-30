package license

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

// TestGetReviewAccountingTotalsNoRowsReturnsZero is a regression test: with
// zero matching loc_usage_ledger rows (every review, until its first LLM call
// is accounted), Postgres's bare SUM() aggregates return NULL, not 0 - only
// total_billable_loc was wrapped in COALESCE, so scanning the unwrapped
// token_tracked_ops SUM into a non-nullable int64 failed with a scan error,
// which the API handler (GetReviewAccounting) turned into a 500 on every
// freshly-triggered review the frontend polled.
func TestGetReviewAccountingTotalsNoRowsReturnsZero(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	var orgID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO orgs (name) VALUES ('accounting-test-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM orgs WHERE id = $1`, orgID) })

	var reviewID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO reviews (repository, status, org_id, metadata)
		VALUES ('test/repo', 'in_progress', $1, '{}'::jsonb) RETURNING id`, orgID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.ExecContext(ctx, `DELETE FROM reviews WHERE id = $1`, reviewID) })

	store := NewReviewAccountingStore(db)
	totals, err := store.GetReviewAccountingTotals(ctx, orgID, reviewID)
	if err != nil {
		t.Fatalf("GetReviewAccountingTotals with no ledger rows returned an error: %v", err)
	}
	if totals.TokenTrackedOps != 0 {
		t.Errorf("expected TokenTrackedOps = 0, got %d", totals.TokenTrackedOps)
	}
	if totals.TotalBillableLOC != 0 {
		t.Errorf("expected TotalBillableLOC = 0, got %d", totals.TotalBillableLOC)
	}
	if totals.AccountedOperations != 0 {
		t.Errorf("expected AccountedOperations = 0, got %d", totals.AccountedOperations)
	}
}
