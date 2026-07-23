# Third-Party Tools Integration – Beta

LiveReview can run external static-analysis tools (ruff, bandit, eslint, etc.) as parallel Lambda jobs alongside every AI review. Results are stored as `tool_result` events in the existing `review_events` table and surfaced in the review UI and `lrc` CLI output.

This feature is **cloud-only** and **owner-gated**. It is delivered in three sequential phases.

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

#### `POST /api/v1/reviews/tool-reviews`

Triggers a tool-only review execution (without any AI/LLM reviews) on a pull request/merge request diff.

**Access:** Any authenticated org member.  
**Cloud gate:** HTTP 403 if not cloud mode.  
**Execution Flow:**
1. **Pre-flight Credit check**: The API handler queries the DB (`org_tool_billing_state`), calculates the sum of multipliers of all currently enabled tools for the organization, and runs a pre-flight check. If the remaining credit balance is insufficient, the API returns **HTTP 402 Payment Required** immediately before scheduling any jobs.
2. **Review creation**: Creates a review record in the database with `trigger_type = 'tool_review'` and status `processing`.
3. **Queue job**: Schedules the background job `tool_review_orchestrator` with the calculated total multiplier.
4. **Asynchronous credit deduction**: When the background worker executes `ExecuteToolsForReview`, it locks the credit table and transactionally deducts the required credits from the organization's monthly credit allowance.

**Request body:**

```json
{
  "pr_url": "https://github.com/HexmosTech/git-lrc/pull/42"
}
```

**Response 200:**

```json
{
  "review_id": "10023",
  "message": "Tool static analysis scheduled successfully"
}
```

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

### Tool result badges in ReviewDetail

File: `ui/src/pages/Reviews/ReviewDetail.tsx`

When the event stream contains events with `event_type === 'tool_result'`:

- A **tool result section** is rendered below AI findings.
- Each tool result has a badge styled distinctly from AI badges (different colour, labelled with `data.tool_name`).
- If no `tool_result` events exist for the review, the section is hidden entirely.

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

## Manual Testing – Triggering Gitleaks via lrc

This section documents how to manually trigger a tool-based review against the self-hosted instance using a prepared test diff.

### Prerequisites

1. **Build and install lrc locally:**
   ```bash
   cd /home/gk/hex/git-lrc
   make build-local && lrc hooks install
   ```

2. **Run both LiveReview processes** (two terminals):
   ```bash
   # Terminal 1 — API server
   cd /home/gk/hex/LiveReview && ./tmp/livereview server

   # Terminal 2 — Background worker (processes tool jobs)
   cd /home/gk/hex/LiveReview && ./tmp/livereview worker
   ```

3. **Enable Gitleaks in org tool settings:**
   - Log in as owner at `https://manual-talent.apps.hexmos.com`
   - Go to **Settings → Third-Party Tools** → toggle **Gitleaks** on

### Trigger Command

```bash
cd /home/gk/hex/git-lrc

LRC_API_KEY=lr_e5ytg2e2evor6zok3zpwos34g5i4l76x7kekuduw2ogbvrjnnv7q \
  lrc r \
  --tools \
  --diff-file test_cases/gitleaks.txt \
  --force \
  --api-url https://manual-talent.apps.hexmos.com
```

#### Flag reference

| Flag | Purpose |
|---|---|
| `--tools` | Enables static analysis tool execution alongside the review |
| `--diff-file test_cases/gitleaks.txt` | Uses a pre-crafted diff with fake secrets instead of a real git diff |
| `--force` | Skips the interactive commit prompt |
| `--api-url` | Points to the self-hosted instance instead of cloud |
| `LRC_API_KEY` | API key scoped to the owner org (Org ID 3) |

### Test Diff File

**Location:** `git-lrc/test_cases/gitleaks.txt`

Contains a synthetic unified diff with deliberate fake secrets:
- `"database_password": "supersecretpassword123"` — triggers **Critical**
- `"slack_token": "xoxb-1234-5678-abcdet"` — triggers **Critical**

### Verifying Results

1. Open the review URL printed by `lrc` (e.g. `http://localhost:8002/?r=<id>`)
2. The **ISSUE FILTERS** bar should show **N issues visible**
3. Each finding appears in the diff view labelled **CRITICAL** with classification `tool-generated`
4. Comment text reads: *"Gitleaks secret detected: ..."* with the matched secret redacted

---

## Repo-Level Tool Configuration via `.lrc/`

In addition to organization-level tool settings managed via the UI (`org_tools`), repositories can specify tool configurations locally inside their `.lrc/` directory using `.lrc/tools.toml`.

### Specification

Location: `<repo-root>/.lrc/tools.toml`

```toml
[tools]
gitleaks = true
ruff = true
bandit = true
eslint = false
```

### Resolution Logic (Backend `ExecuteToolsForReview`)

When a review is submitted by `lrc`, the `.lrc/tools.toml` file is bundled into the review payload ZIP (`diff_zip_base64`).

During review execution:
1. **Org-Level Tools**: LiveReview fetches enabled tools for the organization from `org_tools` where `enabled = true`.
2. **Repo-Level Tools**: LiveReview parses `.lrc/tools.toml` from the submitted bundle.
3. **Effective Tool Set (Union)**: Any tool enabled in `org_tools` **OR** enabled in `.lrc/tools.toml` (`tools.<tool_name> = true`) will be executed for the review, provided the tool exists in `available_tools`.
4. **Overrides**: If a tool like `gitleaks` is disabled in `org_tools` for the organization, but `.lrc/tools.toml` specifies `gitleaks = true`, LiveReview resolves the Lambda ARN from `available_tools` and triggers the `gitleaks` Lambda execution for this review.




## Tool Config Should also be 