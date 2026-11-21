# Multi-repo context for blast radius ("Related Repos")

Status: implemented (server-side path). See `docs/blast-radius-server-side-plan.md`
for the single-repo server-side scoring pipeline this builds on.

## Problem

Blast-radius scoring (`internal/blastradius`, imported from git-lrc) only ever sees
one repository's diff and one repository's code graph. A PR in a frontend repo that
calls a now-broken endpoint in a separate backend repo looks "low risk" - the caller
and the callee never appear in the same graph.

## Non-goals / rejected approaches

- **Extending git-lrc's CLI engine to accept a list of related project directories.**
  Rejected: the CLI runs on a developer's own machine, so this would require every
  developer to have every related repo cloned locally too. Off the table.
- **A parallel, bespoke cross-repo scoring pipeline.** Rejected in favor of folding
  cross-repo impact into the existing single `Signal`/`Combined`/`Tier` pipeline
  (`internal/blastradius/mathmode.go`) as one more bounded, capped signal - so sorting,
  tiering, and the UI's existing breakdown all pick it up for free.

## How it works

1. **Linking** (`internal/api/repository_relations.go`, `repository_relations` table,
   UI: Explore → Repository → Settings → Related Repos,
   `ui/src/components/Explore/RelatedReposSection.tsx`). An org owner links two
   repositories (any connected providers, same org). Relations are stored directionally
   but read symmetrically.
2. **Scoring a PR** still runs exactly as in `docs/blast-radius-server-side-plan.md`:
   the `blast_radius` River job clones/indexes the PR's own repo and saves a report.
3. **Cross-repo enrichment** runs as a second, best-effort job,
   `internal/jobqueue/cross_repo_link_worker.go` (`cross_repo_link` kind). It is
   queued by `BlastRadiusWorker` *after* the report it enriches has been saved, not
   alongside it. The previous "insert both, share a MaxWorkers:1 queue" approach was
   not sufficient in production: both jobs were observed starting within milliseconds
   of each other, and the enrichment silently skipped a report that did not exist yet.
   It:
   - Resolves the review's own `repositories` row robustly (`reviews.repository` is a
     full provider URL for manual PR reviews and a `full_name` for webhook reviews) and
     walks `repository_relations` **symmetrically to a bounded depth** (default 3), not
     just one hop, so A → B → C links are available.
   - Skips immediately, with **zero** CBM calls, if there are no related repos
     configured - the common case for most orgs/repos.
   - For each related repo, checks out its *default branch tip* (not a PR -
     `repocache.PrepareDefaultBranch`) and indexes it
     (`serverscore.IndexOnly`, mode `fast`). This checkout participates in the same
     `repocache` LRU/size accounting (`EntrySize`/`Evict`/`TooLarge`) as PR checkouts,
     so it's self-limiting under the existing cache-size setting.
   - Runs codebase-memory-mcp's native `cli index_repository --mode
     cross-repo-intelligence` pass (`serverscore.LinkRelatedProjects`) between the
     PR's project and every related repo's project. This creates `CROSS_HTTP_CALLS` /
     `CROSS_ASYNC_CALLS` / `CROSS_CHANNEL` / `CROSS_GRPC_CALLS` / `CROSS_GRAPHQL_CALLS`
     / `CROSS_TRPC_CALLS` edges by matching `Route` nodes against outbound call sites -
     entirely via the engine's own symbol/route resolution, no filesystem colocation
     and no cloning onto a developer's machine.
   - Queries those edges for every symbol the PR's diff touches
     (`serverscore.CrossRepoImpactsForSymbols`).
   - Runs a **version-alias fallback** (`versionAliasedImpacts` in the worker): the
     engine's exact route matcher cannot see URI version segments that NestJS/Spring
     keep out of the controller path (`version: '1'`), so a call to `/v1/sms/send-otp`
     never matched a route indexed as `/sms/send-otp`. The fallback compares normalized
     paths (host/query stripped, one leading `/vN` removed), only when exactly one side
     is versioned, and marks those edges `MatchKind=version-alias` with lower
     confidence. This keeps target repos unchanged.
   - Walks **extended chains**: for each direct impact, finds the route's handler in
     the first repo (`HANDLES`), follows that handler's outbound `HTTP_CALLS` into
     declared routes of other indexed related repos, and repeats up to
     `crossRepoMaxDepth`. Chains are informational; the direct hop already scores, and
     an additional capped `Cross-repo chain` graph signal nudges depth.
   - Folds matches into the already-saved report (`blastradius.RecordCrossRepoImpact`,
     `blastradius.RecordCrossRepoChains`, `blastradius.Renormalize`) and re-saves it via
     the same `blobstore`/`storageblastradius` write path `blast_radius` used.
4. **Scoring integration** (`internal/blastradius/crossrepo.go`): each match becomes a
   capped `Signal{Category: "graph", Name: "Cross-repo impact"}` on the touched symbol
   (capped at `crossRepoMaxBonus`, same order of magnitude as the existing
   `FileCouplingBonus`) plus a `CrossRepoImpact` entry in a new `ImpactedRelatedRepos`
   field (on both `SymbolContribution` and `HunkReport`), plus
   `ImpactedRepoChains` for multi-hop paths. Each impact carries LiveReview's
   `RelatedRepoID`/`RelatedRepoName`/`RelatedRepoWebURL` (from the `repositories` row)
   in addition to the engine project id, so the UI can show a friendly name and link
   instead of an index-cache path. Because `"graph"` is already
   in `blastCategories`/`BLAST_CATEGORIES` (Go and `BlastRadiusPanel.tsx` agree on this
   set), the new signal participates in `Combined`/`Tier` via the existing pipeline with
   no frontend category change needed. `Renormalize` recomputes `BlastRadiusNorm`/
   `Combined`/`Tier` for the whole report afterward, the same way the original scorer's
   one-pass normalization works.
5. **Presentation** (`ui/src/components/reviews/diffviewer/BlastRadiusPanel.tsx`): an
   "Impacted related repos" chip list at both the per-symbol and per-hunk level. Chips
   show the repository `full_name` and link to
   `/explore/repositories/:id/settings`; a collapsed **Extended impact chain** control
   expands to `This repo → B (route) → C (route)` when multi-hop data exists. The
   Related Repos settings page carries a collapsed info panel with the language tiers
   and cross-repo link types the analysis supports.

## Efficiency

- Only the PR's own repo is ever freshly scored against a diff; related repos are
  indexed (not scored) at their default-branch tip, and that indexing is incremental
  after the first run (same `codebase-memory-mcp` index, same `repocache` entry reuse
  as any other repo this server has seen).
- A repo with no related repos configured costs nothing extra: the job returns before
  touching the index engine at all.
- Related-repo discovery is a bounded BFS: depth 3, at most 10 repos per review, with
  depth-1 neighbours visited first. A large mesh cannot turn one review into a
  fleet-wide indexing job.
- Both jobs share one `MaxWorkers: 1` queue, so cross-repo work never competes with (or
  starves) a different review's indexing/scoring. Enrichment is now *enqueued by* the
  blast-radius worker after it saves, so it is ordered even if the queue config no
  longer guarantees single-worker execution.
- **Cycle safety:** related-repo discovery keeps a visited set, so relation cycles
  (A→B→C→A) cannot loop. The chain walk also keeps a visited-project set, caps depth at
  3, caps chains per symbol at 10, and has a 400-step per-symbol budget. Tests cover
  both the BFS cycle and a chain that points back at the reviewed repo/intermediate
  repo.
- **Fallback resilience:** the engine's exact cross-repo pass and the full graph read
  are best-effort. If either fails, the direct version-alias matcher still runs from
  the touched symbols' `HTTP_CALLS` plus each target's declared routes. Failures are
  recorded under `CrossRepoDiagnostics` in the artifact instead of silently producing
  an empty enrichment.
- Related-repo checkouts obey the same cache-size/eviction settings
  (Settings → Storage) as PR checkouts - no separate size budget to tune.

## Validated empirically

Confirmed against a live `codebase-memory-mcp` v0.12.1 binary with three toy
services (`v2-gateway`, `v2-ledger`, `v2-worker`, different languages/frameworks):
`index_repository --mode cross-repo-intelligence` produced a real `CROSS_HTTP_CALLS`
edge between two of the three; `internal/blastradius` unit tests
(`crossrepo_test.go`) replay this shape against a real production report fixture and
assert `Tier()` moves from `blast-radius-medium` to `blast-radius-high` once the
cross-repo signal is folded in - i.e. a change invisible without linking becomes
visible with it, which was the point.

### A regression this work caught

Verifying the v0.12.1 pin end-to-end (not just via the raw `cli` binary) surfaced that
`codebase-memory-mcp` v0.12.x changed `cli query_graph`'s default output format from
JSON to a human-readable "tree" - silently breaking git-lrc's `client.QueryGraph`,
which the entire scoring engine is built on (routes, file-coupling, similarity,
coverage, methods, interfaces). Fixed upstream in git-lrc by pinning `--format json`
explicitly on every affected `cli` call (`blastradius/client/client.go`) rather than
relying on the engine's current default.

## Rendering the impact graph

The Sunburst/Flamegraph charts started as pure caller trees, so a symbol that
reached a linked repo but had few (or zero) inbound CALLS edges drew a single
arc or "no callers" — even though the panel's text said the change crosses into
another repository. `buildImpactHierarchy` now keeps the real caller tree and
appends each impacted related repo as a branch, with its matched route(s)
beneath it. Those branches are flagged and drawn in blue, with a caption saying
they are reach routes rather than callers. Direct impacts and chain hops are
deduped by repository + route, so a repo reached both directly and through a
chain is not drawn twice.

The panel also opens on the highest-scoring symbol that has something to draw
(callers or related repos), instead of a class that only has text references.
