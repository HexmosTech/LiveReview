# LiveReview Update Guide

This guide explains how the `lrops.sh update` command works internally and provides step-by-step instructions for updating LiveReview installations.

## Overview

The `lrops.sh update` command provides a comprehensive, safe update mechanism that includes automatic backups, version validation, and rollback capabilities. It supports both specific version updates and automatic latest version detection.

## Update Process Flow

The update process follows these main phases:

### Phase 1: Pre-Update Validation and Setup

1. **Installation Detection**
   - Verifies that LiveReview is installed by checking for `$LIVEREVIEW_INSTALL_DIR`
   - Confirms `docker-compose.yml` exists in the installation directory
   - If either check fails, the update is aborted with an error message

2. **Version Resolution**
   - If a specific version is requested: validates it's a semantic version (e.g., `v1.4.2`, `2.1.0`)
   - If no version specified: queries GitHub Container Registry (GHCR) to find the latest semantic version
   - Uses GHCR API to get available tags from `ghcr.io/hexmostech/livereview`
   - Filters for semantic versions only (ignores tags like `latest`, `dev`, etc.)
   - Returns highest semantic version found

3. **Docker Environment Setup**
   - Detects available Docker Compose command (`docker-compose` vs `docker compose`)
   - Handles sudo requirements for Docker if needed
   - Sets up proper command wrappers for subsequent operations

### Phase 2: Backup Creation

The system creates a comprehensive pre-update backup to enable safe rollbacks:

1. **Backup Directory Creation**
   ```
   $LIVEREVIEW_INSTALL_DIR/backups/preupdate-YYYYMMDD_HHMMSS-from-[current_version]-to-[target_version]/
   ```

2. **Configuration Backup**
   - Copies `.env` file (environment variables)
   - Copies `docker-compose.yml` file (container configuration)
   - Creates timestamped `.env.bak` file for quick reference

3. **Database Backup (Dual Strategy)**
   
   **Physical Snapshot (Recommended)**
   - Creates compressed tar archive of `lrdata/postgres` directory
   - Attempts without sudo first, falls back to sudo if permissions require it
   - Results in `postgres-data.tgz` file for fast restoration
   
   **Logical Dump (Compatibility)**
   - Executes `pg_dump` inside the running database container
   - Uses credentials from `.env` file (`DB_PASSWORD`)
   - Creates `db.sql` text dump for cross-version compatibility

4. **Docker Metadata Capture**
   - Saves current container states (`docker-ps.txt`)
   - Records current image information (`docker-images.txt`)
   - Creates `metadata.json` with backup details and flags

5. **Backup Retention Management**
   - Keeps only the most recent 10 pre-update backups (configurable via `BACKUP_RETENTION_COUNT`)
   - Automatically prunes older backups to manage disk space

### Phase 3: Version Update Process

1. **Environment Variable Update**
   - Updates or adds `LIVEREVIEW_VERSION=` line in `.env` file
   - Strips 'v' prefix from version for consistency (e.g., `v1.4.2` → `1.4.2`)

2. **Docker Image Management**
   - Pulls new image: `ghcr.io/hexmostech/livereview:{version}`
   - Validates successful pull before proceeding
   - Keeps old image available for potential rollback

3. **Docker Compose Configuration Update**
   - Updates `image:` line in `docker-compose.yml`
   - Uses `awk` to preserve file formatting and indentation
   - Validates the update was successful by re-reading the file

### Phase 4: Container Recreation

1. **Selective Container Recreation**
   - Uses `docker compose up -d --no-deps --force-recreate livereview-app`
   - Only recreates the main application container (preserves database container)
   - `--no-deps` prevents unnecessary recreation of dependent services
   - `--force-recreate` ensures the new image is used even if configuration appears unchanged

2. **Health Check Monitoring**
   - Monitors container health status for up to 120 seconds
   - Checks Docker health check endpoint every 5 seconds
   - Reports progress during waiting period
   - Fails update if container doesn't become healthy within timeout

### Phase 5: Post-Update Validation

1. **Status Verification**
   - Runs `show_status` function to display updated container information
   - Confirms all services are running correctly
   - Shows version information and access URLs

2. **User Feedback**
   - Reports successful update completion
   - Provides rollback instructions if issues are detected
   - Suggests diagnostic commands if health checks fail

## Command Usage Examples

### Update to Latest Version
```bash
lrops.sh update
```
This queries GHCR for the latest semantic version and updates to it.

### Update to Specific Version
```bash
lrops.sh update v1.4.2
lrops.sh update 2.1.0
```
Updates to the specified semantic version (with or without 'v' prefix).

### Check Available Versions
```bash
lrops.sh list-versions
```
Shows all available semantic versions from the container registry.

### Check Latest Version
```bash
lrops.sh latest-version
```
Displays the latest available semantic version without updating.

## How Rollback Works

The LiveReview rollback system provides comprehensive restoration capabilities using the backups created before each update. The rollback process is designed to be safe, reliable, and handle both configuration and data restoration.

### Rollback Architecture

The rollback system operates on timestamped backup directories created during updates:
```
$LIVEREVIEW_INSTALL_DIR/backups/
├── preupdate-20250911_143022-from-1.3.1-to-1.4.2/
│   ├── .env                    # Configuration backup
│   ├── docker-compose.yml     # Container configuration
│   ├── postgres-data.tgz      # Physical database snapshot
│   ├── db.sql                 # Logical database dump
│   ├── metadata.json          # Backup information
│   ├── docker-ps.txt          # Container state snapshot
│   └── docker-images.txt      # Image information
└── preupdate-20250910_092014-from-1.2.9-to-1.3.1/
    └── ...
```

### Rollback Process Flow

#### Phase 1: Backup Selection and Validation

1. **Backup Identification**
   ```bash
   lrops.sh restore latest              # Most recent backup
   lrops.sh restore preupdate-20250911  # Specific backup by ID
   ```

2. **Backup Discovery**
   - Scans `$LIVEREVIEW_INSTALL_DIR/backups/` directory
   - For "latest": sorts backups by timestamp, selects most recent
   - For specific ID: looks for exact directory match
   - Validates backup directory exists and contains required files

3. **Backup Integrity Check**
   - Verifies backup directory structure
   - Checks for presence of configuration files (`.env`, `docker-compose.yml`)
   - Identifies available restore methods (physical vs logical database)
   - Reads `metadata.json` for backup information

#### Phase 2: Container Management

1. **Application Container Shutdown**
   ```bash
   docker compose stop livereview-app
   ```
   - Stops only the application container initially
   - Keeps database container running for logical restores
   - Preserves active connections and state where possible

2. **Database Container Management**
   - For **Physical Restore**: Stops database container completely
   - For **Logical Restore**: Keeps database container running
   - Manages container lifecycle based on restore method

#### Phase 3: Configuration Restoration

1. **Configuration File Restoration**
   - Restores `.env` file with previous environment variables
   - Restores `docker-compose.yml` with previous container configuration
   - Overwrites current configuration with backup versions
   - Preserves file permissions and ownership

2. **Version Rollback**
   - `LIVEREVIEW_VERSION` in `.env` reverts to backup version
   - Docker image tags in `docker-compose.yml` revert to previous values
   - Ensures version consistency across all configuration files

#### Phase 4: Database Restoration (Dual Strategy)

The system attempts database restoration using the best available method:

**Strategy 1: Physical Snapshot Restoration (Preferred)**

When `postgres-data.tgz` is available:

1. **Database Container Shutdown**
   ```bash
   docker compose stop livereview-db
   ```

2. **Existing Data Preservation**
   - Moves current `lrdata/postgres` to `lrdata/postgres.pre-restore.TIMESTAMP`
   - Creates safety backup of current state before restore
   - Handles permission issues with sudo escalation if needed

3. **Physical Data Extraction**
   ```bash
   tar -xzf postgres-data.tgz -C $LIVEREVIEW_INSTALL_DIR/lrdata
   ```
   - Extracts compressed database directory
   - Preserves exact database state from backup time
   - Handles permission issues transparently

4. **Database Container Restart**
   ```bash
   docker compose up -d livereview-db
   ```
   - Starts database with restored data
   - PostgreSQL automatically recognizes restored data files

**Strategy 2: Logical Dump Restoration (Fallback)**

When only `db.sql` is available:

1. **Database Container Preparation**
   - Ensures database container is running
   - Starts container if not already running
   - Waits for database readiness

2. **Logical Data Import**
   ```bash
   cat db.sql | docker exec -i -e PGPASSWORD="$db_pass" $db_container psql -U livereview -d livereview
   ```
   - Streams SQL dump into running database
   - Uses credentials from restored `.env` file
   - Replaces current database content with backup data

#### Phase 5: Application Container Recreation

1. **Application Restart with Force Recreation**
   ```bash
   docker compose up -d --force-recreate livereview-app
   ```
   - Forces recreation of application container
   - Uses restored configuration and image versions
   - Connects to restored database

2. **Service Health Verification**
   - Monitors container startup process
   - Checks for successful application initialization
   - Verifies database connectivity

### Rollback Command Reference

#### List Available Backups
```bash
lrops.sh list-backups
```
Output format:
```
AVAILABLE BACKUPS
- preupdate-20250911_143022-from-1.3.1-to-1.4.2
- preupdate-20250910_092014-from-1.2.9-to-1.3.1
- preupdate-20250909_154511-from-1.2.8-to-1.2.9
```

#### Restore Latest Backup
```bash
lrops.sh restore latest
```
Automatically selects and restores the most recent backup.

#### Restore Specific Backup
```bash
lrops.sh restore preupdate-20250911_143022-from-1.3.1-to-1.4.2
```
Restores from a specific backup identified by its directory name.

#### Verify Restoration
```bash
lrops.sh status
```
Shows current system state after restoration.

### Rollback Safety Features

#### Data Protection
- **Non-Destructive**: Current data is moved, not deleted
- **Incremental Safety**: Creates restore point before rollback
- **Permission Handling**: Transparent sudo escalation for file operations
- **Container Isolation**: Database and application containers handled independently

#### Error Recovery
- **Graceful Degradation**: Falls back to logical restore if physical fails
- **Partial Restoration**: Can restore configuration even if database restore fails
- **State Validation**: Verifies backup integrity before starting restoration
- **Process Isolation**: Each restore phase is independent and recoverable

#### Backup Integrity
- **Metadata Validation**: Checks backup completeness before restoration
- **File Verification**: Ensures required backup files exist
- **Version Tracking**: Records source and target versions in metadata
- **Timestamp Preservation**: Maintains creation time for backup identification

### Rollback Scenarios and Solutions

#### Scenario 1: Failed Update (Automatic Rollback)
```bash
# Update fails during process
lrops.sh update v1.4.2
# Error: Container health check timeout

# Immediate rollback to working state
lrops.sh restore latest
```

#### Scenario 2: Post-Update Issues Discovery
```bash
# Update completed but application has issues
lrops.sh status  # Shows containers running but application problematic

# Rollback to last known good state
lrops.sh restore latest
lrops.sh status  # Verify restoration success
```

#### Scenario 3: Partial Update Corruption
```bash
# Update partially completed, mixed state
lrops.sh restore latest              # Full restoration
lrops.sh restart                     # Restart with restored configuration
```

#### Scenario 4: Rollback to Specific Version
```bash
# Need to rollback to specific earlier version
lrops.sh list-backups               # Find desired backup
lrops.sh restore preupdate-20250909_154511-from-1.2.8-to-1.2.9
```

### Advanced Rollback Operations

#### Manual Backup Analysis
```bash
# Examine backup contents
ls -la $LIVEREVIEW_INSTALL_DIR/backups/preupdate-*/
cat $LIVEREVIEW_INSTALL_DIR/backups/preupdate-*/metadata.json

# Check backup database size
ls -lh $LIVEREVIEW_INSTALL_DIR/backups/preupdate-*/postgres-data.tgz
```

#### Selective Restoration
For advanced users who need to restore only specific components:

```bash
# Restore only configuration (manual process)
cp $LIVEREVIEW_INSTALL_DIR/backups/preupdate-latest/.env $LIVEREVIEW_INSTALL_DIR/
cp $LIVEREVIEW_INSTALL_DIR/backups/preupdate-latest/docker-compose.yml $LIVEREVIEW_INSTALL_DIR/
lrops.sh restart

# Restore only database (manual process)
# Note: This requires careful container management
```

#### Backup Cleanup and Management
```bash
# Manual backup removal (if needed)
rm -rf $LIVEREVIEW_INSTALL_DIR/backups/preupdate-old-backup-name

# Backup space analysis
du -sh $LIVEREVIEW_INSTALL_DIR/backups/*
```

### Rollback Best Practices

#### Before Rollback
1. **Document the Issue**: Note what problems prompted the rollback
2. **Check Current State**: Run `lrops.sh status` to understand current system state
3. **Identify Target Backup**: Use `lrops.sh list-backups` to find appropriate restore point
4. **Verify Backup Integrity**: Ensure target backup directory is complete

#### During Rollback
1. **Monitor Process**: Watch for error messages during restoration
2. **Don't Interrupt**: Allow restoration process to complete fully
3. **Check Permissions**: Be prepared for sudo prompts if file permissions require it
4. **Verify Each Phase**: Confirm configuration and database restoration succeed

#### After Rollback
1. **Verify Functionality**: Test application through web interface
2. **Check Logs**: Review container logs for any issues
3. **Document Resolution**: Record what was rolled back and why
4. **Plan Forward**: Determine next steps (stay on rolled-back version, retry update, etc.)

### Rollback Troubleshooting

#### "Backup not found" Error
```bash
# Check available backups
lrops.sh list-backups

# Verify backup directory exists
ls -la $LIVEREVIEW_INSTALL_DIR/backups/
```

#### "Permission denied" During Restore
```bash
# Ensure proper permissions
sudo chown -R $USER:$USER $LIVEREVIEW_INSTALL_DIR

# Or run restore with appropriate privileges
sudo lrops.sh restore latest
```

#### Database Restore Failures
```bash
# Check database container status
docker ps | grep postgres

# Check database logs
lrops.sh logs livereview-db

# Verify database credentials
grep DB_PASSWORD $LIVEREVIEW_INSTALL_DIR/.env
```

#### Incomplete Restoration
```bash
# Check what was restored
lrops.sh status

# Manually restart containers
lrops.sh restart

# Check for configuration issues
docker compose config
```

The rollback system provides enterprise-grade reliability with multiple safety layers, ensuring that LiveReview installations can be safely restored to previous working states when needed.

## Error Handling and Safety

### Automatic Rollback Triggers
- Failed image pull
- Failed container health checks
- Docker Compose configuration errors
- Version validation failures

### Manual Rollback
If an update fails or causes issues:
```bash
lrops.sh restore latest
```
This restores from the most recent backup created during update.

### Backup Verification
Check available backups:
```bash
lrops.sh list-backups
```

### Diagnostic Commands
If update fails, use these for troubleshooting:
```bash
lrops.sh status           # Check current system state
lrops.sh logs             # View container logs
docker ps                 # Check container status directly
docker images             # Verify image availability
```

## Configuration Files Updated

During the update process, these files are modified:

1. **`.env`** - Version variable updated
2. **`docker-compose.yml`** - Image tag updated
3. **Backup files** - Created in `backups/` directory

## Recovery Scenarios

### Failed Update Recovery
1. If container fails to start: `lrops.sh restore latest`
2. If partial update: Check `lrops.sh status` and manually restart
3. If configuration corruption: Restore from `.env.bak.TIMESTAMP` file

### Version Mismatch Issues
1. Check actual running version: `docker ps` and inspect image tags
2. Verify `.env` file has correct `LIVEREVIEW_VERSION`
3. Ensure `docker-compose.yml` has matching image tag
4. Use `lrops.sh restart` to apply configuration changes

## Best Practices

### Before Updating
1. Verify current system is stable: `lrops.sh status`
2. Check disk space for backups: `df -h $LIVEREVIEW_INSTALL_DIR`
3. Note current version for rollback reference
4. Ensure no critical operations are running

### During Update
1. Monitor the process for any error messages
2. Don't interrupt the process during backup or container recreation
3. Allow sufficient time for health checks (up to 2 minutes)

### After Update
1. Verify application functionality through web interface
2. Check logs for any warnings: `lrops.sh logs`
3. Test critical workflows (sign-in, repository connections, etc.)
4. Document the update in your maintenance log

### Maintenance Schedule
1. Check for updates monthly: `lrops.sh latest-version`
2. Plan updates during maintenance windows
3. Test updates in staging environment if possible
4. Keep at least 3-5 backups for safety

## Troubleshooting Common Issues

### "Version not found" Error
- Ensure version exists: `lrops.sh list-versions`
- Check network connectivity to GitHub Container Registry
- Verify version format (semantic versions only)

### "Failed to pull image" Error
- Check internet connectivity
- Verify Docker daemon is running
- Ensure sufficient disk space
- Check Docker Hub rate limits

### "Container health check timeout" Error
- Check container logs: `lrops.sh logs livereview-app`
- Verify port availability
- Check system resources (CPU, memory, disk)
- Consider extending timeout for slower systems

### "Permission denied" Errors
- Ensure user has Docker permissions
- Use sudo if required: run with appropriate privileges
- Check file ownership in installation directory

## Technical Implementation Notes

### Version Detection Algorithm
The script uses GitHub Container Registry API to:
1. Get authentication token from `https://ghcr.io/token`
2. Query available tags from `https://ghcr.io/v2/hexmostech/livereview/tags/list`
3. Filter tags using semantic version regex: `^v?([0-9]+)\.([0-9]+)\.([0-9]+)(-[a-zA-Z0-9\-\.]+)?(\+[a-zA-Z0-9\-\.]+)?$`
4. Sort versions and return the highest

### Backup Strategy
The dual backup approach ensures reliability:
- **Physical snapshots**: Fast, complete, but version-dependent
- **Logical dumps**: Slower, but cross-version compatible and readable

### Container Recreation Strategy
Using `--no-deps --force-recreate` ensures:
- Only the application container is recreated (database remains untouched)
- New image is definitely used (not cached)
- Dependent services aren't unnecessarily restarted
- Minimal downtime and service disruption

This comprehensive update mechanism provides enterprise-grade safety and reliability for LiveReview installations.
