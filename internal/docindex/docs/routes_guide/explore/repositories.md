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

## Related pages

- [Explore → Merge Requests](merge-requests.md)
- [Repository settings](repository-settings.md)
- [Git Providers](../git/git-providers.md)
- [Scheduled Reviews](../reviews/scheduled-reviews.md)
