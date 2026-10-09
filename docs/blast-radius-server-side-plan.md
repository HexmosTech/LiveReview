# Server-side blast radius scoring

## Status: implemented on branch `blast-radius` (phases 0–5), not yet tested end to end

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
   - The Docker image's `codebase-memory-mcp` is pinned to **v0.9.0** (same as git-lrc's `graphengine.PinnedVersion`) and locked in `PINNED_DOCKER_DEPS`. v0.10.x prints `query_graph` results as text even with `--json`, which the engine can't parse (checked with v0.10.8). Bump only together with git-lrc.
   - Indexes go to `/app/lrdata/blastradius/index/` via `CBM_CACHE_DIR`; each index file is named after the repo's full cache path (which includes org and connector), so they never collide across tenants. Indexes are big (prometheus: 126 MB index vs 44 MB repo), so `max_gb` and eviction (step 6) must count and delete them together with the repo.

3. **Scoring engine** — Option B above: `go get github.com/HexmosTech/git-lrc/blastradius@<commit>`, call `blastradius.ScoreHunks`.

4. **Job** — `internal/jobqueue/blast_radius_worker.go`
   - `BlastRadiusJobArgs{OrgID, ReviewID}`, `Kind() = "blast_radius"`, on its own `blast_radius` River queue with 1 worker, so indexing never takes a review worker.
   - Enqueued inside `QueueManualReviewJob`, right after the review job, so both run in parallel. Every PR review that creates a review row goes through it: the UI "review this PR" button, the API/MCP trigger and the PR list trigger. Comment-triggered webhook reviews create no review row, so they have nothing to attach a report to.
   - Skipped (logged, nothing saved): `cli_diff` reviews (git-lrc uploads its own report), scheduled reviews (no PR, `pr_mr_url` is empty), reviews without a connector, Bitbucket PRs whose source branch isn't known.
   - Repo and PR come from the review row, all queries scoped by `org_id`: `pr_mr_url` gives the clone URL and PR number for every provider; the connector (`integration_tokens`) gives the token, using the same PAT-vs-OAuth choice as `buildProviderConfig`; `pull_requests` (when linked) gives the base and source branch. The token is only ever sent to the connector's own host.
   - Diff source: `git diff -M <merge-base> <head>` from the cached checkout (repocache), no provider API.
   - Save via `blobstore.SaveArtifact` + `storageblastradius.Store.ReplaceFromReport` (moved out of `internal/api`; the CLI upload handler uses it too).
   - Best-effort: 15 min timeout, `MaxAttempts: 1`, every failure logged and the job returns success, never affects the review.
   - **Repo too large for the cache** (phase 5, needs `max_gb`): 5 GB covers almost every repo without blobs. After the clone + index, if the repo alone is bigger than `max_gb`, delete it, write a `<owner>__<repo>.too_large` marker with the measured size, and save a small `blast-radius` artifact `{ "status": "skipped", "reason": "repo_too_large", "repo_gb": 7.2, "max_gb": 5 }` instead of a report. Later reviews of that repo skip straight away (no re-clone) until `max_gb` is raised above the recorded size. The skipped artifact ships together with the UI that renders it, since today's panel expects a full report.

5. **Settings — Settings → Storage → "Blast Radius Repo Cache"**
   - Stored as one `system_settings` row (`blast_radius_cache`): `{ "enabled": true, "max_gb": 5 }`. Read fresh on every job/sweep, like `blob_storage`, so changes apply without a restart.
   - **Enabled by default.** Instance-wide; a per-org toggle can come later.
   - **Cache size** is set in the UI: default **5 GB**, minimum **1 GB**; admins can raise it (10 GB, 15 GB, ...) to keep more repos warm. The API rejects values below 1.
   - The section shows current cache usage (used / max GB, number of cached repos) so admins can see when to raise the limit.
   - Same access as the rest of the Storage tab: `adminOrOwnerGroup` (`/settings/storage/...` in `server.go`) — self-hosted owners and super admins; super-admin only in cloud.
   - Mega menu: add a `link('Blast Radius Repo Cache', ..., '/settings#storage', ...)` next to the existing Storage / Log Compaction / Preloaded Changes Archival entries in `megaMenuData.ts`, with the same predicate as `Storage`.
   - **Install requirement**: document "at least 5 GB free disk on the `lrdata` volume" in the self-hosted install docs. If free space on the volume is too low for a clone, the job is skipped with the same notice as a too-large repo (step 4).

6. **Cache eviction (least-recently-used, size-based)** — at the end of every job, plus right away when an admin saves a lower cache size. No periodic job: the cache only grows when a job runs. No new tables.
   - Size of a cached repo = its folder + its graph index files (`index/<project>.db*`, named in the repo's `.index` file). While the whole cache is over `max_gb`, delete the repo with the oldest `.lastused`, together with its index. Repos that are reviewed often keep getting touched, so they stay; rarely used repos are removed first.
   - Never evict a repo a job is using right now (its in-process lock is held).
   - If the repo the job just used is itself bigger than `max_gb`: its report is still saved this time, then the repo is deleted and a `<owner>__<repo>.too_large` marker records its size. Later reviews of that repo save a "skipped: repo too large" notice without cloning, until `max_gb` is raised above the recorded size.
   - Under 1 GB free disk on the volume: the job saves a "skipped: low disk" notice instead of cloning.
   - A connector's cached repos are deleted when the connector is deleted; an org's when the org is deactivated.
   - No separate age limit and no `git gc` for now: size-based eviction already removes idle repos once space is needed.

7. **Docs**
   - CLAUDE.md "Artifact sync channel": note the server can now compute blast radius for webhook/UI reviews.
   - `internal/docindex/docs/routes_guide/reviews/review-detail.md`: blast radius is no longer CLI-only; explain the "repo too large" notice and where to raise the cache size.
   - `internal/docindex/docs/routes_guide/settings/storage.md`: the new Blast Radius Repo Cache section (toggle, size, usage).
   - Self-hosted install docs: at least 5 GB free on the `lrdata` volume.

## Scope of the first version

- All providers from the start. Both provider-facing parts are already provider-agnostic: the clone is one HTTPS implementation (step 1, only the username differs), and the PR diff + `head_sha` come from `fetchLiveDiffFromPR`, which goes through the existing provider factory (GitHub, GitLab, Bitbucket, Gitea, Azure DevOps). Test GitHub end to end first, then verify the others.
- Fork PRs need the provider's PR ref (`refs/pull/<n>/head` on GitHub, `refs/merge-requests/<n>/head` on GitLab); providers without such a ref (e.g. Bitbucket Cloud) are skipped for fork PRs in v1.
- UI changes:
  - The Blast Radius Repo Cache section in Settings → Storage (plus its mega-menu link).
  - Review page: when the artifact says `skipped`, the Blast Radius panel shows why, e.g. "This repo (7.2 GB) is larger than the blast radius cache (5 GB)." Owners/admins get a link "Increase cache size" → `/settings#storage`; members see "Ask an admin to increase the cache size in Settings → Storage" (they can't open that tab).

## Risks / open questions

- **Disk**: roughly one source tree + ~1 year of commit metadata + graph index per repo, capped by the admin-set `max_gb` (default 5 GB, min 1 GB, step 5). A repo bigger than `max_gb` is skipped and the review page says so, with a link to raise the limit (step 4).
- **Rename detection off**: with `diff.renames=false`, a renamed file shows up under both its old and new name in co-change data. Small effect; turn it back on if it matters (costs a one-time lazy download).
- **Engine history window** (`--since="1 year ago"`) is read from the v0.9.0 binary; re-check it for v0.10.8 and keep `--shallow-since` a month wider.
- **CPU/RAM**: first index of a large repo takes minutes; concurrency must stay low so it doesn't starve review workers.
- **Engine version drift**: v0.10.8 (server) vs v0.9.0 (CLI) may score the same diff slightly differently.
- **PR updated after review**: the UI re-fetches the *current* PR diff, so if new commits are pushed later, old blast-radius keys stop matching (AI comments have the same issue today). Hunks with no match simply show no badge.
- **Page open before blast radius finishes**: the UI fetches the artifact once on load; a user who opens the review early sees it only after a reload. Polling the artifact while it's absent is a possible follow-up.
- **Scores are relative per diff** (normalized to the top hunk), same as the CLI — not comparable across reviews.
