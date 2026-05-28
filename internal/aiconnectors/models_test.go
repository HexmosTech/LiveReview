package aiconnectors

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"
)

type mockDriver struct{}
type mockConn struct{}
type mockStmt struct{}
type mockRows struct {
	rows []driver.Value
	idx  int
}

func (d mockDriver) Open(name string) (driver.Conn, error) { return mockConn{}, nil }
func (c mockConn) Close() error                           { return nil }
func (c mockConn) Begin() (driver.Tx, error)             { return nil, nil }
func (c mockConn) Prepare(query string) (driver.Stmt, error) {
	return mockStmt{}, nil
}
func (s mockStmt) Close() error { return nil }
func (s mockStmt) NumInput() int { return -1 }
func (s mockStmt) Exec(args []driver.Value) (driver.Result, error) { return nil, nil }
func (s mockStmt) Query(args []driver.Value) (driver.Rows, error) {
	provider := ""
	if len(args) > 0 {
		if str, ok := args[0].(string); ok {
			provider = str
		}
	}
	
	if provider == "openai" {
		return &mockRows{
			rows: []driver.Value{"o4-mini", "gpt-4.1", "gpt-4.1-mini"},
		}, nil
	}
	if provider == "claude" {
		return &mockRows{
			rows: []driver.Value{
				"claude-haiku-4-5-20251001",
				"claude-3-opus-20240229",
				"claude-3-sonnet-20240229",
				"claude-3-haiku-20240307",
			},
		}, nil
	}
	return &mockRows{}, nil
}

func (r *mockRows) Columns() []string { return []string{"model_id"} }
func (r *mockRows) Close() error      { return nil }
func (r *mockRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	dest[0] = r.rows[r.idx]
	r.idx++
	return nil
}

var testDB *sql.DB

func init() {
	sql.Register("mock-ai-db", mockDriver{})
	db, err := sql.Open("mock-ai-db", "")
	if err != nil {
		panic(err)
	}
	testDB = db
}

func TestGetDefaultModelOpenAI(t *testing.T) {
	storage := NewStorage(testDB)
	if got := storage.GetDefaultModel(context.Background(), ProviderOpenAI); got != "o4-mini" {
		t.Fatalf("expected OpenAI default model o4-mini, got %q", got)
	}
}

func TestGetProviderModelsOpenAIIncludesO4MiniFirst(t *testing.T) {
	storage := NewStorage(testDB)
	models := storage.GetProviderModels(context.Background(), ProviderOpenAI)
	if len(models) == 0 {
		t.Fatal("expected OpenAI model list to be non-empty")
	}
	if models[0] != "o4-mini" {
		t.Fatalf("expected first OpenAI model to be o4-mini, got %q", models[0])
	}
}

func TestGetDefaultModelClaude(t *testing.T) {
	storage := NewStorage(testDB)
	if got := storage.GetDefaultModel(context.Background(), ProviderClaude); got != "claude-haiku-4-5-20251001" {
		t.Fatalf("expected Claude default model claude-haiku-4-5-20251001, got %q", got)
	}
}

func TestGetProviderModelsClaudeIncludesHaikuFirst(t *testing.T) {
	storage := NewStorage(testDB)
	models := storage.GetProviderModels(context.Background(), ProviderClaude)
	if len(models) == 0 {
		t.Fatal("expected Claude model list to be non-empty")
	}
	if models[0] != "claude-haiku-4-5-20251001" {
		t.Fatalf("expected first Claude model to be claude-haiku-4-5-20251001, got %q", models[0])
	}
}

func TestGetProviderModelsClaudeIncludesLegacyModels(t *testing.T) {
	storage := NewStorage(testDB)
	models := storage.GetProviderModels(context.Background(), ProviderClaude)
	set := make(map[string]struct{}, len(models))
	for _, m := range models {
		set[m] = struct{}{}
	}

	legacy := []string{
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}

	for _, model := range legacy {
		if _, ok := set[model]; !ok {
			t.Fatalf("expected legacy Claude model %q to remain available for compatibility", model)
		}
	}
}

func TestConnectorRecordGetConnectorOptionsDefaultsOpenAIModel(t *testing.T) {
	storage := NewStorage(testDB)
	record := &ConnectorRecord{
		ProviderName: string(ProviderOpenAI),
		Provider:     ProviderOpenAI,
		ApiKey:       "sk-test",
		SelectedModel: sql.NullString{
			Valid: false,
		},
	}

	opts := storage.GetConnectorOptions(context.Background(), record)
	if opts.ModelConfig.Model != "o4-mini" {
		t.Fatalf("expected default OpenAI model o4-mini, got %q", opts.ModelConfig.Model)
	}
}
