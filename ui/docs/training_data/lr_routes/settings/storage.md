# Settings → Storage

**Route:** `/settings#storage`
**Who sees it:** super_admin only

## Purpose

Configure the blob storage backend LiveReview uses to persist review
artifacts (diffs, blast-radius data, etc. synced from `lrc`). Backed by a
`system_settings` row (`blob_storage`) read fresh on every artifact request —
switching backends or rotating credentials needs no redeploy. See
`internal/api/storage_settings.go`.

## Key actions

- Choose and configure the storage backend: local filesystem, S3-compatible
  (AWS S3, Backblaze B2), Google Cloud Storage, or Azure Blob Storage.
- Enter/rotate credentials for the selected backend.

## Related pages

[Settings overview](settings-overview.md)
