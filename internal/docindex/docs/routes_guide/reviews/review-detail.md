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

## Related pages

- [Reviews list](reviews-list.md)
- [New Review](new-review.md)
