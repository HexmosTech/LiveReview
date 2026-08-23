# Schema drift detection for the dbctx pre-built `.dtx` path

## Context

Shrijith's RCA (LiveReview local-dev outage) surfaced two related problems:

1. **Nobody should have to remember "if you change the DB and terminology,
   redo the .dtx by hand."** That's exactly the kind of step a person
   forgets under pressure. The system must detect staleness itself, not
   depend on discipline.
2. Concretely: `dbctx` needs a **cheap schema fingerprint**, stored inside
   the `.dtx` file at build time. On every load, LiveReview recomputes the
   *live* DB's fingerprint and compares it to the one stored in the `.dtx`.
   A mismatch means the index is stale relative to the real schema, and
   **must** force a hard failure — never proceed quietly on stale data,
   never a soft warning that's easy to miss.

This directly extends the hard-fail machinery already built this session
(`schemaIdxHardFailed`, `hardFailOut`, the "missing .dtx file" case in
`internal/mcpagent/schema_index.go`) — staleness becomes a second reason to
trip the same gate, not a new mechanism.

Scope: only the **pre-built `.dtx` path**
(`DBCTX_SCHEMA_INDEX_ENABLED=false`). The default async in-memory build
path (`dbctx.BuildAsync`) always rebuilds fresh from the live schema on
every server restart by construction (see that branch's own doc comment)
— it cannot go stale, so no fingerprint check is needed there.

Fingerprint scope: **table/column shape only** — `(schema, table, column,
data_type, nullable)` tuples, sorted and hashed. Deliberately excludes
constraints/indexes so unrelated churn (e.g. adding an index) doesn't force
unnecessary rebuilds — only changes that would actually make dbctx's
retrieval/schema text wrong (added/dropped/retyped columns and tables)
count as drift.

## Part 1 — `dbctx` library (`/home/lovestaco/hex/lr/dbctx`)

Confirmed via exploration:
- `internal/schema/extract.go`: `Extract(ctx, pg, schemas) (*ExtractedSchema, error)` already returns exactly the `Table`/`Column` data needed — reuse it, don't write a new query.
- `internal/db/sqlite.go`: `InitSchema()` already creates a generic, currently-unused `metadata (key TEXT PRIMARY KEY, value TEXT)` table — this is the natural home for the fingerprint, no new table/migration needed (the codebase's own convention for schema evolution is `CREATE TABLE IF NOT EXISTS` on every open, not migrations).
- `internal/db/pg.go`: `Connect`/`ConnectWithMaxConns` + `(*PG).Close` are the existing connection helpers for a fresh live-DB round trip.
- `dbctx.go`: `Index` struct wraps `store *db.Store`; `Stats()` (reads via `idx.store.DB().QueryRow`) is the pattern to follow for a new store-reading method.

**New file `fingerprint.go`:**
- `computeFingerprint(ext *schema.ExtractedSchema) string` — sort `(schema, table, column, data_type, nullable)` tuples deterministically, `sha256` the joined string, hex-encode.
- `func (idx *Index) SchemaFingerprint() (string, error)` — reads `metadata` row (`key = 'schema_fingerprint'`) from `idx.store`, mirroring `Stats()`'s query pattern.
- `func LiveFingerprint(ctx context.Context, dsn string, opts *Options) (string, error)` — `db.Connect`/`ConnectWithMaxConns` (opts may be nil → defaults), `schema.Extract`, `computeFingerprint`, `pg.Close`. No store, no file written — read-only comparison primitive.

**Modify `Build`/`BuildAsync` in `dbctx.go`:** right after the existing `schema.Extract` call, compute the fingerprint from that same `ext` result (no extra query) and `INSERT OR REPLACE INTO metadata(key, value) VALUES ('schema_fingerprint', ?)` via the store.

**Tests — new `dbctx_fingerprint_test.go`** (follow `dbctx_terminology_test.go`'s style, `newTestIndex`/`newTestIndexFile` helpers from `dbctx_test.go`):
- Fingerprint is non-empty and deterministic across two builds of the same fixture.
- Fingerprint changes when the fixture schema changes (add/drop/retype a column).
- `LiveFingerprint` against the fixture DSN matches `SchemaFingerprint()` of a `.dtx` just built from it.

**Docs — `README.md`:** add a "Schema fingerprint" section (near the existing "Terminology" section, same style: short explainer + code block) documenting `SchemaFingerprint()` and `LiveFingerprint()` — what they're for (cheap drift detection without a full rebuild), the exact fields hashed (table/column shape only, not constraints/indexes), and a minimal usage example:
```go
stored, _ := idx.SchemaFingerprint()
live, _ := dbctx.LiveFingerprint(ctx, dsn, nil)
if stored != live {
    // schema drifted since this .dtx was built — rebuild before trusting it
}
```
Also add both new symbols to the package doc comment / godoc if `dbctx.go`'s top-level doc lists public API surface.

**Release:** implementation + docs only in this pass — no commits, no version tag/push. Tagging `v0.1.4` and updating LiveReview's `go.mod` happens as a separate, explicit step once the code is reviewed.

## Part 2 — LiveReview integration (`internal/mcpagent/`)

**`go.mod`:** since Part 1 isn't tagged/pushed in this pass, point LiveReview at the local checkout with a temporary directive so it builds against the new API right away:
```
replace github.com/shrsv/dbctx => /home/lovestaco/hex/lr/dbctx
```
(run `go mod tidy` after). This is a placeholder for local dev/testing only — swap it for a real `go get github.com/shrsv/dbctx@vX.Y.Z` once dbctx is tagged and pushed as its own separate, explicit step.

**`schema_index.go`:**
- Generalize the existing `schemaIdxHardFailed atomic.Bool` into a reason-carrying value: `schemaIdxFailureReason atomic.Value` (holds a `string`; empty = no failure). Add `schemaIndexFailureReason() string` alongside the existing `schemaIndexHardFailed()` (which becomes `schemaIndexFailureReason() != ""`).
- Existing hard-fail sites (missing file, open error, `Stats()` error, 0 tables, build-failed-to-start) each set a specific reason string (`"missing"`, `"open_failed"`, etc.) instead of just `true`.
- **New check**, right after the existing 0-tables guard and before `schemaIdx = idx` is assigned (still inside the `DBCTX_SCHEMA_INDEX_ENABLED=false` branch):
  ```go
  storedFP, fpErr := idx.SchemaFingerprint()
  liveFP, liveErr := dbctx.LiveFingerprint(context.Background(), dsn, nil)
  ```
  - Either call erroring (e.g. DB unreachable to verify) → hard-fail with reason `"unverifiable"`. Per Shrijith: an unverifiable index must not be treated as good — "forced, not quietly ignored" applies to the check itself failing, not just to a confirmed mismatch.
  - `storedFP != liveFP` → hard-fail with reason `"stale"`.
  - Match → proceed exactly as today (log ready, terminology, warm-up).
  - All three paths use the existing `hardFailOut` writer (stdout + `dbctx_debug.log` + `chat_debug.log`) plus `log.Error()`.

**`agent.go`:** replace the two-message branch in `RunTurnWithArtifacts` with a small reason→message table:
- `"missing"` (and other pre-existing hard-fail reasons) → keep current message: *"Livi cannot find the Database Context source, please contact Hexmos team for this."*
- `"stale"` → new, actionable: *"Livi's Database Context is out of date — the database schema changed since it was last built. Run `make prep-dbctx` to refresh it, then restart the server."*
- `"unverifiable"` → *"Livi could not verify its Database Context is up to date (couldn't reach the database to check). Please contact Hexmos team for this."*
- empty reason (still building, transient) → unchanged existing "preparing mode, please come back after 60s" message.

## Verification

- `dbctx`: `go test ./...` (new fingerprint tests plus full existing suite, since `metadata` table and `Build` are shared code paths).
- `LiveReview`: `go build ./internal/mcpagent/... ./internal/logging/...` (per this session's convention — build only, Air handles running).
- Manual (documented for the user, not run by me):
  1. `make prep-dbctx` → confirm normal startup with `DBCTX_SCHEMA_INDEX_ENABLED=false` (fingerprints match, no change in behavior).
  2. `ALTER TABLE` a throwaway column on the local DB, restart → confirm the new `"stale"` hard-fail message appears and the turn is refused, instead of silently proceeding on the old schema.
