# OAuth Callback

**Route(s):** `/oauth-callback` (`OAuthCallbackHandler`), plus inline
handling of `?code=`/`?error=` params on `/` via `HomeWithOAuthCheck` →
`CodeHostCallback`
**Components:** `ui/src/pages/Auth/OAuthCallbackHandler.tsx`,
`ui/src/pages/Auth/CodeHostCallback.tsx`

## Purpose

Completes OAuth redirect flows — both login OAuth and git-provider connector
OAuth (e.g. connecting GitLab). Reads `code`/`error`/`state` query params,
stashes them in `sessionStorage`, and routes to the appropriate handler
without leaking the raw params into the visible URL.

## Who can access it

Anyone mid-OAuth-flow (redirected here by the OAuth provider).

## Key actions

None directly user-initiated — this is a transit page that finishes the
OAuth handshake and redirects onward (to Dashboard, or back to
[Git Providers](../git/git-providers.md) connector setup on error/success).

## Related pages

[Login](login.md), [Git Providers](../git/git-providers.md)
