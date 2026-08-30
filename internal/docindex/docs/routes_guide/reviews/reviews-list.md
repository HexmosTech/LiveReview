# Reviews (list)

**Route:** `/reviews`
**Component:** `ui/src/pages/Reviews/Reviews.tsx`

## Purpose

Table of every code review LiveReview has run for the org — one row per
review submission (from `lrc` CLI, webhook trigger, MCP, or manual). Shows
status, repository, PR/branch, findings count, and relative time.

## Who can access it

Any authenticated org member.

## Key actions

- Search/filter/sort reviews (by status, repo, provider, etc.) using
  column header filters and a search box.
- Click a row to open [Review Detail](review-detail.md).
- Start a new review via "New Review" (`/reviews/new`).
- Navigate to scheduled reviews (`/reviews/scheduled`).

## Related pages

- [Review Detail](review-detail.md)
- [New Review](new-review.md)
- [Scheduled Reviews](scheduled-reviews.md)
- [Dashboard](../dashboard.md)
