# Settings → Preloaded Changes Archival

**Route:** `/settings#preloaded-changes-archival` (also accessible via `/settings#storage`)
**Who sees it:** super_admin or org owner (non-cloud)

## Purpose

Configure automated background offloading of historical code diffs (`preloaded_changes`) from PostgreSQL metadata into external Blob Storage to prevent database bloat. Backed by `system_settings` (`preloaded_changes_archival_settings`).

## Key actions

- Toggle automatic background archival on/off.
- Configure retention period (e.g. 30 days).
- Set daily execution schedule using cron expression.
- Trigger manual archival cycle on-demand.

## Related pages

- [Storage settings](storage.md)
- [Settings overview](settings-overview.md)
