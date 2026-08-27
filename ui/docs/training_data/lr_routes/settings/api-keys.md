# Settings → API Keys

**Route:** `/settings#api-keys`
**Who sees it:** any org member

## Purpose

Manage API keys used for programmatic/automation access to LiveReview's
API (e.g. from the `lrc` CLI or CI pipelines). An API key inherits the
exact access boundaries of the user who created it, and cannot perform
sensitive account actions (password changes, email updates, member
deactivation) — those require an active JWT session (see root `AGENTS.md`,
"API Key Scoping").

## Key actions

- Create a new API key.
- View existing keys (with creator/role context).
- Revoke or delete a key.

## Related pages

[Settings overview](settings-overview.md), [Create Review via CLI](../reviews/create-review-cli.md)
