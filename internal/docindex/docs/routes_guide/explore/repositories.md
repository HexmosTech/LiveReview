# Explore → Repositories

**Route:** `/explore/repositories`
**Component:** `ui/src/pages/Explore/Repositories.tsx`

## Purpose

Table of every repository connected to the org across all git connectors
(GitHub, GitLab, Bitbucket, Gitea, Azure DevOps), with sync status.

## Who can access it

Any authenticated org member.

## Key actions

- Search/filter/sort repositories by provider, name, etc.
- Click a row to see that repository's [pull requests](merge-requests.md). Rows
  are real links, so right-click gives "Open in new tab" and middle/Ctrl-click
  work anywhere in the row; the repo name still links to the provider, and the
  Actions buttons work as before.
- Trigger a sync of a repository's pull requests
  (`syncRepositoryPullRequests`).
- Trigger a sync of all repositories for a connector
  (`syncConnectorRepositories`).
- Navigate to a repo's [Scheduled Reviews](../reviews/scheduled-reviews.md)
  runs.
- Open a repo's [settings](repository-settings.md) (gear icon in the Actions
  column), e.g. to see or clear its repo cache.

## Connector error alert banners

When one or more repositories have sync errors (e.g., authentication failures
or provider mismatches), **a single orange alert banner is shown per affected
connector** — not one per repository. This prevents notification spam when an
entire connector's credentials have expired.

Each banner shows:
- Title stating: *"Your <Provider> provider credential expired, please renew it."* (e.g. GitHub, GitLab, Bitbucket)
- Body stating the connector name and how many repositories could not sync.
- An internal action link (*"Renew <Provider> credentials"*) taking the user directly to `#/git/connector/:id` to update the token.
- An external link to the LiveReview Wiki guide for that Git provider (`GitHub`, `GitLab`, `Self-Hosted GitLab`, `Bitbucket`).

The same notification is also dispatched to the global notification bell tray and toast system with the matching action buttons. Banners are dismissible per-connector for the current session.

## Related pages

- [Explore → Merge Requests](merge-requests.md)
- [Repository settings](repository-settings.md)
- [Git Providers](../git/git-providers.md)
- [Scheduled Reviews](../reviews/scheduled-reviews.md)
