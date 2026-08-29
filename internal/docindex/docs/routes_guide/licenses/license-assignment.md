# License Assignment

**Route:** `/subscribe/subscriptions/:id/assign`
**Component:** `ui/src/pages/Licenses/LicenseAssignment.tsx`

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
