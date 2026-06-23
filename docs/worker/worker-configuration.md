# Worker Configuration Guide

This document covers how to configure the LiveReview background worker, and the planned improvements to make configuration easier for self-hosted deployments.

For a general overview of the worker architecture, queues, and job lifecycle, see [worker.md](./worker.md).


## Current Configuration

The worker's concurrency is controlled by a single environment variable:

```bash
LIVEREVIEW_WORKER_CONCURRENT_REVIEWS=10
```

| Variable | Default | Description |
|---|---|---|
| `LIVEREVIEW_WORKER_CONCURRENT_REVIEWS` | `10` | Number of parallel review jobs the worker runs at the same time. |

**Scale guidance**: Keep this value at or below `40`. Higher values can saturate the PostgreSQL connection pool. If you need to go higher, use a connection pooler like `pgBouncer`.

Currently, this variable must be set manually in the `.env` file or in environment configuration before starting the service.

---

## Phase 1 — Docker Compose Integration

**Goal**: Make worker configuration visible and easy to tune directly in `docker-compose.yml`, so that anyone onboarding with Docker does not need to hunt through `.env.example` to discover this setting.

### What Changes

**`docker-compose.yml`** — Add the worker concurrency variable explicitly in the `environment` block of the `livereview-app` service:

```yaml
services:
  livereview-app:
    # ... existing config ...
    environment:
      - LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION:-test}
      - DATABASE_URL=${DATABASE_URL:-postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable}
      - LIVEREVIEW_BACKEND_PORT=${LIVEREVIEW_BACKEND_PORT:-8888}
      - LIVEREVIEW_FRONTEND_PORT=${LIVEREVIEW_FRONTEND_PORT:-8081}
      - LIVEREVIEW_REVERSE_PROXY=${LIVEREVIEW_REVERSE_PROXY:-false}

      # Worker configuration
      - LIVEREVIEW_WORKER_CONCURRENT_REVIEWS=${LIVEREVIEW_WORKER_CONCURRENT_REVIEWS:-10}
```

**`docker-compose.prod.yml`** — Same addition to the production compose file.

**`.env.example`** — The variable is already present under the `WORKER / QUEUE` section (line 34). No change needed, but the section comment should be kept clearly visible.

### Why This Matters

- The variable is already read from the environment by the worker (`internal/jobqueue/queue_config.go`).
- Declaring it explicitly in `docker-compose.yml` with a sane default makes it **discoverable** — a new operator sees it immediately alongside ports and other settings.
- The `${VAR:-default}` pattern means it works out of the box with no `.env` changes required, but can be overridden trivially by setting it in `.env`.

### Acceptance Criteria

- [x] `LIVEREVIEW_WORKER_CONCURRENT_REVIEWS` appears in both `docker-compose.yml` and `docker-compose.prod.yml` with a default of `10`.
- [x] The service starts and the worker respects the value when it is overridden (e.g. `LIVEREVIEW_WORKER_CONCURRENT_REVIEWS=5`).
- [x] No `.env` file is required to be edited for the default behaviour to work.


## Phase 2 — Super Admin-Only UI Setting

**Goal**: Let the system's Super Admin change the worker concurrency for any organization from the LiveReview UI. Only the **`super_admin`** role can see or edit this setting — organization owners and members cannot.

### User Experience

- A new **"Worker Settings"** card appears inside the system settings or organization management view.
- It shows the current value of `worker_concurrent_reviews` and allows the Super Admin to update it.
- Organization owners and members visiting the settings pages do **not** see this section.
- Changes take effect without restarting the container (the worker picks up the new value on the next scheduling cycle, or via a live reload signal).

### Architecture

#### Database Storage (JSONB Column)

Instead of creating a new table or adding a dedicated column to the `orgs` table (which would introduce redundant columns populated with the default `10` value for all organizations), we store this setting inside the **existing `settings` JSONB column on the `orgs` table**.

* **Implicit Default**: If the key `"worker_concurrent_reviews"` is not in the JSONB object (which is true for 99.9% of organizations), we automatically fall back to the default value of `10` in the application logic.

##### GET Query (Fetch settings):
```sql
SELECT COALESCE((settings->>'worker_concurrent_reviews')::integer, 10) 
FROM orgs 
WHERE id = $1;
```

##### PUT Query (Update settings):
```sql
UPDATE orgs 
SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{worker_concurrent_reviews}', $1::text::jsonb), 
    updated_at = NOW() 
WHERE id = $2;
```

This is mapped to the Go `models.Org` struct in `pkg/models/models.go`.

#### Backend

1. **New endpoints** (Super Admin-only, session-gated):
   - `GET /api/v1/admin/orgs/:org_id/worker-config` — returns the current concurrency value.
   - `PUT /api/v1/admin/orgs/:org_id/worker-config` — updates the concurrency value.

2. **Middleware chain** — Gated by Super Admin middleware:
   - `RequireAuth()` → `RequireSuperAdmin()`
   - Handlers check that the user has the active global `super_admin` role in the database. 
   - **Access Gating**: If an owner or member of the organization (or any other unauthorized role) attempts to call these endpoints, the request is intercepted by the middleware, returning `403 Forbidden`. No query is run to fetch or modify settings.
   - API keys are **not** allowed for this endpoint (session-only gate).

3. **Validation**:
   - The backend `PUT` endpoint validates that the requested value is an integer between `1` and `40` inclusive.
   - Values outside this range return a `400 Bad Request` error.

4. **Worker hot-reload** — After a successful `PUT`, the server publishes an internal signal (e.g. via a channel or a `pg_notify` event) so the River queue manager updates the concurrency limit without a full restart.

#### Frontend

- The Super Admin settings **Deployment** tab (`#/settings#deployment`) gains a **"Worker Concurrency Settings"** card.
- The card contains a numeric input for `Concurrent Reviews` with inline help text showing the safe range (1–40).
- Frontend validation blocks form submission and displays an error if the input value is not within `1` to `40`.
- The card is not rendered unless the authenticated user's role is strictly `super_admin`.

### API & Query Data Flow Example

Below is the concrete HTTP request/response format and PostgreSQL query trace, using a mock Organization ID of `999` and a target limit of `15`.

#### 1. Fetch Config (`GET /api/v1/admin/orgs/999/worker-config`)

- **PostgreSQL Query**:
  ```sql
  SELECT COALESCE((settings->>'worker_concurrent_reviews')::integer, 10) 
  FROM orgs 
  WHERE id = 999;
  ```
- **DB Response**: Returns `15` (or `10` if not set).
- **HTTP Response (to browser)**:
  - **Status**: `200 OK`
  - **Body**:
    ```json
    {
      "worker_concurrent_reviews": 15
    }
    ```

#### 2. Update Config (`PUT /api/v1/admin/orgs/999/worker-config`)

- **HTTP Request (from browser)**:
  - **Body**:
    ```json
    {
      "worker_concurrent_reviews": 15
    }
    ```
- **PostgreSQL Query**:
  ```sql
  UPDATE orgs 
  SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{worker_concurrent_reviews}', '15'::jsonb), 
      updated_at = NOW() 
  WHERE id = 999;
  ```
- **DB Response**: `UPDATE 1`
- **HTTP Response (to browser)**:
  - **Status**: `200 OK`
  - **Body**:
    ```json
    {
      "message": "Worker concurrency updated successfully",
      "worker_concurrent_reviews": 15
    }
    ```

#### 3. Unauthorized User Access Gating (e.g. Non-Super Admin)

- **HTTP Request**: Any request to the `/api/v1/admin/...` group from a regular member or owner.
- **PostgreSQL Middleware Query (Role Validation Check)**:
  ```sql
  SELECT EXISTS(
      SELECT 1 FROM user_roles ur
      JOIN roles r ON ur.role_id = r.id
      WHERE ur.user_id = $1 AND r.name = 'super_admin'
  );
  ```
- **Middleware Action**: Since the query returns `false`, the request is aborted immediately before running any configuration queries.
- **HTTP Response (to browser)**:
  - **Status**: `403 Forbidden`
  - **Body**:
    ```json
    {
      "message": "Super admin access required"
    }
    ```

### Security Notes

- Follows the strict **Super Admin** role boundary defined in `AGENTS.md` — only global super admins can mutate worker capacity.
- Organization owners (even paid ones) and members have no access and receive no visual indication that the setting exists.
- The endpoint is **session-only** (no API key access) because changing runtime worker capacity is a sensitive operational action.

### Acceptance Criteria

- [ ] `GET /api/v1/admin/orgs/:org_id/worker-config` returns `200` with the limit value for super admins, `403` for organization owners and members.
- [ ] `PUT /api/v1/admin/orgs/:org_id/worker-config` updates the `settings` JSONB column in the `orgs` table and returns the new value.
- [ ] Backend validation returns `400 Bad Request` for values `< 1` or `> 40`.
- [ ] Frontend input restricts input range to `1` to `40` and displays validation errors.
- [ ] The worker respects the updated value within one scheduling cycle (no restart required).
- [ ] The UI card is visible only to super admins on the **Deployment** tab; paid owners and members see no trace of it.
- [ ] An API key cannot call these endpoints (returns `403`).
- [ ] The value is persisted in the database (`settings` JSONB column of `orgs` table) and survives a container restart.

---

1. **Phase 2 / Live reload**: Should the worker hot-reload immediately on `PUT`, or on the next scheduling interval (e.g. every 30 s)? Immediate is better UX but adds a signal/channel mechanism.
2. **Phase 2 / Scope**: Should the UI also expose the `default` queue concurrency (currently hardcoded at 10), or only the `review` queue concurrency?
