package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/lib/pq"
	"github.com/livereview/internal/prompts"
	vendorpack "github.com/livereview/internal/prompts/vendor"
)

func main() {
	orgID := flag.Int64("org", 0, "Org ID (required)")
	promptKey := flag.String("prompt", "code_review", "Prompt key")
	flag.Parse()
	if *orgID == 0 {
		log.Fatal("--org is required")
	}

	// Load DATABASE_URL from .env similar to main server logic
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// attempt reading .env manually (simple parse)
		f, err := os.ReadFile(".env")
		if err == nil {
			lines := string(f)
			for _, ln := range strings.Split(lines, "\n") {
				if strings.HasPrefix(ln, "DATABASE_URL=") {
					dbURL = strings.TrimPrefix(ln, "DATABASE_URL=")
					break
				}
			}
		}
	}
	if dbURL == "" {
		log.Fatal("DATABASE_URL not found in environment or .env")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	store := prompts.NewStore(db)
	pack := vendorpack.New()
	mgr := prompts.NewManager(store, pack)
	ctxSel := prompts.Context{OrgID: *orgID}

	out, err := mgr.Render(context.Background(), ctxSel, *promptKey, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("---- RENDERED PROMPT ----")
	fmt.Println(out)
}
