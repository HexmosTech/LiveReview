# LiveReview One-Line Installer Specification

## Overview
Create a user-friendly, self-contained one-line installer that customers can use to deploy LiveReview in production environments with minimal effort and maximum clarity.

## Core Requirements

### 1. Installation Command
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash
```
*Alternative: `wget -qO- https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash`*

**Script location**: `https://github.com/HexmosTech/LiveReview/lrops.sh`
**Script name**: `lrops.sh` (LiveReview Operations Script)

### 2. Self-Contained Design
- The installer script must embed the complete docker-compose.yml as a string
- No external file dependencies except for the script itself
- Should work on fresh systems with minimal prerequisites
- **Installation safety**: Will not overwrite existing installations
- **Force override**: Requires `--force` flag to overwrite existing installations
- **Self-installing**: Installs itself to `/usr/local/bin/lrops.sh` for ongoing management

### 3. Docker Image Source
- **Current**: `ghcr.io/hexmostech/livereview:dev-29cae7`
- **Target**: Use GitHub Container Registry (ghcr.io) for public distribution
- Need to update docker-compose.prod.yml to use `ghcr.io/hexmostech/livereview` instead of `git.apps.hexmos.com:5050/hexmos/livereview`

## Key Features

### 4. Interactive Setup Process
The installer should:
- **Check for existing installations** - abort if found (unless `--force` specified)
- Check system prerequisites (Docker, Docker Compose)
- **Express mode**: `--express` flag skips all prompts, uses secure defaults
- **Interactive mode** (default): Guide users through optional configuration:
  - Database password (auto-generated secure default)
  - Port configuration (default: 8888 for API, 8081 for UI)
  - Domain/hostname setup (default: localhost)
  - Basic networking options
- Create necessary directories and files
- Generate `.env` file with secure defaults
- **Install lrops.sh** to `/usr/local/bin/lrops.sh` for ongoing management
- **No mandatory configuration required** - all prompts have good defaults

### 5. Version Management Strategy
**Based on existing lrops.py implementation:**
- Uses semantic versioning with Git tags (format: `v1.2.3`)
- Tags are created with `git tag -a v1.2.3 -m "Release v1.2.3"`
- Version resolution follows `--sort=-version:refname` (latest semantic version first)

**Installation approach:**
- **Default behavior**: Install `latest` (highest semantic version tag from GitHub releases)
- **Version pinning**: Allow specific version: `curl -fsSL https://install.livereview.com | bash -s -- --version=v1.2.3`
- **Version validation**: Ensure requested version exists in GitHub releases
- **Migration handling**: Use dbmate for database migrations with up/down sections

**Version resolution logic:**
```
latest → Query GitHub API for latest release tag → Filter only semantic version tags (v1.2.3) → Use highest
v1.2.3 → Validate tag exists → Use specified version
```

**Important**: Only semantic version tags (`v1.2.3` format) are considered for `latest`. Development tags like `dev-29cae7` are excluded from latest resolution.

### 6. User Experience Goals
- **Installation safety**: Prevents accidental overwrites of existing installations
- **Auto-updating script**: Every curl/wget run updates the installed `lrops.sh`
- **Express installation**: `curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --express`
- **Force reinstall**: `curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --force --express`
- **Clear progress indicators** during installation
- **Helpful error messages** with troubleshooting guidance
- **Comprehensive post-installation summary** with:
  - Access URLs (http://localhost:8888/api, http://localhost:8081)
  - Container status and health
  - Configuration file locations
  - Next steps and configuration guidance
  - Instructions for using installed `lrops.sh` command
- **Validation checks** to ensure successful deployment

## Technical Implementation

### 7. Version Resolution Flow
```bash
# Default installation (latest)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash
  ↓
Check for existing installation → Abort if found (unless --force)
  ↓
GitHub API → /repos/hexmostech/livereview/releases/latest
  ↓
Extract tag_name (e.g., "v1.2.3")
  ↓
Use ghcr.io/hexmostech/livereview:1.2.3
  ↓
Install lrops.sh to /usr/local/bin/lrops.sh

# Pinned version installation  
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --version=v1.1.5
  ↓
Check for existing installation → Abort if found (unless --force)
  ↓
Validate v1.1.5 exists in GitHub releases
  ↓
Use ghcr.io/hexmostech/livereview:1.1.5
```

### 8. Embedded Docker Compose
The script will contain a production-ready docker-compose.yml that:
- Uses the GitHub Container Registry image
- Includes health checks and proper restart policies
- Sets up PostgreSQL with persistent storage
- Configures networking and security properly
- Handles environment variable injection

### 9. Directory Structure Created
```
/opt/livereview/           # Installation directory
├── docker-compose.yml    # Generated from embedded template
├── .env                  # User-configured environment
├── lrdata/              # Persistent data directory
│   └── postgres/        # Database storage
├── migrations/          # dbmate migration files (if needed)
├── config/              # Configuration templates and examples
│   ├── nginx.conf.example        # Nginx reverse proxy config
│   ├── caddy.conf.example        # Caddy reverse proxy config
│   ├── apache.conf.example       # Apache reverse proxy config
│   └── backup-cron.example       # Backup cron script template
└── scripts/             # Helper scripts
    ├── backup.sh        # Manual backup script
    └── restore.sh       # Manual restore script
```

### 10. Migration Handling
- Use existing dbmate infrastructure for database schema changes
- Migrations have up/down sections for safe rollbacks
- Installer should run `dbmate up` after container startup
- Future: Version-aware migration validation

### 11. Configuration Templates and Assistance

#### Ongoing Management with lrops.sh
Once installed, users can use `lrops.sh` for ongoing management:

```bash
# Installation and system status
lrops.sh status                    # Show installation status, version, container health
lrops.sh info                      # Show access URLs, configuration locations

# Docker container management  
lrops.sh start                     # Start LiveReview containers
lrops.sh stop                      # Stop LiveReview containers
lrops.sh restart                   # Restart LiveReview containers
lrops.sh logs [service]            # Show container logs (optional service filter)

# Help and templates for various topics
lrops.sh help ssl                  # SSL/TLS setup guidance and certbot commands
lrops.sh help backup               # Backup/restore strategies and scripts
lrops.sh help nginx                # Nginx reverse proxy configuration
lrops.sh help caddy                # Caddy reverse proxy configuration
lrops.sh help apache               # Apache reverse proxy configuration

# Script management (auto-updates on every curl/wget run)
lrops.sh self-update               # Manually update lrops.sh script
lrops.sh version                   # Show lrops.sh script version

# Future features
lrops.sh uninstall                 # Clean removal of LiveReview installation
```

#### Reverse Proxy Templates
**Nginx configuration template** (`config/nginx.conf.example`):
```nginx
server {
    listen       80;
    server_name  _;

    # Route /api/* to port 8888
    location ^~ /api/ {
        proxy_pass http://127.0.0.1:8888;
    }

    # Everything else to port 8081
    location / {
        proxy_pass http://127.0.0.1:8081;
    }
}
```

**Caddy configuration template** (`config/caddy.conf.example`):
```caddy
your-domain.com {
    handle /api/* {
        reverse_proxy localhost:8888
    }
    handle {
        reverse_proxy localhost:8081
    }
}
```

### 12. Script Organization for Implementation

#### Embedded Data Management
To maintain script readability while embedding multiple data files:

```bash
#!/bin/bash
# lrops.sh - LiveReview Operations Script

# Main script logic here...

# =============================================================================
# EMBEDDED DATA SECTION
# =============================================================================

# Function to extract embedded data
extract_data() {
    local data_name="$1"
    local output_file="$2"
    
    # Extract data between markers
    sed -n "/^# === DATA:${data_name} ===/,/^# === END:${data_name} ===/p" "$0" \
        | grep -v "^# === " > "$output_file"
}

# Script continues...
exit 0

# =============================================================================
# EMBEDDED DATA FILES BELOW THIS LINE
# =============================================================================

# === DATA:docker-compose.yml ===
services:
  livereview-app:
    image: ghcr.io/hexmostech/livereview:${LIVEREVIEW_VERSION}
    # ... full docker-compose content
# === END:docker-compose.yml ===

# === DATA:nginx.conf ===
server {
    listen       80;
    server_name  _;
    # ... nginx config
# === END:nginx.conf ===

# === DATA:caddy.conf ===
your-domain.com {
    # ... caddy config
# === END:caddy.conf ===

# === DATA:backup.sh ===
#!/bin/bash
# Backup script content
# === END:backup.sh ===
```

#### Data Organization Benefits
- **Readable main script**: Logic separated from data
- **Easy maintenance**: Each template has clear boundaries
- **Simple extraction**: Single function handles all data extraction
- **Version control friendly**: Clear structure for diffs and updates
- Template commands for certbot installation and certificate generation
- Instructions for updating nginx/caddy configs for HTTPS
- Automatic renewal setup guidance

#### SSL/TLS Setup Guidance
- Template commands for certbot installation and certificate generation
- Instructions for updating nginx/caddy configs for HTTPS
- Automatic renewal setup guidance

#### Backup/Restore Templates
**Local backup script** (`scripts/backup.sh`):
```bash
#!/bin/bash
# Simple local backup to /opt/livereview-backups/
# Usage: ./backup.sh [backup-name]
```

**Rclone S3 backup template** (`config/backup-cron.example`):
```bash
# Daily backup to S3-compatible storage
0 2 * * * cd /opt/livereview && ./scripts/backup.sh && rclone sync ./backups/ s3:mybucket/livereview-backups/
```

## Questions for Clarification

### Versioning Strategy
1. ✅ **Resolved**: Semantic versioning with Git tags (`v1.2.3` format)
2. ✅ **Resolved**: Support version pinning via `--version=v1.2.3` parameter
3. ✅ **Resolved**: `latest` resolves to highest semantic version from GitHub releases
4. **Future consideration**: Database migration handling with dbmate (up/down migrations)
5. **Future consideration**: Upgrade path validation and incremental update strategy

### Configuration Options
4. ✅ **Resolved**: No mandatory configuration - all prompts have secure defaults with `--express` mode
5. ✅ **Resolved**: SSL/TLS assistance via configuration templates (nginx/caddy/apache) and certbot guidance
6. ✅ **Resolved**: Reverse proxy setup templates provided, not automated

### Deployment Environment
7. ✅ **Resolved**: Target is customer company servers (varied environments) - Docker requirement sufficient
8. ✅ **Resolved**: Linux machines with Docker support (amd64/arm64 via universal image)
9. ✅ **Resolved**: Backup/restore via templates and helper scripts (local folder, rclone, S3-compatible)

### Update Strategy
10. **Future consideration**: Incremental update mechanism design
11. **Future consideration**: Safe update criteria and validation rules  
12. ✅ **Resolved**: Database migrations handled via dbmate with up/down sections

## Success Criteria
- User can go from zero to running LiveReview in under 5 minutes
- **Express mode**: Zero-prompt installation for immediate deployment
- Installation works on major Linux distributions (Ubuntu, CentOS, Debian)
- Supports both amd64 and arm64 architectures via universal Docker image
- Clear feedback at each step with actionable error messages
- Generated setup is production-ready with security best practices
- Comprehensive post-installation guidance with templates for:
  - Reverse proxy configuration (nginx/caddy/apache)
  - SSL/TLS setup with certbot
  - Backup/restore strategies (local, rclone, S3-compatible)
- Easy to maintain and update the installer itself

## Installation Modes

### Express Mode (Recommended for Quick Start)
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --express
```
- Uses all secure defaults
- Auto-generates strong database password
- Deploys to `/opt/livereview/`
- Installs `lrops.sh` to `/usr/local/bin/`
- Provides comprehensive post-installation summary

### Interactive Mode (Default)
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash
```
- Guides through optional configuration
- All prompts have sensible defaults
- No mandatory fields
- Installs management script for ongoing operations

### Version-Specific Installation
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --version=v1.2.3 --express
```

### Force Reinstall (Overwrite Existing)
```bash
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --force --express
```

### Self-Update Installed Script
```bash
# Manual update
lrops.sh self-update

# Automatic update (every curl/wget run updates lrops.sh)
curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --help
```
- Updates the installed `/usr/local/bin/lrops.sh` to latest version
- Preserves existing LiveReview installation
- **Auto-update**: Every curl/wget execution updates the installed script