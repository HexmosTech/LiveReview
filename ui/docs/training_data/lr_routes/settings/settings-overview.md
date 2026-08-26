# Settings

**Route:** `/settings/*` (tab selected via URL hash, e.g. `/settings#users`)
**Component:** `ui/src/pages/Settings/Settings.tsx`

## Purpose

Central hub for org and instance configuration, organized into permission-gated
tabs. Which tabs are visible depends on the user's role
(`super_admin`/`owner`/`member`) and deployment mode (cloud vs self-hosted).

## Tabs (visibility-gated)

| Tab | id | Who sees it |
|---|---|---|
| Instance | `instance` | super_admin only |
| SMTP | `smtp` | super_admin, self-hosted only |
| Storage | `storage` | super_admin only |
| Deployment | `deployment` | super_admin only |
| License | `license` | super_admin (cloud); super_admin or owner (self-hosted) |
| Prompts | `prompts` | super_admin, or org owner/member |
| Learnings | `learnings` | any org member |
| API Keys | `api-keys` | any org member |
| MCP Integration | `mcp` | any org member |
| User Management | `users` | any org member (read-only for non-owners) |
| Integrations | `integrations` | any org member (cloud mode shows an "Enterprise only" notice instead of connect options) |
| Plan & Usage | `subscriptions` | any org member, cloud mode only |

The default tab (when no hash is set) is the first visible tab for that
user's role. Subscription sub-routes (`/settings-subscriptions-overview`,
`-breakdown`, `-assign`, `-portfolio`) render the Settings page pinned to
the `subscriptions` tab.

## Related pages

Each tab has its own file in this folder: [Instance](instance.md),
[SMTP](smtp.md), [Storage](storage.md), [Deployment](deployment.md),
[License](license.md), [Prompts](prompts.md), [Learnings](learnings.md),
[API Keys](api-keys.md), [MCP Integration](mcp-integration.md),
[User Management](user-management.md), [Integrations](integrations.md),
[Plan & Usage](plan-and-usage.md).
