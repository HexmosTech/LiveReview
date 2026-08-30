// Command dashboard-cache-backfill is a one-time maintenance script that populates
// dashboard_cache with historical review_layers and issue_treemap data for every org
// that has run at least one review in the backfill window.
//
// Root cause this fixes: dashboard_cache is only ever kept fresh going forward by
// DashboardManager's regular 5-minute tick, which only ever computes "today" - it
// never backfills past days. Orgs that had review history before this cache existed
// (or before a gap in the ticker) show thin/empty week/month/all totals on the
// dashboard until enough new days accumulate naturally. This script replays the same
// day-by-day merge the ticker already does, just across a date range instead of one day.
//
// Safe to re-run: each day's merge is idempotent (upsert), and days outside the
// requested window are left untouched.
//
// Usage:
//
//	go run ./cmd/dashboard-cache-backfill               # last 30 days, every active org
//	go run ./cmd/dashboard-cache-backfill -days=7        # last 7 days instead
package main

import (
	"context"
	"flag"
	"log"

	"github.com/livereview/internal/api"
	"github.com/livereview/internal/database"
	"github.com/livereview/storage/dashboard"
)

func main() {
	days := flag.Int("days", 30, "Number of days of history to backfill, ending today")
	flag.Parse()

	db, err := database.NewDB()
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	cacheStore := dashboard.NewCacheStore(db)
	dashboardManager := api.NewDashboardManager(db, nil, cacheStore)

	if err := dashboardManager.BackfillReviewLayers(context.Background(), *days); err != nil {
		log.Fatalf("review_layers backfill failed: %v", err)
	}

	if err := dashboardManager.BackfillIssueTreemap(context.Background(), *days); err != nil {
		log.Fatalf("issue_treemap backfill failed: %v", err)
	}

	log.Printf("backfill complete: %d day(s)", *days)
}
