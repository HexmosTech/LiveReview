package tools

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/license"
)

func testDatabaseURL() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	dir, err := os.Getwd()
	if err == nil {
		for {
			candidate := filepath.Join(dir, ".env")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				if f, err := os.Open(candidate); err == nil {
					sc := bufio.NewScanner(f)
					for sc.Scan() {
						line := strings.TrimSpace(sc.Text())
						if line == "" || strings.HasPrefix(line, "#") {
							continue
						}
						if strings.HasPrefix(line, "DATABASE_URL=") {
							_ = f.Close()
							return strings.Trim(strings.TrimPrefix(line, "DATABASE_URL="), "\"'")
						}
					}
					_ = f.Close()
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "postgres://livereview:livereview_password_123@localhost:5432/lr-tools?sslmode=disable"
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database integration test in short mode")
	}
	db, err := sql.Open("postgres", testDatabaseURL())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	return db
}

func createTestOrgAndReview(t *testing.T, db *sql.DB, orgName, repoName string) (int64, int64) {
	t.Helper()
	ctx := context.Background()

	var orgID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO public.orgs (name)
		VALUES ($1)
		RETURNING id
	`, orgName).Scan(&orgID)
	if err != nil {
		t.Fatalf("create test org: %v", err)
	}

	var reviewID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO public.reviews (repository, branch, commit_hash, status, org_id)
		VALUES ($1, 'main', 'abcdef123456', 'in_progress', $2)
		RETURNING id
	`, repoName, orgID).Scan(&reviewID)
	if err != nil {
		t.Fatalf("create test review: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM public.tool_credit_ledger WHERE org_id = $1`, orgID)
		_, _ = db.ExecContext(ctx, `DELETE FROM public.org_billing_state WHERE org_id = $1`, orgID)
		_, _ = db.ExecContext(ctx, `DELETE FROM public.reviews WHERE id = $1`, reviewID)
		_, _ = db.ExecContext(ctx, `DELETE FROM public.orgs WHERE id = $1`, orgID)
	})

	return orgID, reviewID
}

func getSequenceState(ctx context.Context, db *sql.DB) (int64, bool, error) {
	var lastValue int64
	var isCalled bool
	err := db.QueryRowContext(ctx, `
		SELECT last_value, is_called
		FROM public.tool_credit_ledger_id_seq
	`).Scan(&lastValue, &isCalled)
	return lastValue, isCalled, err
}

// Test 1: DeductCredits with multi-tool breakdown serializing to JSONB and storing in tool_credit_ledger.
func TestDeductCredits_MultiToolBreakdown(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgName := fmt.Sprintf("test-multi-tool-org-%d", os.Getpid())
	orgID, reviewID := createTestOrgAndReview(t, db, orgName, "test/multi-tool-repo")
	store := NewCreditStore(db)

	breakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 1.5},
		{ToolName: "gitleaks", Multiplier: 0.5},
		{ToolName: "govulncheck", Multiplier: 2.0},
	}
	expectedTotalMultiplier := 4.0

	err := store.DeductCredits(ctx, orgID, reviewID, breakdown, license.PlanTeam)
	if err != nil {
		t.Fatalf("DeductCredits failed: %v", err)
	}

	// Verify row was created in tool_credit_ledger
	var ledgerID int64
	var creditsDeducted float64
	var rawBreakdown []byte
	var idempotencyKey string
	err = db.QueryRowContext(ctx, `
		SELECT id, credits_deducted, breakdown, idempotency_key
		FROM public.tool_credit_ledger
		WHERE org_id = $1 AND review_id = $2
	`, orgID, reviewID).Scan(&ledgerID, &creditsDeducted, &rawBreakdown, &idempotencyKey)
	if err != nil {
		t.Fatalf("query tool_credit_ledger: %v", err)
	}

	if ledgerID <= 0 {
		t.Errorf("expected positive ledger ID, got %d", ledgerID)
	}

	if math.Abs(creditsDeducted-expectedTotalMultiplier) > 1e-4 {
		t.Errorf("expected credits_deducted=%.2f, got %.2f", expectedTotalMultiplier, creditsDeducted)
	}

	expectedKey := fmt.Sprintf("tool_review_%d", reviewID)
	if idempotencyKey != expectedKey {
		t.Errorf("expected idempotency_key=%s, got %s", expectedKey, idempotencyKey)
	}

	// Verify JSONB structure contains expected JSON keys ("tool" and "multiplier")
	var genericList []map[string]interface{}
	if err := json.Unmarshal(rawBreakdown, &genericList); err != nil {
		t.Fatalf("unmarshal raw JSONB into map slice: %v", err)
	}
	if len(genericList) != len(breakdown) {
		t.Fatalf("expected %d entries in JSONB, got %d", len(breakdown), len(genericList))
	}
	for i, item := range genericList {
		if _, ok := item["tool"]; !ok {
			t.Errorf("entry %d missing 'tool' key in JSONB: %s", i, string(rawBreakdown))
		}
		if _, ok := item["multiplier"]; !ok {
			t.Errorf("entry %d missing 'multiplier' key in JSONB: %s", i, string(rawBreakdown))
		}
	}

	// Verify billing state updated
	var creditsUsed float64
	err = db.QueryRowContext(ctx, `
		SELECT tool_credits_used_month
		FROM public.org_billing_state
		WHERE org_id = $1
	`, orgID).Scan(&creditsUsed)
	if err != nil {
		t.Fatalf("query org_billing_state: %v", err)
	}
	if math.Abs(creditsUsed-expectedTotalMultiplier) > 1e-4 {
		t.Errorf("expected tool_credits_used_month=%.2f, got %.2f", expectedTotalMultiplier, creditsUsed)
	}
}

// Test 2: Verifying that the breakdown preserves tool names and multipliers.
func TestDeductCredits_PreservesToolNamesAndMultipliers(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgName := fmt.Sprintf("test-preserves-breakdown-org-%d", os.Getpid())
	orgID, reviewID := createTestOrgAndReview(t, db, orgName, "test/preserves-breakdown-repo")
	store := NewCreditStore(db)

	testCases := []ToolCharge{
		{ToolName: "semgrep_oss", Multiplier: 1.25},
		{ToolName: "git-leaks-scanner", Multiplier: 0.75},
		{ToolName: "custom_analyzer_v2", Multiplier: 3.125},
		{ToolName: "eslint_security", Multiplier: 0.5},
	}

	var expectedTotal float64
	for _, tc := range testCases {
		expectedTotal += tc.Multiplier
	}

	err := store.DeductCredits(ctx, orgID, reviewID, testCases, license.PlanTeam)
	if err != nil {
		t.Fatalf("DeductCredits failed: %v", err)
	}

	var rawBreakdown []byte
	var creditsDeducted float64
	err = db.QueryRowContext(ctx, `
		SELECT credits_deducted, breakdown
		FROM public.tool_credit_ledger
		WHERE org_id = $1 AND review_id = $2
	`, orgID, reviewID).Scan(&creditsDeducted, &rawBreakdown)
	if err != nil {
		t.Fatalf("query tool_credit_ledger: %v", err)
	}

	if math.Abs(creditsDeducted-expectedTotal) > 1e-4 {
		t.Errorf("expected credits_deducted=%.4f, got %.4f", expectedTotal, creditsDeducted)
	}

	var stored []ToolCharge
	if err := json.Unmarshal(rawBreakdown, &stored); err != nil {
		t.Fatalf("unmarshal breakdown JSON: %v", err)
	}

	if len(stored) != len(testCases) {
		t.Fatalf("expected length %d, got %d", len(testCases), len(stored))
	}

	for i := range testCases {
		if stored[i].ToolName != testCases[i].ToolName {
			t.Errorf("item %d: expected ToolName=%q, got %q", i, testCases[i].ToolName, stored[i].ToolName)
		}
		if math.Abs(stored[i].Multiplier-testCases[i].Multiplier) > 1e-4 {
			t.Errorf("item %d: expected Multiplier=%.4f, got %.4f", i, testCases[i].Multiplier, stored[i].Multiplier)
		}
	}
}

// Test 3: Verifying that zero-tools breakdown returns nil without database writes.
func TestDeductCredits_ZeroToolsBreakdown_NoDatabaseWrites(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgName := fmt.Sprintf("test-zero-writes-org-%d", os.Getpid())
	orgID, reviewID := createTestOrgAndReview(t, db, orgName, "test/zero-writes-repo")
	store := NewCreditStore(db)

	// Check baseline row counts
	var ledgerCountBefore int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.tool_credit_ledger WHERE org_id = $1`, orgID).Scan(&ledgerCountBefore)
	if err != nil {
		t.Fatalf("query baseline ledger count: %v", err)
	}

	var billingCountBefore int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.org_billing_state WHERE org_id = $1`, orgID).Scan(&billingCountBefore)
	if err != nil {
		t.Fatalf("query baseline billing state count: %v", err)
	}

	// 1. Empty slice []ToolCharge{}
	err = store.DeductCredits(ctx, orgID, reviewID, []ToolCharge{}, license.PlanTeam)
	if err != nil {
		t.Errorf("expected nil for empty breakdown slice, got: %v", err)
	}

	// 2. Nil slice
	err = store.DeductCredits(ctx, orgID, reviewID, nil, license.PlanTeam)
	if err != nil {
		t.Errorf("expected nil for nil breakdown slice, got: %v", err)
	}

	// 3. Breakdown with total multiplier <= 0
	zeroMultiplierBreakdown := []ToolCharge{
		{ToolName: "free_tool", Multiplier: 0.0},
	}
	err = store.DeductCredits(ctx, orgID, reviewID, zeroMultiplierBreakdown, license.PlanTeam)
	if err != nil {
		t.Errorf("expected nil for zero-multiplier breakdown, got: %v", err)
	}

	// Verify no ledger rows were inserted
	var ledgerCountAfter int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.tool_credit_ledger WHERE org_id = $1`, orgID).Scan(&ledgerCountAfter)
	if err != nil {
		t.Fatalf("query after ledger count: %v", err)
	}
	if ledgerCountAfter != ledgerCountBefore {
		t.Errorf("tool_credit_ledger count changed: before=%d, after=%d", ledgerCountBefore, ledgerCountAfter)
	}

	// Verify no org_billing_state rows were created or modified
	var billingCountAfter int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.org_billing_state WHERE org_id = $1`, orgID).Scan(&billingCountAfter)
	if err != nil {
		t.Fatalf("query after billing state count: %v", err)
	}
	if billingCountAfter != billingCountBefore {
		t.Errorf("org_billing_state count changed: before=%d, after=%d", billingCountBefore, billingCountAfter)
	}
}

// Test 4: Verifying that no sequence calls are made.
func TestDeductCredits_ZeroToolsBreakdown_NoSequenceCalls(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgName := fmt.Sprintf("test-seq-calls-org-%d", os.Getpid())
	orgID, reviewID := createTestOrgAndReview(t, db, orgName, "test/seq-calls-repo")
	store := NewCreditStore(db)

	// Step A: Record initial sequence state
	lastValBefore, isCalledBefore, err := getSequenceState(ctx, db)
	if err != nil {
		t.Fatalf("get sequence state before zero-tool calls: %v", err)
	}

	// Step B: Perform zero-tool breakdown operations
	if err := store.DeductCredits(ctx, orgID, reviewID, []ToolCharge{}, license.PlanTeam); err != nil {
		t.Fatalf("empty breakdown returned error: %v", err)
	}
	if err := store.DeductCredits(ctx, orgID, reviewID, nil, license.PlanTeam); err != nil {
		t.Fatalf("nil breakdown returned error: %v", err)
	}
	if err := store.DeductCredits(ctx, orgID, reviewID, []ToolCharge{{ToolName: "zero", Multiplier: 0.0}}, license.PlanTeam); err != nil {
		t.Fatalf("zero-multiplier breakdown returned error: %v", err)
	}

	// Step C: Verify sequence state is completely unchanged
	lastValAfter, isCalledAfter, err := getSequenceState(ctx, db)
	if err != nil {
		t.Fatalf("get sequence state after zero-tool calls: %v", err)
	}

	if lastValAfter != lastValBefore || isCalledAfter != isCalledBefore {
		t.Fatalf("sequence was modified! Before: (last_val=%d, is_called=%v), After: (last_val=%d, is_called=%v)",
			lastValBefore, isCalledBefore, lastValAfter, isCalledAfter)
	}

	// Step D: Contrast check - verify that a real multi-tool deduction DOES advance the sequence
	activeBreakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 1.0},
	}
	if err := store.DeductCredits(ctx, orgID, reviewID, activeBreakdown, license.PlanTeam); err != nil {
		t.Fatalf("active breakdown deduction failed: %v", err)
	}

	lastValAfterActive, isCalledAfterActive, err := getSequenceState(ctx, db)
	if err != nil {
		t.Fatalf("get sequence state after active deduction: %v", err)
	}

	if !isCalledAfterActive {
		t.Errorf("expected sequence is_called=true after active insertion")
	}
	if isCalledBefore && lastValAfterActive <= lastValBefore {
		t.Errorf("expected sequence last_value to increase after active insertion, before=%d, after=%d",
			lastValBefore, lastValAfterActive)
	}
}

// Test 5: Idempotency check - duplicate review deductions do not duplicate ledger entries or double-bill.
func TestDeductCredits_Idempotency(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgName := fmt.Sprintf("test-idempotent-org-%d", os.Getpid())
	orgID, reviewID := createTestOrgAndReview(t, db, orgName, "test/idempotent-repo")
	store := NewCreditStore(db)

	breakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 2.0},
	}

	// First deduction
	if err := store.DeductCredits(ctx, orgID, reviewID, breakdown, license.PlanTeam); err != nil {
		t.Fatalf("first DeductCredits call failed: %v", err)
	}

	// Second deduction with same reviewID (should be idempotent via idempotency_key)
	if err := store.DeductCredits(ctx, orgID, reviewID, breakdown, license.PlanTeam); err != nil {
		t.Fatalf("second DeductCredits call failed: %v", err)
	}

	// Verify only 1 ledger row exists
	var ledgerCount int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.tool_credit_ledger WHERE org_id = $1 AND review_id = $2`, orgID, reviewID).Scan(&ledgerCount)
	if err != nil {
		t.Fatalf("query ledger count: %v", err)
	}
	if ledgerCount != 1 {
		t.Errorf("expected exactly 1 ledger row for idempotent deduction, got %d", ledgerCount)
	}

	// Verify usage is 2.0, not 4.0
	var creditsUsed float64
	err = db.QueryRowContext(ctx, `SELECT tool_credits_used_month FROM public.org_billing_state WHERE org_id = $1`, orgID).Scan(&creditsUsed)
	if err != nil {
		t.Fatalf("query billing state: %v", err)
	}
	if math.Abs(creditsUsed-2.0) > 1e-4 {
		t.Errorf("expected tool_credits_used_month=2.00, got %.2f", creditsUsed)
	}
}

// Test 6: Ineligible plan rejects deduction.
func TestDeductCredits_IneligiblePlan(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	orgName := fmt.Sprintf("test-ineligible-org-%d", os.Getpid())
	orgID, reviewID := createTestOrgAndReview(t, db, orgName, "test/ineligible-repo")
	store := NewCreditStore(db)

	breakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 1.0},
	}

	ineligiblePlan := license.PlanType("invalid_unknown_plan")
	err := store.DeductCredits(ctx, orgID, reviewID, breakdown, ineligiblePlan)
	if err == nil {
		t.Fatalf("expected error for ineligible plan, got nil")
	}

	// Verify no ledger writes
	var ledgerCount int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.tool_credit_ledger WHERE org_id = $1`, orgID).Scan(&ledgerCount)
	if ledgerCount != 0 {
		t.Errorf("expected 0 ledger rows, got %d", ledgerCount)
	}
}
