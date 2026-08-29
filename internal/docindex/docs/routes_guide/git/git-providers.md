# Git Providers

**Route(s):** `/git` (list, `GitProvidersList`), `/git/connector/:connectorId`
(details), `/git/*` fallback (add/edit form, `ConnectorForm`)
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

Any authenticated org member can view; adding/editing/removing connectors
may be gated by license (`LicenseUpgradeDialog`) on self-hosted plans.

## Key actions

- Add a new git provider connector (OAuth or token-based, per provider).
- Edit or delete an existing connector.
- View a connector's accessible repositories/projects and per-project
  status.
- Enable/disable manual review triggering for all projects under a
  connector (`enableManualTriggerForAllProjects` /
  `disableManualTriggerForAllProjects`).

## Related pages

- [Explore → Repositories](../explore/repositories.md)
- [Settings → Integrations](../settings/integrations.md)
