# Setup

**Route:** shown in place of any route when the instance requires initial
setup (`isSetupRequired`)
**Component:** `ui/src/pages/Setup/Setup.tsx`

## Purpose

First-run wizard for a fresh self-hosted LiveReview instance — creates the
first super_admin account and the initial organization.

## Who can access it

Anyone, only while the instance has no admin account yet.

## Key actions

- Submit admin email, password (min 8 characters), and organization name to
  bootstrap the instance. Logs the new admin in immediately on success.

## Related pages

[Login](login.md), [Dashboard](../dashboard.md)
