Schedule automatic LiveReview backups, retention cleanup, and S3 sync with cron.

## View the cron template

`lrops.sh` extracts a ready-made cron template during install:

```
cat ~/livereview/config/backup-cron.example
```

## Add entries to your crontab

```
crontab -e
```

Example entries:

```
# Daily backup at 2 AM
0 2 * * * /usr/local/bin/lrops.sh backup daily_$(date +\%Y\%m\%d) >> /var/log/livereview-backup.log 2>&1

# Weekly backup on Sundays at 3 AM
0 3 * * 0 /usr/local/bin/lrops.sh backup weekly_$(date +\%Y_week\%U) >> /var/log/livereview-backup.log 2>&1

# Monthly backup on the 1st at 4 AM
0 4 1 * * /usr/local/bin/lrops.sh backup monthly_$(date +\%Y\%m) >> /var/log/livereview-backup.log 2>&1
```

`lrops.sh` is called by its installed absolute path (`/usr/local/bin/lrops.sh`) since cron's `PATH` won't include wherever you first ran it from. It finds your install itself via `$HOME/livereview` (cron sets `HOME` for whichever user's crontab this is) — no extra path setup needed unless your install lives somewhere else, in which case prefix each line with `LIVEREVIEW_INSTALL_DIR=/your/path`.

## Backup + S3 sync in one line

```
30 2 * * * /usr/local/bin/lrops.sh backup daily_$(date +\%Y\%m\%d) && rclone sync ~/livereview/backups/ livereview-s3:your-bucket/backups/livereview/ --log-file=/var/log/livereview-s3-sync.log
```

See [Backing Up LiveReview to S3](Backing-Up-LiveReview-to-S3) for the rclone setup this depends on.

## Retention (cleanup old backups)

`lrops.sh backup` produces directories, not single files — match on `-type d` and delete with `-exec rm -rf {} +` (`-delete` alone refuses to remove non-empty directories).

```
# Keep only last 7 daily backups (run at 5 AM)
0 5 * * * find ~/livereview/backups -maxdepth 1 -name "daily_*" -type d -mtime +7 -exec rm -rf {} +

# Keep only last 4 weekly backups
0 5 * * 1 find ~/livereview/backups -maxdepth 1 -name "weekly_*" -type d -mtime +28 -exec rm -rf {} +

# Keep only last 12 monthly backups
0 5 1 * * find ~/livereview/backups -maxdepth 1 -name "monthly_*" -type d -mtime +365 -exec rm -rf {} +
```

## Test a cron entry without waiting for its schedule

Add a temporary once-a-minute entry:

```
crontab -e
```

```
* * * * * /usr/local/bin/lrops.sh backup crontest_$(date +\%Y\%m\%d_\%H\%M) >> /var/log/livereview-backup.log 2>&1
```

Wait a couple of minutes, then check:

```
tail -f /var/log/livereview-backup.log
ls ~/livereview/backups/ | grep crontest
```

Then remove the test line (`crontab -e`) and clean up:

```
rm -rf ~/livereview/backups/crontest_*
```

## Related

- [Backup LiveReview](Backup-LiveReview)
- [Backing Up LiveReview to S3](Backing-Up-LiveReview-to-S3)
- [Restoring a LiveReview Backup](Restoring-a-LiveReview-Backup)
