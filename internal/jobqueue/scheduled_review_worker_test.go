package jobqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/livereview/internal/providers"
	"github.com/riverqueue/river"
)

func TestScheduledReviewWorker_MissingToken_CancelsJob(t *testing.T) {
	db, _ := testDBAndURL(t)

	var connectorID int64
	err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, pat_token, connection_name, provider_url, org_id)
		VALUES ('github', 'test-app', '', '', 'sched-missing-token-test', 'https://github.com', 1)
		RETURNING id
	`).Scan(&connectorID)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, connectorID) })

	var repoID int64
	err = db.QueryRow(`
		INSERT INTO repositories (org_id, connector_id, provider, provider_repo_id, full_name, name, web_url, last_sync_status)
		VALUES (1, $1, 'github', 'test-missing-token-repo', 'acme/sched-missing-token', 'sched-missing-token', 'https://github.com/acme/sched-missing-token', 'ok')
		RETURNING id
	`, connectorID).Scan(&repoID)
	if err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, repoID) })

	var configID int64
	err = db.QueryRow(`
		INSERT INTO scheduled_review_configs (org_id, repository_id, enabled, cron_expression, default_branch, next_run_at)
		VALUES (1, $1, true, '0 9 * * *', 'main', NOW() - INTERVAL '1 hour')
		RETURNING id
	`, repoID).Scan(&configID)
	if err != nil {
		t.Fatalf("insert scheduled review config: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM scheduled_review_configs WHERE id = $1`, configID) })

	worker := &ScheduledReviewWorker{db: db}
	job := &river.Job[ScheduledReviewJobArgs]{
		Args: ScheduledReviewJobArgs{ConfigID: configID},
	}

	workErr := worker.Work(context.Background(), job)
	if workErr == nil {
		t.Fatal("expected Work to return an error, got nil")
	}

	var cancelErr *river.JobCancelError
	if !errors.As(workErr, &cancelErr) {
		t.Errorf("expected error to wrap river.JobCancel, got %T: %v", workErr, workErr)
	}

	var nextRunAt time.Time
	err = db.QueryRow(`SELECT next_run_at FROM scheduled_review_configs WHERE id = $1`, configID).Scan(&nextRunAt)
	if err != nil {
		t.Fatalf("query next_run_at: %v", err)
	}
	if !nextRunAt.After(time.Now()) {
		t.Errorf("expected next_run_at to be in the future, got %v", nextRunAt)
	}
}

func TestScheduledReviewWorker_AuthError_CancelsJob(t *testing.T) {
	db, _ := testDBAndURL(t)

	var connectorID int64
	err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, pat_token, connection_name, provider_url, org_id)
		VALUES ('github', 'test-app', '', 'ghp_invalid_dummy_token_for_test', 'sched-auth-fail-test', 'https://github.com', 1)
		RETURNING id
	`).Scan(&connectorID)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, connectorID) })

	var repoID int64
	err = db.QueryRow(`
		INSERT INTO repositories (org_id, connector_id, provider, provider_repo_id, full_name, name, web_url, last_sync_status)
		VALUES (1, $1, 'github', 'test-auth-fail-repo', 'acme/sched-auth-fail', 'sched-auth-fail', 'https://github.com/acme/sched-auth-fail', 'ok')
		RETURNING id
	`, connectorID).Scan(&repoID)
	if err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, repoID) })

	var configID int64
	err = db.QueryRow(`
		INSERT INTO scheduled_review_configs (org_id, repository_id, enabled, cron_expression, default_branch, next_run_at)
		VALUES (1, $1, true, '0 9 * * *', 'main', NOW() - INTERVAL '1 hour')
		RETURNING id
	`, repoID).Scan(&configID)
	if err != nil {
		t.Fatalf("insert scheduled review config: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM scheduled_review_configs WHERE id = $1`, configID) })

	worker := &ScheduledReviewWorker{db: db}
	job := &river.Job[ScheduledReviewJobArgs]{
		Args: ScheduledReviewJobArgs{ConfigID: configID},
	}

	workErr := worker.Work(context.Background(), job)
	if workErr == nil {
		t.Fatal("expected Work to return an error, got nil")
	}

	var cancelErr *river.JobCancelError
	if !errors.As(workErr, &cancelErr) {
		t.Errorf("expected error to wrap river.JobCancel, got %T: %v", workErr, workErr)
	}

	var outcome, errMsg string
	err = db.QueryRow(`SELECT outcome, error_message FROM scheduled_review_runs WHERE config_id = $1 ORDER BY id DESC LIMIT 1`, configID).
		Scan(&outcome, &errMsg)
	if err != nil {
		t.Fatalf("query scheduled_review_runs: %v", err)
	}
	if outcome != "failed" {
		t.Errorf("expected run outcome to be 'failed', got %q", outcome)
	}
	if !providers.IsAuthError(errors.New(errMsg)) {
		t.Errorf("expected error message to be recognized as an auth failure, got %q", errMsg)
	}

	var nextRunAt time.Time
	err = db.QueryRow(`SELECT next_run_at FROM scheduled_review_configs WHERE id = $1`, configID).Scan(&nextRunAt)
	if err != nil {
		t.Fatalf("query next_run_at: %v", err)
	}
	if !nextRunAt.After(time.Now()) {
		t.Errorf("expected next_run_at to be in the future, got %v", nextRunAt)
	}
}
