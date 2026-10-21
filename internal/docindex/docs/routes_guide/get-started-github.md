# Get started with GitHub

**Route(s):** `/get-started/github`
**Component:** `ui/src/pages/GetStarted/GetStartedGitHub.tsx`

## Purpose

A guided first run for organizations connected through the LiveReview GitHub
App: confirm which repositories are connected, make sure an AI provider is
set up, and run the first review on a real pull request.

If the organization has no GitHub App connection yet, or the app was given no
repositories, the user is sent to GitHub (a spinner shows briefly): into the
install flow when the app isn't installed (the user stays signed in and comes back in
through the GitHub flow), or to the installation's settings page on GitHub when
it is installed but has no repositories. This happens only once per browser
session. If the user comes back (Back button) without
finishing, the page shows a small card with a **Continue with GitHub** button
(or **Add repositories on GitHub**) instead of redirecting again.

## Who can access it

Any authenticated org member. It appears in the mega menu (Reviews) once the
organization has a GitHub App connection.

## Key actions

- See the connected repositories grouped by GitHub account or organization:
  each account is a panel with its name, a repository count and its repositories as chips (a toggle switches to a
  searchable list that shows five rows and scrolls), and a
  **Manage on GitHub** button that opens that account's installation settings
  (to add or remove repositories). GitHub shows that page only to owners of the
  account or organization.
- **Add repositories or organization** — below the connected accounts:
  opens GitHub's install page so the app can be added to
  another account or organization (it lists the ones you can install on); after
  installing, the user returns here and the new account appears in the list.
- **Disconnect** (organization owners only) — next to each account: uninstalls
  the LiveReview GitHub App from that GitHub account or organization (using the
  app's own credentials, so the user does not need to be a GitHub owner), stops
  LiveReview using it for the organization, and drops its repositories. To use
  it again, install the app on GitHub again.
- **Configure AI** — opens AI Providers in a new tab when no provider is
  configured. A link to Google AI Studio explains where to get a free Gemini
  API key.
- **See open MRs/PRs and trigger a review** — goes to
  [Merge Requests](explore/merge-requests.md), where open pull requests are
  listed and a review can be triggered.
- **Trigger a review via URL** — goes to the [New review](reviews/new-review.md) page,
  where a pull request URL can be pasted.
- **Next: catch issues before you commit with the CLI** — continues to the CLI
  setup page.

## Related pages

- [Continue with GitHub (sign-in landing page)](auth/gh-login.md)
- [Git Providers](git/git-providers.md)
- [Reviews](reviews/)
