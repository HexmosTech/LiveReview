# LiveReview One-Line Installer Implementation Checklist

This checklist breaks down the implementation of `lrops.sh` into manageable phases based on the specification in `one-line-installer.md`.

## Phase 1: Core Infrastructure & Basic Script Structure

### 1.1 Script Foundation
- [x] Create `lrops.sh` with proper shebang and header
- [x] Implement command-line argument parsing
  - [x] `--express` flag for non-interactive mode
  - [x] `--force` flag for overwriting existing installations
  - [x] `--version` flag for specific version installation
  - [x] `--help` flag for usage information
- [x] Add script version management and `--version` output
- [x] Implement logging/output functions with progress indicators
- [x] Add error handling and cleanup functions

### 1.2 Safety Checks
- [x] Implement existing installation detection
  - [x] Check for `/opt/livereview/` directory
  - [x] Check for running LiveReview containers
  - [x] Abort if found (unless `--force` specified)
- [x] System prerequisites validation
  - [x] Check for Docker installation and version
  - [x] Check for Docker Compose availability
  - [x] Verify Docker daemon is running
  - [x] Check system architecture (amd64/arm64)
- [x] Permissions and sudo access verification

### 1.3 Self-Installation Framework
- [x] Implement self-installation to `/usr/local/bin/lrops.sh`
- [x] Add executable permissions setting
- [x] Implement self-update functionality
- [x] Auto-update logic (update script on every curl/wget run)

### Phase 1 Validation Commands
```bash
# Test basic script functionality
./lrops.sh --help                    # Should show usage information   [x]
./lrops.sh --version                 # Should show script version      [x]
./lrops.sh --express --dry-run       # Should parse args without errors [x]

# Test safety checks
./lrops.sh --express                 # Should check prerequisites and abort safely [x]
docker --version                     # Verify Docker detection works   [x]
docker-compose --version             # Verify Docker Compose detection works [x]

# Test self-installation (if implemented)
sudo ./lrops.sh --install-self       # Should install to /usr/local/bin/ [x]
which lrops.sh                       # Should find installed script    [x]
lrops.sh --version                   # Should work from PATH           [x]
```

## Phase 2: Version Management & GitHub Integration

### 2.1 GitHub API Integration
- [x] Implement GitHub Container Registry API client
  - [x] Query GitHub Container Registry for available tags
  - [x] Parse tag data and extract semantic versions
  - [x] Filter for semantic version tags only (exclude dev-*, etc.)
- [x] Version validation
  - [x] Validate semantic version format (`v1.2.3`)
  - [x] Verify specified versions exist in GHCR
- [x] Error handling for network failures and API rate limits

### 2.2 Version Resolution Logic
- [x] Implement `latest` version resolution
  - [x] Query GHCR API for all available tags
  - [x] Extract highest semantic version tag
  - [x] Convert to Docker image tag format (remove 'v' prefix)
- [x] Implement pinned version handling
  - [x] Validate user-specified version exists
  - [x] Convert to proper Docker image tag
- [x] Version comparison and sorting utilities

### Phase 2 Validation Commands
```bash
# Test GitHub API integration
./lrops.sh --test-github-api          # Should show latest release info [x]
./lrops.sh --version=v1.0.0 --dry-run # Should validate version exists  [x]
./lrops.sh --version=v999.999.999 --dry-run # Should fail with clear error [x]

# Test version resolution
./lrops.sh --show-latest-version      # Should show resolved latest version [x]
./lrops.sh --version=latest --dry-run # Should resolve to specific version [x]

# Test semantic version filtering
./lrops.sh --list-versions            # Should show only semantic versions (no dev-*) [x]
```

## Phase 3: Embedded Templates & Configuration Files

### 3.1 Template Embedding System
- [x] Implement `extract_data()` function to parse `# === DATA:name ===` markers
- [x] Add embedded data extraction to bottom of `lrops.sh` script
- [x] Test template extraction and file generation

### 3.2 Core Configuration Templates
- [x] Embed `docker-compose.yml` template
  - [x] Use `ghcr.io/hexmostech/livereview:${LIVEREVIEW_VERSION}` image
  - [x] Include PostgreSQL service with persistent volumes
  - [x] Add health checks and restart policies
  - [x] Include environment variable placeholders
- [x] Embed `.env` template with secure defaults
  - [x] Database credentials with auto-generated password
  - [x] Port configurations (8888/8081)
  - [x] Basic security settings

### 3.3 Reverse Proxy Templates
- [x] Embed `nginx.conf.example` template
  - [x] HTTP configuration routing /api/* to port 8888
  - [x] Route everything else to port 8081 (UI)
  - [x] Include HTTPS/SSL template sections
- [x] Embed `caddy.conf.example` template
  - [x] Caddy v2 configuration with automatic HTTPS
  - [x] API and UI reverse proxy setup
- [x] Embed `apache.conf.example` template
  - [x] Virtual host configuration
  - [x] ProxyPass rules for API and UI

### 3.4 Backup & Maintenance Scripts
- [x] Embed `backup.sh` script template
  - [x] Database dump functionality
  - [x] Configuration file backup
  - [x] Timestamped backup directories
  - [x] Compression and cleanup
- [x] Embed `restore.sh` script template
  - [x] Database restoration from backups
  - [x] Configuration restoration
  - [x] Validation and error handling
- [x] Embed `backup-cron.example` template
  - [x] Cron job examples for automated backups
  - [x] Rclone S3 integration examples
  - [x] Log rotation and cleanup

### 3.5 SSL/TLS Setup Templates
- [x] Embed certbot installation commands
  - [x] Ubuntu/Debian installation steps
  - [x] CentOS/RHEL installation steps
- [x] Embed certificate generation templates
  - [x] Domain validation setup
  - [x] Certificate renewal automation
- [x] Embed HTTPS configuration updates for nginx/caddy/apache

### Phase 3 Validation Commands
```bash
# Test template extraction framework
./lrops.sh --test-extract docker-compose.yml # Should extract docker-compose to temp file [x]
./lrops.sh --test-extract nginx.conf.example # Should extract nginx config [x]
./lrops.sh --test-extract backup.sh          # Should extract backup script [x]
./lrops.sh --list-embedded-templates         # Should show all available templates [x]

# Test extracted content validity
./lrops.sh --extract-to /tmp/lr-templates/
ls -la /tmp/lr-templates/                     # Should show all extracted templates [x]
grep "ghcr.io/hexmostech/livereview" /tmp/lr-templates/docker-compose.yml # Should find correct image [x]
grep "LIVEREVIEW_VERSION" /tmp/lr-templates/docker-compose.yml            # Should find variable placeholder [x]

# Validate template syntax
docker-compose -f /tmp/lr-templates/docker-compose.yml config # Should validate without errors [x]
nginx -t -c /tmp/lr-templates/config/nginx.conf.example 2>/dev/null || echo "Nginx config OK" [x]
bash -n /tmp/lr-templates/scripts/backup.sh                           # Should validate bash syntax [x]

# Test template installation to /opt/livereview/config/
./lrops.sh --install-templates-only --output-dir=/tmp/lr-test
ls -la /tmp/lr-test/config/                   # Should show nginx.conf.example, caddy.conf.example, etc. [x]
ls -la /tmp/lr-test/scripts/                  # Should show backup.sh, restore.sh [x]
cat /tmp/lr-test/config/backup-cron.example   # Should show cron examples [x]
```

## ✅ Phase 4: Installation Core Logic

### 4.1 Interactive Configuration ✅
- [x] Implement interactive prompts with defaults
  - [x] Database password (auto-generate secure default)
  - [x] Port configuration (8888/8081 defaults)
  - [x] Domain/hostname setup (localhost default)
- [x] Implement express mode (skip all prompts)
- [x] Configuration validation and sanitization
- [x] Generate secure random passwords

### 4.2 Directory Structure Creation ✅
- [x] Create `/opt/livereview/` directory structure
  - [x] Main installation directory
  - [x] `lrdata/` for persistent data
  - [x] `lrdata/postgres/` for database storage
  - [x] `config/` for templates and examples
  - [x] `scripts/` for helper scripts
- [x] Set proper ownership and permissions
- [x] Handle existing directory conflicts

### 4.3 File Generation ✅
- [x] Generate docker-compose.yml from template
  - [x] Substitute environment variables
  - [x] Set correct image version
  - [x] Configure volume mounts
- [x] Generate .env file with user configuration
- [x] Extract configuration templates to config/ directory
- [x] Extract helper scripts to scripts/ directory

### Phase 4 Validation Commands
```bash
# Test interactive configuration (if no existing installation)
./lrops.sh --dry-run                         # Should prompt for config, not install
echo -e "\n\n\n\n" | ./lrops.sh --dry-run   # Should accept defaults

# Test express mode
./lrops.sh --express --dry-run               # Should use all defaults, no prompts

# Test directory structure creation
./lrops.sh --express --dry-run --show-plan   # Should show what directories will be created
ls -la /opt/livereview 2>/dev/null || echo "No existing installation (good)"

# Test configuration generation
./lrops.sh --express --generate-config-only --output-dir=/tmp/lr-test
ls -la /tmp/lr-test/                          # Should show generated files
cat /tmp/lr-test/.env                         # Should show populated environment
grep -v "^#" /tmp/lr-test/.env | grep "="     # Should show actual values, not placeholders
```

## Phase 5: Docker Deployment

### 5.1 Container Management
- [ ] Pull required Docker images
  - [ ] LiveReview application image
  - [ ] PostgreSQL image
- [ ] Start containers with docker-compose
- [ ] Wait for containers to become healthy
- [ ] Verify successful deployment

### 5.2 Database Setup
- [ ] Wait for PostgreSQL to be ready
- [ ] Run database migrations if needed
- [ ] Verify database connectivity
- [ ] Handle migration errors gracefully

### 5.3 Health Checks
- [ ] Implement container health verification
- [ ] Check API endpoint availability (port 8888)
- [ ] Check UI availability (port 8081)
- [ ] Verify database connectivity

### Phase 5 Validation Commands
```bash
# Test actual installation (requires clean system or --force)
./lrops.sh --express --force                 # Should complete full installation

# Verify container deployment
docker ps                                    # Should show livereview-app and livereview-db
docker-compose -f /opt/livereview/docker-compose.yml ps # Should show healthy containers

# Test application accessibility
curl -f http://localhost:8888/api/health || echo "API not ready yet"
curl -f http://localhost:8081/ || echo "UI not ready yet"

# Test database connectivity
docker exec livereview-db pg_isready -U livereview # Should show "accepting connections"

# Verify data persistence
ls -la /opt/livereview/lrdata/postgres/       # Should show PostgreSQL data files
docker logs livereview-app | tail -10         # Should show app startup logs
docker logs livereview-db | tail -10          # Should show db startup logs
```

## Phase 6: Management Commands

### 6.1 Status and Information Commands
- [ ] `lrops.sh status`
  - [ ] Show installation status
  - [ ] Display version information
  - [ ] Show container health status
  - [ ] Report database status
- [ ] `lrops.sh info`
  - [ ] Display access URLs
  - [ ] Show configuration file locations
  - [ ] List important directories

### 6.2 Container Management Commands
- [ ] `lrops.sh start` - Start all containers
- [ ] `lrops.sh stop` - Stop all containers
- [ ] `lrops.sh restart` - Restart all containers
- [ ] `lrops.sh logs [service]` - Show container logs
  - [ ] Support optional service filtering
  - [ ] Add timestamp and follow options

### 6.3 Help System Commands
- [ ] `lrops.sh help ssl`
  - [ ] SSL/TLS setup guidance
  - [ ] Certbot installation commands
  - [ ] Certificate renewal setup
- [ ] `lrops.sh help backup`
  - [ ] Backup strategies explanation
  - [ ] Local backup script usage
  - [ ] Rclone S3 backup setup
- [ ] `lrops.sh help nginx` - Nginx reverse proxy configuration
- [ ] `lrops.sh help caddy` - Caddy reverse proxy configuration
- [ ] `lrops.sh help apache` - Apache reverse proxy configuration

### Phase 6 Validation Commands
```bash
# Test management commands (requires Phase 5 completed)
lrops.sh status                               # Should show installation status
lrops.sh info                                 # Should show access URLs and file locations

# Test container management
lrops.sh stop                                 # Should stop containers
docker ps | grep livereview || echo "Containers stopped"
lrops.sh start                                # Should start containers
docker ps | grep livereview                  # Should show running containers
lrops.sh restart                              # Should restart containers
lrops.sh logs                                 # Should show recent logs
lrops.sh logs livereview-app                  # Should show app-specific logs

# Test help system
lrops.sh help ssl                             # Should show SSL guidance
lrops.sh help backup                          # Should show backup instructions
lrops.sh help nginx                           # Should show nginx config
lrops.sh help caddy                           # Should show caddy config
lrops.sh help apache                          # Should show apache config

# Test self-update
lrops.sh self-update                          # Should update script
lrops.sh version                              # Should show current version
```

## Phase 7: Configuration Templates

### 7.1 Reverse Proxy Templates
- [ ] Create comprehensive nginx.conf.example
  - [ ] HTTP configuration for API and UI routing
  - [ ] HTTPS configuration template
  - [ ] SSL certificate paths
- [ ] Create caddy.conf.example with automatic HTTPS
- [ ] Create apache.conf.example with virtual host setup
- [ ] Include setup instructions for each proxy type

### 7.2 Backup Templates
- [ ] Create backup.sh script
  - [ ] Local backup to timestamped directories
  - [ ] Database dump functionality
  - [ ] Configuration backup
- [ ] Create restore.sh script
  - [ ] Restore from backup archives
  - [ ] Database restoration
  - [ ] Configuration restoration
- [ ] Create backup-cron.example
  - [ ] Cron job templates
  - [ ] Rclone S3 integration examples

### 7.3 SSL/TLS Templates
- [ ] Create certbot installation scripts
- [ ] Create certificate generation commands
- [ ] Create automatic renewal setup
- [ ] Include security best practices

### Phase 7 Validation Commands
```bash
# Test configuration templates extraction
ls -la /opt/livereview/config/                # Should show all template files
cat /opt/livereview/config/nginx.conf.example # Should show complete nginx config
cat /opt/livereview/config/caddy.conf.example # Should show caddy config
cat /opt/livereview/config/apache.conf.example # Should show apache config

# Test backup scripts
ls -la /opt/livereview/scripts/               # Should show backup/restore scripts
bash -n /opt/livereview/scripts/backup.sh    # Should validate bash syntax
bash -n /opt/livereview/scripts/restore.sh   # Should validate bash syntax
cat /opt/livereview/config/backup-cron.example # Should show cron examples

# Test template validity
nginx -t -c /opt/livereview/config/nginx.conf.example 2>/dev/null || echo "Nginx template needs adjustment"

# Test help content completeness
lrops.sh help ssl | wc -l                    # Should show substantial content (>10 lines)
lrops.sh help backup | grep -i rclone        # Should mention rclone
lrops.sh help nginx | grep -i certbot        # Should mention certbot for SSL
```

## Phase 8: Post-Installation Experience

### 8.1 Installation Summary
- [ ] Display comprehensive post-installation report
  - [ ] Access URLs with clickable links
  - [ ] Container status summary
  - [ ] Configuration file locations
  - [ ] Next steps and recommendations
- [ ] Generate installation report file
- [ ] Display management command examples

### 8.2 Validation and Testing
- [ ] Verify all services are accessible
- [ ] Test API endpoints
- [ ] Test UI responsiveness
- [ ] Validate database connectivity
- [ ] Check log output for errors

### 8.3 Troubleshooting Support
- [ ] Implement common error detection
- [ ] Provide helpful error messages with solutions
- [ ] Include links to documentation
- [ ] Log detailed debugging information

### Phase 8 Validation Commands
```bash
# Test post-installation experience
./lrops.sh --express --force 2>&1 | tee /tmp/install.log # Capture full installation output
grep -i "access urls" /tmp/install.log        # Should show access information
grep -i "next steps" /tmp/install.log         # Should show guidance
grep -i "error" /tmp/install.log && echo "Found errors!" || echo "Clean installation"

# Test validation and health checks
lrops.sh status                               # Should show all green/healthy
curl -f http://localhost:8888/api/health && echo "API healthy"
curl -f http://localhost:8081/ && echo "UI accessible"

# Test troubleshooting features
lrops.sh --diagnose                           # Should run diagnostic checks
lrops.sh status --verbose                     # Should show detailed status
lrops.sh logs --tail=50                       # Should show recent logs for debugging

# Verify installation report
ls -la /opt/livereview/                       # Should show all expected files
cat /opt/livereview/installation-report.txt || echo "Report file not found"
```

## Phase 9: Advanced Features

### 9.1 Update/Upgrade Framework (Future)
- [ ] Design upgrade detection logic
- [ ] Plan database migration handling
- [ ] Design configuration preservation
- [ ] Plan rollback mechanisms

### 9.2 Uninstall Functionality (Future)
- [ ] `lrops.sh uninstall` command
- [ ] Safe container removal
- [ ] Data preservation options
- [ ] Clean removal of files and directories

### 9.3 Configuration Management
- [ ] Configuration validation commands
- [ ] Configuration backup/restore
- [ ] Environment variable management
- [ ] Secret rotation utilities

### Phase 9 Validation Commands
```bash
# Test future upgrade framework (when implemented)
lrops.sh --check-upgrades                    # Should check for newer versions
lrops.sh --backup-before-upgrade             # Should create backup before upgrade
lrops.sh upgrade --dry-run                   # Should show upgrade plan

# Test uninstall functionality (when implemented)
lrops.sh uninstall --dry-run                 # Should show what would be removed
lrops.sh uninstall --keep-data --dry-run     # Should show data preservation option

# Test configuration management
lrops.sh config validate                     # Should validate current configuration
lrops.sh config backup                       # Should backup configuration
lrops.sh config show                         # Should display current config (masked secrets)
lrops.sh config rotate-secrets               # Should generate new passwords/secrets
```

## Phase 10: Testing & Quality Assurance

### 10.1 Testing Framework
- [ ] Create test scripts for each phase
- [ ] Test on different Linux distributions
  - [ ] Ubuntu 20.04/22.04
  - [ ] CentOS 7/8
  - [ ] Debian 10/11
- [ ] Test different Docker versions
- [ ] Test on both amd64 and arm64 architectures

### 10.2 Error Scenarios Testing
- [ ] Test network failures during installation
- [ ] Test Docker daemon not running
- [ ] Test insufficient permissions
- [ ] Test port conflicts
- [ ] Test existing installation scenarios

### 10.3 User Experience Testing
- [ ] Test express mode installation
- [ ] Test interactive mode with various inputs
- [ ] Test all management commands
- [ ] Test help system completeness
- [ ] Test self-update functionality

### Phase 10 Validation Commands
```bash
# Full system testing
./test-all-distributions.sh                  # Test on Ubuntu, CentOS, Debian
./test-architectures.sh                      # Test on amd64 and arm64
./test-error-scenarios.sh                    # Test network failures, permission issues

# Performance and reliability testing
time ./lrops.sh --express --force            # Should complete in <5 minutes
for i in {1..5}; do lrops.sh restart; sleep 30; lrops.sh status | grep healthy; done

# User experience validation
./lrops.sh --help | wc -l                    # Should show comprehensive help (>30 lines)
lrops.sh status --json | jq .                # Should provide machine-readable status
lrops.sh info --urls-only                    # Should show just access URLs

# Final integration test
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --express --force
# Should work end-to-end from GitHub
```

### Continuous Validation Commands
```bash
# Quick health check (run after any changes)
lrops.sh status && echo "✅ System healthy"

# Quick functionality test
lrops.sh restart && sleep 30 && curl -f http://localhost:8888/api/health && echo "✅ API working"

# Quick management test
lrops.sh logs --tail=5 && echo "✅ Logs accessible"
```

## Implementation Dependencies

### Prerequisites
- Access to GitHub API (no authentication required for public repos)
- Docker and Docker Compose availability
- Bash 4.0+ for script functionality
- `curl` or `wget` for API calls
- Standard Unix utilities (`sed`, `grep`, `awk`)

### External Dependencies
- GitHub Container Registry access for Docker images
- Internet connectivity for downloads
- Sufficient disk space for Docker images and data

## Success Criteria

### Functional Requirements
- [ ] Zero-to-running in under 5 minutes
- [ ] Works on major Linux distributions
- [ ] Supports both amd64 and arm64
- [ ] No mandatory configuration required
- [ ] Safe by default (no overwrites without --force)

### Quality Requirements
- [ ] Clear error messages and troubleshooting guidance
- [ ] Comprehensive help system
- [ ] Robust error handling and recovery
- [ ] Self-updating mechanism
- [ ] Production-ready security defaults

### User Experience Requirements
- [ ] Intuitive command structure
- [ ] Comprehensive post-installation guidance
- [ ] Easy ongoing management
- [ ] Clear documentation and examples
