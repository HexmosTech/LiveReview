# License Management

**Route:** `/subscribe/manage`
**Component:** `ui/src/pages/Licenses/LicenseManagement.tsx`

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
