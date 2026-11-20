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

func mockSampleToolsRows() *mockRows {
	return &mockRows{
		columns: []string{"id", "name", "description", "lambda_arn", "multiplier", "use_case"},
		rows: [][]driver.Value{
			{int64(1), "tool-a", "Tool A description", "arn:aws:lambda:us-east-1:042198857528:function:tool-a", 1.0, "Linting"},
			{int64(2), "tool-b", "Tool B description", "arn:aws:lambda:us-east-1:042198857528:function:tool-b", 5.0, "Security"},
		},
	}
}

func defaultMockQueryHandler(query string, args []driver.NamedValue) (driver.Rows, error) {
	if strings.Contains(query, "available_tools") {
		return mockSampleToolsRows(), nil
	}
	if strings.Contains(query, "system_settings") {
		return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
	}
	if strings.Contains(query, "settings->'tools'") {
		return &mockRows{columns: []string{"tools"}, rows: [][]driver.Value{}}, nil
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
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

	t.Run("LoadCatalog_EmptyWhenNoDBOrNoTools", func(t *testing.T) {
		ctx := context.Background()

		// 1. Nil DB returns empty list
		storeNilDB := NewToolsStore(nil)
		catalog, err := storeNilDB.LoadCatalog(ctx)
		if err != nil {
			t.Fatalf("LoadCatalog(nil DB) returned error: %v", err)
		}
		if len(catalog) != 0 {
			t.Fatalf("expected 0 tools on nil DB, got %d", len(catalog))
		}

		// 2. Empty available_tools table returns empty list
		setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(query, "available_tools") {
				return &mockRows{
					columns: []string{"id", "name", "description", "lambda_arn", "multiplier", "use_case"},
					rows:    [][]driver.Value{},
				}, nil
			}
			if strings.Contains(query, "system_settings") {
				return &mockRows{columns: []string{"data"}, rows: [][]driver.Value{}}, nil
			}
			return nil, fmt.Errorf("unexpected query: %s", query)
		}, nil)
		defer resetMockHandlers()

		storeWithDB := NewToolsStore(db)
		emptyCatalog, err := storeWithDB.LoadCatalog(ctx)
		if err != nil {
			t.Fatalf("LoadCatalog returned error: %v", err)
		}
		if len(emptyCatalog) != 0 {
			t.Fatalf("expected 0 tools when available_tools table is empty, got %d", len(emptyCatalog))
		}
	})

	t.Run("LoadCatalog_LoadsFromAvailableToolsTable", func(t *testing.T) {
		ctx := context.Background()
		setMockHandlers(defaultMockQueryHandler, nil)
		defer resetMockHandlers()

		store := NewToolsStore(db)
		catalog, err := store.LoadCatalog(ctx)
		if err != nil {
			t.Fatalf("LoadCatalog failed: %v", err)
		}
		if len(catalog) != 2 {
			t.Fatalf("expected 2 tools from DB, got %d", len(catalog))
		}

		if catalog[0].Name != "tool-a" || catalog[0].LambdaARN == "" {
			t.Errorf("unexpected tool[0]: %+v", catalog[0])
		}
		if catalog[1].Name != "tool-b" || catalog[1].Multiplier != 5.0 {
			t.Errorf("unexpected tool[1]: %+v", catalog[1])
		}
	})

	t.Run("GetAvailableToolsForOrg_MergesSettings", func(t *testing.T) {
		ctx := context.Background()

		t.Run("DefaultSettingsWhenNoOrgConfig", func(t *testing.T) {
			setMockHandlers(defaultMockQueryHandler, nil)
			defer resetMockHandlers()

			store := NewToolsStore(db)
			views, err := store.GetAvailableToolsForOrg(ctx, 123)
			if err != nil {
				t.Fatalf("GetAvailableToolsForOrg failed: %v", err)
			}
			if len(views) != 2 {
				t.Fatalf("expected 2 views, got %d", len(views))
			}
			for _, v := range views {
				if v.Enabled {
					t.Errorf("tool %s: expected Enabled=false by default, got true", v.Name)
				}
			}
		})

		t.Run("MergesCustomOrgSettings", func(t *testing.T) {
			orgSettings := map[string]interface{}{
				"tool-a": true,
				"tool-b": false,
			}
			settingsBytes, _ := json.Marshal(orgSettings)

			setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
				if strings.Contains(query, "available_tools") {
					return mockSampleToolsRows(), nil
				}
				if strings.Contains(query, "settings->'tools'") {
					return &mockRows{
						columns: []string{"tools"},
						rows:    [][]driver.Value{{settingsBytes}},
					}, nil
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
			if len(views) != 2 {
				t.Fatalf("expected 2 views, got %d", len(views))
			}

			viewMap := make(map[string]OrgToolView)
			for _, v := range views {
				viewMap[v.Name] = v
			}

			if !viewMap["tool-a"].Enabled {
				t.Errorf("expected tool-a Enabled=true")
			}
			if viewMap["tool-b"].Enabled {
				t.Errorf("expected tool-b Enabled=false")
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
					if strings.Contains(query, "available_tools") {
						return mockSampleToolsRows(), nil
					}
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
			row, err := store.UpsertOrgTool(ctx, orgID, 1, true)
			if err != nil {
				t.Fatalf("UpsertOrgTool(enable) returned error: %v", err)
			}

			if row.OrgID != orgID || row.ToolID != 1 || !row.Enabled {
				t.Errorf("unexpected upsert result: %+v", row)
			}
			if !strings.Contains(capturedQuery, "UPDATE public.orgs") {
				t.Errorf("expected UPDATE query on public.orgs, got: %s", capturedQuery)
			}
			if len(capturedArgs) < 3 {
				t.Fatalf("expected at least 3 arguments in UPDATE, got %d", len(capturedArgs))
			}
		})

		t.Run("ToolNotFound_ReturnsErrNoRows", func(t *testing.T) {
			setMockHandlers(defaultMockQueryHandler, nil)
			defer resetMockHandlers()
			store := NewToolsStore(db)

			// Unknown tool ID
			_, err := store.UpsertOrgTool(ctx, orgID, 9999, true)
			if err != sql.ErrNoRows {
				t.Errorf("expected sql.ErrNoRows for toolID 9999, got %v", err)
			}
		})
	})

	t.Run("GetEnabledToolsForOrg", func(t *testing.T) {
		ctx := context.Background()
		orgSettings := `{"tool-a": true, "tool-b": false}`

		setMockHandlers(func(query string, args []driver.NamedValue) (driver.Rows, error) {
			if strings.Contains(query, "available_tools") {
				return mockSampleToolsRows(), nil
			}
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

		if len(enabledTools) != 1 {
			t.Fatalf("expected exactly 1 enabled tool, got %d", len(enabledTools))
		}
		if enabledTools[0].Name != "tool-a" {
			t.Errorf("expected tool-a to be enabled, got %s", enabledTools[0].Name)
		}
	})

	t.Run("GetAvailableToolByName", func(t *testing.T) {
		ctx := context.Background()
		setMockHandlers(defaultMockQueryHandler, nil)
		defer resetMockHandlers()
		store := NewToolsStore(db)

		// Exact match
		tool, err := store.GetAvailableToolByName(ctx, "tool-a")
		if err != nil || tool == nil || tool.Name != "tool-a" {
			t.Fatalf("expected tool-a, got %v, err=%v", tool, err)
		}

		// Case-insensitive with whitespace
		toolCase, err := store.GetAvailableToolByName(ctx, "  TOOL-A  ")
		if err != nil || toolCase == nil || toolCase.Name != "tool-a" {
			t.Fatalf("expected tool-a, got %v, err=%v", toolCase, err)
		}

		// Unknown tool
		unknownTool, err := store.GetAvailableToolByName(ctx, "nonexistent-tool")
		if err != nil || unknownTool != nil {
			t.Fatalf("expected nil for nonexistent tool, got %v, err=%v", unknownTool, err)
		}
	})
}
