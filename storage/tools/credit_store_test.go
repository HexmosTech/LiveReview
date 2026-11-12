package tools

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livereview/internal/license"
)

// recordedCall records a query or exec call made to the mock database.
type recordedCall struct {
	Query string
	Args  []driver.NamedValue
}

// mockCreditConn implements driver.Conn, driver.ConnBeginTx, driver.ExecerContext, and driver.QueryerContext.
type mockCreditConn struct {
	t             *testing.T
	mu            sync.Mutex
	billingUsed   float64
	billingLimit  float64
	periodStart   time.Time
	periodEnd     time.Time
	ledgerID      int64
	beginCount    int
	commitCount   int
	rollbackCount int
	execCalls     []recordedCall
	queryCalls    []recordedCall
	onExec        func(query string, args []driver.NamedValue) (driver.Result, error)
	onQuery       func(query string, args []driver.NamedValue) (driver.Rows, error)
}

func (c *mockCreditConn) Prepare(query string) (driver.Stmt, error) {
	return &mockCreditStmt{conn: c, query: query}, nil
}

func (c *mockCreditConn) Close() error {
	return nil
}

func (c *mockCreditConn) Begin() (driver.Tx, error) {
	c.mu.Lock()
	c.beginCount++
	c.mu.Unlock()
	return &mockCreditTx{conn: c}, nil
}

func (c *mockCreditConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.mu.Lock()
	c.beginCount++
	c.mu.Unlock()
	return &mockCreditTx{conn: c}, nil
}

func (c *mockCreditConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.mu.Lock()
	c.execCalls = append(c.execCalls, recordedCall{Query: query, Args: args})
	handler := c.onExec
	c.mu.Unlock()

	if handler != nil {
		return handler(query, args)
	}

	return mockCreditResult{rowsAffected: 1}, nil
}

func (c *mockCreditConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.mu.Lock()
	c.queryCalls = append(c.queryCalls, recordedCall{Query: query, Args: args})
	handler := c.onQuery
	billingUsed := c.billingUsed
	billingLimit := c.billingLimit
	periodStart := c.periodStart
	periodEnd := c.periodEnd
	ledgerID := c.ledgerID
	c.mu.Unlock()

	if handler != nil {
		return handler(query, args)
	}

	if strings.Contains(query, "SELECT tool_credits_used_month") {
		cols := []string{"tool_credits_used_month", "tool_credits_limit_month", "billing_period_start", "billing_period_end"}
		row := []driver.Value{billingUsed, billingLimit, periodStart, periodEnd}
		return &mockCreditRows{columns: cols, rows: [][]driver.Value{row}}, nil
	}

	if strings.Contains(query, "INSERT INTO public.tool_credit_ledger") {
		cols := []string{"id"}
		if ledgerID > 0 {
			row := []driver.Value{ledgerID}
			return &mockCreditRows{columns: cols, rows: [][]driver.Value{row}}, nil
		}
		// When ledgerID <= 0, simulate conflict DO NOTHING returning no rows
		return &mockCreditRows{columns: cols, rows: nil}, nil
	}

	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (c *mockCreditConn) TotalCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.execCalls) + len(c.queryCalls) + c.beginCount
}

func (c *mockCreditConn) CountExecsMatching(pattern string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, call := range c.execCalls {
		if strings.Contains(call.Query, pattern) {
			count++
		}
	}
	return count
}

func (c *mockCreditConn) CountQueriesMatching(pattern string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for _, call := range c.queryCalls {
		if strings.Contains(call.Query, pattern) {
			count++
		}
	}
	return count
}

func (c *mockCreditConn) FindExec(pattern string) *recordedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.execCalls {
		if strings.Contains(call.Query, pattern) {
			copied := call
			return &copied
		}
	}
	return nil
}

func (c *mockCreditConn) FindQuery(pattern string) *recordedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, call := range c.queryCalls {
		if strings.Contains(call.Query, pattern) {
			copied := call
			return &copied
		}
	}
	return nil
}

type mockCreditStmt struct {
	conn  *mockCreditConn
	query string
}

func (s *mockCreditStmt) Close() error  { return nil }
func (s *mockCreditStmt) NumInput() int { return -1 }

func (s *mockCreditStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}

func (s *mockCreditStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

type mockCreditTx struct {
	conn *mockCreditConn
}

func (t *mockCreditTx) Commit() error {
	t.conn.mu.Lock()
	t.conn.commitCount++
	t.conn.mu.Unlock()
	return nil
}

func (t *mockCreditTx) Rollback() error {
	t.conn.mu.Lock()
	t.conn.rollbackCount++
	t.conn.mu.Unlock()
	return nil
}

type mockCreditRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *mockCreditRows) Columns() []string { return r.columns }
func (r *mockCreditRows) Close() error     { return nil }

func (r *mockCreditRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.idx] {
		dest[i] = v
	}
	r.idx++
	return nil
}

type mockCreditResult struct {
	rowsAffected int64
}

func (r mockCreditResult) LastInsertId() (int64, error) { return 0, nil }
func (r mockCreditResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

type mockCreditDriver struct {
	mu    sync.Mutex
	mocks map[string]*mockCreditConn
}

func (d *mockCreditDriver) Open(name string) (driver.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	conn, ok := d.mocks[name]
	if !ok {
		return nil, fmt.Errorf("mock connection not registered for %s", name)
	}
	return conn, nil
}

var (
	globalCreditMockDriver = &mockCreditDriver{
		mocks: make(map[string]*mockCreditConn),
	}
	testDBCounter int64
)

func init() {
	sql.Register("credit-store-mock", globalCreditMockDriver)
}

func newMockCreditDB(t *testing.T) (*mockCreditConn, *CreditStore, *sql.DB) {
	t.Helper()
	n := atomic.AddInt64(&testDBCounter, 1)
	dsn := fmt.Sprintf("mock-credit-db-%d-%d", time.Now().UnixNano(), n)

	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	conn := &mockCreditConn{
		t:            t,
		billingUsed:  0.0,
		billingLimit: 50000.0,
		periodStart:  startOfMonth,
		periodEnd:    endOfMonth,
		ledgerID:     1,
	}

	globalCreditMockDriver.mu.Lock()
	globalCreditMockDriver.mocks[dsn] = conn
	globalCreditMockDriver.mu.Unlock()

	db, err := sql.Open("credit-store-mock", dsn)
	if err != nil {
		t.Fatalf("failed to open mock db: %v", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	t.Cleanup(func() {
		_ = db.Close()
		globalCreditMockDriver.mu.Lock()
		delete(globalCreditMockDriver.mocks, dsn)
		globalCreditMockDriver.mu.Unlock()
	})

	store := NewCreditStore(db)
	return conn, store, db
}

// C1: DeductCredits with breakdown marshals JSON and deducts totalMultiplier.
func TestCreditStore_C1_DeductCreditsWithBreakdown(t *testing.T) {
	conn, store, _ := newMockCreditDB(t)
	conn.billingUsed = 100.0
	conn.billingLimit = 50000.0
	conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)
	conn.ledgerID = 42

	ctx := context.Background()
	breakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 1.5},
		{ToolName: "eslint", Multiplier: 2.0},
	}

	err := store.DeductCredits(ctx, 42, 999, breakdown, license.PlanTeam)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ledger query was called
	ledgerCall := conn.FindQuery("INSERT INTO public.tool_credit_ledger")
	if ledgerCall == nil {
		t.Fatalf("expected ledger insert query to be executed")
	}

	// Verify ledger arguments: orgID=42, reviewID=999, multiplier=3.5, breakdownJSON, idempotencyKey
	if gotOrg, ok := ledgerCall.Args[0].Value.(int64); !ok || gotOrg != 42 {
		t.Errorf("expected orgID 42, got %v", ledgerCall.Args[0].Value)
	}
	if gotReview, ok := ledgerCall.Args[1].Value.(int64); !ok || gotReview != 999 {
		t.Errorf("expected reviewID 999, got %v", ledgerCall.Args[1].Value)
	}
	if gotMult, ok := ledgerCall.Args[2].Value.(float64); !ok || gotMult != 3.5 {
		t.Errorf("expected multiplier 3.5, got %v", ledgerCall.Args[2].Value)
	}

	rawJSON, ok := ledgerCall.Args[3].Value.([]byte)
	if !ok {
		t.Fatalf("expected breakdownJSON to be []byte, got %T", ledgerCall.Args[3].Value)
	}
	var unmarshaled []ToolCharge
	if err := json.Unmarshal(rawJSON, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal breakdown JSON: %v", err)
	}
	if len(unmarshaled) != 2 || unmarshaled[0].ToolName != "semgrep" || unmarshaled[0].Multiplier != 1.5 ||
		unmarshaled[1].ToolName != "eslint" || unmarshaled[1].Multiplier != 2.0 {
		t.Errorf("unexpected unmarshaled breakdown: %+v", unmarshaled)
	}

	if gotKey, ok := ledgerCall.Args[4].Value.(string); !ok || gotKey != "tool_review_999" {
		t.Errorf("expected idempotencyKey 'tool_review_999', got %v", ledgerCall.Args[4].Value)
	}

	// Verify billing state update was called with newUsed = 100.0 + 3.5 = 103.5
	updateCall := conn.FindExec("UPDATE public.org_billing_state")
	if updateCall == nil {
		t.Fatalf("expected org_billing_state update to be executed")
	}
	if gotNewUsed, ok := updateCall.Args[0].Value.(float64); !ok || gotNewUsed != 103.5 {
		t.Errorf("expected newUsed 103.5, got %v", updateCall.Args[0].Value)
	}
	if gotOrg, ok := updateCall.Args[1].Value.(int64); !ok || gotOrg != 42 {
		t.Errorf("expected orgID 42 in update, got %v", updateCall.Args[1].Value)
	}

	if conn.commitCount != 1 {
		t.Errorf("expected 1 commit, got %d", conn.commitCount)
	}
}

// C2: DeductCredits idempotency: calling twice with same reviewID only deducts once.
func TestCreditStore_C2_DeductCreditsIdempotency(t *testing.T) {
	conn, store, _ := newMockCreditDB(t)
	conn.billingUsed = 100.0
	conn.billingLimit = 50000.0
	conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)
	conn.ledgerID = 42

	ctx := context.Background()
	breakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 2.0},
	}

	// First call: succeeds and deducts
	err1 := store.DeductCredits(ctx, 42, 1001, breakdown, license.PlanTeam)
	if err1 != nil {
		t.Fatalf("first DeductCredits failed: %v", err1)
	}

	updatesAfterFirst := conn.CountExecsMatching("UPDATE public.org_billing_state")
	if updatesAfterFirst != 1 {
		t.Fatalf("expected 1 billing update after first call, got %d", updatesAfterFirst)
	}

	// Second call with same reviewID: simulate conflict by returning 0 rows (ledgerID = 0)
	conn.ledgerID = 0
	err2 := store.DeductCredits(ctx, 42, 1001, breakdown, license.PlanTeam)
	if err2 != nil {
		t.Fatalf("second DeductCredits failed: %v", err2)
	}

	// Verify billing update was NOT executed again
	updatesAfterSecond := conn.CountExecsMatching("UPDATE public.org_billing_state")
	if updatesAfterSecond != 1 {
		t.Errorf("expected billing update count to remain 1, got %d", updatesAfterSecond)
	}
}

// C3: DeductCredits with empty breakdown or totalMultiplier <= 0 returns nil without DB interaction.
func TestCreditStore_C3_DeductCreditsZeroOrEmpty(t *testing.T) {
	tests := []struct {
		name      string
		breakdown []ToolCharge
	}{
		{"nil breakdown", nil},
		{"empty breakdown", []ToolCharge{}},
		{"single zero multiplier", []ToolCharge{{ToolName: "noop", Multiplier: 0.0}}},
		{"multiple zero multipliers", []ToolCharge{{ToolName: "a", Multiplier: 0.0}, {ToolName: "b", Multiplier: 0.0}}},
		{"negative multiplier", []ToolCharge{{ToolName: "neg", Multiplier: -1.5}}},
		{"sum is zero", []ToolCharge{{ToolName: "pos", Multiplier: 2.0}, {ToolName: "neg", Multiplier: -2.0}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conn, store, _ := newMockCreditDB(t)
			err := store.DeductCredits(context.Background(), 42, 1002, tc.breakdown, license.PlanTeam)
			if err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			if total := conn.TotalCalls(); total != 0 {
				t.Errorf("expected 0 DB interactions, got %d", total)
			}
		})
	}
}

// C4: DeductCredits rejects when remaining credits < totalMultiplier.
func TestCreditStore_C4_DeductCreditsInsufficientCredits(t *testing.T) {
	conn, store, _ := newMockCreditDB(t)
	conn.billingUsed = 49995.0
	conn.billingLimit = 50000.0
	conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

	ctx := context.Background()
	breakdown := []ToolCharge{
		{ToolName: "heavy_tool", Multiplier: 10.0},
	}

	err := store.DeductCredits(ctx, 42, 1003, breakdown, license.PlanTeam)
	if err == nil {
		t.Fatalf("expected error for insufficient credits, got nil")
	}

	expectedErrMsg := "insufficient tool credits: review requires 10.00 credits, but only 5.00 remaining this month"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("expected error %q, got %q", expectedErrMsg, err.Error())
	}

	// Verify ledger insert was NOT executed
	if count := conn.CountQueriesMatching("INSERT INTO public.tool_credit_ledger"); count != 0 {
		t.Errorf("expected 0 ledger queries, got %d", count)
	}

	// Verify billing update was NOT executed
	if count := conn.CountExecsMatching("UPDATE public.org_billing_state"); count != 0 {
		t.Errorf("expected 0 billing updates, got %d", count)
	}

	// Verify tx was rolled back
	if conn.rollbackCount != 1 {
		t.Errorf("expected 1 rollback, got %d", conn.rollbackCount)
	}
	if conn.commitCount != 0 {
		t.Errorf("expected 0 commits, got %d", conn.commitCount)
	}
}

// C5: DeductCredits rejects when planCode is ineligible (e.g. PlanCommunity).
func TestCreditStore_C5_DeductCreditsIneligiblePlan(t *testing.T) {
	ineligiblePlans := []license.PlanType{
		license.PlanFree30K,
		license.PlanEnterpriseSelfhosted,
		"community",
		"invalid-plan",
		"unsupported_tier",
	}

	breakdown := []ToolCharge{
		{ToolName: "semgrep", Multiplier: 1.0},
	}

	for _, plan := range ineligiblePlans {
		t.Run(string(plan), func(t *testing.T) {
			conn, store, _ := newMockCreditDB(t)
			err := store.DeductCredits(context.Background(), 42, 1004, breakdown, plan)
			if err == nil {
				t.Fatalf("expected error for ineligible plan %s, got nil", plan)
			}

			expectedSub := fmt.Sprintf("tools credits are not available on the %s plan", plan)
			if !strings.Contains(err.Error(), expectedSub) {
				t.Errorf("expected error containing %q, got %q", expectedSub, err.Error())
			}

			if total := conn.TotalCalls(); total != 0 {
				t.Errorf("expected 0 DB interactions, got %d", total)
			}
		})
	}
}

// C6: ensureAndLockBillingState resets usage and advances period if now >= periodEnd.
func TestCreditStore_C6_EnsureAndLockBillingStatePeriodReset(t *testing.T) {
	t.Run("expired period resets usage and updates period", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		now := time.Now().UTC()
		// period ended in the past
		conn.periodStart = now.AddDate(0, -2, 0)
		conn.periodEnd = now.AddDate(0, -1, 0)
		conn.billingUsed = 1250.0
		conn.billingLimit = 50000.0

		ctx := context.Background()
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer func() { _ = tx.Rollback() }()

		used, limit, err := store.ensureAndLockBillingState(ctx, tx, 55)
		if err != nil {
			t.Fatalf("ensureAndLockBillingState failed: %v", err)
		}

		if used != 0.0 {
			t.Errorf("expected used to be reset to 0.0, got %f", used)
		}
		if limit != 50000.0 {
			t.Errorf("expected limit 50000.0, got %f", limit)
		}

		// Verify reset UPDATE was executed
		resetCall := conn.FindExec("UPDATE public.org_billing_state")
		if resetCall == nil {
			t.Fatalf("expected billing reset update to be executed")
		}
		if !strings.Contains(resetCall.Query, "SET tool_credits_used_month = 0.0") {
			t.Errorf("expected reset query, got: %s", resetCall.Query)
		}

		// Arg 1: startOfMonth, Arg 2: endOfMonth, Arg 3: orgID (55)
		if gotOrg, ok := resetCall.Args[2].Value.(int64); !ok || gotOrg != 55 {
			t.Errorf("expected orgID 55 in reset update, got %v", resetCall.Args[2].Value)
		}
	})

	t.Run("active period does not reset usage", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		now := time.Now().UTC()
		// period ends in the future
		conn.periodStart = now.AddDate(0, 0, -5)
		conn.periodEnd = now.AddDate(0, 1, 0)
		conn.billingUsed = 1250.0
		conn.billingLimit = 50000.0

		ctx := context.Background()
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		defer func() { _ = tx.Rollback() }()

		used, limit, err := store.ensureAndLockBillingState(ctx, tx, 55)
		if err != nil {
			t.Fatalf("ensureAndLockBillingState failed: %v", err)
		}

		if used != 1250.0 {
			t.Errorf("expected used to remain 1250.0, got %f", used)
		}
		if limit != 50000.0 {
			t.Errorf("expected limit 50000.0, got %f", limit)
		}

		// Verify no reset UPDATE was executed
		if count := conn.CountExecsMatching("UPDATE public.org_billing_state"); count != 0 {
			t.Errorf("expected 0 billing update execs, got %d", count)
		}
	})
}

// C7: GetCreditUsage calculates usage, limit, and blocked status correctly.
func TestCreditStore_C7_GetCreditUsage(t *testing.T) {
	ctx := context.Background()

	t.Run("normal usage not blocked", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		conn.billingUsed = 10000.0
		conn.billingLimit = 50000.0
		conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

		usage, err := store.GetCreditUsage(ctx, 42, 50.0, license.PlanTeam)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage.CreditsUsedMonth != 10000.0 {
			t.Errorf("expected CreditsUsedMonth 10000.0, got %f", usage.CreditsUsedMonth)
		}
		if usage.CreditsLimitMonth != 50000.0 {
			t.Errorf("expected CreditsLimitMonth 50000.0, got %f", usage.CreditsLimitMonth)
		}
		// (50000 - 10000) / 50 = 40000 / 50 = 800
		if usage.ReviewsRemaining != 800 {
			t.Errorf("expected ReviewsRemaining 800, got %d", usage.ReviewsRemaining)
		}
		if usage.Blocked {
			t.Errorf("expected Blocked to be false, got true")
		}
	})

	t.Run("insufficient credits blocks", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		conn.billingUsed = 49990.0
		conn.billingLimit = 50000.0
		conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

		usage, err := store.GetCreditUsage(ctx, 42, 20.0, license.PlanTeam)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// remaining = 10.0 < 20.0
		if !usage.Blocked {
			t.Errorf("expected Blocked to be true, got false")
		}
		if usage.ReviewsRemaining != 0 {
			t.Errorf("expected ReviewsRemaining 0, got %d", usage.ReviewsRemaining)
		}
	})

	t.Run("usage exceeds limit clamps remaining credits to zero", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		conn.billingUsed = 52000.0
		conn.billingLimit = 50000.0
		conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

		usage, err := store.GetCreditUsage(ctx, 42, 5.0, license.PlanTeam)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !usage.Blocked {
			t.Errorf("expected Blocked to be true, got false")
		}
		if usage.ReviewsRemaining != 0 {
			t.Errorf("expected ReviewsRemaining 0, got %d", usage.ReviewsRemaining)
		}
	})

	t.Run("zero multiplier not blocked", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		conn.billingUsed = 50000.0
		conn.billingLimit = 50000.0
		conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

		usage, err := store.GetCreditUsage(ctx, 42, 0.0, license.PlanTeam)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if usage.Blocked {
			t.Errorf("expected Blocked to be false when multiplier <= 0, got true")
		}
		if usage.ReviewsRemaining != 0 {
			t.Errorf("expected ReviewsRemaining 0, got %d", usage.ReviewsRemaining)
		}
	})

	t.Run("ineligible plan returns error", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		_, err := store.GetCreditUsage(ctx, 42, 10.0, license.PlanType("community"))
		if err == nil {
			t.Fatalf("expected error for ineligible plan, got nil")
		}
		if !strings.Contains(err.Error(), "tools credits are not available on the community plan") {
			t.Errorf("unexpected error: %v", err)
		}
		if total := conn.TotalCalls(); total != 0 {
			t.Errorf("expected 0 DB interactions, got %d", total)
		}
	})
}

// C8: CheckCreditPreflight blocks when requiredMultiplier > remaining.
func TestCreditStore_C8_CheckCreditPreflight(t *testing.T) {
	ctx := context.Background()

	t.Run("blocks when requiredMultiplier exceeds remaining credits", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		conn.billingUsed = 49950.0
		conn.billingLimit = 50000.0
		conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

		err := store.CheckCreditPreflight(ctx, 42, 100.0, license.PlanTeam)
		if err == nil {
			t.Fatalf("expected error for insufficient credits, got nil")
		}

		expectedErrMsg := "insufficient tool credits: review requires 100.00 credits, but only 50.00 remaining this month"
		if !strings.Contains(err.Error(), expectedErrMsg) {
			t.Errorf("expected %q, got %q", expectedErrMsg, err.Error())
		}
	})

	t.Run("succeeds when credits are sufficient", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		conn.billingUsed = 10000.0
		conn.billingLimit = 50000.0
		conn.periodEnd = time.Now().UTC().AddDate(0, 1, 0)

		err := store.CheckCreditPreflight(ctx, 42, 100.0, license.PlanTeam)
		if err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("fails when plan is ineligible", func(t *testing.T) {
		conn, store, _ := newMockCreditDB(t)
		err := store.CheckCreditPreflight(ctx, 42, 10.0, license.PlanType("community"))
		if err == nil {
			t.Fatalf("expected error for ineligible plan, got nil")
		}
		if !strings.Contains(err.Error(), "tools credits are not available on the community plan") {
			t.Errorf("unexpected error: %v", err)
		}
		if total := conn.TotalCalls(); total != 0 {
			t.Errorf("expected 0 DB interactions, got %d", total)
		}
	})
}
