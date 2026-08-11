# Replacing the hand-written analytics schema with dbctx

> **Revised after running dbctx locally against the LiveReview dev DB** (`go get
> github.com/shrsv/dbctx@latest`, `dbctx.Build` against `DATABASE_URL` from
> `.env`). The README's Library section is stale relative to the actual
> `v0.0.0-20260811083710-faff4ccc88cc` API, and one empirical finding
> invalidates the views-based isolation this plan originally proposed. See
> "What running it actually showed" below — that section is normative, the
> rest of the document has been updated to match it.

Companion to `livi_analytics_flow.mmd`. This document is the implementation
detail behind the "Schema source" box added to that diagram.

## Context

`prompts/analytics_schema.md` hand-lists every table and column the analytics
SQL path may read (`internal/mcpagent/prompts/analytics_schema.md`), and its
own comment in `prompts.go` admits why: it has to *mirror* the shadow
allowlist in `internal/livisql/catalog.go` by hand. Two problems fall out of
that:

1. **Drift.** Add a column to a shadow in `catalog.go` and forget the prompt
   (or the reverse) and the LLM either can't see a column it's allowed to use,
   or is taught about a column that no longer exists. Nothing catches this
   except a human diffing two files that don't look alike.
2. **No semantics.** The hand-written doc has to manually explain `status`'s
   four values, that `author_username` is the reviewer, etc. That knowledge
   is sitting in `pg_stats` already (cardinality, distinct values) — we're
   re-typing what Postgres can tell us.

[dbctx](https://github.com/shrsv/dbctx) (`github.com/shrsv/dbctx`) compiles a
Postgres schema into exactly this kind of compact, LLM-ready text —
tables/columns/FKs, state/categorical field detection with representative
values, JSONB path inference — without an LLM and without touching row data
beyond `pg_stats` sampling. It's a Go library: `dbctx.Build(ctx, dsn, opts)` →
`*Index`, then `idx.Query(text)` or `idx.All()` / `idx.Include(names...)` →
`.Text()`.

The goal: delete the hand-written `### Tables` section of
`analytics_schema.md` and generate it from dbctx pointed at the real schema,
so `catalog.go` becomes the *only* place table/column names are typed.

## The constraint dbctx doesn't know about: tenancy and role

dbctx introspects whatever schema/tables you point it at and samples real
column data (via `pg_stats` and `TABLESAMPLE`) to build its representative
values and JSONB paths. Pointing it at `public.reviews` directly would be
wrong for two reasons specific to this app:

- **Withheld columns.** `catalog.go`'s shadows deliberately drop
  `users.password_hash`, `users.onboarding_api_key`, and `reviews.metadata`
  (raw diff/PR JSONB — no analytics value, and `metadata` sampling would leak
  arbitrary PR content into a schema doc). dbctx has no notion of this; it
  would introspect and describe all of them.
- **Cross-tenant leakage via JSONB sampling.** dbctx's JSONB path inference
  samples *actual values* from the table via `TABLESAMPLE` to show examples
  (`$.repository.name string`). If it ever samples a jsonb column across all
  orgs, an org A prompt could show example values that came from org B's
  rows. State/categorical detection for plain columns is safe (`status` has
  the same 4 values everywhere), but arbitrary JSONB sampling is not — this
  is exactly why `metadata` must never be exposed to dbctx at all, not just
  filtered at query time.
- **Role gating.** `loc_usage_ledger` / `org_billing_state` are owner-only.
  dbctx has no concept of caller role.

Row-level org scoping is *not* dbctx's job — `livisql`'s shadow-CTE rewrite
already enforces that at query execution time and is untouched by this plan.
dbctx only ever produces the **schema description** injected into the system
prompt, never touches real query execution, and must only ever see the
columns a role is already allowed to see.

## What running it actually showed

The original draft of this plan proposed pointing dbctx at Postgres **views**
(`livi_analytics.<name>`) mirroring the shadow bodies, so dbctx would
structurally never see withheld columns. Running dbctx locally against the
dev DB disproved this mechanism outright:

```
$ dbctx.Build(ctx, dsn, &Options{Schemas: "livi_analytics"})
1/4 Extracting schema (schemas=livi_analytics)...
  0 tables, 0 constraints
```

`internal/schema/extract.go:47` in the dbctx source filters
`WHERE c.relkind IN ('r', 'p')` — ordinary and partitioned tables only.
Views (`v`) and materialized views (`m`) are invisible to it. **dbctx cannot
introspect a view, full stop**, so the "views as an isolation boundary"
design is dead; there is no schema-level way to hide columns from it short of
column-level `GRANT`/`REVOKE` on the real tables for the DB role dbctx
connects as (a bigger operational change than this plan should take on for a
prompt-generation feature).

Two more things surfaced by pointing it at the real `public.reviews` table
directly (`Options.Schemas` defaults to `"public"`; also confirmed
`Options.Schemas` is a comma-separated **string**, not `[]string`, contrary
to the README):

1. **JSONB sampling reads real content.** `reviews.metadata` — deliberately
   excluded from `catalog.go`'s shadow precisely because it holds diff/PR
   content — got fully expanded: real file paths
   (`internal/api/diff_review.go`, `internal/staticserve/.../FeedbackPopup.js`),
   AI connector names, per-review USD costs, and truncated review comment
   text all showed up as JSONB path samples. Confirms the shadow's exclusion
   was correct and shows exactly what's at stake if dbctx is ever pointed at
   a table without column filtering downstream.
2. **Representative values are unscoped by tenant.** `ColumnInfo.Values` and
   `pg_stats`/`TABLESAMPLE` sampling (`internal/analyze/fields.go`,
   `internal/analyze/jsonb.go`) run over the **whole table, no `org_id`
   filter, no view in between even if one existed**. On the single-org dev DB
   this showed `repository {git-lrc, LiveReview}` and
   `author_username {athreyac4}` as "representative values" — harmless here
   because there's one org, but on the real multi-tenant DB the exact same
   mechanism would show org A's repository names, usernames, and emails as
   example values in org B's system prompt. This is a tenant-boundary bug,
   not a nice-to-have to fix later — the *views* design in the original
   draft wouldn't have caught it either, since an unfiltered view has the
   same unscoped rows as the table underneath it.

**Revised design: dbctx runs against the real `public` schema (no views —
they don't work), and the code that renders its output into the prompt does
two things `catalog.go` already gives it for free:**

1. **Column allowlist filter.** Never call `Selection.Text()`/`TextRaw()`
   directly on an unfiltered `TableContext`. Walk `TableContext.Columns` and
   keep only the column names present in that table's `shadow.body` `SELECT`
   list from `catalog.go` (parsed once, cached) — this is exactly the
   projection a view would have applied, just enforced in Go at render time
   instead of in the database. `reviews.metadata`,
   `users.password_hash`, `users.onboarding_api_key` are dropped here because
   they're absent from the shadow's column list, same as today.
2. **Value/sample suppression, with one carve-out for JSONB structure.**
   Never render `ColumnInfo.Values` (representative values for
   state/categorical columns) or `JSONBPathInfo.SampleValues` — full stop,
   not just for withheld columns. dbctx's structural facts (type, nullable,
   PK, FK target, `IsState`/`IsCategoric` flags) are safe: they're the same
   across tenants and catch drift. The *values* dbctx infers from data are
   not safe to reuse verbatim in a multi-tenant prompt, because they were
   never computed per-tenant. Known-safe enums that are genuinely fixed by
   application code (`status`, `vote_type`, `event_type`/`level`,
   `provider`) stay hand-written in `analytics_schema_intro.md` exactly as
   today — dbctx *confirms* a column is state-like, it doesn't get to supply
   the tenant-unsafe value list for it.

   **The carve-out**: `JSONBPathInfo` splits structure from content —
   `Path` + `InferredType` (e.g. `$.repository.name string`,
   `$.preloaded_changes[].FilePath string`) versus `SampleValues` (the
   actual sampled content of that key across rows). The first half is safe
   to render *if a JSONB column is ever added to the allowlist*: key names
   and their nesting/array shape are schema, not data — every org's
   `metadata` blob (were it ever exposed) uses the same key names, so
   listing them is no different from listing a regular column's name and
   type. Only `SampleValues` carries tenant content and stays suppressed,
   same as `ColumnInfo.Values`. See "Why the model doesn't need to see a row
   to avoid hallucinating a JSONB path" below for why this is sufficient —
   no live per-request row sample is needed to prevent the model from
   inventing keys that don't exist.

### Why the model doesn't need to see a row to avoid hallucinating a JSONB path

The failure mode this guards against is the model inventing a key that
doesn't exist (`metadata->>'connector'` when the real key is
`ai_connector_name`) or misjudging a key's shape (treating
`preloaded_changes` as a scalar when it's an array of objects, so
`metadata->>'preloaded_changes'` silently returns null instead of
`metadata->'preloaded_changes'->0->>'FilePath'`). Both of those are answered
by **`Path` + `InferredType` alone** — the full key path including
array/object nesting, and the scalar type. Neither requires knowing what any
given row's value actually *is*: the model was never going to compute
anything from a literal value it saw in the prompt anyway, since SQL
execution against real rows happens downstream in Postgres, not in the
model's head. So structure-only JSONB rendering fully closes the
hallucination gap without reopening the "the model never sees a data row"
principle this whole design protects — no live, org-scoped sample query
needed at prompt-build time.

(No JSONB column is in `catalog.go`'s allowlist today — `reviews.metadata`
and `review_events.data` are both excluded — so this section is forward
guidance for whenever one gets added, not a current gap.)

This is a weaker use of dbctx than originally planned — it becomes a
**structure and drift-detection layer** (table exists, column exists, type
matches, FK graph, is-this-column-enum-shaped) rather than a full generator
of the `### Tables` prompt section including example values. That's the
correct scope given what actually leaks; a design that traded a real
tenant-isolation bug for solving a documentation-drift annoyance would be a
regression, not an improvement.

This still means `catalog.go`'s `shadow` bodies become the single source of
truth for two things that today are duplicated:
1. the CTE rewrite target / role-scoped table allowlist (existing, unchanged)
2. the column allowlist dbctx's output is filtered through before rendering
   (new — replaces the "view DDL" idea, same effect, enforced in Go)

## Package layout

```
internal/livisql/
  catalog.go            UNCHANGED — shadow structs, CatalogFor, Tables()
  shadow_columns.go      NEW — parses each shadow.body's SELECT list into
                          []string once (regex on the existing string, not a
                          full SQL parser — the bodies are hand-written and
                          simple); exposes ColumnsFor(table string) []string.
                          This is the "column allowlist" both the CTE rewrite
                          comment and the dbctx renderer now read from.
  shadow_columns_test.go NEW — drift guard: every name ColumnsFor returns
                          must exist in dbctx's live TableDetail for that
                          table (catches a shadow typo or a renamed real
                          column before it reaches the LLM as either an
                          allowed or a silently-dropped name)

internal/mcpagent/
  schema_index.go         NEW — package-level *dbctx.Index singleton;
                           dbctx.BuildAsync(ctx, dsn, nil) at startup against
                           the default "public" schema, in-memory (no .dtx
                           file — schema is only known after migrations run;
                           rebuild ~2-3s for this table count per the local
                           run's timing)
  schema_render.go         NEW — dbctxTableText(role) string: for each table
                           in livisql.CatalogFor(role).Tables(), pulls
                           idx.TableDetail(name), filters Columns to
                           livisql.ColumnsFor(name), renders type/nullable/PK/
                           FK/state-flag per column. NEVER renders
                           ColumnInfo.Values (see "What running it actually
                           showed" above). For any JSONBPaths present, renders
                           Path + InferredType only, never SampleValues (see
                           "Why the model doesn't need to see a row..." —
                           currently a no-op since no JSONB column is
                           allowlisted yet, but the renderer should get this
                           right from day one rather than bolt it on later).
                           Own small formatter, not Selection.Text()/
                           TextRaw(), because those render values
                           unconditionally and undo the point of this file.
  prompts.go               MODIFIED — analyticsSchema splits into two embeds:
                           analyticsSchemaIntro (prompts/analytics_schema_intro.md:
                           the "Answering data questions with SQL" prose +
                           the state-value enums that are safe because
                           they're fixed by application code, not learned
                           from data + Rules + Worked examples) and the
                           dynamic table section, no longer embedded
  agent.go                 MODIFIED — buildSystemPrompt gains a role param
                           (already available at both call sites via
                           mcpSession.UserRole); assembles:
                           analyticsSchemaIntro + dbctxTableText(role) +
                           analyticsPlanInstructions
  prompts/analytics_schema.md   RENAMED to analytics_schema_intro.md, the
                           column-by-column ### Tables listing deleted (now
                           generated); the four-value `status` enum and
                           similar hand-verified-safe value lists move here
                           as prose since dbctx no longer supplies any values
```

No `db/migrations/` changes and no new Postgres schema/views — confirmed
locally that dbctx cannot see views, so there is nothing to migrate.

## Startup sequence

1. Server boot, after migrations run (schema is stable):
   `dbctx.BuildAsync(ctx, dsn, nil)` against the real `public` schema,
   non-blocking, in-memory. Per dbctx's own docs, calls made before the build
   finishes block automatically, so there's no cold-start race to handle
   explicitly.
2. At prompt-build time (`buildSystemPrompt`, `analytics.go:66`):
   `dbctxTableText(role)` iterates `livisql.CatalogFor(role).Tables()`
   (~10 tables today), calls `idx.TableDetail` per table, and renders through
   the column-allowlist + no-values filter above — never a raw
   `Selection.Text()` call on unfiltered dbctx output. Table count is small
   enough that per-question relevance filtering (`idx.Query(userQuestion)`)
   isn't worth the added complexity yet; revisit past ~50 tables per dbctx's
   own stated threshold.

## What stays hand-written

dbctx describes *structure* (columns, types, PK/FK, whether a column is
enum-shaped). It has no opinion on, and — per the tenant-leakage finding
above — is not trusted to supply:
- **the actual enum values** for state/categorical columns (`status`'s 4
  values, `vote_type`'s up/down, `event_type`/`level`). These are fixed by
  application code, verified once by a human, and safe to hand-write because
  they don't vary per tenant — unlike `repository`/`author_username`, which
  dbctx also flags as categorical but whose value sets are exactly the
  tenant data this plan must not leak.
- the SQL dialect rules (`## Rules`: alias every column, allowed function
  list, no `WITH RECURSIVE`, compute `lag()` in SQL not Go)
- worked examples (`### Worked examples`: monthly counts, top reviewers,
  month-over-month % change)
- which timestamp answers which question, `COALESCE(completed_at,
  started_at, created_at)` guidance

These stay in `analytics_schema_intro.md`, hand-maintained exactly as today.

## Rollout / risk

- **New runtime dependency**: `github.com/shrsv/dbctx`
  (`v0.0.0-20260811083710-faff4ccc88cc` as of this writing, reachable via
  `proxy.golang.org` — confirmed by actually `go get`-ing it). Its own
  transitive deps (`modernc.org/sqlite`, `modernc.org/libc`) are pure Go, so
  it doesn't break `CGO_ENABLED=0` Docker builds — confirmed by building the
  scratch test binary locally with default `CGO_ENABLED` unset and no cgo
  toolchain errors. The raw-SQL parser decision in `livi_analytics_plan.md`
  hit this exact constraint once already, so it was worth checking rather
  than assuming.
- **API surface is younger than its docs.** The README's Library section
  (`idx.All()`, `Options.Schemas []string`, single-value `Stats()`/`Tables()`)
  does not match the real API on the version actually pulled
  (`ResultSet.All()`/`.Matched()`/`.Include()` exist but `*Index` itself has
  no `All()`; `Stats()`/`Tables()` return `(T, error)`; `Options.Schemas` is
  a comma-separated string). Pin the version once implementation starts and
  re-verify against `go doc github.com/shrsv/dbctx` rather than the README
  when writing `schema_index.go`.
- **Fallback**: keep a minimal static table list as the safety net in
  `analytics_schema_intro.md` — if `schema_index.go`'s index build errors
  (`Index.Err()`), log and fall back rather than sending the LLM an empty
  `### Tables` section. Two separate log events, not one: `schema_index.go`
  logs the build failure itself via `zerolog` at the point it happens
  (process startup, no session id exists yet — see `livi_analytics_plan.md`'s
  Observability section), and `schema_render.go` calls
  `ChatTurnLogger.SchemaSourceDegraded(reason)` on every individual chat turn
  that actually rendered the fallback instead of the live index. The second
  one is the one that matters for debugging a specific bad answer — "the
  index failed to build at 09:14" doesn't tell you which of the next
  thousand turns were affected by it, but a `SchemaSourceDegraded` line in
  that turn's own `chat_debug_logs/chat_debug.log` entry does.
- **Test**: `shadow_columns_test.go` is the drift guard — it fails the moment
  `catalog.go`'s shadow SELECT list names a column dbctx's live
  `TableDetail` doesn't have (typo, renamed column, dropped column), which is
  strictly more useful than today's `catalog_test.go` (checks against
  `db/schema.sql`, a file that can itself drift from the real DB) because it
  checks against the actual running schema.
- **No change** to `livisql/guard.go`, the CTE rewrite, or query execution —
  this plan only touches how the schema section of the prompt is produced.
- **Scope reduction from the original draft**: this no longer eliminates
  `analytics_schema_intro.md` — it eliminates the *column-drift* class of bug
  and adds structural verification, while the actual enum values a
  tenant-shared LLM prompt can safely mention stay hand-verified. That's a
  smaller win than "delete the hand-written schema doc entirely," but it's
  the version that doesn't introduce a cross-org data leak to fix a
  documentation-maintenance annoyance.
