# Dashboard

**Route(s):** `/` (`HomeWithOAuthCheck`, redirects to Dashboard after handling
any pending OAuth callback params), `/dashboard` (`Dashboard`)
**Component:** `ui/src/pages/Home/HomeWithOAuthCheck.tsx`,
`ui/src/components/Dashboard/Dashboard.tsx`

## Purpose

The landing page after login. Shows a summary of the organization's
LiveReview usage: total code reviews run, active AI connectors, whether the
CLI (`lrc`) is installed, and onboarding progress. Also surfaces quota/trial
billing banners when usage limits are being approached or exceeded.

## Who can access it

Any authenticated user of the org.

## Key actions

- View onboarding checklist (install CLI, connect an AI provider, run first
  review) via `OnboardingSteps`.
- See quota warnings/exhaustion banners (`QuotaWarningBanner`,
  `QuotaExhaustedBanner`) with links to upgrade.
- Navigate to Reviews, Explore, Settings, etc. from here.

## Related pages

- [Create Review via CLI](reviews/create-review-cli.md), [Create Review via
  MCP](reviews/create-review-mcp.md) — dedicated onboarding entry points
  reachable from the mega menu, sharing the same onboarding data as this page.
- [Reviews](reviews/reviews-list.md)
