package license

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func TestLOCAccountingService_ConcurrentIdempotency(t *testing.T) {
	dsn := getDatabaseURL()
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB-backed concurrency test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}

	var orgID int64
	if err := db.QueryRow(`SELECT id FROM orgs ORDER BY id LIMIT 1`).Scan(&orgID); err != nil {
		t.Skipf("no org rows available for concurrency test: %v", err)
	}

	var reviewID int64
	if err := db.QueryRow(`SELECT id FROM reviews WHERE org_id = $1 ORDER BY id LIMIT 1`, orgID).Scan(&reviewID); err != nil {
		t.Skipf("no review rows available for org %d: %v", orgID, err)
	}

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	_, err = db.Exec(`
		INSERT INTO org_billing_state (org_id, current_plan_code, billing_period_start, billing_period_end, loc_used_month, loc_blocked, last_reset_at)
		VALUES ($1, $2, $3, $4, 0, FALSE, NOW())
		ON CONFLICT (org_id) DO UPDATE SET current_plan_code = EXCLUDED.current_plan_code, billing_period_start = EXCLUDED.billing_period_start, billing_period_end = EXCLUDED.billing_period_end
	`, orgID, PlanStarter100K.String(), periodStart, periodEnd)
	if err != nil {
		t.Fatalf("ensure org_billing_state: %v", err)
	}

	var baseline int64
	if err := db.QueryRow(`SELECT loc_used_month FROM org_billing_state WHERE org_id = $1`, orgID).Scan(&baseline); err != nil {
		t.Fatalf("read baseline usage: %v", err)
	}

	prefix := fmt.Sprintf("test-concurrency-%d", time.Now().UnixNano())
	defer func() {
		_, _ = db.Exec(`DELETE FROM loc_usage_ledger WHERE operation_id LIKE $1`, prefix+"%")
		_, _ = db.Exec(`UPDATE org_billing_state SET loc_used_month = $1, loc_blocked = FALSE WHERE org_id = $2`, baseline, orgID)
	}()

	svc := NewLOCAccountingService(db)

	// Concurrent duplicate idempotency calls should be counted once.
	dup := LOCAccountSuccessInput{
		OrgID:          orgID,
		ReviewID:       &reviewID,
		OperationType:  "manual_review",
		TriggerSource:  "manual",
		OperationID:    prefix + "-dup-op",
		IdempotencyKey: prefix + "-dup-key",
		BillableLOC:    50,
		PlanCode:       PlanStarter100K,
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = svc.AccountSuccess(t.Context(), dup)
		}()
	}
	wg.Wait()

	var afterDup int64
	if err := db.QueryRow(`SELECT loc_used_month FROM org_billing_state WHERE org_id = $1`, orgID).Scan(&afterDup); err != nil {
		t.Fatalf("read post-dup usage: %v", err)
	}
	if got := afterDup - baseline; got != 50 {
		t.Fatalf("duplicate idempotency delta = %d, want 50", got)
	}

	// Concurrent unique idempotency calls should all be counted.
	for i := 0; i < 6; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			resolvedReviewID := reviewID
			_ = svc.AccountSuccess(t.Context(), LOCAccountSuccessInput{
				OrgID:          orgID,
				ReviewID:       &resolvedReviewID,
				OperationType:  "manual_review",
				TriggerSource:  "manual",
				OperationID:    fmt.Sprintf("%s-uniq-op-%d", prefix, i),
				IdempotencyKey: fmt.Sprintf("%s-uniq-key-%d", prefix, i),
				BillableLOC:    10,
				PlanCode:       PlanStarter100K,
			})
		}()
	}
	wg.Wait()

	var finalUsed int64
	if err := db.QueryRow(`SELECT loc_used_month FROM org_billing_state WHERE org_id = $1`, orgID).Scan(&finalUsed); err != nil {
		t.Fatalf("read final usage: %v", err)
	}
	if got := finalUsed - baseline; got != 110 {
		t.Fatalf("final usage delta = %d, want 110", got)
	}
}
