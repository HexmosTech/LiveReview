package prompts

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
	vendorpack "github.com/livereview/internal/prompts/vendor"
)

// This is a smoke test that requires a Postgres DATABASE_URL and vendor build.
// It is skipped unless RUN_RENDER_INTEGRATION=1.
func TestRenderSmoke(t *testing.T) {
	if os.Getenv("RUN_RENDER_INTEGRATION") != "1" {
		t.Skip("set RUN_RENDER_INTEGRATION=1 to run")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	defer db.Close()

	st := NewStore(db)
	// Construct vendor pack; in default builds, this returns a stub that will ErrNotFound.
	pk := vendorpack.New()
	m := NewManager(st, pk)

	ctx := context.Background()
	c := Context{OrgID: 1}
	_, _ = m.ResolveApplicationContext(ctx, c)

	// Render a known prompt key; vars map empty triggers chunk resolution.
	_, _ = m.Render(ctx, c, "code_review", map[string]string{})
}
