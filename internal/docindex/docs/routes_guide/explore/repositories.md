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
- Trigger a sync of a repository's pull requests
  (`syncRepositoryPullRequests`).
- Trigger a sync of all repositories for a connector
  (`syncConnectorRepositories`).
- Navigate to a repo's [Scheduled Reviews](../reviews/scheduled-reviews.md)
  runs.

## Related pages

- [Explore → Merge Requests](merge-requests.md)
- [Git Providers](../git/git-providers.md)
- [Scheduled Reviews](../reviews/scheduled-reviews.md)
