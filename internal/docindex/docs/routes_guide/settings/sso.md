# Settings → SSO

**Route:** `/settings#sso`
**Who sees it:** self-hosted deployments only (super_admin or org owner).
The tab is not rendered at all in cloud mode - see `isCloudMode()` in
`ui/src/pages/Settings/Settings.tsx` (same gating as [License](license.md)).

## Purpose

Configure enterprise Single Sign-On so an organization's members can log
in with the company's identity provider (Okta, Azure AD/Entra ID, Google
Workspace, Keycloak, ADFS, etc.) instead of - or in addition to - an
email/password account. SSO is per-organization, additive, and optional:
turning it on for an org never disables password login for that org.

Both OIDC and SAML 2.0 are supported, as two independently configured
providers an org can enable at the same time. Each has its own tab inside
the page (`ui/src/pages/Settings/SSOSettingsTab.tsx`).

## Provisioning behavior

The **first successful SSO login** for a given email auto-creates ("JIT
provisions") that user as a `member` of the org the SSO config belongs to,
if they aren't already a member. There is no separate "invite" step - an
org owner enabling SSO is granting entry to anyone who can authenticate
against that configured IdP, so the IdP's own group/access rules are the
real gatekeeper.

## OIDC fields

- **Display Name** - shown on the login page's "Continue with {name}" button.
- **Issuer URL** - the IdP's OIDC discovery issuer (e.g.
  `https://your-idp.example.com/realms/your-realm`); discovery
  (`/.well-known/openid-configuration`) is fetched live on every login, not cached.
- **Client ID** / **Client Secret** - from the IdP's OIDC client registration.
  The secret is encrypted at rest and never redisplayed once saved (the
  input just shows whether one is set).
- **Scopes** - space-separated, optional (defaults cover `openid email profile`).
- Give the IdP this **Redirect URI**: `{origin}/api/v1/auth/sso/oidc/callback`.

## SAML fields

- **Display Name** - same as above.
- **IdP Metadata URL** or pasted **IdP Metadata XML** (the XML, if provided,
  takes priority over the URL).
- Give the IdP these: **SP Entity ID / Metadata URL**
  `{origin}/api/v1/auth/sso/saml/{org_id}/metadata` and **ACS URL**
  `{origin}/api/v1/auth/sso/saml/{org_id}/acs`.
- Requests are sent unsigned; LiveReview's SP has no signing keypair in this
  release. The IdP's response/assertion signature is still verified.

## Key actions

- Toggle between the OIDC and SAML tabs; each provider's config is independent.
- Fill in the provider's fields and click **Test Connection** (hits OIDC
  discovery or fetches/parses SAML metadata without saving) before saving.
- **Save** to persist, **Delete** to remove a provider's configuration entirely.
- Flip **Enabled** on a saved config to let users sign in with it immediately,
  or off to stop new SSO logins without losing the saved settings.

## Deprovisioning: how a removed user actually loses access

JIT provisioning only covers getting users *in*. There are three distinct
mechanisms for getting a removed/disabled IdP user *out*, each covering a
different gap:

1. **Manual deactivation always works.** An owner deactivating a user
   (Settings → Users) immediately revokes every one of that user's live
   sessions - their JWT stops working on the very next request, not just on
   future logins. SSO login itself can never undo a deactivation: if a
   deactivated user's IdP credentials still work, login is rejected rather
   than silently reactivating them. Reactivating is an explicit admin action.
2. **SCIM (below) is the real-time, automated answer**, if the org wires it
   up at their IdP. Without it, deprovisioning depends on a human noticing
   or the two fallbacks below.
3. **The reconciliation poll** (OIDC panel, "Automatic deprovisioning
   check") and **OIDC back-channel logout** are two best-effort fallbacks
   for orgs that skip SCIM - see their own sections below. Neither applies
   to SAML; a SAML-only org that wants automated deprovisioning needs SCIM.

## SCIM provisioning

A "SCIM Provisioning" panel sits below the OIDC/SAML tabs on this same page,
independent of which login protocol(s) are configured. This lets the IdP
itself (Okta, Azure AD/Entra ID, Google Workspace) push user lifecycle
events directly - most importantly, deactivating a LiveReview user the
moment the IdP admin removes or disables them there, rather than waiting for
that user to attempt another login.

- **Generate Token** creates the org's SCIM bearer token, shown once (same
  reveal-once convention as an API key) - copy it into the IdP's SCIM app
  configuration alongside the shown **SCIM base URL**
  (`{origin}/api/v1/scim/v2`).
- **Rotate Token** replaces the token; the old one stops working
  immediately, so the IdP's SCIM config needs updating with the new one.
- **Revoke** disables SCIM entirely until a new token is generated.
- Requires the `scim` plan feature (same plan entry as `sso`,
  `internal/license/plans.go`'s `PlanEnterpriseSelfhosted`).
- The IdP authenticates every SCIM call with this bearer token alone - there
  is no user session involved, and the org is resolved from the token
  itself, never a client-supplied id.

## Automatic deprovisioning check (OIDC reconciliation poll)

Only shown on the OIDC panel. A fallback for orgs that configure OIDC login
but never wire up SCIM above (a separate integration step many admins skip).
When enabled, LiveReview periodically re-checks each signed-in user against
the IdP (silently refreshing a stored OAuth token) and deactivates them here
if the IdP rejects it - the same outcome a SCIM deactivation would produce,
just on a delay bounded by the interval you pick (hourly/daily/weekly,
default daily) instead of instantly. Not available for SAML - there's no
equivalent standing credential to silently re-check with that protocol.

## Where this shows up elsewhere

- The self-hosted login page (`ui/src/pages/Auth/SelfHosted.tsx`) calls the
  public `GET /api/v1/auth/sso/providers` endpoint and renders a "Continue
  with {name}" button per enabled provider, above the password form.
- A successful login (either protocol) redirects the browser to
  `/sso/callback` (`ui/src/pages/Auth/SSOCallback.tsx`), which picks the
  issued token pair out of the URL fragment and completes sign-in exactly
  like password login.
- This feature requires the `sso` entry in the org's plan features
  (`internal/license/plans.go`'s `PlanEnterpriseSelfhosted`); cloud orgs
  don't have it today.

## Related pages

[Settings overview](settings-overview.md), [License](license.md)
