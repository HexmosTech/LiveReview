# Pin external docs sources to commit IDs, drop hash-based reclone

## Context

The chatbot's RAG corpus (`internal/docindex/docs/{lr_wiki,lrc_wiki}`, gitignored,
embedded via `//go:embed docs` in `internal/docindex/docs.go`) is currently
refreshed by `make prep-training-data` (`scripts/prep_training_data.sh`), which
`git clone`s 4 GitHub repos (`git-lrc`, `git-lrc.wiki`, `LiveReview`,
`LiveReview.wiki`) into `ui/docs/training_data/` and copies `.md` files into the
embed target. Freshness is decided by `check-training-data`, wired as a
prerequisite of the dev-server targets (`run-fast`, `run-debug`, `develop`,
`develop-reflex`), which compares a local content hash against a
`TRAINING_DATA_HASH` constant baked into the Makefile.

This broke down (Slack thread, 2026-09-05) for several concrete reasons:

- **Moving-target comparison.** `TRAINING_DATA_HASH` is a snapshot of one
  person's fetch. The thing it's compared against (GitHub HEAD of 4 actively-
  committed repos) changes continuously, so the hash is stale almost
  immediately for everyone else — every `make run-fast` reclones all 4 repos,
  every time, for most developers (shrijith: "every single time it is
  cloning... 3 repos").
- **No cheap freshness check.** Determining "did anything change" requires a
  full clone of the remote content first — there's no cheap way to ask "is my
  local copy still current" without fetching.
- **Wrong source of truth.** "If I update it on my machine it should update
  everybody's" (lovestaco) conflates a local machine with GitHub. Shrijith:
  "github is source of truth."
- **Non-reproducible builds.** Nothing enforces which commit of each source
  ends up embedded. `docker-multiarch-push`/`raw-deploy` never run any fetch
  step at all — confirmed by reading `Dockerfile`/`Dockerfile.crosscompile`
  (no `prep-training-data`/`check-training-data` step) and `.dockerignore`
  (its `docs/` entry is the top-level `docs/`, not
  `internal/docindex/docs/`). So today, a Docker image embeds whatever
  happened to be sitting in the build machine's `internal/docindex/docs/`
  folder at `docker build` time — stale, empty, or from an unrelated branch,
  with zero visibility into what actually shipped.
- **Wrong location.** The intermediate clone/copy staging directory is
  `ui/docs/training_data/` — inside the frontend tree, even though it's pure
  backend RAG-indexing data (shrijith: "why is it in UI? isn't the backend
  indexing it").
- **Full clones of large repos just to read a few docs.** `LiveReview` itself
  (one of the 4 sources) is a large repo; `git clone --depth 1` of it still
  pulls every blob in the tip commit even though only `docs/` is ever used.
  Shrijith: "for a bigger repo like LiveReview you can get only the folder
  you need."
- **The single-hash marker can silently go missing.** On the reported
  customer-visible failure, the log read `recorded (empty), actual
  <hash>` — `TRAINING_DATA_HASH` was blank in that checkout even though the
  target folder already had files in it, so it recloned anyway. A single
  monolithic constant baked into the Makefile has no way to fail safely when
  it's absent/corrupted; per-source state (§2 below) makes this class of
  failure impossible to hit silently — a missing marker is scoped to one
  source's KEY=value line, and a completely empty marker file just means
  "nothing synced yet," visibly.

Shrijith's requested design, which this plan implements as-is:
> have a local folder, gitignored → pull once, store the commit id → during
> build, check the commit id cheaply without cloning, sync if necessary →
> embed as a Go FS → settle on remote repo(s) as the sole source of truth,
> with an enforced list of commit IDs, so the build is reproducible and no
> full clone is ever needed just to check.

This repo already has the right precedent for exactly this shape of problem:
`docker/docker-deps.env` + `scripts/check_docker_deps.py` pin third-party
Docker dependency versions, with `make check-docker-deps` /
`update-docker-deps` / `update-docker-deps-yes` targets and a
`PINNED_DOCKER_DEPS` lock-list mechanism. This plan mirrors that pattern for
the doc sources instead of inventing a new one.

## Design

### 1. Lockfile: `scripts/docs_sources.env` — only for sources that are NOT this repo

Checked into git (this repo's own commit history is the audit trail — matches
"enforced list of commit IDs"). One line per *externally-hosted* source,
holding a pinned 40-char commit SHA, not a branch name:

```
GIT_LRC_COMMIT=<sha>
GIT_LRC_WIKI_COMMIT=<sha>
LIVEREVIEW_WIKI_COMMIT=<sha>
```

`livereview`'s own `docs/` subtree is dropped from this lockfile entirely —
it's not an external source, it's the same repo being built right now. Read
it straight from the local working tree (`$ROOT_DIR/docs`) into the embed
target with a plain file copy, no clone, no pin, no possibility of drift —
whatever's checked out is by definition current. This also removes the
self-referential absurdity of the old design: fetching LiveReview's own
`docs/` over the network from GitHub while already sitting inside a
LiveReview checkout.

The 3 remaining pinned sources are bumped only by an explicit `make
update-docs-sources` (see §4) — a deliberate, reviewable commit, same
workflow as `update-docker-deps`. Nobody's laptop is ever the source of
truth; GitHub is, and this file is just a pointer into it.

### 1b. Automating the bump — the agent that edits the docs does it, in the same commit

The old design's failure mode ("people will forget to bump it") is fixed by
putting the bump inside the same workflow that touches the docs, not by
adding a reminder step that's easy to skip. `git-lrc`, `git-lrc.wiki`, and
`LiveReview.wiki` docs are, in practice, edited by an AI coding agent working
in this same environment (both `git-lrc/` and `LiveReview/` are sibling
directories the agent already has open — see the repo-level `CLAUDE.md`).

**Rule (goes in `AGENTS.md`, see Files touched): whenever an agent commits a
change to `git-lrc/docs/`, `git-lrc`'s wiki, or `LiveReview`'s wiki, it must,
in that same task, `git ls-remote` (or use the commit it just made/pushed)
to get the new SHA, update the matching `*_COMMIT` line in
`LiveReview/scripts/docs_sources.env`, and commit that alongside — not as a
separate follow-up someone has to remember to trigger.** `make
check-docs-sources` (§4) still exists as a read-only backstop that reports
(and can fail CI on) any pin that's fallen behind, in case a change is ever
made outside agent-driven edits — but the primary mechanism is the editing
agent bumping the pin itself as part of finishing its task, not a separate
bot or a human ritual.

### 2. Local sync-state marker (gitignored)

`internal/docindex/docs/.synced-commits.env` (same `KEY=value` shape as the
lockfile). Written after every successful sync, records which commit is
*currently embedded on disk*. Add to `.gitignore` next to the existing
`lr_wiki`/`lrc_wiki` entries.

### 3. `scripts/sync_docs_sources.sh` (replaces `scripts/prep_training_data.sh`)

`livereview`'s `docs/` subtree: no network, no pin — `cp -r $ROOT_DIR/docs
internal/docindex/docs/lr_wiki_local/` (or merged into `lr_wiki/` per the
existing copy convention) straight from the working tree, every run,
unconditionally. Cheap enough that "compare then skip" isn't even needed for
this one.

For each of the remaining 3 externally-hosted sources (`git-lrc`,
`git-lrc-wiki`, `livereview-wiki`):

1. Read the pinned SHA from `scripts/docs_sources.env` and the last-synced SHA
   from `internal/docindex/docs/.synced-commits.env` — **pure local string
   comparison, zero network.** Skip the source entirely if they match.
2. On mismatch (or marker/target missing), **never clone the whole
   repository.** Fetch *exactly that pinned commit*, and for every source
   that only needs a subdirectory, restrict the checkout to that
   subdirectory *before* fetching any blobs, so blobs outside it are never
   downloaded — full stop, not "downloaded then discarded". This is what
   makes `LiveReview` itself (large repo, one of its own doc sources) and
   `git-lrc` cheap to sync even though each is a full project repo, not a
   docs-only repo:
   ```bash
   git init -q "$tmp"
   git -C "$tmp" remote add origin "$url"
   git -C "$tmp" config core.sparseCheckoutCone true
   git -C "$tmp" sparse-checkout set "$subdir"     # e.g. "docs" — set BEFORE fetch
   git -C "$tmp" fetch --depth 1 --filter=blob:none origin "$sha"
   git -C "$tmp" checkout -q FETCH_HEAD
   ```
   With `sparse-checkout set` done first and `--filter=blob:none` on the
   fetch, git only fetches blobs for paths inside `$subdir` at checkout time
   — commit + tree objects for the rest of the repo are walked (unavoidable
   metadata) but their file contents never transfer. `git-lrc` → `docs`,
   `livereview` → `docs` (same `SUBDIRS` mapping as today's
   `prep_training_data.sh`). The two wiki sources have no subdirectory to
   restrict (a wiki repo's entire content is docs), so for them the
   `sparse-checkout` step is simply omitted and `--filter=blob:none
   --depth 1` alone already fetches only what's used.
3. Copy straight into the final embed target — **no `ui/docs/training_data`
   staging hop**:
   - `git-lrc` → `internal/docindex/docs/lrc_wiki/LRC_README.md` (mirrors
     current behavior — check exact current mapping in
     `prep_training_data.sh` and preserve it)
   - `git-lrc-wiki` → merged into `lrc_wiki/`
   - `livereview-wiki` → `lr_wiki/` (alongside the locally-copied
     `livereview` `docs/` subtree from the step above)
   - Keep the existing non-ASCII-hyphen/comma filename sanitize step (needed
     for Go embed).
4. Update `.synced-commits.env` with the SHA just synced, for each source
   independently (so a partial run still records progress).
5. Drop the whole `ui/docs/training_data/*` tree and its `.gitignore` entries
   — nothing needs it anymore; it was an accidental UI-directory detour for
   backend data.

### 4. Checker/updater scripts + Makefile targets (mirrors docker-deps exactly)

- `make check-docs-sources` — read-only: for each source, `git ls-remote
  <url> <branch>` (single round-trip per source, no clone) to see if the
  pinned SHA is behind the remote branch tip; report, exit 1 if any pinned
  entry is behind (CI-usable), matching `check_docker_deps.py --check`'s
  contract.
- `make update-docs-sources` — interactive, shows what moved, asks per-entry,
  rewrites `scripts/docs_sources.env` (like `update-docker-deps`).
- `make update-docs-sources-yes` — non-interactive version.
- `make sync-docs-sources` — runs `scripts/sync_docs_sources.sh` (the cheap
  compare-then-fetch-if-needed step from §3). This is what actually keeps the
  embedded FS current; it never touches the lockfile.

### 5. Wire `sync-docs-sources` into build, not just dev servers

Replace `check-training-data` as a prerequisite of `run-fast`, `run-debug`,
`develop`, `develop-reflex` with `sync-docs-sources` — same place, cheap
no-op on every normal iteration (pure string compare) instead of a clone on
every one.

Add it as a prerequisite of `build`, `build-prod`, `build-with-ui` too, so
`go build` output is only ever produced from a docindex tree that matches the
committed lockfile — this is the direct fix for "even `go build` should check
the commit id and sync if necessary." (`build-ci` intentionally stays
untouched — CI builds shouldn't require network for docs sync; confirm with
the team whether CI should instead fail-closed via `check-docs-sources`.)

Remove `TRAINING_DATA_HASH` and the `check-training-data`/`prep-training-data`
Makefile targets entirely once the new targets are in place.

### 6. Docker builds: same sync step, cache-friendly

Add to `Dockerfile` and `Dockerfile.crosscompile`, before the `go build`
step:

```dockerfile
COPY scripts/docs_sources.env scripts/sync_docs_sources.sh ./scripts/
RUN ./scripts/sync_docs_sources.sh
```

placed so the Docker layer cache keys off `docs_sources.env` — the layer
only re-runs (and only then touches the network) when someone bumps a pinned
commit, exactly like the existing `docker-deps.env` build-arg pattern. This
directly answers "what will `docker-multiarch-push`/`raw-deploy` embed?" —
deterministically, whatever `scripts/docs_sources.env` says, fetched fresh
inside the image build rather than inherited from the host machine's stale
local state.

## Files touched

- `scripts/docs_sources.env` — new lockfile (mirrors `docker/docker-deps.env`)
- `scripts/sync_docs_sources.sh` — new, replaces `scripts/prep_training_data.sh`
- `scripts/check_docs_sources.py` — new, mirrors `scripts/check_docker_deps.py`
  (reuse its `_http_json`/argparse/report-table structure; swap Docker Hub/
  GitHub-release lookups for `git ls-remote`)
- `scripts/training_data_hash.sh` — delete (no longer needed)
- `Makefile` — remove `TRAINING_DATA_HASH`, `check-training-data`,
  `prep-training-data`; add `check-docs-sources`, `update-docs-sources`,
  `update-docs-sources-yes`, `sync-docs-sources`; rewire prerequisites per §5
- `Dockerfile`, `Dockerfile.crosscompile` — add the sync `COPY`+`RUN` stage
  per §6
- `.gitignore` — drop the 4 `ui/docs/training_data/*` lines, add
  `/internal/docindex/docs/.synced-commits.env`
- `internal/docindex/docs.go` — no change (embed target directory unchanged)
- `AGENTS.md` — rewrite the "Route Documentation for RAG" section (currently
  at `ui/docs/training_data/lr_routes/`, itself moving as part of dropping
  `ui/docs/training_data/`): drop the manual "run `make prep-training-data`,
  commit the Makefile hash" instruction (that whole step no longer exists).
  Add the §1b rule as its own short section (e.g. "Keeping
  `scripts/docs_sources.env` in Sync" — matches the existing "Keeping
  `config/*-ssl.conf.example` in Sync with `lrops.sh`" section's naming
  pattern): an agent-driven `git-lrc`/wiki docs commit must bump the matching
  pin in the same task, not as a separate reminder.

## Verification

- `make sync-docs-sources` twice in a row: first run fetches (network,
  3 single-commit checkouts for `git-lrc`/`git-lrc-wiki`/`livereview-wiki`,
  plus one local copy of this repo's own `docs/`, populating
  `internal/docindex/docs/{lr_wiki,lrc_wiki}` and `.synced-commits.env`);
  second run must do **zero** network calls and print that everything's
  already in sync.
- Edit one SHA in `scripts/docs_sources.env` by hand to an older valid commit,
  re-run `make sync-docs-sources`, confirm only that one source re-fetches
  and the embedded content actually changes to match that older commit.
- `make check-docs-sources` against the repo's current pinned SHAs: confirm
  it reports without cloning anything (watch for absence of `git clone`/
  `fetch --depth 1` of full branches in its output) and exits 1 only when a
  pinned SHA is behind its remote branch tip.
- `make run-fast`: confirm startup no longer clones when already in sync
  (this is the original customer-visible bug from the Slack thread).
- Edit a file under this repo's own `docs/`, run `make sync-docs-sources`,
  confirm the change shows up in `internal/docindex/docs/` immediately with
  no network call and no lockfile entry involved — proves the self-reference
  is gone.
- `make check-docs-sources` with one pinned SHA deliberately rolled back:
  confirm it reports that source as behind (the read-only backstop from
  §1b/§4 still catches drift that didn't go through an agent-driven edit).
- During the `git-lrc`/wiki sources' sync, confirm (e.g. via `GIT_TRACE=1` or
  checking `.git/objects` size in the temp checkout) that only `docs/`
  blobs at the pinned commit were fetched, not the whole repo's tip tree —
  proves the sparse/partial-clone fix for "bigger repo → only the folder you
  need" actually takes effect.
- `docker build` (or `make docker-build`) from a machine with an *empty*
  `internal/docindex/docs/{lr_wiki,lrc_wiki}`: confirm the built image still
  contains the expected docs (exec into the image / check binary size or a
  debug endpoint), proving the sync now happens inside the image build
  rather than depending on host state.
