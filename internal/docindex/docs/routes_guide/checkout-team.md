# Team Checkout

**Route:** `/checkout/team`
**Component:** `ui/src/pages/Checkout/TeamCheckout.tsx`

> **Deprecated - per-seat licensing.** LiveReview no longer gates team size on
> purchased seats. Org owners simply add as many users as they want in
> [Settings -> User Management](settings/user-management.md); no seat needs
> to be bought or assigned first. This page still exists for orgs on legacy
> seat-based subscriptions, but it is **not** part of onboarding a new team
> member - do not recommend it as an onboarding step.

## Purpose

Checkout flow specifically for purchasing a team plan (multiple seats) via
Razorpay, separate from the general [Subscribe](subscribe.md) plan-picker.

## Who can access it

Org owner or super_admin (cloud mode).

## Key actions

- Configure seat quantity and complete Razorpay payment for a team plan.

## Related pages

[Subscribe](subscribe.md), [License Assignment](licenses/license-assignment.md)
