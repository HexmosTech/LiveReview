# Create Review via CLI

**Route:** `/reviews/create-cli`
**Component:** `ui/src/pages/Reviews/CreateReviewCLI.tsx`

## Purpose

Standalone onboarding page (reachable from the mega menu's "Create via CLI"
entry) that walks a user through installing and using `lrc`, the git-lrc CLI
tool that triggers reviews from `git commit`. Shows the same onboarding
content as the dashboard's floating banner (via the shared
`OnboardingSteps` component) but landed on directly instead of appearing as
an overlay on the dashboard. Reuses the dashboard's cached data query so
visiting right after the dashboard doesn't refetch.

## Who can access it

Any authenticated org member. Free-plan orgs may see an upgrade dialog
(`LicenseUpgradeDialog`) for gated capabilities.

## Key actions

- Copy the CLI install command (pre-filled with the org's onboarding API key
  when available).
- See live onboarding status: CLI installed?, AI provider connected?, first
  review run yet?

## Related pages

- [Dashboard](../dashboard.md)
- [Create Review via MCP](create-review-mcp.md)
- [New Review](new-review.md)
