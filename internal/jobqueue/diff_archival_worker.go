package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/blobstore"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/rs/zerolog/log"
)

const stuckJobThreshold = 6 * time.Hour

// DiffArchivalJobArgs represents arguments for offloading a single review diff to blob storage.
// 1 River job = 1 review ID. Retry isolation is clean: if upload fails, only this review retries.
type DiffArchivalJobArgs struct {
	ReviewID int64 `json:"review_id"`
	OrgID    int64 `json:"org_id"`
}

func (DiffArchivalJobArgs) Kind() string {
	return "diff_archival"
}

func (DiffArchivalJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "diff_archival",
		MaxAttempts: 10,
	}
}

// DiffArchivalPurgeJobArgs represents arguments for the purge job that fires after all
// archival jobs in a batch run complete. It executes ONE bulk UPDATE to remove
// preloaded_changes from PostgreSQL metadata for all successfully uploaded reviews.
type DiffArchivalPurgeJobArgs struct {
	BatchRunID string  `json:"batch_run_id"`
	ReviewIDs  []int64 `json:"review_ids"`
	OrgID      int64   `json:"org_id"`
}

func (DiffArchivalPurgeJobArgs) Kind() string {
	return "diff_archival_purge"
}

func (DiffArchivalPurgeJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "diff_archival",
		MaxAttempts: 100,
	}
}

// DiffArchivalWorker handles offloading a SINGLE review diff payload from Postgres metadata to Blob Storage.
// It does NOT update reviews metadata — that is deferred to DiffArchivalPurgeWorker via one bulk UPDATE.
// Parallelism comes from River's MaxWorkers running multiple workers concurrently (consumer-side).
type DiffArchivalWorker struct {
	river.WorkerDefaults[DiffArchivalJobArgs]
	db   *sql.DB
	pool *pgxpool.Pool
}

func (w *DiffArchivalWorker) Timeout(job *river.Job[DiffArchivalJobArgs]) time.Duration {
	return 5 * time.Minute
}

func (w *DiffArchivalWorker) Work(ctx context.Context, job *river.Job[DiffArchivalJobArgs]) error {
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
		// Already offloaded or does not exist — treat as success, nothing to do.
		log.Info().Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] review already offloaded or not found, skipping")
		return nil
	}
	if err != nil {
		log.Error().Err(err).Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] failed to fetch preloaded_changes from DB")
		return fmt.Errorf("failed to fetch preloaded_changes for review %d: %w", reviewID, err)
	}

	if len(rawDiff) == 0 || string(rawDiff) == "null" {
		log.Info().Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] empty diff, skipping upload")
		return nil
	}

	// 2. Upload to Blob Storage. No DB UPDATE here — DiffArchivalPurgeWorker does the bulk UPDATE.
	uploadErr := blobstore.SaveArtifact(ctx, w.db, orgID, reviewID, blobstore.ArtifactPreloadedChanges, rawDiff)
	if uploadErr != nil {
		log.Error().Err(uploadErr).Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] blob upload failed")
		return fmt.Errorf("failed to upload diff for review %d: %w", reviewID, uploadErr)
	}

	log.Info().Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] blob upload succeeded")
	return nil
}

// DiffArchivalPurgeWorker waits for all archival jobs in a batch run to complete,
// then executes ONE bulk UPDATE to remove preloaded_changes from PostgreSQL metadata.
// It retries every 5 minutes until all archival jobs are done (or stuck > 6h).
type DiffArchivalPurgeWorker struct {
	river.WorkerDefaults[DiffArchivalPurgeJobArgs]
	db     *sql.DB
	client *river.Client[pgx.Tx]
}

func (w *DiffArchivalPurgeWorker) Timeout(job *river.Job[DiffArchivalPurgeJobArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *DiffArchivalPurgeWorker) NextRetry(job *river.Job[DiffArchivalPurgeJobArgs]) time.Time {
	return time.Now().Add(5 * time.Minute)
}

func (w *DiffArchivalPurgeWorker) Work(ctx context.Context, job *river.Job[DiffArchivalPurgeJobArgs]) error {
	if w.client == nil {
		log.Error().Msg("[diff_archival_purge] river client is nil, cannot query job status")
		return fmt.Errorf("diff_archival_purge worker: river client is nil")
	}

	targetMap := make(map[int64]bool, len(job.Args.ReviewIDs))
	for _, id := range job.Args.ReviewIDs {
		targetMap[id] = true
	}

	// 1. Check for active (non-finalized) diff_archival jobs in this batch with full pagination
	activeStates := []rivertype.JobState{
		rivertype.JobStateAvailable,
		rivertype.JobStateRunning,
		rivertype.JobStateRetryable,
		rivertype.JobStateScheduled,
	}

	var stillActive int
	var stuckCount int

	for _, state := range activeStates {
		var lastCursor *river.JobListCursor
		for {
			params := river.NewJobListParams().
				Kinds("diff_archival").
				States(state).
				First(1000)
			if lastCursor != nil {
				params = params.After(lastCursor)
			}
			result, err := w.client.JobList(ctx, params)
			if err != nil {
				return fmt.Errorf("JobList(%s) error: %w", state, err)
			}
			if len(result.Jobs) == 0 {
				break
			}
			for _, j := range result.Jobs {
				lastCursor = river.JobListCursorFromJob(j)
				var args DiffArchivalJobArgs
				if err := json.Unmarshal(j.EncodedArgs, &args); err != nil {
					continue
				}
				if args.OrgID != job.Args.OrgID {
					continue
				}
				if !targetMap[args.ReviewID] {
					continue
				}
				if time.Since(j.CreatedAt) > stuckJobThreshold {
					stuckCount++
				} else {
					stillActive++
				}
			}
			if len(result.Jobs) < 1000 {
				break
			}
		}
	}

	if stillActive > 0 {
		log.Info().Int("still_active", stillActive).Int("stuck", stuckCount).Str("batch_run_id", job.Args.BatchRunID).Msg("[diff_archival_purge] active jobs remaining, retrying in 5m")
		return fmt.Errorf("%d batch archival jobs still active", stillActive)
	}

	// 2. Collect completed review IDs from River with full pagination
	completedReviewIDs := make(map[int64]bool)
	var lastCursor *river.JobListCursor
	for {
		params := river.NewJobListParams().
			Kinds("diff_archival").
			States(rivertype.JobStateCompleted).
			First(1000)
		if lastCursor != nil {
			params = params.After(lastCursor)
		}
		result, err := w.client.JobList(ctx, params)
		if err != nil {
			return fmt.Errorf("JobList(completed) error: %w", err)
		}
		if len(result.Jobs) == 0 {
			break
		}
		for _, j := range result.Jobs {
			lastCursor = river.JobListCursorFromJob(j)
			var args DiffArchivalJobArgs
			if err := json.Unmarshal(j.EncodedArgs, &args); err != nil {
				continue
			}
			if args.OrgID != job.Args.OrgID {
				continue
			}
			if targetMap[args.ReviewID] {
				completedReviewIDs[args.ReviewID] = true
			}
		}
		if len(result.Jobs) < 1000 {
			break
		}
	}

	purgeIDs := make([]int64, 0, len(completedReviewIDs))
	for rID := range completedReviewIDs {
		purgeIDs = append(purgeIDs, rID)
	}

	// Safe behavior: if 0 jobs completed, do NOT purge un-archived reviews from PostgreSQL
	if len(purgeIDs) == 0 {
		log.Info().Str("batch_run_id", job.Args.BatchRunID).Msg("[diff_archival_purge] 0 completed review diffs to purge, skipping DB UPDATE")
		return nil
	}

	// 3. ONE bulk UPDATE — remove preloaded_changes from Postgres for all uploaded reviews
	updateQuery := `
		UPDATE reviews
		SET metadata = metadata - 'preloaded_changes'
		WHERE id = ANY($1::bigint[])
		  AND org_id = $2
		  AND metadata ? 'preloaded_changes';
	`
	res, err := w.db.ExecContext(ctx, updateQuery, purgeIDs, job.Args.OrgID)
	if err != nil {
		log.Error().Err(err).Int64("org_id", job.Args.OrgID).Int("count", len(purgeIDs)).Msg("[diff_archival_purge] failed bulk UPDATE reviews metadata")
		return fmt.Errorf("failed bulk UPDATE reviews metadata: %w", err)
	}

	rowsAffected, _ := res.RowsAffected()
	log.Info().
		Int64("rows_affected", rowsAffected).
		Int("purge_count", len(purgeIDs)).
		Int("stuck_skipped", stuckCount).
		Str("batch_run_id", job.Args.BatchRunID).
		Msg("[diff_archival_purge] bulk metadata purge complete")

	return nil
}
