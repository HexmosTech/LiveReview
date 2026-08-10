// Package scheduledreview runs the background loop that finds repos due for a scheduled review and enqueues a River job for each; the actual review work happens in jobqueue.ScheduledReviewWorker.
package scheduledreview

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/livereview/internal/jobqueue"
	scheduledreviewstore "github.com/livereview/storage/scheduledreview"
)

// maxSleep caps how long the scheduler sleeps before rechecking the DB; also doubles as the claim window (see Claim).
const maxSleep = 1 * time.Minute

// RunScheduler is a dynamically-timed loop (sleeps exactly until the next due config, capped at maxSleep) rather than a fixed-interval poll; wake lets callers interrupt that sleep early. Blocks until ctx is cancelled - invoke in a goroutine.
func RunScheduler(ctx context.Context, db *sql.DB, jq *jobqueue.JobQueue, wake <-chan struct{}) {
	store := scheduledreviewstore.NewStore(db)

	for {
		wait := maxSleep
		if next, ok, err := store.NextDueTime(ctx); err != nil {
			log.Printf("[scheduled-review-scheduler] failed to compute next due time: %v", err)
		} else if ok {
			if until := time.Until(next); until < wait {
				if until < 0 {
					until = 0
				}
				wait = until
			}
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-wake:
			// Something changed - drop this sleep and recompute from fresh DB state.
			timer.Stop()
			continue
		case <-timer.C:
		}

		due, err := store.ListDue(ctx)
		if err != nil {
			log.Printf("[scheduled-review-scheduler] list due configs failed: %v", err)
			continue
		}
		for _, cfg := range due {
			// Claim (push next_run_at out) before enqueueing so this config isn't re-picked-up while its job runs.
			if err := store.Claim(ctx, cfg.ID, time.Now().Add(maxSleep)); err != nil {
				log.Printf("[scheduled-review-scheduler] config=%d project=%s claim failed: %v", cfg.ID, cfg.ProjectFullName, err)
				continue
			}
			if err := jq.QueueScheduledReviewJob(ctx, cfg.ID); err != nil {
				log.Printf("[scheduled-review-scheduler] config=%d project=%s enqueue failed: %v", cfg.ID, cfg.ProjectFullName, err)
			}
		}
	}
}
