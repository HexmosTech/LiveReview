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
