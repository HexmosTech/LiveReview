Automatically review a repository's default branch on a recurring schedule, without waiting for a pull request.

## What it does

A scheduled review diffs a repository's default branch against the last point it reviewed, and runs that diff through the standard AI review engine — posting comments the same way a normal review would. It's meant for repositories where changes land straight on the default branch (or infrequently open PRs), not as a replacement for PR reviews.

It only reviews what changed *since the last run* (a checkpoint commit SHA), not the whole branch history each time.

> Scheduled reviews currently support GitHub repositories only. Other providers will show as skipped in the run history until support is added.

## Enable it

1. Go to **Reviews → Schedule Review** in the sidebar (`/reviews/scheduled`)
2. Find the repository in the list and toggle it **on**
3. Click the schedule to open the editor and set a cron expression, or leave it at the default (daily at 9 AM UTC)

The schedule builder supports Hourly, Daily, Weekly, Monthly, or a raw Custom cron expression. Times you enter are in your browser's local timezone; the resulting schedule is converted to and stored/executed in UTC. You can also select multiple repositories and edit their schedule in bulk.

## View run history

Click into a repository's schedule to see its past runs (`/reviews/scheduled/:repositoryId/runs`). Each run records one of:

| Outcome | Meaning |
|---|---|
| `reviewed` | New commits were found and reviewed |
| `no_changes` | Nothing new since the last checkpoint — nothing to review |
| `failed` | The run errored (see the run's error message) |
| `skipped_unsupported_provider` | The repository's connector isn't GitHub |
| `quota_blocked` | The org's review quota (lines-of-code plan limit) was exhausted |

## Notes

- Scheduled review runs count against your plan's LOC quota, same as any other review.
- If a schedule's cron expression becomes invalid (e.g. hand-edited via the API into a bad value), that run fails and logs the parse error rather than silently running on the wrong schedule — fix the cron expression via the UI to resume.
- Disabling a repository's schedule stops future runs but doesn't delete its run history or checkpoint; re-enabling picks up from where it left off.
