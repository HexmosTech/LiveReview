Backups of a self-hosted LiveReview install, created and managed with `lrops.sh`.

## What's in a backup

Each backup is a plain directory containing:

| File | Contents |
|---|---|
| `db.sql.gz` | Compressed logical database dump (`pg_dump`) |
| `postgres-data.tgz` | Compressed physical snapshot of the Postgres data directory |
| `app-data.tgz` | Compressed snapshot of application data outside Postgres (e.g. the local blob store) |
| `.env`, `docker-compose.yml` | Configuration files |
| `config/`, `scripts/` | Copies of the install's config and scripts directories |
| `docker-ps.txt`, `docker-images.txt` | Container/image metadata, for reference |
| `metadata.json` | Backup name, timestamp, app version, and which of the above actually succeeded |

## Create a backup

```
lrops.sh backup my-backup-name
```

Omit the name for an auto-generated one:

```
lrops.sh backup
# creates: manual-20260906_143022-version-1.0.2
```

Or use `quick-backup` for a one-command timestamped backup:

```
lrops.sh quick-backup
# creates: quickbackup-20260906_143022-version-1.0.2
```

`lrops.sh` also takes an automatic backup (prefixed `preupdate-`) before every `lrops.sh update`, with the same content as above.

## List and inspect backups

```
lrops.sh list-backups
```

```
lrops.sh backup-info my-backup-name-20260906_143022-version-1.0.2
```

## Back up to a custom or external location

```
lrops.sh backup --backup-dir /mnt/external-drive my-backup-name
lrops.sh backup --backup-dir ~/my-backups weekly-backup
```

Useful for network drives, USB drives, or a directory you separately sync elsewhere (see [Backing Up LiveReview to S3](Backing-Up-LiveReview-to-S3)).

## Delete a backup

```
lrops.sh delete-backup my-backup-name-20260906_143022-version-1.0.2
```

## Automate it

See [Automating Backups with Cron](Automating-Backups-with-Cron) to run this on a schedule.

## Related

- [Restoring a LiveReview Backup](Restoring-a-LiveReview-Backup)
- [Backing Up LiveReview to S3](Backing-Up-LiveReview-to-S3)
- [Automating Backups with Cron](Automating-Backups-with-Cron)
