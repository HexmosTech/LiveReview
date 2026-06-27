# Worker Configuration Guide

This document covers how to configure the LiveReview background worker, and the planned improvements to make configuration easier for self-hosted deployments.

For a general overview of the worker architecture, queues, and job lifecycle, see [worker.md](./worker.md).


## Current Configuration

The worker's concurrency is stored in the **`instance_details`** database table and managed via the **Admin Settings → Deployment tab** (Super Admin only). The default value is `10`.

**Scale guidance**: Keep this value at or below `40`. Higher values can saturate the PostgreSQL connection pool. If you need to go higher, use a connection pooler like `pgBouncer`.

On startup the worker reads the value directly from the database. If the table is empty (fresh install before the first login), it falls back to `10`.

---

## Phase 1 — Docker Compose Integration

> **Completed and superseded by Phase 2.** Worker concurrency is now managed entirely via the database (Admin UI). No environment variable is required or read.


## Phase 2 — Super Admin-Only UI Setting

**Goal**: Let the system's Super Admin change the worker concurrency globally for the entire LiveReview instance from the LiveReview UI. Only the **`super_admin`** role can see or edit this setting — organization owners and members cannot.

### User Experience

- A new **"Background Worker Concurrency"** settings card appears inside the **Instance** tab in Settings.
- It shows the current value of `worker_concurrent_reviews` and allows the Super Admin to update it.
- Organization owners and members visiting the settings pages do **not** see this section (they do not have access to the Instance tab).
- Changes take effect when the background worker processes are restarted (to read the new concurrency limit).

### Architecture

#### Database Storage

We store this setting in the **`instance_details`** table, which holds global configuration details.

##### GET Query (Fetch settings):
```sql
SELECT COALESCE(worker_concurrent_reviews, 10) 
FROM instance_details 
LIMIT 1;
```

##### PUT Query (Update settings):
```sql
UPDATE instance_details 
SET worker_concurrent_reviews = $1, 
    updated_at = NOW() 
WHERE id = (SELECT id FROM instance_details LIMIT 1);
```

#### Backend

1. **New endpoints** (Super Admin-only, session-gated):
   - `GET /api/v1/admin/worker-config` — returns the current global concurrency value.
   - `PUT /api/v1/admin/worker-config` — updates the global concurrency value.

2. **Middleware chain** — Gated by Super Admin middleware:
   - `RequireAuth()` → `RequireSuperAdmin()`
   - Handlers check that the user has the active global `super_admin` role in the database. 
   - **Access Gating**: If an owner or member of the organization (or any other unauthorized role) attempts to call these endpoints, the request is intercepted by the middleware, returning `403 Forbidden` (RequireSuperAdmin or RequireAuth will fail).
   - API keys are **not** allowed for this endpoint (session-only gate).

3. **Validation**:
   - The backend `PUT` endpoint validates that the requested value is an integer between `1` and `40` inclusive.
   - Values outside this range return a `400 Bad Request` error.
   - For scaling to high limits (above 15), a connection pooler like `pgBouncer` is recommended to prevent saturating database connections.

#### Frontend

- The Super Admin settings **Instance** tab (`#/settings#instance`) gains a **"Background Worker Concurrency"** card.
- The card contains a numeric input for `Worker Concurrency Limit` with inline help text showing the safe range (1–40) and connection pooling recommendations.
- Frontend validation blocks form submission and displays an error if the input value is not within `1` to `40`.
- The card is not rendered unless the authenticated user's role is strictly `super_admin`.

### API & Query Data Flow Example

Below is the concrete HTTP request/response format and PostgreSQL query trace.

#### 1. Fetch Config (`GET /api/v1/admin/worker-config`)

- **PostgreSQL Query**:
  ```sql
  SELECT COALESCE(worker_concurrent_reviews, 10) 
  FROM instance_details 
  LIMIT 1;
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

#### 2. Update Config (`PUT /api/v1/admin/worker-config`)

- **HTTP Request (from browser)**:
  - **Body**:
    ```json
    {
      "worker_concurrent_reviews": 15
    }
    ```
- **PostgreSQL Query**:
  ```sql
  UPDATE instance_details 
  SET worker_concurrent_reviews = 15, 
      updated_at = NOW() 
  WHERE id = (SELECT id FROM instance_details LIMIT 1);
  ```
- **DB Response**: `UPDATE 1`
- **HTTP Response (to browser)**:
  - **Status**: `200 OK`
  - **Body**:
    ```json
    {
      "success": true,
      "message": "Worker concurrency updated successfully. A restart of the background worker process is required for changes to take effect.",
      "worker_concurrent_reviews": 15
    }
    ```

#### 3. Unauthorized User Access Gating (e.g. Non-Super Admin)

- **HTTP Request**: Any request to the `/api/v1/admin/...` group from a regular member or owner.
- **Middleware Action**: Since the user is not a super_admin, `RequireSuperAdmin` middleware rejects the request.
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

- [x] `GET /api/v1/admin/worker-config` returns `200` with the limit value for super admins, `403` for organization owners and members.
- [x] `PUT /api/v1/admin/worker-config` updates the `worker_concurrent_reviews` column in the `instance_details` table and returns the new value.
- [x] Backend validation returns `400 Bad Request` for values `< 1` or `> 40`.
- [x] Frontend input restricts input range to `1` to `40` and displays validation errors.
- [x] The worker respects the updated value on next start (restart required).
- [x] The UI card is visible only to super admins on the **Deployment** tab.
- [x] An API key cannot call these endpoints (returns `403`).
- [x] The value is persisted in the database (`worker_concurrent_reviews` column of `instance_details` table) and survives a container restart.

---

1. **Phase 2 / Live reload**: Should the worker hot-reload immediately on `PUT`, or on the next scheduling interval (e.g. every 30 s)? Immediate is better UX but adds a signal/channel mechanism.
2. **Phase 2 / Scope**: Should the UI also expose the `default` queue concurrency (currently hardcoded at 10), or only the `review` queue concurrency?

---

## Phase 3 — Distributed Worker Setup (Outside Docker)

This section describes how to scale the LiveReview background worker to run on multiple physical or virtual machines outside of Docker, using process managers like `systemd` or `pm2`.

### Architectural Principles

Because the job queue is powered by **River** (PostgreSQL-backed), distributed locking is handled entirely at the database layer using Postgres transactions and `SKIP LOCKED`.
- **Concurrency Safety**: Multiple worker instances running on different hosts can safely point to the same database. When one worker picks up a job, it locks the database row; other workers automatically skip it.
- **No Shared State**: Worker nodes do not need to communicate with each other or with the API server directly. They only need a network connection to the central PostgreSQL database.

### Requirements & Setup

To run a background worker on an external machine:

#### 1. Database Access
The central PostgreSQL database must be network-accessible from all worker machines.
- Update `postgresql.conf` on the database server to listen on public/private IPs:
  ```ini
  listen_addresses = '*'
  ```
- Configure `pg_hba.conf` to allow client connections from the worker IPs.
- Use proper connection pooling (e.g. `pgBouncer`) if the total connection count across all workers exceeds 100.

#### 2. Deploy Worker Binary
Build the `livereview` binary on the target worker architecture and copy it (along with its `.env` file) to the host.
```bash
# Build binary
go build -o livereview livereview.go
```

#### 3. Environment Configuration
Every worker machine needs a `.env` file containing:
```env
DATABASE_URL=postgres://livereview:password@db-host-ip:5432/livereview?sslmode=disable
LIVEREVIEW_WORKER_CONCURRENT_REVIEWS=10
```

#### 4. Run the Worker Process
Start the worker using the `worker` subcommand:
```bash
./livereview worker --env-file .env
```

---

### Process Management Guides

To ensure the worker process survives crashes, restarts on boot, and logs errors properly, run it under a process manager.

#### Option A: Systemd (Recommended for Linux VMs)

Create a systemd service file at `/etc/systemd/system/livereview-worker.service`:

```ini
[Unit]
Description=LiveReview Background Worker
After=network.target

[Service]
Type=simple
User=gk
WorkingDirectory=/home/gk/hex/LiveReview
ExecStart=/home/gk/hex/LiveReview/livereview worker --env-file /home/gk/hex/LiveReview/.env
Restart=always
RestartSec=5
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

Enable and start the service:
```bash
sudo systemctl daemon-reload
sudo systemctl enable livereview-worker
sudo systemctl start livereview-worker
```
Monitor logs with:
```bash
journalctl -u livereview-worker -f
```

#### Option B: PM2 (Node.js-based Process Manager)

If you already use PM2 for managing frontend/node processes, create an `ecosystem.config.js` file:

```javascript
module.exports = {
  apps: [
    {
      name: 'livereview-worker',
      script: './livereview',
      args: 'worker',
      cwd: '/home/gk/hex/LiveReview',
      instances: 1,
      autorestart: true,
      watch: false,
      env: {
        DATABASE_URL: 'postgres://livereview:password@db-host-ip:5432/livereview?sslmode=disable'
        // Worker concurrency is read from the database (set via Admin Settings → Deployment tab).
        // Default is 10 if the database has no value yet.
      }
    }
  ]
};
```

Start the worker via PM2:
```bash
pm2 start ecosystem.config.js
pm2 save
```
Monitor logs with:
```bash
pm2 logs livereview-worker
```
