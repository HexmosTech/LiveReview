# Settings → Account

**Route:** `/settings#account`
**Who sees it:** every authenticated user (self-service, no org-role gate)

## Purpose

Manage your own profile and credentials: display name and password. This
is the only Settings tab that isn't permission-gated by org role — it acts
on the signed-in user's own account, not on org or instance configuration.

## Key actions

- Update your first/last name.
- Change your password (requires the current password; requires an active
  user session — rejected if called via an API key, per the scoping
  guardrails for sensitive account actions).

## Related pages

[Settings overview](settings-overview.md), [API Keys](api-keys.md)
