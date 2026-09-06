LiveReview data and configuration can be backed up easily with `lrops.sh`.

Here's a quick reference to the options available:

```
lrops.sh --help | grep backup
    lrops.sh backup [--backup-dir <path>] [name]  # Create manual backup (see detailed options below)
    lrops.sh quick-backup              # Create quick timestamped backup
    lrops.sh list-backups              # List all available backups
    lrops.sh backup-info <name>        # Show detailed information about a backup
    lrops.sh delete-backup <name>      # Delete a specific backup
    lrops.sh restore <id|latest>       # Restore a previous backup
    (Use --backup-dir as backup subcommand option - see BACKUP OPTIONS below)
    lrops.sh uninstall                 # Safely uninstall (moves directory, keeps backups)
    lrops.sh help backup               # Backup strategies
    1. Default backup (to installation directory):
       lrops.sh backup                     # Auto-named: manual-YYYYMMDD_HHMMSS
    2. Named backup (to installation directory):
       lrops.sh backup my-backup-name      # Custom name: my-backup-name-YYYYMMDD_HHMMSS
       lrops.sh backup --backup-dir /path/to/backups
       lrops.sh backup --backup-dir /path/to/backups custom-name
    4. Quick timestamped backup:
       lrops.sh quick-backup               # Creates: quickbackup-YYYYMMDD_HHMMSS
    lrops.sh backup                                    # Default backup
    lrops.sh backup before-upgrade                     # Named backup
    lrops.sh backup --backup-dir ~/my-backups          # Custom directory
    lrops.sh backup --backup-dir ~/my-backups my-name  # Custom directory + name
```

