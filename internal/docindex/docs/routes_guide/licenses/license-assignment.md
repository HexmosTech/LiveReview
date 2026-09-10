# License Assignment

**Route:** `/subscribe/subscriptions/:id/assign`
**Component:** `ui/src/pages/Licenses/LicenseAssignment.tsx`

> **Deprecated - per-seat licensing.** LiveReview no longer gates team size on
> purchased seats. Org owners simply add as many users as they want in
> [Settings -> User Management](../settings/user-management.md); no seat needs
> to be bought or assigned first. This page still exists for orgs on legacy
> seat-based subscriptions, but it is **not** part of onboarding a new team
> member - do not recommend it as an onboarding step.

## Purpose

Assign purchased seats on a specific subscription to individual org
members (per-user licensing), and see payment status for that
subscription.

## Who can access it

Org owner or super_admin.

## Key actions

- Assign a seat to a member (by email).
- Unassign a seat.
- View subscription payment status (last payment id/status, owner info).
- Trigger upgrade flow if seats are exhausted (`UpgradePromptModal`).

## Related pages

[License Management](license-management.md), [Settings → Plan & Usage](../settings/plan-and-usage.md)
