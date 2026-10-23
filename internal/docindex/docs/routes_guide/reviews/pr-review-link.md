# PR Review Link (start a review from a PR description link)

**Route:** `/pr/:id/review`
**Component:** `ui/src/pages/Reviews/PullRequestReviewTrigger.tsx`

## Purpose

When a new pull/merge request is opened on GitHub, GitLab, Bitbucket, Gitea, or Azure DevOps in a repo with the
LiveReview webhook installed, LiveReview adds a section to the end of its description:

- **Start a review** → this page. It starts a LiveReview review of that PR and
  opens the review's detail page.
- **Review history** → the [Reviews list](reviews-list.md) filtered to that
  PR (`/reviews?pull_request_id=<id>`).
- A line under the auto-approval decisions (or under the two links before any
  decision): "Customize risk-based approval policy for **this repository** or
  **your organization**". The repository link opens the repo's settings page,
  Auto Approve section (`/explore/repositories/<repo id>/settings#auto-approve`);
  the organization link opens [Settings → Auto Approve Policies](../settings/auto-approve.md)
  (`/settings#auto-approve`). Both pages are for org owners and admins, and
  open in the user's current org.

`:id` is LiveReview's internal pull request id, not the GitHub/GitLab PR number.
The section is added once per PR, only when it's opened (not for PRs that
already existed, reopens, or pushes).

## Who can access it

Any authenticated member of the org the PR belongs to. Logged-out users are
asked to log in and then sent back to the link. If the user's active org is a
different one, LiveReview switches to the PR's org automatically. When several
organizations share one GitHub App installation, each keeps its own copy of the
PR and the link names one of them; a user from another of those organizations
is taken to their own organization's copy, and the review runs there. Users who
aren't members of any organization with that PR see "pull request wasn't found".

## Key actions

- Opening the link starts a review right away; there is no confirm step.
- If a review of the same PR started less than 10 minutes ago and is still
  running, the link opens that review instead of starting another one.
- Each later click starts a new review (counts against the org's review quota).
- If the organization has no AI provider (the Free plan needs one), the page
  explains that instead of showing a raw error: it points to Google AI Studio
  for a free Gemini API key, offers **Configure AI** (opens AI Providers in a
  new tab) and a **Review this PR** button to start the review again once the
  provider is added.
- When a review started from this link finishes, LiveReview runs the org's
  [auto-approve policy](../settings/auto-approve.md) on it. The result is shown
  in two places:
  - **PR description:** under the two links, an "Auto-approval decisions"
    table with one row per policy-checked review, latest first: Review (link),
    Decision (✅ Approved / ❌ Not approved / ⚠️ Force-approved), and Reason
    (the failing stage and why, the repo mode override such as
    "mode override: always_approve", the force-approve reason, or "All policy
    checks passed").
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
- Reviews started by hand for a known PR also run the policy check and get the
  Auto Approve tab: pasting the PR URL on New Review, or Start review in the
  Explore PR table. Reviewer assignment and @mention reviews don't.

## Related pages

- [Reviews (list)](reviews-list.md)
- [Review Detail](review-detail.md)
