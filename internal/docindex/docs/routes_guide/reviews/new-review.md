# New Review

**Route:** `/reviews/new`
**Component:** `ui/src/pages/Reviews/NewReview.tsx`

## Purpose

Start a code review of one pull/merge request right away (as opposed to it
firing from a git webhook or the `lrc` CLI hook). Below the form, an "Other
ways to review" section lists the other ways reviews can start.

## Who can access it

Any authenticated org member, on every plan including Free (a Free org reviews
with its own AI key) — but review triggering can be blocked by
quota/billing state (trial exhausted, plan usage cap hit), shown via
`QuotaExhaustedBanner` / `QuotaWarningBanner` and `LicenseUpgradeDialog` for
self-hosted license limits.

## Key actions

- **Back** (top left) returns to the previous page.
- Type a repository name, PR title or author to see matching open PRs. Click
  one, or use ↑/↓ and Enter, to start its review.
- Or paste a PR/MR URL and click **Start review**. Either way the new review's
  page opens right away, and for a PR LiveReview already synced the org's
  auto-approve policy runs when the review finishes.
- On LiveReview Cloud, if the pasted URL is a GitHub pull request in a repository
  the org's LiveReview GitHub App connections don't cover, a dialog asks first:
  **Install GitHub App on `<owner>`** or **Continue review without posting
  comments** (runs the review and shows the findings in LiveReview without
  posting to the pull request). Installing opens GitHub's install page, already
  on that owner, in a new tab, so this page and the review being set up stay
  put; the other tab signs in again and lands on the connected GitHub accounts.
  The collapsible **Finished installing on
  GitHub?** section appears under the button right away (the button shows "Opening GitHub…" for two seconds before the tab actually opens), with a highlighted **I've
  installed it, continue review** button. It checks that LiveReview can now see
  the installation and then starts the review; if it can't yet, it says so and
  the user can retry.
- Hover a provider chip (GitHub, GitLab, Bitbucket, Gitea, Azure DevOps) to see
  its URL format.
- **Other ways to review** cards: On the PR (assign the bot as reviewer,
  @mention it, or use the PR description's Start a review link), Pre-commit CLI,
  IDE / MCP, CI/CD & API (CI rulesets), Chatbot (opens Livi with a review prompt),
  and Scheduled reviews.
- If blocked by quota, see an upgrade prompt (`UpgradePromptModal`) instead.

## Related pages

- [Reviews list](reviews-list.md)
- [Review Detail](review-detail.md)
- [Explore → Merge Requests](../explore/merge-requests.md)
- [Create Review via CLI](create-review-cli.md)
- [Create Review via MCP](create-review-mcp.md)
- [AI Providers](../ai/ai-providers.md)
