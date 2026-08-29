# New Review

**Route:** `/reviews/new`
**Component:** `ui/src/pages/Reviews/NewReview.tsx`

## Purpose

Form to manually trigger a code review (as opposed to it firing from a git
webhook or the `lrc` CLI hook). Lets the user pick a connected AI provider
and target repository/PR, then submits the review job.

## Who can access it

Any authenticated org member — but review triggering can be blocked by
quota/billing state (trial exhausted, plan usage cap hit), shown via
`QuotaExhaustedBanner` / `QuotaWarningBanner` and `LicenseUpgradeDialog` for
self-hosted license limits.

## Key actions

- Select an AI connector and repository/PR to review.
- Submit the review (`triggerReview` API call); redirects to the new
  review's detail page on success.
- If blocked by quota, see an upgrade prompt (`UpgradePromptModal`) instead.

## Related pages

- [Reviews list](reviews-list.md)
- [Review Detail](review-detail.md)
- [Create Review via CLI](create-review-cli.md)
- [Create Review via MCP](create-review-mcp.md)
- [AI Providers](../ai/ai-providers.md)
