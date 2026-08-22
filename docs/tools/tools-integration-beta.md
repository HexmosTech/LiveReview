# Third-Party Tools Integration – Beta

LiveReview can run external static-analysis tools (ruff, bandit, eslint, etc.) as parallel Lambda jobs alongside every AI review. Results are stored as `tool_result` events in the existing `review_events` table and surfaced in the review UI and `lrc` CLI output.

This feature is **cloud-only** and **owner-gated**. It is delivered in three sequential phases.

---

## Current Status (feat/tool-integration-v2)

**Only UI iterations are done. Backend architecture is not started.**

What exists today:

- The **Tool Analysis card** (`ui/src/components/reviews/ToolAnalysisCard.tsx`) renders on `ReviewDetail`.
- Three test review pages simulate the card across stages:
  - `/#/reviews/test1` — all tools queued/pending.
  - `/#/reviews/test2` — mixed running/completed/queued with animated spinners.
  - `/#/reviews/test3` — all 15 tools completed with findings and failures.
- The mock data for these stages lives in `ui/src/pages/Reviews/ReviewDetail.tsx` (`fetchReviewDetails`, test IDs only). The card consumes the mock `toolBreakdown`, not a real API.
- The card is collapsed by default. Its toggle button shows live state (running count, findings count) so users know what to expect before expanding.

What does **not** exist yet:

- Database migrations (`available_tools`, `org_tools`).
- Settings UI tab and its API endpoints.
- River `tool_invocation` job and Lambda fan-out.
- `tool_result` events written to `review_events`.
- `lrc` CLI rendering of tool findings.
- Any backend integration or cost billing.

The sections below document the **planned** backend architecture. Treat them as the design target, not as a description of the current code.

---

## Table of Contents

1. [Cost Model](#cost-model)
2. [Phase 1 – DB Schema & Settings Tab](#phase-1--db-schema--settings-tab)
3. [Phase 2 – Settings UI & API](#phase-2--settings-ui--api)
4. [Phase 3 – Queue, Lambda Trigger & Review UI](#phase-3--queue-lambda-trigger--review-ui)
5. [Shared Schemas](#shared-schemas)

---

## Cost Model

Each tool runs as an independent Lambda invocation. Cost is billed in GB-seconds at the AWS ARM64 rate (`$0.0000133334 / GB-s`).

**Formula per tool invocation:**

```
cost = (memory_mb / 1024) × timeout_seconds × rate
```

**Credit budget:** LiveReview provides **50,000 credits** per org. One credit equals the cost of one invocation of the cheapest tool (the baseline). Orgs spend credits from this pool each time a tool runs on a review.

### Tool catalog reference

The table below lists available tools. `multiplier` is computed by the API (`(memory_mb / 1024) × timeout_s` relative to the cheapest tool) and returned in the `GET /api/v1/orgs/:org_id/tools` response. Tools marked **Beta** are included in the initial seed.

| Tool | Multiplier | Use Case |
|---|---|---|
| openapi | computed | OpenAPI/YAML validation |
| actionlint | computed | GitHub Actions lint |
| shellcheck | computed | Shell script lint |
| hadolint | computed | Dockerfile lint |
| ruff | computed | Python lint/format |
| tfsec | computed | Terraform IaC |
| zizmor | computed | GitHub Actions security |
| gitleaks | computed | Secret detection |
| bandit | computed | Python SAST |
| eslint | computed | JavaScript/TypeScript SAST |
| detect-secrets | computed | Secret scanning |
| trufflehog | computed | Secret scanning (deep) |
| spectral | computed | API spec lint |
| kubescape | computed | Kubernetes IaC |
| trivy | computed | Container/IaC CVE scan |
| brakeman | computed | Ruby SAST |
| semgrep | computed | Multi-language SAST |
| golangci-lint | computed | Go SAST |

**What users see in the UI:** tool name, multiplier tier, use case, and the running total cost per review so they can choose a tool budget that fits within their credit allowance.

---

## Phase 1 – DB Schema & Settings Tab

### DB migrations (dbmate, local only)

Two migrations are added to `db/migrations/`. **Never apply directly to production** — use dbmate.

#### Migration 1: `available_tools` catalog

```sql
-- migrate:up
CREATE TABLE IF NOT EXISTS public.available_tools (
    id          bigserial PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    description text NOT NULL,
    lambda_arn  text NOT NULL,
    multiplier  numeric(6,2) NOT NULL DEFAULT 1.0,
    use_case    text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- Seed initial tools (ruff and bandit as the two cheapest beta tools)
INSERT INTO public.available_tools (name, description, lambda_arn, multiplier, use_case) VALUES
  ('ruff',   'Fast Python linter and formatter', 'arn:aws:lambda:us-east-1:ACCOUNT:function:ruff-python-linter',  1.0, 'Python lint/format'),
  ('bandit', 'Python security linter (SAST)',    'arn:aws:lambda:us-east-1:ACCOUNT:function:bandit-linter',       1.0, 'Python SAST')
ON CONFLICT (name) DO NOTHING;

-- migrate:down
DROP TABLE IF EXISTS public.available_tools;
```

#### Migration 2: `org_tools` per-org selection

```sql
-- migrate:up
CREATE TABLE IF NOT EXISTS public.org_tools (
    org_id      bigint NOT NULL REFERENCES public.organizations(id) ON DELETE CASCADE,
    tool_id     bigint NOT NULL REFERENCES public.available_tools(id) ON DELETE CASCADE,
    enabled     boolean NOT NULL DEFAULT false,
    config_json jsonb   NOT NULL DEFAULT '{}',
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, tool_id)
);

CREATE INDEX IF NOT EXISTS idx_org_tools_org_id ON public.org_tools (org_id);

-- migrate:down
DROP INDEX  IF EXISTS idx_org_tools_org_id;
DROP TABLE  IF EXISTS public.org_tools;
```

**Key design decisions:**
- `available_tools` is a global catalog — rows are added by platform operators, never by org owners.
- `org_tools` stores one row per (org, tool) pair when an org has ever interacted with that tool. Rows with `enabled = false` are stored explicitly so toggle state is preserved.
- `multiplier` on `available_tools` is denormalised from the Lambda config so the UI can display cost tiers without a live Lambda call.

### Settings tab (UI, Phase 1 scope)

A new tab entry is added to `ui/src/pages/Settings/Settings.tsx`:

```typescript
// Added to the tabs array — only shown when isCloudMode() AND role is 'owner'
...(isCloudMode() && currentOrg?.role === 'owner' ? [{
    id: 'third-party-tools',
    name: 'Third-Party Tools',
    icon: <ToolsIcon />
}] : [])
```

At this phase the tab renders a placeholder ("Tool configuration coming in Phase 2"). Non-owners who navigate directly to `/#/settings#third-party-tools` see a read-only message; the tab button is not shown in the sidebar.

---

## Phase 2 – Settings UI & API

### API endpoints

Both endpoints live under the existing `orgGroup` in `server.go`, which already applies the full middleware chain:

```
RequireAuthOrAPIKey → BuildOrgContext → ValidateOrgAccess → BuildPermissionContext
```

The billing check middlewares (`BuildOrgBillingPlanContext`, `BuildPlanContext`) are also applied.

---

#### `GET /api/v1/orgs/:org_id/tools`

Returns the full available tools catalog joined with this org's enabled state.

**Access:** any authenticated org member (owner or member).  
**Cloud gate:** returns HTTP 403 if `isCloudMode()` is false on the server.  
**Org isolation:** query is scoped by `org_id` from `PermissionContext`, not from the URL path parameter.

**Response 200:**

```json
{
  "tools": [
    {
      "id": 1,
      "name": "ruff",
      "description": "Fast Python linter and formatter",
      "multiplier": 1.0,
      "use_case": "Python lint/format",
      "enabled": true,
      "config_json": {}
    },
    {
      "id": 2,
      "name": "bandit",
      "description": "Python security linter (SAST)",
      "multiplier": 1.0,
      "use_case": "Python SAST",
      "enabled": false,
      "config_json": {}
    }
  ]
}
```

Fields `enabled` and `config_json` default to `false` / `{}` when no `org_tools` row exists for that tool.

**Error responses:**

| Status | Condition |
|---|---|
| 401 | Missing or invalid auth token |
| 403 | Not cloud mode, or org mismatch |
| 500 | Database error |

---

#### `PUT /api/v1/orgs/:org_id/tools/:tool_id`

Enables or disables a specific tool for the org (upsert).

**Access:** `owner` role only.  
**Cloud gate:** HTTP 403 if not cloud mode.  
**Org isolation:** upsert uses `org_id` from `PermissionContext`.

**Request body:**

```json
{ "enabled": true }
```

The `enabled` field is required and must be a boolean. Any other value returns HTTP 400.

**Response 200:**

```json
{
  "tool_id": 1,
  "org_id": 42,
  "enabled": true,
  "config_json": {}
}
```

**Error responses:**

| Status | Condition |
|---|---|
| 400 | `enabled` field absent or not a boolean |
| 401 | Missing or invalid auth token |
| 403 | Not cloud mode, not owner, or org mismatch |
| 404 | `tool_id` not found in `available_tools` |
| 500 | Database error |

---

### Settings UI – ThirdPartyToolsTab component

File: `ui/src/pages/Settings/ThirdPartyToolsTab.tsx`

The tab replaces the Phase 1 placeholder. It fetches `GET /api/v1/orgs/:org_id/tools` on mount and renders a table with the following columns:

| Column | Description |
|---|---|
| Tool name | Human-readable name |
| Use case | Short category label (e.g. "Python SAST") |
| Multiplier | Cost tier (e.g. `1×`, `3×`, `20×`) |
| Toggle | Enable/disable switch (owner only) |

**Cost summary bar** at the top of the tab shows:
- Number of enabled tools
- Total multiplier of all enabled tools (sum)
- Estimated credits consumed per review = sum of enabled tool multipliers × baseline cost

**Owner behaviour:**
- Toggling a tool calls `PUT /api/v1/orgs/:org_id/tools/:tool_id` immediately.
- On API error: inline error message shown, toggle reverted to previous state.
- While any request is in flight: all toggles are disabled and a spinner is shown.

**Non-owner / member behaviour:**
- Table renders in read-only state. Toggles are replaced with a static enabled/disabled badge.
- No PUT calls are made.

---

## Phase 3 – Queue, Lambda Trigger & Review UI

### River job: `tool_invocation`

File: `internal/jobqueue/jobqueue.go` (alongside existing `webhook_install` / `webhook_removal` jobs)

#### Job args

```go
type ToolInvocationJobArgs struct {
    ReviewID int64  `json:"review_id"`
    OrgID    int64  `json:"org_id"`
    ToolID   int64  `json:"tool_id"`
    ToolName string `json:"tool_name"`
    LambdaARN string `json:"lambda_arn"`
}

func (ToolInvocationJobArgs) Kind() string { return "tool_invocation" }
```

#### Worker

```go
type ToolInvocationWorker struct {
    river.WorkerDefaults[ToolInvocationJobArgs]
    db         *sql.DB
    httpClient *http.Client
}
```

**Work() logic:**

1. Load the diff from `SELECT diff FROM reviews WHERE id = $1 AND org_id = $2`. If the review has no diff, log and return without error (nothing to analyse).
2. POST the diff as the Lambda payload to the tool's `lambda_arn` via HTTPS.
3. On non-2xx response: return an error so River applies its standard retry policy.
4. On 2xx: insert a `review_events` row (see schema below).

#### Fan-out trigger

In `WebhookOrchestratorV2` (or the unified processor), after diff extraction completes:

```go
enabledTools, err := store.GetEnabledToolsForOrg(ctx, orgID)
for _, tool := range enabledTools {
    _, err = riverClient.Insert(ctx, ToolInvocationJobArgs{
        ReviewID:  reviewID,
        OrgID:     orgID,
        ToolID:    tool.ID,
        ToolName:  tool.Name,
        LambdaARN: tool.LambdaARN,
    }, nil)
}
```

All jobs are inserted in a single loop — River runs them concurrently up to `MaxWorkers`.

### Lambda payload & response

**Payload sent to Lambda (JSON):**

```json
{
  "review_id": 1234,
  "diff": "<unified diff string>"
}
```

**Expected Lambda response (JSON):**

```json
{
  "exit_code": 0,
  "findings": [
    {
      "file": "src/main.py",
      "line": 42,
      "col": 5,
      "rule": "E501",
      "message": "Line too long (92 > 79 characters)"
    }
  ],
  "lines_of_code": 312,
  "stderr": ""
}
```

The full response body is stored verbatim in the `data` JSONB column of `review_events`.

### `review_events` row for tool results

No new table is needed. A new `event_type` value is added to the existing `review_events` table:

```sql
-- No migration required — event_type is free-text.
-- New rows look like:
INSERT INTO public.review_events (review_id, org_id, event_type, data)
VALUES (
  $1,                    -- review_id
  $2,                    -- org_id (from review record, never from job args directly)
  'tool_result',
  '{
    "tool_id":   1,
    "tool_name": "ruff",
    "exit_code": 0,
    "findings":  [...],
    "lines_of_code": 312,
    "stderr": ""
  }'
);
```

`org_id` is always read from the `reviews` row, not from the job args, to prevent any spoofing.

### Beta review UI

Route: `/#/reviews-tools/new`  
File: `ui/src/pages/Reviews/BetaToolReviewPage.tsx`

- Registered in the React Router config but **not** added to the sidebar or any nav surface.
- If `isCloudMode()` returns false, renders: *"Tool-based reviews are only available in cloud mode."*
- Otherwise renders a layout matching the existing AI review page (`NewReview.tsx`) with the trigger form at the top and a live event stream below.
- `tool_result` events in the stream are rendered with a coloured badge showing the tool name (e.g. `[ruff]`), followed by the findings list.

### Tool Analysis & Credits Panel in ReviewDetail

File: `ui/src/pages/Reviews/ReviewDetail.tsx`

When a review includes third-party static analysis tool invocations (`event_type === 'tool_result'`):

- A dedicated **Tool Analysis & Credits** card is rendered directly below the main Review Info header.
- **Top 3 KPI Summary Boxes**:
  - `Total Tool Credits`: Sum of credits consumed by tool executions on this review (e.g. `2.0 Credits`).
  - `Tools Executed`: Count of static analysis tools run (e.g. `2 Tools`).
  - `Total Comments Generated`: Total comments/findings posted by tools (e.g. `14 Comments`).
- **Tool Summary Cards Grid**:
  - Displays a grid of individual tool cards (`ruff`, `bandit`, `gitleaks`, `eslint`, etc.).
  - Each card shows: **Tool Name**, **Credits Used** (e.g. `1.0 Credits`), **Comments Generated** (e.g. `14 Comments`), and **Status Badge** (e.g. Green `Clean` badge vs Amber `14 Comments` badge).
- If no `tool_result` events exist for the review, the Tool Analysis panel is hidden.

### `lrc` CLI output

When `lrc` renders a completed review and encounters events with `event_type === 'tool_result'`:

```
[ruff] src/auth/login.py:42:5  E501  Line too long (92 > 79 characters)
[ruff] src/auth/login.py:78:1  F401  'os' imported but unused
[bandit] src/utils/crypto.py:12:0  B303  Use of MD5 not recommended
```

Tag format: `[<tool_name>]` followed by the finding in standard linter format.  
If no `tool_result` events are present, the tool section is skipped entirely (no empty header rendered).

---

## Shared Schemas

### `tool_result` event data shape

```json
{
  "tool_id":        1,
  "tool_name":      "ruff",
  "exit_code":      0,
  "findings": [
    {
      "file":    "src/main.py",
      "line":    42,
      "col":     5,
      "rule":    "E501",
      "message": "Line too long (92 > 79 characters)"
    }
  ],
  "lines_of_code":  312,
  "stderr":         ""
}
```

### `tool_invocation` River job schema

```json
{
  "review_id":  1234,
  "org_id":     42,
  "tool_id":    1,
  "tool_name":  "ruff",
  "lambda_arn": "arn:aws:lambda:us-east-1:ACCOUNT:function:ruff-python-linter"
}
```

### `available_tools` table

| Column | Type | Notes |
|---|---|---|
| `id` | bigserial | PK |
| `name` | text | Unique, e.g. `ruff` |
| `description` | text | Human-readable |
| `lambda_arn` | text | Full ARN of the Lambda function |
| `multiplier` | numeric(6,2) | Cost tier relative to baseline tool |
| `use_case` | text | Short label, e.g. `Python SAST` |
| `created_at` | timestamptz | |

### `org_tools` table

| Column | Type | Notes |
|---|---|---|
| `org_id` | bigint | FK → `organizations.id` |
| `tool_id` | bigint | FK → `available_tools.id` |
| `enabled` | boolean | Default `false` |
| `config_json` | jsonb | Per-org tool config, default `{}` |
| `updated_at` | timestamptz | |

---

## ReviewDetail Header – Data Requirements & API Design

### Problem: Current page makes 5 serial/parallel API calls on load

```
Promise.all([
  GET /api/v1/reviews/:id           → Review row
  GET /api/v1/reviews/:id/events    → up to 1000 events (huge payload, used only for severity counts + events tab)
  GET /api/v1/reviews/:id/summary   → batchCount, lastActivity
])
+ sequential:
  GET /api/v1/reviews/:id/accounting  → cost, tokens (used only for Accounting tab + tool summary)
  GET /api/v1/reviews/:id/commits     → commit SHAs
```

Severity counts (High/Medium/Low) are computed client-side by scanning 1000 events — a payload fetched solely to count three numbers. The accounting call is a round trip just to show `$0.04` in the header.

---

### What the header needs to display

| UI element | Data field | Current source |
|---|---|---|
| Repo name | `review.repository` | `GET /reviews/:id` |
| Branch | `review.branch` | `GET /reviews/:id` |
| Provider icon | `review.provider` | `GET /reviews/:id` |
| PR/MR link | `review.prMrUrl` | `GET /reviews/:id` |
| MR Title | `review.mrTitle` | `GET /reviews/:id` (field exists, not displayed) |
| Status badge | `review.status` | `GET /reviews/:id` |
| Created by | `review.userEmail` | `GET /reviews/:id` |
| Created at | `review.createdAt` | `GET /reviews/:id` |
| Author name | `review.authorName` | `GET /reviews/:id` (field exists, not displayed) |
| Trigger type | `review.triggerType` | `GET /reviews/:id` (field exists, not displayed) |
| Severity: High | computed from events | `GET /reviews/:id/events` (1000 items) ← **expensive** |
| Severity: Medium | computed from events | same |
| Severity: Low | computed from events | same |
| Tools executed | accounting-derived | `GET /reviews/:id/accounting` ← **separate call** |
| Total findings | accounting-derived | same |
| Total cost (USD) | `accounting.totalCostUsd` | same |
| Batch count | `summary.batchCount` | `GET /reviews/:id/summary` |
| Last activity | `summary.lastActivity` | `GET /reviews/:id/summary` |
| Duration | `review.startedAt` + `review.completedAt` | `GET /reviews/:id` |
| Commits list | `commits[]` | `GET /reviews/:id/commits` |
| Tool breakdown | accounting-derived | `GET /reviews/:id/accounting` |

---

### Proposed: 2-call design

#### Call 1 — Eager, on page load

**Enrich `GET /api/v1/reviews/:id/summary`** to be the single source of truth for the header. The backend computes severity counts and pulls tool/cost aggregates from existing tables in one query, rather than making the client do 3 round trips.

**Proposed enriched response shape:**

```json
{
  "reviewId": 123,
  "currentStatus": "completed",
  "lastActivity": "2026-08-17T16:10:00Z",
  "batchCount": 4,
  "eventCounts": { "log": 45, "batch": 4, "tool_result": 15 },

  "severityCounts": {
    "high":   0,
    "medium": 1,
    "low":    11
  },

  "toolSummary": {
    "toolsExecuted":          15,
    "totalCommentsGenerated": 11,
    "totalCostUsd":           0.042,
    "toolBreakdown": [
      { "toolName": "ruff",     "creditsUsed": 1.0, "commentsGenerated": 3,  "status": "completed" },
      { "toolName": "bandit",   "creditsUsed": 1.0, "commentsGenerated": 0,  "status": "clean"     },
      { "toolName": "gitleaks", "creditsUsed": 1.0, "commentsGenerated": 8,  "status": "completed" }
    ]
  }
}
```

**Backend implementation notes:**

- `severityCounts` — computed with a single `SELECT level, COUNT(*) FROM review_events WHERE review_id = $1 AND org_id = $2 AND event_type = 'tool_result' GROUP BY level` (or equivalent severity field on the data JSONB). If severity is embedded in `data->>'level'`, use a `jsonb` extraction in the GROUP BY.
- `toolSummary` — aggregated from `review_events` rows where `event_type = 'tool_result'`, pulling `data->>'tool_name'`, `data->>'exit_code'`, and `jsonb_array_length(data->'findings')` per row. No join to `accounting` needed for the header.
- `totalCostUsd` — pulled from `review_accounting` (existing table) if present; `null` if not yet recorded.

#### Call 2 — Lazy, only when Events tab is opened

```
GET /api/v1/reviews/:id/events?limit=1000
```

Events are **not** needed until the user opens the Events tab or Findings tab. Defer this call entirely. No severity pre-computation needed on the client.

#### Commits — unchanged (best-effort, non-blocking)

```
GET /api/v1/reviews/:id/commits
```

Already fire-and-forget. Keep as is.

---

### Summary of calls after redesign

| Call | When | Purpose |
|---|---|---|
| `GET /reviews/:id` | eager | Review identity, status, PR info |
| `GET /reviews/:id/summary` (enriched) | eager | All header aggregates: severity, tools, cost, batches |
| `GET /reviews/:id/commits` | eager, non-blocking | Commit list in Details panel |
| `GET /reviews/:id/events` | lazy (Events tab) | Full event stream |
| `GET /reviews/:id/accounting` | lazy (Accounting tab only) | Detailed token/LOC breakdown |


**Result:** header renders completely from 2 parallel calls instead of 5. The 1000-event payload is deferred until actually needed.

---

## Tool Status Lifecycle – How Each Tool's Status is Tracked

### The core problem

A `tool_result` event is only written **when a Lambda returns**. There is no event for "this tool was dispatched" or "this tool is currently running". Without a record of what tools were dispatched, the UI has no way to distinguish between:

- A tool that hasn't started yet (`pending`)
- A tool that is actively running (`running`)
- A review that never had that tool enabled at all

This section defines the two-event approach that solves this without a new table.

---

### Two event types for tools

#### 1. `tool_dispatch` — written at fan-out time

When `WebhookOrchestratorV2` inserts River jobs for the enabled tools, it **also** writes one `tool_dispatch` event per tool into `review_events` immediately:

```sql
INSERT INTO public.review_events (review_id, org_id, event_type, data)
VALUES (
  $1,
  $2,
  'tool_dispatch',
  '{"tool_id": 1, "tool_name": "ruff", "status": "pending"}'
);
```

This gives the UI a complete, authoritative list of **every tool that was launched for this review**, even before any results come back.

Go insert (inside the fan-out loop, same transaction as the River job inserts):

```go
for _, tool := range enabledTools {
    _, err = riverClient.Insert(ctx, ToolInvocationJobArgs{...}, nil)
    // immediately record the dispatch
    _, err = store.InsertReviewEvent(ctx, InsertReviewEventParams{
        ReviewID:  reviewID,
        OrgID:     orgID,
        EventType: "tool_dispatch",
        Data:      json.RawMessage(fmt.Sprintf(
            `{"tool_id":%d,"tool_name":%q,"status":"pending"}`,
            tool.ID, tool.Name,
        )),
    })
}
```

#### 2. `tool_result` — written when Lambda returns

When the River worker receives the Lambda response, it writes a `tool_result` event (already defined in Phase 3):

```json
{
  "tool_id":   1,
  "tool_name": "ruff",
  "exit_code": 0,
  "findings":  [...],
  "lines_of_code": 312,
  "stderr": ""
}
```

---

### Status derivation rules

The frontend (and the enriched summary endpoint) derives per-tool status by **joining dispatch events with result events** for the same `tool_name`:

| Condition | Derived status |
|---|---|
| `tool_dispatch` exists, no `tool_result` yet | `pending` |
| `tool_dispatch` exists, River job is running (no result yet, time elapsed) | `running` (approximated by age — see note below) |
| `tool_result` exists, `exit_code = 0`, `findings = []` | `clean` |
| `tool_result` exists, `exit_code = 0`, `findings.length > 0` | `completed` |
| `tool_result` exists, `exit_code != 0` | `failed` |
| River job exhausted retries → worker writes a synthetic `tool_result` with `exit_code: -1` | `failed` |

> **Running approximation:** The exact `running` status requires querying River's internal `river_jobs` table, which is brittle. Instead: if a `tool_dispatch` event is older than N seconds and no `tool_result` has appeared, the summary endpoint classifies it as `running`. A reasonable threshold is 10 seconds (most Lambda cold starts complete within 5s). This is an approximation — the UI already handles the ambiguity gracefully by showing a spinner for both `pending` and `running`.

---

### SQL for the enriched summary endpoint

The `GetReviewSummary` handler builds `toolSummary` with two queries:

**Query 1 — dispatched tools (complete list):**

```sql
SELECT
    data->>'tool_name'   AS tool_name,
    data->>'tool_id'     AS tool_id,
    created_at
FROM review_events
WHERE review_id = $1
  AND org_id    = $2
  AND event_type = 'tool_dispatch'
ORDER BY created_at ASC;
```

**Query 2 — completed results:**

```sql
SELECT
    data->>'tool_name'                        AS tool_name,
    (data->>'exit_code')::int                 AS exit_code,
    jsonb_array_length(data->'findings')      AS finding_count,
    data->>'stderr'                           AS stderr
FROM review_events
WHERE review_id = $1
  AND org_id    = $2
  AND event_type = 'tool_result'
ORDER BY created_at ASC;
```

The handler merges the two result sets in Go:

```go
// resultMap: tool_name → tool_result row
resultMap := map[string]ToolResultRow{}
for _, r := range results {
    resultMap[r.ToolName] = r
}

breakdown := []ToolBreakdownItem{}
for _, d := range dispatched {
    r, done := resultMap[d.ToolName]
    item := ToolBreakdownItem{ToolName: d.ToolName}

    if !done {
        age := time.Since(d.CreatedAt)
        if age > 10*time.Second {
            item.Status = "running"
        } else {
            item.Status = "pending"
        }
    } else if r.ExitCode != 0 {
        item.Status = "failed"
    } else if r.FindingCount > 0 {
        item.Status = "completed"
        item.CommentsGenerated = r.FindingCount
    } else {
        item.Status = "clean"
    }

    breakdown = append(breakdown, item)
}
```

**Credits** per tool come from `available_tools.multiplier` joined by `tool_id` from the dispatch events — no Lambda call needed.

---

### What happens when River exhausts retries (error path)

If the Lambda call fails after all River retries, the worker writes a synthetic failure event before returning:

```go
// In ToolInvocationWorker.Work(), after final retry failure:
store.InsertReviewEvent(ctx, InsertReviewEventParams{
    ReviewID:  args.ReviewID,
    OrgID:     orgID,  // always from review row, never job args
    EventType: "tool_result",
    Data: json.RawMessage(fmt.Sprintf(
        `{"tool_id":%d,"tool_name":%q,"exit_code":-1,"findings":[],"stderr":"Lambda invocation failed after retries"}`,
        args.ToolID, args.ToolName,
    )),
})
```

This ensures the UI never shows a tool stuck in `pending` forever — it will transition to `failed` once River gives up.

---

### Frontend: building `ToolAccountingData` from real API data

Once the enriched summary endpoint is live, the frontend replaces the mock `setToolAccounting(...)` calls with a direct mapping from `summary.toolSummary`:

```typescript
// In fetchReviewDetails, after getReviewSummary():
if (summaryData.toolSummary) {
    setToolAccounting({
        totalToolCredits:        summaryData.toolSummary.totalCostUsd ?? 0,
        toolsExecuted:           summaryData.toolSummary.toolsExecuted,
        totalCommentsGenerated:  summaryData.toolSummary.totalCommentsGenerated,
        toolBreakdown:           summaryData.toolSummary.toolBreakdown,
    });
} else {
    setToolAccounting(null); // hides the tools tab if no tools ran
}
```

The `status` field on each `ToolBreakdownItem` comes directly from the backend merge logic above — the frontend does not re-derive it.

---

### Summary: full status lifecycle

```
Fan-out trigger
  │
  ├─ INSERT tool_dispatch (status: "pending")  ← UI sees: pending
  └─ riverClient.Insert(ToolInvocationJobArgs)
        │
        ├─ [job starts running]               ← UI sees: running (age > 10s)
        │
        ├─ Lambda returns 200
        │     └─ INSERT tool_result (exit_code, findings)
        │           ├─ findings > 0           ← UI sees: completed
        │           └─ findings = 0           ← UI sees: clean
        │
        └─ Lambda fails / retries exhausted
              └─ INSERT tool_result (exit_code: -1)
                                              ← UI sees: failed
```
