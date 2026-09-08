package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/blobstore"
	"github.com/riverqueue/river"
	"github.com/rs/zerolog/log"
)

// DiffArchivalJobArgs represents arguments for offloading diffs to blob storage.
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
		UniqueOpts:  river.UniqueOpts{ByArgs: true},
	}
}

// DiffArchivalWorker handles offloading a single review diff payload from Postgres metadata to Blob Storage.
type DiffArchivalWorker struct {
	river.WorkerDefaults[DiffArchivalJobArgs]
	db   *sql.DB
	pool *pgxpool.Pool
}

func (w *DiffArchivalWorker) Timeout(job *river.Job[DiffArchivalJobArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *DiffArchivalWorker) Work(ctx context.Context, job *river.Job[DiffArchivalJobArgs]) error {
	return ProcessDiffArchivalForReview(ctx, w.db, job.Args.OrgID, job.Args.ReviewID)
}

// ProcessDiffArchivalForReview offloads a single review diff payload from PostgreSQL metadata to Blob Storage and prunes DB metadata.
func ProcessDiffArchivalForReview(ctx context.Context, db *sql.DB, orgID, reviewID int64) error {
	// 1. Fetch preloaded_changes from PostgreSQL reviews table metadata
	var rawDiff []byte
	query := `SELECT metadata->'preloaded_changes' FROM reviews WHERE id = $1 AND org_id = $2 AND metadata ? 'preloaded_changes'`
	err := db.QueryRowContext(ctx, query, reviewID, orgID).Scan(&rawDiff)
	if err != nil {
		if err == sql.ErrNoRows {
			// Already pruned or review removed — nothing to do
			return nil
		}
		log.Error().Err(err).Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] failed to fetch preloaded_changes from DB")
		return fmt.Errorf("failed to fetch preloaded_changes: %w", err)
	}

	if len(rawDiff) == 0 || string(rawDiff) == "null" {
		// Empty payload or null, safely prune
		updateQuery := `UPDATE reviews SET metadata = metadata - 'preloaded_changes' WHERE id = $1 AND org_id = $2 AND metadata ? 'preloaded_changes'`
		_, _ = db.ExecContext(ctx, updateQuery, reviewID, orgID)
		return nil
	}

	// 2. Upload diff JSON bytes to Blob Storage at org/{org_id}/review/{review_id}/artifacts/preloaded_changes.json
	err = blobstore.SaveArtifact(ctx, db, orgID, reviewID, blobstore.ArtifactPreloadedChanges, rawDiff)
	if err != nil {
		log.Error().Err(err).Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] failed to upload diff to blob storage")
		return fmt.Errorf("failed to upload diff to blob storage: %w", err)
	}

	// 3. On confirmed upload success, strip preloaded_changes from PostgreSQL metadata
	updateQuery := `UPDATE reviews SET metadata = metadata - 'preloaded_changes' WHERE id = $1 AND org_id = $2 AND metadata ? 'preloaded_changes'`
	_, err = db.ExecContext(ctx, updateQuery, reviewID, orgID)
	if err != nil {
		log.Error().Err(err).Int64("review_id", reviewID).Int64("org_id", orgID).Msg("[diff_archival_worker] failed to strip preloaded_changes from DB metadata")
		return fmt.Errorf("failed to strip preloaded_changes from metadata: %w", err)
	}

	return nil
}
