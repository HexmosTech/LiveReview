# LiveReview Background Worker & River Queue Guide

This guide describes the architecture, operation, configuration, and monitoring of the **LiveReview Background Worker** which processes automated code reviews asynchronously.

---

## 1. Architectural Overview

LiveReview offloads heavy tasks (like processing code reviews via LLM providers and posting feedback to GitLab/GitHub) to a background worker to keep the API server responsive.

The job execution framework is powered by **River**, a high-performance Go job queue library built on PostgreSQL.

```
┌──────────────────┐               ┌───────────────┐               ┌────────────────────┐
│                  │  Webhook Event│               │ Enqueue Job   │                    │
│   GitLab / Git   ├──────────────>│ LiveReview API├──────────────>│ PostgreSQL DB      │
│                  │               │               │               │ (river_job table)  │
└──────────────────┘               └───────────────┘               └─────────┬──────────┘
                                                                             │
                                                                             │ Listen / Poll
                                                                             ▼
┌──────────────────┐               ┌───────────────┐               ┌────────────────────┐
│                  │  Post Review  │  Background   │ Run Review    │  Background        │
│   GitLab / Git   │<──────────────┤  AI Provider  │<──────────────┤  Worker Process    │
│                  │               │               │               │ (livereview worker)│
└──────────────────┘               └───────────────┘               └────────────────────┘
```

---

## 2. Queues & Concurrency

LiveReview segments background jobs into two distinct queues:

1. **`default` Queue**: Used for auxiliary/operational tasks (such as installing and registering project webhooks).
2. **`review` Queue**: Used strictly for executing AI code reviews.

### Concurrency Configuration
Concurrency limits can be tuned per queue to protect CPU, memory, and database connections:
* **Default Concurrency**: The `default` queue runs with **10 workers** (or 3 in development).
* **Review Concurrency**: The `review` queue concurrency is controlled by the environment variable:
  ```bash
  LIVEREVIEW_WORKER_CONCURRENT_REVIEWS=10
  ```
  * **Default value**: `10` (if empty or not specified).
  * **Scale considerations**: Avoid setting this higher than `40` without scaling the PostgreSQL connection pool (or using a pooler like `pgBouncer`), as database connection saturation can occur.

---

## 3. How a Job is Triggered & Processed

### Step 1: Webhook Reception
A user performs an action in GitLab (e.g. opens a Merge Request, pushes a commit, or writes a comment like `@livereviewbot review`). GitLab fires a webhook to the LiveReview API server.

### Step 2: Job Insertion
The API server validates the webhook and enqueues a new job into the PostgreSQL `river_job` table with target arguments:
```go
// Example enqueuing a review
_, err := riverClient.Insert(ctx, &jobs.ReviewJobArgs{
    ReviewID: reviewID,
}, nil)
```

### Step 3: Worker Notification & Polling
The database triggers a `pg_notify('river_insert')` event. The background worker process (`livereview worker`) is listening to this notification and immediately wakes up a goroutine.

### Step 4: Execution
A free worker thread pulls the job from `river_job`, marks its state as `running`, executes the LLM prompting/review logic, updates usage/billing ledgers, and posts comments back to the GitLab merge request.

### Step 5: Finalization
Once successful, the job state is updated to `completed`. If it fails (e.g. due to rate limits or network issues), it is transitioned to `retryable` and scheduled for backoff retries.

---

## 4. River Queue Database Schema

River stores all states directly in PostgreSQL, making queues durable and transaction-safe. The key tables include:

* **`river_job`**: The main queue log. Contains:
  * `state`: `available`, `running`, `completed`, `discarded`, `retryable`, `cancelled`.
  * `args`: JSON payload for the job (e.g. `{"review_id": 123}`).
  * `errors`: Error trace stack if the job failed during execution.
  * `scheduled_at` / `finalized_at`: Time tracking.
* **`river_migration`**: Tracks active schema migration versions of the River system.
* **`river_queue`**: Operational table to track queues and pause/resume states.
* **`river_client` / `river_client_queue`**: System metadata tables tracking active worker nodes.