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
- View one pull request's review history: `/reviews?pull_request_id=<id>`
  (the "View review history" link LiveReview adds to the description of new GitHub/GitLab
  PRs). Switches to the PR's org if needed. A "Filtered to one pull request"
  chip shows above the table; click **Clear** to see all reviews again.

## Related pages

- [Review Detail](review-detail.md)
- [PR Review Link](pr-review-link.md)
- [New Review](new-review.md)
- [Scheduled Reviews](scheduled-reviews.md)
- [CI/CD Gates](ci-cd-gates.md)
- [Dashboard](../dashboard.md)
