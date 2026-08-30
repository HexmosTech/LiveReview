# Login

**Route:** shown whenever the user is unauthenticated (root `/`, or any
route redirect target)
**Component:** `ui/src/pages/Auth/Login.tsx`, which renders `Cloud.tsx` in
cloud mode or `SelfHosted.tsx` in self-hosted mode

## Purpose

Authenticate a user into LiveReview. Cloud mode uses hosted OAuth/login
flows; self-hosted mode uses email/password against the local instance
(`/admin` path also allows the email/password form in cloud mode, for
initial setup/troubleshooting).

## Who can access it

Unauthenticated users.

## Key actions

- Log in with email/password (self-hosted) or OAuth (cloud).
- On success, redirected to the originally requested URL if one was
  captured (`redirectAfterLogin`), else to the dashboard.

## Related pages

[Setup](setup.md), [OAuth Callback](oauth-callback.md), [Dashboard](../dashboard.md)
