# Reports → Auto Approve

**Route:** `/reports/auto-approve`
**Component:** `ui/src/pages/Reports/AutoApproveReport.tsx`
**API:** `GET /api/v1/auto-approve-insights?window=7|30|90`
(`internal/api/auto_approve_insights_handlers.go`)

## Purpose

The "is the risk-based auto-approve policy behaving reasonably" view. The
per-review explain panel (`AutoApprovePanel.tsx`, on a review's detail page)
answers "why did this one PR get approved or blocked" — this page answers
the aggregate question across an org's real traffic, over a selectable
window: where real review scores land, how many PRs were approved/blocked/
overridden, and — once production outcomes are reported — how often an
approval turned out to be wrong.

## Sections

| Section | Covers |
|---|---|
| Score distribution | A histogram of every review's computed risk score in the window, with the org's current percentile cutoff highlighted, so an admin can see directly how many real PRs fall on each side of the threshold before changing it. |
| Decisions | KPI counts (approved/blocked/overridden, override rate) plus a per-stage block breakdown (eligibility/static/risk/LLM) and a day-bucketed approved-vs-blocked trend chart. |
| Calibration | Reported production outcomes (`revert`/`incident`/`confirmed_safe`) cross-tabbed against what the policy actually decided, surfacing a false-approve rate. Shows a low-volume warning below 10 reported outcomes rather than presenting a rate that isn't yet meaningful. |

## Who can access it

Owner/admin only (`canManageCurrentOrg` client-side gate; the API's
`/auto-approve-insights` route requires org access via the standard
`RequireAuthOrAPIKey` → `BuildOrgContextFromHeader` → `ValidateOrgAccess` →
`BuildPermissionContext` chain, scoped strictly to the caller's `org_id`).

## How to get here

- Mega menu: **Reports → Auto Approve**.
- From **Settings → Auto Approve Policy → Production Feedback**, via the
  "View full report" link.
- Directly at `/reports/auto-approve`.

## Key actions

- Switch the window (7/30/90 days) to see more or less history.
- Read the score histogram against the current threshold before adjusting
  the risk percentile in Settings → Risk Model.
- Watch the calibration table as outcomes accumulate to judge whether the
  policy's false-approve rate is acceptable.

## Related pages

[Settings → Auto Approve Policy](../settings/auto-approve.md),
[Reports → Taxonomy Reports](taxonomy-reports.md)
