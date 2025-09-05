package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/lib/pq"

	prompts "github.com/livereview/internal/prompts"
	vendorpack "github.com/livereview/internal/prompts/vendor"
)

// This tiny program exercises the Manager.Render path repeatedly to make it
// easy to capture a memory dump and search for raw vendor template markers.
// Build with: go build -tags vendor_prompts -o render-smoke ./cmd/render-smoke
func main() {
	orgID := int64(1)
	if v := os.Getenv("ORG_ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			orgID = n
		}
	}

	// Optional: connect to DB to allow chunk resolution during renders.
	var store *prompts.Store
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		defer db.Close()
		store = prompts.NewStore(db)
	}

	// Use real vendor pack when built with vendor_prompts. If not, fall back to stub via New().
	pack := vendorpack.New()
	mgr := prompts.NewManager(store, pack)

	ctx := context.Background()
	ac := prompts.Context{OrgID: orgID}

	// Render in a loop.
	key := envOr("PROMPT_KEY", "code_review")
	loops := envInt("LOOPS", 50)
	for i := 0; i < loops; i++ {
		_, err := mgr.Render(ctx, ac, key, map[string]string{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "render error: %v\n", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Sleep to allow attaching gcore/gdb if desired.
	fmt.Println("render-smoke complete; sleeping 5s...")
	time.Sleep(5 * time.Second)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
