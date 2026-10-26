# LiveReview Audit Log — Implementation Plan

Status: **implementing** (see `docs/audit-log-todo.md` for live progress).

## Goal

Give org owners and super admins a unified, enterprise-grade audit trail of
every meaningful action on the platform — authentication, CRUD, settings,
exports, API-key lifecycle, and AI/model calls — with live browsing, search,
export, retention, and tamper-evidence.

The module is built as a **framework primitive**, not a one-off page: every new
mutating endpoint is captured automatically, and new features get semantic
quality by writing one line.

## Locked product decisions

| Area | Decision |
|---|---|
| Audit write failure | Fail-closed for sensitive actions (user/role, API key, password, billing, org, settings); fail-open with error log for everything else |
| Deleted users/orgs | Rows are immutable and survive removal; no cascade/anonymization. Age-based retention still applies |
| Captured detail | Full IP, user agent, attempted login email, request ID; privacy switches available |
| Readers | Super admin: everything. Owner: own org, excluding super-admin actions. Member: only their own events |
| SIEM | Dedicated audit-only API key; cannot call any other endpoint |
| Retention | 90 days online + 12 months archived; super-admin controlled, enterprise plan override in cloud |
| Read auditing | Changes + sensitive reads (exports, downloads, audit reads, key reveals) |
| LLM calls | Provider/model/purpose/tokens/cost/latency + prompt SHA-256 and size; no raw prompts |
| Tamper evidence | Append-only trigger; per-org hash chain; daily anchor file in blob storage |
| Legacy history | Import `user_management_audit` as `source=legacy` |
| Foundations | Severity, request ID, stable action-name contract, UTC storage |

## Architecture

### Storage

`audit_events`, range-partitioned by month on `occurred_at`:

- Typed columns for filtering: `org_id`, `actor_*`, `action`, `category`,
  `resource_*`, `outcome`, `severity`, `occurred_at`.
- `details JSONB` for action-specific payloads, sanitized before write.
- `source` (`api`/`webhook`/`system`/`legacy`), `legacy_id` for idempotent import.
- Reserved `seq`/`prev_hash`/`hash` for the per-org hash chain.
- Append-only trigger: rejects `UPDATE`; rejects `DELETE` unless
  `app.audit_purge = on` (retention only).
- Monthly partitions created ahead of time; archive + drop after the hot window.

`audit_archives` records each exported month (object key, row count, SHA-256).

`api_keys.kind` adds `standard` vs `audit_reader`.

`system_settings.audit_settings` holds runtime policy.

### Automatic capture

1. Global Echo middleware records every non-GET `/api/v1` request, plus
   allowlisted sensitive GETs. Derived action if no spec exists.
2. `s.audited(...)` registers a route + `audit.Spec` atomically.
3. Privileged handlers call `audit.RecordTx(...)` inside the business tx;
   middleware records failed attempts outside it.
4. Coverage validation at startup + test: new non-GET routes need a spec or an
   allowlist entry with a reason.
5. `auditlog.QueryFor(scope)` enforces org scope and visibility rules for all
   audit reads. Handlers never hand-write audit WHERE clauses.

### Visibility

- Super admin: all orgs, all events.
- Owner: `org_id = scope.OrgID AND actor_capacity <> 'admin' AND actor_kind <> 'super_admin'`.
- Every super-admin action is recorded with `actor_capacity=admin`; omitting it
  is never allowed. A one-time migration normalizes rows written before this
  rule was introduced (the append-only trigger is disabled only for that
  transactional migration).
- Member: `org_id = scope.OrgID AND actor_user_id = scope.UserID`.
- Audit-only key: acts for its creator's live role, so it inherits the same
  visibility; it can call only audit read/export/archive endpoints.

### Retention & archive

- Hot window: 90 days in Postgres.
- Monthly archive: NDJSON.gz per org under
  `audit-archive/org/<org_id>/<YYYY-MM>.ndjson.gz`; manifest + SHA-256.
- Total retention: 12 months of archives.
- Daily maintenance job: ensure partitions, archive+drop old partitions, prune
  expired archives, write the daily hash anchor.
- UI beyond the hot window offers archive download rather than instant search.

### Hash chain

- Per org, `seq` starts at 1; `hash = SHA-256(prev_hash || canonical event)`.
- Serialized with a per-org advisory lock.
- Daily anchor: latest seq/hash per org written to blob storage.
- Admin integrity endpoint recomputes the chain and reports the first break.

### AI/model events

Emitted from central usage points with provider, model, purpose, token counts,
cost, latency, outcome, prompt SHA-256, and prompt size. Raw prompts are never
stored.

## API

```
GET  /api/v1/orgs/:org_id/audit-events
GET  /api/v1/orgs/:org_id/audit-events/meta
GET  /api/v1/orgs/:org_id/audit-events/facets
GET  /api/v1/orgs/:org_id/audit-events/:id
GET  /api/v1/orgs/:org_id/audit-events/export?format=csv|ndjson|package
GET  /api/v1/orgs/:org_id/audit-events/archive?month=YYYY-MM
GET  /api/v1/admin/audit-events
GET  /api/v1/admin/audit-events/integrity
GET/PUT /api/v1/admin/audit-settings
```

API-key creation accepts `kind: "audit_reader"`; audit-only keys are owner-
created and cannot call non-audit routes.

## UI

- `/reports/audit` with range presets, filters, live polling, table, detail
  drawer, export menu, archive download, retention banner.
- Super admin: org selector, retention controls, integrity check.
- Settings → API Keys: key-kind selector and badge.
- Mega menu: Reports → Audit Log (owner/super admin).
- Chatbot route guide: `internal/docindex/docs/routes_guide/reports/audit-log.md`.

## Framework rules

The authoritative developer rules live in `AGENTS.md` under
"Audit Logging Is Automatic". Summary:

1. Register mutation routes with `s.audited(...)`.
2. Privileged handlers use `audit.RecordTx(...)` in the same tx.
3. Never log secrets/bodies; use `audit.Details(...)`.
4. Exports/downloads/audit reads use `audit.SensitiveRead(...)`.
5. Background/webhook work uses `audit.System(...)`; LLM calls use
   `audit.AIInvocation(...)`.
6. Audit reads use `auditlog.QueryFor(...)`.
7. Every action appears in `docs/audit-events.md`; names are a stable contract.
8. `go test ./internal/auditlog/...` enforces route coverage.

## Implementation status

Implemented and smoke-tested. See `docs/audit-log-todo.md` for the verified
checklist and known limitations. Phase C's plan-gated retention override,
general API-key scopes, and SIEM push remain deliberate follow-ups; the
audit-only key + NDJSON/CSV pull path is the v1 SIEM integration.

## Phases

- **A — Framework + usable slice:** schema, catalog, recorder, middleware,
  request IDs, visibility query builder, auth events, sensitive reads, read API,
  UI, audit-only key, legacy import, docs, AGENTS.md.
- **B — Retention + integrity:** partition manager, archive/download, prune,
  hash chain, daily anchor, integrity endpoint, settings.
- **C — AI + enterprise:** LLM invocation events, evidence package export,
  archive browsing, integrity UI, plan-gated retention (follow-up).

## Non-goals / deliberate deferrals

- Raw prompt/response storage (opt-in only, not enabled).
- General API-key scope enforcement for every endpoint (the audit-only key is
  the v1 answer; a global scope system is a separate hardening project).
- SIEM push webhooks (pull via audit-only key + NDJSON is the v1 integration).
- Full-text search over `details` (filters + ILIKE on typed columns v1).
