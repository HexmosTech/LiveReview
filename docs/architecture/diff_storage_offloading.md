# Database Storage Optimization and Diff Storage Offloading Architecture

## Problem

### Problem 1: Code Diff Storage Bloat in reviews.metadata

PostgreSQL stores review records in the reviews table. The metadata column of the reviews table contains a JSON field named preloaded_changes. The preloaded_changes field stores raw Git code diff strings. The preloaded_changes field used 791.30 megabytes in PostgreSQL. This field used 90.4 percent of the metadata column storage. Unchecked accumulation of raw source code diffs in preloaded_changes creates PostgreSQL TOAST table bloat. It increases database backup dump sizes by 150 megabytes. It degrades database query performance.

### Problem 2: Background Job Record Accumulation in river_job

Completed background job records in the river_job table accumulated over time. They consumed 263.5 megabytes in PostgreSQL. They added 79.09 megabytes to database backup files because the job queue had a 365 day retention configuration.

```mermaid
flowchart TD
    A["Review Creation & Job Execution"] --> B["Problem 1: Raw diffs in reviews.metadata JSONB (791 MB / 90.4%)"]
    A --> C["Problem 2: River job retention for 365 days (263 MB)"]
    B --> D["PostgreSQL TOAST Table Bloat (791 MB)"]
    C --> E["River Job Table Bloat (263 MB)"]
    D --> F["Large Database Backups & High I/O"]
    E --> F
```

## Solution

The system combines database reads for recent reviews with low PostgreSQL disk usage. The system uses a 30 day database retention window and an automated background offloading cron job.

### Key Principles

1. Newly generated code diffs remain stored in PostgreSQL for the first 30 days. This ensures that recent code reviews get high read speed and low query latency.
2. A background scheduled cron manager named PreloadedChangesArchivalManager periodically scans PostgreSQL. It finds reviews older than 30 days that still hold preloaded_changes in database metadata.
3. For reviews older than 30 days, the cron job uploads the raw code diffs to Blob Storage under the key path org/org_id/review/review_id/artifacts/preloaded_changes.json. When the upload succeeds, the job removes preloaded_changes from reviews.metadata in PostgreSQL.
4. The background manager operates with controlled batch sizes of 50 reviews per batch. It pauses for 50 milliseconds between batches. It uses streaming JSON serialization. This prevents CPU spikes, memory bloat, and database connection pool exhaustion.
5. The API server inspects PostgreSQL metadata first for active diffs. If preloaded_changes is absent from metadata, the server reads the diff payload from Blob Storage.

## Component Architecture

The architecture contains five primary components.

1. Review Worker Creation
2. Automated Background Offloading Cron
3. Blob Storage Transfer and PostgreSQL Pruning
4. API Handler Fallback Strategy
5. River Job Queue Retention Strategy

### Review Worker Creation

When DiffReviewWorker executes a review job, it writes raw code diffs to the metadata column of the reviews table in PostgreSQL. During the first 30 days, the system does not make external storage calls when users display reviews in the Web UI or CLI.

```mermaid
flowchart TD
    A["Diff Review Job Queue"] --> B["DiffReviewWorker Process"]
    B --> C["Parse CodeDiff Payload"]
    C --> D["Store preloaded_changes in reviews.metadata JSONB"]
    D --> E["Save Review to PostgreSQL"]
    E --> F["Fast DB Reads for 30 Days"]
```

### Automated Background Offloading Cron

The PreloadedChangesArchivalManager background scheduler runs at scheduled cron intervals. The default schedule runs daily at 2:30 AM IST.

The system loads configuration from system_settings (`preloaded_changes_archival_settings`). You can update configuration without restarting the server.

The enabled configuration option enables or disables the offloading background manager.
The cron_expression configuration option sets the schedule for running offloading cycles.
The retention_days configuration option sets the number of days to retain diffs in PostgreSQL metadata before offloading.
The batch_size configuration option sets the number of reviews processed in each database query batch.
The inter_batch_delay_ms configuration option sets the pause duration between batches to maintain a low resource footprint.

### Blob Storage Transfer and PostgreSQL Pruning Workflow

The offloading cycle runs four steps.

First, query PostgreSQL for reviews created more than 30 days ago that still have preloaded_changes in metadata.

```sql
SELECT id, org_id, metadata->'preloaded_changes'
FROM reviews
WHERE created_at < NOW() - ($1 * INTERVAL '1 day')
  AND metadata ? 'preloaded_changes'
ORDER BY created_at ASC
LIMIT $2;
```

Second, for each review in the batch, write the preloaded_changes payload to Blob Storage under the key path org/org_id/review/review_id/artifacts/preloaded_changes.json.

Third, when the upload succeeds, remove the preloaded_changes key from PostgreSQL metadata.

```sql
UPDATE reviews
SET metadata = metadata - 'preloaded_changes'
WHERE id = $1 AND org_id = $2;
```

Fourth, if the Blob Storage upload fails for any review, the system keeps metadata unchanged in PostgreSQL. The system logs the error and retries the upload during the next scheduled cycle.

```mermaid
flowchart TD
    A["PreloadedChangesArchivalManager Cron Trigger"] --> B{"Is Offloading Enabled?"}
    B -- "No" --> C["Skip Offloading Cycle"]
    B -- "Yes" --> D["Query Reviews older than 30 Days with preloaded_changes in DB"]
    D --> E{"Eligible Reviews Found?"}
    E -- "No" --> F["Cycle Complete"]
    E -- "Yes" --> G["Fetch Batch of 50 Reviews"]
    G --> H["Upload preloaded_changes to Blob Storage org/org_id/review/review_id/artifacts/preloaded_changes.json"]
    H --> I{"Upload Successful?"}
    I -- "Yes" --> J["Prune: UPDATE reviews SET metadata = metadata - 'preloaded_changes'"]
    I -- "No" --> K["Log Error & Retain in DB Metadata for Next Cycle"]
    J --> L["Pause 50ms Between Batches"]
    K --> L
    L --> G
```

### API Handler Fallback Strategy

When a client requests review details, the server processes three steps.

First, make sure that the request has a valid organization context.

Second, inspect preloaded_changes inside PostgreSQL reviews.metadata. If preloaded_changes is present in metadata, return the diff payload immediately from PostgreSQL.

Third, if preloaded_changes is absent from metadata, read the diff payload from Blob Storage at key path org/org_id/review/review_id/artifacts/preloaded_changes.json. If the object exists in Blob Storage, return the diff payload to the client. If the object does not exist, return an empty diff payload.

```mermaid
flowchart TD
    A["Client GET /api/v1/diff-review/:id"] --> B["Resolve org_id & Fetch Review from PostgreSQL"]
    B --> C{"Is preloaded_changes present in DB metadata?"}
    C -- "Yes (Review <= 30 Days)" --> D["Return CodeDiff payload immediately from PostgreSQL"]
    C -- "No (Review > 30 Days)" --> E["Read Artifact from Blob Storage org/org_id/review/review_id/artifacts/preloaded_changes.json"]
    E --> F{"Object Exists in Blob Storage?"}
    F -- "Yes" --> G["Return CodeDiff payload from Blob Storage"]
    F -- "No" --> H["Return Empty Diff Payload"]
```

### River Job Queue Retention Strategy

The background job queue system configures River job auto-cleaning to purge historical job records from PostgreSQL after 30 days. The queue initialization configures CompletedJobRetentionPeriod, CancelledJobRetentionPeriod, and DiscardedJobRetentionPeriod to 30 days in internal/jobqueue/jobqueue.go. This policy purges completed, cancelled, and discarded job records automatically. It keeps river_job table storage below one megabyte.

## Resource Footprint and Optimization Strategy

You must keep system resource usage low during offloading.

1. Query and process reviews in chunks of 50 to prevent large memory allocations.
2. Pause for 50 milliseconds between batches to allow PostgreSQL to process application queries.
3. Convert diffs to JSON bytes and stream them to storage backends.
4. Use atomic execution guards to prevent overlapping cron runs.
5. Run periodic PostgreSQL vacuum jobs to compact TOAST space freed by the removal of preloaded_changes.

## Security and Organization Isolation

All Blob Storage artifact keys include the organization identifier org_id. The API handler verifies permissions before reading from Blob Storage or PostgreSQL metadata. If a user requests diff artifacts from another organization, the API server returns an HTTP 404 response.

---

## Architecture: 5-Step River-based Archival Flow

The diff offloading system executes via an automated, distributed 5-step pipeline managed by River job queue. Both scheduled periodic sweeps and manual UI triggers execute the exact same 5-step flow.

### Step 1: Read Eligible Review IDs in 1 Single Query
When `PreloadedChangesArchivalSweepWorker` runs (triggered periodically by River or manually via `TriggerManualCycle()`), it executes a single query to discover all review records eligible for offloading (`created_at < NOW() - retentionDays` and containing `preloaded_changes` in metadata):

```sql
SELECT id, COALESCE(org_id, 0)
FROM reviews
WHERE created_at < NOW() - ($1 * INTERVAL '1 day')
  AND trigger_type = 'cli_diff'
  AND metadata ? 'preloaded_changes'
ORDER BY created_at ASC;
```

### Step 2: Bulk Insert with Exponential Retry Mechanism
The sweep worker generates a unique `batch_run_id` (e.g. `batch_<timestamp>`), creates `PreloadedChangesArchivalJobArgs` for each review ID, and bulk inserts them into River queue using `jq.client.InsertMany()`:
- Each upload job includes exponential retry backoff (`NextRetry` up to 10 attempts).
- The worker also enqueues 1 `PreloadedChangesArchivalPurgeJobArgs` coordinator job for the batch.

### Step 3: Independent Upload & River Completion
River worker pool processes `PreloadedChangesArchivalWorker` jobs in parallel across workers:
- Each worker fetches `metadata->'preloaded_changes'` for its single `ReviewID`.
- Uploads the diff payload to Blob Storage at `org/<org_id>/review/<review_id>/artifacts/preloaded_changes.json`.
- **No DB write** during upload — on success, the worker returns `nil` and River marks the individual job as `completed`.
- If an upload fails, River retries only that specific failed job using exponential backoff.

### Step 4: Single-Query Metadata Purge on Full Completion
`PreloadedChangesArchivalPurgeWorker` monitors `river_job` for `batch_run_id`:
- Waits while any upload jobs for `batch_run_id` remain pending or retrying.
- Once **ALL** upload jobs in the batch reach `completed` state, it executes **ONE single SQL UPDATE** query to strip `preloaded_changes` from PostgreSQL metadata:

```sql
UPDATE reviews
SET metadata = metadata - 'preloaded_changes'
WHERE created_at < NOW() - ($1 * INTERVAL '1 day')
  AND trigger_type = 'cli_diff'
  AND metadata ? 'preloaded_changes';
```

### Step 5: Job Cleanup
After the single-query metadata purge succeeds, the purge worker clears all job records created for that batch from the database:

```sql
DELETE FROM river_job
WHERE args->>'batch_run_id' = $1;
```

```mermaid
flowchart TD
    A["Periodic Schedule or Manual Trigger"] --> B["Step 1: SELECT all eligible review IDs in 1 query"]
    B --> C["Step 2: InsertMany all upload jobs with batch_run_id + 1 Purge Job into River Queue"]
    C --> D["River Worker Pool (Parallel execution)"]

    D --> E1["Worker 1"]
    D --> E2["Worker 2"]
    D --> E3["Worker N"]

    E1 --> F1["Step 3: Upload Review 1 diff to Blob Storage (Independent)"]
    E2 --> F2["Step 3: Upload Review 2 diff to Blob Storage (Independent)"]
    E3 --> F3["Step 3: Upload Review N diff to Blob Storage (Independent)"]

    F1 --> G1{"Upload Success?"}
    G1 -- "Yes" --> H1["Mark job completed in River"]
    G1 -- "No" --> I1["Exponential retry ONLY Review 1"]

    H1 --> P["PreloadedChangesArchivalPurgeWorker"]
    P --> Q{"All jobs in batch completed?"}
    Q -- "No" --> R["Retry purge check in 15s"]
    Q -- "Yes" --> S["Step 4: ONE Single SQL UPDATE to delete preloaded_changes from DB metadata"]
    S --> T["Step 5: DELETE FROM river_job WHERE args->>'batch_run_id' = batch_id"]
```

#### Safety & Performance Guarantees

* **Unified Pipeline:** Manual triggers ("Run Now") and scheduled periodic sweeps run through the exact same 5-step flow.
* **Granular Retries:** Upload failures on individual review diffs retry independently with exponential backoff without blocking or rolling back other reviews.
* **1 Bulk Database Write:** PostgreSQL metadata is purged in **1 single SQL UPDATE** after all archival uploads finish.
* **Zero Job Accumulation:** Completed batch jobs in `river_job` are automatically purged upon batch completion.


