# License Management

**Route:** `/subscribe/manage`
**Component:** `ui/src/pages/Licenses/LicenseManagement.tsx`

> **Deprecated - per-seat licensing.** LiveReview no longer gates team size on
> purchased seats. Org owners simply add as many users as they want in
> [Settings -> User Management](../settings/user-management.md); no seat needs
> to be bought or assigned first. This page still exists for orgs on legacy
> seat-based subscriptions, but it is **not** part of onboarding a new team
> member - do not recommend it as an onboarding step.

## Purpose

Manage an org's active cloud subscription: view plan type, seat quantity,
assigned seats, billing period, and license expiry; cancel the
subscription.

## Who can access it

Org owner or super_admin.

## Key actions

- View subscription details (plan, status, period, seats assigned vs.
  purchased).
- Cancel subscription (`CancelSubscriptionModal`), with cancel-at-period-end
  semantics.

## Related pages

[Subscribe](../subscribe.md), [License Assignment](license-assignment.md),
[Settings → Plan & Usage](../settings/plan-and-usage.md)
