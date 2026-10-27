# Fix Plan: Repository PR Sync River Job Bloat & Auth Error Handling

## 1. Problem Overview & Impact

In production, the `river_job` table suffered extreme bloat:
* **Size:** **1,185 MB** (1.4 GB including indexes/TOAST), accounting for **65% of total database storage**.
* **Row Count:** Over **1,125,000 job records** (and even after a 30-day retention cleanup, **804,965 jobs** generated within 30 days).
* **Bloat Column:** `river_job.errors` (`jsonb[]`), storing serialized error stack traces and provider HTTP responses on every retry attempt (~15MB to ~30MB per day).

### Root Cause: Unauthenticated Repo Sync Infinite Loop

1. When a Git provider connection (GitHub, GitLab, Bitbucket, Azure DevOps) has an invalid, revoked, or expired token:
   * `RepoPRSyncWorker.Work()` attempts to fetch pull requests via `providers.ListPullRequests()`.
   * The provider returns HTTP 401 Unauthorized or 403 Forbidden ("Bad credentials", "Invalid token").
   * `RepoPRSyncWorker` calls `markSyncError(repositoryID, syncErr)`, which sets `last_sync_status = 'error'`, but leaves `last_synced_at` untouched (`NULL` or stale).
   * It returns a generic Go `error`, causing River to retry the job **5 times** (`MaxAttempts: 5`). Each attempt records a full error payload into `river_job.errors`.
2. Every 15 minutes, `ReconciliationSweepWorker` runs:
   ```sql
   SELECT id FROM repositories 
   WHERE last_synced_at IS NULL OR last_synced_at < now() - make_interval(secs => $1)
   ```
   * Because `last_synced_at` was never updated, every broken repository qualifies **again on every sweep** (96 times/day).
   * There is no error backoff and no job deduplication (`UniqueOpts`).
   * 1 broken repo $\times$ 96 sweeps/day $\times$ 5 attempts = **480 failed executions/day**.
   * 50 broken repos = **24,000 failed job executions/day** ($\approx$ 720,000 jobs/month).
3. **The Critical Production Incident (River `JobSchedule` Meltdown):**
   * Over a 3-hour window, repositories with GitHub 401 Bad credentials (e.g., repos 100–103) generated over **2,600 `retryable` jobs** and **1,300 `discarded` jobs**.
   * When thousands of retryable jobs became due at the same time, River's internal `JobSchedule` query (which moves jobs from `retryable`/`scheduled` $\to$ `available`) executed a nested-loop anti-join across `river_job` (720k+ rows).
   * Result: The query ran for **over 15 minutes in a single transaction (PID 2470400)**, burning CPU quadratically and locking `river_job`, blocking all other worker processes from executing.
4. In the UI (`/explore/repositories`), the table has a `Sync error` badge, but the tooltip only displays the relative time (`"Never synced"` or `"3 days ago"`), completely omitting `last_sync_error`. Users have no way of knowing why the repository failed to sync.

---

## 2. Solution Architecture

```
┌────────────────────────────────────────────────────────┐
│ 1. ReconciliationSweepWorker (Runs every 15m)          │
│    Queries stale repos with 6-HOUR ERROR BACKOFF       │
│    (Excludes recently errored repos)                   │
└──────────────────────────┬─────────────────────────────┘
                           │ Enqueues with UniqueOpts (ByArgs: true)
                           ▼
┌────────────────────────────────────────────────────────┐
│ 2. RepoPRSyncWorker (Processes repo_pr_sync)            │
│    Detects providers.IsAuthError(err)                  │
└────────────┬─────────────────────────────┬─────────────┘
             │ If Auth Error               │ If Transient Error
             ▼                             ▼
┌──────────────────────────┐  ┌──────────────────────────┐
│ river.JobCancel(err)     │  │ Standard River Retry     │
│ - Cancel immediately     │  │ - Retries with backoff   │
│ - NO 5x retry loop       │  │ - MaxAttempts lowered    │
│ - Record clean message   │  │   from 5 to 3            │
│   in last_sync_error     │  └──────────────────────────┘
└────────────┬─────────────┘
             ▼
┌────────────────────────────────────────────────────────┐
│ 3. User Visibility in UI (/explore/repositories)       │
│    - "Sync error" badge tooltip displays               │
│      readable explanation:                             │
│      "Authentication failed: Git provider token is     │
│       invalid or expired. Update in Settings."         │
└────────────────────────────────────────────────────────┘
```

---

## 3. Implementation Phases

### Phase 1: Provider Typed Error Classification
**Target files:**
- `internal/providers/shared.go`
- `internal/providers/github/pr_list.go`
- `internal/providers/gitlab/http_client.go`
- `internal/providers/gitlab/pr_list.go`

1. Define `AuthError` in `internal/providers/shared.go`:
   ```go
   type AuthError struct {
       Provider   string
       StatusCode int
       Message    string
   }
   func (e *AuthError) Error() string { ... }
   func IsAuthError(err error) bool { ... }
   ```
2. Update provider HTTP handlers:
   - In GitHub `ListPullRequests`: Return `&providers.AuthError` on HTTP 401, 403 (excluding rate limits), or when the response contains `"Bad credentials"`.
   - In GitLab `ListMergeRequests`: Return `&providers.AuthError` on HTTP 401 or 403.
   - In `loadRepositoryConnectorInfo`: If `githubapp.ResolveIfApp` fails with invalid/uninstalled App credentials, wrap as `AuthError`.

### Phase 2: Cancel Auth Failures in `RepoPRSyncWorker`
**Target file:**
- `internal/jobqueue/repo_sync_worker.go`

1. When `err != nil` in `RepoPRSyncWorker.Work`:
   - If `errors.As(err, &rlErr)` $\to$ `return river.JobSnooze(rlErr.RetryAfter)`.
   - If `providers.IsAuthError(err)`:
     - Store a clean, human-readable message in `last_sync_error`:
       `"Authentication failed: Git provider token is invalid, expired, or revoked. Please update connector credentials in Settings."`
     - Call `return river.JobCancel(fmt.Errorf("fatal authentication error for repository %d: %w", args.RepositoryID, err))`.
     - River cancels the job on **attempt 1** without retrying.
   - For transient network/server errors (5xx, timeouts):
     - Record standard error in `last_sync_error`.
     - Return normal error for River retry.

### Phase 3: Error Backoff in `ReconciliationSweepWorker`
**Target file:**
- `internal/jobqueue/repo_sync_worker.go`

Update the reconciliation query to back off on errored repositories:
```sql
SELECT id FROM repositories 
WHERE (
    -- 1. Repositories that have never synced and haven't errored yet
    (last_synced_at IS NULL AND last_sync_status != 'error')
    
    -- 2. Healthy repositories due for periodic sync
    OR (last_sync_status = 'ok' AND last_synced_at < now() - make_interval(secs => $1))
    
    -- 3. Errored repositories: re-attempt at most once every 6 hours
    OR (last_sync_status = 'error' AND updated_at < now() - interval '6 hours')
)
```

### Phase 4: Job Deduplication & MaxAttempts Adjustment
**Target file:**
- `internal/jobqueue/repo_sync_worker.go`

In `QueueRepoPRSyncJob`:
```go
func (jq *JobQueue) QueueRepoPRSyncJob(ctx context.Context, repositoryID int64, initialBackfill bool) error {
	_, err := jq.client.Insert(ctx, RepoPRSyncJobArgs{
		RepositoryID:    repositoryID,
		InitialBackfill: initialBackfill,
	}, &river.InsertOpts{
		Queue:       "repo_sync",
		MaxAttempts: 3, // Lower from 5 to 3 for transient retries
		UniqueOpts: river.UniqueOpts{
			ByArgs:   true,
			ByPeriod: 15 * time.Minute,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to queue repo PR sync job: %w", err)
	}
	return nil
}
```

### Phase 5: Display Error Details in UI
**Target file:**
- `ui/src/pages/Explore/Repositories.tsx`

Update the `sync_status` column cell renderer:
```tsx
cell: ({ row }) => {
  const repo = row.original;
  const badge = syncStatusBadge(repo.last_sync_status);
  const BadgeIcon = badge.icon;
  
  const tooltipContent = repo.last_sync_status === 'error'
    ? (repo.last_sync_error || 'Sync failed: an error occurred during repository sync')
    : (repo.last_synced_at ? `Synced ${formatRelativeTime(repo.last_synced_at)}` : 'Never synced');

  return (
    <div className="flex items-center justify-center">
      <Tooltip content={tooltipContent}>
        <span
          className="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm font-medium w-fit max-w-[120px] truncate cursor-help"
          style={{ backgroundColor: 'transparent', color: badge.color, border: `1px solid ${badge.borderColor}` }}
        >
          <BadgeIcon />
          {badge.label}
        </span>
      </Tooltip>
    </div>
  );
}
```

### Phase 6: River Dead-Job Retention & Backlog Purge
**Target files / scripts:**
- `internal/jobqueue/jobqueue.go`:
  - Change `CancelledJobRetentionPeriod` from `30 * 24 * time.Hour` to `7 * 24 * time.Hour`.
- SQL cleanup script for production:
  ```sql
  DELETE FROM river_job 
  WHERE kind IN ('repo_pr_sync', 'scheduled_review') 
    AND state IN ('cancelled', 'discarded');
  ```

### Phase 7: Scheduled Review Worker Auth Error Cancellation & Deduplication
**Target files:**
- `internal/providers/github/github.go`:
  - Return `&providers.AuthError` on HTTP 401/403 for `GetBranchHeadSHA`, `GetDefaultBranch`, `GetCommitBefore`, and `GetCompareChanges`.
- `internal/jobqueue/scheduled_review_worker.go`:
  - Check `providers.IsAuthError(err)` and empty tokens during review execution.
  - Advance the configuration checkpoint to `nextRunAt` so it does not immediately re-enqueue when the claim window expires.
  - Return `river.JobCancel(err)` to fail immediately on Attempt 1 instead of retrying 5 times with invalid credentials.
- `internal/jobqueue/jobqueue.go`:
  - Add `UniqueOpts: river.UniqueOpts{ByArgs: true, ByPeriod: 10 * time.Minute}` to `QueueScheduledReviewJob`.

---

## 4. Verification & Testing

1. **Unit Tests (`internal/jobqueue/repo_sync_worker_test.go`):**
   * Test that an `AuthError` causes `RepoPRSyncWorker` to return `river.JobCancel` and update `last_sync_status = 'error'`.
   * Test that `ReconciliationSweepWorker` does not select recently errored repositories (respects 6-hour backoff).
   * Test that `QueueRepoPRSyncJob` deduplicates with `UniqueOpts`.
2. **Unit Tests (`internal/jobqueue/scheduled_review_worker_test.go`):**
   * Test that `ScheduledReviewWorker` immediately cancels (`river.JobCancel`) on provider authentication errors.
   * Test that missing/empty tokens cancel immediately.
   * Test that `next_run_at` advances to avoid 10-minute re-queue loops.
3. **Frontend Typecheck:**
   * Run `npx tsc --noEmit` in `ui/` to confirm zero type errors.
4. **Go Test Suite:**
   * Run `go test ./internal/jobqueue/...` to verify all worker tests pass.
