# Continue with GitHub (sign-in landing page)

**Route(s):** `/gh-login` (a shareable path that opens `#/gh-login`; LiveReview Cloud only)
**Component:** `ui/src/pages/Auth/GhLogin.tsx`

## Purpose

The quickest way for a new user to try LiveReview. One button, **Continue
with GitHub**, takes the user to a single GitHub screen where they install the
LiveReview GitHub App on the repositories they choose and authorize sign-in at
the same time. No personal access token, no webhook setup. Afterwards they are
signed in to LiveReview (the account is created if it does not exist) and
land on the [Get started with GitHub](../get-started-github.md) page.

Under the headline the page lists three short points, each with a green
check: risk scoring decides which PRs are safe to ship without a human; the
team only spends review time on risky and critical changes; and GitHub can be
connected and shown on a real pull request in about 2 minutes.

The page only appears when the LiveReview server has the GitHub App
configured. If it is not configured, or this is not LiveReview Cloud,
`/gh-login` simply shows the normal login screen.

## Who can access it

Anyone who is not signed in. A signed-in user who opens `/gh-login` is sent on
to [Get started with GitHub](../get-started-github.md).

## Key actions

- **Continue with GitHub** — sign in with GitHub. If the app is already
  installed the user lands on [Get started with GitHub](../get-started-github.md)
  with their repositories (first time) or on the GitHub App card of Git Providers (returning, below). If not, their account is created and they go
  straight to GitHub's install page to choose repositories, then land on that
  page.
- **Returning users:** a plain GitHub sign-in by someone whose organization and
  GitHub connection already exist lands on the GitHub App card of [Git Providers](../git/git-providers.md)
  (`/git/github-app`), where their connected accounts are managed, instead of the first-run page.
- **The app is already installed by someone else** (for example a teammate
  installed it on the GitHub organization from their own LiveReview account):
  the new user's own LiveReview organization is connected to the same
  installation, with no invite needed. It gets only the repositories that the
  user's GitHub account can access; GitHub reports these at sign-in and the
  list is refreshed on every GitHub sign-in. Reviews, the pull request list and
  bot replies then work in their organization with their own AI provider. The
  get-started page and the GitHub App card say the app was already installed.
- **A failed GitHub sign-in leaves nothing behind:** the landing page is decided
  only when the sign-in succeeds, so choosing **Other ways to sign in** after an
  error lands on the dashboard as usual, not on get-started.
- **Or sign in with** — a row of Google, LinkedIn and Email tiles under the
  GitHub button. Each goes straight to the Hexmos sign-in page
  (`hexmos.com/signin?app=livereview`), which offers all of these methods, and
  returns to LiveReview signed in, like the normal login.
- **`?src=launch-trial`** (the Product Hunt launch link): when this sign-in creates
  a new organization, that organization can review on the LiveReview AI Model on
  the Free plan, with no AI key to add, within the Free plan's monthly LOC limit.
  Any other `src`, or an existing account, gets the normal flow.

## Related pages

- [Get started with GitHub](../get-started-github.md)
- [Git Providers](../git/git-providers.md)
