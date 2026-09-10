# Settings → Plan & Usage

**Route(s):** `/settings#subscriptions`, plus the sub-tab deep links
`/settings-subscriptions-overview`, `/settings-subscriptions-breakdown`,
`/settings-subscriptions-assign`, `/settings-subscriptions-portfolio`
(all render `Settings.tsx` pinned to the `subscriptions` tab)
**Component:** `ui/src/pages/Settings/SubscriptionTab.tsx`
**Who sees it:** any org member, **cloud mode only** — the tab is registered
under `isCloudMode()` in `Settings.tsx`, so self-hosted instances never show
it (they use [Settings → License](license.md) instead)

## Purpose

The org's cloud billing home: which plan you're on, what you've used against
quota (lines of code billed, etc.), and the billing period.

> **Note on seats.** Per-seat licensing is deprecated — this page is not where
> you make room for a new team member. Adding users is unlimited and happens
> in [User Management](user-management.md). The assignment UI described below
> survives for orgs on older seat-based subscriptions.

## Sub-tabs

| Sub-tab | Deep link | Shows |
|---|---|---|
| Overview | `/settings-subscriptions-overview` | Current plan, status, billing period |
| Breakdown | `/settings-subscriptions-breakdown` | Usage against quota, itemized |
| Control | `/settings-subscriptions-assign` | Cancellation, downgrade, payment link, and legacy access assignment |

The Control sub-tab links out to the standalone advanced assignment page at
`/subscribe/subscriptions/:id/assign` — legacy seat-based plans only.

## Key actions

- View current plan, usage breakdown, and billing period.
- Cancel or downgrade the subscription (`CancelSubscriptionModal`).
- Open the payment link.
- Upgrade the plan (links to [Subscribe](../subscribe.md)).

## Related pages

[Settings overview](settings-overview.md), [Subscribe](../subscribe.md),
[Settings → License](license.md),
[License Management](../licenses/license-management.md)
