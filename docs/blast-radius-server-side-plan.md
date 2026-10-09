# Server-side blast radius scoring

## Status: proposal — not implemented

## Problem

Blast radius only exists for reviews run through `git lrc review`. The CLI
scores the diff locally against a `codebase-memory-mcp` graph of the whole
repo and uploads the report to
`POST /api/v1/diff-review/:review_id/artifacts/blast-radius`.

Reviews triggered by a webhook or from the web UI never get one, because the
LiveReview server has no checkout of the repo and no call graph. Those
reviews show no risk badges, no breakdown panel, and no rows in
`blast_radius_hunks` for Livi to query.

## Solution

Let the server produce the same report the CLI produces, and store it through
the path that already exists. Nothing downstream changes: blob storage, the
Postgres mirror, Livi, and the diff viewer UI all keep working as-is.

```
review finishes (webhook / UI)
  -> enqueue blast_radius job (skip if CLI already uploaded one)
  -> blobless clone / fetch into lrdata volume, checkout head_sha
  -> codebase-memory-mcp index_repository (incremental after first run)
  -> ScoreHunks(review's stored hunks)
  -> blobstore.SaveArtifact(..., "blast-radius", report)
  -> replicateBlastRadiusToPostgres (already exists)
```

## Getting the scoring engine into LiveReview

The engine is git-lrc's `blastradius/` module: ~3,150 lines of Go, stdlib
only (`client/`, `score/`, `diffparse/`, `symbols/`, root package). Its
module path is `github.com/HexmosTech/blastradius` and it is only reachable
through a local `replace` in git-lrc today. Two options:

### Option A — Copy it into LiveReview

- Copy git-lrc's `blastradius/` (minus `cmd/` and `explorer/`) into
  `internal/blastradius/engine/`, rewriting only the import paths.
- Add the `// Ported from git-lrc:blastradius/<file> (as of <sha>)` header
  to every file.
- Never edit the copy in LiveReview. Fixes land in git-lrc, then get
  re-copied. Add a `make sync-blastradius` target that does copy + `sed`
  so a re-sync is mechanical and `diff -r` shows upstream changes.

Pros: no dependency on git-lrc's repo; no git-lrc changes needed.
Cons: two copies drift (the recent sqrt-dampening change already drifted
from LiveReview's Math Mode); every scoring fix must be synced by hand, or
server-scored and CLI-scored reviews rank hunks differently.

### Option B — Import it as a Go module

- In git-lrc, rename the module in `blastradius/go.mod` to
  `github.com/HexmosTech/git-lrc/blastradius` and update git-lrc's own
  imports and `replace` line.
- In LiveReview, `go get github.com/HexmosTech/git-lrc/blastradius@<commit>`.
  No tag or separate repo needed (requires git-lrc to be fetchable by Go).

Pros: one source of truth; picking up a fix is a version bump.
Cons: a small one-time change in git-lrc; LiveReview builds depend on
git-lrc being reachable (or on the Go module proxy cache).

**Decision:** _pending_ — pick A or B before starting step 3.

## Implementation steps

1. **Repo cache** — `internal/blastradius/repocache/`
   - Path: `/app/lrdata/blastradius/<org_id>/<connector_id>/<repo>/`
     (`./lrdata` is already the persistent volume in `docker-compose.yml`).
   - First time: `git clone --filter=blob:none --no-checkout <url>`.
     Later: `git fetch origin <head_sha>`. Then `git checkout --detach <head_sha>`.
   - Blobless, not shallow: `codebase-memory-mcp` reads git history to build
     the file co-change (`FILE_CHANGES_WITH`) signal; `--depth=1` would
     silently zero it.
   - Token comes from `integration_tokens` via the review's `connector_id`.
     Pass it through a credential helper / env, never in a logged URL.
   - One lock per repo dir so two jobs never fetch the same checkout at once.

2. **Graph index**
   - Use the `codebase-memory-mcp` binary already in the Docker image
     (`docker/docker-deps.env`, v0.10.8). git-lrc is tested against v0.9.0 —
     confirm query output still matches with one real run.
   - Confirm where it stores its index and point it inside the per-org repo
     cache dir, so indexes are tenant-isolated and incremental.

3. **Scoring engine** — Option A or B above.

4. **Job** — `internal/jobqueue/blast_radius_worker.go`
   - `BlastRadiusJobArgs{OrgID, ReviewID}`, `Kind() = "blast_radius"`,
     registered like the other workers in `jobqueue.go`.
   - Enqueued by `WebhookReviewWorker` / `ManualReviewWorker` after a
     review completes successfully.
   - Skip if a `blast-radius` artifact already exists (the CLI's local graph
     wins).
   - Score the review's stored `preloaded_changes` hunks — the UI joins on
     `file:new_start:new_lines`, so any other diff source won't match.
   - Save via `blobstore.SaveArtifact` + `replicateBlastRadiusToPostgres`
     (move the latter out of `internal/api` so the worker can call it).
   - Best-effort: timeout (~15 min, same as the CLI), low concurrency (1–2),
     failures logged, never retried in a loop, never affects the review.

5. **On/off switch**
   - Instance setting (default off for self-hosted, since it clones repos to
     disk). Per-org toggle can come later.

6. **Cleanup**
   - Delete a repo's cache dir when its connector is removed.
   - Periodic sweep: drop cache dirs not used in N days.

7. **Docs**
   - CLAUDE.md "Artifact sync channel": note the server can now compute
     blast radius for webhook/UI reviews.
   - `internal/docindex/docs/routes_guide/reviews/review-detail.md`: blast
     radius is no longer CLI-only.

## Scope of the first version

- GitHub first (token + `head_sha` handling already exists in
  `fetchLiveDiffFromMetadata`); GitLab/Bitbucket through the provider
  factory after that.
- No UI changes.

## Risks / open questions

- **Disk**: roughly one source tree + history + graph index per repo.
- **CPU/RAM**: first index of a large repo takes minutes; concurrency must
  stay low so it doesn't starve review workers.
- **Engine version drift**: v0.10.8 (server) vs v0.9.0 (CLI) may score the
  same diff slightly differently.
- **Scores are relative per diff** (normalized to the top hunk), same as
  the CLI — not comparable across reviews.
