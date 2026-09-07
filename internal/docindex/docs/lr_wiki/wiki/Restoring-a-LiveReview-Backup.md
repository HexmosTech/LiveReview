Restore a self-hosted LiveReview install from a backup made with `lrops.sh`.

## Restore process

```
lrops.sh stop
lrops.sh restore my-backup-name-20260906_143022-version-1.0.2
lrops.sh start
```

Or restore the most recent backup, regardless of name:

```
lrops.sh restore latest
```

## Verify a restore actually worked

Don't just trust that the command exited without error — confirm your data is really there:

```
docker compose exec livereview-db psql -U livereview -d livereview -c "SELECT * FROM your_table;"
```

## Restoring from external storage

If the backup you want only exists in S3 (or another remote location), pull it into `~/livereview/backups/` first, then restore it exactly like a local backup:

```
rclone copy livereview-s3:your-bucket/backups/livereview/my-backup-name-20260906_143022-version-1.0.2 \
  ~/livereview/backups/my-backup-name-20260906_143022-version-1.0.2 --progress

lrops.sh stop
lrops.sh restore my-backup-name-20260906_143022-version-1.0.2
lrops.sh start
```

See [Backing Up LiveReview to S3](Backing-Up-LiveReview-to-S3) for the full setup.

## Related

- [Backup LiveReview](Backup-LiveReview)
- [Backing Up LiveReview to S3](Backing-Up-LiveReview-to-S3)
