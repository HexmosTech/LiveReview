# Server-side blast radius scoring

## Status: proposal — not implemented

## Problem

Blast radius only exists for reviews run through `git lrc review`. The CLI scores the diff locally against a `codebase-memory-mcp` graph of the whole repo and uploads the report to `POST /api/v1/diff-review/:review_id/artifacts/blast-radius`.

Reviews triggered by a webhook or from the web UI never get one, because the LiveReview server has no checkout of the repo and no call graph. Those reviews show no risk badges, no breakdown panel, and no rows in `blast_radius_hunks` for Livi to query.

## Solution

Let the server produce the same report the CLI produces, and store it through the path that already exists. Nothing downstream changes: blob storage, the Postgres mirror, Livi, and the diff viewer UI all keep working as-is.

The blast radius job runs **in parallel** with the AI review, the same way git-lrc does it locally — both start when the review is queued, neither waits for the other.

```
review queued (webhook / UI)
  ├─> review job (unchanged)  -> AI comments saved
  └─> blast_radius job
        -> fetch PR diff + head_sha via the provider
        -> blobless clone (last ~13 months of history) / fetch into lrdata volume, checkout head_sha
        -> codebase-memory-mcp index_repository (incremental after first run)
        -> ScoreHunks(PR diff hunks)
        -> blobstore.SaveArtifact(..., "blast-radius", report)
        -> replicateBlastRadiusToPostgres (already exists)

review page load -> UI joins comments + blast-radius report (already does)
```

**No server-side merge step is needed.** The two results already get combined at read time: the diff viewer fetches the blast-radius artifact separately and joins it to hunks by `file:new_start:new_lines`, and the finding-severity blend (`blendRiskScore`) happens there too. Whichever job finishes first just lands first; the page shows blast radius once the artifact exists.

## Getting the scoring engine into LiveReview

The engine is git-lrc's `blastradius/` module: ~3,150 lines of Go, stdlib only (`client/`, `score/`, `diffparse/`, `symbols/`, root package). Its module path is `github.com/HexmosTech/blastradius` and it is only reachable through a local `replace` in git-lrc today. Two options:

### Option A — Copy it into LiveReview

- Copy git-lrc's `blastradius/` (minus `cmd/` and `explorer/`) into `internal/blastradius/engine/`, rewriting only the import paths.
- Add the `// Ported from git-lrc:blastradius/<file> (as of <sha>)` header to every file.
- Never edit the copy in LiveReview. Fixes land in git-lrc, then get re-copied. Add a `make sync-blastradius` target that does copy + `sed` so a re-sync is mechanical and `diff -r` shows upstream changes.

Pros: no dependency on git-lrc's repo; no git-lrc changes needed.

Cons: two copies drift (the recent sqrt-dampening change already drifted from LiveReview's Math Mode); every scoring fix must be synced by hand, or server-scored and CLI-scored reviews rank hunks differently.

### Option B — Import it as a Go module

- In git-lrc, rename the module in `blastradius/go.mod` to `github.com/HexmosTech/git-lrc/blastradius` and update git-lrc's own imports and `replace` line.
- In LiveReview, `go get github.com/HexmosTech/git-lrc/blastradius@<commit>`. No tag or separate repo needed (requires git-lrc to be fetchable by Go).

Pros: one source of truth; picking up a fix is a version bump.

Cons: a small one-time change in git-lrc; LiveReview builds depend on git-lrc being reachable (or on the Go module proxy cache).

**Decision: Option B.**

Steps:

1. **git-lrc** (one small PR):
   - `blastradius/go.mod`: `module github.com/HexmosTech/blastradius` → `module github.com/HexmosTech/git-lrc/blastradius`.
   - Rewrite the import path in every file that uses it (35 files across `blastradius/`, `internal/`, `cmd/`, etc.): `sed -i 's#github.com/HexmosTech/blastradius#github.com/HexmosTech/git-lrc/blastradius#g'`.
   - Root `go.mod`: update the `require` and `replace ... => ./blastradius` lines to the new path.
   - `go build ./... && go vet ./...` in both the root and `blastradius/`.
2. **LiveReview**:
   - `go get github.com/HexmosTech/git-lrc/blastradius@<git-lrc commit>`. git-lrc is public, so no `GOPRIVATE` or token setup is needed; the Go proxy resolves a commit hash as a pseudo-version, no tag required.
   - Import it from the new worker only; don't touch the existing `internal/blastradius` (Postgres mirror / Math Mode port).
3. **Upgrading later**: re-run `go get ...@<newer commit>`. Every scoring fix in git-lrc reaches LiveReview through this one line, so server-scored and CLI-scored reviews stay identical.

Optional later: tag releases as `blastradius/vX.Y.Z` in git-lrc (Go's convention for a module in a subfolder) for readable versions instead of pseudo-versions.

## Implementation steps

1. **Repo cache** — `internal/blastradius/repocache/`

   Clone each repo **once**, keep it on the volume, and reuse it for every later review of that repo. A repo already in the cache is never cloned again — later reviews only fetch the one commit they need.

   **Folder layout** (`./lrdata` is already the persistent volume in `docker-compose.yml`):

   ```
   /app/lrdata/blastradius/
     <org_id>/                     # tenant boundary - never shared across orgs
       <connector_id>/
         <owner>__<repo>/
           repo/                   # the git checkout (one working tree)
           index/                  # codebase-memory-mcp graph, if its location is configurable
           .lastused               # touched on every use; drives eviction
           .lock                   # one job per repo at a time
   ```

   **How much history we need:** `codebase-memory-mcp` (v0.9.0) builds the file co-change signal with `git log --name-only --since="1 year ago" --max-count=10000`. So we keep ~1 year of commits, not all of history and not just one commit. `--depth=1` is ruled out: it would silently zero that signal.

   **Clone flags** (each one cuts download size):

   | Flag | Effect |
   |---|---|
   | `--filter=blob:none` | Download commits + folder listings for history, but file contents only for the commit we check out |
   | `--shallow-since="13 months ago"` | Drop history the engine never reads. 13, not 12, so the cut-off commit falls outside the engine's 1-year window (the cut-off commit looks like "every file added" and would fake co-change data) |
   | `--single-branch --no-tags` | Skip other branches and tags |
   | `-c diff.renames=false` (repo config) | The engine's `git log` would otherwise do rename detection, which lazily downloads old file contents one by one |
   | `-c checkout.workers=8` | Write checked-out files in parallel |

   **Measured** (prometheus/prometheus, cold clone, same machine/network):

   | Strategy | Time | `.git` size | Co-change works? |
   |---|---|---|---|
   | Full clone | 53.5 s | 291 MB | yes |
   | `--filter=blob:none` | 20.8 s | 30 MB | yes |
   | **`--filter=blob:none --shallow-since="13 months ago"`** | **5.1 s** | **11 MB** | **yes** |
   | `--depth=1` | 2.3 s | 6.7 MB | no |
   | Warm cache: checkout a new commit | 0.07 s | – | yes |

   The engine's `git log` on that clone: 5.3 s with rename detection (lazy downloads), 0.08 s with `diff.renames=false`.

   **Per job:**

   ```
   lock <owner>__<repo>
   if repo/ missing:
       git clone --filter=blob:none --shallow-since="13 months ago" \
                 --single-branch --no-tags --no-checkout <url> repo/
       git -C repo config diff.renames false
       git -C repo config checkout.workers 8
   git fetch --filter=blob:none origin <head_sha>   # only new commits since last time
   git checkout --force --detach <head_sha>
   touch .lastused
   index (incremental) -> score -> save
   unlock
   ```

   **Making it fast — what runs in parallel:**

   - **With the AI review:** the whole blast-radius job already runs alongside the review (see the flow above), so clone time is mostly hidden behind the LLM review time.
   - **Inside the job:** the provider diff/MR-details call and the git clone/fetch run concurrently; scoring waits for both.
   - **Across repos:** different repos clone/index in parallel (within the job concurrency limit).
   - **Within one clone:** git streams one pack per fetch, so a single clone can't be split across connections. The speed-up comes from downloading less (the flags above), plus parallel file writes (`checkout.workers`).
   - **Pre-warming:** when a repo is connected, and on pushes to its default branch (webhooks already arrive; `repo_sync_worker.go` already syncs repos), run a background fetch. Then a review almost always hits a warm cache and pays only the ~0.1 s incremental fetch.

   Other notes:

   - **Commit, not branch.** We always check out the exact `head_sha` the review is about, in "detached" mode. No `git pull`, no branch switching: a pull merges and can conflict, and a branch name moves while the job runs. Fetching by SHA works on GitHub/GitLab. For PRs from forks, the SHA isn't on any branch of the base repo — fetch the PR ref instead (`refs/pull/<n>/head` on GitHub, `refs/merge-requests/<n>/head` on GitLab).
   - **Why reuse is fast:** the fetch only downloads commits and files that changed since the last cached commit, and the graph index updates incrementally instead of re-parsing the whole repo.
   - **One job per repo at a time** (the `.lock`): there is one working tree, so two reviews of the same repo run one after the other. Different repos run in parallel.
   - **Self-healing:** if any git step fails (corrupt clone, force-pushed history, etc.), delete `repo/` and clone once more before giving up.
   - **History grows slowly:** the 13-month cut-off is fixed at clone time, so a long-lived cache keeps more than a year. Harmless (the engine only reads the last year), and step 6 evicts idle repos when space is needed (a re-clone starts a fresh 13-month window).
   - **Auth: HTTPS + the connector's existing token, one implementation for every provider.** The token comes from `integration_tokens` via the review's `connector_id`; only the HTTPS username differs per provider:

     | Provider | Clone URL |
     |---|---|
     | GitHub | `https://x-access-token:<token>@github.com/owner/repo.git` |
     | GitLab | `https://oauth2:<token>@gitlab.com/owner/repo.git` |
     | Bitbucket | `https://x-token-auth:<token>@bitbucket.org/owner/repo.git` |
     | Gitea | `https://<user>:<token>@<host>/owner/repo.git` |
     | Azure DevOps | `https://pat:<token>@dev.azure.com/org/project/_git/repo` |

     Pass the token through a credential helper / env, never in a logged URL or in `repo/.git/config`. SSH is not used: it would need a key registered per repo (GitHub deploy keys can't be shared across repos) or a machine-user account, plus `known_hosts` per git host, while every connector already has a token.

2. **Graph index**
   - Use the `codebase-memory-mcp` binary already in the Docker image (`docker/docker-deps.env`, v0.10.8). git-lrc is tested against v0.9.0 — confirm query output still matches with one real run.
   - Confirm where it stores its index and point it inside the per-org repo cache dir, so indexes are tenant-isolated and incremental.

3. **Scoring engine** — Option B above: `go get github.com/HexmosTech/git-lrc/blastradius@<commit>`, call `blastradius.ScoreHunks`.

4. **Job** — `internal/jobqueue/blast_radius_worker.go`
   - `BlastRadiusJobArgs{OrgID, ReviewID}`, `Kind() = "blast_radius"`, registered like the other workers in `jobqueue.go`.
   - Enqueued **in the same place the review job is enqueued** (`internal/jobqueue/jobqueue.go`, alongside the webhook/manual review job), so both run in parallel. It has no dependency on the review job.
   - Not enqueued for `cli_diff` reviews — git-lrc uploads its own report (its local graph is the better source).
   - Diff source: fetch the PR diff with the same provider call the UI uses (`fetchLiveDiffFromPR` / `fetchLiveDiffFromMetadata` — move these out of `internal/api` so the worker can call them). Webhook/manual reviews never persist their diff (only `cli_diff` stores `preloaded_changes`), and the UI joins on `file:new_start:new_lines`, so scoring the same provider diff the UI renders is what keeps the keys matching.
   - Take `head_sha` from the same MR details call, so the checkout matches the diff being scored.
   - Save via `blobstore.SaveArtifact` + `replicateBlastRadiusToPostgres` (move the latter out of `internal/api` so the worker can call it).
   - Best-effort: timeout (~15 min, same as the CLI), low concurrency (1–2), failures logged, never retried in a loop, never affects the review.

5. **Settings — Settings → Storage → "Blast Radius Repo Cache"**
   - Stored as one `system_settings` row (`blast_radius_cache`): `{ "enabled": true, "max_gb": 5 }`. Read fresh on every job/sweep, like `blob_storage`, so changes apply without a restart.
   - **Enabled by default.** Instance-wide; a per-org toggle can come later.
   - **Cache size** is set in the UI: minimum **5 GB** (default), admins can raise it (10 GB, 15 GB, ...) to keep more repos warm. The API rejects values below 5.
   - The section shows current cache usage (used / max GB, number of cached repos) so admins can see when to raise the limit.
   - Same access as the rest of the Storage tab: `adminOrOwnerGroup` (`/settings/storage/...` in `server.go`) — self-hosted owners and super admins; super-admin only in cloud.
   - Mega menu: add a `link('Blast Radius Repo Cache', ..., '/settings#storage', ...)` next to the existing Storage / Log Compaction / Preloaded Changes Archival entries in `megaMenuData.ts`, with the same predicate as `Storage`.
   - **Install requirement**: document "at least 5 GB free disk on the `lrdata` volume" in the self-hosted install docs. If free space on the volume drops below the configured `max_gb`, the job still runs but the section shows a warning.

6. **Cache eviction (least-recently-used, size-based)** — a periodic River job plus a check after every clone/fetch, no new tables.
   - Measure total size of `/app/lrdata/blastradius/`. While it's over `max_gb`, delete the repo dir with the oldest `.lastused`. Repos that are reviewed often keep getting touched, so they stay; rarely used repos are removed first.
   - Never evict a repo whose `.lock` is held (a job is using it).
   - Run `git gc --prune=now` on the repos that remain.
   - Delete the `<connector_id>/` dir when its connector is removed, and the `<org_id>/` dir when the org is deactivated.
   - No separate age limit: size-based eviction already removes idle repos once space is needed.

7. **Docs**
   - CLAUDE.md "Artifact sync channel": note the server can now compute blast radius for webhook/UI reviews.
   - `internal/docindex/docs/routes_guide/reviews/review-detail.md`: blast radius is no longer CLI-only.
   - `internal/docindex/docs/routes_guide/settings/storage.md`: the new Blast Radius Repo Cache section (toggle, size, usage).
   - Self-hosted install docs: at least 5 GB free on the `lrdata` volume.

## Scope of the first version

- All providers from the start. Both provider-facing parts are already provider-agnostic: the clone is one HTTPS implementation (step 1, only the username differs), and the PR diff + `head_sha` come from `fetchLiveDiffFromPR`, which goes through the existing provider factory (GitHub, GitLab, Bitbucket, Gitea, Azure DevOps). Test GitHub end to end first, then verify the others.
- Fork PRs need the provider's PR ref (`refs/pull/<n>/head` on GitHub, `refs/merge-requests/<n>/head` on GitLab); providers without such a ref (e.g. Bitbucket Cloud) are skipped for fork PRs in v1.
- Only UI change: the Blast Radius Repo Cache section in Settings → Storage (plus its mega-menu link). The review page itself is unchanged.

## Risks / open questions

- **Disk**: roughly one source tree + ~1 year of commit metadata + graph index per repo, capped by the admin-set `max_gb` (min 5 GB, step 5). If one repo is bigger than `max_gb`, it is evicted after every job and re-cloned each time; the usage display should make that visible.
- **Rename detection off**: with `diff.renames=false`, a renamed file shows up under both its old and new name in co-change data. Small effect; turn it back on if it matters (costs a one-time lazy download).
- **Engine history window** (`--since="1 year ago"`) is read from the v0.9.0 binary; re-check it for v0.10.8 and keep `--shallow-since` a month wider.
- **CPU/RAM**: first index of a large repo takes minutes; concurrency must stay low so it doesn't starve review workers.
- **Engine version drift**: v0.10.8 (server) vs v0.9.0 (CLI) may score the same diff slightly differently.
- **PR updated after review**: the UI re-fetches the *current* PR diff, so if new commits are pushed later, old blast-radius keys stop matching (AI comments have the same issue today). Hunks with no match simply show no badge.
- **Page open before blast radius finishes**: the UI fetches the artifact once on load; a user who opens the review early sees it only after a reload. Polling the artifact while it's absent is a possible follow-up.
- **Scores are relative per diff** (normalized to the top hunk), same as the CLI — not comparable across reviews.
