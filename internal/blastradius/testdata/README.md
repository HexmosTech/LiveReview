# Blast-radius replay fixtures

Real review artifacts used by `mathmode_test.go`. They are immutable inputs:
the numbers in the tests are what the product actually showed, not values
recomputed from a fresh index.

| File | Review | Why |
|---|---|---|
| `review_11632_report.json` | LiveReview repo, commit `1f92bf94`, 2026-08-23 | The original linear-roll-up artifact. Zero-signal, single-symbol, multi-symbol, and both `IsSelf` branches. |
| `review_11632_current_report.json` | Same diff, re-scored by git-lrc's current `sqrt` formula | Pins the saturating steps: sum → sqrt → add file-level signals. |
| `review_14285_crossrepo_report.json` | Consumer repo, production review 14285, 2026-11-21 | Real cross-repo review. The artifact predates `SymbolCombine`; its 3 impacted hunks carry a post-scoring bonus, so detection must discount the bonus before choosing `sqrt`. |

Provenance and the reproduction recipe (including why a historical
codebase-memory-mcp v0.9.0 `moderate` index is needed for an exact replay) live
in git-lrc's `blastradius/testdata/reviews/README.md`.
