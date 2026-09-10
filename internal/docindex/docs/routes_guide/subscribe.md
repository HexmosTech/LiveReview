# Subscribe

**Route:** `/subscribe`
**Component:** `ui/src/pages/Subscribe/Subscribe.tsx`

## Purpose

Plan-selection/upgrade page for **cloud** orgs — shows available plan tiers
and their features, and kicks off checkout via Razorpay.

> **Note on seats.** Per-seat licensing is deprecated. Plans are no longer a
> way to buy headcount: an org owner adds as many users as they want in
> [Settings → User Management](settings/user-management.md) without buying or
> assigning a seat first. Upgrading is about plan tier and features, not team
> size.

Self-hosted instances do not use this page at all — they are unlocked with a
license key instead; see [Settings → License](settings/license.md) and
[Contact us](contact.md).

## Who can access it

Org owner or super_admin (cloud mode).

## Key actions

- Compare plans/pricing.
- Start checkout (loads the Razorpay checkout script, opens the payment
  modal).

## Related pages

[Settings → Plan & Usage](settings/plan-and-usage.md),
[Settings → License](settings/license.md), [Contact us](contact.md)
