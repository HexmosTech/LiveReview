package toolclassifier

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/livereview/internal/license"
	"github.com/livereview/pkg/models"
)

type mockDBDriver struct {
	mu           sync.Mutex
	queryHandler func(query string, args []driver.NamedValue) (driver.Rows, error)
	execHandler  func(query string, args []driver.NamedValue) (driver.Result, error)
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
func (c *mockDBConn) Begin() (driver.Tx, error) { return &mockDBTx{}, nil }

func (c *mockDBConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.driver.mu.Lock()
	handler := c.driver.queryHandler
	c.driver.mu.Unlock()
	if handler != nil {
		return handler(query, args)
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (c *mockDBConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.driver.mu.Lock()
	handler := c.driver.execHandler
	c.driver.mu.Unlock()
	if handler != nil {
		return handler(query, args)
	}
	return nil, fmt.Errorf("unexpected exec: %s", query)
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

var (
	testMockDriver mockDBDriver
	testDB         *sql.DB
)

func init() {
	sql.Register("toolclassifier-mock", &testMockDriver)
	db, err := sql.Open("toolclassifier-mock", "")
	if err != nil {
		panic(err)
	}
	testDB = db
}

func resetMockDriver() {
	testMockDriver.mu.Lock()
	defer testMockDriver.mu.Unlock()
	testMockDriver.queryHandler = nil
	testMockDriver.execHandler = nil
}

// Test Tier 1 Deterministic Rules
func TestTier1DeterministicRules(t *testing.T) {
	findings := []RawToolFinding{
		{
			File:    "secret.py",
			Line:    10,
			Rule:    "generic-api-key",
			Message: "Potential hardcoded API key",
		},
		{
			File:    "app.py",
			Line:    25,
			Rule:    "f401",
			Message: "'os' imported but unused",
		},
	}

	comments, err := ClassifyToolResult(context.Background(), nil, 1, license.PlanFree, "gitleaks", findings[:1], nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 1 || comments[0] == nil {
		t.Fatalf("expected 1 comment, got: %v", comments)
	}
	if comments[0].Category != "Security" || comments[0].Subcategory != "Secrets Management" {
		t.Errorf("expected Security/Secrets Management, got %s/%s", comments[0].Category, comments[0].Subcategory)
	}
	if comments[0].Severity != models.SeverityCritical {
		t.Errorf("expected critical severity, got %v", comments[0].Severity)
	}

	commentsRuff, err := ClassifyToolResult(context.Background(), nil, 1, license.PlanFree, "ruff", findings[1:], nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(commentsRuff) != 1 || commentsRuff[0] == nil {
		t.Fatalf("expected 1 comment, got: %v", commentsRuff)
	}
	if commentsRuff[0].Category != "Maintainability" || commentsRuff[0].Subcategory != "Dead Code" {
		t.Errorf("expected Maintainability/Dead Code, got %s/%s", commentsRuff[0].Category, commentsRuff[0].Subcategory)
	}
}

// Test Tier 2 Database Cache Hit
func TestTier2DatabaseCacheHit(t *testing.T) {
	resetMockDriver()
	defer resetMockDriver()

	queryExecuted := false
	testMockDriver.queryHandler = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		queryExecuted = true
		if !strings.Contains(query, "FROM tool_rule_taxonomies") {
			return nil, fmt.Errorf("unexpected query: %s", query)
		}
		return &mockRows{
			columns: []string{"rule_key", "category", "subcategory", "severity", "confidence", "type"},
			rows: [][]driver.Value{
				{"ruff:e711", "Maintainability", "Code Complexity", "warning", "High", "Code Smell"},
			},
		}, nil
	}

	findings := []RawToolFinding{
		{
			File:    "query.py",
			Line:    42,
			Rule:    "E711",
			Message: "Comparison to None should be 'if cond is None:'",
		},
	}

	comments, err := ClassifyToolResult(context.Background(), testDB, 1, license.PlanFree, "ruff", findings, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !queryExecuted {
		t.Fatal("expected database cache query to execute")
	}

	if len(comments) != 1 || comments[0] == nil {
		t.Fatalf("expected 1 comment, got: %v", comments)
	}
	c := comments[0]
	if c.Category != "Maintainability" || c.Subcategory != "Code Complexity" {
		t.Errorf("expected Maintainability/Code Complexity, got %s/%s", c.Category, c.Subcategory)
	}
	if c.Severity != models.SeverityWarning {
		t.Errorf("expected SeverityWarning, got %v", c.Severity)
	}
	if c.Confidence != "High" {
		t.Errorf("expected High confidence, got %s", c.Confidence)
	}
	if c.Type != "Code Smell" {
		t.Errorf("expected Code Smell type, got %s", c.Type)
	}
	if c.Line != 42 || c.FilePath != "query.py" {
		t.Errorf("unexpected line or file: %d, %s", c.Line, c.FilePath)
	}
}

// Test Tier 2 Database Cache Partial Hit and Fallback for Misses
func TestTier2DatabaseCachePartialHit(t *testing.T) {
	resetMockDriver()
	defer resetMockDriver()

	testMockDriver.queryHandler = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		// Only ruff:e711 is cached, ruff:unknown999 is missing
		return &mockRows{
			columns: []string{"rule_key", "category", "subcategory", "severity", "confidence", "type"},
			rows: [][]driver.Value{
				{"ruff:e711", "Maintainability", "Code Complexity", "warning", "High", "Code Smell"},
			},
		}, nil
	}

	findings := []RawToolFinding{
		{
			File:    "a.py",
			Line:    1,
			Rule:    "E711",
			Message: "comparison to None",
		},
		{
			File:    "b.py",
			Line:    10,
			Rule:    "unknown999",
			Message: "some unclassified tool finding",
		},
	}

	comments, err := ClassifyToolResult(context.Background(), testDB, 1, license.PlanFree, "ruff", findings, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}

	// First finding should be resolved from DB cache
	c1 := comments[0]
	if c1.Category != "Maintainability" || c1.Subcategory != "Code Complexity" {
		t.Errorf("expected c1 Maintainability/Code Complexity, got %s/%s", c1.Category, c1.Subcategory)
	}

	// Second finding should have received fallback since AI connector is not configured in unit test
	c2 := comments[1]
	if c2 == nil {
		t.Fatal("expected c2 to have fallback comment")
	}
	if c2.Category != "Security" || c2.Subcategory != "Secrets Management" {
		t.Errorf("expected fallback Security/Secrets Management, got %s/%s", c2.Category, c2.Subcategory)
	}
}

// Test Tier 2 Database Cache Error Fallthrough (e.g. table not found or connection error)
func TestTier2DatabaseCacheQueryErrorFallthrough(t *testing.T) {
	resetMockDriver()
	defer resetMockDriver()

	testMockDriver.queryHandler = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		return nil, errors.New(`pq: relation "tool_rule_taxonomies" does not exist`)
	}

	findings := []RawToolFinding{
		{
			File:    "test.py",
			Line:    5,
			Rule:    "custom-rule-xyz",
			Message: "custom violation",
		},
	}

	// Should not fail! Must fall through gracefully and apply fallback comments.
	comments, err := ClassifyToolResult(context.Background(), testDB, 1, license.PlanFree, "customtool", findings, nil)
	if err != nil {
		t.Fatalf("expected graceful fallthrough without error, got: %v", err)
	}
	if len(comments) != 1 || comments[0] == nil {
		t.Fatalf("expected 1 fallback comment, got: %v", comments)
	}
	if comments[0].Category != "Security" {
		t.Errorf("expected fallback Security category, got: %s", comments[0].Category)
	}
}

// Test ClassifyToolResult with nil db
func TestClassifyToolResultNilDB(t *testing.T) {
	findings := []RawToolFinding{
		{
			File:    "foo.py",
			Line:    12,
			Rule:    "unregistered-rule",
			Message: "something",
		},
	}

	comments, err := ClassifyToolResult(context.Background(), nil, 1, license.PlanFree, "mytool", findings, nil)
	if err != nil {
		t.Fatalf("unexpected error with nil db: %v", err)
	}
	if len(comments) != 1 || comments[0] == nil {
		t.Fatalf("expected fallback comment with nil db, got: %v", comments)
	}
}

// Test ClassifyToolResult with empty findings
func TestClassifyToolResultEmpty(t *testing.T) {
	comments, err := ClassifyToolResult(context.Background(), testDB, 1, license.PlanFree, "mytool", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error on empty findings: %v", err)
	}
	if comments != nil {
		t.Fatalf("expected nil comments on empty findings, got: %v", comments)
	}
}

// Test Step 4: UpsertTaxonomies batch query and parameters
func TestUpsertTaxonomies(t *testing.T) {
	resetMockDriver()
	defer resetMockDriver()

	var executedQuery string
	var executedArgs []driver.NamedValue

	testMockDriver.execHandler = func(query string, args []driver.NamedValue) (driver.Result, error) {
		executedQuery = query
		executedArgs = args
		return mockResult{rowsAffected: int64(len(args) / 8)}, nil
	}

	entries := []TaxonomyCacheEntry{
		{
			RuleKey:     "ruff:e711",
			ToolName:    "ruff",
			RuleID:      "E711",
			Category:    "Maintainability",
			Subcategory: "Code Complexity",
			Severity:    "warning",
			Confidence:  "High",
			Type:        "Code Smell",
		},
		{
			RuleKey:     "hadolint:dl3008",
			ToolName:    "hadolint",
			RuleID:      "DL3008",
			Category:    "Maintainability",
			Subcategory: "Configuration Management",
			Severity:    "warning",
			Confidence:  "High",
			Type:        "Best Practice",
		},
	}

	err := UpsertTaxonomies(context.Background(), testDB, entries)
	if err != nil {
		t.Fatalf("UpsertTaxonomies failed: %v", err)
	}

	if !strings.Contains(executedQuery, "INSERT INTO tool_rule_taxonomies") {
		t.Errorf("expected INSERT INTO tool_rule_taxonomies, got: %s", executedQuery)
	}
	if !strings.Contains(executedQuery, "ON CONFLICT (rule_key) DO UPDATE SET") {
		t.Errorf("expected ON CONFLICT clause, got: %s", executedQuery)
	}
	if !strings.Contains(executedQuery, "($1, $2, $3, $4, $5, $6, $7, $8, NOW()), ($9, $10, $11, $12, $13, $14, $15, $16, NOW())") {
		t.Errorf("expected 2 parameter groups, got: %s", executedQuery)
	}

	if len(executedArgs) != 16 {
		t.Fatalf("expected 16 args, got %d", len(executedArgs))
	}
	if executedArgs[0].Value != "ruff:e711" {
		t.Errorf("expected first arg ruff:e711, got %v", executedArgs[0].Value)
	}
	if executedArgs[8].Value != "hadolint:dl3008" {
		t.Errorf("expected 9th arg hadolint:dl3008, got %v", executedArgs[8].Value)
	}
}

// Test UpsertTaxonomies edge cases (nil db, empty entries)
func TestUpsertTaxonomiesEdgeCases(t *testing.T) {
	if err := UpsertTaxonomies(context.Background(), nil, []TaxonomyCacheEntry{{RuleKey: "k"}}); err != nil {
		t.Errorf("expected nil error on nil db, got %v", err)
	}
	if err := UpsertTaxonomies(context.Background(), testDB, nil); err != nil {
		t.Errorf("expected nil error on empty entries, got %v", err)
	}
}

// Test autoCacheTaxonomiesAsync
func TestAutoCacheTaxonomiesAsync(t *testing.T) {
	resetMockDriver()
	defer resetMockDriver()

	var execDone = make(chan struct{})
	testMockDriver.execHandler = func(query string, args []driver.NamedValue) (driver.Result, error) {
		return mockResult{rowsAffected: 1}, nil
	}

	onTaxonomyCacheUpsertDone = func(err error) {
		if err != nil {
			t.Errorf("unexpected upsert error: %v", err)
		}
		close(execDone)
	}
	defer func() { onTaxonomyCacheUpsertDone = nil }()

	entries := []TaxonomyCacheEntry{
		{
			RuleKey:     "shellcheck:sc2086",
			ToolName:    "shellcheck",
			RuleID:      "SC2086",
			Category:    "Reliability",
			Subcategory: "Fault Tolerance",
			Severity:    "info",
			Confidence:  "Medium",
			Type:        "Code Smell",
		},
	}

	autoCacheTaxonomiesAsync(testDB, "shellcheck", entries)

	select {
	case <-execDone:
		// Succeeded
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async auto-cache upsert")
	}
}

// Test Helpers: MakeRuleKey, CleanRuleID, NormalizeType, NormalizeConfidence, parseSeverity
func TestHelperFunctions(t *testing.T) {
	// MakeRuleKey
	if k := MakeRuleKey("ruff", "E711"); k != "ruff:e711" {
		t.Errorf("expected ruff:e711, got %s", k)
	}
	if k := MakeRuleKey("ruff", "ruff:E711"); k != "ruff:e711" {
		t.Errorf("expected ruff:e711, got %s", k)
	}
	if k := MakeRuleKey(" Bandit ", " B303 "); k != "bandit:b303" {
		t.Errorf("expected bandit:b303, got %s", k)
	}

	// CleanRuleID
	if id := CleanRuleID("ruff", "ruff:E711"); id != "E711" {
		t.Errorf("expected E711, got %s", id)
	}
	if id := CleanRuleID("ruff", "E711"); id != "E711" {
		t.Errorf("expected E711, got %s", id)
	}

	// NormalizeType
	if typ := NormalizeType("b"); typ != "Bug" {
		t.Errorf("expected Bug for 'b', got %s", typ)
	}
	if typ := NormalizeType("s"); typ != "Code Smell" {
		t.Errorf("expected Code Smell for 's', got %s", typ)
	}
	if typ := NormalizeType("best practice"); typ != "Best Practice" {
		t.Errorf("expected Best Practice, got %s", typ)
	}
	if typ := NormalizeType("invalid_type"); typ != "Risk" {
		t.Errorf("expected fallback Risk, got %s", typ)
	}

	// NormalizeConfidence
	if conf := NormalizeConfidence("h"); conf != "High" {
		t.Errorf("expected High for 'h', got %s", conf)
	}
	if conf := NormalizeConfidence("l"); conf != "Low" {
		t.Errorf("expected Low for 'l', got %s", conf)
	}
	if conf := NormalizeConfidence("medium"); conf != "Medium" {
		t.Errorf("expected Medium, got %s", conf)
	}
	if conf := NormalizeConfidence("unknown"); conf != "High" {
		t.Errorf("expected fallback High, got %s", conf)
	}

	// parseSeverity
	if sev := parseSeverity("c"); sev != models.SeverityCritical {
		t.Errorf("expected critical for 'c', got %v", sev)
	}
	if sev := parseSeverity("critical"); sev != models.SeverityCritical {
		t.Errorf("expected critical, got %v", sev)
	}
	if sev := parseSeverity("i"); sev != models.SeverityInfo {
		t.Errorf("expected info for 'i', got %v", sev)
	}
	if sev := parseSeverity("info"); sev != models.SeverityInfo {
		t.Errorf("expected info, got %v", sev)
	}
	if sev := parseSeverity("w"); sev != models.SeverityWarning {
		t.Errorf("expected warning for 'w', got %v", sev)
	}
}

// TestRuleDeduplicationLogic verifies that multiple findings with the same rule key
// are appropriately grouped and all get populated.
func TestRuleDeduplicationLogic(t *testing.T) {
	findings := []RawToolFinding{
		{File: "a.py", Line: 1, Rule: "b105", Message: "hardcoded pass 1"},
		{File: "b.py", Line: 10, Rule: "b105", Message: "hardcoded pass 2"},
		{File: "c.py", Line: 20, Rule: "b105", Message: "hardcoded pass 3"},
		{File: "d.py", Line: 30, Rule: "other_rule", Message: "other message"},
	}

	// When db is nil, all findings fall through to applyFallbackComments
	comments, err := ClassifyToolResult(context.Background(), nil, 1, license.PlanFree, "bandit", findings, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 4 {
		t.Fatalf("expected 4 comments, got %d", len(comments))
	}
	for i, c := range comments {
		if c == nil {
			t.Errorf("expected non-nil comment at index %d", i)
		} else if c.Content != findings[i].Message {
			t.Errorf("comment content mismatch at %d: got %s, want %s", i, c.Content, findings[i].Message)
		}
	}
}
