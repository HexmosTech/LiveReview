package seed_demo

import (
	"context"
	"database/sql"

	"github.com/riverqueue/river"
)

// JobArgs is the River job payload for one scheduled seed-demo-activity run.
type JobArgs struct {
	OrgID int64 `json:"org_id"`
}

func (JobArgs) Kind() string { return "seed_demo_activity" }

func (JobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       "seed_demo_activity",
		MaxAttempts: 3,
	}
}

// Worker runs Run() for the job's OrgID once per scheduled tick. Registered as a River
// periodic job (see internal/jobqueue/jobqueue.go), gated on LIVEREVIEW_IS_CLOUD=true
// and SEED_DEMO_ENABLED=true, always targeting the hardcoded DemoOrgID.
type Worker struct {
	river.WorkerDefaults[JobArgs]
	DB *sql.DB
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	count, err := DefaultCount()
	if err != nil {
		return err
	}
	return Run(ctx, w.DB, job.Args.OrgID, count)
}
