Sync LiveReview backups to S3 with `rclone`, and restore from them.

## 1. Create a backup

```
lrops.sh backup pretest
```

See [Backup LiveReview](Backup-LiveReview) for the full command reference.

## 2. Set up rclone + S3 (one-time)

```
curl https://rclone.org/install.sh | sudo bash
```

Create the remote — key and value as **separate arguments**, not `key=value` (that gets misparsed and silently corrupts the saved config):

```
rclone config create livereview-s3 s3 \
  provider AWS \
  access_key_id YOUR_ACCESS_KEY \
  secret_access_key YOUR_SECRET_KEY \
  region ap-south-1
```

Verify before trusting it:

```
rclone config show livereview-s3
rclone lsd livereview-s3:
```

## 3. Move a backup to S3

```
rclone sync ~/livereview/backups/ livereview-s3:your-bucket/backups/livereview/ --progress
```

Confirm it landed:

```
rclone ls livereview-s3:your-bucket/backups/livereview/
```

## 4. Restore a backup that only exists in S3

`lrops.sh restore` reads from the local `backups/` directory, so pull the backup back down first, then restore normally — see [Restoring a LiveReview Backup](Restoring-a-LiveReview-Backup) for the full restore process:

```
rclone copy livereview-s3:your-bucket/backups/livereview/pretest-20260905_143910-version-1.0.1 \
  ~/livereview/backups/pretest-20260905_143910-version-1.0.1 --progress

lrops.sh stop
lrops.sh restore pretest-20260905_143910-version-1.0.1
lrops.sh start
```

## Related

- [Backup LiveReview](Backup-LiveReview)
- [Restoring a LiveReview Backup](Restoring-a-LiveReview-Backup)
- [Automating Backups with Cron](Automating-Backups-with-Cron)
