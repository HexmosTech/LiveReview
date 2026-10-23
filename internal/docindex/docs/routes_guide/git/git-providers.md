# Git Providers

**Route(s):** `/git` (list, `GitProvidersList`), `/git/connector/:connectorId`
(details), `/git/github-app` (GitHub App card, `GitHubAppConnector`), `/git/*` fallback (add/edit form, `ConnectorForm`)
**Component:** `ui/src/pages/GitProviders/GitProviders.tsx`,
`ui/src/pages/GitProviders/ConnectorDetails.tsx`

## Purpose

Manage git provider connections ("connectors") — GitHub, GitLab, Bitbucket,
Gitea, Azure DevOps — that LiveReview uses to receive webhooks and post
review comments. The list page shows all configured connectors with
webhook health status; the details page drills into one connector's
repository access and manual-trigger settings; the fallback route renders
the add/edit connector form (provider selection → credentials/OAuth →
save).

## Who can access it

Any authenticated org member can view. On LiveReview Cloud every plan,
including Free, can add, edit and remove connectors (and their webhooks, so
pull requests get reviewed automatically); on self-hosted plans this may be
gated by license (`LicenseUpgradeDialog`).

## Key actions

- Add a new git provider connector (OAuth or token-based, per provider).
- Edit or delete an existing connector.
- View a connector's accessible repositories/projects and per-project
  status.
- Enable/disable manual review triggering for all projects under a
  connector (`enableManualTriggerForAllProjects` /
  `disableManualTriggerForAllProjects`).

## GitHub App connections

Organizations that signed in with **Continue with GitHub** have a connector
per GitHub App installation. These are labelled "GitHub App", their
repositories come from the installation, and webhooks are delivered by the
app, so the manual-trigger buttons are replaced by a note. Repositories are
added or removed on GitHub, not here.

Several LiveReview organizations can use the same installation (for example two
people from one GitHub organization who each signed up). Each has its own
connection. When a user signs in with GitHub and the app is already installed on
an account by another LiveReview organization, it isn't connected to theirs
automatically: it's listed as **Available** and an owner clicks **Connect** to
opt in. (Installing or configuring the app from LiveReview connects it
straight away.) An organization that joined an installation someone else
installed only gets the repositories its user's GitHub account can access. Each account
on the card shows "Also used by N other LiveReview accounts" when others share
it; clicking that lists them by their owners' masked emails (`at•••@gmail.com`). Pull request
updates reach every organization that has the repository; a reply to a comment
on GitHub comes from the organization that reviewed that pull request most
recently.

### The GitHub App card

On LiveReview Cloud the **Select Git Provider** grid starts with a **GitHub App**
card (marked Recommended). It opens `/git/github-app`, which is titled **GitHub App** and lists every
connected GitHub account or organization as a panel (name, repository count, repositories as chips; a toggle at the top right of the repositories switches to a searchable list that shows five rows and scrolls),
a **Manage on GitHub** button (opens that account's installation settings;
GitHub shows it only to owners), and, for organization owners, **Disconnect**
(stops using it in this organization only; the app stays installed on GitHub
for anyone else using it) and **Uninstall** (uninstalls the app from that GitHub
account, which removes it for every LiveReview account using it; the
confirmation says how many other accounts are affected). Below the connected
accounts, each account that isn't connected is one line with no repositories:
**Available** (installed by another LiveReview account; "Already installed by N
other LiveReview accounts" opens the masked list) or **Disconnected**
(disconnected here), plus a **Connect** button for organization owners.
Connect needs no trip to GitHub; the repositories sync in the background.
Once an account is connected, a **Next steps** section below the accounts shows
the same two steps as get-started: the AI provider (LiveReview AI already
connected, or **Configure AI**) and **Review a pull request** (**See open
MRs/PRs and trigger a review** or **Trigger a review via URL**).
**Add repositories or organization** opens
GitHub's install page; afterwards the user returns to `/git/github-app` and the
new account appears. With nothing connected or available yet the page shows **Install the
GitHub App**. Returning GitHub sign-ins land directly on this page.

## Related pages

- [Explore → Repositories](../explore/repositories.md)
- [Settings → Integrations](../settings/integrations.md)
