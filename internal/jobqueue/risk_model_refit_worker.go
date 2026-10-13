package jobqueue

import (
	"context"
	"database/sql"

	"github.com/livereview/internal/riskscore"
	"github.com/riverqueue/river"
)

// RiskModelRefitJobArgs runs the daily stabilizing batch refit over every org
// that has labeled outcomes (docs/risk-policy-engine.md §3, path 2).
type RiskModelRefitJobArgs struct{}

func (RiskModelRefitJobArgs) Kind() string { return "risk_model_refit" }

type RiskModelRefitWorker struct {
	river.WorkerDefaults[RiskModelRefitJobArgs]
	db *sql.DB
}

func (w *RiskModelRefitWorker) Work(ctx context.Context, job *river.Job[RiskModelRefitJobArgs]) error {
	rows, err := w.db.QueryContext(ctx, `SELECT DISTINCT org_id FROM risk_feature_snapshots`)
	if err != nil {
		return err
	}
	defer rows.Close()

	store := riskscore.NewStore(w.db)
	for rows.Next() {
		var orgID int64
		if err := rows.Scan(&orgID); err != nil {
			return err
		}
		// Best-effort per org: one org failing shouldn't block the sweep.
		if _, err := store.Refit(ctx, orgID); err != nil {
			// log and continue
			_ = err
		}
	}
	return rows.Err()
}
