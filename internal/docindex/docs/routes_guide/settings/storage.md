# Settings → Storage

**Route:** `/settings#storage`
**Who sees it:** super_admin, or an org **owner** on a self-hosted instance
(`canManageInstanceConfig` in `ui/src/pages/Settings/Settings.tsx`). In cloud
mode the blob store is shared across every tenant, so it stays
super_admin-only there.

## Purpose

Configure the blob storage backend LiveReview uses to persist review
artifacts (diffs, blast-radius data, etc. synced from `lrc`). Backed by a
`system_settings` row (`blob_storage`) read fresh on every artifact request —
switching backends or rotating credentials needs no redeploy. See
`internal/api/storage_settings.go`.

## Layout

The page has five sub-tabs, each with its own Save button. The selected one
is kept in the URL (`/settings?section=<id>#storage`), so links open it directly:

- **Blob Storage** (`section=blob`, the default)
- **Log Compaction** (`section=log-compaction`)
- **Preloaded Changes Archival** (`section=archival`)
- **Repo Cache** (`section=repo-cache`)
- **AI Review Performance** (`section=ai-review-performance`)

## Key actions

- Choose and configure the storage backend: local filesystem, S3-compatible
  (AWS S3, Backblaze B2), Google Cloud Storage, or Azure Blob Storage.
- Enter/rotate credentials for the selected backend.
- **Repo Cache** sub-tab: turn server-side blast radius on/off
  (on by default) and set the cache size (default 5 GB, minimum 1 GB). The
  server keeps a shallow clone + code graph of each reviewed repo under
  `lrdata/repo_cache/` and removes the least-recently-used repos when the
  cache is full. Shows how much is used and how many repos are cached. Raise
  the size when a review says its repo is too large for the cache. **Clear
  cache** deletes every cached repo on the server (all orgs, after a
  confirmation); repos a review is using right now are skipped. To clear one
  repo, use its [repository settings](../explore/repository-settings.md).
  Backed by
  the `system_settings` row `blast_radius_cache`
  (`/api/v1/admin/settings/blast-radius-cache`).
- **AI Review Performance** sub-tab: set instance-wide safety bounds for AI
  review concurrency and timeout - the max concurrent batches ceiling
  (default 16) and the min/max review timeout in seconds (default 60s floor,
  240s ceiling). These bound, but don't replace, the self-tuning
  per-AI-connector concurrency limiter (`internal/llm/adaptivelimiter`) that
  discovers the actual safe concurrency for your provider at runtime, and
  they also bound any organization's own optional preference set on the
  [AI Providers](../ai/ai-providers.md) page (an org can turn its own
  concurrency/timeout down, never above what the instance allows). Backed by
  the `system_settings` row `ai_review_performance`
  (`/api/v1/admin/settings/ai-review-performance`).

## Related pages

[Settings overview](settings-overview.md)
