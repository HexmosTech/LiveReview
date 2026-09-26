// Command seed-demo-activity keeps a demo org's dashboard looking alive by cloning a
// few of that org's own historical completed reviews into new, lightly-mutated
// synthetic reviews dated today - without spending AI provider tokens on real reviews.
//
// This exists because the Ostrelle Systems demo account's AI connector broke (Gemini
// 2.5 Flash was deprecated by Google) and real usage stopped on 2026-09-20, leaving the
// demo dashboard stuck on stale data. Rather than replaying raw historical data
// verbatim (risking duplicate-looking content and unique-constraint collisions),
// each run picks a random subset of the org's own real reviews and clones them with a
// fresh commit SHA, a randomized time-of-day, and jittered LOC/token/cost numbers.
// The org's existing pull_requests row is reused as-is (same PR, "another commit
// pushed" - which matches how this org's real historical data already looks: many
// reviews per PR). Synthetic rows are tagged metadata.seed_demo=true so the source
// pool never re-clones its own output, and so synthetic rows stay identifiable/wipeable.
//
// Restricted to the one demo org (seed_demo.DemoOrgID, Ostrelle Systems) and gated by
// seed_demo.Enabled (LIVEREVIEW_IS_CLOUD=true and SEED_DEMO_ENABLED=true), the same rule
// the daily River job uses. The daily job is the normal path; this tool is for manual
// backfills, where both env vars must be passed explicitly.
//
// See internal/seed_demo for the implementation.
//
// Usage:
//
//	LIVEREVIEW_IS_CLOUD=true SEED_DEMO_ENABLED=true go run ./cmd/seed-demo-activity -org-id=677
//	LIVEREVIEW_IS_CLOUD=true SEED_DEMO_ENABLED=true go run ./cmd/seed-demo-activity -org-id=677 -count=3 -days-ago=1
package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/livereview/internal/database"
	"github.com/livereview/internal/seed_demo"
)

func main() {
	orgID := flag.Int64("org-id", 0, "Org ID to seed demo activity for (required, and must equal seed_demo.DemoOrgID - typed explicitly so this is never run against the wrong org by accident)")
	count := flag.Int("count", 0, "Number of synthetic reviews to create this run (0 = random 1-7)")
	daysAgo := flag.Int("days-ago", 0, "Backdate the synthetic reviews this many days (0 = today, 1 = yesterday, etc.)")
	flag.Parse()

	if *orgID != seed_demo.DemoOrgID {
		log.Fatalf("-org-id must be %d (the Ostrelle Systems demo org), got %d", seed_demo.DemoOrgID, *orgID)
	}
	if *daysAgo < 0 {
		log.Fatalf("-days-ago must be non-negative")
	}
	if ok, reason := seed_demo.Enabled(); !ok {
		log.Fatalf("seed-demo-activity refusing to run: %s", reason)
	}

	n := *count
	if n <= 0 {
		var err error
		n, err = seed_demo.DefaultCount()
		if err != nil {
			log.Fatalf("failed to pick random count: %v", err)
		}
	}

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	day := time.Now().AddDate(0, 0, -*daysAgo)
	if err := seed_demo.RunForDay(context.Background(), db, *orgID, n, day); err != nil {
		log.Fatalf("seed demo activity failed: %v", err)
	}
}
