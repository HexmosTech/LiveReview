# Settings → Plan & Usage

**Route(s):** `/settings#subscriptions`, plus dedicated sub-routes
`/settings-subscriptions-overview`, `/settings-subscriptions-breakdown`,
`/settings-subscriptions-assign`, `/settings-subscriptions-portfolio`
(all render `Settings.tsx` pinned to the `subscriptions` tab)
**Who sees it:** any org member, cloud mode only

## Purpose

View and manage the org's cloud subscription/billing: current plan, usage
against quota (lines-of-code billed, etc.), and seat assignment for
license-per-user plans.

## Key actions

- View current plan, usage breakdown, and billing period.
- Assign/unassign seats to org members.
- Cancel subscription (`CancelSubscriptionModal`).
- Upgrade plan (links to [Subscribe](../subscribe.md) / [Team Checkout](../checkout-team.md)).

## Related pages

[Settings overview](settings-overview.md), [Subscribe](../subscribe.md),
[License Management](../licenses/license-management.md),
[License Assignment](../licenses/license-assignment.md)
