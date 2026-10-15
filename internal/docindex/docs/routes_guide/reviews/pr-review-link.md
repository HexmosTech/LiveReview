# PR Review Link (start a review from a PR description link)

**Route:** `/pr/:id/review`
**Component:** `ui/src/pages/Reviews/PullRequestReviewTrigger.tsx`

## Purpose

When a new pull/merge request is opened on GitHub, GitLab, Bitbucket, Gitea, or Azure DevOps in a repo with the
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
- When a review started from this link finishes, LiveReview runs the org's
  [auto-approve policy](../settings/auto-approve.md) on it. The result is shown
  in two places:
  - **PR description:** under the two links, an "Auto-approval decisions"
    table with one row per policy-checked review, latest first: Review (link),
    Decision (✅ Approved / ❌ Not approved), and Reason (the failing
    stage and why, or "All policy checks passed").
  - **Separate PR comment:** the verdict plus a stage-by-stage breakdown
    (eligibility, static rules, risk score, LLM classification, final decision).
    How the verdict is applied depends on the provider:
    - GitHub / Gitea: an approve or "Request changes" review (a plain comment
      if the provider refuses, e.g. the bot authored the PR).
    - GitLab: a comment; the MR is approved when it passes.
    - Bitbucket: a comment, then Approve or "Request changes".
    - Azure DevOps: a comment thread, then the bot votes Approved or
      "Waiting for author".
  - The decisions table shows the last 10 policy-checked reviews.
- Reviews started any other way (reviewer assignment, @mentions, Explore) don't
  run the policy check.

## Related pages

- [Reviews (list)](reviews-list.md)
- [Review Detail](review-detail.md)
