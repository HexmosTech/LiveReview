# Port blast radius scoring/signals/Math Mode to LiveReview's Go backend

## Status: backend/storage only — frontend read-path reverted

Everything through step 3 below shipped and is live: `internal/blastradius`
(the Go port), `storage/blastradius` + the `blast_radius_hunks` table/
`blast_radius_reviews` view, wired into `PutDiffReviewArtifact` so every new
upload replicates to Postgres, `terminology.json` updated, and a one-time
backfill run against production data (365 reviews / 5521 hunks).

**Steps 4 and 5 (reading `MathMode` back out of `GetDiffReviewArtifact` and
having `BlastRadiusPanel.tsx` render it instead of computing it) were
implemented, verified working end-to-end, and then explicitly reverted** —
the decision was to leave the diff viewer's UI untouched and use this purely
as a Postgres mirror for Livi to query. `GetDiffReviewArtifact` still returns
the raw S3 blob unmodified; `BlastRadiusPanel.tsx` still recomputes
everything client-side, unchanged. The design for the reverted steps is kept
below for reference in case that direction is revisited, but it does not
reflect the current code.

## Context

Blast radius data lives entirely in S3 (`git-lrc` uploads one JSON artifact per
review via `PutDiffReviewArtifact`), and everything the UI shows from it —
tiering, the Summary tab's signal grouping, and the Math Mode tab's full
step-by-step score derivation — is recomputed from scratch in the browser on
every page load (`ui/src/components/reviews/diffviewer/BlastRadiusPanel.tsx`).
Two problems follow from that:

1. **Livi can't answer blast-radius questions at all.** Its chat pipeline only
   generates SQL against Postgres; this data isn't there, so it hallucinated a
   fake column path when asked (`chat_debug_logs/chat_debug.log`).
2. **The scoring/derivation logic only exists as ~150 lines of React**
   (`renderMathDimension`, `MathModeView`, `sumExpression`,
   `splitSignalsByDimension`) — duplicated conceptually in git-lrc's own
   vanilla-JS copy, and unavailable to anything that isn't that one React tree.

Per your answers: port the **scoring/signals side only** (tiering, Summary's
signal split, Math Mode) — not Sunburst/Flamegraph's call-graph viz, which
stays frontend since it just renders as SVG either way and needs the full
`Callers`/`Path` graph regardless. Compute **at artifact upload time**, not
lazily per-read. Store the **full signal-level detail** so Math Mode's
drill-down loses nothing. Scoped to **LiveReview only** — git-lrc's own copy
is untouched.

## Architecture

```
git-lrc uploads artifact ──▶ PutDiffReviewArtifact ──┬──▶ S3 (raw blob, unchanged - still needed for Sunburst/Flamegraph's Callers/Path)
                                                        └──▶ internal/blastradius.ComputeMathMode (per hunk)
                                                             ──▶ storage/blastradius.ReplaceHunksForReview ──▶ Postgres (blast_radius_hunks)

GetDiffReviewArtifact ──▶ fetch S3 raw report (unchanged, for Symbols/Callers)
                       ──▶ storage/blastradius.GetForReview (one SELECT)
                       ──▶ merge: attach stored Tier/MathMode onto each hunk before returning

BlastRadiusPanel.tsx  ──▶ Summary + Math Mode tabs render hunk.MathMode directly (no client math)
                       ──▶ Sunburst/Flamegraph unchanged (still reads Symbols[].Callers from the S3 report)

Livi chat             ──▶ SQL directly against blast_radius_hunks (org_id/review_id/file_path/combined all flat columns)
```

## 1. Go port of the scoring logic — new package `internal/blastradius`

Port the read-only types + derivation from
`ui/src/types/reviews.ts` (`BlastRadiusSignal`, `BlastRadiusSymbolContribution`,
`BlastRadiusHunkReport`, `BlastRadiusWeights`) and
`BlastRadiusPanel.tsx` (`BLAST_CATEGORIES = {architecture, graph}`,
`PRIORITY_CATEGORIES = {duplication, code-metrics}`, `splitSignalsByDimension`,
`renderMathDimension`'s Steps 1-8, `sumExpression`). Verified this session
against review 11632's real artifact — every step reproduces exactly
(BlastRadiusRaw 30.7 → norm 100, ReviewPriorityRaw 69.7 → norm 100, final 100).

```go
type Signal struct { Name, Detail, Category string; Points float64 }
type SymbolContribution struct {
    QualifiedName, Name string
    Signals []Signal
    BlastRadiusRaw, ReviewPriorityRaw float64
}
type Weights struct { BlastRadius, ReviewPriority float64 }
type HunkReport struct {
    FilePath, Header string
    NewStart, NewLines int
    Signals []Signal
    BlastRadiusRaw, BlastRadiusNorm, MaxBlastRadiusRaw float64
    MaxBlastRadiusHunkFile, MaxBlastRadiusHunkHeader string
    ReviewPriorityRaw, ReviewPriorityNorm, MaxReviewPriorityRaw float64
    MaxReviewPriorityHunkFile, MaxReviewPriorityHunkHeader string
    Combined, HygieneMultiplier float64
    Weights Weights
    Symbols []SymbolContribution
}
type FileReport struct { Path string; Hunks []HunkReport }
type Report struct { Project, GeneratedAt string; Files []FileReport }
```

Entry point, mirroring `renderMathDimension`/`MathModeView` 1:1 as data instead
of JSX — the frontend then just formats/renders this, no arithmetic of its own:

```go
type SubtotalLine struct { Name string; Signals []Signal; Subtotal float64 }
type DimensionBreakdown struct {
    Title string
    PerSymbol []SubtotalLine
    HunkSignals []Signal      // "This hunk (file-level signals)" line
    HunkSignalsSubtotal float64
    Total, Max, Norm float64
    MaxSourceFile, MaxSourceHeader string
    IsSelf bool
    StepAdd, StepSum, StepScale int // StepSum == 0 means "step omitted" (see below)
}
type MathMode struct {
    BlastRadius, ReviewPriority DimensionBreakdown
    Weights Weights
    BlastShare, PriorityShare, Blended, HygieneMultiplier, Final float64
}
func ComputeMathMode(h HunkReport) MathMode
func Tier(combined float64) string // mirrors ui/src/lib/blastRadius.ts's blastRadiusTier thresholds
```

**Emit raw `float64`, never pre-formatted strings.** Go's `fmt.Sprintf("%.1f",
…)` uses banker's rounding while JS `.toFixed(1)` rounds half away from zero —
`0.25` formats as `0.2` in Go and `0.3` in JS. Verified directly (`go run` a
one-liner vs `node -e`) during implementation.

**Three behaviours that are easy to miss** — all captured in the golden fixture:
- **Dynamic step numbering.** The *"add every subtotal together"* step only
  exists when more than one part contributes; otherwise it is skipped and every
  later step shifts down by one (review 11632's single-symbol hunks end at step
  6, its multi-symbol hunk ends at step 8).
- **Two symbol orderings coexist.** Math Mode iterates `Symbols` in raw artifact
  order; the carousel sorts by `BlastRadiusRaw` desc. Conflating them silently
  reorders the Math Mode lines.
- **Signal ordering + dormant split.** Lists sort by `|Points|` desc, with
  zero-point signals separated into the collapsed "checked, not contributing"
  group.

Table tests assert against the checked-in real artifact
(`internal/blastradius/testdata/review_11632_report.json`), whose 4 hunks
cover every branch: zero-signal, single-symbol (skipped sum step),
multi-symbol full 8-step, `IsSelf` true and false. Also assert the invariant
**`MathMode.Final == hunk.Combined`** — if the derivation stops landing on the
number git-lrc computed, the port has drifted.

## 2. Storage: `blast_radius_hunks` table + `storage/blastradius` package

New migration (follow `db/migrations/20260809120000_create_review_commits.sql`'s
pattern — `org_id` denormalized per `AGENTS.md`'s "Direct Context Filtering"
rule, so every query can filter `WHERE org_id = $1` without a join):

```sql
CREATE TABLE blast_radius_hunks (
    id         BIGSERIAL PRIMARY KEY,
    review_id  BIGINT NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    org_id     BIGINT NOT NULL REFERENCES orgs(id),
    file_path  TEXT NOT NULL,
    new_start  INTEGER NOT NULL,
    new_lines  INTEGER NOT NULL,
    combined   NUMERIC(5,2) NOT NULL,
    math_mode  JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_blast_radius_hunks_review_hunk UNIQUE (review_id, file_path, new_start, new_lines)
);
CREATE INDEX idx_blast_radius_hunks_review_id ON blast_radius_hunks (review_id);
CREATE INDEX idx_blast_radius_hunks_org_combined ON blast_radius_hunks (org_id, combined DESC);
```

New `storage/blastradius/store.go` (matches `storage/chat`'s shape):
- `ReplaceHunksForReview(ctx, orgID, reviewID int64, hunks []internal/blastradius.HunkReport) error` —
  computes `MathMode`+`Tier` per hunk, then **inside one transaction**:
  `DELETE FROM blast_radius_hunks WHERE review_id = $1 AND org_id = $2`
  followed by the inserts. Plain `ON CONFLICT DO UPDATE` is **not** sufficient:
  a re-uploaded artifact with *fewer* hunks (rebased/amended diff) would leave
  the vanished hunks behind as orphan rows, silently inflating any
  `COUNT`/`MAX` Livi runs over the table. The `UNIQUE` constraint still guards
  against duplicates within a single upload.
- `GetForReview(ctx, orgID, reviewID int64) (map[hunkKey]StoredHunk, error)` —
  one SELECT, keyed by `(file_path, new_start, new_lines)` (the exact join key
  `ui/src/lib/blastRadius.ts`'s `hunkBlastKey()` already uses client-side).

Per `CLAUDE.md`, adding storage functions **requires updating
`storage/storage_status.md`** with the new operations, then validating line
references with `make check-status-doc`.

## 3. Write path: `PutDiffReviewArtifact`

`internal/api/diff_review.go:413` — after the existing S3 write succeeds (S3
stays the raw source of truth, unchanged), when `artifactType ==
"blast-radius"`: `json.Unmarshal` the payload into `blastradius.Report`, flatten
`Files[].Hunks[]`, call `storage/blastradius.ReplaceHunksForReview`. Best-effort: log and
continue on failure (don't fail the upload over the derived-data write, same
posture as the rest of this codebase's fire-and-forget artifact writes).

## 4. Read path: `GetDiffReviewArtifact`

`internal/api/diff_review.go:461` — for `artifact_type == "blast-radius"`,
after reading the raw blob from S3 (unchanged, still needed for
`Symbols[].Callers`/`Path` that Sunburst/Flamegraph render), call
`storage/blastradius.GetForReview`, and walk the parsed report's
`Files[].Hunks[]` attaching each hunk's stored `MathMode` + `Tier` by matching
the same `(FilePath, NewStart, NewLines)` key before re-marshaling and
returning. One API call still, no new endpoint — `ui/src/api/reviews.ts`'s
`getBlastRadiusReport` is unchanged.

## 5. Frontend: `BlastRadiusPanel.tsx` becomes pure rendering

- Add `MathMode`+`Tier` fields to `BlastRadiusHunkReport` in
  `ui/src/types/reviews.ts`, matching the Go `MathMode` JSON shape.
- Delete `renderMathDimension`, `sumExpression`, `MathModeView`'s internal
  arithmetic, `splitSignalsByDimension`'s Summary-tab usage — `MathModeView`
  renders `detail.MathMode` directly; the Summary tab's two `DimensionCard`s
  read `detail.MathMode.BlastRadius`/`.ReviewPriority` for their signal lists
  instead of calling `splitSignalsByDimension(allSignals(detail))`.
- Leave `blastRadiusTier()` (`ui/src/lib/blastRadius.ts`) in place as a
  fallback for any hunk without a stored `Tier` (pre-backfill data, or a
  brand-new upload race) — it's a 4-line pure bucket function, not worth
  deleting for a "some computation still client-side" purity argument; prefer
  `detail.Tier` when present.
- Sunburst/Flamegraph, `groupCallers`, `buildHierarchy` — untouched.

## 6. Livi: `terminology.json`

Add an entry so Livi's SQL generator resolves "blast radius" to the real table:

```json
{
  "aliases": ["blast radius", "blast-radius", "change impact", "review risk score"],
  "targets": ["blast_radius_hunks.combined", "blast_radius_hunks"],
  "term": "blast radius"
}
```

Then **`make prep-dbctx` is mandatory, not optional** — adding a table changes
the schema fingerprint, and `internal/mcpagent/agent.go` *refuses every
analytics turn* while the index is stale ("Livi's Database Context is out of
date — the database schema changed since it was last built"). Skipping this
step doesn't just leave blast radius unqueryable, it takes Livi down entirely
until the index is rebuilt. Same applies on every deploy target.

## 7. Backfill

New `scripts/backfill_blast_radius_hunks.go` (same shape as
`scripts/backfill_chat_chart_stats.go`): enumerate
`org/*/review/*/artifacts/blast-radius.json` via `bucket.List(&blob.ListOptions{
Prefix: "org/"})` — `internal/blobstore.OpenBucket` returns a gocloud
`*blob.Bucket`, whose `List` covers this natively, so no aws CLI shell-out like
`scripts/adoption_chart/generate_blast_radius.py` used. Parse the `org_id` and
`review_id` back out of each key, fetch, parse, `ReplaceHunksForReview`.

Two things the backfill must handle: **skip artifacts whose `review_id` no
longer exists in Postgres** (the FK would otherwise abort the run — S3 objects
outlive deleted reviews), and fetch **concurrently** (~381 artifacts in prod
today; sequential per-object GETs against B2 measured ~2 min per 40 objects in
this session, ~20 workers brought the full set under a minute).

Run once locally, then against prod after deploy (prod run is your call).

## Verification

`internal/blastradius/testdata/review_11632_report.json` is a real S3
artifact checked in as the test fixture. Its 4 hunks were hand-verified
against the live UI's Math Mode output (a Python transliteration of
`BlastRadiusPanel.tsx` matched shrijith's pasted output character-for-
character before any Go was written) and cover every branch: zero-signal,
single-symbol (skipped sum step), multi-symbol full 8-step, and both
`IsSelf` narration branches.

- `go test ./internal/blastradius/...` — asserts `ComputeMathMode` against
  hand-computed expected values for all 4 hunks, plus the `Final ==
  Combined` invariant and the `effectiveHygiene` absent-vs-real-zero case
  (a real divergence from the frontend caught in code review).
- `go test ./storage/blastradius/...` — replace/get round-trip incl. the
  shrinking-hunk-set case (upload N hunks, re-upload N-1, assert no orphan
  row survives).
- `go test ./internal/api/...` — `PutDiffReviewArtifact`/
  `GetDiffReviewArtifact` unchanged for every artifact type (blast-radius
  included on the read side - only the write side changed).
- Ran the backfill locally against production data:
  `SELECT combined FROM blast_radius_hunks WHERE review_id = 11632 ORDER BY combined DESC`
  → `100.00, 35.51, 35.51, 0.00`, matching the hand-verified numbers exactly.
- Manual: `make prep-dbctx`, then ask Livi *"How many reviews had critical or
  high blast radius this month? Show those repositories"* (the exact question
  that hallucinated `metadata -> 'review_result' -> 'comments' ->> 'Severity'`
  in `chat_debug_logs/chat_debug.log`) — confirm it now generates real SQL
  against `blast_radius_hunks` and renders the per-repository tiered bar chart
  matching `scripts/adoption_chart/blast_radius.html`.
