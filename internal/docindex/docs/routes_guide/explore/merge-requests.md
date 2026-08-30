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
- Trigger a review directly for a specific pull request
  (`triggerReviewForPullRequest`).

## Related pages

- [Explore → Repositories](repositories.md)
- [Reviews list](../reviews/reviews-list.md)
