# Scheduled Reviews

**Route(s):** `/reviews/scheduled` (list), `/reviews/scheduled/:repositoryId/runs` (run history)
**Components:** `ui/src/pages/Reviews/ScheduledReviews.tsx`,
`ui/src/pages/Reviews/ScheduledReviewRuns.tsx`

## Purpose

Manage cron-based recurring reviews per repository (e.g. "review this repo
every day at 9am") instead of only reviewing on push/PR events. The list
page shows every repo with its current schedule (on/off + cron expression,
shown in the user's local timezone); the runs page shows the history of
scheduled-review executions for one repository, with outcomes (`reviewed`,
`no_changes`, etc.).

## Who can access it

Any authenticated org member with access to the relevant repositories.

## Key actions

- Toggle a repository's scheduled review on/off.
- Edit a repository's cron schedule via `EditScheduleModal` (default
  `0 9 * * *`).
- Search/filter repositories by provider, name, etc.
- Click a repo to view its scheduled-run history
  (`/reviews/scheduled/:repositoryId/runs`).

## Related pages

- [Reviews list](reviews-list.md)
- [Explore Repositories](../explore/repositories.md)
