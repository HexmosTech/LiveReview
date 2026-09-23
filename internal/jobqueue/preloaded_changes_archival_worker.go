package jobqueue

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/livereview/internal/blobstore"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog/log"
)

// PreloadedChangesArchivalJobArgs represents arguments for offloading a single review diff to blob storage.
type PreloadedChangesArchivalJobArgs struct {
	ReviewID   int64  `json:"review_id"`
	OrgID      int64  `json:"org_id"`
	BatchRunID string `json:"batch_run_id,omitempty"`
}

func (PreloadedChangesArchivalJobArgs) Kind() string {
	return "preloaded_changes_archival"
}

func (PreloadedChangesArchivalJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "preloaded_changes_archival",
		MaxAttempts: 10,
	}
}

// PreloadedChangesArchivalSweepJobArgs represents arguments for periodic or manual background sweep of eligible reviews.
type PreloadedChangesArchivalSweepJobArgs struct {
	RetentionDays int `json:"retention_days,omitempty"`
}

func (PreloadedChangesArchivalSweepJobArgs) Kind() string {
	return "preloaded_changes_archival_sweep"
}

func (PreloadedChangesArchivalSweepJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue: "preloaded_changes_archival_sweep",
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByState:  []rivertype.JobState{rivertype.JobStateAvailable, rivertype.JobStateRunning, rivertype.JobStateRetryable, rivertype.JobStateScheduled},
		},
	}
}



// PreloadedChangesArchivalPurgeJobArgs represents arguments for the coordinator purge worker.
// It waits until all upload jobs for a BatchRunID are completed, then executes a single DB purge query and clears created jobs.
type PreloadedChangesArchivalPurgeJobArgs struct {
	BatchRunID    string `json:"batch_run_id"`
	RetentionDays int    `json:"retention_days"`
}

func (PreloadedChangesArchivalPurgeJobArgs) Kind() string {
	return "preloaded_changes_archival_purge"
}

func (PreloadedChangesArchivalPurgeJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "preloaded_changes_archival",
		MaxAttempts: 100,
	}
}

// PreloadedChangesArchivalSweepWorker is triggered periodically (or manually) to find eligible reviews,
// bulk insert upload jobs into River, and enqueue a completion purge worker.
type PreloadedChangesArchivalSweepWorker struct {
	river.WorkerDefaults[PreloadedChangesArchivalSweepJobArgs]
	db *sql.DB
	jq *JobQueue
}

func (w *PreloadedChangesArchivalSweepWorker) Timeout(job *river.Job[PreloadedChangesArchivalSweepJobArgs]) time.Duration {
	return 10 * time.Minute
}

func (w *PreloadedChangesArchivalSweepWorker) NextRetry(job *river.Job[PreloadedChangesArchivalSweepJobArgs]) time.Time {
	shift := job.Attempt - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 10 {
		shift = 10
	}
	backoff := time.Duration(1<<shift) * time.Minute
	if backoff > 1*time.Hour {
		backoff = 1 * time.Hour
	}
	return time.Now().Add(backoff)
}

func (w *PreloadedChangesArchivalSweepWorker) Work(ctx context.Context, job *river.Job[PreloadedChangesArchivalSweepJobArgs]) error {
	retentionDays := job.Args.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}

	batchRunID := fmt.Sprintf("batch_%d", time.Now().UnixNano())

	// Step 1: Read all eligible review IDs in ONE single query from DB
	query := `
		SELECT id, org_id
		FROM reviews
		WHERE org_id IS NOT NULL
		  AND created_at < NOW() - make_interval(days => $1)
		  AND metadata ? 'preloaded_changes'
		ORDER BY created_at ASC;
	`
	rows, err := w.db.QueryContext(ctx, query, retentionDays)
	if err != nil {
		log.Error().Err(err).Msg("[preloaded_changes_archival_sweep] failed to query eligible reviews")
		return fmt.Errorf("failed to query eligible reviews for archival: %w", err)
	}
	defer rows.Close()

	var totalEnqueued int
	var chunk []PreloadedChangesArchivalJobArgs

	for rows.Next() {
		var reviewID, orgID int64
		if err := rows.Scan(&reviewID, &orgID); err != nil {
			return fmt.Errorf("failed to scan preloaded_changes_archival row: %w", err)
		}
		chunk = append(chunk, PreloadedChangesArchivalJobArgs{
			ReviewID:   reviewID,
			OrgID:      orgID,
			BatchRunID: batchRunID,
		})

		if len(chunk) >= 10000 {
			if w.jq == nil {
				return fmt.Errorf("job queue reference is nil")
			}
			if _, err := w.jq.QueuePreloadedChangesArchivalJobs(ctx, chunk); err != nil {
				log.Error().Err(err).Msg("[preloaded_changes_archival_sweep] error bulk inserting archival jobs chunk")
				return fmt.Errorf("failed to bulk insert archival jobs chunk: %w", err)
			}
			totalEnqueued += len(chunk)
			chunk = nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows iteration error: %w", err)
	}

	if len(chunk) > 0 {
		if w.jq == nil {
			return fmt.Errorf("job queue reference is nil")
		}
		if _, err := w.jq.QueuePreloadedChangesArchivalJobs(ctx, chunk); err != nil {
			log.Error().Err(err).Msg("[preloaded_changes_archival_sweep] error bulk inserting archival jobs final chunk")
			return fmt.Errorf("failed to bulk insert archival jobs final chunk: %w", err)
		}
		totalEnqueued += len(chunk)
	}

	if totalEnqueued == 0 {
		log.Info().Msg("[preloaded_changes_archival_sweep] 0 eligible reviews found, sweep complete")
		return nil
	}

	// Enqueue the purge coordinator job.
	// It will wait for all upload jobs to complete (up to 6 hours with exponential backoff),
	// then perform a bulk DB metadata purge.
	_, err = w.jq.client.Insert(ctx, PreloadedChangesArchivalPurgeJobArgs{
		BatchRunID:    batchRunID,
		RetentionDays: retentionDays,
	}, &river.InsertOpts{
		MaxAttempts: 100,
	})
	if err != nil {
		log.Error().Err(err).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_archival_sweep] failed to enqueue purge coordinator job")
		return fmt.Errorf("failed to enqueue purge coordinator job: %w", err)
	}

	log.Info().Str("batch_run_id", batchRunID).Int("eligible", totalEnqueued).Int("enqueued", totalEnqueued).Msg("[preloaded_changes_archival_sweep] bulk insert complete, purge coordinator enqueued")
	return nil
}

// PreloadedChangesArchivalWorker handles Step 3: Uploading a SINGLE review diff to Blob Storage independently
// and updating the River job to completed. Does NOT mutate DB metadata row-by-row.
type PreloadedChangesArchivalWorker struct {
	river.WorkerDefaults[PreloadedChangesArchivalJobArgs]
	db *sql.DB
}

func (w *PreloadedChangesArchivalWorker) Timeout(job *river.Job[PreloadedChangesArchivalJobArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *PreloadedChangesArchivalWorker) NextRetry(job *river.Job[PreloadedChangesArchivalJobArgs]) time.Time {
	shift := job.Attempt - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 10 {
		shift = 10
	}
	backoff := time.Duration(15*(1<<shift)) * time.Second
	if backoff > 30*time.Minute {
		backoff = 30 * time.Minute
	}
	return time.Now().Add(backoff)
}

func (w *PreloadedChangesArchivalWorker) Work(ctx context.Context, job *river.Job[PreloadedChangesArchivalJobArgs]) error {
	reviewID := job.Args.ReviewID
	orgID := job.Args.OrgID

	// 1. Fetch preloaded_changes for this single review
	var rawDiff []byte
	query := `
		SELECT metadata->'preloaded_changes'
		FROM reviews
		WHERE id = $1
		  AND org_id = $2
		  AND metadata ? 'preloaded_changes';
	`
	err := w.db.QueryRowContext(ctx, query, reviewID, orgID).Scan(&rawDiff)
	if err == sql.ErrNoRows {
		log.Info().Int64("review_id", reviewID).Msg("[preloaded_changes_archival_worker] review already offloaded or missing, skipping")
		return nil
	}
	if err != nil {
		log.Error().Err(err).Int64("review_id", reviewID).Msg("[preloaded_changes_archival_worker] DB query error")
		return fmt.Errorf("failed to fetch preloaded_changes for review %d: %w", reviewID, err)
	}

	if len(rawDiff) == 0 || bytes.Equal(rawDiff, []byte("null")) {
		log.Info().Int64("review_id", reviewID).Msg("[preloaded_changes_archival_worker] empty diff, marking completed")
		return nil
	}

	// 2. Upload payload independently to Blob Storage
	uploadErr := blobstore.SaveArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactPreloadedChanges, rawDiff)
	if uploadErr != nil {
		log.Error().Err(uploadErr).Int64("review_id", reviewID).Msg("[preloaded_changes_archival_worker] blob upload failed")
		
		// If it's a fatal write permission error, cancel the job permanently so it doesn't infinitely retry
		if strings.Contains(strings.ToLower(uploadErr.Error()), "permission denied") {
			return river.JobCancel(fmt.Errorf("fatal permission error on blob storage: %w", uploadErr))
		}
		
		// Otherwise (e.g. rate limit, network drop), return normal error to trigger River's exponential retry
		return fmt.Errorf("failed to upload diff for review %d: %w", reviewID, uploadErr)
	}

	log.Info().Int64("review_id", reviewID).Msg("[preloaded_changes_archival_worker] blob upload succeeded, job completed")
	return nil
}

// PreloadedChangesArchivalPurgeWorker handles Steps 4 & 5:
// 4. If all upload jobs for BatchRunID are completed (or 6 hours have passed), executes ONE single SQL query to delete all preloaded_changes from metadata.
// 5. Clears created jobs from river_job table for this batch.
// ⚠️ ATTENTION FUTURE DEVELOPERS / AI AGENTS ⚠️
// This worker does NOT use `MaxAttempts` hacking or error backoffs.
// It uses River's native `JobSnooze` to check if jobs are pending, and cleanly sleep for 30 minutes.
// If 6 hours pass since the batch started, it checks one last time and cancels if jobs are still pending (no data deleted).
// ⚠️ --------------------------------------- ⚠️
type PreloadedChangesArchivalPurgeWorker struct {
	river.WorkerDefaults[PreloadedChangesArchivalPurgeJobArgs]
	db *sql.DB
	jq *JobQueue
}

func (w *PreloadedChangesArchivalPurgeWorker) Timeout(job *river.Job[PreloadedChangesArchivalPurgeJobArgs]) time.Duration {
	return 10 * time.Minute
}

// We removed the custom NextRetry backoff because this job no longer loops.
// It relies on ScheduledAt to wake up 30 minutes later and runs once.

func (w *PreloadedChangesArchivalPurgeWorker) Work(ctx context.Context, job *river.Job[PreloadedChangesArchivalPurgeJobArgs]) error {
	batchRunID := job.Args.BatchRunID

	// If 6 hours have passed since the Sweep started the batch, check one last time.
	// If jobs are still pending, give up and discard — never force-purge incomplete batches.
	if time.Since(job.CreatedAt) >= 6*time.Hour {
		var pendingCount int
		timeoutQuery := `
			SELECT COUNT(*)
			FROM river_job
			WHERE args @> jsonb_build_object('batch_run_id', $1::text)
			  AND kind = 'preloaded_changes_archival'
			  AND state NOT IN ('completed', 'discarded');
		`
		if err := w.db.QueryRowContext(ctx, timeoutQuery, batchRunID).Scan(&pendingCount); err != nil {
			log.Error().Err(err).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] failed to query pending jobs at 6h timeout")
			return fmt.Errorf("failed to query pending jobs for batch %s: %w", batchRunID, err)
		}
		if pendingCount > 0 {
			log.Warn().Str("batch_run_id", batchRunID).Int("pending", pendingCount).Msg("[preloaded_changes_purge_worker] 6 hours passed with jobs still pending — discarding purge job (no data deleted)")
			return river.JobCancel(fmt.Errorf("batch %s timed out with %d pending upload jobs after 6 hours", batchRunID, pendingCount))
		}
		log.Info().Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] 6 hours passed but all jobs completed — proceeding with purge")
	} else {
		// 1. O(1) check for pending jobs in Postgres
		var pendingCount int
		checkQuery := `
			SELECT COUNT(*)
			FROM river_job
			WHERE args @> jsonb_build_object('batch_run_id', $1::text)
			  AND kind = 'preloaded_changes_archival'
			  AND state NOT IN ('completed', 'discarded');
		`
		err := w.db.QueryRowContext(ctx, checkQuery, batchRunID).Scan(&pendingCount)
		if err != nil {
			log.Error().Err(err).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] failed to query pending jobs")
			return fmt.Errorf("failed to query pending jobs for batch %s: %w", batchRunID, err)
		}

		if pendingCount > 0 {
			// Two-phase exponential backoff within the 6-hour window:
			//   Phase 1 (first ~1h): 1m, 2m, 4m, 8m, 16m, 30m  — catches fast completions quickly
			//   Phase 2 (1h–6h):     1h, 2h                     — avoids excessive DB polling
			//   At 6h:               force-purge (handled above)
			elapsed := time.Since(job.CreatedAt)
			var snoozeDuration time.Duration
			if elapsed < 1*time.Hour {
				// Phase 1: minute-level exponential backoff
				attempt := job.Attempt
				if attempt < 1 {
					attempt = 1
				}
				snoozeDuration = time.Duration(1<<(attempt-1)) * time.Minute
				if snoozeDuration > 30*time.Minute {
					snoozeDuration = 30 * time.Minute
				}
			} else if elapsed < 3*time.Hour {
				// Phase 2: check again in 1 hour
				snoozeDuration = 1 * time.Hour
			} else {
				// Phase 2: check again in 2 hours (final check before 6h cutoff)
				snoozeDuration = 2 * time.Hour
			}
			log.Info().Str("batch_run_id", batchRunID).Int("pending", pendingCount).Str("snooze", snoozeDuration.String()).Str("elapsed", elapsed.Round(time.Second).String()).Msg("[preloaded_changes_purge_worker] jobs still pending, snoozing with exponential backoff...")
			// JobSnooze safely reschedules the job without counting it as a failure,
			// and automatically extends MaxAttempts so it never permanently dies from snoozing.
			return river.JobSnooze(snoozeDuration)
		}
		
		log.Info().Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] ALL upload jobs done! Executing Step 4: single DB metadata purge")
	}

	// 2. O(1) Fetch & Update (Postgres does all the work internally)
	purgeQuery := `
		UPDATE reviews
		SET metadata = metadata - 'preloaded_changes'
		WHERE (id, org_id) IN (
			SELECT (args->>'review_id')::bigint, (args->>'org_id')::bigint
			FROM river_job
			WHERE args @> jsonb_build_object('batch_run_id', $1::text)
			  AND kind = 'preloaded_changes_archival'
			  AND state = 'completed'
		)
		  AND metadata ? 'preloaded_changes';
	`
	res, purgeErr := w.db.ExecContext(ctx, purgeQuery, batchRunID)
	if purgeErr != nil {
		log.Error().Err(purgeErr).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] Step 4 failed: metadata purge error")
		return fmt.Errorf("failed metadata purge for batch %s: %w", batchRunID, purgeErr)
	}
	rowsAffected, _ := res.RowsAffected()
	log.Info().Int64("purged_reviews", rowsAffected).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] Step 4 complete: metadata purged")

	// 3. O(1) Cleanup of river_job so no junk is left
	cleanupQuery := `
		DELETE FROM river_job 
		WHERE args @> jsonb_build_object('batch_run_id', $1::text)
		  AND kind != 'preloaded_changes_archival_purge';
	`
	cleanupRes, cleanupErr := w.db.ExecContext(ctx, cleanupQuery, batchRunID)
	if cleanupErr != nil {
		log.Error().Err(cleanupErr).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] Step 5 failed: job cleanup error")
		return fmt.Errorf("failed job cleanup for batch %s: %w", batchRunID, cleanupErr)
	}
	cleanedJobs, _ := cleanupRes.RowsAffected()
	log.Info().Int64("cleaned_jobs", cleanedJobs).Str("batch_run_id", batchRunID).Msg("[preloaded_changes_purge_worker] Step 5 complete: batch upload jobs cleaned from river_job")

	return nil
}


