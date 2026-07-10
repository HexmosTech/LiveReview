// Package scheduledreview runs the periodic background loop that finds repos due for a
// scheduled review and enqueues a River job for each. The actual review work happens in
// jobqueue.ScheduledReviewWorker; this package only decides "what's due right now".
package scheduledreview

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/livereview/internal/jobqueue"
	scheduledreviewstore "github.com/livereview/storage/scheduledreview"
)

// RunScheduler polls for due scheduled-review configs on a ticker and enqueues a River job
// for each. It blocks until ctx is cancelled, so callers should invoke it in a goroutine.
func RunScheduler(ctx context.Context, db *sql.DB, jq *jobqueue.JobQueue, interval time.Duration) {
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	store := scheduledreviewstore.NewStore(db)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			due, err := store.ListDue(ctx)
			if err != nil {
				log.Printf("[scheduled-review-scheduler] list due configs failed: %v", err)
				continue
			}
			for _, cfg := range due {
				// Claim it (push next_run_at out) before enqueueing so a slow-running job
				// doesn't get picked up again by the next tick. The worker sets the real
				// next_run_at based on interval_hours once the run completes.
				if err := store.Claim(ctx, cfg.ID, time.Now().Add(interval)); err != nil {
					log.Printf("[scheduled-review-scheduler] config=%d project=%s claim failed: %v", cfg.ID, cfg.ProjectFullName, err)
					continue
				}
				if err := jq.QueueScheduledReviewJob(ctx, cfg.ID); err != nil {
					log.Printf("[scheduled-review-scheduler] config=%d project=%s enqueue failed: %v", cfg.ID, cfg.ProjectFullName, err)
				}
			}
		}
	}
}
