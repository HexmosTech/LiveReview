// Package seed_demo replays an org's own historical review activity forward in time,
// with light mutation, so a demo account that has stopped generating real usage (e.g.
// its AI connector broke) keeps showing fresh, varying activity without spending AI
// tokens on real reviews. See cmd/seed-demo-activity for the manual entrypoint and
// worker.go for the automatic (River periodic job) entrypoint. All DB access lives in
// storage/seed_demo, per this repo's storage/business-logic boundary rule.
package seed_demo

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	storageseeddemo "github.com/livereview/storage/seed_demo"
)

// DemoOrgID is the Ostrelle Systems demo org (org_id 677) - the only org this package
// is allowed to generate synthetic activity for. This is a demo-account cosmetic fix,
// not a general-purpose data seeding tool, so the org is hardcoded rather than taking
// an arbitrary org ID: nothing about this package should ever run against a real
// customer's data, even by a config typo.
const DemoOrgID int64 = 677

// DefaultCount returns a random 1-3, the default number of synthetic reviews per run
// when the caller doesn't specify an explicit count.
func DefaultCount() (int, error) {
	return randomCount(1, 3)
}

// Run clones `count` randomly-picked historical reviews for orgID into new synthetic
// reviews dated today, so a demo account whose real usage has gone quiet keeps showing
// fresh activity without spending AI tokens on real reviews.
func Run(ctx context.Context, db *sql.DB, orgID int64, count int) error {
	return RunForDay(ctx, db, orgID, count, time.Now())
}

// RunForDay is Run, but dating the synthetic reviews to the given day instead of today -
// e.g. to backfill a day or two of activity that was missed. day's time-of-day component
// is ignored; only its calendar date is used (see randomTimeToday).
func RunForDay(ctx context.Context, db *sql.DB, orgID int64, count int, day time.Time) error {
	if orgID != DemoOrgID {
		return fmt.Errorf("seed_demo is restricted to org %d (Ostrelle Systems demo), got %d", DemoOrgID, orgID)
	}
	if count <= 0 {
		return fmt.Errorf("count must be positive, got %d", count)
	}

	store := storageseeddemo.NewStore(db)
	pool, err := store.LoadSourcePool(ctx, orgID)
	if err != nil {
		return err
	}

	indices, err := pickN(len(pool), count)
	if err != nil {
		return fmt.Errorf("pick sources: %w", err)
	}

	created := 0
	for _, idx := range indices {
		src := pool[idx]
		if err := cloneOne(ctx, store, db, orgID, src, day); err != nil {
			log.Printf("[seed_demo] org_id=%d source_review_id=%d failed: %v", orgID, src.ID, err)
			continue
		}
		created++
	}

	if created == 0 {
		return fmt.Errorf("all %d clone attempt(s) failed for org %d", len(indices), orgID)
	}

	log.Printf("[seed_demo] org_id=%d day=%s created %d/%d synthetic review(s)", orgID, day.Format("2006-01-02"), created, len(indices))
	return nil
}
