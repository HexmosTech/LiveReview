package jobqueue

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/database"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/lrcconfig"
	storagetools "github.com/livereview/storage/tools"
)

// -------------------------------------------------------------------------
// Mock Database Infrastructure for Tool Dispatcher Tests
// -------------------------------------------------------------------------

type mockDBState struct {
	mu sync.Mutex

	queryCount int
	execCount  int
	queries    []string
	execs      []string

	failAllQueries      bool
	failDeductCredits   bool
	failReviewEvents    bool
	failReviewsMetadata bool

	orgSettingsJSON     []byte
	billingUsed         float64
	billingLimit        float64

	ledgerDeducted      float64
	ledgerBreakdown     []storagetools.ToolCharge
	idempotencyKey      string

	dispatchEvents      []string
	metaDispatchedCount int
}

type mockDBDriver struct {
	mu    sync.Mutex
	state *mockDBState
}

func (d *mockDBDriver) Open(name string) (driver.Conn, error) {
	return &mockDBConn{driver: d}, nil
}

type mockDBConn struct {
	driver *mockDBDriver
}

func (c *mockDBConn) Prepare(query string) (driver.Stmt, error) {
	return &mockDBStmt{conn: c, query: query}, nil
}

func (c *mockDBConn) Close() error { return nil }
func (c *mockDBConn) Begin() (driver.Tx, error) {
	c.driver.mu.Lock()
	state := c.driver.state
	c.driver.mu.Unlock()
	if state != nil && state.failDeductCredits {
		return nil, errors.New("simulated begin tx error")
	}
	return &mockDBTx{}, nil
}

type mockDBTx struct{}

func (t *mockDBTx) Commit() error   { return nil }
func (t *mockDBTx) Rollback() error { return nil }

type mockRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *mockRows) Columns() []string { return r.columns }
func (r *mockRows) Close() error     { return nil }
func (r *mockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.idx] {
		dest[i] = v
	}
	r.idx++
	return nil
}

type mockResult struct {
	rowsAffected int64
}

func (r mockResult) LastInsertId() (int64, error) { return 0, nil }
func (r mockResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

func (c *mockDBConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.driver.mu.Lock()
	state := c.driver.state
	c.driver.mu.Unlock()

	if state == nil {
		return nil, fmt.Errorf("unexpected query on nil state: %s", query)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.queryCount++
	state.queries = append(state.queries, query)

	if state.failAllQueries {
		return nil, errors.New("query failed: mock database failure")
	}

	// 1. system_settings (LoadCatalog)
	if strings.Contains(query, "system_settings") {
		// Return ErrNoRows so LoadCatalog gracefully falls back to defaultCatalog
		return &mockRows{columns: []string{"data"}, rows: nil}, nil
	}

	// 2. orgs (getOrgToolSettings)
	if strings.Contains(query, "orgs") && strings.Contains(query, "settings") {
		raw := state.orgSettingsJSON
		if raw == nil {
			raw = []byte("{}")
		}
		return &mockRows{
			columns: []string{"settings"},
			rows:    [][]driver.Value{{raw}},
		}, nil
	}

	// 3. org_billing_state (ensureAndLockBillingState in DeductCredits)
	if strings.Contains(query, "org_billing_state") {
		if state.failDeductCredits {
			return nil, errors.New("insufficient tool credits: review requires credits")
		}
		limit := state.billingLimit
		if limit <= 0 {
			limit = 50000.0
		}
		now := time.Now()
		return &mockRows{
			columns: []string{"tool_credits_used_month", "tool_credits_limit_month", "billing_period_start", "billing_period_end"},
			rows:    [][]driver.Value{{state.billingUsed, limit, now.Add(-24 * time.Hour), now.Add(24 * 30 * time.Hour)}},
		}, nil
	}

	// 4. tool_credit_ledger (insert in DeductCredits)
	if strings.Contains(query, "tool_credit_ledger") {
		if state.failDeductCredits {
			return nil, errors.New("failed to insert ledger")
		}
		for _, arg := range args {
			if arg.Ordinal == 3 {
				if v, ok := arg.Value.(float64); ok {
					state.ledgerDeducted = v
				}
			} else if arg.Ordinal == 4 {
				switch v := arg.Value.(type) {
				case []byte:
					_ = json.Unmarshal(v, &state.ledgerBreakdown)
				case string:
					_ = json.Unmarshal([]byte(v), &state.ledgerBreakdown)
				}
			} else if arg.Ordinal == 5 {
				if v, ok := arg.Value.(string); ok {
					state.idempotencyKey = v
				}
			}
		}
		return &mockRows{
			columns: []string{"id"},
			rows:    [][]driver.Value{{int64(1001)}},
		}, nil
	}

	return nil, fmt.Errorf("unhandled query in mock: %s", query)
}

func (c *mockDBConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.driver.mu.Lock()
	state := c.driver.state
	c.driver.mu.Unlock()

	if state == nil {
		return nil, fmt.Errorf("unexpected exec on nil state: %s", query)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	state.execCount++
	state.execs = append(state.execs, query)

	if state.failAllQueries {
		return nil, errors.New("exec failed: mock database failure")
	}

	// 1. UPDATE org_billing_state
	if strings.Contains(query, "org_billing_state") {
		if len(args) > 0 {
			if v, ok := args[0].Value.(float64); ok {
				state.billingUsed = v
			}
		}
		return mockResult{rowsAffected: 1}, nil
	}

	// 2. INSERT INTO review_events
	if strings.Contains(query, "review_events") {
		if state.failReviewEvents {
			return nil, errors.New("failed to insert review event")
		}
		for _, arg := range args {
			if arg.Ordinal == 3 {
				switch v := arg.Value.(type) {
				case []byte:
					state.dispatchEvents = append(state.dispatchEvents, string(v))
				case string:
					state.dispatchEvents = append(state.dispatchEvents, v)
				}
			}
		}
		return mockResult{rowsAffected: 1}, nil
	}

	// 3. UPDATE reviews SET metadata = ...
	if strings.Contains(query, "reviews") {
		if state.failReviewsMetadata {
			return nil, errors.New("failed to update reviews metadata")
		}
		if len(args) > 0 {
			switch v := args[0].Value.(type) {
			case int64:
				state.metaDispatchedCount = int(v)
			case int:
				state.metaDispatchedCount = v
			}
		}
		return mockResult{rowsAffected: 1}, nil
	}

	return mockResult{rowsAffected: 1}, nil
}

type mockDBStmt struct {
	conn  *mockDBConn
	query string
}

func (s *mockDBStmt) Close() error { return nil }
func (s *mockDBStmt) NumInput() int { return -1 }
func (s *mockDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}
func (s *mockDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

var (
	testMockDriver   mockDBDriver
	testMockDriverMu sync.Mutex
)

func init() {
	sql.Register("tooldispatcher-mock", &testMockDriver)
}

func setupMockDB(t *testing.T) (*sql.DB, *mockDBState) {
	testMockDriverMu.Lock()
	state := &mockDBState{
		billingLimit: 50000.0,
	}
	testMockDriver.mu.Lock()
	testMockDriver.state = state
	testMockDriver.mu.Unlock()

	db, err := sql.Open("tooldispatcher-mock", "")
	if err != nil {
		testMockDriverMu.Unlock()
		t.Fatalf("failed to open mock db: %v", err)
	}

	t.Cleanup(func() {
		_ = db.Close()
		testMockDriver.mu.Lock()
		testMockDriver.state = nil
		testMockDriver.mu.Unlock()
		testMockDriverMu.Unlock()
	})

	return db, state
}

func createTestJobQueue(t *testing.T, mockDB *sql.DB) *JobQueue {
	dbURL, err := database.LoadDatabaseURL()
	if err == nil {
		realDB, err := database.NewDB()
		if err == nil {
			defer realDB.Close()
			jq, err := NewJobQueue(dbURL, realDB)
			if err == nil {
				jq.db = mockDB
				t.Cleanup(func() {
					if jq.pool != nil {
						jq.pool.Close()
					}
				})
				return jq
			}
		}
	}
	return &JobQueue{db: mockDB}
}

// -------------------------------------------------------------------------
// Test Event Sink
// -------------------------------------------------------------------------

type testEventSink struct {
	mu     sync.Mutex
	events []struct {
		Level   string
		Message string
	}
}

func (s *testEventSink) EmitStatusEvent(ctx context.Context, reviewID, orgID int64, status string) error {
	return nil
}

func (s *testEventSink) EmitLogEvent(ctx context.Context, reviewID, orgID int64, level, message, batchID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, struct {
		Level   string
		Message string
	}{Level: level, Message: message})
	return nil
}

func (s *testEventSink) EmitBatchEvent(ctx context.Context, reviewID, orgID int64, batchID, status string, tokenEstimate, fileCount int, comments interface{}) error {
	return nil
}

func (s *testEventSink) EmitArtifactEvent(ctx context.Context, reviewID, orgID int64, kind, url, batchID string, sizeBytes int64, previewHead, previewTail string) error {
	return nil
}

func (s *testEventSink) EmitCompletionEvent(ctx context.Context, reviewID, orgID int64, resultSummary string, commentCount int, errorSummary string) error {
	return nil
}

// -------------------------------------------------------------------------
// Existing Field and Nil Tests
// -------------------------------------------------------------------------

func TestDispatchTools_NilDBError(t *testing.T) {
	ctx := context.Background()

	// In cloud mode, nil jq or nil db must return an error
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	var nilJQ *JobQueue
	if _, err := nilJQ.DispatchTools(ctx, ToolDispatchParams{}); err == nil {
		t.Error("expected error when jq is nil in cloud mode, got nil")
	}

	emptyJQ := &JobQueue{}
	if _, err := emptyJQ.DispatchTools(ctx, ToolDispatchParams{}); err == nil {
		t.Error("expected error when jq.db is nil in cloud mode, got nil")
	}

	// In self-hosted mode, DispatchTools short-circuits to (0, nil) without checking DB
	t.Setenv("LIVEREVIEW_IS_CLOUD", "")
	if count, err := nilJQ.DispatchTools(ctx, ToolDispatchParams{}); err != nil || count != 0 {
		t.Errorf("expected (0, nil) for nilJQ in self-hosted mode, got (%d, %v)", count, err)
	}
	if count, err := emptyJQ.DispatchTools(ctx, ToolDispatchParams{}); err != nil || count != 0 {
		t.Errorf("expected (0, nil) for emptyJQ in self-hosted mode, got (%d, %v)", count, err)
	}
}

func TestToolDispatchParams_FieldInitialization(t *testing.T) {
	params := ToolDispatchParams{
		OrgID:         123,
		ReviewID:      456,
		PlanCode:      "team_plan",
		DiffZipBase64: "dGVzdA==",
		LocalDiffs:   []lib.LocalCodeDiff{{NewPath: "main.go"}},
		LRCBundle:     lrcconfig.Bundle{Files: map[string][]byte{}},
		Logger:        nil,
		EventSink:     nil,
	}

	if params.OrgID != 123 || params.ReviewID != 456 || params.PlanCode != "team_plan" {
		t.Errorf("ToolDispatchParams fields not properly initialized: %+v", params)
	}
	if len(params.LocalDiffs) != 1 || params.LocalDiffs[0].NewPath != "main.go" {
		t.Errorf("ToolDispatchParams LocalDiffs mismatch: %+v", params.LocalDiffs)
	}
}

// -------------------------------------------------------------------------
// Required Tests D1 - D8
// -------------------------------------------------------------------------

// D1: When LIVEREVIEW_IS_CLOUD="" (self-hosted), DispatchTools returns 0, nil without querying DB.
func TestDispatchTools_D1_SelfHostedNoDBQuery(t *testing.T) {
	ctx := context.Background()
	mockDB, state := setupMockDB(t)
	// Fail all queries so that if any query is executed, it errors immediately
	state.failAllQueries = true

	jq := &JobQueue{db: mockDB}

	// Test with LIVEREVIEW_IS_CLOUD=""
	t.Setenv("LIVEREVIEW_IS_CLOUD", "")
	count, err := jq.DispatchTools(ctx, ToolDispatchParams{
		OrgID:    1,
		ReviewID: 100,
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "main.py"},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error in self-hosted mode, got %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tools dispatched in self-hosted mode, got %d", count)
	}
	if state.queryCount != 0 {
		t.Fatalf("expected 0 queries in self-hosted mode, got %d", state.queryCount)
	}
	if state.execCount != 0 {
		t.Fatalf("expected 0 execs in self-hosted mode, got %d", state.execCount)
	}

	// Test with LIVEREVIEW_IS_CLOUD="false"
	t.Setenv("LIVEREVIEW_IS_CLOUD", "false")
	count, err = jq.DispatchTools(ctx, ToolDispatchParams{
		OrgID:    1,
		ReviewID: 100,
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "main.py"},
		},
	})
	if err != nil {
		t.Fatalf("expected nil error for LIVEREVIEW_IS_CLOUD=false, got %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tools dispatched for LIVEREVIEW_IS_CLOUD=false, got %d", count)
	}
	if state.queryCount != 0 {
		t.Fatalf("expected 0 queries for LIVEREVIEW_IS_CLOUD=false, got %d", state.queryCount)
	}
}

// Ineligible plans (free, community, self-hosted, empty) short-circuit to 0 tools without querying or dispatching.
func TestDispatchTools_IneligiblePlan(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)
	// Fail all queries so that if any query is executed, it errors immediately
	state.failAllQueries = true
	jq := &JobQueue{db: mockDB}

	ineligiblePlans := []string{
		string(license.PlanFree30K),
		string(license.PlanEnterpriseSelfhosted),
		"free",
		"community",
		"",
	}

	for _, plan := range ineligiblePlans {
		count, err := jq.DispatchTools(ctx, ToolDispatchParams{
			OrgID:    1,
			ReviewID: 100,
			PlanCode: plan,
			LocalDiffs: []lib.LocalCodeDiff{
				{NewPath: "main.py"},
			},
		})
		if err != nil {
			t.Fatalf("expected nil error for plan %q, got %v", plan, err)
		}
		if count != 0 {
			t.Fatalf("expected 0 tools dispatched for ineligible plan %q, got %d", plan, count)
		}
	}
}

// D2: When LIVEREVIEW_IS_CLOUD="true" (cloud), tools are evaluated and dispatched.
func TestDispatchTools_D2_CloudDispatched(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)

	// Org has ruff and gitleaks enabled
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)

	jq := createTestJobQueue(t, mockDB)

	params := ToolDispatchParams{
		OrgID:         1,
		ReviewID:      200,
		PlanCode:      string(license.PlanTeam),
		DiffZipBase64: "UEsDBBQAAAAIA...",
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "service.py"},
			{NewPath: "secrets.env"},
		},
	}

	count, err := jq.DispatchTools(ctx, params)
	if err != nil {
		t.Fatalf("DispatchTools failed in cloud mode: %v", err)
	}

	// Both tools should be evaluated and dispatched
	if count != 2 {
		t.Fatalf("expected 2 tools dispatched, got %d", count)
	}

	// Verify DeductCredits recorded ledger insert with 2 tools
	if len(state.ledgerBreakdown) != 2 {
		t.Fatalf("expected 2 tools in ledger breakdown, got %d: %+v", len(state.ledgerBreakdown), state.ledgerBreakdown)
	}

	// ruff (1.0) + gitleaks (12.0) = 13.0
	expectedDeduction := 13.0
	if state.ledgerDeducted < expectedDeduction-0.001 || state.ledgerDeducted > expectedDeduction+0.001 {
		t.Errorf("expected total deducted %.2f, got %.4f", expectedDeduction, state.ledgerDeducted)
	}

	// Verify review_events received tool_dispatch events
	if len(state.dispatchEvents) != 2 {
		t.Errorf("expected 2 dispatch events, got %d", len(state.dispatchEvents))
	}

	// Verify reviews table metadata was updated with tools_dispatched count
	if state.metaDispatchedCount != 2 {
		t.Errorf("expected metaDispatchedCount 2, got %d", state.metaDispatchedCount)
	}
}

// D3: Privilege escalation check: .lrc/policy/tools.toml with enabled = true on a tool disabled by the org is NOT enabled.
func TestDispatchTools_D3_PrivilegeEscalationPrevented(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)

	// Org ONLY has gitleaks enabled. bandit and ruff are NOT enabled by the org.
	state.orgSettingsJSON = []byte(`{"gitleaks": {"enabled": true}}`)

	jq := createTestJobQueue(t, mockDB)

	// Repo tries to maliciously force-enable bandit and ruff via .lrc/policy/tools.toml
	policyTOML := []byte(`
[bandit]
enabled = true

[ruff]
enabled = true

[gitleaks]
enabled = true
`)

	params := ToolDispatchParams{
		OrgID:    1,
		ReviewID: 300,
		PlanCode: string(license.PlanTeam),
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "app.py"},
		},
		LRCBundle: lrcconfig.Bundle{
			Files: map[string][]byte{
				"policy/tools.toml": policyTOML,
			},
		},
	}

	count, err := jq.DispatchTools(ctx, params)
	if err != nil {
		t.Fatalf("DispatchTools failed: %v", err)
	}

	// Only gitleaks should run. bandit and ruff must be excluded despite repo config.
	if count != 1 {
		t.Fatalf("expected exactly 1 tool dispatched (gitleaks), got %d", count)
	}

	if len(state.ledgerBreakdown) != 1 {
		t.Fatalf("expected 1 tool in breakdown, got %d: %+v", len(state.ledgerBreakdown), state.ledgerBreakdown)
	}

	if strings.ToLower(state.ledgerBreakdown[0].ToolName) != "gitleaks" {
		t.Errorf("expected gitleaks in breakdown, got %s", state.ledgerBreakdown[0].ToolName)
	}
}

// D4: Repo config enabled = false disables an org-enabled tool and excludes its multiplier from the breakdown.
func TestDispatchTools_D4_RepoConfigDisableExcludesFromBreakdown(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)

	// Org has ruff (multiplier 1.0) and gitleaks (multiplier 12.0) enabled
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}, "gitleaks": {"enabled": true}}`)

	jq := createTestJobQueue(t, mockDB)

	// Repo config explicitly disables ruff
	policyTOML := []byte(`
[ruff]
enabled = false
`)

	params := ToolDispatchParams{
		OrgID:    1,
		ReviewID: 400,
		PlanCode: string(license.PlanTeam),
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "app.py"},
		},
		LRCBundle: lrcconfig.Bundle{
			Files: map[string][]byte{
				"policy/tools.toml": policyTOML,
			},
		},
	}

	count, err := jq.DispatchTools(ctx, params)
	if err != nil {
		t.Fatalf("DispatchTools failed: %v", err)
	}

	// Only gitleaks should be dispatched
	if count != 1 {
		t.Fatalf("expected 1 tool dispatched, got %d", count)
	}

	// Breakdown must contain ONLY gitleaks, with multiplier 0.20
	if len(state.ledgerBreakdown) != 1 {
		t.Fatalf("expected exactly 1 breakdown entry, got %d: %+v", len(state.ledgerBreakdown), state.ledgerBreakdown)
	}

	chargedTool := state.ledgerBreakdown[0]
	if strings.ToLower(chargedTool.ToolName) != "gitleaks" {
		t.Errorf("expected gitleaks charged, got %s", chargedTool.ToolName)
	}

	expectedGitleaksMultiplier := 12.0
	if state.ledgerDeducted < expectedGitleaksMultiplier-0.001 || state.ledgerDeducted > expectedGitleaksMultiplier+0.001 {
		t.Errorf("expected ledger deduction %.2f, got %.4f (ruff multiplier must be excluded)", expectedGitleaksMultiplier, state.ledgerDeducted)
	}
}

// D5: Zero matched tools returns 0 and does NOT call DeductCredits.
func TestDispatchTools_D5_ZeroMatchedToolsNoDeductCredits(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()

	// Subtest 1: Org has no tools enabled at all
	t.Run("OrgHasNoToolsEnabled", func(t *testing.T) {
		mockDB, state := setupMockDB(t)
		state.orgSettingsJSON = []byte(`{}`)

		jq := createTestJobQueue(t, mockDB)

		count, err := jq.DispatchTools(ctx, ToolDispatchParams{
			OrgID:    1,
			ReviewID: 501,
			LocalDiffs: []lib.LocalCodeDiff{
				{NewPath: "main.py"},
			},
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 tools dispatched, got %d", count)
		}
		if len(state.ledgerBreakdown) != 0 || state.ledgerDeducted != 0 {
			t.Fatalf("DeductCredits must NOT be called when no tools matched")
		}
	})

	// Subtest 2: Org has tools enabled, but diff files match no trigger rules
	t.Run("NoDiffFilesMatchedRules", func(t *testing.T) {
		mockDB, state := setupMockDB(t)
		state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}}`)

		jq := createTestJobQueue(t, mockDB)

		// Repo config configures ruff to only run on python files
		policyTOML := []byte(`
[ruff]
include = ["*.py"]
`)

		// Diffs only contain Markdown files
		count, err := jq.DispatchTools(ctx, ToolDispatchParams{
			OrgID:    1,
			ReviewID: 502,
			LocalDiffs: []lib.LocalCodeDiff{
				{NewPath: "README.md"},
			},
			LRCBundle: lrcconfig.Bundle{
				Files: map[string][]byte{
					"policy/tools.toml": policyTOML,
				},
			},
		})
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Fatalf("expected 0 tools dispatched, got %d", count)
		}
		if len(state.ledgerBreakdown) != 0 || state.ledgerDeducted != 0 {
			t.Fatalf("DeductCredits must NOT be called when no tools matched trigger rules")
		}
	})
}

// D6: Failure in DeductCredits returns error and aborts tool execution.
func TestDispatchTools_D6_DeductCreditsFailureAbortsExecution(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)

	// Org has ruff enabled
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}}`)
	// Simulate DeductCredits failure (e.g. credit limit exceeded or database failure)
	state.failDeductCredits = true

	sink := &testEventSink{}
	jq := createTestJobQueue(t, mockDB)

	params := ToolDispatchParams{
		OrgID:    1,
		ReviewID: 600,
		PlanCode: string(license.PlanTeam),
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "app.py"},
		},
		EventSink: sink,
	}

	count, err := jq.DispatchTools(ctx, params)
	if err == nil {
		t.Fatal("expected error from DispatchTools when DeductCredits fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to deduct tool credits") {
		t.Errorf("expected error to mention failed to deduct tool credits, got: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tools dispatched on deduction failure, got %d", count)
	}

	// Tool execution must be completely aborted: no dispatch events inserted
	if len(state.dispatchEvents) != 0 {
		t.Errorf("expected 0 dispatch events on credit deduction failure, got %d", len(state.dispatchEvents))
	}

	// Review metadata update for tools_dispatched must NOT occur
	if state.metaDispatchedCount != 0 {
		t.Errorf("expected metaDispatchedCount 0, got %d", state.metaDispatchedCount)
	}

	// Warning event should be emitted to EventSink
	if len(sink.events) == 0 {
		t.Errorf("expected EventSink to receive warning event")
	} else if sink.events[0].Level != "warn" {
		t.Errorf("expected warn level event, got %s", sink.events[0].Level)
	}
}

// D7: Failure in QueueToolReviewJob logs error and continues with remaining tools.
func TestDispatchTools_D7_QueueToolReviewJobFailure(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)

	// Org has ruff enabled
	state.orgSettingsJSON = []byte(`{"ruff": {"enabled": true}}`)

	jq := createTestJobQueue(t, mockDB)

	// Close the job queue connection pool so QueueToolReviewJob fails on insert
	if jq.pool != nil {
		jq.pool.Close()
	}

	params := ToolDispatchParams{
		OrgID:    1,
		ReviewID: 700,
		PlanCode: string(license.PlanTeam),
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "app.py"},
		},
	}

	count, err := jq.DispatchTools(ctx, params)
	// DispatchTools logs the queue error and continues gracefully
	if err != nil {
		t.Fatalf("expected nil error from DispatchTools when queueing fails, got %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 tools successfully dispatched since queue failed, got %d", count)
	}

	// But credits were still deducted (documented behavior: deduction succeeds before fanout)
	if len(state.ledgerBreakdown) != 1 {
		t.Fatalf("expected credit deduction to have occurred before queueing, got breakdown: %+v", state.ledgerBreakdown)
	}
}

// D8: Mixed tools: 3 enabled, 1 repo-disabled, 1 repo-unknown -> Breakdown has exactly 3 entries.
func TestDispatchTools_D8_MixedToolsBreakdownCount(t *testing.T) {
	t.Setenv("LIVEREVIEW_IS_CLOUD", "true")
	ctx := context.Background()
	mockDB, state := setupMockDB(t)

	// Org has 4 tools enabled: ruff, gitleaks, semgrep, bandit
	state.orgSettingsJSON = []byte(`{
		"ruff": {"enabled": true},
		"gitleaks": {"enabled": true},
		"semgrep": {"enabled": true},
		"bandit": {"enabled": true}
	}`)

	jq := createTestJobQueue(t, mockDB)

	// Repo config:
	// - disables bandit (repo-disabled)
	// - attempts to enable unknown_tool (repo-unknown, not in org)
	policyTOML := []byte(`
[bandit]
enabled = false

[unknown_tool]
enabled = true
`)

	params := ToolDispatchParams{
		OrgID:    1,
		ReviewID: 800,
		PlanCode: string(license.PlanTeam),
		LocalDiffs: []lib.LocalCodeDiff{
			{NewPath: "app.py"},
		},
		LRCBundle: lrcconfig.Bundle{
			Files: map[string][]byte{
				"policy/tools.toml": policyTOML,
			},
		},
	}

	count, err := jq.DispatchTools(ctx, params)
	if err != nil {
		t.Fatalf("DispatchTools failed: %v", err)
	}

	// Exactly 3 tools dispatched: ruff, gitleaks, semgrep
	if count != 3 {
		t.Fatalf("expected exactly 3 tools dispatched, got %d", count)
	}

	// Breakdown must have exactly 3 entries
	if len(state.ledgerBreakdown) != 3 {
		t.Fatalf("expected exactly 3 entries in ledger breakdown, got %d: %+v", len(state.ledgerBreakdown), state.ledgerBreakdown)
	}

	toolNames := make(map[string]bool)
	for _, tc := range state.ledgerBreakdown {
		toolNames[strings.ToLower(tc.ToolName)] = true
	}

	if !toolNames["ruff"] || !toolNames["gitleaks"] || !toolNames["semgrep"] {
		t.Errorf("expected ruff, gitleaks, semgrep in breakdown, got %+v", toolNames)
	}

	if toolNames["bandit"] {
		t.Errorf("bandit should have been disabled by repo config")
	}

	if toolNames["unknown_tool"] {
		t.Errorf("unknown_tool should not have been added without org enablement")
	}
}
