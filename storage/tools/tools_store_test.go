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
	"testing"
)

// mockDBDriver provides an in-memory SQL mock for ToolsStore unit tests.
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

func (c *mockDBConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return &mockDBStmt{conn: c, query: query}, nil
}

func (c *mockDBConn) Close() error              { return nil }
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

func (s *mockDBStmt) Close() error  { return nil }
func (s *mockDBStmt) NumInput() int { return -1 }

func (s *mockDBStmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.ExecContext(context.Background(), s.query, named)
}

func (s *mockDBStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}

func (s *mockDBStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, v := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
	}
	return s.conn.QueryContext(context.Background(), s.query, named)
}

func (s *mockDBStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
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
	testMockDB     *sql.DB
	mockDriverOnce sync.Once
)

func getTestMockDB() *sql.DB {
	mockDriverOnce.Do(func() {
		sql.Register("tools-store-mock-driver", &testMockDriver)
		var err error
		testMockDB, err = sql.Open("tools-store-mock-driver", "")
		if err != nil {
			panic(err)
		}
	})
	return testMockDB
}

func setMockHandlers(queryHandler func(string, []driver.NamedValue) (driver.Rows, error), execHandler func(string, []driver.NamedValue) (driver.Result, error)) {
	testMockDriver.mu.Lock()
	defer testMockDriver.mu.Unlock()
	testMockDriver.queryHandler = queryHandler
	testMockDriver.execHandler = execHandler
}

func resetMockHandlers() {
	setMockHandlers(nil, nil)
}

func TestToolsStore(t *testing.T) {
	db := getTestMockDB()

	t.Run("LoadCatalog_Default15LaunchTools", func(t *testing.T) {
		ctx := context.Background()

		// 1. Verify with nil DB (pure fallback)
		storeNilDB := NewToolsStore(nil)
		catalog, err := storeNilDB.LoadCatalog(ctx)
		if err != nil {
			t.Fatalf("LoadCatalog(nil DB) returned error: %v", err)
		}

		if len(catalog) != 15 {
			t.Fatalf("expected exactly 15 launch tools by default, got %d", len(catalog))
		}

		expectedTools := []struct {
			id         int64
			name       string
			multiplier float64
			useCase    string
		}{
			{1, "ruff", 1.0, "Python lint/format"},
			{2, "openapi", 1.0, "OpenAPI/YAML validation"},
			{3, "osv", 1.0, "Dependencies & Supply Chain"},
			{4, "gitleaks", 12.0, "Secrets & Security"},
			{5, "trivy", 4.0, "Dependencies & Supply Chain"},
			{6, "trufflehog", 12.0, "Secrets & Security"},
			{7, "tfsec", 1.0, "Infrastructure & Config"},
			{8, "hadolint", 1.0, "Containers & Docker"},
			{9, "actionlint", 1.0, "CI/CD & Workflow"},
			{10, "golangci-lint", 12.0, "Go Code Quality"},
			{11, "spectral", 3.0, "API Schema"},
			{12, "bandit", 12.0, "Python SAST"},
			{13, "detect-secrets", 12.0, "Secrets & Security"},
			{14, "semgrep", 12.0, "Security & Code Quality"},
			{15, "eslint", 12.0, "JS/TS Code Quality"},
		}

		for i, expected := range expectedTools {
			tool := catalog[i]
			if tool.ID != expected.id {
				t.Errorf("tool[%d] ID: expected %d, got %d", i, expected.id, tool.ID)
			}
			if tool.Name != expected.name {
				t.Errorf("tool[%d] Name: expected %q, got %q", i, expected.name, tool.Name)
			}
			if tool.Multiplier != expected.multiplier {
				t.Errorf("tool[%d] Multiplier: expected %f, got %f", i, expected.multiplier, tool.Multiplier)
			}
			if tool.UseCase != expected.useCase {
				t.Errorf("tool[%d] UseCase: expected %q, got %q", i, expected.useCase, tool.UseCase)
			}
			if strings.TrimSpace(tool.Description) == "" {
				t.Errorf("tool[%d] (%s) has empty Description", i, tool.Name)
			}
		}

		// 2. Verify with DB returning sql.ErrNoRows from system_settings
		setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(query, "system_settings") {
				return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
			}
			return nil, fmt.Errorf("unexpected query: %s", query)
		}, nil)
		defer resetMockHandlers()

		storeWithDB := NewToolsStore(db)
		dbCatalog, err := storeWithDB.LoadCatalog(ctx)
		if err != nil {
			t.Fatalf("LoadCatalog(mockDB) returned error: %v", err)
		}
		if len(dbCatalog) != 15 {
			t.Fatalf("expected exactly 15 default launch tools on ErrNoRows fallback, got %d", len(dbCatalog))
		}
	})

	t.Run("ExcludedExperimentalTools_NotInDefaultCatalog", func(t *testing.T) {
		ctx := context.Background()
		store := NewToolsStore(nil)
		catalog, err := store.LoadCatalog(ctx)
		if err != nil {
			t.Fatalf("LoadCatalog failed: %v", err)
		}

		// The 8 excluded experimental tools (IDs 16-23)
		excludedTools := []string{
			"kubescape",
			"shellcheck",
			"zizmor",
			"checkov",
			"brakeman",
			"rubocop",
			"ktlint",
			"phpcs",
		}

		catalogNames := make(map[string]bool)
		catalogIDs := make(map[int64]bool)
		for _, tool := range catalog {
			catalogNames[strings.ToLower(tool.Name)] = true
			catalogIDs[tool.ID] = true
		}

		for _, excluded := range excludedTools {
			if catalogNames[strings.ToLower(excluded)] {
				t.Errorf("excluded experimental tool %q was unexpectedly found in default catalog", excluded)
			}

			// Verify GetAvailableToolByName returns nil for excluded tools
			tool, err := store.GetAvailableToolByName(ctx, excluded)
			if err != nil {
				t.Errorf("GetAvailableToolByName(%q) returned error: %v", excluded, err)
			}
			if tool != nil {
				t.Errorf("GetAvailableToolByName(%q) expected nil, got %+v", excluded, tool)
			}
		}

		// Verify no tool has ID in range 16-23 or any ID > 15
		for id := int64(16); id <= 23; id++ {
			if catalogIDs[id] {
				t.Errorf("default catalog contains tool with excluded experimental ID %d", id)
			}
		}

		for _, tool := range catalog {
			if tool.ID > 15 {
				t.Errorf("found tool %q with ID %d > 15; only launch tools 1-15 should be in default catalog", tool.Name, tool.ID)
			}
		}
	})

	t.Run("GetAvailableToolsForOrg_MergesSettings", func(t *testing.T) {
		ctx := context.Background()

		t.Run("DefaultSettingsWhenNoOrgConfig", func(t *testing.T) {
			store := NewToolsStore(nil)
			views, err := store.GetAvailableToolsForOrg(ctx, 123)
			if err != nil {
				t.Fatalf("GetAvailableToolsForOrg(nil DB) failed: %v", err)
			}
			if len(views) != 15 {
				t.Fatalf("expected 15 views, got %d", len(views))
			}
			for _, v := range views {
				if v.Enabled {
					t.Errorf("tool %s: expected Enabled=false by default, got true", v.Name)
				}
				if string(v.ConfigJSON) != "{}" {
					t.Errorf("tool %s: expected ConfigJSON='{}', got %s", v.Name, string(v.ConfigJSON))
				}
			}
		})

		t.Run("MergesCustomOrgSettings", func(t *testing.T) {
			orgSettings := map[string]interface{}{
				"ruff": true,
				"semgrep": map[string]interface{}{
					"enabled": true,
					"config_json": map[string]interface{}{
						"rules": []string{"p/default", "p/security"},
					},
				},
				"bandit": false,
			}
			settingsBytes, err := json.Marshal(orgSettings)
			if err != nil {
				t.Fatalf("failed to marshal org settings: %v", err)
			}

			setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if strings.Contains(query, "settings->'tools'") {
					if len(args) > 0 && args[0].Value == int64(456) {
						return &mockRows{
							columns: []string{"tools"},
							rows:    [][]driver.Value{{settingsBytes}},
						}, nil
					}
				}
				if strings.Contains(query, "system_settings") {
					return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
				}
				return nil, fmt.Errorf("unexpected query: %s", query)
			}, nil)
			defer resetMockHandlers()

			store := NewToolsStore(db)
			views, err := store.GetAvailableToolsForOrg(ctx, 456)
			if err != nil {
				t.Fatalf("GetAvailableToolsForOrg failed: %v", err)
			}

			if len(views) != 15 {
				t.Fatalf("expected 15 views, got %d", len(views))
			}

			viewMap := make(map[string]OrgToolView)
			for _, v := range views {
				viewMap[strings.ToLower(v.Name)] = v
			}

			// 1. ruff: enabled via boolean true
			ruff, ok := viewMap["ruff"]
			if !ok {
				t.Fatal("ruff missing from views")
			}
			if !ruff.Enabled {
				t.Errorf("expected ruff Enabled=true, got %v", ruff.Enabled)
			}
			if string(ruff.ConfigJSON) != "{}" {
				t.Errorf("expected ruff ConfigJSON='{}', got %s", string(ruff.ConfigJSON))
			}

			// 2. semgrep: enabled via map with custom config_json
			semgrep, ok := viewMap["semgrep"]
			if !ok {
				t.Fatal("semgrep missing from views")
			}
			if !semgrep.Enabled {
				t.Errorf("expected semgrep Enabled=true, got %v", semgrep.Enabled)
			}
			if !strings.Contains(string(semgrep.ConfigJSON), "p/security") {
				t.Errorf("expected semgrep ConfigJSON to contain 'p/security', got %s", string(semgrep.ConfigJSON))
			}

			// 3. bandit: explicitly disabled via boolean false
			bandit, ok := viewMap["bandit"]
			if !ok {
				t.Fatal("bandit missing from views")
			}
			if bandit.Enabled {
				t.Errorf("expected bandit Enabled=false, got %v", bandit.Enabled)
			}

			// 4. eslint: not configured in org settings, defaults to disabled
			eslint, ok := viewMap["eslint"]
			if !ok {
				t.Fatal("eslint missing from views")
			}
			if eslint.Enabled {
				t.Errorf("expected eslint Enabled=false, got %v", eslint.Enabled)
			}
			if string(eslint.ConfigJSON) != "{}" {
				t.Errorf("expected eslint ConfigJSON='{}', got %s", string(eslint.ConfigJSON))
			}
		})

		t.Run("MergesByToolIDKey", func(t *testing.T) {
			// Tool ID 1 corresponds to ruff
			idSettings := `{"1": true}`
			setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if strings.Contains(query, "settings->'tools'") {
					return &mockRows{
						columns: []string{"tools"},
						rows:    [][]driver.Value{{[]byte(idSettings)}},
					}, nil
				}
				return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
			}, nil)
			defer resetMockHandlers()

			store := NewToolsStore(db)
			views, err := store.GetAvailableToolsForOrg(ctx, 789)
			if err != nil {
				t.Fatalf("GetAvailableToolsForOrg failed: %v", err)
			}

			for _, v := range views {
				if v.ID == 1 {
					if !v.Enabled {
						t.Errorf("expected tool ID 1 (ruff) to be enabled via ID key, got false")
					}
				} else {
					if v.Enabled {
						t.Errorf("expected tool %s (ID %d) to be disabled, got true", v.Name, v.ID)
					}
				}
			}
		})
	})

	t.Run("UpsertOrgTool_EnableDisable", func(t *testing.T) {
		ctx := context.Background()
		const orgID = int64(300)

		t.Run("EnableTool", func(t *testing.T) {
			var capturedQuery string
			var capturedArgs []driver.NamedValue

			setMockHandlers(
				func(query string, args []driver.NamedValue) (driver.Rows, error) {
					if strings.Contains(query, "settings->'tools'") {
						return &mockRows{
							columns: []string{"tools"},
							rows:    [][]driver.Value{{[]byte(`{}`)}},
						}, nil
					}
					return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
				},
				func(query string, args []driver.NamedValue) (driver.Result, error) {
					capturedQuery = query
					capturedArgs = args
					return mockResult{rowsAffected: 1}, nil
				},
			)
			defer resetMockHandlers()

			store := NewToolsStore(db)
			// Tool ID 1 = ruff
			row, err := store.UpsertOrgTool(ctx, orgID, 1, true)
			if err != nil {
				t.Fatalf("UpsertOrgTool(enable) returned error: %v", err)
			}

			if row.OrgID != orgID {
				t.Errorf("expected OrgID %d, got %d", orgID, row.OrgID)
			}
			if row.ToolID != 1 {
				t.Errorf("expected ToolID 1, got %d", row.ToolID)
			}
			if !row.Enabled {
				t.Errorf("expected Enabled=true, got %v", row.Enabled)
			}
			if string(row.ConfigJSON) != "{}" {
				t.Errorf("expected default ConfigJSON '{}', got %s", string(row.ConfigJSON))
			}

			if !strings.Contains(capturedQuery, "UPDATE public.orgs") {
				t.Errorf("expected UPDATE query on public.orgs, got: %s", capturedQuery)
			}
			if len(capturedArgs) < 3 {
				t.Fatalf("expected at least 3 arguments in UPDATE, got %d", len(capturedArgs))
			}
			if capturedArgs[0].Value != orgID {
				t.Errorf("arg 1 expected orgID %d, got %v", orgID, capturedArgs[0].Value)
			}
			if capturedArgs[1].Value != "ruff" {
				t.Errorf("arg 2 expected toolName 'ruff', got %v", capturedArgs[1].Value)
			}

			var savedCfg OrgToolConfig
			cfgBytes, ok := capturedArgs[2].Value.([]byte)
			if !ok {
				t.Fatalf("arg 3 expected []byte, got %T", capturedArgs[2].Value)
			}
			if err := json.Unmarshal(cfgBytes, &savedCfg); err != nil {
				t.Fatalf("failed to unmarshal saved config: %v", err)
			}
			if !savedCfg.Enabled {
				t.Errorf("expected saved config Enabled=true, got %v", savedCfg.Enabled)
			}
		})

		t.Run("DisableTool_PreservingConfigJSON", func(t *testing.T) {
			var capturedArgs []driver.NamedValue
			existingSettings := `{"ruff": {"enabled": true, "config_json": {"timeout_seconds": 60}}}`

			setMockHandlers(
				func(query string, args []driver.NamedValue) (driver.Rows, error) {
					if strings.Contains(query, "settings->'tools'") {
						return &mockRows{
							columns: []string{"tools"},
							rows:    [][]driver.Value{{[]byte(existingSettings)}},
						}, nil
					}
					return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
				},
				func(query string, args []driver.NamedValue) (driver.Result, error) {
					capturedArgs = args
					return mockResult{rowsAffected: 1}, nil
				},
			)
			defer resetMockHandlers()

			store := NewToolsStore(db)
			// Disable tool ID 1 (ruff)
			row, err := store.UpsertOrgTool(ctx, orgID, 1, false)
			if err != nil {
				t.Fatalf("UpsertOrgTool(disable) returned error: %v", err)
			}

			if row.Enabled {
				t.Errorf("expected Enabled=false, got %v", row.Enabled)
			}
			if !strings.Contains(string(row.ConfigJSON), "timeout_seconds") {
				t.Errorf("expected row ConfigJSON to preserve 'timeout_seconds', got %s", string(row.ConfigJSON))
			}

			if len(capturedArgs) >= 3 {
				var savedCfg OrgToolConfig
				cfgBytes := capturedArgs[2].Value.([]byte)
				if err := json.Unmarshal(cfgBytes, &savedCfg); err != nil {
					t.Fatalf("failed to unmarshal saved config: %v", err)
				}
				if savedCfg.Enabled {
					t.Errorf("expected saved config Enabled=false, got true")
				}
				if !strings.Contains(string(savedCfg.ConfigJSON), "timeout_seconds") {
					t.Errorf("expected saved config ConfigJSON to preserve 'timeout_seconds', got %s", string(savedCfg.ConfigJSON))
				}
			}
		})

		t.Run("ToolNotFound_ReturnsErrNoRows", func(t *testing.T) {
			store := NewToolsStore(db)

			// Unknown tool ID
			_, err := store.UpsertOrgTool(ctx, orgID, 9999, true)
			if err != sql.ErrNoRows {
				t.Errorf("expected sql.ErrNoRows for toolID 9999, got %v", err)
			}

			// Excluded experimental tool IDs (16 to 23) must return ErrNoRows
			for id := int64(16); id <= 23; id++ {
				_, err := store.UpsertOrgTool(ctx, orgID, id, true)
				if err != sql.ErrNoRows {
					t.Errorf("expected sql.ErrNoRows for excluded toolID %d, got %v", id, err)
				}
			}
		})
	})

	t.Run("GetEnabledToolsForOrg", func(t *testing.T) {
		ctx := context.Background()
		orgSettings := `{"ruff": true, "semgrep": {"enabled": true}, "bandit": false}`

		setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(query, "settings->'tools'") {
				return &mockRows{
					columns: []string{"tools"},
					rows:    [][]driver.Value{{[]byte(orgSettings)}},
				}, nil
			}
			return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
		}, nil)
		defer resetMockHandlers()

		store := NewToolsStore(db)
		enabledTools, err := store.GetEnabledToolsForOrg(ctx, 500)
		if err != nil {
			t.Fatalf("GetEnabledToolsForOrg failed: %v", err)
		}

		if len(enabledTools) != 2 {
			t.Fatalf("expected exactly 2 enabled tools, got %d", len(enabledTools))
		}

		enabledMap := make(map[string]bool)
		for _, tool := range enabledTools {
			enabledMap[strings.ToLower(tool.Name)] = true
		}
		if !enabledMap["ruff"] {
			t.Errorf("expected ruff to be enabled")
		}
		if !enabledMap["semgrep"] {
			t.Errorf("expected semgrep to be enabled")
		}
		if enabledMap["bandit"] {
			t.Errorf("bandit should not be enabled")
		}
	})

	t.Run("GetAvailableToolByName", func(t *testing.T) {
		ctx := context.Background()
		store := NewToolsStore(nil)

		// Exact match
		tool, err := store.GetAvailableToolByName(ctx, "ruff")
		if err != nil || tool == nil || tool.Name != "ruff" {
			t.Fatalf("expected ruff tool, got %v, err=%v", tool, err)
		}

		// Case-insensitive with whitespace
		toolCase, err := store.GetAvailableToolByName(ctx, "  ESLINT  ")
		if err != nil || toolCase == nil || toolCase.Name != "eslint" {
			t.Fatalf("expected eslint tool, got %v, err=%v", toolCase, err)
		}

		// Unknown tool
		unknownTool, err := store.GetAvailableToolByName(ctx, "nonexistent-tool")
		if err != nil || unknownTool != nil {
			t.Fatalf("expected nil for nonexistent tool, got %v, err=%v", unknownTool, err)
		}
	})
}
