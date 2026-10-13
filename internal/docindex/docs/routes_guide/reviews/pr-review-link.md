# PR Review Link (start a review from a PR description link)

**Route:** `/pr/:id/review`
**Component:** `ui/src/pages/Reviews/PullRequestReviewTrigger.tsx`

## Purpose

When a new pull/merge request is opened on GitHub or GitLab in a repo with the
LiveReview webhook installed, LiveReview adds a section with two links to the end of its description:

- **Start a review** → this page. It starts a LiveReview review of that PR and
  opens the review's detail page.
- **Review history** → the [Reviews list](reviews-list.md) filtered to that
  PR (`/reviews?pull_request_id=<id>`).

`:id` is LiveReview's internal pull request id, not the GitHub/GitLab PR number.
The section is added once per PR, only when it's opened (not for PRs that
already existed, reopens, or pushes).

## Who can access it

Any authenticated member of the org the PR belongs to. Logged-out users are
asked to log in and then sent back to the link. If the user's active org is a
different one, LiveReview switches to the PR's org automatically. Users who
aren't members of that org see "pull request wasn't found".

## Key actions

- Opening the link starts a review right away; there is no confirm step.
- If a review of the same PR started less than 10 minutes ago and is still
  running, the link opens that review instead of starting another one.
- Each later click starts a new review (counts against the org's review quota).
- When a review started from this link finishes, LiveReview checks it against a
  fixed approval policy: **no critical findings**. It posts a policy comment on
  the PR (counts of critical/warning/info findings, plus up to 5 critical
  findings) and then:
  - GitHub: approves the PR, or submits "Request changes" if it failed. If
    GitHub refuses (e.g. the bot account authored the PR), only the comment is
    posted.
  - GitLab: approves the MR when it passes; a failure is comment-only.
- Reviews started any other way (reviewer assignment, @mentions, Explore) don't
  run the policy check.

## Related pages

- [Reviews (list)](reviews-list.md)
- [Review Detail](review-detail.md)
