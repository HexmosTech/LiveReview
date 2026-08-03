package jobqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/database"
	"github.com/riverqueue/river"
)

// testDBAndURL returns a live *sql.DB plus the raw connection string River's
// pgxpool needs. Unlike internal/database.NewDB() (which also accepts a
// .env-file fallback), NewJobQueue's pgxpool.New requires the URL as a plain
// string, so this test helper requires DATABASE_URL to be exported directly.
func testDBAndURL(t *testing.T) (*sql.DB, string) {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("skipping: DATABASE_URL must be exported for River client construction")
	}
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dbURL
}

// TestNewJobQueue_StartsWithPeriodicJobs is a smoke test that river.Config
// with PeriodicJobs configured (the first use of this River feature in the
// codebase) constructs successfully - i.e. river.NewClient doesn't reject the
// PeriodicJob definition (duplicate ID, bad schedule, etc). It does not call
// Start(), so no periodic job actually fires during the test.
func TestNewJobQueue_StartsWithPeriodicJobs(t *testing.T) {
	db, dbURL := testDBAndURL(t)

	jq, err := NewJobQueue(dbURL, db)
	if err != nil {
		t.Fatalf("NewJobQueue with PeriodicJobs configured: %v", err)
	}
	defer jq.pool.Close()
}

func TestReconciliationSweepWorker_OnlyQueuesStaleRepositories(t *testing.T) {
	db, dbURL := testDBAndURL(t)

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	jq, err := NewJobQueue(dbURL, db)
	if err != nil {
		t.Fatalf("NewJobQueue: %v", err)
	}
	defer jq.pool.Close()

	var connectorID int64
	err = db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id)
		VALUES ('github', 'test-app', 'na', 'reconciliation-test', 'https://example.test', 1)
		RETURNING id
	`).Scan(&connectorID)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, connectorID) })

	insertRepo := func(providerRepoID string, lastSyncedAt interface{}) int64 {
		var id int64
		err := db.QueryRow(`
			INSERT INTO repositories (org_id, connector_id, provider, provider_repo_id, full_name, name, web_url, last_synced_at)
			VALUES (1, $1, 'github', $2, 'acme/'||$2, $2, 'https://github.com/acme/'||$2, $3)
			RETURNING id
		`, connectorID, providerRepoID, lastSyncedAt).Scan(&id)
		if err != nil {
			t.Fatalf("insert repository %s: %v", providerRepoID, err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, id) })
		return id
	}

	neverSynced := insertRepo("never-synced", nil)
	staleRepo := insertRepo("stale", time.Now().Add(-1*time.Hour))
	freshRepo := insertRepo("fresh", time.Now().Add(-1*time.Minute))
	_ = freshRepo

	worker := &ReconciliationSweepWorker{
		db:                 db,
		pool:               pool,
		stalenessThreshold: 20 * time.Minute,
		jq:                 jq,
	}

	job := &river.Job[ReconciliationSweepJobArgs]{}
	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work: %v", err)
	}

	queuedRepoIDs := map[int64]bool{}
	rows, err := db.Query(`SELECT args FROM river_job WHERE kind = 'repo_pr_sync' ORDER BY id DESC LIMIT 20`)
	if err != nil {
		t.Fatalf("query river_job: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var argsJSON []byte
		if err := rows.Scan(&argsJSON); err != nil {
			t.Fatalf("scan: %v", err)
		}
		var args RepoPRSyncJobArgs
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			t.Fatalf("unmarshal args: %v", err)
		}
		queuedRepoIDs[args.RepositoryID] = true
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM river_job WHERE kind = 'repo_pr_sync' AND args->>'repository_id' IN ($1,$2,$3)`,
			neverSynced, staleRepo, freshRepo)
	})

	if !queuedRepoIDs[neverSynced] {
		t.Errorf("expected never-synced repository %d to be queued for sync", neverSynced)
	}
	if !queuedRepoIDs[staleRepo] {
		t.Errorf("expected stale repository %d to be queued for sync", staleRepo)
	}
	if queuedRepoIDs[freshRepo] {
		t.Errorf("expected freshly-synced repository %d to NOT be queued for sync", freshRepo)
	}
}
