# Subscribe

**Route:** `/subscribe`
**Component:** `ui/src/pages/Subscribe/Subscribe.tsx`

## Purpose

Plan-selection/upgrade page for cloud orgs — shows available plan tiers and
their features, and kicks off checkout via Razorpay.

## Who can access it

Org owner or super_admin (cloud mode).

## Key actions

- Compare plans/pricing.
- Start checkout (loads Razorpay checkout script, opens payment modal).

## Related pages

[License Management](licenses/license-management.md), [Team Checkout](checkout-team.md),
[Settings → Plan & Usage](settings/plan-and-usage.md)
