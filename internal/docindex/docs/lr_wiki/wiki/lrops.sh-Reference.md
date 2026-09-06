```
LiveReview Operations Script (lrops.sh)

USAGE:
    # Quick installation (recommended)
    curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | bash -s -- --express

    # Two-mode setup commands (new!)
    lrops.sh setup-demo                # Quick demo mode setup (localhost only)
    lrops.sh setup-production          # Production mode setup (with reverse proxy)

    # Interactive installation
    curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | bash

    # Specific version installation
    curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/master/lrops.sh | bash -s -- --version=v1.2.3 --express

    # Management commands (after installation)
    lrops.sh status                    # Show installation status
    lrops.sh info                      # Show installation details and file locations
    lrops.sh start                     # Start LiveReview services
    lrops.sh stop                      # Stop LiveReview services
    lrops.sh restart                   # Restart LiveReview services
    lrops.sh update [version]          # Pull newer image (or specific version) and restart
    lrops.sh backup [name]             # Create manual backup with optional custom name
    lrops.sh quick-backup              # Create quick timestamped backup
    lrops.sh list-backups              # List all available backups
    lrops.sh backup-info <name>        # Show detailed information about a backup
    lrops.sh delete-backup <name>      # Delete a specific backup
    lrops.sh restore <id|latest>       # Restore a previous backup
    lrops.sh set-mode <demo|production> # Switch between demo and production modes
    lrops.sh show-mode                 # Show current deployment mode and configuration
    
    # Backup options:
    --backup-dir <path>                # Specify custom backup directory (default: $LIVEREVIEW_INSTALL_DIR/backups)
    lrops.sh uninstall                 # Safely uninstall (moves directory, keeps backups)
    lrops.sh logs [service]            # Show container logs
    lrops.sh env validate              # Validate .env and suggest fixes
    lrops.sh help ssl                  # SSL/TLS setup guidance
    lrops.sh help backup               # Backup strategies

INSTALLATION OPTIONS:
    --express                          Use secure defaults, no prompts (demo mode)
    --force                           Overwrite existing installation
    --version=v1.2.3                  Install specific version (default: latest)
    --dry-run                         Show what would be done without installing
    --verbose, -v                     Enable verbose output
    --debug                           Enable bash debug tracing (set -x, also enables verbose output)

MANAGEMENT OPTIONS:
    --help, -h                        Show this help message
    --version                         Show script version
    --diagnose                        Run diagnostic checks

TWO-MODE DEPLOYMENT SYSTEM:
    Demo Mode (default):              Perfect for localhost development and testing
    - Access: http://localhost:8081/  
    - Webhooks: Disabled (manual triggers only)
    - No external access required
    
    Production Mode:                  Ready for external access with reverse proxy
    - Requires reverse proxy setup
    - Webhooks enabled for automatic triggers
    - SSL/TLS recommended

TEMPLATE & CONFIGURATION OPTIONS:
    --list-embedded-data              List all available embedded templates
    --test-extract <template>         Test extraction of specific template
    --extract-to <directory>          Extract all templates to directory

DEVELOPMENT & TESTING:
    --test-github-api                 Test GitHub Container Registry API
    --show-latest-version             Show latest available version
    --list-versions                   List all available versions

SAFETY:
    This script will NOT overwrite existing installations unless --force is specified.
    All configuration prompts have secure defaults.
    Express mode requires no user input and completes in under 5 minutes.

EXAMPLES:
    # Quick demo setup (recommended for first time)
    lrops.sh setup-demo

    # Production setup with reverse proxy
    lrops.sh setup-production

    # Force reinstall with specific version
    lrops.sh --force --version=v1.2.3 --express

    # Preview installation plan
    lrops.sh --dry-run --verbose

For more information, visit: https://github.com/HexmosTech/LiveReview
```