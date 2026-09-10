# Settings → Instance

**Route:** `/settings#instance`
**Who sees it:** super_admin, or an org **owner** on a self-hosted instance
(`canManageInstanceConfig` in `ui/src/pages/Settings/Settings.tsx`). In cloud
mode this config is shared across every tenant, so it stays super_admin-only
there.

## Purpose

Instance-wide configuration for a self-hosted LiveReview deployment (e.g.
production URL). Not org-scoped — applies to the whole instance.

## Key actions

- View/update the instance's production URL and related instance-level
  settings.

## Related pages

[Settings overview](settings-overview.md)
