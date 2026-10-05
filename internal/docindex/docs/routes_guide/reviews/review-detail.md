# Review Detail

**Route:** `/reviews/:id`
**Component:** `ui/src/pages/Reviews/ReviewDetail.tsx`

## Purpose

Full detail view of a single code review: status, timeline of review events,
AI-generated findings/summary, diff viewer, commit list, and accounting
(cost/token usage per stage, refreshed every 15s while a review is
in-flight).

## Who can access it

Any authenticated org member; the review must belong to their org
(org-scoped, enforced server-side).

## Key actions

- View the diff being reviewed (`DiffViewerPanel`) alongside AI findings.
- View the event/log timeline of the review run (`ReviewEventsPage`).
- View accounting: which stages ran, tokens/cost per stage
  (`getReviewAccounting`).
- View the commits included in this review (up to a preview limit, expandable).
- Navigate back to the reviews list.

## Blast-radius scoring and ordering

For reviews run through the git-lrc CLI with blast radius, the diff viewer
(`DiffViewerPanel`) orders hunks by a risk score instead of plain diff order.
The score blends three dimensions:

- **Blast Radius** — how far the change can reach (callers, entry points,
  repository hotspots, architectural layers).
- **Review Priority** — how much scrutiny it needs (complexity, duplication,
  missing test coverage).
- **Finding Severity** — the most severe AI finding on the hunk
  (`critical`/`warning`/`info`), blended in at ~10% weight so a critical
  security bug outranks a trivial one.

The "Score: Whole" / "Score: Per file" / "Diff order" control switches between
the whole-diff ranking, per-file ranking, and original diff order. Each hunk's
breakdown panel (Summary and Math Mode tabs) shows the exact math behind its
score, including the finding-severity contribution.

## Related pages

- [Reviews list](reviews-list.md)
- [New Review](new-review.md)

## Learn more (public docs)

- [Turn findings into tickets and follow-up tasks](https://hexmos.com/livereview/docs/livereview/mcp/usecases/turn-findings-into-tickets)
