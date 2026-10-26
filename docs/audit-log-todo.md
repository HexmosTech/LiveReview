# Audit Log — Physical TODO

Status: **Phases A/B fully implemented; Phase C implemented except the three
deliberate follow-ups listed at the end.** This file is the on-disk
progress log; see `docs/audit-log-plan.md` for the design and
`docs/audit-events.md` for the action contract.

Legend: `[x]` done + verified, `[~]` partially done (see note), `[ ]` not done.

## Phase A — Framework + usable slice

### Schema
- [x] Migration: `audit_events` monthly range-partitioned table + indexes
- [x] Migration: append-only trigger (`UPDATE` always rejected; `DELETE` only
      with `app.audit_purge=on`)
- [x] Migration: `audit_archives` table (org, month, object key, rows, sha256)
- [x] Migration: `api_keys.kind` (`standard` | `audit_reader`)
- [x] Migration: legacy import from `user_management_audit` (42 rows locally)
- [x] Migration: backfill historical super-admin rows to `admin` capacity so
      owners cannot see them

### Framework package `internal/auditlog`
- [x] `catalog.go` — typed actions/categories/severities + labels
- [x] `event.go` — Event, Fields, Details, Changes
- [x] `sanitize.go` — redaction, truncation, size cap
- [x] `echo.go` — request-id/actor/org/auth-method enrichment
- [x] `recorder.go` — Record / RecordTx / System / Webhook / AIInvocation
- [x] `spec.go` + registry — route specs and coverage validation
- [x] `query.go` — visibility-scoped query builder, filters, cursor
- [x] `integrity.go` — hash-chain verification + latest tip
- [x] Unit tests: sanitizer, catalog/docs, scoping, cursor, coverage

### API wiring
- [x] Server struct: audit recorder + registry + maintenance manager
- [x] Global `auditMiddleware` records every non-GET `/api/v1` request
- [x] `s.audited(...)` route+spec registration helper
- [x] Auth events: login success/failure, logout, password change, token refresh
- [x] `users.last_login_at` update on successful login
- [x] Sensitive-read events: audit list/detail/export/archive, taxonomy
      CSV/XLSX, onboarding report download, chat CSV/PDF/compile exports
- [x] `audit_handlers.go`: list, meta, facets, detail, CSV/NDJSON/package
      export, archive download, AI usage
- [x] Admin handlers: cross-org list, integrity, settings GET/PUT,
      run-maintenance
- [x] Audit-only key: create/list/badge, enforced on every non-audit route
- [x] Retention/maintenance manager: partitions, archive, prune, anchor
- [x] Server start/stop wiring

### UI
- [x] `AuditLog.tsx`: range presets + custom, category/action/outcome/actor/
      search filters, live 5s tail, table, detail drawer, exports, archive,
      AI usage tab, super-admin org selector, retention/integrity panel
- [x] `App.tsx` lazy route `/reports/audit`
- [x] Mega menu: Reports → Audit Log
- [x] API Keys tab: key-kind selector + "Audit only" badge
- [x] `npx tsc --noEmit` clean
- [x] Webpack dev bundle compiles successfully (315 src modules)

### Docs
- [x] `docs/audit-events.md` catalog (test-enforced)
- [x] `docs/audit-log-plan.md`
- [x] `docs/audit-log-todo.md`
- [x] `internal/docindex/docs/routes_guide/reports/audit-log.md`
- [x] `AGENTS.md` → "Audit Logging Is Automatic"

## Phase B — Retention + integrity
- [x] Partition manager (13 months ahead; DEFAULT safety net)
- [x] Archive export to blob store (NDJSON.gz per org + SHA-256)
- [x] Default-partition archiving for legacy/historical rows
- [x] Archive prune after the retention window
- [x] Archive download endpoint + UI button
- [x] Per-org hash chain (seq/prev_hash/hash, advisory lock)
- [x] Daily anchor file (`audit-archive/anchors/YYYY-MM-DD.json`)
- [x] Integrity check endpoint + super-admin UI panel
- [x] `audit_settings` GET/PUT (hot days, retention, archive, anchor, hash,
      cron)
- [x] Evidence package (zip = NDJSON + manifest with SHA-256)

## Phase C — AI + enterprise
- [x] Review AI usage: surfaced from `loc_usage_ledger` via
      `/audit-events/ai-usage` + UI tab (provider, model, tokens, cost, actor,
      review)
- [x] Chat/Livi AI usage: `AIInvocation` events with prompt SHA-256, byte size,
      provider/model, tokens, latency, outcome
- [x] Evidence package export
- [x] Super-admin retention controls
- [~] Plan-gated retention override: **global policy is implemented**; wiring a
      per-org enterprise-plan override into the plan catalog is deliberately
      deferred (commercial/pricing decision, not an audit-module change)
- [~] General API-key scope system: the dedicated `audit_reader` key is the
      agreed v1 boundary. Enforcing scopes on every other endpoint is a
      separate hardening project.
- [~] SIEM push webhooks: v1 is pull-based (audit-only key + NDJSON/CSV). A
      signed push forwarder is deferred.
- [x] Final regression pass (see below)

## Known limitations / follow-ups

1. **User-service domain events lack IP/user-agent/request-id.** They are
   written inside the business transaction (fail-closed) but the service layer
   has no Echo context. The HTTP middleware's generic row is skipped because
   the handler marks the request as audited. Threading request metadata into
   the service is a follow-up; actor + target + before/after are present.
2. **Archive/restore is download-only.** Months beyond the hot window are
   downloadable NDJSON.gz, not re-imported into a searchable staging table.
   This matches the "90 days instantly searchable" decision.
3. **`internal/mcpagent` test suite has pre-existing failures** unrelated to
   this work (`TestCountQueryBranchRejectsTruncatedJSON`,
   `TestDispatchClassifyFailureDegradesToChat`, `TestLawbookCarriesMachineContracts`).
   The package compiles and its audit changes are additive.
4. **`go build ./...` fails on a pre-existing package conflict** in `tests/`
   (`livereview` vs `main` in the same directory). App packages build cleanly
   with `go build ./internal/... ./cmd/... ./pkg/... ./storage/...`.

## Verification log

| Check | Result |
|---|---|
| baseline `npx tsc --noEmit` | clean |
| `go build ./internal/...` | clean |
| `go test ./internal/auditlog/...` | pass |
| `go test ./internal/api/... -run '^$'` | all packages compile |
| `go test ./internal/api/users/... ./internal/mcpagent/... -run '^$'` | compile clean |
| migrations apply | applied; 42 legacy rows imported |
| append-only trigger | UPDATE/DELETE rejected; purge escape works |
| server boots | health 200 |
| audit list (audit key) | 200, events returned |
| audit key on non-audit route | 403 |
| standard key on audit route | 403 |
| audit key on admin route | 403 (fixed) |
| duplicate fallback row | fixed; verified one `api_key.create` only |
| failed login + success | recorded; owner sees both; member sees own |
| user update via owner key | `user.update` with resource id + details, no duplicate |
| org role change via owner key | `org.member.role.changed` with old/new role, no duplicate |
| facets / meta / detail / search | 200 |
| CSV / NDJSON / evidence package | 200; package has manifest |
| admin cross-org export | route added (`/admin/audit-events/export`), session-only |
| admin settings GET/PUT | 200; API key rejected (403) |
| integrity org 151 | 16/16 verified, intact |
| visibility rules | unit-tested: owner hides admin/super-admin, member self-only, super-admin org action gets admin capacity |
| visibility live check | super admin saw 33 rows / 30 admin; owner saw 3 rows / 0 admin / 0 super-admin; member saw only their own 1 login (failed login hidden) |
| UI layout | matches Reviews: `container mx-auto px-4 py-8`, 1280px centered at 1440; `SimpleTable` shell (#1F2836/#2A3340), px-6 cells, fixed column widths, two-line timestamps, footer; verified headless |
| archive default rows | 9 archive rows; partition/default rows pruned |
| archive download | 200; gzip valid; SHA-256 matches DB |
| anchor file | written (`audit-archive/anchors/2026-10-25.json`) |
| AI usage (ledger) | 200; 172 rows returned for an active org |
| UI webpack dev bundle | compiled successfully, no TS errors |
