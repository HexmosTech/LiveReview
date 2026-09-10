# License Management

**Route:** `/subscribe/manage`
**Component:** `ui/src/pages/Licenses/LicenseManagement.tsx`

> **Legacy page — per-seat licensing is deprecated.** LiveReview no longer
> gates team size on purchased seats. Org owners add as many users as they
> want in [Settings → User Management](../settings/user-management.md), with
> no seat to buy or assign first. This page remains for orgs still on older
> seat-based subscriptions. **Never present it as part of adding or
> onboarding a team member.**

## Purpose

View and manage an existing cloud subscription: plan type, billing period,
license expiry, and cancellation. On seat-based legacy subscriptions it also
shows seat quantity and how many are assigned — historical detail, not
something a current org needs to act on.

## Who can access it

Org owner or super_admin.

## Key actions

- View subscription details (plan, status, billing period).
- Cancel the subscription (`CancelSubscriptionModal`), with
  cancel-at-period-end semantics.
- On legacy seat plans only: see seats assigned vs. purchased.

## Related pages

[Subscribe](../subscribe.md),
[Settings → Plan & Usage](../settings/plan-and-usage.md)
