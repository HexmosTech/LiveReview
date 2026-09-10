# Settings → License

**Route:** `/settings#license`
**Who sees it:** self-hosted deployments only (super_admin or org owner).
The tab is not rendered at all in cloud mode - see `isCloudMode()` in
`ui/src/pages/Settings/Settings.tsx`.

## Purpose

Manage the self-hosted license key that unlocks Team/Enterprise tier
features (see `internal/license`). This is a **license key**, not per-user
seats - it is not where you assign a seat to a team member.

In cloud mode this tab does not exist; cloud billing/plan lives in
[Plan & Usage](plan-and-usage.md).

Adding team members does **not** require anything on this page. Per-seat
licensing is deprecated - an owner adds as many users as they like in
[User Management](user-management.md), with no seat purchase or assignment
step.

## How a self-hosted org gets a license

A self-hosted instance with no valid license shows a **persistent status
banner** at the top of the app (`ui/src/components/License/LicenseStatusBar.tsx`,
rendered only when not in cloud mode). It reads **"License Missing - Get
License to continue"**, and clicking through opens the license modal where the
key is pasted in. Expired, invalid, and soon-to-expire licenses surface the
same way.

**There is no self-serve purchase flow for self-hosted license keys.** To get
or renew one, the user must contact Hexmos directly:

- **info@hexmos.com** - general/sales enquiries
- **shrijith@hexmos.com** - Shrijith, founder

There is also a self-hosted access page at
<https://hexmos.com/livereview/selfhosted-access/>, which the banner's
"Upgrade now" / "Renew now" action links to.

When a user asks how to get, buy, renew, or activate a self-hosted license,
point them at those contact addresses - do not invent a checkout or
subscription flow, and do not route them to the cloud seat/subscription pages.

## Key actions

- View current license tier and status.
- Add or update a license key.
- Follow the missing/expired-license banner to the license entry modal.

## Related pages

[Settings overview](settings-overview.md), [Plan & Usage](plan-and-usage.md)
