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
		if !strings.Contains(query, "classifier_version = 1") {
			return nil, fmt.Errorf("expected classifier_version = 1 in query: %s", query)
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
		if !strings.Contains(query, "classifier_version = 1") {
			return nil, fmt.Errorf("expected classifier_version = 1 in query: %s", query)
		}
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

	// Second finding should have received safe fallback since AI connector is not configured in unit test
	c2 := comments[1]
	if c2 == nil {
		t.Fatal("expected c2 to have fallback comment")
	}
	if c2.Category != "Maintainability" || c2.Subcategory != "Technical Debt" {
		t.Errorf("expected safe fallback Maintainability/Technical Debt, got %s/%s", c2.Category, c2.Subcategory)
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
	if comments[0].Category != "Maintainability" {
		t.Errorf("expected safe fallback Maintainability category, got: %s", comments[0].Category)
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
	if !strings.Contains(executedQuery, "classifier_version") {
		t.Errorf("expected classifier_version in query column list, got: %s", executedQuery)
	}
	if !strings.Contains(executedQuery, "ON CONFLICT (rule_key) DO UPDATE SET") {
		t.Errorf("expected ON CONFLICT clause, got: %s", executedQuery)
	}
	if !strings.Contains(executedQuery, "($1, $2, $3, $4, $5, $6, $7, $8, 1, NOW()), ($9, $10, $11, $12, $13, $14, $15, $16, 1, NOW())") {
		t.Errorf("expected 2 parameter groups with classifier_version 1, got: %s", executedQuery)
	}

	if len(executedArgs) != 16 {
		t.Fatalf("expected 16 args, got %d", len(executedArgs))
	}
	if executedArgs[0].Value != "ruff:e711" {
		t.Errorf("expected first arg ruff:e711, got %v", executedArgs[0].Value)
	}
	if executedArgs[1].Value != "ruff" {
		t.Errorf("expected second arg lowercased tool_name 'ruff', got %v", executedArgs[1].Value)
	}
	if executedArgs[2].Value != "e711" {
		t.Errorf("expected third arg lowercased rule_id 'e711', got %v", executedArgs[2].Value)
	}
	if executedArgs[8].Value != "hadolint:dl3008" {
		t.Errorf("expected 9th arg hadolint:dl3008, got %v", executedArgs[8].Value)
	}
	if executedArgs[9].Value != "hadolint" {
		t.Errorf("expected 10th arg lowercased tool_name 'hadolint', got %v", executedArgs[9].Value)
	}
	if executedArgs[10].Value != "dl3008" {
		t.Errorf("expected 11th arg lowercased rule_id 'dl3008', got %v", executedArgs[10].Value)
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

// T1: Case normalization: verify MakeRuleKey("Ruff", "E501") and MakeRuleKey("ruff", "e501") produce "ruff:e501".
func TestT1_CaseNormalization(t *testing.T) {
	k1 := MakeRuleKey("Ruff", "E501")
	k2 := MakeRuleKey("ruff", "e501")

	if k1 != "ruff:e501" {
		t.Errorf("T1: expected MakeRuleKey(\"Ruff\", \"E501\") = \"ruff:e501\", got %q", k1)
	}
	if k2 != "ruff:e501" {
		t.Errorf("T1: expected MakeRuleKey(\"ruff\", \"e501\") = \"ruff:e501\", got %q", k2)
	}
	if k1 != k2 {
		t.Errorf("T1: expected k1 == k2, got %q != %q", k1, k2)
	}

	// Additional case normalization edge cases
	variants := []struct {
		tool, rule string
	}{
		{"RUFF", "E501"},
		{"Ruff", "ruff:E501"},
		{"ruff", "RUFF:e501"},
		{"  Ruff  ", "  E501  "},
		{"RUFF", "  ruff:e501  "},
	}
	for _, v := range variants {
		got := MakeRuleKey(v.tool, v.rule)
		if got != "ruff:e501" {
			t.Errorf("T1: MakeRuleKey(%q, %q) = %q; want \"ruff:e501\"", v.tool, v.rule, got)
		}
	}
}

// T2: UpsertTaxonomies inserts lowercased tool_name and rule_id, and classifier_version = 1.
func TestT2_UpsertTaxonomiesInsertsLowercasedAndVersion(t *testing.T) {
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
			RuleKey:     "ruff:e501",
			ToolName:    "RUFF",
			RuleID:      "E501",
			Category:    "Maintainability",
			Subcategory: "Code Complexity",
			Severity:    "info",
			Confidence:  "High",
			Type:        "Code Smell",
		},
		{
			RuleKey:     "hadolint:dl3006",
			ToolName:    "  HadoLint  ",
			RuleID:      "  DL3006  ",
			Category:    "Maintainability",
			Subcategory: "Configuration Management",
			Severity:    "warning",
			Confidence:  "Medium",
			Type:        "Best Practice",
		},
	}

	err := UpsertTaxonomies(context.Background(), testDB, entries)
	if err != nil {
		t.Fatalf("T2: UpsertTaxonomies failed: %v", err)
	}

	// Verify query includes classifier_version column in INSERT statement
	if !strings.Contains(executedQuery, "classifier_version") {
		t.Errorf("T2: expected classifier_version in INSERT query columns, got: %s", executedQuery)
	}

	// Verify values template includes literal '1' for classifier_version
	if !strings.Contains(executedQuery, "1, NOW()") {
		t.Errorf("T2: expected classifier_version literal 1 in query VALUES, got: %s", executedQuery)
	}

	// Verify tool_name and rule_id are lowercased and trimmed in args
	if len(executedArgs) != 16 {
		t.Fatalf("T2: expected 16 args, got %d", len(executedArgs))
	}

	// Entry 1
	if executedArgs[1].Value != "ruff" {
		t.Errorf("T2: expected lowercased tool_name 'ruff', got %v", executedArgs[1].Value)
	}
	if executedArgs[2].Value != "e501" {
		t.Errorf("T2: expected lowercased rule_id 'e501', got %v", executedArgs[2].Value)
	}

	// Entry 2
	if executedArgs[9].Value != "hadolint" {
		t.Errorf("T2: expected lowercased & trimmed tool_name 'hadolint', got %v", executedArgs[9].Value)
	}
	if executedArgs[10].Value != "dl3006" {
		t.Errorf("T2: expected lowercased & trimmed rule_id 'dl3006', got %v", executedArgs[10].Value)
	}
}

// T3: queryTaxonomyCache includes classifier_version = 1 in SQL query.
func TestT3_QueryTaxonomyCacheIncludesClassifierVersion(t *testing.T) {
	resetMockDriver()
	defer resetMockDriver()

	var executedQuery string
	testMockDriver.queryHandler = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		executedQuery = query
		return &mockRows{
			columns: []string{"rule_key", "category", "subcategory", "severity", "confidence", "type"},
			rows: [][]driver.Value{
				{"ruff:e501", "Maintainability", "Code Complexity", "info", "High", "Code Smell"},
			},
		}, nil
	}

	tuples, err := queryTaxonomyCache(context.Background(), testDB, []string{"ruff:e501"})
	if err != nil {
		t.Fatalf("T3: queryTaxonomyCache failed: %v", err)
	}

	if !strings.Contains(executedQuery, "classifier_version = 1") {
		t.Errorf("T3: expected query to contain 'classifier_version = 1', got: %s", executedQuery)
	}

	expected := "SELECT rule_key, category, subcategory, severity, confidence, type FROM tool_rule_taxonomies WHERE rule_key = ANY($1) AND classifier_version = 1"
	if executedQuery != expected {
		t.Errorf("T3: query mismatch:\ngot:  %s\nwant: %s", executedQuery, expected)
	}

	if len(tuples) != 1 {
		t.Fatalf("T3: expected 1 tuple returned, got %d", len(tuples))
	}
	if tuple, ok := tuples["ruff:e501"]; !ok || tuple.Category != "Maintainability" {
		t.Errorf("T3: unexpected tuple result: %+v", tuple)
	}
}

// T4: Fallback classification applies when LLM fails or is empty.
func TestT4_FallbackClassificationWhenLLMFailsOrEmpty(t *testing.T) {
	// Subcase 4A: applyFallbackComments direct behavior
	findings := []RawToolFinding{
		{
			File:    "services/auth.py",
			Line:    88,
			Rule:    "custom-audit-check",
			Message: "Suspicious token reflection",
		},
	}
	emptyComments := make([]*models.ReviewComment, 1)
	resolved := applyFallbackComments("customtool", findings, emptyComments, []int{0})

	if len(resolved) != 1 || resolved[0] == nil {
		t.Fatalf("T4: expected fallback comment to be created")
	}
	c := resolved[0]
	if c.Category != "Maintainability" || c.Subcategory != "Technical Debt" {
		t.Errorf("T4: expected Category=Maintainability/Subcategory=Technical Debt, got %s/%s", c.Category, c.Subcategory)
	}
	if c.Severity != models.SeverityInfo {
		t.Errorf("T4: expected Severity=SeverityInfo, got %v", c.Severity)
	}
	if c.Confidence != "Low" {
		t.Errorf("T4: expected Confidence=Low, got %s", c.Confidence)
	}
	if c.Type != "Technical Debt" {
		t.Errorf("T4: expected Type=Technical Debt, got %s", c.Type)
	}
	if c.ToolName != "customtool" {
		t.Errorf("T4: expected ToolName=customtool, got %s", c.ToolName)
	}
	if c.FilePath != "services/auth.py" || c.Line != 88 || c.Content != "Suspicious token reflection" {
		t.Errorf("T4: finding file/line/content mismatch: %+v", c)
	}

	// Subcase 4B: parsePipeDelimitedResponse returns empty slice on empty or invalid LLM response
	if lines := parsePipeDelimitedResponse(""); len(lines) != 0 {
		t.Errorf("T4: expected empty slice for empty string, got: %v", lines)
	}
	if lines := parsePipeDelimitedResponse("[]"); len(lines) != 0 {
		t.Errorf("T4: expected empty slice for empty JSON array, got: %v", lines)
	}
	if lines := parsePipeDelimitedResponse("invalid json"); len(lines) != 0 {
		t.Errorf("T4: expected empty slice for invalid JSON, got: %v", lines)
	}

	// Subcase 4C: ClassifyToolResult when DB cache misses and LLM config is missing/fails
	resetMockDriver()
	defer resetMockDriver()

	testMockDriver.queryHandler = func(query string, args []driver.NamedValue) (driver.Rows, error) {
		// Cache miss
		return &mockRows{
			columns: []string{"rule_key", "category", "subcategory", "severity", "confidence", "type"},
			rows:    [][]driver.Value{},
		}, nil
	}

	comments, err := ClassifyToolResult(context.Background(), testDB, 1, license.PlanFree, "customtool", findings, nil)
	if err != nil {
		t.Fatalf("T4: expected nil error with fallback, got: %v", err)
	}
	if len(comments) != 1 || comments[0] == nil {
		t.Fatalf("T4: expected 1 fallback comment, got: %v", comments)
	}
	if comments[0].Category != "Maintainability" || comments[0].Subcategory != "Technical Debt" {
		t.Errorf("T4: expected safe fallback Maintainability/Technical Debt, got %s/%s", comments[0].Category, comments[0].Subcategory)
	}
	if comments[0].Severity != models.SeverityInfo {
		t.Errorf("T4: expected SeverityInfo, got %v", comments[0].Severity)
	}
	if comments[0].Content != "Suspicious token reflection" {
		t.Errorf("T4: expected content to match finding, got %s", comments[0].Content)
	}
}

// T5: Case normalization and deduplication across different rule representations
func TestT5_RuleCaseDeduplicationAndStorage(t *testing.T) {
	// Mixed casing in findings
	findings := []RawToolFinding{
		{File: "a.py", Line: 10, Rule: "E501", Message: "line too long 1"},
		{File: "b.py", Line: 20, Rule: "e501", Message: "line too long 2"},
		{File: "c.py", Line: 30, Rule: "ruff:E501", Message: "line too long 3"},
	}

	// Verify all map to same rule key
	for _, f := range findings {
		key := MakeRuleKey("RUFF", f.Rule)
		if key != "ruff:e501" {
			t.Errorf("T5: expected key ruff:e501, got %s for rule %s", key, f.Rule)
		}
	}

	// Verify CleanRuleID handles uppercase prefix
	if id := CleanRuleID("RUFF", "ruff:E501"); id != "E501" {
		t.Errorf("T5: CleanRuleID failed, got %s want E501", id)
	}
	if id := CleanRuleID("ruff", "RUFF:E501"); id != "E501" {
		t.Errorf("T5: CleanRuleID failed, got %s want E501", id)
	}
}

func TestBuildClassifierPrompt_FormattingAndSanitization(t *testing.T) {
	findings := []RawToolFinding{
		{File: "app.go", Line: 12, Rule: "sql-inject", Message: "SQL injection | dangerous taint"},
		{File: "utils.go", Line: 50, Rule: "dead-code", Message: "Unused var\nnewline item"},
	}

	uniqueKeys := []string{"semgrep:sql-inject", "semgrep:dead-code"}
	ruleToIndices := map[string][]int{
		"semgrep:sql-inject": {0},
		"semgrep:dead-code":   {1},
	}

	prompt := BuildClassifierPrompt(uniqueKeys, ruleToIndices, findings)

	// Verify all 10 categories are present
	for cat := range ValidTaxonomyMap {
		if !strings.Contains(prompt, cat+":") {
			t.Errorf("expected prompt to contain category %q", cat)
		}
	}

	// Verify pipe in message was sanitized to /
	if strings.Contains(prompt, "SQL injection | dangerous taint") {
		t.Errorf("pipe in message should have been sanitized")
	}
	if !strings.Contains(prompt, "0|sql-inject|SQL injection / dangerous taint") {
		t.Errorf("expected sanitized pipe format for item 0, got:\n%s", prompt)
	}

	// Verify newline in message was replaced by space
	if !strings.Contains(prompt, "1|dead-code|Unused var newline item") {
		t.Errorf("expected sanitized newline format for item 1, got:\n%s", prompt)
	}
}

func TestDynamicTaxonomyAdditionUpdatesPromptAndValidation(t *testing.T) {
	// Temporarily add a new experimental taxonomy category thread-safely
	teardown := RegisterTaxonomyCategory("Experimental", []string{"TestSubcategory"})
	defer teardown()

	rendered := RenderTaxonomyList()
	if !strings.Contains(rendered, "- Experimental: TestSubcategory") {
		t.Errorf("expected RenderTaxonomyList to dynamically include Experimental category, got:\n%s", rendered)
	}

	prompt := BuildClassifierPrompt([]string{"tool:rule"}, map[string][]int{"tool:rule": {0}}, []RawToolFinding{{Rule: "rule", Message: "msg"}})
	if !strings.Contains(prompt, "- Experimental: TestSubcategory") {
		t.Errorf("expected BuildClassifierPrompt to dynamically include Experimental category, got:\n%s", prompt)
	}

	cat, sub := ValidateAndNormalizeTaxonomy("experimental", "testsubcategory")
	if cat != "Experimental" || sub != "TestSubcategory" {
		t.Errorf("expected ValidateAndNormalizeTaxonomy to recognize dynamic category, got %s / %s", cat, sub)
	}
}

func TestMalformedLLMPipeResponse_GracefulHandling(t *testing.T) {
	// 1. Partial/incomplete pipe tokens (fewer than 6 fields)
	malformedInput := `["0|Security|OnlyThreeFields", "invalid-no-pipes", "999|Security|Injection Vulnerabilities|c|H|B"]`
	lines := parsePipeDelimitedResponse(malformedInput)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines parsed, got %d", len(lines))
	}

	// 2. ValidateAndNormalizeTaxonomy with completely unrecognized inputs
	cat, subcat := ValidateAndNormalizeTaxonomy("NonExistentCategory", "RandomSubcategory")
	if cat != "Maintainability" || subcat != "Technical Debt" {
		t.Errorf("expected fallback Maintainability/Technical Debt for bogus category, got %s/%s", cat, subcat)
	}
}

func TestUnknownRuleSafeFallbackState(t *testing.T) {
	// Verify that applying fallback on unresolved findings never marks them as critical or security/secrets
	findings := []RawToolFinding{
		{File: "main.go", Line: 10, Rule: "unknown-linter-check", Message: "Something flagged"},
	}
	comments := make([]*models.ReviewComment, 1)
	resolved := applyFallbackComments("newlinter", findings, comments, []int{0})

	if len(resolved) != 1 || resolved[0] == nil {
		t.Fatalf("expected 1 comment, got: %v", resolved)
	}
	c := resolved[0]
	if c.Category == "Security" || c.Subcategory == "Secrets Management" {
		t.Errorf("unknown rule should NOT receive arbitrary Security/Secrets Management classification")
	}
	if c.Category != "Maintainability" || c.Subcategory != "Technical Debt" {
		t.Errorf("expected safe fallback Maintainability/Technical Debt, got %s/%s", c.Category, c.Subcategory)
	}
	if c.Severity == models.SeverityCritical {
		t.Errorf("unknown rule fallback should never default to critical")
	}
	if c.Confidence != "Low" {
		t.Errorf("unknown rule fallback should have Low confidence, got %s", c.Confidence)
	}
}
