# Admin → Billing Portfolio

**Route:** `/admin/billing-portfolio`
**Component:** `ui/src/pages/Admin/BillingPortfolio.tsx`

## Purpose

Cross-org billing dashboard for LiveReview operators (not org-scoped):
total orgs, active orgs, total billable lines-of-code, total operations,
net revenue collected, failed payments — plus a per-org and per-member
usage breakdown.

## Who can access it

super_admin only.

## Key actions

- View portfolio-wide billing summary.
- Drill into per-org usage (LOC used, plan, billing period, revenue,
  failed payments).
- Drill into per-member usage within an org.

## Related pages

[Settings → Plan & Usage](settings/plan-and-usage.md)
