# Explore → Merge Requests

**Route:** `/explore/merge-requests`
**Component:** `ui/src/pages/Explore/MergeRequests.tsx`

## Purpose

Table of open pull/merge requests across all connected repositories, with
their review state (PR open/merged/closed and whether it's been reviewed).

## Who can access it

Any authenticated org member.

## Key actions

- Search/filter/sort PRs by provider, repo, state, etc.
- Click a row to see that PR's review history in the [Reviews list](../reviews/reviews-list.md).
  Rows are real links, so right-click gives "Open in new tab" and middle/Ctrl-click
  work anywhere in the row; the PR title still links to the provider, and the
  Actions button works as before.
- Trigger a review directly for a specific pull request
  (`triggerReviewForPullRequest`). It opens the new review's page right away,
  and the review runs the auto-approve policy check when it finishes.

## Related pages

- [Explore → Repositories](repositories.md)
- [Reviews list](../reviews/reviews-list.md)
