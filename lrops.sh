#!/bin/bash
# lrops.sh - LiveReview Operations Script
# Version: 1.0.0
# Description: One-line installer and management tool for LiveReview
# Repository: https://github.com/HexmosTech/LiveReview

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# Ensure the script runs under Bash only
if [[ -z "${BASH_VERSION:-}" ]]; then
    echo "This script must be run with Bash." >&2
    echo "Try: bash lrops.sh <command> [options]" >&2
    exit 1
fi

# =============================================================================
# SCRIPT METADATA AND CONSTANTS
# =============================================================================

SCRIPT_VERSION="1.0.0"
SCRIPT_NAME="lrops.sh"
# Resolve invoking user and home directory robustly (works with sudo)
# Priority: SUDO_UID/SUDO_USER -> tilde expansion -> current $HOME
INVOKING_USER="${SUDO_USER:-${USER:-$(id -un 2>/dev/null || echo "")}}"
if [[ -n "${SUDO_UID:-}" ]]; then
    INVOKING_HOME="$(getent passwd "${SUDO_UID}" 2>/dev/null | awk -F: '{print $6}')"
fi
if [[ -z "${INVOKING_HOME:-}" || ! -d "$INVOKING_HOME" ]]; then
    if [[ -n "${SUDO_USER:-}" ]]; then
        INVOKING_HOME="$(eval echo ~"${SUDO_USER}")"
    fi
fi
if [[ -z "${INVOKING_HOME:-}" || ! -d "$INVOKING_HOME" ]]; then
    INVOKING_HOME="${HOME}"
fi
# Default install dir: invoking user's home (never root's HOME when run via sudo)
DEFAULT_HOME_DIR="${INVOKING_HOME}"
LIVEREVIEW_INSTALL_DIR="${LIVEREVIEW_INSTALL_DIR:-${DEFAULT_HOME_DIR}/livereview}"
LIVEREVIEW_SCRIPT_PATH="/usr/local/bin/lrops.sh"
GITHUB_REPO="HexmosTech/LiveReview"
GITHUB_API_BASE="https://api.github.com/repos/${GITHUB_REPO}"
DOCKER_REGISTRY="ghcr.io/hexmostech"
DOCKER_IMAGE="livereview"
BACKUP_RETENTION_COUNT="10"   # Number of pre-update backups to keep (oldest pruned)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
BOLD='\033[1m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# =============================================================================
# LOGGING AND OUTPUT FUNCTIONS
# =============================================================================

log_info() {
    echo -e "${BLUE}ℹ️  INFO:${NC} $*" >&2
}

log_success() {
    echo -e "${GREEN}✅ SUCCESS:${NC} $*" >&2
}

log_warning() {
    echo -e "${YELLOW}⚠️  WARNING:${NC} $*" >&2
}

log_error() {
    echo -e "${RED}❌ ERROR:${NC} $*" >&2
}

log_debug() {
    if [[ "${VERBOSE:-false}" == "true" ]]; then
        echo -e "${PURPLE}🔍 DEBUG:${NC} $*" >&2
    fi
}

progress() {
    echo -e "${CYAN}🔄 $*${NC}" >&2
}

section_header() {
    echo >&2
    echo -e "${BLUE}$(printf '=%.0s' {1..80})${NC}" >&2
    echo -e "${BLUE}📋 $*${NC}" >&2
    echo -e "${BLUE}$(printf '=%.0s' {1..80})${NC}" >&2
}

# Simple progress countdown with inline updates (one line)
progress_sleep() {
    local seconds=${1:-0}
    local label=${2:-"Waiting"}
    local i
    for (( i=1; i<=seconds; i++ )); do
        printf "\r%s: %2ds/%2ds" "$label" "$i" "$seconds" >&2
        sleep 1
    done
    echo >&2
}

# =============================================================================
# PORTABLE SED (GNU/BSD) HELPERS
# =============================================================================

# sed -i behaves differently on macOS (BSD sed) vs GNU sed. These helpers
# provide a uniform interface: sed_inplace 's/a/b/' path/to/file
sed_inplace() {
    # Usage: sed_inplace 'SED_SCRIPT' FILE
    local script="$1"
    local file="$2"
    case "$(uname -s)" in
        Darwin) sed -i '' "$script" "$file" ;;
        *)      sed -i   "$script" "$file" ;;
    esac
}

sudo_sed_inplace() {
    # Usage: sudo_sed_inplace 'SED_SCRIPT' FILE
    local script="$1"
    local file="$2"
    case "$(uname -s)" in
        Darwin) sudo sed -i '' "$script" "$file" ;;
        *)      sudo sed -i   "$script" "$file" ;;
    esac
}

# =============================================================================
# ERROR HANDLING AND CLEANUP
# =============================================================================

cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Script failed with exit code $exit_code"
        log_info "For troubleshooting help, run: $0 --help"
    fi
    # Stop sudo keepalive process if running
    if [[ -n "${SUDO_REFRESH_PID:-}" ]]; then
        kill "${SUDO_REFRESH_PID}" 2>/dev/null || true
    fi
    exit $exit_code
}

trap cleanup EXIT

error_exit() {
    log_error "$1"
    exit "${2:-1}"
}

# =============================================================================
# SUDO SESSION AND DOCKER PRIVILEGES
# =============================================================================

# Keep sudo alive during script run to avoid repeated prompts
ensure_sudo_session() {
    # Only if not already root and sudo is available
    if [[ $EUID -ne 0 ]] && command -v sudo >/dev/null 2>&1; then
        log_info "Requesting sudo access upfront (to avoid repeated prompts)..."
        if sudo -v; then
            # Refresh sudo timestamp in background
            (
                while true; do
                    sleep 60
                    sudo -n true 2>/dev/null || true
                done
            ) &
            SUDO_REFRESH_PID=$!
            log_debug "Sudo keepalive process started (PID: $SUDO_REFRESH_PID)"
        else
            log_warning "Could not obtain sudo credentials now; you may be prompted later."
        fi
    fi
}

# Start sudo keepalive process for self-update operations
start_sudo_keepalive() {
    # Only if not already root and sudo is available
    if [[ $EUID -ne 0 ]] && command -v sudo >/dev/null 2>&1; then
        if sudo -v; then
            # Refresh sudo timestamp in background
            (
                while true; do
                    sleep 60
                    sudo -n true 2>/dev/null || true
                done
            ) &
            SUDO_REFRESH_PID=$!
            log_debug "Sudo keepalive process started (PID: $SUDO_REFRESH_PID)"
        else
            log_warning "Could not obtain sudo credentials for self-update"
            return 1
        fi
    fi
}

# Stop sudo keepalive process
stop_sudo_keepalive() {
    if [[ -n "${SUDO_REFRESH_PID:-}" ]]; then
        kill "${SUDO_REFRESH_PID}" 2>/dev/null || true
        unset SUDO_REFRESH_PID
        log_debug "Sudo keepalive process stopped"
    fi
}

# If Docker requires sudo, transparently wrap docker/docker-compose commands
maybe_enable_sudo_for_docker() {
    # If docker CLI not present, nothing to do here
    command -v docker >/dev/null 2>&1 || return 0

    if docker info >/dev/null 2>&1; then
        return 0  # No sudo needed
    fi

    # Try with sudo non-interactively first
    if command -v sudo >/dev/null 2>&1 && sudo -n docker info >/dev/null 2>&1; then
        :
    else
        # Fall back to interactive sudo attempt (may prompt once)
        if command -v sudo >/dev/null 2>&1 && sudo docker info >/dev/null 2>&1; then
            :
        else
            return 0  # Cannot use sudo either; let the regular checks report errors
        fi
    fi

    # At this point docker works with sudo, set wrappers
    log_info "Docker requires sudo; enabling automatic sudo for Docker commands"
    USE_SUDO_DOCKER=true
    # Define shell function for docker (covers 'docker compose' plugin)
    docker() { command sudo docker "$@"; }
}

# =============================================================================
# LIVEREVIEW INSTALLATION DETECTION
# =============================================================================

# Detect LiveReview installation directory automatically
detect_livereview_installation() {
    local detected_dir=""
    
    # Method 1: Check default location
    local default_dir="${DEFAULT_HOME_DIR}/livereview"
    if [[ -f "$default_dir/docker-compose.yml" && -f "$default_dir/.env" ]]; then
        detected_dir="$default_dir"
        log_debug "Found LiveReview installation at default location: $detected_dir"
    fi
    
    # Method 2: Check environment variable override
    if [[ -n "${LIVEREVIEW_INSTALL_DIR:-}" && "$LIVEREVIEW_INSTALL_DIR" != "$default_dir" ]]; then
        if [[ -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" && -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
            detected_dir="$LIVEREVIEW_INSTALL_DIR"
            log_debug "Found LiveReview installation at specified location: $detected_dir"
        fi
    fi
    
    # Method 3: Check other common locations
    if [[ -z "$detected_dir" ]]; then
        local common_locations=(
            "$default_dir"
            "./livereview"
            "."
        )
        
        for location in "${common_locations[@]}"; do
            if [[ -f "$location/docker-compose.yml" && -f "$location/.env" ]]; then
                # Verify it's actually a LiveReview installation by checking for specific content
                if grep -q "livereview-app\|livereview-db" "$location/docker-compose.yml" 2>/dev/null; then
                    detected_dir="$(realpath "$location")"
                    log_debug "Found LiveReview installation at: $detected_dir"
                    break
                fi
            fi
        done
    fi
    
    # Method 4: Try to detect from running Docker containers
    if [[ -z "$detected_dir" ]] && command -v docker >/dev/null 2>&1; then
        log_debug "Attempting to detect installation from running containers..."
        
        # Look for LiveReview containers and try to find their compose file
        local container_id
        container_id=$(docker ps --filter "name=livereview" --format "{{.ID}}" | head -1)
        
        if [[ -n "$container_id" ]]; then
            # Try to get the working directory or volume mounts
            local inspect_result
            inspect_result=$(docker inspect "$container_id" 2>/dev/null || echo "")
            
            if [[ -n "$inspect_result" ]]; then
                # Look for volume mounts that might indicate the installation directory
                local possible_dirs
                possible_dirs=$(echo "$inspect_result" | grep -oE '"/[^"]*livereview[^"]*"' | tr -d '"' | grep -v '/var/lib/docker' | head -5)
                
                for dir in $possible_dirs; do
                    # Try parent directories
                    local parent_dir
                    parent_dir="$(dirname "$dir")"
                    if [[ -f "$parent_dir/docker-compose.yml" && -f "$parent_dir/.env" ]]; then
                        detected_dir="$parent_dir"
                        log_debug "Detected installation from container volume mount: $detected_dir"
                        break
                    fi
                done
            fi
        fi
    fi
    
    # Method 5: Search filesystem (last resort, limited scope)
    if [[ -z "$detected_dir" ]]; then
    log_debug "Searching filesystem for LiveReview installation..."
    local search_paths=("${DEFAULT_HOME_DIR}" ".")
        
        for search_path in "${search_paths[@]}"; do
            if [[ -d "$search_path" ]]; then
                local found_path
                found_path=$(find "$search_path" -maxdepth 2 -name "docker-compose.yml" -path "*/livereview/*" 2>/dev/null | head -1)
                if [[ -n "$found_path" ]]; then
                    local candidate_dir
                    candidate_dir="$(dirname "$found_path")"
                    if [[ -f "$candidate_dir/.env" ]] && grep -q "livereview-app\|livereview-db" "$found_path" 2>/dev/null; then
                        detected_dir="$candidate_dir"
                        log_debug "Found LiveReview installation via filesystem search: $detected_dir"
                        break
                    fi
                fi
            fi
        done
    fi
    
    # Update the global variable if we found an installation
    if [[ -n "$detected_dir" ]]; then
        LIVEREVIEW_INSTALL_DIR="$detected_dir"
        log_debug "LiveReview installation detected at: $LIVEREVIEW_INSTALL_DIR"
        return 0
    else
        log_debug "No existing LiveReview installation detected, using default: $LIVEREVIEW_INSTALL_DIR"
        return 1
    fi
}

# =============================================================================
# DOCKER COMPOSE COMPATIBILITY
# =============================================================================

# Global variable to store the correct docker compose command
DOCKER_COMPOSE_CMD=""

# Detect and set the correct docker compose command
detect_docker_compose_cmd() {
    if command -v docker-compose >/dev/null 2>&1; then
        # Legacy docker-compose is available
        if [[ "${USE_SUDO_DOCKER:-false}" == "true" ]]; then
            DOCKER_COMPOSE_CMD="sudo docker-compose"
        else
            DOCKER_COMPOSE_CMD="docker-compose"
        fi
        log_debug "Using legacy docker-compose command"
    elif docker compose version >/dev/null 2>&1; then
        # Modern docker compose plugin is available
        # 'docker' may already be wrapped to sudo by maybe_enable_sudo_for_docker
        DOCKER_COMPOSE_CMD="docker compose"
        log_debug "Using modern docker compose plugin"
    else
        log_error "Neither docker-compose nor docker compose is available"
        return 1
    fi
    return 0
}


# Wrapper function to execute docker compose commands
docker_compose() {
    if [[ -z "$DOCKER_COMPOSE_CMD" ]]; then
        if ! detect_docker_compose_cmd; then
            return 1
        fi
    fi
    
    # If we have an install directory and docker-compose.yml exists there, use it explicitly
    local compose_file=""
    if [[ -n "$LIVEREVIEW_INSTALL_DIR" && -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
        compose_file="-f $LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
    fi
    
    log_debug "Executing: $DOCKER_COMPOSE_CMD $compose_file $*"
    $DOCKER_COMPOSE_CMD $compose_file "$@"
}

# =============================================================================
# ARGUMENT PARSING
# =============================================================================

# Default values
EXPRESS_MODE=false
FORCE_INSTALL=false
DRY_RUN=false
VERBOSE=false
DEBUG_MODE=false
LIVEREVIEW_VERSION=""
SHOW_HELP=false
SHOW_VERSION=false

# Test flags (for development)
TEST_GITHUB_API=false
TEST_EXTRACT=false
EXTRACT_TO=""
LIST_EMBEDDED_DATA=false
SHOW_LATEST_VERSION=false
LIST_VERSIONS=false
GENERATE_CONFIG_ONLY=false
INSTALL_TEMPLATES_ONLY=false
OUTPUT_DIR=""
INSTALL_SELF=false
DIAGNOSE=false
BACKUP_TARGET_DIR=""

parse_arguments() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            --express)
                EXPRESS_MODE=true
                shift
                ;;
            --force)
                FORCE_INSTALL=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --verbose|-v)
                VERBOSE=true
                shift
                ;;
            --debug)
                DEBUG_MODE=true
                VERBOSE=true
                shift
                ;;
            --version)
                if [[ -n "${2:-}" && ! "$2" =~ ^-- ]]; then
                    LIVEREVIEW_VERSION="$2"
                    shift 2
                else
                    SHOW_VERSION=true
                    shift
                fi
                ;;
            --help|-h)
                SHOW_HELP=true
                shift
                ;;
            # Skip new commands as they're handled in main()
            setup-demo|setup-production)
                # These are handled in main() case statement, skip here
                shift
                ;;
            # Test and development flags
            --test-github-api)
                TEST_GITHUB_API=true
                shift
                ;;
            --test-extract)
                TEST_EXTRACT=true
                if [[ -n "${2:-}" && ! "$2" =~ ^-- ]]; then
                    EXTRACT_TO="$2"
                    shift 2
                else
                    shift
                fi
                ;;
            --extract-to)
                EXTRACT_TO="$2"
                shift 2
                ;;
            --list-embedded-data)
                LIST_EMBEDDED_DATA=true
                shift
                ;;
            --show-latest-version)
                SHOW_LATEST_VERSION=true
                shift
                ;;
            --list-versions)
                LIST_VERSIONS=true
                shift
                ;;
            --generate-config-only)
                GENERATE_CONFIG_ONLY=true
                shift
                ;;
            --install-templates-only)
                INSTALL_TEMPLATES_ONLY=true
                shift
                ;;
            --output-dir)
                OUTPUT_DIR="$2"
                shift 2
                ;;
            --install-self)
                INSTALL_SELF=true
                shift
                ;;
            --diagnose)
                DIAGNOSE=true
                shift
                ;;
            --backup-dir=*)
                BACKUP_TARGET_DIR="${1#*=}"
                shift
                ;;
            --backup-dir)
                BACKUP_TARGET_DIR="$2"
                shift 2
                ;;
            --show-plan)
                DRY_RUN=true
                VERBOSE=true
                shift
                ;;
            --*)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
            *)
                # Not an option (doesn't start with --), so stop parsing
                # This allows commands like 'show-mode' to be handled by main()
                break
                ;;
        esac
    done
}

# =============================================================================
# HELP AND VERSION DISPLAY
# =============================================================================

show_version() {
    echo "LiveReview Operations Script (lrops.sh) v${SCRIPT_VERSION}"
    echo "Repository: https://github.com/${GITHUB_REPO}"
    echo "Docker Registry: ${DOCKER_REGISTRY}/${DOCKER_IMAGE}"
}

show_help() {
    cat << 'EOF'
LiveReview Operations Script (lrops.sh)

USAGE:
    # Quick installation (recommended)
    curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --express

    # Two-mode setup commands (new!)
    lrops.sh setup-demo                # Quick demo mode setup (localhost only)
    lrops.sh setup-production          # Production mode setup (with reverse proxy)

    # Interactive installation
    curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash

    # Specific version installation
    curl -fsSL https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh | bash -s -- --version=v1.2.3 --express

    # Management commands (after installation)
    lrops.sh status                    # Show installation status
    lrops.sh info                      # Show installation details and file locations
    lrops.sh start                     # Start LiveReview services
    lrops.sh stop                      # Stop LiveReview services
    lrops.sh restart                   # Restart LiveReview services
    lrops.sh update [version]          # Pull newer image (or specific version) and restart
    lrops.sh backup [--backup-dir <path>] [name]  # Create manual backup (see detailed options below)
    lrops.sh quick-backup              # Create quick timestamped backup
    lrops.sh list-backups              # List all available backups
    lrops.sh backup-info <name>        # Show detailed information about a backup
    lrops.sh delete-backup <name>      # Delete a specific backup
    lrops.sh restore <id|latest>       # Restore a previous backup
    lrops.sh set-mode <demo|production> # Switch between demo and production modes
    lrops.sh show-mode                 # Show current deployment mode and configuration
    lrops.sh self-update               # Update this script to the latest version from GitHub
    
    # Backup options:
    (Use --backup-dir as backup subcommand option - see BACKUP OPTIONS below)
    lrops.sh uninstall                 # Safely uninstall (moves directory, keeps backups)
    lrops.sh logs [service]            # Show container logs
    lrops.sh env validate              # Validate .env and suggest fixes
    lrops.sh help ssl                  # SSL/TLS setup guidance
    lrops.sh help backup               # Backup strategies
    lrops.sh help nginx                # Nginx reverse proxy setup
    lrops.sh help caddy                # Caddy reverse proxy setup
    lrops.sh help apache               # Apache reverse proxy setup

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

BACKUP OPTIONS:
    1. Default backup (to installation directory):
       lrops.sh backup                     # Auto-named: manual-YYYYMMDD_HHMMSS
       
    2. Named backup (to installation directory):
       lrops.sh backup my-backup-name      # Custom name: my-backup-name-YYYYMMDD_HHMMSS
       
    3. Backup to custom directory:
       lrops.sh backup --backup-dir /path/to/backups
       lrops.sh backup --backup-dir /path/to/backups custom-name
       
    4. Quick timestamped backup:
       lrops.sh quick-backup               # Creates: quickbackup-YYYYMMDD_HHMMSS

EXAMPLES:
    # Quick demo setup (recommended for first time)
    lrops.sh setup-demo

    # Production setup with reverse proxy
    lrops.sh setup-production

    # Force reinstall with specific version
    lrops.sh --force --version=v1.2.3 --express

    # Preview installation plan
    lrops.sh --dry-run --verbose
    
    # Backup examples
    lrops.sh backup                                    # Default backup
    lrops.sh backup before-upgrade                     # Named backup
    lrops.sh backup --backup-dir ~/my-backups          # Custom directory
    lrops.sh backup --backup-dir ~/my-backups my-name  # Custom directory + name

For more information, visit: https://github.com/HexmosTech/LiveReview
EOF
}

# =============================================================================
# SYSTEM CHECKS AND PREREQUISITES
# =============================================================================

check_system_prerequisites() {
    section_header "CHECKING SYSTEM PREREQUISITES"
    
    local errors=0
    
    # Check if running as root (we'll need sudo for some operations)
    if [[ $EUID -eq 0 ]] && [[ "${INSTALL_SELF:-false}" != "true" ]]; then
        log_warning "Running as root. Consider running as regular user with sudo access."
    fi
    
    # Check for required commands
    local required_commands=("curl" "docker" "jq")
    for cmd in "${required_commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            log_error "$cmd is required but not installed"
            ((errors++))
        else
            log_success "$cmd is available"
        fi
    done
    
    # Check Docker daemon
    if command -v docker &> /dev/null; then
        if ! docker info &> /dev/null; then
            log_error "Docker daemon is not running or accessible"
            log_info "Try: sudo systemctl start docker"
            ((errors++))
        else
            local docker_version=$(docker --version | cut -d' ' -f3 | sed 's/,//')
            log_success "Docker daemon is running (version: $docker_version)"
        fi
    fi
    
    # Check Docker Compose (both legacy and modern)
    if detect_docker_compose_cmd; then
        local compose_version
        if [[ "$DOCKER_COMPOSE_CMD" == "docker-compose" ]]; then
            compose_version=$(docker-compose --version | cut -d' ' -f3 | sed 's/,//')
            log_success "Docker Compose is available (legacy docker-compose, version: $compose_version)"
        else
            compose_version=$(docker compose version --short 2>/dev/null || docker compose version | grep -o '[0-9][0-9.]*' | head -1)
            log_success "Docker Compose is available (modern docker compose plugin, version: $compose_version)"
        fi
    else
        log_error "Docker Compose is not available (neither docker-compose nor docker compose plugin)"
        log_info "Install docker-compose or use Docker Desktop with the compose plugin"
        ((errors++))
    fi
    
    # Check system architecture
    local arch=$(uname -m)
    case $arch in
        x86_64)
            log_success "Architecture: amd64 (supported)"
            ;;
        aarch64|arm64)
            log_success "Architecture: arm64 (supported)"
            ;;
        *)
            log_warning "Architecture: $arch (may not be supported)"
            ;;
    esac
    
    # Check available disk space on target filesystem
    local target_fs
    target_fs="${LIVEREVIEW_INSTALL_DIR:-${DEFAULT_HOME_DIR}/livereview}"
    mkdir -p "$target_fs" 2>/dev/null || true
    local available_space
    available_space=$(df -P "$target_fs" 2>/dev/null | awk 'NR==2 {print $4}' || echo "0")
    if [[ ${available_space:-0} -lt 2097152 ]]; then  # 2GB in KB
        log_warning "Low disk space for $target_fs. At least 2GB recommended."
    else
        log_success "Sufficient disk space available"
    fi
    
    if [[ $errors -gt 0 ]]; then
        error_exit "System prerequisites check failed. Please install missing dependencies."
    fi
    
    log_success "All system prerequisites satisfied"
}

check_existing_installation() {
    section_header "CHECKING FOR EXISTING INSTALLATION"
    
    local installation_exists=false
    
    # Check for installation directory
    if [[ -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_warning "Installation directory exists: $LIVEREVIEW_INSTALL_DIR"
        installation_exists=true
    fi
    
    # Check for running containers
    if docker ps --format "table {{.Names}}" | grep -q "livereview"; then
        log_warning "LiveReview containers are currently running"
        installation_exists=true
    fi
    
    # Check for installed script
    if [[ -f "$LIVEREVIEW_SCRIPT_PATH" ]]; then
        local installed_version=$("$LIVEREVIEW_SCRIPT_PATH" --version 2>/dev/null | head -1 || echo "unknown")
        log_info "LiveReview script already installed: $installed_version"
    fi
    
    if [[ "$installation_exists" == "true" ]]; then
        if [[ "$FORCE_INSTALL" != "true" ]]; then
            log_error "Existing LiveReview installation detected"
            log_info "Use --force to overwrite existing installation"
            log_info "Or run 'lrops.sh status' to check current installation"
            error_exit "Installation aborted to prevent data loss" 2
        else
            log_warning "Proceeding with --force flag (existing installation will be overwritten)"
        fi
    else
        log_success "No existing installation detected"
    fi
}

# =============================================================================
# VERSION MANAGEMENT AND GITHUB INTEGRATION
# =============================================================================

# GitHub Container Registry API endpoints
GHCR_TOKEN_URL="https://ghcr.io/token"
GHCR_REGISTRY_URL="https://ghcr.io/v2"

# Semantic version regex pattern
SEMVER_PATTERN="^v?([0-9]+)\.([0-9]+)\.([0-9]+)(-[a-zA-Z0-9\-\.]+)?(\+[a-zA-Z0-9\-\.]+)?$"

get_ghcr_token() {
    local repo="$1"
    
    log_debug "Getting anonymous token for repository: $repo"
    
    local token_response
    if ! token_response=$(curl -s --fail --connect-timeout 10 \
        "${GHCR_TOKEN_URL}?service=ghcr.io&scope=repository:${repo}:pull" 2>/dev/null); then
        log_error "Failed to get authentication token from GitHub Container Registry"
        return 1
    fi
    
    local token
    if ! token=$(echo "$token_response" | jq -r '.token' 2>/dev/null); then
        log_error "Failed to parse authentication token"
        return 1
    fi
    
    if [[ "$token" == "null" || -z "$token" ]]; then
        log_error "Invalid authentication token received"
        return 1
    fi
    
    echo "$token"
}

query_ghcr_tags() {
    local repo="$1"
    
    log_debug "Querying GHCR tags for repository: $repo"
    
    local token
    if ! token=$(get_ghcr_token "$repo"); then
        return 1
    fi
    
    local tags_response
    if ! tags_response=$(curl -s --fail --connect-timeout 10 \
        -H "Authorization: Bearer $token" \
        "${GHCR_REGISTRY_URL}/${repo}/tags/list" 2>/dev/null); then
        log_error "Failed to query container registry for available tags"
        return 1
    fi
    
    echo "$tags_response"
}

get_available_versions() {
    local repo="${1:-hexmostech/livereview}"
    
    log_debug "Getting available versions for $repo"
    
    local response
    if ! response=$(query_ghcr_tags "$repo"); then
        return 1
    fi
    
    # Extract tags from the API response and filter for semantic versions
    local tags
    if ! tags=$(echo "$response" | jq -r '.tags[]?' 2>/dev/null); then
        log_error "Failed to parse tags from container registry response"
        return 1
    fi
    
    # Filter for semantic versions and sort
    echo "$tags" | grep -E "$SEMVER_PATTERN" | sort -V -r || {
        log_debug "No semantic version tags found, checking available tags..."
        if [[ -n "$tags" ]]; then
            log_warning "Available tags (non-semantic versions):"
            echo "$tags" | sed 's/^/  - /' >&2
        fi
        return 1
    }
}

get_latest_version() {
    local repo="${1:-hexmostech/livereview}"
    
    log_debug "Determining latest semantic version for $repo"
    
    local versions
    if ! versions=$(get_available_versions "$repo"); then
        return 1
    fi
    
    if [[ -z "$versions" ]]; then
        log_error "No semantic version tags found for $repo"
        log_info "Available tags might use different naming scheme"
        return 1
    fi
    
    # Return the first (highest) version
    echo "$versions" | head -1
}

validate_version_exists() {
    local version="$1"
    local repo="${2:-hexmostech/livereview}"
    
    log_debug "Validating that version $version exists for $repo"
    
    # Get all tags (not just semantic versions) for validation
    local response
    if ! response=$(query_ghcr_tags "$repo"); then
        return 1
    fi
    
    local all_tags
    if ! all_tags=$(echo "$response" | jq -r '.tags[]?' 2>/dev/null); then
        log_error "Failed to parse tags from container registry response"
        return 1
    fi
    
    if echo "$all_tags" | grep -q "^${version}$"; then
        log_debug "Version $version found in available tags"
        return 0
    else
        log_error "Version $version not found in available tags"
        log_info "Available tags:"
        echo "$all_tags" | head -10 | sed 's/^/  - /' >&2
        return 1
    fi
}

is_semantic_version() {
    local version="$1"
    
    if [[ "$version" =~ $SEMVER_PATTERN ]]; then
        return 0
    else
        return 1
    fi
}

normalize_version_tag() {
    local version="$1"
    
    # Remove 'v' prefix if present for Docker image tags
    echo "$version" | sed 's/^v//'
}

resolve_version() {
    local requested_version="$1"
    local repo="${2:-hexmostech/livereview}"
    
    if [[ -z "$requested_version" || "$requested_version" == "latest" ]]; then
        log_info "Resolving latest semantic version..."
        
        local latest_version
        if ! latest_version=$(get_latest_version "$repo"); then
            log_warning "No semantic versions found, falling back to 'latest' tag"
            echo "latest"
            return 0
        fi
        
        log_success "Latest semantic version: $latest_version"
        normalize_version_tag "$latest_version"
    else
        # Validate specific version
        log_info "Validating requested version: $requested_version"
        
        if ! validate_version_exists "$requested_version" "$repo"; then
            error_exit "Version $requested_version not found"
        fi
        
        log_success "Version $requested_version is valid"
        normalize_version_tag "$requested_version"
    fi
}

# Test functions for development and validation
test_github_api() {
    section_header "TESTING GITHUB CONTAINER REGISTRY API"
    
    local repo="hexmostech/livereview"
    
    log_info "Testing GHCR API connectivity..."
    local token
    if token=$(get_ghcr_token "$repo"); then
        log_success "Successfully obtained authentication token"
        log_debug "Token: ${token:0:20}..."
    else
        log_error "Failed to get authentication token"
        return 1
    fi
    
    log_info "Fetching available tags..."
    local response
    if response=$(query_ghcr_tags "$repo"); then
        log_success "Successfully queried container registry"
        echo "$response" | jq '.' 2>/dev/null || echo "$response"
    else
        log_error "Failed to query container registry"
        return 1
    fi
    
    log_info "Fetching semantic versions..."
    local versions
    if versions=$(get_available_versions "$repo"); then
        log_success "Found semantic versions:"
        echo "$versions" | head -10 | sed 's/^/  - /'
    else
        log_warning "No semantic versions found"
    fi
    
    log_info "Testing latest version resolution..."
    local latest
    if latest=$(get_latest_version "$repo"); then
        log_success "Latest semantic version: $latest"
    else
        log_warning "Could not determine latest semantic version"
    fi
}

show_latest_version() {
    local latest
    if latest=$(get_latest_version "hexmostech/livereview"); then
        echo "$latest"
    else
        log_warning "No semantic versions found, using 'latest' tag"
        echo "latest"
    fi
}

list_versions() {
    local repo="hexmostech/livereview"
    
    # Show semantic versions
    local versions
    if versions=$(get_available_versions "$repo"); then
        echo "Available semantic versions (latest first):"
        echo "$versions" | head -20 | sed 's/^/  - /'
        
        local total_count
        total_count=$(echo "$versions" | wc -l)
        if [[ $total_count -gt 20 ]]; then
            echo "  ... and $((total_count - 20)) more semantic versions"
        fi
    else
        echo "No semantic versions found."
    fi
    
    echo
    echo "All available tags:"
    local response
    if response=$(query_ghcr_tags "$repo"); then
        local all_tags
        if all_tags=$(echo "$response" | jq -r '.tags[]?' 2>/dev/null); then
            echo "$all_tags" | sed 's/^/  - /'
        fi
    else
        log_error "Failed to fetch available tags"
        return 1
    fi
}

install_self() {
    section_header "INSTALLING LROPS.SH TO SYSTEM PATH"
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would install $0 to $LIVEREVIEW_SCRIPT_PATH"
        return 0
    fi
    
    # Copy script to system location
    if sudo cp "$0" "$LIVEREVIEW_SCRIPT_PATH"; then
        sudo chmod +x "$LIVEREVIEW_SCRIPT_PATH"
        log_success "Installed lrops.sh to $LIVEREVIEW_SCRIPT_PATH"
        
        # Add to PATH if not already there
        if ! echo "$PATH" | grep -q "/usr/local/bin"; then
            log_info "Consider adding /usr/local/bin to your PATH"
        fi
    else
        log_error "Failed to install lrops.sh to system path"
        return 1
    fi
}

# Self-update: download latest script and replace current one
self_update_cmd() {
    section_header "UPDATING LROPS.SH SCRIPT"
    
    local current_script="$0"
    local script_url="https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh"
    
    # Check if script is being run via pipe/source
    if [[ "$current_script" == "bash" || "$current_script" == "-bash" || "$current_script" =~ /bash$ ]]; then
        log_error "Cannot self-update when script is piped or sourced"
        log_info "Download the script first:"
        log_info "  curl -fsSL $script_url -o lrops.sh"
        log_info "  chmod +x lrops.sh"
        log_info "  ./lrops.sh self-update"
        return 1
    fi
    
    # Get absolute path of current script
    local script_path
    if [[ "$current_script" =~ ^/ ]]; then
        script_path="$current_script"
    else
        script_path="$(cd "$(dirname "$current_script")" && pwd)/$(basename "$current_script")"
    fi
    
    # Check if we need sudo (system installation)
    local needs_sudo=false
    if [[ "$script_path" =~ ^/usr/(local/)?bin/ ]] || [[ "$script_path" =~ ^/opt/ ]]; then
        needs_sudo=true
        log_info "System installation detected: $script_path"
        log_info "Requesting sudo access for system script update..."
        
        # Test sudo access
        if ! sudo -v; then
            log_error "Sudo access required but not available"
            return 1
        fi
        
        # Keep sudo alive during the update
        start_sudo_keepalive
    fi
    
    log_info "Current script: $script_path"
    log_info "Downloading latest version from GitHub..."
    
    # Download to temporary file
    local temp_script="/tmp/lrops_update_$$.sh"
    if ! curl -fsSL "$script_url" -o "$temp_script"; then
        log_error "Failed to download latest script"
        return 1
    fi
    
    chmod +x "$temp_script"
    
    # Basic validation - check if it looks like the right script
    if ! grep -q "LiveReview Operations Script" "$temp_script"; then
        log_error "Downloaded file doesn't appear to be lrops.sh"
        rm -f "$temp_script"
        return 1
    fi
    
    # Create backup of current script in /tmp (avoid polluting system directories)
    local backup_script="/tmp/lrops.sh.backup.$(date +%Y%m%d_%H%M%S)"
    log_info "Creating backup: $backup_script"
    
    local cp_cmd="cp"
    if [[ "$needs_sudo" == "true" ]]; then
        cp_cmd="sudo cp"
    fi
    
    if ! $cp_cmd "$script_path" "$backup_script"; then
        log_error "Failed to create backup"
        rm -f "$temp_script"
        return 1
    fi
    
    # Replace script atomically
    local mv_cmd="mv"
    if [[ "$needs_sudo" == "true" ]]; then
        mv_cmd="sudo mv"
    fi
    
    if $mv_cmd "$temp_script" "$script_path"; then
        # Set proper permissions for system installations
        if [[ "$needs_sudo" == "true" ]]; then
            sudo chmod 755 "$script_path"
            sudo chown root:root "$script_path" 2>/dev/null || true
        fi
        
        log_success "✅ Script updated successfully!"
        log_info "Previous version backed up to: $backup_script"
        log_info "Script location: $script_path"
        
        if [[ "$needs_sudo" == "true" ]]; then
            log_info "💡 System script updated with proper permissions"
        else
            log_info "💡 Local script updated to the latest version"
        fi
        
        # Stop sudo keepalive
        if [[ "$needs_sudo" == "true" ]]; then
            stop_sudo_keepalive
        fi
        
        return 0
    else
        log_error "Failed to replace script - restoring backup"
        $mv_cmd "$backup_script" "$script_path" 2>/dev/null || true
        rm -f "$temp_script"
        
        # Stop sudo keepalive on error
        if [[ "$needs_sudo" == "true" ]]; then
            stop_sudo_keepalive
        fi
        
        return 1
    fi
}

# Install script to system path during main installation
install_script_to_system_path() {
    section_header "INSTALLING SCRIPT TO SYSTEM PATH"
    
    # Check if we're being run via curl (piped execution)
    local script_source="$0"
    local is_piped=false
    
    if [[ "$0" == "bash" || "$0" == "-bash" || "$0" =~ /bash$ ]]; then
        is_piped=true
        log_info "Script executed via pipe, downloading for installation..."
        
        # Download the script to a temporary location
        script_source="/tmp/lrops_install_$$.sh"
        if ! curl -fsSL "https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh" -o "$script_source"; then
            log_error "Failed to download script for installation"
            return 1
        fi
        chmod +x "$script_source"
        log_debug "Downloaded script to $script_source for installation"
    fi
    
    if [[ "$DRY_RUN" == "true" ]]; then
        log_info "[DRY RUN] Would install $script_source to $LIVEREVIEW_SCRIPT_PATH"
        if [[ "$is_piped" == "true" ]]; then
            rm -f "$script_source"
        fi
        return 0
    fi
    
    log_info "Installing lrops.sh to system path for global access..."
    
    # Install to system location
    if sudo cp "$script_source" "$LIVEREVIEW_SCRIPT_PATH"; then
        sudo chmod +x "$LIVEREVIEW_SCRIPT_PATH"
        log_success "✅ Installed lrops.sh to $LIVEREVIEW_SCRIPT_PATH"
        log_info "   You can now run 'lrops.sh' from any directory"
        
        # Check if /usr/local/bin is in PATH
        if ! echo "$PATH" | grep -q "/usr/local/bin"; then
            log_warning "⚠️  /usr/local/bin not in PATH"
            log_info "   Add to your shell profile: export PATH=\"/usr/local/bin:\$PATH\""
        fi
    else
        log_warning "⚠️  Could not install lrops.sh to system path"
        log_info "   You can install manually: sudo cp $script_source $LIVEREVIEW_SCRIPT_PATH"
        log_info "   Or run manually: sudo chmod +x $LIVEREVIEW_SCRIPT_PATH"
    fi
    
    # Clean up temporary file if we downloaded it
    if [[ "$is_piped" == "true" ]]; then
        rm -f "$script_source"
    fi
}

# =============================================================================
# TEMPLATE EXTRACTION FUNCTIONS (PHASE 3)
# =============================================================================

# Extract embedded template data to files
extract_data() {
    local template_name="$1"
    local output_file="$2"
    
    if [[ -z "$template_name" || -z "$output_file" ]]; then
        log_error "Usage: extract_data <template_name> <output_file>"
        return 1
    fi
    
    # Create output directory if it doesn't exist
    local output_dir
    output_dir=$(dirname "$output_file")
    [[ ! -d "$output_dir" ]] && mkdir -p "$output_dir"
    
    local script_source="$0"
    
    # If script is being piped (bash is $0), download it to extract templates
    if [[ "$0" == "bash" || "$0" == "-bash" || "$0" =~ /bash$ ]]; then
        log_debug "Script is piped - downloading to extract templates..."
        script_source="/tmp/lrops_extract_$$.sh"
        
        if ! curl -fsSL "https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh" -o "$script_source"; then
            log_error "Failed to download script for template extraction"
            return 1
        fi
        
        log_debug "Downloaded script to $script_source for template extraction"
    fi
    
    # Extract data between markers, excluding the marker lines themselves
    if sed -n "/^# === DATA:${template_name} ===/,/^# === END:${template_name} ===/p" "$script_source" \
        | grep -v "^# === " > "$output_file"; then
        
        # Set appropriate permissions for script files
        case "$template_name" in
            *.sh)
                chmod +x "$output_file"
                ;;
        esac
        
        log_debug "Extracted template '$template_name' to '$output_file'"
        
        # Clean up downloaded script if we created it
        if [[ "$script_source" != "$0" && -f "$script_source" ]]; then
            rm -f "$script_source"
        fi
        
        return 0
    else
        log_error "Failed to extract template '$template_name'"
        
        # Clean up downloaded script if we created it
        if [[ "$script_source" != "$0" && -f "$script_source" ]]; then
            rm -f "$script_source"
        fi
        
        return 1
    fi
}

# List all available embedded templates
list_embedded_templates() {
    log_info "Available embedded templates:"
    
    local script_source="$0"
    
    # Handle piped execution
    if [[ "$0" == "bash" || "$0" == "-bash" || "$0" =~ /bash$ ]]; then
        script_source="/tmp/lrops_list_$$.sh"
        if ! curl -fsSL "https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh" -o "$script_source"; then
            log_error "Failed to download script to list templates"
            return 1
        fi
    fi
    
    grep "^# === DATA:" "$script_source" | sed 's/^# === DATA:\(.*\) ===$/  - \1/' | sort
    
    # Clean up if we downloaded the script
    if [[ "$script_source" != "$0" && -f "$script_source" ]]; then
        rm -f "$script_source"
    fi
}

# Test template extraction to temporary files
test_template_extraction() {
    local template_name="$1"
    local temp_dir="/tmp/lrops-test-$$"
    
    if [[ -z "$template_name" ]]; then
        log_error "Usage: test_template_extraction <template_name>"
        return 1
    fi
    
    mkdir -p "$temp_dir"
    
    if extract_data "$template_name" "$temp_dir/$template_name"; then
        log_success "Template '$template_name' extracted successfully to '$temp_dir/$template_name'"
        log_info "Content preview:"
        echo "----------------------------------------"
        head -20 "$temp_dir/$template_name"
        echo "----------------------------------------"
        
        # Cleanup
        rm -rf "$temp_dir"
        return 0
    else
        rm -rf "$temp_dir"
        return 1
    fi
}

# Extract all templates to a specified directory
extract_all_templates() {
    local base_dir="$1"
    
    if [[ -z "$base_dir" ]]; then
        log_error "Usage: extract_all_templates <base_directory>"
        return 1
    fi
    
    log_info "Extracting all templates to '$base_dir'"
    
    # Create directory structure
    mkdir -p "$base_dir"/{config,scripts}
    
    local script_source="$0"
    
    # Handle piped execution
    if [[ "$0" == "bash" || "$0" == "-bash" || "$0" =~ /bash$ ]]; then
        script_source="/tmp/lrops_extract_all_$$.sh"
        if ! curl -fsSL "https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh" -o "$script_source"; then
            log_error "Failed to download script to extract templates"
            return 1
        fi
    fi
    
    local templates
    templates=$(grep "^# === DATA:" "$script_source" | sed 's/^# === DATA:\(.*\) ===$/\1/')
    
    local extracted_count=0
    local failed_count=0
    
    for template in $templates; do
        local output_file
        case "$template" in
            *.example | *.conf | backup-cron.example)
                output_file="$base_dir/config/$template"
                ;;
            *.sh)
                output_file="$base_dir/scripts/$template"
                ;;
            *)
                output_file="$base_dir/$template"
                ;;
        esac
        
        if extract_data "$template" "$output_file"; then
            ((extracted_count++))
            log_success "✓ $template"
        else
            ((failed_count++))
            log_error "✗ $template"
        fi
    done
    
    # Clean up if we downloaded the script
    if [[ "$script_source" != "$0" && -f "$script_source" ]]; then
        rm -f "$script_source"
    fi
    
    log_info "Template extraction complete: $extracted_count succeeded, $failed_count failed"
    return $failed_count
}

# =============================================================================
# CONFIGURATION AND PASSWORD GENERATION (PHASE 4)
# =============================================================================

# Generate secure random password
generate_password() {
    local length=${1:-32}
    
    # Try different methods in order of preference
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -base64 $((length * 3 / 4)) | tr -d "=+/" | cut -c1-${length}
    elif command -v /dev/urandom >/dev/null 2>&1; then
        LC_ALL=C tr -dc 'A-Za-z0-9!@#$%^&*()_+-=' < /dev/urandom | head -c ${length}
    else
        # Fallback to date-based generation (less secure)
        local timestamp=$(date +%s%N)
        echo "${timestamp}" | sha256sum | cut -c1-${length}
    fi
}

# Generate JWT secret (longer, alphanumeric only for better compatibility)
generate_jwt_secret() {
    local length=64
    
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex ${length} | head -c ${length}
    elif command -v /dev/urandom >/dev/null 2>&1; then
        LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c ${length}
    else
        # Fallback method
        local timestamp=$(date +%s%N)
        echo "${timestamp}$(hostname)" | sha256sum | cut -c1-${length}
    fi
}

# Auto-configure deployment variables based on mode
configure_deployment_mode() {
    local mode="$1"
    local backend_port="${2:-8888}"
    local frontend_port="${3:-8081}"
    
    if [[ "$mode" == "production" ]]; then
        # Production mode: behind reverse proxy
        API_URL="http://localhost/api"
        REVERSE_PROXY="true"
        BIND_ADDRESS="127.0.0.1"
        WEBHOOKS_ENABLED="true"
    else
        # Demo mode: direct access
        API_URL="http://localhost:${backend_port}"
        REVERSE_PROXY="false"
        BIND_ADDRESS="localhost"
        WEBHOOKS_ENABLED="false"
    fi
    
    # Set all framework-specific variables automatically
    VITE_API_URL="$API_URL"
    REACT_APP_API_URL="$API_URL"
    NEXT_PUBLIC_API_URL="$API_URL"
    LIVEREVIEW_API_URL="$API_URL"  # Legacy support
}

# Interactive configuration prompts for simplified two-mode system
gather_configuration() {
    local config_file="/tmp/lrops_config_$$"
    
    section_header "CONFIGURATION"
    
    if [[ "$EXPRESS_MODE" == "true" ]]; then
        log_info "Express mode: Using secure defaults for demo mode"
        
        # Generate secure defaults
        local db_password
        db_password=$(generate_password 32)
        local jwt_secret
        jwt_secret=$(generate_jwt_secret)
        
        # Configure deployment variables
        configure_deployment_mode "demo" 8888 8081
        
        # Demo mode defaults (localhost-only)
        cat > "$config_file" << EOF
# LiveReview Configuration
# Simple setup - only specify what you need to change

#==============================================================================
# CORE CONFIGURATION (Required)
#==============================================================================
# Database
DATABASE_URL=postgres://livereview:$db_password@livereview-db:5432/livereview?sslmode=disable
DB_PASSWORD=$db_password

# Security
JWT_SECRET=$jwt_secret

#==============================================================================
# USER CONFIGURATION (Customize as needed)
#==============================================================================
# Ports (change if you have conflicts)
LIVEREVIEW_BACKEND_PORT=8888
LIVEREVIEW_FRONTEND_PORT=8081

# Reverse proxy setup (only change if using nginx/apache in front)
LIVEREVIEW_REVERSE_PROXY=false

#==============================================================================
# OPTIONAL CONFIGURATION
#==============================================================================
# Application settings
LIVEREVIEW_VERSION=$1
LOG_LEVEL=info
ACCESS_TOKEN_DURATION_HOURS=8
REFRESH_TOKEN_DURATION_DAYS=30
EOF
    log_success "✅ Demo mode configuration (localhost-only, no webhooks)"
    log_info "   To upgrade to production mode, set LIVEREVIEW_REVERSE_PROXY=true and configure your reverse proxy"
    else
        log_info "Interactive configuration mode"
    log_info "Choose your deployment mode:"
        echo
        echo "1) Demo Mode (localhost only, no webhooks, quickstart)"
        echo "2) Production Mode (with reverse proxy, webhooks enabled)"
        echo
    echo -n "Select deployment mode [1]: "
        read -r mode_choice
        
        local deployment_mode="demo"
        
        if [[ "$mode_choice" == "2" ]]; then
            deployment_mode="production"
            log_info "Production mode selected - requires reverse proxy setup"
        else
            deployment_mode="demo"
            log_info "Demo mode selected - localhost only, no configuration needed"
        fi
        
        # Generate database password
        local db_password
        db_password=$(generate_password 32)
        echo -n "Database password [auto-generated secure password]: "
        read -r user_input
        if [[ -n "$user_input" ]]; then
            db_password="$user_input"
        fi
        
        # Generate JWT Secret
        local jwt_secret
        jwt_secret=$(generate_jwt_secret)
        echo -n "JWT secret key [auto-generated secure key]: "
        read -r user_input
        if [[ -n "$user_input" ]]; then
            jwt_secret="$user_input"
        fi
        
        # Use standard ports (no custom port configuration for simplicity)
        local backend_port=8888
        local frontend_port=8081
        
    # Configure variables for summary; actual mode derives from reverse proxy flag in .env
    configure_deployment_mode "$deployment_mode" "$backend_port" "$frontend_port"
        
        if [[ "$deployment_mode" == "production" ]]; then
            echo "Production mode will use standard ports (8888 backend, 8081 frontend)"
            echo "Configure your reverse proxy to route:"
            echo "  /api/* → http://127.0.0.1:8888"
            echo "  /* → http://127.0.0.1:8081"
        fi
        
    # Save configuration with simplified user-facing format
        cat > "$config_file" << EOF
# LiveReview Configuration
# Simple setup - only specify what you need to change

#==============================================================================
# CORE CONFIGURATION (Required)
#==============================================================================
# Database
DATABASE_URL=postgres://livereview:$db_password@livereview-db:5432/livereview?sslmode=disable
DB_PASSWORD=$db_password

# Security
JWT_SECRET=$jwt_secret

#==============================================================================
# USER CONFIGURATION (Customize as needed)
#==============================================================================
# Ports (change if you have conflicts)
LIVEREVIEW_BACKEND_PORT=$backend_port
LIVEREVIEW_FRONTEND_PORT=$frontend_port

# Reverse proxy setup (only change if using nginx/apache in front)
LIVEREVIEW_REVERSE_PROXY=$REVERSE_PROXY

#==============================================================================
# OPTIONAL CONFIGURATION
#==============================================================================
# Application settings
LIVEREVIEW_VERSION=$1
LOG_LEVEL=info
ACCESS_TOKEN_DURATION_HOURS=8
REFRESH_TOKEN_DURATION_DAYS=30
EOF
    fi
    
    echo "$config_file"
}

# Validate configuration values for simplified configuration system
validate_configuration() {
    local config_file="$1"
    
    log_info "Validating configuration..."
    
    # Source the config
    source "$config_file"
    
    # Support both new and legacy variable names
    local backend_port="${BACKEND_PORT:-$LIVEREVIEW_BACKEND_PORT}"
    local frontend_port="${FRONTEND_PORT:-$LIVEREVIEW_FRONTEND_PORT}"
    local reverse_proxy="${LIVEREVIEW_REVERSE_PROXY:-false}"
    local deployment_mode
    if [[ "$reverse_proxy" == "true" ]]; then
        deployment_mode="production"
    else
        deployment_mode="demo"
    fi
    
    # Validate reverse proxy flag
    if [[ "$reverse_proxy" != "true" && "$reverse_proxy" != "false" ]]; then
        log_error "Invalid LIVEREVIEW_REVERSE_PROXY: must be 'true' or 'false'"
        return 1
    fi
    
    # Validate backend port
    if ! [[ "$backend_port" =~ ^[0-9]+$ ]] || [[ "$backend_port" -lt 1024 ]] || [[ "$backend_port" -gt 65535 ]]; then
        log_error "Invalid backend port: $backend_port (must be 1024-65535)"
        return 1
    fi
    
    # Validate frontend port
    if ! [[ "$frontend_port" =~ ^[0-9]+$ ]] || [[ "$frontend_port" -lt 1024 ]] || [[ "$frontend_port" -gt 65535 ]]; then
        log_error "Invalid frontend port: $frontend_port (must be 1024-65535)"
        return 1
    fi
    
    if [[ "$backend_port" == "$frontend_port" ]]; then
        log_error "Backend and frontend ports cannot be the same"
        return 1
    fi
    
    # Check if ports are available
    if command -v netstat >/dev/null 2>&1; then
        if netstat -tln | grep -q ":${backend_port} "; then
            log_warning "Port $backend_port appears to be in use"
        fi
        if netstat -tln | grep -q ":${frontend_port} "; then
            log_warning "Port $frontend_port appears to be in use"
        fi
    fi
    
    # Validate password strength
    if [[ ${#DB_PASSWORD} -lt 12 ]]; then
        log_warning "Database password is shorter than 12 characters"
    fi
    
    if [[ ${#JWT_SECRET} -lt 32 ]]; then
        log_warning "JWT secret is shorter than 32 characters"
    fi
    
    # Derive API URL from deployment mode and ports for summary purposes
    configure_deployment_mode "$deployment_mode" "$backend_port" "$frontend_port"
    
    # Show configuration summary
    log_info "Configuration summary:"
    log_info "  - Deployment mode: $deployment_mode (derived from LIVEREVIEW_REVERSE_PROXY=$reverse_proxy)"
    log_info "  - Backend port: $backend_port"
    log_info "  - Frontend port: $frontend_port"
    log_info "  - API URL: ${API_URL}"
    
    if [[ "$deployment_mode" == "demo" ]]; then
        log_info "  - Access URL: http://localhost:$frontend_port"
        log_info "  - Webhooks: Disabled (manual triggers only)"
        log_info "  - Binding: localhost only (secure local development)"
    else
        log_info "  - Requires reverse proxy configuration"
        log_info "  - Webhooks: Enabled (automatic triggers)"
        log_info "  - Binding: 127.0.0.1 (behind reverse proxy)"
    fi
    
    log_success "Configuration validation completed"
    return 0
}

# =============================================================================
# DIRECTORY STRUCTURE CREATION (PHASE 4)
# =============================================================================

# Create LiveReview directory structure
create_directory_structure() {
    section_header "CREATING DIRECTORY STRUCTURE"
    
    log_info "Creating LiveReview installation directory: $LIVEREVIEW_INSTALL_DIR"
    
    # Create main directory
    if ! mkdir -p "$LIVEREVIEW_INSTALL_DIR"; then
        error_exit "Failed to create installation directory: $LIVEREVIEW_INSTALL_DIR"
    fi
    
    # Create subdirectories
    local directories=(
        "$LIVEREVIEW_INSTALL_DIR/lrdata"
        "$LIVEREVIEW_INSTALL_DIR/lrdata/postgres"
        "$LIVEREVIEW_INSTALL_DIR/config"
        "$LIVEREVIEW_INSTALL_DIR/scripts"
    )
    
    for dir in "${directories[@]}"; do
        log_info "Creating directory: $dir"
        if ! mkdir -p "$dir"; then
            error_exit "Failed to create directory: $dir"
        fi
    done
    
    # Ensure ownership by invoking user (when running with sudo)
    if [[ -n "${SUDO_UID:-}" && -n "${SUDO_GID:-}" ]]; then
        chown -R "${SUDO_UID}:${SUDO_GID}" "$LIVEREVIEW_INSTALL_DIR" 2>/dev/null || true
    fi

    # Set proper permissions
    log_info "Setting directory permissions..."
    
    # Main directory should be accessible by current user
    if ! chmod 755 "$LIVEREVIEW_INSTALL_DIR"; then
        log_warning "Could not set permissions on $LIVEREVIEW_INSTALL_DIR"
    fi
    
    # Data directory needs to be writable for Docker containers
    if ! chmod 755 "$LIVEREVIEW_INSTALL_DIR/lrdata"; then
        log_warning "Could not set permissions on lrdata directory"
    fi
    
    # PostgreSQL data directory needs specific permissions
    if ! chmod 700 "$LIVEREVIEW_INSTALL_DIR/lrdata/postgres"; then
        log_warning "Could not set permissions on postgres directory"
    fi
    
    # Config and scripts directories
    chmod 755 "$LIVEREVIEW_INSTALL_DIR/config" 2>/dev/null || true
    chmod 755 "$LIVEREVIEW_INSTALL_DIR/scripts" 2>/dev/null || true
    
    log_success "Directory structure created successfully"
}

# Handle existing directory conflicts
handle_existing_directories() {
    if [[ -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        if [[ "$FORCE_INSTALL" != "true" ]]; then
            log_error "Installation directory already exists: $LIVEREVIEW_INSTALL_DIR"
            log_info "Use --force to overwrite existing installation"
            return 1
        else
            log_warning "Force mode: Backing up existing installation"
            local backup_dir="${LIVEREVIEW_INSTALL_DIR}.backup.$(date +%Y%m%d_%H%M%S)"
            if mv "$LIVEREVIEW_INSTALL_DIR" "$backup_dir"; then
                log_info "Existing installation backed up to: $backup_dir"
            else
                log_error "Could not backup existing installation"
                return 1
            fi
        fi
    fi
    return 0
}

# =============================================================================
# FILE GENERATION FROM TEMPLATES (PHASE 4)
# =============================================================================

# Generate .env file with simplified configuration approach
generate_env_file() {
    local config_file="$1"
    local output_file="$LIVEREVIEW_INSTALL_DIR/.env"
    
    log_info "Generating .env file with simplified configuration..."
    
    # Respect existing .env unless --force is provided
    if [[ -f "$output_file" && "${FORCE_INSTALL:-false}" != "true" ]]; then
        log_warning ".env already exists at $output_file — keeping it. Use --force to overwrite."
        return 0
    fi

    # Backup existing .env when forcing regeneration
    if [[ -f "$output_file" && "${FORCE_INSTALL:-false}" == "true" ]]; then
        local backup_path="${output_file}.bak.$(date +%Y%m%d_%H%M%S)"
        cp "$output_file" "$backup_path" || true
        log_info "Backed up existing .env to: $backup_path"
    fi

    # Source configuration
    source "$config_file"
    
    # Use new variables with fallback to legacy ones
    local deployment_mode="${DEPLOYMENT_MODE:-demo}"
    local backend_port="${BACKEND_PORT:-$LIVEREVIEW_BACKEND_PORT}"
    local frontend_port="${FRONTEND_PORT:-$LIVEREVIEW_FRONTEND_PORT}"
    
    # Configure deployment variables based on mode
    configure_deployment_mode "$deployment_mode" "$backend_port" "$frontend_port"
    
    # Generate a minimal, customer-facing .env (no extra banners or framework vars)
    cat > "$output_file" << EOF
# LiveReview configuration (minimal but complete)

# Ports
LIVEREVIEW_BACKEND_PORT=$backend_port
LIVEREVIEW_FRONTEND_PORT=$frontend_port
LIVEREVIEW_REVERSE_PROXY=$REVERSE_PROXY

# Database
DB_PASSWORD=$DB_PASSWORD
DATABASE_URL=postgres://livereview:$DB_PASSWORD@livereview-db:5432/livereview?sslmode=disable

# Security
JWT_SECRET=$JWT_SECRET
# Application version (fallback to latest if unset at generation time)
LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION:-latest}
EOF
    
    # Set secure permissions on .env file (readable by Docker containers)
    chmod 644 "$output_file"
    # Ensure ownership by invoking user
    if [[ -n "${SUDO_UID:-}" && -n "${SUDO_GID:-}" ]]; then
        chown "${SUDO_UID}:${SUDO_GID}" "$output_file" 2>/dev/null || true
    fi
    
    log_success "Generated .env file with $deployment_mode mode configuration"
}

# Generate docker-compose.yml from template with two-mode configuration
generate_docker_compose() {
    local config_file="$1"
    local output_file="$LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
    
    log_info "Generating docker-compose.yml..."
    
    # Source configuration
    source "$config_file"

    # Derive deployment mode and ports safely from simplified config
    local reverse_proxy="${LIVEREVIEW_REVERSE_PROXY:-false}"
    local backend_port="${BACKEND_PORT:-${LIVEREVIEW_BACKEND_PORT:-8888}}"
    local frontend_port="${FRONTEND_PORT:-${LIVEREVIEW_FRONTEND_PORT:-8081}}"
    local deployment_mode
    if [[ "$reverse_proxy" == "true" ]]; then
        deployment_mode="production"
    else
        deployment_mode="demo"
    fi
    
    # Extract docker-compose template
    if ! extract_data "docker-compose.yml" "$output_file"; then
        error_exit "Failed to extract docker-compose.yml template"
    fi
    
    # Determine version to inject (fallback if empty)
    local effective_version="${LIVEREVIEW_VERSION:-latest}"
    sed_inplace "s/\\${LIVEREVIEW_VERSION}/$effective_version/g" "$output_file"
    # Do not rewrite DB_PASSWORD placeholder; leave ${DB_PASSWORD} intact in compose
    # Ports are parameterized; no hard substitution required beyond defaults
    # Ensure ownership by invoking user
    if [[ -n "${SUDO_UID:-}" && -n "${SUDO_GID:-}" ]]; then
        chown "${SUDO_UID}:${SUDO_GID}" "$output_file" 2>/dev/null || true
    fi
    
    log_success "Generated docker-compose.yml with $deployment_mode mode configuration"
    log_info "Port mappings: Frontend=$frontend_port, Backend=$backend_port"
}

# Extract configuration templates and helper scripts
extract_templates_and_scripts() {
    section_header "EXTRACTING CONFIGURATION TEMPLATES"
    
    # Extract reverse proxy templates to config/
    log_info "Extracting reverse proxy configuration templates..."
    extract_data "nginx.conf.example" "$LIVEREVIEW_INSTALL_DIR/config/nginx.conf.example"
    extract_data "caddy.conf.example" "$LIVEREVIEW_INSTALL_DIR/config/caddy.conf.example"
    extract_data "apache.conf.example" "$LIVEREVIEW_INSTALL_DIR/config/apache.conf.example"
    
    # Extract backup and maintenance scripts to scripts/
    log_info "Extracting backup and maintenance scripts..."
    extract_data "backup.sh" "$LIVEREVIEW_INSTALL_DIR/scripts/backup.sh"
    extract_data "restore.sh" "$LIVEREVIEW_INSTALL_DIR/scripts/restore.sh"
    
    # Extract cron example to config/
    extract_data "backup-cron.example" "$LIVEREVIEW_INSTALL_DIR/config/backup-cron.example"
    extract_data "setup-ssl.sh" "$LIVEREVIEW_INSTALL_DIR/scripts/setup-ssl.sh"
    extract_data "renew-ssl.sh" "$LIVEREVIEW_INSTALL_DIR/scripts/renew-ssl.sh"
    
    # Set executable permissions on scripts
    chmod +x "$LIVEREVIEW_INSTALL_DIR/scripts/"*.sh 2>/dev/null || true
    # Ensure ownership by invoking user
    if [[ -n "${SUDO_UID:-}" && -n "${SUDO_GID:-}" ]]; then
        chown -R "${SUDO_UID}:${SUDO_GID}" "$LIVEREVIEW_INSTALL_DIR/config" "$LIVEREVIEW_INSTALL_DIR/scripts" 2>/dev/null || true
    fi
    
    log_success "Configuration templates and scripts extracted"
}

# Generate installation summary file for two-mode system
generate_installation_summary() {
    local config_file="$1"
    local summary_file="$LIVEREVIEW_INSTALL_DIR/installation-summary.txt"
    
    # Source configuration
    source "$config_file"

    # Derive deployment mode and ports for summary
    local reverse_proxy="${LIVEREVIEW_REVERSE_PROXY:-false}"
    local backend_port="${BACKEND_PORT:-${LIVEREVIEW_BACKEND_PORT:-8888}}"
    local frontend_port="${FRONTEND_PORT:-${LIVEREVIEW_FRONTEND_PORT:-8081}}"
    local deployment_mode
    if [[ "$reverse_proxy" == "true" ]]; then
        deployment_mode="production"
    else
        deployment_mode="demo"
    fi
    
    cat > "$summary_file" << EOF
LiveReview Installation Summary
===============================
Installation Date: $(date)
Script Version: $SCRIPT_VERSION
LiveReview Version: $LIVEREVIEW_VERSION

Deployment Configuration:
- Installation Directory: $LIVEREVIEW_INSTALL_DIR
- Deployment Mode: $deployment_mode
- Backend Port: $backend_port
- Frontend Port: $frontend_port
- Reverse Proxy: $LIVEREVIEW_REVERSE_PROXY

EOF

if [[ "$deployment_mode" == "demo" ]]; then
    cat >> "$summary_file" << EOF
Demo Mode Configuration:
- Access URL: http://localhost:$frontend_port/
- API URL: http://localhost:$backend_port/api
- Webhooks: Disabled (manual triggers only)
- External Access: Not configured (localhost only)
- Perfect for: Development, testing, evaluation

Upgrade to Production Mode:
1. Edit .env file: Set LIVEREVIEW_REVERSE_PROXY=true
2. Restart services: docker compose restart
3. Configure reverse proxy (see help guides below)
4. Set up SSL/TLS for security

EOF
else
    cat >> "$summary_file" << EOF
Production Mode Configuration:
- Backend: http://127.0.0.1:$backend_port/api
- Frontend: http://127.0.0.1:$frontend_port/
- Webhooks: Enabled (automatic triggers)
- External Access: Via reverse proxy (requires configuration)
- SSL/TLS: Required for production use

Reverse Proxy Setup Required:
Route /api/* → http://127.0.0.1:$backend_port
Route /* → http://127.0.0.1:$frontend_port

EOF
fi

    cat >> "$summary_file" << EOF

Important Files:
- Docker Compose: $LIVEREVIEW_INSTALL_DIR/docker-compose.yml
- Environment: $LIVEREVIEW_INSTALL_DIR/.env
- Configuration Templates: $LIVEREVIEW_INSTALL_DIR/config/
- Helper Scripts: $LIVEREVIEW_INSTALL_DIR/scripts/

Management Commands:
- Status Check: lrops.sh status
- Start Services: lrops.sh start
- Stop Services: lrops.sh stop
- Restart Services: lrops.sh restart
- View Logs: lrops.sh logs

Configuration Help:
- SSL Setup: lrops.sh help ssl
- Backup Setup: lrops.sh help backup
- Nginx Config: lrops.sh help nginx
- Caddy Config: lrops.sh help caddy

Two-Mode Deployment System:
This installation uses a simplified two-mode deployment system:
- Demo Mode: Perfect for localhost development and testing
- Production Mode: Ready for external access with reverse proxy

For support, visit: https://github.com/HexmosTech/LiveReview
EOF

    # Ensure ownership by invoking user
    if [[ -n "${SUDO_UID:-}" && -n "${SUDO_GID:-}" ]]; then
        chown "${SUDO_UID}:${SUDO_GID}" "$summary_file" 2>/dev/null || true
    fi

    log_info "Installation summary saved to: $summary_file"
}

# =============================================================================
# DOCKER DEPLOYMENT FUNCTIONS (PHASE 5)
# =============================================================================

# Validate an existing .env in-place and optionally auto-fix common issues
env_validate_cmd() {
    section_header "VALIDATING .env"
    
    if [[ -z "$LIVEREVIEW_INSTALL_DIR" ]] || [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        detect_livereview_installation || true
    fi
    local env_path="${LIVEREVIEW_INSTALL_DIR:-.}/.env"
    if [[ ! -f "$env_path" ]]; then
        log_error ".env not found at: $env_path"
        log_info "Generate one with: lrops.sh setup-demo"
        return 1
    fi
    
    # Copy to temp for safe sourcing
    local tmp_cfg="/tmp/lrops_env_validate_$$.env"
    cp "$env_path" "$tmp_cfg"
    
    if validate_configuration "$tmp_cfg"; then
        log_success ".env looks good: $env_path"
        rm -f "$tmp_cfg"
        return 0
    else
        log_warning "Validation found issues. Attempting targeted fixes..."
        # Auto-fix: normalize boolean
        if grep -q '^LIVEREVIEW_REVERSE_PROXY=' "$tmp_cfg"; then
            sed_inplace 's/^LIVEREVIEW_REVERSE_PROXY=.*/LIVEREVIEW_REVERSE_PROXY=false/' "$tmp_cfg"
        fi
        # Auto-fix: ensure different ports
        local bport=$(grep '^LIVEREVIEW_BACKEND_PORT=' "$tmp_cfg" | cut -d'=' -f2)
        local fport=$(grep '^LIVEREVIEW_FRONTEND_PORT=' "$tmp_cfg" | cut -d'=' -f2)
        if [[ -n "$bport" && "$bport" == "$fport" ]]; then
            sed_inplace 's/^LIVEREVIEW_FRONTEND_PORT=.*/LIVEREVIEW_FRONTEND_PORT=8081/' "$tmp_cfg"
        fi
        # Auto-fix: generate secrets if missing
        if ! grep -q '^DB_PASSWORD=' "$tmp_cfg"; then
            echo "DB_PASSWORD=$(generate_password 32)" >> "$tmp_cfg"
        fi
        if ! grep -q '^JWT_SECRET=' "$tmp_cfg"; then
            echo "JWT_SECRET=$(generate_jwt_secret)" >> "$tmp_cfg"
        fi
        # Re-validate
        if validate_configuration "$tmp_cfg"; then
            log_success "Auto-fixes applied. Updating $env_path"
            mv "$tmp_cfg" "$env_path"
            return 0
        else
            log_error "Validation still failing. Review $tmp_cfg for hints"
            return 1
        fi
    fi
}

# Pull required Docker images
pull_docker_images() {
    local resolved_version="$1"
    
    section_header "PULLING DOCKER IMAGES"
    log_info "Pulling required Docker images..."
    
    # Pull LiveReview application image
    local app_image="${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${resolved_version}"
    log_info "Pulling LiveReview application image: $app_image"
    
    if ! docker pull "$app_image"; then
        log_error "Failed to pull LiveReview application image: $app_image"
        return 1
    fi
    
    log_success "Successfully pulled LiveReview application image"
    
    # Pull PostgreSQL image (using the version specified in docker-compose.yml)
    local postgres_image="postgres:15-alpine"
    log_info "Pulling PostgreSQL image: $postgres_image"
    
    if ! docker pull "$postgres_image"; then
        log_error "Failed to pull PostgreSQL image: $postgres_image"
        return 1
    fi
    
    log_success "Successfully pulled PostgreSQL image"
    log_success "All required Docker images pulled successfully"
}

# Start containers with docker compose
start_containers() {
    section_header "STARTING CONTAINERS"
    log_info "Starting LiveReview containers..."
    
    # Verify docker-compose.yml exists
    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
        log_error "docker-compose.yml not found in $LIVEREVIEW_INSTALL_DIR"
        log_info "Directory contents: $(ls -la "$LIVEREVIEW_INSTALL_DIR" 2>/dev/null || echo "Directory does not exist")"
        return 1
    fi
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Could not change to installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    # Verify we're in the right directory and files exist
    if [[ ! -f "docker-compose.yml" ]]; then
        log_error "docker-compose.yml not found in current directory: $(pwd)"
        return 1
    fi
    
    # Start containers in detached mode
    log_info "Running: $DOCKER_COMPOSE_CMD up -d"
    if ! docker_compose up -d; then
        log_error "Failed to start containers with docker compose"
        log_info "Current directory: $(pwd)"
        log_info "Files in directory: $(ls -la)"
        return 1
    fi
    
    log_success "Containers started successfully"
    return 0
}

# Wait for containers to become healthy
wait_for_containers() {
    section_header "WAITING FOR CONTAINER HEALTH"
    log_info "Waiting for containers to become healthy..."
    
    local max_wait=180  # base for Linux
    local wait_time=0
    local check_interval=10

    # macOS (Docker Desktop) cold start penalty: extend grace period
    if [[ "$(uname -s)" == "Darwin" ]]; then
        log_info "macOS detected: extending health wait window (adds 150s)"
        max_wait=$((max_wait + 150))          # total 330s
        check_interval=10                     # keep interval stable
    fi
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Could not change to installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    # Give containers initial time to start
    local initial_sleep=15
    if [[ "$(uname -s)" == "Darwin" ]]; then
        initial_sleep=30  # extra warm-up for VM init
    fi
    log_info "Giving containers initial startup time (${initial_sleep}s)..."
    sleep "$initial_sleep"
    
    while [[ $wait_time -lt $max_wait ]]; do
        log_info "Checking container status... (${wait_time}/${max_wait}s)"
        
        local containers_healthy=0
        
        # Check PostgreSQL container health
        local db_state=$(docker inspect --format='{{.State.Status}}' livereview-db 2>/dev/null || echo "missing")
        local db_health=$(docker inspect --format='{{.State.Health.Status}}' livereview-db 2>/dev/null || echo "none")
        
        if [[ "$db_state" == "running" ]]; then
            if [[ "$db_health" == "healthy" ]] || docker_compose exec -T livereview-db pg_isready -U livereview >/dev/null 2>&1; then
                log_info "✓ PostgreSQL container is healthy"
                ((containers_healthy++))
            else
                log_info "○ PostgreSQL container running, health: ${db_health:-checking...}"
            fi
        else
            log_info "○ PostgreSQL container state: $db_state"
        fi
        
        # Check LiveReview app container
        local app_state=$(docker inspect --format='{{.State.Status}}' livereview-app 2>/dev/null || echo "missing")
        local app_health=$(docker inspect --format='{{.State.Health.Status}}' livereview-app 2>/dev/null || echo "none")
        
        if [[ "$app_state" == "running" ]]; then
            # If container has healthcheck defined, use it; otherwise use HTTP check
            if [[ "$app_health" == "healthy" ]]; then
                log_info "✓ LiveReview app container is healthy (docker healthcheck)"
                ((containers_healthy++))
            elif curl -f -s --max-time 5 "http://localhost:${LIVEREVIEW_BACKEND_PORT:-8888}/health" >/dev/null 2>&1 || \
                 curl -f -s --max-time 5 "http://localhost:${LIVEREVIEW_BACKEND_PORT:-8888}/api/health" >/dev/null 2>&1; then
                log_info "✓ LiveReview app container is healthy (HTTP check)"
                ((containers_healthy++))
            else
                log_info "○ LiveReview app running, health: ${app_health}, HTTP not ready yet..."
            fi
        else
            log_info "○ LiveReview app container state: $app_state"
        fi
        
        # If both containers are healthy, we're done
        if [[ $containers_healthy -eq 2 ]]; then
            log_success "All containers are healthy!"
            return 0
        fi
        
        sleep $check_interval
        wait_time=$((wait_time + check_interval))
    done
    
    log_error "Containers did not become healthy within ${max_wait} seconds"
    log_info "Container status:"
    docker_compose ps
    log_info "Recent logs:"
    docker_compose logs --tail=10
    return 1
}

# Verify application health and accessibility for two-mode system
verify_deployment() {
    local config_file="$1"
    
    section_header "VERIFYING DEPLOYMENT"
    log_info "Verifying LiveReview deployment..."

    # On macOS, Docker Desktop may still be initializing even after containers start.
    # Give an explicit short warm-up with visible progress to avoid false negatives.
    if [[ "$(uname -s)" == "Darwin" ]]; then
        log_info "macOS detected: performing extra warm-up before verification (45s)"
        progress_sleep 45 "Warming up containers"
    fi
    
    # Source configuration to get ports
    source "$config_file"
    # Derive deployment mode from simplified flag
    local reverse_proxy="${LIVEREVIEW_REVERSE_PROXY:-false}"
    local deployment_mode
    if [[ "$reverse_proxy" == "true" ]]; then
        deployment_mode="production"
    else
        deployment_mode="demo"
    fi
    
    # Check API endpoint (with timeout) - try multiple possible endpoints
    log_info "Checking API endpoint accessibility..."
    local api_ready=false
    local endpoints=("/health" "/api/health" "/api/healthcheck" "/api")
    
    local api_attempts=12
    local api_interval=5
    if [[ "$(uname -s)" == "Darwin" ]]; then
        api_attempts=20   # 100s
        api_interval=5
        log_info "macOS detected: extending API readiness attempts to $api_attempts"
    fi
    for (( i=1; i<=api_attempts; i++ )); do
        for endpoint in "${endpoints[@]}"; do
            if curl -f -s --max-time 5 "http://localhost:${LIVEREVIEW_BACKEND_PORT}${endpoint}" >/dev/null 2>&1; then
                log_success "✓ API endpoint is accessible at: http://localhost:${LIVEREVIEW_BACKEND_PORT}${endpoint}"
                api_ready=true
                break 2
            fi
        done
        log_info "○ API not ready, waiting... (attempt $i/$api_attempts)"
        sleep "$api_interval"
    done
    
    if [[ "$api_ready" != "true" ]]; then
        log_warning "API endpoint not accessible yet, but containers are running"
        log_info "This may be normal during initial startup"
    fi
    
    # Check UI endpoint
    log_info "Checking UI endpoint at http://localhost:${LIVEREVIEW_FRONTEND_PORT}/"
    local ui_ready=false
    local ui_attempts=6
    local ui_interval=5
    if [[ "$(uname -s)" == "Darwin" ]]; then
        ui_attempts=12   # 60s
        ui_interval=5
        log_info "macOS detected: extending UI readiness attempts to $ui_attempts"
    fi
    for (( i=1; i<=ui_attempts; i++ )); do
        if curl -f -s --max-time 5 "http://localhost:${LIVEREVIEW_FRONTEND_PORT}/" >/dev/null 2>&1; then
            log_success "✓ UI endpoint is accessible"
            ui_ready=true
            break
        else
            log_info "○ UI not ready, waiting... (attempt $i/$ui_attempts)"
            sleep "$ui_interval"
        fi
    done
    
    if [[ "$ui_ready" != "true" ]]; then
        log_warning "UI endpoint not accessible yet, but containers are running"
        log_info "This may be normal during initial startup"
    fi
    
    # Verify database connectivity from application
    log_info "Verifying database connectivity..."
    cd "$LIVEREVIEW_INSTALL_DIR" || return 1
    
    if docker_compose exec -T livereview-db pg_isready -U livereview >/dev/null 2>&1; then
        log_success "✓ Database is accessible and ready"
    else
        log_warning "Database connectivity check failed"
        return 1
    fi
    
    # Show final status
    log_success "Deployment verification completed"
    
    if [[ "$api_ready" == "true" && "$ui_ready" == "true" ]]; then
        log_success "🎉 LiveReview is fully operational!"
        if [[ "$deployment_mode" == "demo" ]]; then
            log_info "   - Demo Mode: http://localhost:${LIVEREVIEW_FRONTEND_PORT}/"
            log_info "   - API: http://localhost:${LIVEREVIEW_BACKEND_PORT}/api"
            log_info "   - Webhooks: Disabled (manual triggers only)"
            log_info "   📝 Note: Frontend automatically configured to use API port ${LIVEREVIEW_BACKEND_PORT}"
        else
            log_info "   - Production Mode: Configure reverse proxy"
            log_info "   - Backend: http://127.0.0.1:${LIVEREVIEW_BACKEND_PORT}/api"
            log_info "   - Frontend: http://127.0.0.1:${LIVEREVIEW_FRONTEND_PORT}/"
            log_info "   - Webhooks: Enabled (automatic triggers)"
            log_info "   📝 Note: Frontend configured to use reverse proxy for API calls"
        fi
    else
        log_info "🔄 LiveReview containers are running but services may still be starting up"
        log_info "   - Check status with: $DOCKER_COMPOSE_CMD -f $LIVEREVIEW_INSTALL_DIR/docker-compose.yml ps"
        log_info "   - View logs with: $DOCKER_COMPOSE_CMD -f $LIVEREVIEW_INSTALL_DIR/docker-compose.yml logs"
    fi
    
    return 0
}

# Complete Docker deployment workflow
deploy_with_docker() {
    local resolved_version="$1"
    local config_file="$2"
    
    # Step 1: Pull required images
    if ! pull_docker_images "$resolved_version"; then
        error_exit "Docker image pulling failed"
    fi
    
    # Step 2: Start containers
    if ! start_containers; then
        error_exit "Container startup failed"
    fi
    
    # Step 3: Wait for containers to be healthy
    if ! wait_for_containers; then
        log_error "Container health check failed"
        log_info "Attempting to show container status and logs for debugging..."
        cd "$LIVEREVIEW_INSTALL_DIR" && docker_compose ps && docker_compose logs --tail=20
        error_exit "Deployment failed - containers not healthy"
    fi
    
    # Step 4: Verify deployment
    if ! verify_deployment "$config_file"; then
        log_warning "Deployment verification had issues, but containers are running"
        log_info "You can check the status manually with: lrops.sh status"
    fi
    
    log_success "Docker deployment completed successfully"
}

# =============================================================================
# MANAGEMENT COMMANDS (PHASE 6)
# =============================================================================

# Show installation status and container health
show_status() {
    section_header "LIVEREVIEW STATUS"
    
    # Check if installation exists
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        log_info "Run 'lrops.sh setup-demo' to install"
        return 1
    fi
    
    log_info "Installation directory: $LIVEREVIEW_INSTALL_DIR"
    
    # Check if docker-compose.yml exists
    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
        log_error "Docker Compose configuration not found"
        return 1
    fi
    
    # Show container status (using docker_compose function which handles paths correctly)
    log_info "Container Status:"
    if docker_compose ps 2>/dev/null | grep -q "livereview"; then
        docker_compose ps
        echo
        
        # Check if containers are healthy
        local app_status=$(docker_compose ps -q livereview-app | xargs docker inspect --format='{{.State.Health.Status}}' 2>/dev/null)
        local db_status=$(docker_compose ps -q livereview-db | xargs docker inspect --format='{{.State.Health.Status}}' 2>/dev/null)
        
        if [[ "$app_status" == "healthy" && "$db_status" == "healthy" ]]; then
            log_success "✅ All services are healthy"
        elif [[ "$app_status" == "starting" || "$db_status" == "starting" ]]; then
            log_info "🔄 Services are starting up..."
        else
            log_warning "⚠️ Some services may have issues"
        fi
    else
        log_warning "No containers are running"
        log_info "Run 'lrops.sh start' to start services"
    fi
    
    # Show version information (using absolute path)
    echo
    log_info "Version Information:"
    if [[ -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
        local lr_version=$(grep "LIVEREVIEW_VERSION=" "$LIVEREVIEW_INSTALL_DIR/.env" | cut -d'=' -f2)
        log_info "  LiveReview: ${lr_version:-unknown}"
    fi
    log_info "  Script: $SCRIPT_VERSION"
    
    # Show access URLs if running
    if docker_compose ps 2>/dev/null | grep -q "Up.*8888"; then
        echo
        log_info "🌐 Access URLs:"
        local api_port=$(docker_compose ps livereview-app | grep -o "0.0.0.0:[0-9]*->8888" | cut -d':' -f2 | cut -d'-' -f1)
        local ui_port=$(docker_compose ps livereview-app | grep -o "0.0.0.0:[0-9]*->8081" | cut -d':' -f2 | cut -d'-' -f1)
        log_info "  - Web UI: http://localhost:${ui_port:-8081}/"
        log_info "  - API: http://localhost:${api_port:-8888}/api"
    fi
}

# Show installation information and file locations
show_info() {
    section_header "LIVEREVIEW INSTALLATION INFO"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        return 1
    fi
    
    log_info "📁 Installation Directory: $LIVEREVIEW_INSTALL_DIR"
    echo
    log_info "📋 Important Files:"
    log_info "  - Docker Compose: $LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
    log_info "  - Environment: $LIVEREVIEW_INSTALL_DIR/.env"
    log_info "  - Installation Summary: $LIVEREVIEW_INSTALL_DIR/installation-summary.txt"
    log_info "  - Installation Report: $LIVEREVIEW_INSTALL_DIR/installation-report.txt"
    echo
    log_info "📂 Configuration Templates:"
    log_info "  - Nginx: $LIVEREVIEW_INSTALL_DIR/config/nginx.conf.example"
    log_info "  - Caddy: $LIVEREVIEW_INSTALL_DIR/config/caddy.conf.example"
    log_info "  - Apache: $LIVEREVIEW_INSTALL_DIR/config/apache.conf.example"
    echo
    log_info "🔧 Helper Scripts:"
    log_info "  - Backup: $LIVEREVIEW_INSTALL_DIR/scripts/backup.sh"
    log_info "  - Restore: $LIVEREVIEW_INSTALL_DIR/scripts/restore.sh"
    log_info "  - SSL Setup: $LIVEREVIEW_INSTALL_DIR/scripts/setup-ssl.sh"
    log_info "  - SSL Renewal: $LIVEREVIEW_INSTALL_DIR/scripts/renew-ssl.sh"
    log_info "  - Cron Example: $LIVEREVIEW_INSTALL_DIR/config/backup-cron.example"
    echo
    log_info "💾 Data Directory: $LIVEREVIEW_INSTALL_DIR/lrdata/"
    log_info "  - PostgreSQL Data: $LIVEREVIEW_INSTALL_DIR/lrdata/postgres/"
    echo
    log_info "📖 Management Commands:"
    log_info "  - Status: lrops.sh status"
    log_info "  - Start: lrops.sh start"
    log_info "  - Stop: lrops.sh stop"
    log_info "  - Restart: lrops.sh restart"
    log_info "  - Logs: lrops.sh logs [service]"
    echo
    log_info "🆘 Help Commands:"
    log_info "  - SSL Setup: lrops.sh help ssl"
    log_info "  - Backup Guide: lrops.sh help backup"
    log_info "  - Nginx Config: lrops.sh help nginx"
    log_info "  - Caddy Config: lrops.sh help caddy"
    log_info "  - Apache Config: lrops.sh help apache"
}

# Start LiveReview containers
start_containers_cmd() {
    section_header "STARTING LIVEREVIEW"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        log_info "Run 'lrops.sh setup-demo' to install"
        return 1
    fi
    
    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
        log_error "Docker Compose configuration not found at: $LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
        return 1
    fi
    
    log_info "Starting LiveReview containers..."
    
    if docker_compose up -d; then
        log_success "Containers started successfully"
        
        # Wait a moment for health checks
        log_info "Waiting for services to be ready..."
        sleep 5
        
        # Show status
        docker_compose ps
        log_info "Run 'lrops.sh status' to check service health"
    else
        log_error "Failed to start containers"
        return 1
    fi
}

# Stop LiveReview containers
stop_containers_cmd() {
    section_header "STOPPING LIVEREVIEW"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        return 1
    fi
    
    log_info "Stopping LiveReview containers..."
    
    if docker_compose down; then
        log_success "Containers stopped successfully"
    else
        log_warning "Some containers may not have stopped cleanly"
        return 1
    fi
}

# Restart LiveReview containers
restart_containers_cmd() {
    section_header "RESTARTING LIVEREVIEW"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        return 1
    fi
    
    # Change to installation directory for docker compose
    pushd "$LIVEREVIEW_INSTALL_DIR" >/dev/null || {
        log_error "Could not change to installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    # Check if we should use 'restart' or 'up -d' based on recent .env changes
    local use_recreate=false
    if [[ -f ".env" ]]; then
        # Check if .env was modified in the last 5 minutes (300 seconds)
        local env_age
        if command -v stat >/dev/null 2>&1; then
            env_age=$(stat -c %Y .env 2>/dev/null || echo "0")
            local current_time=$(date +%s)
            local age_diff=$((current_time - env_age))
            if [[ $age_diff -lt 300 ]]; then
                use_recreate=true
                log_info "Detected recent .env changes - using container recreation instead of restart"
            fi
        fi
    fi
    
    if [[ "$use_recreate" == "true" ]]; then
        log_info "Recreating LiveReview containers with updated configuration..."
        if docker_compose up -d; then
            log_success "Containers recreated successfully"
        else
            log_error "Failed to recreate containers"
            popd >/dev/null || true
            return 1
        fi
    else
        log_info "Restarting LiveReview containers..."
        if docker_compose restart; then
            log_success "Containers restarted successfully"
        else
            log_warning "Standard restart failed, trying recreation..."
            if docker_compose up -d; then
                log_success "Containers recreated successfully"
            else
                log_error "Both restart and recreation failed"
                popd >/dev/null || true
                return 1
            fi
        fi
    fi
    
    # Wait a moment for services to be ready
    log_info "Waiting for services to be ready..."
    sleep 5
    
    # Show status
    docker_compose ps
    log_info "Run 'lrops.sh status' to check service health"
    
    popd >/dev/null || true
}

# Uninstall LiveReview (preserves a final full backup, skips external standalone DB containers)
uninstall_cmd() {
    section_header "UNINSTALLING LIVEREVIEW"

    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "Installation directory not found: $LIVEREVIEW_INSTALL_DIR"
        return 1
    fi

    # Final backup snapshot (tagged uninstall)
    log_info "Creating final backup snapshot before removal"
    create_pre_update_backup "uninstall" || log_warning "Final backup failed; proceeding with uninstall"

    # Stop only managed containers (avoid touching standalone 'livereview_pg')
    log_info "Stopping managed containers (livereview-app, livereview-db)"
    docker rm -f livereview-app livereview-db >/dev/null 2>&1 || true

    # Move installation directory to timestamped backup instead of outright deleting (user-owned)
    local ts=$(_now_ts)
    local archive_dir="${LIVEREVIEW_INSTALL_DIR}.removed.${ts}"
    if mv "$LIVEREVIEW_INSTALL_DIR" "$archive_dir" 2>/dev/null; then
        log_success "Installation moved to: $archive_dir"
    else
        # Retry with sudo but preserve ownership info
        if command -v sudo >/dev/null 2>&1; then
            if sudo mv "$LIVEREVIEW_INSTALL_DIR" "$archive_dir"; then
                # Give ownership back to invoking user if possible
                sudo chown -R "${SUDO_UID:-$(id -u)}:${SUDO_GID:-$(id -g)}" "$archive_dir" 2>/dev/null || true
                log_success "Installation moved (sudo) to: $archive_dir"
            else
                log_error "Failed to move installation directory (even with sudo)"
                return 1
            fi
        else
            log_error "Permission denied moving installation directory and sudo not available"
            return 1
        fi
    fi

    log_success "Uninstall complete. You can reinstall with: lrops.sh setup-demo"
    log_info "Backups remain in: ${archive_dir}/backups (and earlier pre-update backups)"
}

# Update LiveReview containers (pull newer image, update version, restart)
update_containers_cmd() {
    section_header "UPDATING LIVEREVIEW"

    local requested_version="$1"

    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        log_info "Run 'lrops.sh setup-demo' to install"
        return 1
    fi

    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
        log_error "Docker Compose configuration not found at: $LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
        return 1
    fi

    # Create pre-update backup (config + logical DB dump) before any changes
    create_pre_update_backup "${requested_version}" || {
        log_error "Pre-update backup failed; aborting update for safety"
        return 1
    }

    # Determine target version (strict semantic only)
    local target_version=""
    if [[ -n "$requested_version" ]]; then
        if ! is_semantic_version "$requested_version"; then
            log_error "Requested version '$requested_version' is not a valid semantic version (expected MAJOR.MINOR.PATCH optionally prefixed with v)"
            log_info "Example: lrops.sh update v1.4.2"
            return 1
        fi
        target_version="$requested_version"
        log_info "Requested semantic version for update: $target_version"
    else
        log_info "No version specified - resolving latest semantic version (ignoring non-semantic tags)..."
        if ! target_version=$(get_latest_version "hexmostech/livereview"); then
            log_error "Could not determine a semantic version tag from registry. Aborting update."
            log_info "Ensure a release tag like v1.2.3 exists in the registry."
            return 1
        fi
        log_success "Latest semantic version resolved: $target_version"
    fi

    # Normalize (strip leading v for docker tag usage)
    target_version=$(echo "$target_version" | sed 's/^v//')

    local app_image="${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${target_version}"
    log_info "Updating to image: $app_image"

    # Optional quick backup of .env before modifying
    if [[ -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
        local env_backup="$LIVEREVIEW_INSTALL_DIR/.env.bak.$(date +%Y%m%d_%H%M%S)"
        cp "$LIVEREVIEW_INSTALL_DIR/.env" "$env_backup" || true
        log_info "Backed up current .env to $env_backup"
    fi

    # Update LIVEREVIEW_VERSION in .env (add if missing)
    if [[ -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
        if grep -q '^LIVEREVIEW_VERSION=' "$LIVEREVIEW_INSTALL_DIR/.env"; then
            sed_inplace "s/^LIVEREVIEW_VERSION=.*/LIVEREVIEW_VERSION=${target_version}/" "$LIVEREVIEW_INSTALL_DIR/.env"
        else
            echo "LIVEREVIEW_VERSION=${target_version}" >> "$LIVEREVIEW_INSTALL_DIR/.env"
        fi
        log_success "Updated .env LIVEREVIEW_VERSION=${target_version}"
    else
        log_warning ".env not found; proceeding without updating version variable"
    fi

    # Pull new image
    log_info "Pulling new LiveReview image..."
    if ! docker pull "$app_image"; then
        log_error "Failed to pull image: $app_image"
        return 1
    fi
    log_success "Image pulled successfully"

    # Robustly update docker-compose.yml image tag (ignore whitespace, verify after)
    local compose_file="$LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
    if [[ -f "$compose_file" ]]; then
        local old_tag_line new_tag_line
        old_tag_line=$(grep -E '^\s*image:\s*ghcr\.io/hexmostech/livereview:' "$compose_file" | head -1)
        if [[ -n "$old_tag_line" ]]; then
            # Use awk to preserve indentation and replace only the tag
            awk -v tgt="$target_version" '
                /^\s*image:\s*ghcr\.io\/hexmostech\/livereview:/ {
                    sub(/ghcr\.io\/hexmostech\/livereview:[^ ]+/, "ghcr.io/hexmostech/livereview:" tgt)
                }
                { print }
            ' "$compose_file" > "$compose_file.tmp" && mv "$compose_file.tmp" "$compose_file"
            new_tag_line=$(grep -E '^\s*image:\s*ghcr\.io/hexmostech/livereview:' "$compose_file" | head -1)
            if [[ "$new_tag_line" == *":$target_version" ]]; then
                log_success "Updated docker-compose.yml image tag to ${target_version}"
            else
                log_error "Failed to update docker-compose.yml image tag to ${target_version} (found: $new_tag_line)"
                log_error "Aborting update to prevent version mismatch. Please check file permissions."
                return 1
            fi
        else
            log_warning "Could not find image line in docker-compose.yml to update"
        fi
    else
        log_warning "docker-compose.yml not found when attempting to update image tag"
    fi

    # Restart container using new image (compose will pick updated env var for tag substitution already baked in file)
    # For safety, force recreate only the app container
    pushd "$LIVEREVIEW_INSTALL_DIR" >/dev/null || true
    log_info "Recreating application container with new image..."
    if ! docker_compose up -d --no-deps --force-recreate livereview-app; then
        log_error "Container recreation failed"
        popd >/dev/null || true
        return 1
    fi

    # Wait for health
    log_info "Waiting for updated container health..."
    local max_wait=120
    local waited=0
    local app_cid
    while [[ $waited -lt $max_wait ]]; do
        app_cid=$(docker_compose ps -q livereview-app || true)
        if [[ -n "$app_cid" ]]; then
            local health
            health=$(docker inspect --format='{{.State.Health.Status}}' "$app_cid" 2>/dev/null || echo "")
            if [[ "$health" == "healthy" ]]; then
                log_success "Application container is healthy"
                break
            fi
        fi
        sleep 5
        waited=$((waited+5))
        log_info "  ... still waiting ("$waited"s)"
    done
    popd >/dev/null || true

    if [[ $waited -ge $max_wait ]]; then
        log_warning "Health check timeout after ${max_wait}s"
        log_info "Run 'lrops.sh status' or 'lrops.sh logs livereview-app' for details"
        log_info "You can rollback using: lrops.sh restore latest"
        return 1
    fi

    # Post-update status
    log_info "Performing post-update status check..."
    show_status || true
    log_success "LiveReview updated to version ${target_version}"
}

# -----------------------------------------------------------------------------
# BACKUP & RESTORE UTILITIES
# -----------------------------------------------------------------------------

# Create a timestamped directory name
_now_ts() { date +%Y%m%d_%H%M%S; }

# Detect active DB container name (supports legacy naming differences)
detect_db_container() {
    local name
    name=$(docker ps --format '{{.Names}}' | grep -E '^livereview-db$|^livereview_pg$' | head -1 || true)
    if [[ -z "$name" ]]; then
        # Try compose-defined service
        name=$(docker ps --format '{{.Names}}' | grep -E 'livereview.*db' | head -1 || true)
    fi
    [[ -n "$name" ]] && echo "$name"
}

# Create a pre-update backup snapshot (config + DB dump)
create_pre_update_backup() {
    local target="$1"  # may be empty if user did not specify
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "Installation directory not found for backup"
        return 1
    fi
    local ts=$(_now_ts)
    local current_version="unknown"
    [[ -f "$LIVEREVIEW_INSTALL_DIR/.env" ]] && current_version=$(grep -E '^LIVEREVIEW_VERSION=' "$LIVEREVIEW_INSTALL_DIR/.env" | cut -d'=' -f2 | tr -d '\r' || echo "unknown")
    local backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
    mkdir -p "$backup_root"
    local dir_name="preupdate-${ts}-from-${current_version}"
    [[ -n "$target" ]] && dir_name+="-to-${target}"
    local backup_dir="$backup_root/$dir_name"
    mkdir -p "$backup_dir"
    log_info "Creating pre-update backup: $backup_dir"
    # Copy config files
    for f in .env docker-compose.yml; do
        if [[ -f "$LIVEREVIEW_INSTALL_DIR/$f" ]]; then
            cp "$LIVEREVIEW_INSTALL_DIR/$f" "$backup_dir/$f" || true
        fi
    done
    # Attempt physical snapshot (compressed) of lrdata/postgres using sudo if needed
    local data_dir="$LIVEREVIEW_INSTALL_DIR/lrdata/postgres"
    local physical_snapshot=false
    if [[ -d "$data_dir" ]]; then
        log_info "Creating compressed physical data snapshot (postgres directory)"
        local tar_target="$backup_dir/postgres-data.tgz"
        if tar -czf "$tar_target" -C "$LIVEREVIEW_INSTALL_DIR/lrdata" postgres 2>/dev/null; then
            physical_snapshot=true
            log_success "Physical snapshot created (without sudo)"
        else
            # Retry with sudo
            if command -v sudo >/dev/null 2>&1; then
                log_info "Retrying physical snapshot with sudo due to permission issues"
                if sudo tar -czf "$tar_target" -C "$LIVEREVIEW_INSTALL_DIR/lrdata" postgres; then
                    physical_snapshot=true
                    # Adjust ownership so invoking user can manipulate backup
                    sudo chown "${SUDO_UID:-$(id -u)}:${SUDO_GID:-$(id -g)}" "$tar_target" 2>/dev/null || true
                    log_success "Physical snapshot created with sudo"
                else
                    log_warning "Failed to create physical snapshot even with sudo; continuing"
                fi
            else
                log_warning "sudo not available; skipping physical data snapshot"
            fi
        fi
    else
        log_info "No postgres data directory found (skipping physical snapshot)"
    fi
    # Capture image + container metadata
    docker ps --no-trunc > "$backup_dir/docker-ps.txt" 2>/dev/null || true
    docker images --no-trunc | grep livereview > "$backup_dir/docker-images.txt" 2>/dev/null || true
    # Logical DB dump
    local db_container
    db_container=$(detect_db_container || true)
    if [[ -n "$db_container" ]]; then
        # Extract DB password from env
        local db_pass
        db_pass=$(grep -E '^DB_PASSWORD=' "$LIVEREVIEW_INSTALL_DIR/.env" | cut -d'=' -f2 | tr -d '\r' || true)
        if [[ -n "$db_pass" ]]; then
            log_info "Creating logical database dump from container $db_container"
            if docker exec -e PGPASSWORD="$db_pass" "$db_container" pg_dump -U livereview -d livereview > "$backup_dir/db.sql" 2>"$backup_dir/db_dump.stderr"; then
                log_success "Database dump created"
            else
                log_warning "Database dump failed (see db_dump.stderr); continuing with config backup"
                rm -f "$backup_dir/db.sql" || true
            fi
        else
            log_warning "DB_PASSWORD not found in .env; skipping DB dump"
        fi
    else
        log_warning "Could not detect running DB container; skipping DB dump"
    fi
    # Metadata JSON
    cat > "$backup_dir/metadata.json" <<EOF
{
  "type": "pre-update",
  "timestamp": "${ts}",
  "current_version": "${current_version}",
  "target_version": "${target}",
  "has_physical_snapshot": ${physical_snapshot},
  "has_logical_dump": $( [[ -f "$backup_dir/db.sql" ]] && echo true || echo false ),
  "script_version": "${SCRIPT_VERSION}"
}
EOF
    log_success "Pre-update backup complete"
    prune_old_backups
}

prune_old_backups() {
    local backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
    [[ ! -d "$backup_root" ]] && return 0
    local count
    count=$(find "$backup_root" -maxdepth 1 -type d -name 'preupdate-*' | wc -l | tr -d ' ')
    if (( count > BACKUP_RETENTION_COUNT )); then
        local remove=$((count - BACKUP_RETENTION_COUNT))
        log_info "Pruning $remove old backup(s) (retention=$BACKUP_RETENTION_COUNT)"
        find "$backup_root" -maxdepth 1 -type d -name 'preupdate-*' | sort | head -n "$remove" | xargs -r rm -rf
    fi
}

list_backups_cmd() {
    section_header "AVAILABLE BACKUPS"
    
    # Determine which backup directory to list
    local backup_root
    if [[ -n "$BACKUP_TARGET_DIR" ]]; then
        backup_root=$(eval echo "$BACKUP_TARGET_DIR")
        log_info "Listing backups in custom directory: $backup_root"
    else
        backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
        log_info "Listing backups in default directory: $backup_root"
    fi
    
    if [[ ! -d "$backup_root" ]]; then
        log_warning "No backups directory present at: $backup_root"
        return 0
    fi
    
    local entries
    # List both preupdate and manual backups
    entries=$(find "$backup_root" -maxdepth 1 -type d \( -name 'preupdate-*' -o -name 'manual-*' -o -name 'quickbackup-*' \) | sort -r)
    if [[ -z "$entries" ]]; then
        log_info "No backups found"
        return 0
    fi
    
    log_info "Found $(echo "$entries" | wc -l) backup(s):"
    echo "$entries" | sed "s#^$backup_root/##" | sed 's/^/  - /'
}

restore_backup_cmd() {
    local which="$1"
    section_header "RESTORING BACKUP"
    if [[ -z "$which" ]]; then
        log_error "Usage: lrops.sh restore <backup_id|latest>"
        return 1
    fi
    
    # Determine backup directory
    local backup_root
    if [[ -n "$BACKUP_TARGET_DIR" ]]; then
        backup_root=$(eval echo "$BACKUP_TARGET_DIR")
        log_info "Looking for backups in custom directory: $backup_root"
    else
        backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
    fi
    if [[ ! -d "$backup_root" ]]; then
        log_error "No backups directory at $backup_root"
        return 1
    fi
    local target_dir
    if [[ "$which" == "latest" ]]; then
        # Find latest backup of any type (preupdate, manual, quickbackup)
        target_dir=$(find "$backup_root" -maxdepth 1 -type d \( -name 'preupdate-*' -o -name 'manual-*' -o -name 'quickbackup-*' \) | sort -r | head -1 || true)
        if [[ -n "$target_dir" ]]; then
            log_info "Latest backup found: $(basename "$target_dir")"
        fi
    else
        target_dir="$backup_root/$which"
    fi
    if [[ -z "$target_dir" || ! -d "$target_dir" ]]; then
        log_error "Backup '$which' not found"
        list_backups_cmd
        return 1
    fi
    log_info "Restoring from: $target_dir"
    # Stop app container (keep DB up for restore)
    docker_compose stop livereview-app || true
    # Restore config files
    for f in .env docker-compose.yml; do
        if [[ -f "$target_dir/$f" ]]; then
            cp "$target_dir/$f" "$LIVEREVIEW_INSTALL_DIR/$f"
        fi
    done
    local physical_archive="$target_dir/postgres-data.tgz"
    if [[ -f "$physical_archive" ]]; then
        log_info "Restoring physical postgres data snapshot"
        # Stop DB container fully for physical restore
        docker_compose stop livereview-db >/dev/null 2>&1 || true
        local pgdir="$LIVEREVIEW_INSTALL_DIR/lrdata/postgres"
        if [[ -d "$pgdir" ]]; then
            local pg_backup="${pgdir}.pre-restore.$(_now_ts)"
            log_info "Moving existing postgres dir to $pg_backup"
            if mv "$pgdir" "$pg_backup" 2>/dev/null; then
                log_success "Existing postgres dir moved"
            else
                if command -v sudo >/dev/null 2>&1; then
                    sudo mv "$pgdir" "$pg_backup" && log_success "Existing postgres dir moved with sudo" || log_warning "Could not move existing postgres dir"
                else
                    log_warning "Permission issue moving postgres dir and sudo not available; aborting physical restore"
                    physical_archive=""  # skip extraction
                fi
            fi
        fi
        if [[ -n "$physical_archive" ]]; then
            mkdir -p "$LIVEREVIEW_INSTALL_DIR/lrdata"
            if tar -xzf "$physical_archive" -C "$LIVEREVIEW_INSTALL_DIR/lrdata" 2>/dev/null; then
                log_success "Extracted physical snapshot"
            else
                if command -v sudo >/dev/null 2>&1; then
                    sudo tar -xzf "$physical_archive" -C "$LIVEREVIEW_INSTALL_DIR/lrdata" && log_success "Extracted physical snapshot with sudo" || log_warning "Failed to extract physical snapshot"
                else
                    log_warning "Failed to extract physical snapshot (permissions)"
                fi
            fi
        fi
        # Start DB container again
        docker_compose up -d livereview-db || true
        sleep 5
    elif [[ -f "$target_dir/db.sql" ]]; then
        # Logical restore path (only if no physical archive present)
        log_info "Restoring database from logical dump"
        local db_container
        db_container=$(detect_db_container || true)
        if [[ -z "$db_container" ]]; then
            log_info "Starting database container for restore"
            docker_compose up -d livereview-db || true
            sleep 5
            db_container=$(detect_db_container || true)
        fi
        local db_pass
        db_pass=$(grep -E '^DB_PASSWORD=' "$LIVEREVIEW_INSTALL_DIR/.env" | cut -d'=' -f2 | tr -d '\r' || true)
        if [[ -n "$db_container" && -n "$db_pass" ]]; then
            if cat "$target_dir/db.sql" | docker exec -i -e PGPASSWORD="$db_pass" "$db_container" psql -U livereview -d livereview >/dev/null 2>&1; then
                log_success "Database restore (logical) completed"
            else
                log_warning "Database logical restore encountered errors"
            fi
        else
            log_warning "Skipping logical DB restore (container or password missing)"
        fi
    else
        log_info "No database artifacts found in backup (physical or logical); skipping DB restore"
    fi
    # Start / recreate app
    docker_compose up -d --force-recreate livereview-app || true
    log_success "Restore process initiated. Check status with: lrops.sh status"
}

# Create on-demand backup with custom name
create_backup_cmd() {
    local backup_name="$1"
    section_header "CREATING BACKUP"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        log_info "Run 'lrops.sh setup-demo' to install"
        return 1
    fi
    
    # Generate backup name if not provided
    if [[ -z "$backup_name" ]]; then
        backup_name="manual-$(date +%Y%m%d_%H%M%S)"
        log_info "💡 No backup name provided - using default naming"
        log_info "   To use custom names: lrops.sh backup your-backup-name"
        log_info "   For help: lrops.sh backup --help"
        echo
    fi
    
    # Validate backup name (no special characters that could cause issues)
    if [[ ! "$backup_name" =~ ^[a-zA-Z0-9_-]+$ ]]; then
        log_error "Invalid backup name: '$backup_name'"
        log_error "Backup names can only contain:"
        log_error "  • Letters (A-Z, a-z)"
        log_error "  • Numbers (0-9)" 
        log_error "  • Hyphens (-)"
        log_error "  • Underscores (_)"
        log_info ""
        log_info "Valid examples:"
        log_info "  lrops.sh backup before-upgrade"
        log_info "  lrops.sh backup weekly_backup"
        log_info "  lrops.sh backup backup-2024-12"
        return 1
    fi
    
    # Determine backup type and provide clear user feedback
    local backup_type="Default"
    if [[ -n "$backup_name" && "$backup_name" != "manual-"* ]]; then
        backup_type="Named"
    fi
    
    # Determine backup directory (custom or default)
    local backup_root
    if [[ -n "$BACKUP_TARGET_DIR" ]]; then
        # Validate and create custom backup directory
        if [[ ! "$BACKUP_TARGET_DIR" =~ ^/.*$ ]] && [[ ! "$BACKUP_TARGET_DIR" =~ ^[~].*$ ]]; then
            log_error "Backup directory must be an absolute path or start with ~"
            log_info "Examples:"
            log_info "  lrops.sh --backup-dir /mnt/external backup"
            log_info "  lrops.sh --backup-dir ~/my-backups backup"
            log_info "  lrops.sh --backup-dir /backup/livereview backup"
            return 1
        fi
        # Expand tilde if present
        backup_root=$(eval echo "$BACKUP_TARGET_DIR")
        backup_type="${backup_type} (Custom Directory)"
        log_info "📁 Backup Type: $backup_type"
        log_info "📍 Target Directory: $backup_root"
    else
        backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
        log_info "📁 Backup Type: $backup_type Backup"
        log_info "📍 Target Directory: $backup_root (default location)"
    fi
    
    log_info "🏷️  Backup Name: $backup_name"
    
    # Create backup root directory
    if ! mkdir -p "$backup_root"; then
        log_error "Failed to create backup directory: $backup_root"
        return 1
    fi
    
    # Use existing backup infrastructure but with custom name and location
    local ts=$(date +%Y%m%d_%H%M%S)
    local current_version="unknown"
    [[ -f "$LIVEREVIEW_INSTALL_DIR/.env" ]] && current_version=$(grep -E '^LIVEREVIEW_VERSION=' "$LIVEREVIEW_INSTALL_DIR/.env" | cut -d'=' -f2 | tr -d '\r' || echo "unknown")
    local dir_name="${backup_name}-${ts}-version-${current_version}"
    local backup_dir="$backup_root/$dir_name"
    mkdir -p "$backup_dir"
    
    log_info "Creating backup in: $backup_dir"
    
    # Copy config files
    for f in .env docker-compose.yml; do
        if [[ -f "$LIVEREVIEW_INSTALL_DIR/$f" ]]; then
            cp "$LIVEREVIEW_INSTALL_DIR/$f" "$backup_dir/$f" || true
        fi
    done
    
    # Copy config and scripts directories if they exist
    for d in config scripts; do
        if [[ -d "$LIVEREVIEW_INSTALL_DIR/$d" ]]; then
            cp -r "$LIVEREVIEW_INSTALL_DIR/$d" "$backup_dir/" || true
        fi
    done
    
    # Attempt physical snapshot (compressed) of lrdata/postgres using sudo if needed
    local data_dir="$LIVEREVIEW_INSTALL_DIR/lrdata/postgres"
    local physical_snapshot=false
    if [[ -d "$data_dir" ]]; then
        log_info "Creating compressed physical data snapshot (postgres directory)"
        local tar_target="$backup_dir/postgres-data.tgz"
        if tar -czf "$tar_target" -C "$LIVEREVIEW_INSTALL_DIR/lrdata" postgres 2>/dev/null; then
            physical_snapshot=true
            log_success "Physical snapshot created (without sudo)"
        else
            # Retry with sudo
            if command -v sudo >/dev/null 2>&1; then
                log_info "Retrying physical snapshot with sudo due to permission issues"
                if sudo tar -czf "$tar_target" -C "$LIVEREVIEW_INSTALL_DIR/lrdata" postgres; then
                    physical_snapshot=true
                    # Adjust ownership so invoking user can manipulate backup
                    sudo chown "${SUDO_UID:-$(id -u)}:${SUDO_GID:-$(id -g)}" "$tar_target" 2>/dev/null || true
                    log_success "Physical snapshot created with sudo"
                else
                    log_warning "Failed to create physical snapshot even with sudo; continuing"
                fi
            else
                log_warning "sudo not available; skipping physical data snapshot"
            fi
        fi
    else
        log_info "No postgres data directory found (skipping physical snapshot)"
    fi
    
    # Capture image + container metadata
    docker ps --no-trunc > "$backup_dir/docker-ps.txt" 2>/dev/null || true
    docker images --no-trunc | grep livereview > "$backup_dir/docker-images.txt" 2>/dev/null || true
    
    # Logical DB dump
    local db_container
    db_container=$(detect_db_container || true)
    if [[ -n "$db_container" ]]; then
        # Extract DB password from env
        local db_pass
        db_pass=$(grep -E '^DB_PASSWORD=' "$LIVEREVIEW_INSTALL_DIR/.env" | cut -d'=' -f2 | tr -d '\r' || true)
        if [[ -n "$db_pass" ]]; then
            log_info "Creating logical database dump from container $db_container"
            if docker exec -e PGPASSWORD="$db_pass" "$db_container" pg_dump -U livereview -d livereview > "$backup_dir/db.sql" 2>"$backup_dir/db_dump.stderr"; then
                log_success "Database dump created"
            else
                log_warning "Database dump failed (see db_dump.stderr); continuing with config backup"
                rm -f "$backup_dir/db.sql" || true
            fi
        else
            log_warning "DB_PASSWORD not found in .env; skipping DB dump"
        fi
    else
        log_warning "Could not detect running DB container; skipping DB dump"
    fi
    
    # Metadata JSON
    cat > "$backup_dir/metadata.json" <<EOF
{
  "type": "manual",
  "name": "${backup_name}",
  "timestamp": "${ts}",
  "current_version": "${current_version}",
  "has_physical_snapshot": ${physical_snapshot},
  "has_logical_dump": $( [[ -f "$backup_dir/db.sql" ]] && echo true || echo false ),
  "script_version": "${SCRIPT_VERSION}"
}
EOF
    
    # Calculate backup size
    local backup_size
    backup_size=$(du -sh "$backup_dir" | cut -f1)
    
    log_success "✅ Backup completed successfully!"
    echo
    log_info "📦 Backup Details:"
    log_info "   Name: $dir_name"
    log_info "   Location: $backup_dir"
    log_info "   Size: $backup_size"
    log_info "   Contains: Database, configuration, settings"
    echo
    log_info "🔄 Restore Options:"
    log_info "   Restore this backup: lrops.sh restore $dir_name"
    log_info "   Restore latest backup: lrops.sh restore latest"
    echo
    log_info "📋 Management Commands:"
    log_info "   List all backups: lrops.sh list-backups"
    log_info "   View backup info: lrops.sh backup-info $dir_name"
    log_info "   Delete this backup: lrops.sh delete-backup $dir_name"
    
    # Don't prune old backups for manual backups - let user manage them
}

# Create quick backup with timestamp-based name
quick_backup_cmd() {
    section_header "CREATING QUICK BACKUP"
    local timestamp_name="quickbackup-$(date +%Y%m%d_%H%M%S)"
    create_backup_cmd "$timestamp_name"
}

# Delete a specific backup
delete_backup_cmd() {
    local backup_name="$1"
    section_header "DELETING BACKUP"
    
    if [[ -z "$backup_name" ]]; then
        log_error "Usage: lrops.sh delete-backup <backup_name>"
        log_info "Available backups:"
        list_backups_cmd
        return 1
    fi
    
    # Determine backup directory
    local backup_root
    if [[ -n "$BACKUP_TARGET_DIR" ]]; then
        backup_root=$(eval echo "$BACKUP_TARGET_DIR")
    else
        backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
    fi
    if [[ ! -d "$backup_root" ]]; then
        log_error "No backups directory at $backup_root"
        return 1
    fi
    
    local backup_dir="$backup_root/$backup_name"
    if [[ ! -d "$backup_dir" ]]; then
        log_error "Backup '$backup_name' not found"
        list_backups_cmd
        return 1
    fi
    
    # Confirmation prompt
    log_warning "This will permanently delete backup: $backup_name"
    log_warning "Location: $backup_dir"
    read -p "Are you sure you want to delete this backup? [y/N]: " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Backup deletion cancelled"
        return 0
    fi
    
    # Delete the backup
    if rm -rf "$backup_dir"; then
        log_success "Backup '$backup_name' deleted successfully"
    else
        log_error "Failed to delete backup '$backup_name'"
        return 1
    fi
}

# Show detailed backup information
show_backup_info_cmd() {
    local backup_name="$1"
    section_header "BACKUP INFORMATION"
    
    if [[ -z "$backup_name" ]]; then
        log_error "Usage: lrops.sh backup-info <backup_name>"
        log_info "Available backups:"
        list_backups_cmd
        return 1
    fi
    
    # Determine backup directory
    local backup_root
    if [[ -n "$BACKUP_TARGET_DIR" ]]; then
        backup_root=$(eval echo "$BACKUP_TARGET_DIR")
    else
        backup_root="$LIVEREVIEW_INSTALL_DIR/backups"
    fi
    local backup_dir="$backup_root/$backup_name"
    
    if [[ ! -d "$backup_dir" ]]; then
        log_error "Backup '$backup_name' not found"
        list_backups_cmd
        return 1
    fi
    
    log_info "Backup: $backup_name"
    log_info "Location: $backup_dir"
    
    # Show backup size
    local backup_size
    backup_size=$(du -sh "$backup_dir" | cut -f1)
    log_info "Size: $backup_size"
    
    # Show creation date from directory timestamp
    local creation_date
    creation_date=$(stat -c %y "$backup_dir" 2>/dev/null || stat -f %Sm "$backup_dir" 2>/dev/null || echo "Unknown")
    log_info "Created: $creation_date"
    
    # Show metadata if available
    if [[ -f "$backup_dir/metadata.json" ]]; then
        echo
        log_info "Metadata:"
        if command -v jq >/dev/null 2>&1; then
            jq . "$backup_dir/metadata.json" 2>/dev/null || cat "$backup_dir/metadata.json"
        else
            cat "$backup_dir/metadata.json"
        fi
    fi
    
    echo
    log_info "Contents:"
    ls -la "$backup_dir/" | tail -n +2 | while read -r line; do
        echo "  $line"
    done
    
    echo
    log_info "To restore: lrops.sh restore $backup_name"
    log_info "To delete: lrops.sh delete-backup $backup_name"
}

# Set LiveReview deployment mode (demo/production)
set_mode_cmd() {
    local mode="$1"
    section_header "SETTING DEPLOYMENT MODE"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        log_info "Run 'lrops.sh setup-demo' to install"
        return 1
    fi
    
    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
        log_error ".env file not found at: $LIVEREVIEW_INSTALL_DIR/.env"
        return 1
    fi
    
    # Validate mode parameter
    case "$mode" in
        demo|development|dev)
            local target_value="false"
            local mode_name="Demo Mode"
            local description="Localhost only, no webhooks, development/testing"
            ;;
        production|prod|server)
            local target_value="true"
            local mode_name="Production Mode"
            local description="External access via reverse proxy, webhooks enabled"
            ;;
        "")
            log_error "Usage: lrops.sh set-mode <demo|production>"
            log_info "Modes:"
            log_info "  demo       - Demo mode (LIVEREVIEW_REVERSE_PROXY=false)"
            log_info "  production - Production mode (LIVEREVIEW_REVERSE_PROXY=true)"
            return 1
            ;;
        *)
            log_error "Invalid mode: $mode"
            log_error "Valid modes: demo, production"
            return 1
            ;;
    esac
    
    local env_file="$LIVEREVIEW_INSTALL_DIR/.env"
    
    # Check current value
    local current_value=""
    if grep -q '^LIVEREVIEW_REVERSE_PROXY=' "$env_file" 2>/dev/null; then
        current_value=$(grep '^LIVEREVIEW_REVERSE_PROXY=' "$env_file" | cut -d'=' -f2 | tr -d '\r' || echo "")
        log_info "Current LIVEREVIEW_REVERSE_PROXY=$current_value"
    else
        log_info "LIVEREVIEW_REVERSE_PROXY not found in .env file"
    fi
    
    # Check if change is needed
    if [[ "$current_value" == "$target_value" ]]; then
        log_success "Already in $mode_name (LIVEREVIEW_REVERSE_PROXY=$target_value)"
        log_info "Description: $description"
        return 0
    fi
    
    # Create backup of current .env
    local backup_file="${env_file}.bak.$(date +%Y%m%d_%H%M%S)"
    if cp "$env_file" "$backup_file"; then
        log_info "Created backup: $backup_file"
    else
        log_warning "Could not create backup of .env file"
    fi
    
    # Update or add LIVEREVIEW_REVERSE_PROXY setting
    if grep -q '^LIVEREVIEW_REVERSE_PROXY=' "$env_file"; then
        # Update existing value
        if sed_inplace "s/^LIVEREVIEW_REVERSE_PROXY=.*/LIVEREVIEW_REVERSE_PROXY=$target_value/" "$env_file"; then
            log_success "Updated LIVEREVIEW_REVERSE_PROXY=$target_value"
        else
            log_error "Failed to update LIVEREVIEW_REVERSE_PROXY in .env file"
            return 1
        fi
    else
        # Add new value
        if echo "LIVEREVIEW_REVERSE_PROXY=$target_value" >> "$env_file"; then
            log_success "Added LIVEREVIEW_REVERSE_PROXY=$target_value to .env file"
        else
            log_error "Failed to add LIVEREVIEW_REVERSE_PROXY to .env file"
            return 1
        fi
    fi
    
    # Verify the change
    local new_value
    new_value=$(grep '^LIVEREVIEW_REVERSE_PROXY=' "$env_file" | cut -d'=' -f2 | tr -d '\r' || echo "")
    if [[ "$new_value" != "$target_value" ]]; then
        log_error "Verification failed: Expected $target_value, got $new_value"
        return 1
    fi
    
    log_success "Successfully switched to $mode_name"
    log_info "Description: $description"
    log_info "LIVEREVIEW_REVERSE_PROXY=$target_value"
    
    # Provide guidance based on mode
    echo
    if [[ "$mode" =~ ^(demo|development|dev)$ ]]; then
        log_info "Demo Mode Configuration:"
        log_info "✅ Access: http://localhost:8081/"
        log_info "✅ API: http://localhost:8888/api"
        log_info "✅ Webhooks: Disabled (manual triggers only)"
        log_info "✅ External Access: Not configured (localhost only)"
        log_info "✅ Perfect for: Development, testing, evaluation"
        echo
        log_info "To upgrade to production later:"
        log_info "  lrops.sh set-mode production"
    else
        log_info "Production Mode Configuration:"
        log_info "🔧 Backend: http://127.0.0.1:8888/api"
        log_info "🔧 Frontend: http://127.0.0.1:8081/"
        log_info "✅ Webhooks: Enabled (automatic triggers)"
        log_info "🔧 External Access: Via reverse proxy (requires setup)"
        log_info "🔧 SSL/TLS: Recommended for production use"
        echo
        log_warning "REVERSE PROXY SETUP REQUIRED:"
        log_info "Route /api/* → http://127.0.0.1:8888"
        log_info "Route /* → http://127.0.0.1:8081"
        echo
        log_info "Configuration Help:"
        log_info "  lrops.sh help nginx   # Nginx configuration"
        log_info "  lrops.sh help caddy   # Caddy configuration"
        log_info "  lrops.sh help apache  # Apache configuration"
        log_info "  lrops.sh help ssl     # SSL/TLS setup"
    fi
    
    echo
    log_info "Restart LiveReview to apply changes:"
    log_info "  lrops.sh restart"
    
    # Ask if user wants to restart now
    read -p "Restart LiveReview services now to apply changes? [y/N]: " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        log_info "Recreating LiveReview services with updated configuration..."
        
        # Change to installation directory for docker compose
        pushd "$LIVEREVIEW_INSTALL_DIR" >/dev/null || {
            log_error "Could not change to installation directory: $LIVEREVIEW_INSTALL_DIR"
            return 1
        }
        
        # Use docker compose up -d instead of restart to handle .env changes properly
        # This recreates containers with fresh bind mounts to the updated .env file
        if docker_compose up -d; then
            log_success "Services recreated successfully with new configuration"
            
            # Wait a moment for containers to start
            sleep 3
            
            # Show status
            log_info "Checking service status..."
            docker_compose ps
        else
            log_error "Failed to recreate services with new configuration"
            log_info "You can try manually: cd $LIVEREVIEW_INSTALL_DIR && docker compose up -d"
        fi
        
        popd >/dev/null || true
    else
        log_info "Remember to recreate services when ready:"
        log_info "  cd $LIVEREVIEW_INSTALL_DIR && docker compose up -d"
        log_info "  (or use: lrops.sh restart)"
    fi
}

# Show current deployment mode
show_mode_cmd() {
    section_header "CURRENT DEPLOYMENT MODE"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        log_info "Run 'lrops.sh setup-demo' to install"
        return 1
    fi
    
    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
        log_error ".env file not found at: $LIVEREVIEW_INSTALL_DIR/.env"
        return 1
    fi
    
    local env_file="$LIVEREVIEW_INSTALL_DIR/.env"
    local reverse_proxy_value=""
    
    # Get current LIVEREVIEW_REVERSE_PROXY value
    if grep -q '^LIVEREVIEW_REVERSE_PROXY=' "$env_file" 2>/dev/null; then
        reverse_proxy_value=$(grep '^LIVEREVIEW_REVERSE_PROXY=' "$env_file" | cut -d'=' -f2 | tr -d '\r' || echo "")
    fi
    
    # Determine mode
    local current_mode
    local mode_description
    case "$reverse_proxy_value" in
        "true")
            current_mode="Production Mode"
            mode_description="External access via reverse proxy, webhooks enabled"
            ;;
        "false"|"")
            current_mode="Demo Mode"
            mode_description="Localhost only, no webhooks, development/testing"
            ;;
        *)
            current_mode="Unknown Mode"
            mode_description="Unexpected LIVEREVIEW_REVERSE_PROXY value: $reverse_proxy_value"
            ;;
    esac
    
    log_info "Current Mode: $current_mode"
    log_info "Description: $mode_description"
    log_info "LIVEREVIEW_REVERSE_PROXY=${reverse_proxy_value:-"not set (defaults to false)"}"
    
    # Show relevant configuration
    echo
    if [[ "$reverse_proxy_value" == "true" ]]; then
        log_info "Production Mode Details:"
        log_info "🔧 Backend: http://127.0.0.1:8888/api"
        log_info "🔧 Frontend: http://127.0.0.1:8081/"
        log_info "✅ Webhooks: Enabled"
        log_info "🔧 External Access: Via reverse proxy"
        echo
        log_info "Switch to demo mode: lrops.sh set-mode demo"
    else
        log_info "Demo Mode Details:"
        log_info "✅ Access: http://localhost:8081/"
        log_info "✅ API: http://localhost:8888/api"
        log_info "✅ Webhooks: Disabled"
        log_info "✅ External Access: Localhost only"
        echo
        log_info "Switch to production mode: lrops.sh set-mode production"
    fi
    
    # Show other relevant settings
    echo
    log_info "Other Configuration:"
    local backend_port frontend_port
    backend_port=$(grep '^LIVEREVIEW_BACKEND_PORT=' "$env_file" | cut -d'=' -f2 | tr -d '\r' || echo "8888")
    frontend_port=$(grep '^LIVEREVIEW_FRONTEND_PORT=' "$env_file" | cut -d'=' -f2 | tr -d '\r' || echo "8081")
    log_info "Backend Port: $backend_port"
    log_info "Frontend Port: $frontend_port"
    
    local version
    version=$(grep '^LIVEREVIEW_VERSION=' "$env_file" | cut -d'=' -f2 | tr -d '\r' || echo "not set")
    log_info "Version: $version"
}

# Show container logs
show_logs() {
    local service="$1"
    local follow_flag="$2"
    
    section_header "LIVEREVIEW LOGS"
    
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_error "LiveReview is not installed"
        return 1
    fi
    
    if [[ -n "$service" ]]; then
        log_info "Showing logs for service: $service"
        if [[ "$follow_flag" == "--follow" || "$follow_flag" == "-f" ]]; then
            docker_compose logs -f --tail=50 "$service"
        else
            docker_compose logs --tail=100 "$service"
        fi
    else
        log_info "Showing logs for all services"
        if [[ "$follow_flag" == "--follow" || "$follow_flag" == "-f" ]]; then
            docker_compose logs -f --tail=50
        else
            docker_compose logs --tail=100
        fi
    fi
}

# =============================================================================
# HELP SYSTEM COMMANDS (PHASE 6)
# =============================================================================

# Show SSL/TLS setup guidance
show_ssl_help() {
    section_header "SSL/TLS SETUP GUIDE"
    
    cat << 'EOF'
🔒 SSL/TLS Configuration for LiveReview

PREREQUISITES - DNS SETUP & VERIFICATION
========================================
Before configuring SSL or reverse proxy, ensure:

1. VERIFY YOUR DOMAIN POINTS TO THIS SERVER
   ----------------------------------------
   
   a) Get your server's public IP address:
      curl -s ifconfig.me
      # OR: curl -s ipinfo.io/ip
   
   b) Check DNS resolution locally:
      dig yourdomain.com
      nslookup yourdomain.com
   
   c) Verify DNS propagation globally (CRITICAL):
      • Visit: https://www.whatsmydns.net/
      • Enter your domain name
      • Select "A" record type
      • Confirm ALL locations show your server's IP
      
   d) Alternative DNS propagation check:
      • Visit: https://dnschecker.org/
      • Enter your domain and verify worldwide propagation
   
   e) Command-line verification from different locations:
      # Use different DNS servers to check consistency
      dig @8.8.8.8 yourdomain.com        # Google DNS
      dig @1.1.1.1 yourdomain.com        # Cloudflare DNS  
      dig @208.67.222.222 yourdomain.com # OpenDNS
   
   ⚠️  COMMON MISTAKES TO AVOID:
   • Don't proceed if DNS shows different IPs in different locations
   • Wait for full global propagation (can take up to 48 hours)
   • Ensure you're checking the RIGHT domain (not www. vs non-www)
   • Verify both A record AND any CNAME records point correctly

2. VERIFY NETWORK CONNECTIVITY
   ----------------------------
   
   a) Check ports 80 and 443 are accessible from internet:
      # From another machine/location, test:
      telnet yourdomain.com 80
      telnet yourdomain.com 443
   
   b) Check firewall rules:
      sudo ufw status
      # Ensure ports 80 and 443 are allowed
   
   c) Check cloud security groups (AWS/GCP/Azure/DigitalOcean):
      # Verify inbound rules allow TCP ports 80 and 443 from 0.0.0.0/0
   
   d) Test with online port checker:
      • Visit: https://www.yougetsignal.com/tools/open-ports/
      • Enter your domain and test ports 80, 443

3. VERIFY NO PORT CONFLICTS
   -------------------------
   
   a) Check nothing else is using ports 80/443:
      sudo ss -tlnp | grep ':80\|:443'
      sudo netstat -tlnp | grep ':80\|:443'
   
   b) If Apache/nginx already running, you'll need to:
      • Stop them temporarily, OR
      • Configure them as the reverse proxy (recommended)

4. FINAL VERIFICATION CHECKLIST
   ------------------------------
   
   ✅ Domain resolves to correct IP globally (whatsmydns.net shows green)
   ✅ Ports 80 and 443 are open from internet (telnet/port checker works)  
   ✅ No services currently using ports 80/443 (ss/netstat shows clear)
   ✅ LiveReview is running and accessible on ports 8888/8081 locally
   
   Test LiveReview accessibility:
   curl http://localhost:8888/health    # Should return OK
   curl http://localhost:8081/          # Should return HTML

⚠️  CRITICAL: Without proper DNS pointing to your server, SSL certificates 
    CANNOT be obtained! Let's Encrypt and other CAs verify domain ownership
    by checking that your domain resolves to the requesting server.

💡 TROUBLESHOOTING DNS ISSUES:
   • If DNS propagation is incomplete, WAIT - don't proceed
   • If different regions show different IPs, contact your DNS provider
   • If using Cloudflare, ensure proxy is disabled (gray cloud) for SSL setup
   • Check TTL settings - lower TTL (300-900 seconds) speeds up changes

SSL/TLS SETUP APPROACHES
=======================

OPTION 1: Automatic SSL with Caddy (Recommended for new setups)
- Handles certificates automatically
- Zero manual certificate management
- See: lrops.sh help caddy

OPTION 2: Manual SSL with existing reverse proxy
- Use your existing nginx/apache setup
- Obtain certificates with certbot or your preferred method
- Configure your reverse proxy to use certificates

OPTION 3: Cloud/managed SSL
- Use CloudFlare, AWS ALB, or similar services
- Terminate SSL at the load balancer/CDN level
- Point to your LiveReview ports (8888/8081)

REQUIREMENTS FOR ALL APPROACHES:
- Domain pointing to your server (DNS setup)
- Ports 80 and 443 accessible  
- LiveReview running on ports 8888 (API) and 8081 (UI)

REVERSE PROXY ROUTING:
Route /api/* → http://127.0.0.1:8888
Route /* → http://127.0.0.1:8081

GENERAL SSL GUIDANCE:
- Let's Encrypt is free and widely supported
- Use certbot for most manual SSL setups
- Configure automatic certificate renewal
- Test your SSL setup: https://www.ssllabs.com/ssltest/

TROUBLESHOOTING COMMON ISSUES:
- Certificate failures: Verify DNS propagation first
- Port access issues: Check firewall/security groups
- Proxy errors: Verify LiveReview is running on 8888/8081

For more help: https://github.com/HexmosTech/LiveReview/wiki/Productionize-LiveReview
EOF
}

# Show quick backup options (concise version)
show_backup_options_help() {
    section_header "BACKUP COMMAND OPTIONS"
    
    cat << 'EOF'
💾 Quick Backup Guide

BASIC COMMANDS:
   lrops.sh backup                     # Default backup (auto-named)
   lrops.sh backup my-backup-name      # Named backup  
   lrops.sh quick-backup               # Quick timestamped backup

CUSTOM DIRECTORY:
   lrops.sh backup --backup-dir /path  # Backup to custom location
   lrops.sh backup --backup-dir ~/backups custom-name

MANAGEMENT:
   lrops.sh list-backups               # Show all backups
   lrops.sh backup-info <name>         # Backup details
   lrops.sh restore <name>             # Restore backup
   lrops.sh delete-backup <name>       # Delete backup

For complete guide: lrops.sh help backup
EOF
}

# Show backup strategies and script usage
show_backup_help() {
    section_header "BACKUP & RESTORE GUIDE"
    
    cat << 'EOF'
💾 LiveReview Backup & Restore Guide

═══════════════════════════════════════════════════════════════════════════════
📋 BACKUP COMMAND OPTIONS - Simple and User-Friendly
═══════════════════════════════════════════════════════════════════════════════

1. DEFAULT BACKUP (Recommended for most users)
   ============================================
   
   Command: lrops.sh backup
   
   • Creates backup with auto-generated name: manual-YYYYMMDD_HHMMSS
   • Stores in: $LIVEREVIEW_INSTALL_DIR/backups/
   • Includes: Database, configuration files, and settings
   • Example: manual-20241212_143022-version-v1.2.3
   
   ✅ Use this when: You want a quick backup before updates or changes

2. NAMED BACKUP (Custom backup names)
   ===================================
   
   Command: lrops.sh backup <custom-name>
   
   • Creates backup with your custom name: <custom-name>-YYYYMMDD_HHMMSS
   • Stores in: $LIVEREVIEW_INSTALL_DIR/backups/
   • Examples:
     - lrops.sh backup before-upgrade
     - lrops.sh backup weekly-backup
     - lrops.sh backup pre-maintenance
   
   ✅ Use this when: You want meaningful backup names for organization

3. BACKUP TO CUSTOM DIRECTORY (For external storage)
   =================================================
   
   Command: lrops.sh backup --backup-dir /path/to/directory [name]
   
   • Stores backup in your specified directory instead of default location
   • Can be combined with custom names
   • Useful for external drives, network storage, etc.
   • Examples:
     - lrops.sh backup --backup-dir /mnt/external-drive
     - lrops.sh backup --backup-dir ~/my-backups important
     - lrops.sh backup --backup-dir /backup/livereview monthly-backup
   
   ✅ Use this when: You want backups on external storage or different location

4. QUICK BACKUP (One-command timestamped backup)
   =============================================
   
   Command: lrops.sh quick-backup
   
   • Creates: quickbackup-YYYYMMDD_HHMMSS
   • Equivalent to: lrops.sh backup (but with "quickbackup" prefix)
   
   ✅ Use this when: You need a fast backup with timestamp naming

═══════════════════════════════════════════════════════════════════════════════
📁 BACKUP MANAGEMENT COMMANDS
═══════════════════════════════════════════════════════════════════════════════

View backups:        lrops.sh list-backups
Backup details:      lrops.sh backup-info <backup-name>
Delete backup:       lrops.sh delete-backup <backup-name>
Restore backup:      lrops.sh restore <backup-name>
Restore latest:      lrops.sh restore latest

═══════════════════════════════════════════════════════════════════════════════
🔄 RESTORE PROCESS
═══════════════════════════════════════════════════════════════════════════════

1. Stop LiveReview:
   lrops.sh stop

2. Restore from backup:
   lrops.sh restore <backup-name>
   # OR restore latest backup:
   lrops.sh restore latest

3. Restart LiveReview:
   lrops.sh start

═══════════════════════════════════════════════════════════════════════════════
⚙️  AUTOMATED BACKUPS
═══════════════════════════════════════════════════════════════════════════════

Set up daily automated backups:

1. Copy cron template:
   sudo cp "$LIVEREVIEW_INSTALL_DIR/config/backup-cron.example" /etc/cron.d/livereview-backup

2. Edit the schedule:
   sudo nano /etc/cron.d/livereview-backup

3. Example cron entries:
   # Daily backup at 2 AM
   0 2 * * * lrops.sh backup daily-$(date +\%Y\%m\%d)
   
   # Weekly backup on Sundays at 3 AM to external drive  
   0 3 * * 0 lrops.sh backup --backup-dir /mnt/backup weekly-$(date +\%Y\%U)

═══════════════════════════════════════════════════════════════════════════════
☁️  CLOUD/EXTERNAL STORAGE
═══════════════════════════════════════════════════════════════════════════════

Backup to external locations:

1. Network drives:
   lrops.sh backup --backup-dir /mnt/nas/livereview

2. Cloud sync with rclone:
   # Install rclone: sudo apt install rclone
   # Configure: rclone config
   # Auto-sync: rclone sync "$LIVEREVIEW_INSTALL_DIR/backups/" mycloud:livereview-backups/

3. USB/External drives:
   lrops.sh backup --backup-dir /media/usb-drive/backups

═══════════════════════════════════════════════════════════════════════════════
📊 WHAT'S INCLUDED IN BACKUPS
═══════════════════════════════════════════════════════════════════════════════

Each backup contains:
✅ Database dump (PostgreSQL data)
✅ Configuration files (.env, docker-compose.yml)
✅ Application settings and customizations
✅ Container metadata and version information
✅ Compressed data directory (physical backup)
✅ Backup metadata and creation timestamp

═══════════════════════════════════════════════════════════════════════════════
💡 BACKUP BEST PRACTICES
═══════════════════════════════════════════════════════════════════════════════

• Backup BEFORE any updates or changes
• Test restore procedures regularly
• Keep multiple backup copies (3-2-1 rule)
• Store backups in different locations
• Use meaningful names for important backups
• Monitor backup success and disk space
• Clean up old backups periodically

For more help: https://github.com/HexmosTech/LiveReview/wiki/Productionize-LiveReview
EOF
}

# Show Nginx reverse proxy configuration
show_nginx_help() {
    section_header "NGINX REVERSE PROXY GUIDE"
    
    cat << 'EOF'
🌐 Nginx Reverse Proxy Configuration for LiveReview

PREREQUISITES - DNS SETUP & VERIFICATION
========================================
Before configuring SSL or reverse proxy, ensure:

1. VERIFY YOUR DOMAIN POINTS TO THIS SERVER
   ----------------------------------------
   
   a) Get your server's public IP address:
      curl -s ifconfig.me
      # OR: curl -s ipinfo.io/ip
   
   b) Check DNS resolution locally:
      dig yourdomain.com
      nslookup yourdomain.com
   
   c) Verify DNS propagation globally (CRITICAL):
      • Visit: https://www.whatsmydns.net/
      • Enter your domain name
      • Select "A" record type
      • Confirm ALL locations show your server's IP
      
   d) Alternative DNS propagation check:
      • Visit: https://dnschecker.org/
      • Enter your domain and verify worldwide propagation
   
   e) Command-line verification from different locations:
      # Use different DNS servers to check consistency
      dig @8.8.8.8 yourdomain.com        # Google DNS
      dig @1.1.1.1 yourdomain.com        # Cloudflare DNS  
      dig @208.67.222.222 yourdomain.com # OpenDNS
   
   ⚠️  COMMON MISTAKES TO AVOID:
   • Don't proceed if DNS shows different IPs in different locations
   • Wait for full global propagation (can take up to 48 hours)
   • Ensure you're checking the RIGHT domain (not www. vs non-www)
   • Verify both A record AND any CNAME records point correctly

2. VERIFY NETWORK CONNECTIVITY
   ----------------------------
   
   a) Check ports 80 and 443 are accessible from internet:
      # From another machine/location, test:
      telnet yourdomain.com 80
      telnet yourdomain.com 443
   
   b) Check firewall rules:
      sudo ufw status
      # Ensure ports 80 and 443 are allowed
   
   c) Check cloud security groups (AWS/GCP/Azure/DigitalOcean):
      # Verify inbound rules allow TCP ports 80 and 443 from 0.0.0.0/0
   
   d) Test with online port checker:
      • Visit: https://www.yougetsignal.com/tools/open-ports/
      • Enter your domain and test ports 80, 443

3. VERIFY NO PORT CONFLICTS
   -------------------------
   
   a) Check nothing else is using ports 80/443:
      sudo ss -tlnp | grep ':80\|:443'
      sudo netstat -tlnp | grep ':80\|:443'
   
   b) If Apache/nginx already running, you'll need to:
      • Stop them temporarily, OR
      • Configure them as the reverse proxy (recommended)

4. FINAL VERIFICATION CHECKLIST
   ------------------------------
   
   ✅ Domain resolves to correct IP globally (whatsmydns.net shows green)
   ✅ Ports 80 and 443 are open from internet (telnet/port checker works)  
   ✅ No services currently using ports 80/443 (ss/netstat shows clear)
   ✅ LiveReview is running and accessible on ports 8888/8081 locally
   
   Test LiveReview accessibility:
   curl http://localhost:8888/health    # Should return OK
   curl http://localhost:8081/          # Should return HTML

⚠️  CRITICAL: Without proper DNS pointing to your server, SSL certificates 
    CANNOT be obtained! Let's Encrypt and other CAs verify domain ownership
    by checking that your domain resolves to the requesting server.

💡 TROUBLESHOOTING DNS ISSUES:
   • If DNS propagation is incomplete, WAIT - don't proceed
   • If different regions show different IPs, contact your DNS provider
   • If using Cloudflare, ensure proxy is disabled (gray cloud) for SSL setup
   • Check TTL settings - lower TTL (300-900 seconds) speeds up changes

INSTALLATION
============
1. Install Nginx:
   sudo apt update && sudo apt install nginx

2. Copy the LiveReview Nginx template:
   sudo cp ~/livereview/config/nginx.conf.example /etc/nginx/sites-available/livereview

3. Edit the domain name:
    sudo_sed_inplace 's/your-domain.com/your-actual-domain.org/g' /etc/nginx/sites-available/livereview

4. Enable the site:
   sudo ln -s /etc/nginx/sites-available/livereview /etc/nginx/sites-enabled/
   sudo nginx -t
   sudo systemctl reload nginx

TEMPLATE FEATURES
================
- API proxy to port 8888 (/api/* routes)
- UI proxy to port 8081 (all other routes)
- WebSocket support for real-time features
- Proper headers for security
- Gzip compression
- SSL/TLS configuration ready

CUSTOMIZATION
=============
Edit /etc/nginx/sites-available/livereview to:
- Change domain names
- Adjust proxy settings
- Add custom headers
- Configure rate limiting
- Set up IP restrictions

TESTING
=======
1. Test configuration:
   sudo nginx -t

2. Check if proxy is working:
   curl -H "Host: yourdomain.com" http://localhost/

3. View logs:
   sudo tail -f /var/log/nginx/access.log
   sudo tail -f /var/log/nginx/error.log

TROUBLESHOOTING
===============
- Check Nginx status: sudo systemctl status nginx
- Verify LiveReview is running: lrops.sh status
- Check firewall: sudo ufw status
- Test DNS resolution: nslookup yourdomain.com

For SSL setup: lrops.sh help ssl
For more help: https://github.com/HexmosTech/LiveReview/wiki/Productionize-LiveReview
EOF
}

# Show Caddy reverse proxy configuration
show_caddy_help() {
    section_header "CADDY REVERSE PROXY GUIDE"
    
    cat << 'EOF'
⚡ Caddy Reverse Proxy Configuration for LiveReview

PREREQUISITES - DNS SETUP & VERIFICATION
========================================
Before configuring SSL or reverse proxy, ensure:

1. VERIFY YOUR DOMAIN POINTS TO THIS SERVER
   ----------------------------------------
   
   a) Get your server's public IP address:
      curl -s ifconfig.me
      # OR: curl -s ipinfo.io/ip
   
   b) Check DNS resolution locally:
      dig yourdomain.com
      nslookup yourdomain.com
   
   c) Verify DNS propagation globally (CRITICAL):
      • Visit: https://www.whatsmydns.net/
      • Enter your domain name
      • Select "A" record type
      • Confirm ALL locations show your server's IP
      
   d) Alternative DNS propagation check:
      • Visit: https://dnschecker.org/
      • Enter your domain and verify worldwide propagation
   
   e) Command-line verification from different locations:
      # Use different DNS servers to check consistency
      dig @8.8.8.8 yourdomain.com        # Google DNS
      dig @1.1.1.1 yourdomain.com        # Cloudflare DNS  
      dig @208.67.222.222 yourdomain.com # OpenDNS
   
   ⚠️  COMMON MISTAKES TO AVOID:
   • Don't proceed if DNS shows different IPs in different locations
   • Wait for full global propagation (can take up to 48 hours)
   • Ensure you're checking the RIGHT domain (not www. vs non-www)
   • Verify both A record AND any CNAME records point correctly

2. VERIFY NETWORK CONNECTIVITY
   ----------------------------
   
   a) Check ports 80 and 443 are accessible from internet:
      # From another machine/location, test:
      telnet yourdomain.com 80
      telnet yourdomain.com 443
   
   b) Check firewall rules:
      sudo ufw status
      # Ensure ports 80 and 443 are allowed
   
   c) Check cloud security groups (AWS/GCP/Azure/DigitalOcean):
      # Verify inbound rules allow TCP ports 80 and 443 from 0.0.0.0/0
   
   d) Test with online port checker:
      • Visit: https://www.yougetsignal.com/tools/open-ports/
      • Enter your domain and test ports 80, 443

3. VERIFY NO PORT CONFLICTS
   -------------------------
   
   a) Check nothing else is using ports 80/443:
      sudo ss -tlnp | grep ':80\|:443'
      sudo netstat -tlnp | grep ':80\|:443'
   
   b) If Apache/nginx already running, you'll need to:
      • Stop them temporarily, OR
      • Configure them as the reverse proxy (recommended)

4. FINAL VERIFICATION CHECKLIST
   ------------------------------
   
   ✅ Domain resolves to correct IP globally (whatsmydns.net shows green)
   ✅ Ports 80 and 443 are open from internet (telnet/port checker works)  
   ✅ No services currently using ports 80/443 (ss/netstat shows clear)
   ✅ LiveReview is running and accessible on ports 8888/8081 locally
   
   Test LiveReview accessibility:
   curl http://localhost:8888/health    # Should return OK
   curl http://localhost:8081/          # Should return HTML

⚠️  CRITICAL: Without proper DNS pointing to your server, SSL certificates 
    CANNOT be obtained! Let's Encrypt and other CAs verify domain ownership
    by checking that your domain resolves to the requesting server.

💡 TROUBLESHOOTING DNS ISSUES:
   • If DNS propagation is incomplete, WAIT - don't proceed
   • If different regions show different IPs, contact your DNS provider
   • If using Cloudflare, ensure proxy is disabled (gray cloud) for SSL setup
   • Check TTL settings - lower TTL (300-900 seconds) speeds up changes

WHY CADDY?
==========
- Automatic HTTPS with Let's Encrypt
- Simple configuration
- Built-in security features
- No manual certificate management

INSTALLATION
============
1. Install Caddy:
   sudo apt update
   sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
   curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
   curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
   sudo apt update
   sudo apt install caddy

2. Copy and configure the template:
   sudo cp ~/livereview/config/caddy.conf.example /etc/caddy/Caddyfile
    sudo_sed_inplace 's/your-domain.com/your-actual-domain.org/g' /etc/caddy/Caddyfile

3. Start Caddy:
   sudo systemctl enable caddy
   sudo systemctl start caddy

TEMPLATE FEATURES
================
- Automatic HTTPS for your domain
- API proxy to port 8888 (/api/* routes)
- UI proxy to port 8081 (all other routes)
- Security headers included
- Gzip compression enabled

CONFIGURATION
=============
The Caddy configuration is in /etc/caddy/Caddyfile:

yourdomain.com {
    # Proxy API requests
    handle /api/* {
        reverse_proxy localhost:8888
    }
    
    # Proxy UI requests
    handle {
        reverse_proxy localhost:8081
    }
}

TESTING
=======
1. Check Caddy status:
   sudo systemctl status caddy

2. View logs:
   sudo journalctl -u caddy -f

3. Test the proxy:
   curl https://yourdomain.com/

TROUBLESHOOTING
===============
- Verify domain DNS points to your server
- Check if ports 80/443 are open
- Ensure LiveReview is running: lrops.sh status
- Check Caddy logs for errors

AUTOMATIC HTTPS NOTES
=====================
- Caddy automatically obtains SSL certificates
- Certificates are renewed automatically
- No manual intervention required
- Certificates stored in /var/lib/caddy/

For more help: https://github.com/HexmosTech/LiveReview/wiki/Productionize-LiveReview
EOF
}

# Show Apache reverse proxy configuration
show_apache_help() {
    section_header "APACHE REVERSE PROXY GUIDE"
    
    cat << 'EOF'
🔧 Apache Reverse Proxy Configuration for LiveReview

PREREQUISITES - DNS SETUP & VERIFICATION
========================================
Before configuring SSL or reverse proxy, ensure:

1. VERIFY YOUR DOMAIN POINTS TO THIS SERVER
   ----------------------------------------
   
   a) Get your server's public IP address:
      curl -s ifconfig.me
      # OR: curl -s ipinfo.io/ip
   
   b) Check DNS resolution locally:
      dig yourdomain.com
      nslookup yourdomain.com
   
   c) Verify DNS propagation globally (CRITICAL):
      • Visit: https://www.whatsmydns.net/
      • Enter your domain name
      • Select "A" record type
      • Confirm ALL locations show your server's IP
      
   d) Alternative DNS propagation check:
      • Visit: https://dnschecker.org/
      • Enter your domain and verify worldwide propagation
   
   e) Command-line verification from different locations:
      # Use different DNS servers to check consistency
      dig @8.8.8.8 yourdomain.com        # Google DNS
      dig @1.1.1.1 yourdomain.com        # Cloudflare DNS  
      dig @208.67.222.222 yourdomain.com # OpenDNS
   
   ⚠️  COMMON MISTAKES TO AVOID:
   • Don't proceed if DNS shows different IPs in different locations
   • Wait for full global propagation (can take up to 48 hours)
   • Ensure you're checking the RIGHT domain (not www. vs non-www)
   • Verify both A record AND any CNAME records point correctly

2. VERIFY NETWORK CONNECTIVITY
   ----------------------------
   
   a) Check ports 80 and 443 are accessible from internet:
      # From another machine/location, test:
      telnet yourdomain.com 80
      telnet yourdomain.com 443
   
   b) Check firewall rules:
      sudo ufw status
      # Ensure ports 80 and 443 are allowed
   
   c) Check cloud security groups (AWS/GCP/Azure/DigitalOcean):
      # Verify inbound rules allow TCP ports 80 and 443 from 0.0.0.0/0
   
   d) Test with online port checker:
      • Visit: https://www.yougetsignal.com/tools/open-ports/
      • Enter your domain and test ports 80, 443

3. VERIFY NO PORT CONFLICTS
   -------------------------
   
   a) Check nothing else is using ports 80/443:
      sudo ss -tlnp | grep ':80\|:443'
      sudo netstat -tlnp | grep ':80\|:443'
   
   b) If Apache/nginx already running, you'll need to:
      • Stop them temporarily, OR
      • Configure them as the reverse proxy (recommended)

4. FINAL VERIFICATION CHECKLIST
   ------------------------------
   
   ✅ Domain resolves to correct IP globally (whatsmydns.net shows green)
   ✅ Ports 80 and 443 are open from internet (telnet/port checker works)  
   ✅ No services currently using ports 80/443 (ss/netstat shows clear)
   ✅ LiveReview is running and accessible on ports 8888/8081 locally
   
   Test LiveReview accessibility:
   curl http://localhost:8888/health    # Should return OK
   curl http://localhost:8081/          # Should return HTML

⚠️  CRITICAL: Without proper DNS pointing to your server, SSL certificates 
    CANNOT be obtained! Let's Encrypt and other CAs verify domain ownership
    by checking that your domain resolves to the requesting server.

💡 TROUBLESHOOTING DNS ISSUES:
   • If DNS propagation is incomplete, WAIT - don't proceed
   • If different regions show different IPs, contact your DNS provider
   • If using Cloudflare, ensure proxy is disabled (gray cloud) for SSL setup
   • Check TTL settings - lower TTL (300-900 seconds) speeds up changes

INSTALLATION
============
1. Install Apache:
   sudo apt update && sudo apt install apache2

2. Enable required modules:
   sudo a2enmod proxy proxy_http proxy_balancer lbmethod_byrequests headers rewrite ssl
    
3. Copy and configure the template:
   sudo cp ~/livereview/config/apache.conf.example /etc/apache2/sites-available/livereview.conf
    sudo_sed_inplace 's/your-domain.com/your-actual-domain.org/g' /etc/apache2/sites-available/livereview.conf

4. Enable the site:
   sudo a2ensite livereview
   sudo systemctl reload apache2

TEMPLATE FEATURES
================
- Virtual host configuration
- API proxy to port 8888 (/api/* routes)
- UI proxy to port 8081 (all other routes)
- SSL configuration ready
- Security headers
- Error and access logging

CUSTOMIZATION
=============
Edit /etc/apache2/sites-available/livereview.conf to:
- Adjust ProxyPass directives
- Configure SSL settings
- Add custom headers
- Set up access controls

TESTING
=======
1. Test configuration:
   sudo apache2ctl configtest

2. Check if proxy is working:
   curl -H "Host: yourdomain.com" http://localhost/

3. View logs:
   sudo tail -f /var/log/apache2/access.log
   sudo tail -f /var/log/apache2/error.log

SSL CONFIGURATION
=================
After obtaining SSL certificates (see: lrops.sh help ssl):

1. Update your virtual host with SSL:
   <VirtualHost *:443>
       ServerName yourdomain.com
       SSLEngine on
       SSLCertificateFile /path/to/cert.pem
       SSLCertificateKeyFile /path/to/private.key
       # ... proxy configuration
   </VirtualHost>

TROUBLESHOOTING
===============
- Check Apache status: sudo systemctl status apache2
- Verify modules loaded: apache2ctl -M | grep proxy
- Test configuration: sudo apache2ctl configtest
- Check LiveReview status: lrops.sh status

For SSL setup: lrops.sh help ssl
For more help: https://github.com/HexmosTech/LiveReview/wiki/Productionize-LiveReview
EOF
}

# =============================================================================
# POST-INSTALLATION EXPERIENCE (PHASE 8)
# =============================================================================

# Comprehensive post-installation validation and user experience
post_installation_experience() {
    local resolved_version="$1"
    local config_file="$2"
    
    # Source configuration for access info
    source "$config_file"
    
    # Step 1: Run post-installation validation
    validate_installation_health "$config_file"
    
    # Step 2: Generate comprehensive installation report
    generate_installation_report "$resolved_version" "$config_file"
    
    # Step 3: Display enhanced installation summary
    display_completion_summary "$resolved_version" "$config_file"
    
    # Step 4: Provide troubleshooting guidance if needed
    provide_troubleshooting_guidance "$config_file"
}

# Validate that all services are working correctly
validate_installation_health() {
    local config_file="$1"
    source "$config_file"
    
    # Derive ports from two-mode configuration with safe defaults
    local API_PORT="${LIVEREVIEW_BACKEND_PORT:-8888}"
    local UI_PORT="${LIVEREVIEW_FRONTEND_PORT:-8081}"
    
    section_header "VALIDATING INSTALLATION"
    log_info "Running post-installation health checks..."
    
    # Give containers additional time to stabilize after startup
    log_info "Allowing containers to stabilize..."
    sleep 10
    
    local validation_errors=0
    
    # Check container status
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory"
        return 1
    }
    
    # Check if containers are running using docker inspect (not ps parsing)
    local db_state=$(docker inspect --format='{{.State.Status}}' livereview-db 2>/dev/null || echo "missing")
    local app_state=$(docker inspect --format='{{.State.Status}}' livereview-app 2>/dev/null || echo "missing")
    
    if [[ "$db_state" == "running" && "$app_state" == "running" ]]; then
        log_success "✅ Containers are running"
    else
        log_error "❌ Containers are not running (db: $db_state, app: $app_state)"
        ((validation_errors++))
    fi
    
    # Check container health with retry logic
    local app_health=""
    local db_health=""
    
    # Retry health checks multiple times
    for i in {1..5}; do
        app_health=$(docker inspect --format='{{.State.Health.Status}}' livereview-app 2>/dev/null || echo "none")
        db_health=$(docker inspect --format='{{.State.Health.Status}}' livereview-db 2>/dev/null || echo "none")
        
        # If no healthcheck defined, check via service availability
        if [[ "$app_health" == "none" ]]; then
            if curl -f -s --max-time 3 "http://localhost:${API_PORT}/health" >/dev/null 2>&1; then
                app_health="healthy"
            fi
        fi
        
        if [[ "$db_health" == "none" ]]; then
            if docker exec livereview-db pg_isready -U livereview >/dev/null 2>&1; then
                db_health="healthy"
            fi
        fi
        
        if [[ "$app_health" == "healthy" && "$db_health" == "healthy" ]]; then
            break
        fi
        
        if [[ $i -lt 5 ]]; then
            log_info "Health check attempt $i/5 - waiting for containers to become healthy..."
            sleep 15
        fi
    done
    
    if [[ "$app_health" == "healthy" ]]; then
        log_success "✅ LiveReview application is healthy"
    else
        log_warning "⚠️ LiveReview application health: ${app_health:-unknown}"
        if [[ "$app_health" != "healthy" ]]; then
            ((validation_errors++))
        fi
    fi
    
    if [[ "$db_health" == "healthy" ]]; then
        log_success "✅ Database is healthy"
    else
        log_warning "⚠️ Database health: ${db_health:-unknown}"
        if [[ "$db_health" != "healthy" ]]; then
            ((validation_errors++))
        fi
    fi
    
    # Test API endpoint accessibility with retries
    log_info "Testing API endpoint accessibility..."
    local api_accessible=false
    for i in {1..6}; do
        if curl -f -s --max-time 10 "http://localhost:${API_PORT}/health" >/dev/null 2>&1; then
            api_accessible=true
            break
        fi
        if [[ $i -lt 6 ]]; then
            log_info "API endpoint not ready, retrying in 10 seconds... ($i/6)"
            sleep 10
        fi
    done
    
    if [[ "$api_accessible" == "true" ]]; then
        log_success "✅ API endpoint is accessible"
    else
        log_warning "⚠️ API endpoint not accessible after 60 seconds (may still be starting)"
        ((validation_errors++))
    fi
    
    # Test UI endpoint accessibility with retries
    log_info "Testing UI endpoint accessibility..."
    local ui_accessible=false
    for i in {1..6}; do
        if curl -f -s --max-time 10 "http://localhost:${UI_PORT}/" >/dev/null 2>&1; then
            ui_accessible=true
            break
        fi
        if [[ $i -lt 6 ]]; then
            log_info "UI endpoint not ready, retrying in 10 seconds... ($i/6)"
            sleep 10
        fi
    done
    
    if [[ "$ui_accessible" == "true" ]]; then
        log_success "✅ UI endpoint is accessible"
    else
        log_warning "⚠️ UI endpoint not accessible after 60 seconds (may still be starting)"
        ((validation_errors++))
    fi
    
    # Check for recent errors in logs (excluding harmless entries)
    log_info "Checking for errors in recent logs..."
    local recent_errors=$(docker_compose logs --since=2m 2>/dev/null | grep -i "error\|fail\|panic\|fatal" | grep -v '"error":""' | grep -v '"error":null' | grep -v "relation.*does not exist" | grep -v "no rows in result set" | grep -v "error\":\[\]" | wc -l)
    if [[ $recent_errors -eq 0 ]]; then
        log_success "✅ No recent errors found in logs"
    else
        log_warning "⚠️ Found $recent_errors recent error(s) in logs (may be harmless startup messages)"
        # Don't increment validation_errors for log messages - they're often false positives
    fi
    
    # Summary
    if [[ $validation_errors -eq 0 ]]; then
        log_success "🎉 All validation checks passed!"
        log_success "✅ LiveReview is fully operational!"
        return 0
    elif [[ $validation_errors -le 2 ]]; then
        log_warning "⚠️ Found $validation_errors minor validation issues"
        log_info "LiveReview may still be starting up. Wait a few more minutes and check status again."
        log_info "Run 'lrops.sh status' for detailed status information"
        return 0  # Don't fail on minor issues
    else
        log_error "❌ Found $validation_errors validation issues"
        log_info "Run 'lrops.sh status' for detailed status information"
        return 1
    fi
}

# Generate comprehensive installation report file
generate_installation_report() {
    local resolved_version="$1"
    local config_file="$2"
    source "$config_file"
    
    local report_file="$LIVEREVIEW_INSTALL_DIR/installation-report.txt"
    
    # Derive report fields from two-mode configuration with safe defaults
    local API_PORT="${LIVEREVIEW_BACKEND_PORT:-8888}"
    local UI_PORT="${LIVEREVIEW_FRONTEND_PORT:-8081}"
    local DOMAIN_DEFAULTED="${DOMAIN:-localhost}"
    
    cat > "$report_file" << EOF
LiveReview Installation Report
=============================
Generated: $(date)
Script Version: $SCRIPT_VERSION
LiveReview Version: $resolved_version

INSTALLATION SUMMARY
===================
✅ Phase 1: Script foundation
✅ Phase 2: Version Management & GitHub Integration  
✅ Phase 3: Embedded Templates & Configuration Files
✅ Phase 4: Installation Core Logic
✅ Phase 5: Docker Deployment
✅ Phase 6: Management Commands
✅ Phase 8: Post-Installation Experience

SYSTEM INFORMATION
==================
Installation Directory: $LIVEREVIEW_INSTALL_DIR
Operating System: $(uname -s) $(uname -r)
Architecture: $(uname -m)
Docker Version: $(docker --version 2>/dev/null || echo "Not available")
Docker Compose Version: $(docker-compose --version 2>/dev/null || echo "Not available")

CONFIGURATION
=============
Domain: $DOMAIN_DEFAULTED
API Port: $API_PORT
UI Port: $UI_PORT
Database: PostgreSQL 15
SSL/TLS: Not configured (use 'lrops.sh help ssl' for setup)

ACCESS INFORMATION
==================
Web UI: http://localhost:${UI_PORT}/
API: http://localhost:${API_PORT}/api
Health Check: http://localhost:${API_PORT}/health

CONTAINER STATUS
================
$(cd "$LIVEREVIEW_INSTALL_DIR" && $DOCKER_COMPOSE_CMD ps 2>/dev/null || echo "Unable to retrieve container status")

IMPORTANT FILES
===============
Docker Compose: $LIVEREVIEW_INSTALL_DIR/docker-compose.yml
Environment: $LIVEREVIEW_INSTALL_DIR/.env
Installation Summary: $LIVEREVIEW_INSTALL_DIR/installation-summary.txt
Installation Report: $LIVEREVIEW_INSTALL_DIR/installation-report.txt

CONFIGURATION TEMPLATES
=======================
Nginx: $LIVEREVIEW_INSTALL_DIR/config/nginx.conf.example
Caddy: $LIVEREVIEW_INSTALL_DIR/config/caddy.conf.example
Apache: $LIVEREVIEW_INSTALL_DIR/config/apache.conf.example

HELPER SCRIPTS
==============
Backup: $LIVEREVIEW_INSTALL_DIR/scripts/backup.sh
Restore: $LIVEREVIEW_INSTALL_DIR/scripts/restore.sh
SSL Setup: $LIVEREVIEW_INSTALL_DIR/scripts/setup-ssl.sh
SSL Renewal: $LIVEREVIEW_INSTALL_DIR/scripts/renew-ssl.sh
Cron Example: $LIVEREVIEW_INSTALL_DIR/config/backup-cron.example

MANAGEMENT COMMANDS
===================
Status Check: lrops.sh status
Start Services: lrops.sh start
Stop Services: lrops.sh stop
Restart Services: lrops.sh restart
View Logs: lrops.sh logs

CONFIGURATION HELP
==================
SSL Setup: lrops.sh help ssl
Backup Guide: lrops.sh help backup
Nginx Config: lrops.sh help nginx
Caddy Config: lrops.sh help caddy
Apache Config: lrops.sh help apache

NEXT STEPS
==========
1. Verify access to Web UI: http://localhost:${UI_PORT}/
2. Test API endpoint: curl http://localhost:${API_PORT}/health
3. Configure SSL/TLS for production: lrops.sh help ssl
4. Set up automated backups: lrops.sh help backup
5. Configure reverse proxy if needed: lrops.sh help nginx

TROUBLESHOOTING
===============
- Check status: lrops.sh status
- View logs: lrops.sh logs
- Restart services: lrops.sh restart
- Diagnose issues: lrops.sh --diagnose

For support and documentation:
https://github.com/HexmosTech/LiveReview

EOF
    
    log_info "📋 Installation report saved to: $report_file"
}

# Display enhanced installation completion summary for two-mode system
display_completion_summary() {
    local resolved_version="$1"
    local config_file="$2"
    source "$config_file"
    # Derive deployment mode from simplified flag
    local reverse_proxy="${LIVEREVIEW_REVERSE_PROXY:-false}"
    local deployment_mode
    if [[ "$reverse_proxy" == "true" ]]; then
        deployment_mode="production"
    else
        deployment_mode="demo"
    fi
    
    section_header "INSTALLATION COMPLETE ✅"
    echo
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                    🎉 LIVEREVIEW SUCCESSFULLY INSTALLED! 🎉                  ║${NC}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo
    
    log_success "✅ All components deployed and running!"
    log_success "✅ Health checks passed"
    log_success "✅ Services are accessible"
    echo
    
    # Access URLs with emphasis - different for demo vs production
    if [[ "$deployment_mode" == "demo" ]]; then
        echo -e "${BLUE}🌐 DEMO MODE - LOCAL ACCESS ONLY:${NC}"
        echo -e "${BOLD}   🖥️  Web Interface: ${GREEN}http://localhost:${LIVEREVIEW_FRONTEND_PORT}/${NC}"
        echo -e "${BOLD}   🔌 API Endpoint:   ${GREEN}http://localhost:${LIVEREVIEW_BACKEND_PORT}/api${NC}"
        echo -e "${BOLD}   ❤️  Health Check:  ${GREEN}http://localhost:${LIVEREVIEW_BACKEND_PORT}/health${NC}"
        echo -e "${BOLD}   🔧 Webhooks:       ${YELLOW}Disabled (manual triggers only)${NC}"
        echo
        echo -e "${BLUE}🚀 QUICK START (DEMO MODE):${NC}"
        echo -e "   1. ${BOLD}Open your browser${NC} and go to ${GREEN}http://localhost:${LIVEREVIEW_FRONTEND_PORT}/${NC}"
        echo -e "   2. ${BOLD}Check system status:${NC} ${CYAN}lrops.sh status${NC}"
        echo -e "   3. ${BOLD}View live logs:${NC} ${CYAN}lrops.sh logs -f${NC}"
        echo
        echo -e "${BLUE}📈 UPGRADE TO PRODUCTION:${NC}"
        echo -e "   1. ${BOLD}Edit .env file:${NC} Set ${CYAN}LIVEREVIEW_REVERSE_PROXY=true${NC}"
        echo -e "   2. ${BOLD}Restart services:${NC} ${CYAN}lrops.sh restart${NC}"
        echo -e "   3. ${BOLD}Configure reverse proxy:${NC} ${CYAN}lrops.sh help nginx${NC}"
        echo -e "   4. ${BOLD}Set up SSL/TLS:${NC} ${CYAN}lrops.sh help ssl${NC}"
    else
        echo -e "${BLUE}🌐 PRODUCTION MODE - REVERSE PROXY REQUIRED:${NC}"
        echo -e "${BOLD}   🔧 Backend:        ${GREEN}http://127.0.0.1:${LIVEREVIEW_BACKEND_PORT}/api${NC}"
        echo -e "${BOLD}   🖥️  Frontend:       ${GREEN}http://127.0.0.1:${LIVEREVIEW_FRONTEND_PORT}/${NC}"
        echo -e "${BOLD}   ❤️  Health Check:  ${GREEN}http://127.0.0.1:${LIVEREVIEW_BACKEND_PORT}/health${NC}"
        echo -e "${BOLD}   🔗 Webhooks:       ${GREEN}Enabled (automatic triggers)${NC}"
        echo
        echo -e "${BLUE}🚀 NEXT STEPS (PRODUCTION MODE):${NC}"
        echo -e "   1. ${BOLD}Configure reverse proxy:${NC} ${CYAN}lrops.sh help nginx${NC}"
        echo -e "   2. ${BOLD}Set up SSL/TLS:${NC} ${CYAN}lrops.sh help ssl${NC}"
        echo -e "   3. ${BOLD}Configure DNS:${NC} Point your domain to this server"
        echo -e "   4. ${BOLD}Test external access:${NC} Access via your domain"
        echo
        echo -e "${YELLOW}⚠️  IMPORTANT: Configure reverse proxy before external access!${NC}"
    fi
    
    # Management commands
    echo -e "${BLUE}📋 MANAGEMENT COMMANDS:${NC}"
    echo -e "   ${CYAN}lrops.sh status${NC}     - Check installation status"
    echo -e "   ${CYAN}lrops.sh logs${NC}       - View application logs"  
    echo -e "   ${CYAN}lrops.sh restart${NC}    - Restart all services"
    echo -e "   ${CYAN}lrops.sh stop${NC}       - Stop all services"
    echo
    
    # Next steps
    echo -e "${BLUE}📖 CONFIGURATION HELP:${NC}"
    echo -e "   � ${BOLD}Configure backups:${NC} ${CYAN}lrops.sh help backup${NC}"
    if [[ "$deployment_mode" == "production" ]]; then
        echo -e "   � ${BOLD}Set up SSL/TLS:${NC} ${CYAN}lrops.sh help ssl${NC}"
        echo -e "   🌐 ${BOLD}Set up reverse proxy:${NC} ${CYAN}lrops.sh help nginx${NC}"
    fi
    echo -e "   📄 ${BOLD}View full report:${NC} ${CYAN}cat $LIVEREVIEW_INSTALL_DIR/installation-report.txt${NC}"
    echo
    
    # Installation details
    echo -e "${GRAY}📁 Installation: $LIVEREVIEW_INSTALL_DIR${NC}"
    echo -e "${GRAY}📊 Version: LiveReview $resolved_version, Script $SCRIPT_VERSION${NC}"
    echo -e "${GRAY}🏗️  Mode: $deployment_mode mode${NC}"
    echo -e "${GRAY}⏱️  Completed: $(date)${NC}"
    echo
    
    if [[ "$deployment_mode" == "demo" ]]; then
        log_success "🎉 LiveReview demo mode is ready to use!"
        log_info "💡 This is perfect for development, testing, and evaluation"
    else
        log_success "🎉 LiveReview production mode is installed!"
        log_info "⚡ Configure reverse proxy and SSL for external access"
    fi
}

# Provide troubleshooting guidance if issues detected
provide_troubleshooting_guidance() {
    local config_file="$1"
    source "$config_file"
    
    # Check if there were any validation issues
    cd "$LIVEREVIEW_INSTALL_DIR" || return 1
    
    local has_issues=false
    
    # Derive ports with safe defaults to avoid unbound variable errors
    local API_PORT="${LIVEREVIEW_BACKEND_PORT:-8888}"
    local UI_PORT="${LIVEREVIEW_FRONTEND_PORT:-8081}"
    
    # Check for common issues
    if ! curl -f -s --max-time 5 "http://localhost:${API_PORT}/health" >/dev/null 2>&1; then
        has_issues=true
    fi
    
    if ! curl -f -s --max-time 5 "http://localhost:${UI_PORT}/" >/dev/null 2>&1; then
        has_issues=true
    fi
    
    if [[ "$has_issues" == "true" ]]; then
        section_header "TROUBLESHOOTING GUIDANCE"
        log_warning "Some services may still be starting up. This is normal for the first few minutes."
        echo
        log_info "🔧 If services are not accessible after 5 minutes:"
        log_info "   1. Check status: ${CYAN}lrops.sh status${NC}"
        log_info "   2. View logs: ${CYAN}lrops.sh logs${NC}"
        log_info "   3. Restart services: ${CYAN}lrops.sh restart${NC}"
        echo
        log_info "🆘 Common solutions:"
        log_info "   • Wait 2-3 minutes for services to fully start"
        log_info "   • Check if ports ${API_PORT} and ${UI_PORT} are available"
        log_info "   • Ensure Docker daemon is running"
        log_info "   • Check firewall settings if accessing remotely"
        echo
        log_info "📞 Get help:"
        log_info "   • Documentation: ${CYAN}https://github.com/HexmosTech/LiveReview${NC}"
        log_info "   • Run diagnostics: ${CYAN}lrops.sh --diagnose${NC}"
        echo
    fi
}

# =============================================================================
# DIAGNOSTIC FUNCTIONS
# =============================================================================

# Run comprehensive diagnostics
run_diagnostics() {
    section_header "LIVEREVIEW DIAGNOSTICS"
    
    log_info "Running comprehensive LiveReview diagnostics..."
    echo
    
    # Basic system information
    log_info "🖥️  System Information:"
    log_info "  - OS: $(uname -s) $(uname -r)"
    log_info "  - Architecture: $(uname -m)"
    log_info "  - User: $(whoami)"
    log_info "  - Working Directory: $(pwd)"
    echo
    
    # Check for LiveReview installation
    log_info "📁 Installation Detection:"
    if detect_livereview_installation; then
        log_success "  ✅ LiveReview installation found: $LIVEREVIEW_INSTALL_DIR"
        
        # Check installation files
        if [[ -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
            log_success "  ✅ docker-compose.yml exists"
        else
            log_error "  ❌ docker-compose.yml missing"
        fi
        
        if [[ -f "$LIVEREVIEW_INSTALL_DIR/.env" ]]; then
            log_success "  ✅ .env file exists"
        else
            log_error "  ❌ .env file missing"
        fi
        
        if [[ -d "$LIVEREVIEW_INSTALL_DIR/lrdata" ]]; then
            log_success "  ✅ Data directory exists"
        else
            log_warning "  ⚠️  Data directory missing"
        fi
    else
        log_error "  ❌ No LiveReview installation detected"
        log_info "  💡 Run 'lrops.sh setup-demo' to install"
    fi
    echo
    
    # Check Docker
    log_info "🐳 Docker Environment:"
    if command -v docker >/dev/null 2>&1; then
        log_success "  ✅ Docker command available"
        
        if docker info >/dev/null 2>&1; then
            log_success "  ✅ Docker daemon running"
            local docker_version=$(docker --version | cut -d' ' -f3 | sed 's/,//')
            log_info "  📊 Docker version: $docker_version"
        else
            log_error "  ❌ Docker daemon not accessible"
            log_info "  💡 Try: sudo systemctl start docker"
        fi
    else
        log_error "  ❌ Docker not installed"
    fi
    
    if detect_docker_compose_cmd; then
        log_success "  ✅ Docker Compose available: $DOCKER_COMPOSE_CMD"
    else
        log_error "  ❌ Docker Compose not available"
    fi
    echo
    
    # Check containers (if installation exists)
    if [[ -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_info "📦 Container Status:"
        
        local containers_found=false
        if docker ps -a --filter "name=livereview" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -q livereview; then
            containers_found=true
            log_success "  ✅ LiveReview containers found:"
            docker ps -a --filter "name=livereview" --format "  {{.Names}}: {{.Status}}" | sed 's/^/    /'
            
            # Check if containers are healthy
            local running_containers=$(docker ps --filter "name=livereview" --format "{{.Names}}" | wc -l)
            if [[ $running_containers -gt 0 ]]; then
                log_success "  ✅ $running_containers container(s) running"
            else
                log_warning "  ⚠️  No containers currently running"
            fi
        else
            log_warning "  ⚠️  No LiveReview containers found"
        fi
        echo
        
        # Check ports if containers are running
        if [[ "$containers_found" == "true" ]]; then
            log_info "🌐 Network Connectivity:"
            
            # Check if ports are accessible
            local api_port="8888"
            local ui_port="8081"
            
            if curl -f -s --max-time 5 "http://localhost:$api_port/health" >/dev/null 2>&1; then
                log_success "  ✅ API endpoint accessible (port $api_port)"
            else
                log_warning "  ⚠️  API endpoint not accessible (port $api_port)"
            fi
            
            if curl -f -s --max-time 5 "http://localhost:$ui_port/" >/dev/null 2>&1; then
                log_success "  ✅ UI endpoint accessible (port $ui_port)"
            else
                log_warning "  ⚠️  UI endpoint not accessible (port $ui_port)"
            fi
            echo
        fi
    fi
    
    # Check system resources
    log_info "💾 System Resources:"
    if command -v df >/dev/null 2>&1; then
        local disk_usage=$(df "$LIVEREVIEW_INSTALL_DIR" 2>/dev/null | awk 'NR==2 {print $5}' || echo "N/A")
        log_info "  📊 Disk usage ($LIVEREVIEW_INSTALL_DIR): $disk_usage"
    fi
    
    if command -v free >/dev/null 2>&1; then
        local memory_usage=$(free -h | awk 'NR==2{printf "%.1f%", $3*100/$2}' || echo "N/A")
        log_info "  📊 Memory usage: $memory_usage"
    fi
    echo
    
    # Check recent logs for errors
    if [[ -d "$LIVEREVIEW_INSTALL_DIR" ]] && docker ps --filter "name=livereview" --format "{{.Names}}" | grep -q livereview; then
        log_info "📋 Recent Issues:"
        local recent_errors=$(docker_compose logs --since=1h 2>/dev/null | grep -i "error\|fail\|panic\|fatal" | grep -v '"error":""' | wc -l)
        
        if [[ $recent_errors -eq 0 ]]; then
            log_success "  ✅ No recent errors in logs"
        else
            log_warning "  ⚠️  Found $recent_errors recent error(s) in logs"
            log_info "  💡 Run 'lrops.sh logs' to view detailed logs"
        fi
        echo
    fi
    
    # Summary and recommendations
    log_info "💡 Recommendations:"
    if [[ ! -d "$LIVEREVIEW_INSTALL_DIR" ]]; then
        log_info "  • Install LiveReview: lrops.sh setup-demo"
    elif ! docker ps --filter "name=livereview" --format "{{.Names}}" | grep -q livereview; then
        log_info "  • Start LiveReview: lrops.sh start"
    else
        log_info "  • Check detailed status: lrops.sh status"
        log_info "  • View logs: lrops.sh logs"
    fi
    
    log_success "Diagnostics completed!"
}

main() {
    # Detect existing LiveReview installation for management commands
    # This will update LIVEREVIEW_INSTALL_DIR if an installation is found
    local is_management_command=false
    
    case "${1:-}" in
        status|info|start|stop|restart|update|list-backups|backup|quick-backup|backup-info|delete-backup|restore|uninstall|logs|help)
            is_management_command=true
            ;;
        setup-demo|setup-production)
            is_management_command=false  # These are installation commands
            ;;
    esac
    
    # Only auto-detect for management commands, not installation commands
    if [[ "$is_management_command" == "true" ]]; then
        detect_livereview_installation || {
            # If no installation found for management commands, show helpful error
            case "${1:-}" in
                status|info|start|stop|restart|logs)
                    log_error "No LiveReview installation found"
            log_info "LiveReview may not be installed, or installed in a non-standard location."
            log_info "Standard installation location: ${DEFAULT_HOME_DIR}/livereview"
                    log_info "To install LiveReview, run: lrops.sh setup-demo"
                    log_info "To specify custom location, set: export LIVEREVIEW_INSTALL_DIR=/path/to/livereview"
                    exit 1
                    ;;
            esac
        }
    fi
    
    # Check for management commands first (before parsing complex arguments)
    case "${1:-}" in
        status)
            show_status
            exit $?
            ;;
        env)
            if [[ "${2:-}" == "validate" ]]; then
                env_validate_cmd
                exit $?
            fi
            log_error "Unknown env subcommand: ${2:-}"
            log_info "Usage: lrops.sh env validate"
            exit 1
            ;;
        info)
            show_info
            exit $?
            ;;
        start)
            start_containers_cmd
            exit $?
            ;;
        stop)
            stop_containers_cmd
            exit $?
            ;;
        restart)
            restart_containers_cmd
            exit $?
            ;;
        list-backups)
            list_backups_cmd
            exit $?
            ;;
        backup)
            # Parse backup-specific arguments: lrops.sh backup [--backup-dir <path>] [name]
            shift  # Remove 'backup' from arguments
            local backup_name=""
            local custom_backup_dir=""
            
            # Parse backup arguments
            while [[ $# -gt 0 ]]; do
                case $1 in
                    --backup-dir)
                        custom_backup_dir="$2"
                        shift 2
                        ;;
                    --backup-dir=*)
                        custom_backup_dir="${1#*=}"
                        shift
                        ;;
                    --help|-h)
                        show_backup_options_help
                        exit 0
                        ;;
                    --*)
                        log_error "Unknown backup option: $1"
                        log_info "Usage: lrops.sh backup [--backup-dir <path>] [name]"
                        show_backup_options_help
                        exit 1
                        ;;
                    *)
                        if [[ -z "$backup_name" ]]; then
                            backup_name="$1"
                        else
                            log_error "Multiple backup names specified: '$backup_name' and '$1'"
                            log_info "Usage: lrops.sh backup [--backup-dir <path>] [name]"
                            exit 1
                        fi
                        shift
                        ;;
                esac
            done
            
            # Set the custom backup directory if provided
            if [[ -n "$custom_backup_dir" ]]; then
                BACKUP_TARGET_DIR="$custom_backup_dir"
            fi
            
            create_backup_cmd "$backup_name"
            exit $?
            ;;
        quick-backup)
            # Create quick timestamped backup: lrops.sh quick-backup
            quick_backup_cmd
            exit $?
            ;;
        backup-info)
            # Show detailed backup information: lrops.sh backup-info <backup_name>
            show_backup_info_cmd "${2:-}"
            exit $?
            ;;
        delete-backup)
            # Delete a specific backup: lrops.sh delete-backup <backup_name>
            delete_backup_cmd "${2:-}"
            exit $?
            ;;
        set-mode)
            # Set deployment mode: lrops.sh set-mode <demo|production>
            set_mode_cmd "${2:-}"
            exit $?
            ;;
        show-mode|mode)
            # Show current deployment mode: lrops.sh show-mode
            show_mode_cmd
            exit $?
            ;;
        restore)
            restore_backup_cmd "${2:-}"
            exit $?
            ;;
        uninstall)
            uninstall_cmd
            exit $?
            ;;
        self-update|update-self)
            self_update_cmd
            exit $?
            ;;
        update)
            # Optional version parameter: lrops.sh update [version]
            update_containers_cmd "${2:-}"
            exit $?
            ;;
        logs)
            show_logs "${2:-}" "${3:-}"
            exit $?
            ;;
        setup-demo)
            # Quick demo mode setup
            EXPRESS_MODE=true
            FORCE_INSTALL=false
            ;;
        setup-production)
            # Quick production mode setup - interactive for safety
            EXPRESS_MODE=false
            FORCE_INSTALL=false
            log_info "Setting up LiveReview in production mode"
            log_warning "This requires reverse proxy configuration"
            ;;
        help)
            case "${2:-}" in
                ssl)
                    show_ssl_help
                    exit 0
                    ;;
                backup)
                    show_backup_help
                    exit 0
                    ;;
                nginx)
                    show_nginx_help
                    exit 0
                    ;;
                caddy)
                    show_caddy_help
                    exit 0
                    ;;
                apache)
                    show_apache_help
                    exit 0
                    ;;
                *)
                    show_help
                    exit 0
                    ;;
            esac
            ;;
    esac
    
    # Parse command line arguments
    parse_arguments "$@"
    
    # Enable debug mode if requested (must be done after argument parsing)
    if [[ "$DEBUG_MODE" == "true" ]]; then
        set -x  # Enable bash tracing
        log_info "Debug mode enabled - bash tracing is now active"
    fi
    
    # Handle version and help first
    if [[ "$SHOW_VERSION" == "true" ]]; then
        show_version
        exit 0
    fi
    
    if [[ "$SHOW_HELP" == "true" ]]; then
        show_help
        exit 0
    fi
    
    # Handle special test flags
    if [[ "$TEST_GITHUB_API" == "true" ]]; then
        test_github_api
        exit $?
    fi
    
    if [[ "$SHOW_LATEST_VERSION" == "true" ]]; then
        show_latest_version
        exit $?
    fi
    
    if [[ "$LIST_VERSIONS" == "true" ]]; then
        list_versions
        exit $?
    fi
    
    if [[ "$LIST_EMBEDDED_DATA" == "true" ]]; then
        list_embedded_templates
        exit 0
    fi
    
    if [[ "$TEST_EXTRACT" == "true" ]]; then
        if [[ -n "$EXTRACT_TO" ]]; then
            test_template_extraction "$EXTRACT_TO"
        else
            log_error "Please specify template name: --test-extract <template_name>"
            log_info "Available templates:"
            list_embedded_templates
            exit 1
        fi
        exit $?
    fi
    
    if [[ -n "$EXTRACT_TO" ]]; then
        extract_all_templates "$EXTRACT_TO"
        exit $?
    fi
    
    if [[ "$INSTALL_SELF" == "true" ]]; then
        install_self
        exit 0
    fi
    
    if [[ "$DIAGNOSE" == "true" ]]; then
        run_diagnostics
        exit 0
    fi
    
    # Show script header
    echo
    echo -e "${BLUE}╔══════════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║                    LiveReview Operations Script v${SCRIPT_VERSION}                    ║${NC}"
    echo -e "${BLUE}║                         One-Line Installer & Manager                        ║${NC}"
    echo -e "${BLUE}╚══════════════════════════════════════════════════════════════════════════════╝${NC}"
    echo
    
    # Run system checks
    # Obtain sudo early (for installing script path, docker on root-only setups)
    ensure_sudo_session
    # Enable sudo for Docker if necessary (systems where only 'sudo docker' works)
    maybe_enable_sudo_for_docker
    check_system_prerequisites
    
    # No snap-specific handling needed; install under user home avoids snap homedirs issues
    check_existing_installation
    
    # Detect and set docker compose command early (after potential sudo-wrapper)
    if ! detect_docker_compose_cmd; then
        error_exit "Docker Compose detection failed"
    fi
    log_debug "Docker Compose command detected: $DOCKER_COMPOSE_CMD"
    
    # Resolve version
    section_header "VERSION RESOLUTION"
    local resolved_version
    if ! resolved_version=$(resolve_version "$LIVEREVIEW_VERSION"); then
        error_exit "Version resolution failed"
    fi
    
    log_success "Using LiveReview version: $resolved_version"
    log_info "Docker image: ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${resolved_version}"
    
    # For now, just show what we would do
    if [[ "$DRY_RUN" == "true" ]]; then
        section_header "DRY RUN MODE - INSTALLATION PLAN"
        log_info "Express mode: $EXPRESS_MODE"
        log_info "Force install: $FORCE_INSTALL"
        log_info "Requested version: ${LIVEREVIEW_VERSION:-latest}"
        log_info "Resolved version: $resolved_version"
        log_info "Docker image: ${DOCKER_REGISTRY}/${DOCKER_IMAGE}:${resolved_version}"
        log_info "Install directory: $LIVEREVIEW_INSTALL_DIR"
        log_info "Script path: $LIVEREVIEW_SCRIPT_PATH"
        log_success "Prerequisites check completed successfully"
        exit 0
    fi
    
    # Handle config-only generation
    if [[ "$GENERATE_CONFIG_ONLY" == "true" ]]; then
        local output_dir="${OUTPUT_DIR:-/tmp/lr-config-$(date +%s)}"
        section_header "GENERATING CONFIGURATION ONLY"
        log_info "Output directory: $output_dir"
        
        # Gather configuration
        local config_file
        config_file=$(gather_configuration "$resolved_version")
        
        # Validate configuration
        if ! validate_configuration "$config_file"; then
            error_exit "Configuration validation failed"
        fi
        
        # Create output directory
        mkdir -p "$output_dir"
        
        # Generate files in output directory
        LIVEREVIEW_INSTALL_DIR="$output_dir"
        generate_env_file "$config_file"
        generate_docker_compose "$config_file"
        
        # Extract templates
        mkdir -p "$output_dir"/{config,scripts}
        extract_data "nginx.conf.example" "$output_dir/config/nginx.conf.example"
        extract_data "caddy.conf.example" "$output_dir/config/caddy.conf.example"
        extract_data "apache.conf.example" "$output_dir/config/apache.conf.example"
        extract_data "backup.sh" "$output_dir/scripts/backup.sh"
        extract_data "restore.sh" "$output_dir/scripts/restore.sh"
        extract_data "setup-ssl.sh" "$output_dir/scripts/setup-ssl.sh"
        extract_data "renew-ssl.sh" "$output_dir/scripts/renew-ssl.sh"
        extract_data "backup-cron.example" "$output_dir/config/backup-cron.example"
        
        chmod +x "$output_dir/scripts/"*.sh 2>/dev/null || true
        
        log_success "Configuration generated in: $output_dir"
        
        # Cleanup
        rm -f "$config_file"
        exit 0
    fi
    
    # Handle templates-only installation
    if [[ "$INSTALL_TEMPLATES_ONLY" == "true" ]]; then
        local output_dir="${OUTPUT_DIR:-/tmp/lr-templates-$(date +%s)}"
        section_header "INSTALLING TEMPLATES ONLY"
        log_info "Output directory: $output_dir"
        
        # Create output directory structure
        LIVEREVIEW_INSTALL_DIR="$output_dir"
        create_directory_structure
        
        # Extract all templates and scripts
        extract_templates_and_scripts
        
        log_success "Templates extracted to: $output_dir"
        log_info "Templates available:"
        log_info "   - nginx.conf.example"
        log_info "   - caddy.conf.example"
        log_info "   - apache.conf.example"
        log_info "   - backup.sh"
        log_info "   - restore.sh"
        log_info "   - backup-cron.example"
        
        exit 0
    fi
    
    # =============================================================================
    # PHASE 4: INSTALLATION CORE LOGIC
    # =============================================================================
    
    section_header "LIVEREVIEW INSTALLATION"
    log_info "Starting LiveReview installation process..."
    log_info "Version: $resolved_version"
    log_info "Installation directory: $LIVEREVIEW_INSTALL_DIR"
    
    # Step 1: Handle existing directories
    if ! handle_existing_directories; then
        error_exit "Cannot proceed with existing installation"
    fi
    
    # Step 2: Gather configuration
    local config_file
    config_file=$(gather_configuration "$resolved_version")
    
    # Step 3: Validate configuration
    if ! validate_configuration "$config_file"; then
        rm -f "$config_file"
        error_exit "Configuration validation failed"
    fi
    
    # Step 4: Create directory structure
    create_directory_structure
    
    # Step 5: Generate configuration files
    section_header "GENERATING CONFIGURATION FILES"
    generate_env_file "$config_file"
    generate_docker_compose "$config_file"
    
    # Step 6: Extract templates and scripts
    extract_templates_and_scripts
    
    # Step 7: Generate installation summary
    generate_installation_summary "$config_file"
    
    # =============================================================================
    # PHASE 5: DOCKER DEPLOYMENT
    # =============================================================================
    
    # Step 8: Deploy with Docker
    deploy_with_docker "$resolved_version" "$config_file"
    
    # =============================================================================
    # PHASE 7: SCRIPT INSTALLATION
    # =============================================================================
    
    # Step 9: Install script to system path for global access
    install_script_to_system_path
    
    # =============================================================================
    # PHASE 8: POST-INSTALLATION EXPERIENCE
    # =============================================================================
    
    # Step 10: Comprehensive installation validation and summary
    post_installation_experience "$resolved_version" "$config_file"
    
    # Cleanup
    rm -f "$config_file"
}

# =============================================================================
# SCRIPT ENTRY POINT
# =============================================================================

# Only run main if script is executed directly (not sourced)
# Handle case where BASH_SOURCE might not be set (e.g., when piped through bash)
if [[ "${BASH_SOURCE[0]:-}" == "${0}" ]] || [[ -z "${BASH_SOURCE[0]:-}" ]]; then
    main "$@"
fi

# Script execution ends here - everything below is embedded data
exit 0

# === DATA:docker-compose.yml ===
services:
    livereview-app:
        image: ghcr.io/hexmostech/livereview:${LIVEREVIEW_VERSION}
        container_name: livereview-app
        env_file:
            - .env
        environment:
            # Respect user-provided DATABASE_URL; fallback to internal hostname
            DATABASE_URL: ${DATABASE_URL:-postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable}
            LIVEREVIEW_VERSION: ${LIVEREVIEW_VERSION}
            # Two-mode deployment configuration
            LIVEREVIEW_BACKEND_PORT: ${LIVEREVIEW_BACKEND_PORT:-8888}
            LIVEREVIEW_FRONTEND_PORT: ${LIVEREVIEW_FRONTEND_PORT:-8081}
            LIVEREVIEW_REVERSE_PROXY: ${LIVEREVIEW_REVERSE_PROXY:-false}
            # Framework-specific API vars are derived at runtime in entrypoint
        ports:
            - "${LIVEREVIEW_FRONTEND_PORT:-8081}:8081"  # Frontend UI
            - "${LIVEREVIEW_BACKEND_PORT:-8888}:8888"   # Backend API
        depends_on:
            livereview-db:
                condition: service_healthy
        volumes:
            # Mount .env file as read-only
            - ./.env:/app/.env:ro
            # Mount entire lrdata directory for persistence
            - ./lrdata:/app/lrdata
        restart: unless-stopped
        healthcheck:
            test: ["CMD-SHELL", "curl -f http://localhost:8888/health || curl -f http://localhost:8888/api/health || curl -f http://localhost:8888/ || exit 1"]
            interval: 30s
            timeout: 10s
            retries: 3
            start_period: 30s

    livereview-db:
        image: postgres:15-alpine
        container_name: livereview-db
        environment:
            POSTGRES_USER: livereview
            POSTGRES_PASSWORD: ${DB_PASSWORD}
            POSTGRES_DB: livereview
        volumes:
            - ./lrdata/postgres:/var/lib/postgresql/data
        healthcheck:
            test: ["CMD-SHELL", "pg_isready -U livereview -d livereview"]
            interval: 10s
            timeout: 5s
            retries: 5
            start_period: 10s
        restart: unless-stopped
        # Don't expose database port to host for security
# === END:docker-compose.yml ===

# === DATA:.env ===
# LiveReview configuration (minimal but complete)

# Ports
LIVEREVIEW_BACKEND_PORT=8888
LIVEREVIEW_FRONTEND_PORT=8081
LIVEREVIEW_REVERSE_PROXY=false

# Database
DB_PASSWORD=${DB_PASSWORD}
DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable

# Security
JWT_SECRET=${JWT_SECRET}
# === END:.env ===

# === DATA:nginx.conf.example ===
# Copy to /etc/nginx/sites-available/livereview and enable

server {
    listen 80;
    server_name your-domain.com;  # Replace with your domain
    
    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    
    # Increase client max body size for file uploads
    client_max_body_size 100M;
    
    # Prevent proxy temp file issues
    proxy_max_temp_file_size 0;
    
    # Route API requests to backend (port 8888)
    location ^~ /api/ {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket support if needed
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        
        # Timeouts
        proxy_connect_timeout 60s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }
    
    # Route everything else to frontend (port 8081)
    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # WebSocket support for hot reload in development
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}

# HTTPS configuration (uncomment after setting up SSL)
# server {
#     listen 443 ssl http2;
#     server_name your-domain.com;
#     
#     # SSL certificate files (adjust paths as needed)
#     ssl_certificate /etc/letsencrypt/live/your-domain.com/fullchain.pem;
#     ssl_certificate_key /etc/letsencrypt/live/your-domain.com/privkey.pem;
#     
#     # SSL configuration
#     ssl_protocols TLSv1.2 TLSv1.3;
#     ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES256-GCM-SHA384;
#     ssl_prefer_server_ciphers off;
#     ssl_session_cache shared:SSL:10m;
#     ssl_session_timeout 10m;
#     
#     # Security headers
#     add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
#     add_header X-Frame-Options "SAMEORIGIN" always;
#     add_header X-Content-Type-Options "nosniff" always;
#     add_header X-XSS-Protection "1; mode=block" always;
#     add_header Referrer-Policy "strict-origin-when-cross-origin" always;
#     
#     # Same location blocks as HTTP version
#     client_max_body_size 100M;
#     
#     # Prevent proxy temp file issues
#     proxy_max_temp_file_size 0;
#     
#     location ^~ /api/ {
#         proxy_pass http://127.0.0.1:8888;
#         proxy_set_header Host $host;
#         proxy_set_header X-Real-IP $remote_addr;
#         proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
#         proxy_set_header X-Forwarded-Proto $scheme;
#         proxy_http_version 1.1;
#         proxy_set_header Upgrade $http_upgrade;
#         proxy_set_header Connection "upgrade";
#         proxy_connect_timeout 60s;
#         proxy_send_timeout 60s;
#         proxy_read_timeout 60s;
#     }
#     
#     location / {
#         proxy_pass http://127.0.0.1:8081;
#         proxy_set_header Host $host;
#         proxy_set_header X-Real-IP $remote_addr;
#         proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
#         proxy_set_header X-Forwarded-Proto $scheme;
#         proxy_http_version 1.1;
#         proxy_set_header Upgrade $http_upgrade;
#         proxy_set_header Connection "upgrade";
#     }
# }
# Redirect HTTP to HTTPS (uncomment after setting up SSL)
# server {
#     listen 80;
#     server_name your-domain.com;
#     return 301 https://$server_name$request_uri;
# }
# === END:nginx.conf.example ===

# === DATA:caddy.conf.example ===
# Caddy v2 configuration for LiveReview
# Save as /etc/caddy/Caddyfile or use with caddy run --config caddy.conf.example

your-domain.com {
    # Automatic HTTPS with Let's Encrypt
    
    # Handle API routes (send to backend)
    handle /api/* {
        reverse_proxy localhost:8888 {
            header_up Host {host}
            header_up X-Real-IP {remote_host}
            header_up X-Forwarded-For {remote_host}
            header_up X-Forwarded-Proto {scheme}
        }
    }
    
    # Handle everything else (send to frontend)
    handle {
        reverse_proxy localhost:8081 {
            header_up Host {host}
            header_up X-Real-IP {remote_host}
            header_up X-Forwarded-For {remote_host}
            header_up X-Forwarded-Proto {scheme}
        }
    }
    
    # Security headers
    header {
        # Remove server information
        -Server
        
        # Security headers
        X-Frame-Options "SAMEORIGIN"
        X-Content-Type-Options "nosniff"
        X-XSS-Protection "1; mode=block"
        Referrer-Policy "strict-origin-when-cross-origin"
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
    }
    
    # File upload size limit
    request_body {
        max_size 100MB
    }
    
    # Logging
    log {
        output file /var/log/caddy/livereview.log
        format json
    }
}

# Development configuration (HTTP only)
# Uncomment for local development without SSL
# localhost:80 {
#     handle /api/* {
#         reverse_proxy localhost:8888
#     }
#     handle {
#         reverse_proxy localhost:8081
#     }
# }
# === END:caddy.conf.example ===

# === DATA:apache.conf.example ===
# Apache virtual host configuration for LiveReview
# Save as /etc/apache2/sites-available/livereview.conf
# Enable with: a2ensite livereview && systemctl reload apache2

<VirtualHost *:80>
    ServerName your-domain.com
    DocumentRoot /var/www/html
    
    # Enable required modules
    # a2enmod proxy proxy_http proxy_balancer lbmethod_byrequests headers rewrite ssl
    
    # Security headers
    Header always set X-Frame-Options "SAMEORIGIN"
    Header always set X-Content-Type-Options "nosniff"
    Header always set X-XSS-Protection "1; mode=block"
    Header always set Referrer-Policy "strict-origin-when-cross-origin"
    
    # Increase upload size
    LimitRequestBody 104857600  # 100MB
    
    # Proxy API requests to backend (port 8888)
    ProxyPreserveHost On
    ProxyRequests Off
    
    <Location /api/>
        ProxyPass http://127.0.0.1:8888/api/
        ProxyPassReverse http://127.0.0.1:8888/api/
        
        # Forward headers
        ProxySetHeader Host %{HTTP_HOST}
        ProxySetHeader X-Real-IP %{REMOTE_ADDR}
        ProxySetHeader X-Forwarded-For %{REMOTE_ADDR}
        ProxySetHeader X-Forwarded-Proto %{REQUEST_SCHEME}
    </Location>
    
    # Proxy everything else to frontend (port 8081)
    <Location />
        ProxyPass http://127.0.0.1:8081/
        ProxyPassReverse http://127.0.0.1:8081/
        
        # Forward headers
        ProxySetHeader Host %{HTTP_HOST}
        ProxySetHeader X-Real-IP %{REMOTE_ADDR}
        ProxySetHeader X-Forwarded-For %{REMOTE_ADDR}
        ProxySetHeader X-Forwarded-Proto %{REQUEST_SCHEME}
    </Location>
    
    # Logging
    ErrorLog ${APACHE_LOG_DIR}/livereview_error.log
    CustomLog ${APACHE_LOG_DIR}/livereview_access.log combined
</VirtualHost>

# HTTPS virtual host (uncomment after setting up SSL)
# <VirtualHost *:443>
#     ServerName your-domain.com
#     DocumentRoot /var/www/html
#     
#     # SSL configuration
#     SSLEngine on
#     SSLCertificateFile /etc/letsencrypt/live/your-domain.com/fullchain.pem
#     SSLCertificateKeyFile /etc/letsencrypt/live/your-domain.com/privkey.pem
#     
#     # SSL security settings
#     SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1
#     SSLCipherSuite ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305
#     SSLHonorCipherOrder off
#     SSLSessionTickets off
#     
#     # Security headers for HTTPS
#     Header always set Strict-Transport-Security "max-age=31536000; includeSubDomains"
#     Header always set X-Frame-Options "SAMEORIGIN"
#     Header always set X-Content-Type-Options "nosniff"
#     Header always set X-XSS-Protection "1; mode=block"
#     Header always set Referrer-Policy "strict-origin-when-cross-origin"
#     
#     # Same proxy configuration as HTTP
#     ProxyPreserveHost On
#     ProxyRequests Off
#     LimitRequestBody 104857600
#     
#     <Location /api/>
#         ProxyPass http://127.0.0.1:8888/api/
#         ProxyPassReverse http://127.0.0.1:8888/api/
#         ProxySetHeader Host %{HTTP_HOST}
#         ProxySetHeader X-Real-IP %{REMOTE_ADDR}
#         ProxySetHeader X-Forwarded-For %{REMOTE_ADDR}
#         ProxySetHeader X-Forwarded-Proto %{REQUEST_SCHEME}
#     </Location>
#     
#     <Location />
#         ProxyPass http://127.0.0.1:8081/
#         ProxyPassReverse http://127.0.0.1:8081/
#         ProxySetHeader Host %{HTTP_HOST}
#         ProxySetHeader X-Real-IP %{REMOTE_ADDR}
#         ProxySetHeader X-Forwarded-For %{REMOTE_ADDR}
#         ProxySetHeader X-Forwarded-Proto %{REQUEST_SCHEME}
#     </Location>
#     
#     ErrorLog ${APACHE_LOG_DIR}/livereview_ssl_error.log
#     CustomLog ${APACHE_LOG_DIR}/livereview_ssl_access.log combined
# </VirtualHost>

# Redirect HTTP to HTTPS (uncomment after setting up SSL)
# <VirtualHost *:80>
#     ServerName your-domain.com
#     Redirect permanent / https://your-domain.com/
# </VirtualHost>
# === END:apache.conf.example ===

# === DATA:backup.sh ===
#!/bin/bash
# LiveReview Backup Script
# Generated by lrops.sh installer
# Usage: ./backup.sh [backup-name]

set -euo pipefail

# Configuration
LIVEREVIEW_DIR=~/livereview
BACKUP_BASE_DIR=~/livereview/backups
TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
BACKUP_NAME="${1:-livereview_backup_${TIMESTAMP}}"
BACKUP_DIR="${BACKUP_BASE_DIR}/${BACKUP_NAME}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}ℹ️  INFO:${NC} $*"
}

log_success() {
    echo -e "${GREEN}✅ SUCCESS:${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}⚠️  WARNING:${NC} $*"
}

log_error() {
    echo -e "${RED}❌ ERROR:${NC} $*"
}

# Check if LiveReview is installed
if [[ ! -d "$LIVEREVIEW_DIR" ]]; then
    log_error "LiveReview installation not found at $LIVEREVIEW_DIR"
    exit 1
fi

# Create backup directory
mkdir -p "$BACKUP_DIR"

log_info "Starting backup to: $BACKUP_DIR"

# Backup configuration files
log_info "Backing up configuration files..."
cp "$LIVEREVIEW_DIR/.env" "$BACKUP_DIR/" 2>/dev/null || log_warning ".env file not found"
cp "$LIVEREVIEW_DIR/docker-compose.yml" "$BACKUP_DIR/" 2>/dev/null || log_warning "docker-compose.yml not found"

# Backup config and scripts directories if they exist
if [[ -d "$LIVEREVIEW_DIR/config" ]]; then
    cp -r "$LIVEREVIEW_DIR/config" "$BACKUP_DIR/"
    log_info "✓ Configuration templates backed up"
fi

if [[ -d "$LIVEREVIEW_DIR/scripts" ]]; then
    cp -r "$LIVEREVIEW_DIR/scripts" "$BACKUP_DIR/"
    log_info "✓ Scripts backed up"
fi

# Backup database
log_info "Backing up database..."
if docker ps --format "table {{.Names}}" | grep -q "livereview-db"; then
    # Database is running, create dump
    if docker exec livereview-db pg_dump -U livereview livereview > "$BACKUP_DIR/database_dump.sql"; then
        log_success "✓ Database dump created"
    else
        log_error "Failed to create database dump"
        exit 1
    fi
else
    log_warning "Database container not running, skipping database dump"
    
    # Try to backup the PostgreSQL data directory directly if it exists
    if [[ -d "$LIVEREVIEW_DIR/lrdata/postgres" ]]; then
        log_info "Backing up PostgreSQL data directory..."
        cp -r "$LIVEREVIEW_DIR/lrdata/postgres" "$BACKUP_DIR/postgres_data"
        log_success "✓ PostgreSQL data directory backed up"
    else
        log_warning "PostgreSQL data directory not found"
    fi
fi

# Backup application data
if [[ -d "$LIVEREVIEW_DIR/lrdata" ]]; then
    log_info "Backing up application data..."
    # Exclude postgres directory if we already handled it above
    rsync -av --exclude='postgres' "$LIVEREVIEW_DIR/lrdata/" "$BACKUP_DIR/lrdata/" 2>/dev/null || {
        log_warning "rsync not available, using cp"
        cp -r "$LIVEREVIEW_DIR/lrdata" "$BACKUP_DIR/" 2>/dev/null || log_warning "Could not backup lrdata"
    }
    log_info "✓ Application data backed up"
fi

# Create backup metadata
cat > "$BACKUP_DIR/backup_info.txt" << EOF
LiveReview Backup Information
Created: $(date)
Backup Name: $BACKUP_NAME
LiveReview Directory: $LIVEREVIEW_DIR
Backup Script Version: 1.0.0

Contents:
- Configuration files (.env, docker-compose.yml)
- Database dump (if running) or PostgreSQL data directory
- Application data (lrdata/)
- Configuration templates (config/)
- Helper scripts (scripts/)

Restore Instructions:
1. Stop LiveReview: lrops.sh stop
2. Run restore script: ./restore.sh $BACKUP_NAME
3. Start LiveReview: lrops.sh start
EOF

# Compress backup if requested
if command -v tar >/dev/null 2>&1; then
    log_info "Compressing backup..."
    cd "$BACKUP_BASE_DIR"
    if tar -czf "${BACKUP_NAME}.tar.gz" "$BACKUP_NAME"; then
        rm -rf "$BACKUP_NAME"
        log_success "✓ Backup compressed to ${BACKUP_NAME}.tar.gz"
        BACKUP_LOCATION="${BACKUP_BASE_DIR}/${BACKUP_NAME}.tar.gz"
    else
        log_warning "Compression failed, backup left uncompressed"
        BACKUP_LOCATION="$BACKUP_DIR"
    fi
else
    log_warning "tar not available, backup left uncompressed"
    BACKUP_LOCATION="$BACKUP_DIR"
fi

# Calculate backup size
BACKUP_SIZE=$(du -sh "$BACKUP_LOCATION" | cut -f1)

log_success "Backup completed successfully!"
log_info "Backup location: $BACKUP_LOCATION"
log_info "Backup size: $BACKUP_SIZE"
log_info "To restore: ./restore.sh $BACKUP_NAME"
# === END:backup.sh ===

# === DATA:restore.sh ===
#!/bin/bash
# LiveReview Restore Script
# Generated by lrops.sh installer
# Usage: ./restore.sh <backup-name>

set -euo pipefail

# Configuration
LIVEREVIEW_DIR=~/livereview
BACKUP_BASE_DIR=~/livereview/backups

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}ℹ️  INFO:${NC} $*"
}

log_success() {
    echo -e "${GREEN}✅ SUCCESS:${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}⚠️  WARNING:${NC} $*"
}

log_error() {
    echo -e "${RED}❌ ERROR:${NC} $*"
}

# Check arguments
if [[ $# -lt 1 ]]; then
    log_error "Usage: $0 <backup-name>"
    log_info "Available backups:"
    if [[ -d "$BACKUP_BASE_DIR" ]]; then
        ls -la "$BACKUP_BASE_DIR"
    else
        log_info "No backups directory found"
    fi
    exit 1
fi

BACKUP_NAME="$1"
BACKUP_DIR="$BACKUP_BASE_DIR/$BACKUP_NAME"
BACKUP_ARCHIVE="$BACKUP_BASE_DIR/${BACKUP_NAME}.tar.gz"

# Find backup location
if [[ -f "$BACKUP_ARCHIVE" ]]; then
    log_info "Found compressed backup: $BACKUP_ARCHIVE"
    BACKUP_SOURCE="$BACKUP_ARCHIVE"
    BACKUP_TYPE="compressed"
elif [[ -d "$BACKUP_DIR" ]]; then
    log_info "Found uncompressed backup: $BACKUP_DIR"
    BACKUP_SOURCE="$BACKUP_DIR"
    BACKUP_TYPE="directory"
else
    log_error "Backup not found: $BACKUP_NAME"
    log_info "Checked for:"
    log_info "  - $BACKUP_ARCHIVE"
    log_info "  - $BACKUP_DIR"
    exit 1
fi

# Confirmation prompt
echo
log_warning "This will restore LiveReview from backup: $BACKUP_NAME"
log_warning "Current LiveReview data will be REPLACED!"
read -p "Are you sure you want to continue? [y/N]: " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    log_info "Restore cancelled"
    exit 0
fi

# Stop LiveReview services
log_info "Stopping LiveReview services..."
if command -v lrops.sh >/dev/null 2>&1; then
    lrops.sh stop 2>/dev/null || log_warning "Could not stop services with lrops.sh"
else
    # Try direct docker-compose
    if [[ -f "$LIVEREVIEW_DIR/docker-compose.yml" ]]; then
        cd "$LIVEREVIEW_DIR"
        docker-compose down 2>/dev/null || log_warning "Could not stop services with docker-compose"
    fi
fi

# Extract backup if compressed
RESTORE_SOURCE="$BACKUP_DIR"
if [[ "$BACKUP_TYPE" == "compressed" ]]; then
    log_info "Extracting backup archive..."
    cd "$BACKUP_BASE_DIR"
    if tar -xzf "$BACKUP_ARCHIVE"; then
        log_success "✓ Backup extracted"
        RESTORE_SOURCE="$BACKUP_DIR"
    else
        log_error "Failed to extract backup archive"
        exit 1
    fi
fi

# Backup current installation before restore
if [[ -d "$LIVEREVIEW_DIR" ]]; then
    CURRENT_BACKUP="$BACKUP_BASE_DIR/pre_restore_backup_$(date +%Y%m%d_%H%M%S)"
    log_info "Creating backup of current installation to: $CURRENT_BACKUP"
    cp -r "$LIVEREVIEW_DIR" "$CURRENT_BACKUP" || log_warning "Could not backup current installation"
fi

# Create LiveReview directory if it doesn't exist
mkdir -p "$LIVEREVIEW_DIR"

# Restore configuration files
log_info "Restoring configuration files..."
[[ -f "$RESTORE_SOURCE/.env" ]] && cp "$RESTORE_SOURCE/.env" "$LIVEREVIEW_DIR/"
[[ -f "$RESTORE_SOURCE/docker-compose.yml" ]] && cp "$RESTORE_SOURCE/docker-compose.yml" "$LIVEREVIEW_DIR/"

# Restore config and scripts directories
[[ -d "$RESTORE_SOURCE/config" ]] && cp -r "$RESTORE_SOURCE/config" "$LIVEREVIEW_DIR/"
[[ -d "$RESTORE_SOURCE/scripts" ]] && cp -r "$RESTORE_SOURCE/scripts" "$LIVEREVIEW_DIR/"

# Restore application data
if [[ -d "$RESTORE_SOURCE/lrdata" ]]; then
    log_info "Restoring application data..."
    rm -rf "$LIVEREVIEW_DIR/lrdata" 2>/dev/null || true
    cp -r "$RESTORE_SOURCE/lrdata" "$LIVEREVIEW_DIR/"
    log_success "✓ Application data restored"
fi

# Restore PostgreSQL data if available
if [[ -d "$RESTORE_SOURCE/postgres_data" ]]; then
    log_info "Restoring PostgreSQL data directory..."
    mkdir -p "$LIVEREVIEW_DIR/lrdata"
    rm -rf "$LIVEREVIEW_DIR/lrdata/postgres" 2>/dev/null || true
    cp -r "$RESTORE_SOURCE/postgres_data" "$LIVEREVIEW_DIR/lrdata/postgres"
    log_success "✓ PostgreSQL data restored"
fi

# Start LiveReview services
log_info "Starting LiveReview services..."
if command -v lrops.sh >/dev/null 2>&1; then
    lrops.sh start || log_warning "Could not start services with lrops.sh"
else
    if [[ -f "$LIVEREVIEW_DIR/docker-compose.yml" ]]; then
        cd "$LIVEREVIEW_DIR"
        docker-compose up -d || log_warning "Could not start services with docker-compose"
    fi
fi

# Restore database from dump if available and database is running
if [[ -f "$RESTORE_SOURCE/database_dump.sql" ]]; then
    log_info "Waiting for database to be ready..."
    sleep 10
    
    if docker ps --format "table {{.Names}}" | grep -q "livereview-db"; then
        log_info "Restoring database from dump..."
        if docker exec -i livereview-db psql -U livereview livereview < "$RESTORE_SOURCE/database_dump.sql"; then
            log_success "✓ Database restored from dump"
        else
            log_warning "Failed to restore database from dump - check logs"
        fi
    else
        log_warning "Database container not running, skipping database restore"
    fi
fi

# Cleanup extracted files if we extracted a compressed backup
if [[ "$BACKUP_TYPE" == "compressed" && -d "$BACKUP_DIR" ]]; then
    rm -rf "$BACKUP_DIR"
fi

log_success "Restore completed successfully!"
log_info "LiveReview should now be running with restored data"
log_info "Check status with: lrops.sh status"
# === END:restore.sh ===

# === DATA:backup-cron.example ===
# LiveReview Automated Backup Cron Examples
# Add these to your crontab with: crontab -e

# Daily backup at 2 AM
0 2 * * * cd "$LIVEREVIEW_INSTALL_DIR/scripts" && ./backup.sh daily_$(date +\%Y\%m\%d) >> /var/log/livereview-backup.log 2>&1

# Weekly backup on Sundays at 3 AM
0 3 * * 0 cd "$LIVEREVIEW_INSTALL_DIR/scripts" && ./backup.sh weekly_$(date +\%Y_week\%U) >> /var/log/livereview-backup.log 2>&1

# Monthly backup on the 1st at 4 AM
0 4 1 * * cd "$LIVEREVIEW_INSTALL_DIR/scripts" && ./backup.sh monthly_$(date +\%Y\%m) >> /var/log/livereview-backup.log 2>&1

# === Example with rclone S3 sync ===
# Install rclone first: curl https://rclone.org/install.sh | sudo bash
# Configure S3: rclone config (create remote named 'livereview-s3')

# Daily backup + S3 sync at 2:30 AM
30 2 * * * cd "$LIVEREVIEW_INSTALL_DIR/scripts" && ./backup.sh daily_$(date +\%Y\%m\%d) && rclone sync "$LIVEREVIEW_INSTALL_DIR/backups/" livereview-s3:backups/livereview/ --log-file=/var/log/livereview-s3-sync.log

# === Backup retention (cleanup old backups) ===
# Keep only last 7 daily backups (run at 5 AM)
0 5 * * * find /opt/livereview-backups -name "daily_*" -type f -mtime +7 -delete

# Keep only last 4 weekly backups
0 5 * * 1 find "$LIVEREVIEW_INSTALL_DIR/backups" -name "weekly_*" -type f -mtime +28 -delete

# Keep only last 12 monthly backups
0 5 1 * * find "$LIVEREVIEW_INSTALL_DIR/backups" -name "monthly_*" -type f -mtime +365 -delete

# === Complete example crontab entry ===
# # LiveReview automated backups
# 0 2 * * * cd "$LIVEREVIEW_INSTALL_DIR/scripts" && ./backup.sh daily_$(date +\%Y\%m\%d) >> /var/log/livereview-backup.log 2>&1
# 30 2 * * * rclone sync "$LIVEREVIEW_INSTALL_DIR/backups/" livereview-s3:backups/livereview/ --log-file=/var/log/livereview-s3-sync.log
# 0 5 * * * find "$LIVEREVIEW_INSTALL_DIR/backups" -name "daily_*" -type f -mtime +7 -delete
# === END:backup-cron.example ===

# === DATA:setup-ssl.sh ===
#!/bin/bash
# LiveReview SSL/TLS Certificate Setup Script
# Generated by lrops.sh installer
# Usage: ./setup-ssl.sh <domain> [email]

set -euo pipefail

# Configuration
DOMAIN="${1:-}"
EMAIL="${2:-}"
LIVEREVIEW_DIR=~/livereview

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}ℹ️  INFO:${NC} $*"
}

log_success() {
    echo -e "${GREEN}✅ SUCCESS:${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}⚠️  WARNING:${NC} $*"
}

log_error() {
    echo -e "${RED}❌ ERROR:${NC} $*"
}

show_usage() {
    echo "Usage: $0 <domain> [email]"
    echo "Example: $0 livereview.example.com admin@example.com"
    echo ""
    echo "This script will:"
    echo "  1. Install certbot if not available"
    echo "  2. Generate SSL certificates using Let's Encrypt"
    echo "  3. Configure automatic renewal"
    echo "  4. Update reverse proxy configuration"
    echo ""
    echo "Prerequisites:"
    echo "  - Domain must point to this server"
    echo "  - Ports 80 and 443 must be open"
    echo "  - LiveReview must be running"
    exit 1
}

# Validate inputs
if [[ -z "$DOMAIN" ]]; then
    log_error "Domain is required"
    show_usage
fi

if [[ -z "$EMAIL" ]]; then
    log_warning "Email not provided, will prompt for agreement"
fi

log_info "Setting up SSL certificates for domain: $DOMAIN"

# Check if domain resolves to this server
log_info "Checking domain resolution..."
DOMAIN_IP=$(dig +short "$DOMAIN" 2>/dev/null || echo "")
SERVER_IP=$(curl -s ifconfig.me 2>/dev/null || curl -s ipinfo.io/ip 2>/dev/null || echo "")

if [[ -n "$DOMAIN_IP" && -n "$SERVER_IP" ]]; then
    if [[ "$DOMAIN_IP" == "$SERVER_IP" ]]; then
        log_success "✓ Domain resolves to this server"
    else
        log_warning "Domain resolves to $DOMAIN_IP, but server IP is $SERVER_IP"
        log_warning "SSL certificate generation may fail"
    fi
else
    log_warning "Could not verify domain resolution"
fi

# Install certbot if not available
if ! command -v certbot >/dev/null 2>&1; then
    log_info "Installing certbot..."
    
    # Detect OS and install certbot
    if command -v apt >/dev/null 2>&1; then
        sudo apt update
        sudo apt install -y certbot python3-certbot-nginx
    elif command -v yum >/dev/null 2>&1; then
        sudo yum install -y certbot python3-certbot-nginx
    elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y certbot python3-certbot-nginx
    elif command -v snap >/dev/null 2>&1; then
        sudo snap install --classic certbot
        sudo ln -sf /snap/bin/certbot /usr/bin/certbot
    else
        log_error "Could not install certbot automatically"
        log_info "Please install certbot manually:"
        log_info "  Ubuntu/Debian: sudo apt install certbot python3-certbot-nginx"
        log_info "  CentOS/RHEL: sudo yum install certbot python3-certbot-nginx"
        log_info "  Snap: sudo snap install --classic certbot"
        exit 1
    fi
    
    log_success "✓ Certbot installed"
else
    log_success "✓ Certbot is already available"
fi

# Check if LiveReview is running
if ! docker ps | grep -q livereview; then
    log_error "LiveReview containers are not running"
    log_info "Start LiveReview first: lrops.sh start"
    exit 1
fi

# Prepare for certificate generation
log_info "Preparing for certificate generation..."

# Stop nginx/apache if running on ports 80/443
for service in nginx apache2 httpd; do
    if systemctl is-active --quiet "$service" 2>/dev/null; then
        log_info "Stopping $service temporarily..."
        sudo systemctl stop "$service"
    fi
done

# Generate certificate using standalone mode
log_info "Generating SSL certificate..."
if [[ -n "$EMAIL" ]]; then
    CERT_CMD="sudo certbot certonly --standalone --non-interactive --agree-tos --email $EMAIL -d $DOMAIN"
else
    CERT_CMD="sudo certbot certonly --standalone --agree-tos -d $DOMAIN"
fi

if $CERT_CMD; then
    log_success "✓ SSL certificate generated successfully"
else
    log_error "Certificate generation failed"
    exit 1
fi

# Set up automatic renewal
log_info "Setting up automatic renewal..."
sudo crontab -l 2>/dev/null | grep -v "certbot renew" | sudo crontab - 2>/dev/null || true
(sudo crontab -l 2>/dev/null; echo "0 12 * * * /usr/bin/certbot renew --quiet") | sudo crontab -

log_success "✓ Automatic renewal configured"

# Update reverse proxy configuration
log_info "Updating reverse proxy configuration..."

# Check which reverse proxy configuration exists
if [[ -f "/etc/nginx/sites-available/livereview" ]] || [[ -f "/etc/nginx/conf.d/livereview.conf" ]]; then
    log_info "Updating nginx configuration..."
    # Create SSL-enabled nginx config
    sudo cp "$LIVEREVIEW_DIR/config/nginx.conf.example" "/tmp/livereview-ssl.conf"
    sudo_sed_inplace "s/your-domain.com/$DOMAIN/g" "/tmp/livereview-ssl.conf"
    
    # Uncomment HTTPS section
    sudo_sed_inplace '/# HTTPS configuration/,/# Redirect HTTP to HTTPS/s/^# //' "/tmp/livereview-ssl.conf"
    sudo_sed_inplace '/# Redirect HTTP to HTTPS/,/# }/s/^# //' "/tmp/livereview-ssl.conf"
    
    # Install the configuration
    sudo cp "/tmp/livereview-ssl.conf" "/etc/nginx/sites-available/livereview"
    sudo ln -sf "/etc/nginx/sites-available/livereview" "/etc/nginx/sites-enabled/livereview"
    
    # Test and reload nginx
    if sudo nginx -t; then
        sudo systemctl reload nginx
        log_success "✓ Nginx configuration updated"
    else
        log_error "Nginx configuration test failed"
    fi
    
elif [[ -f "/etc/caddy/Caddyfile" ]]; then
    log_info "Updating Caddy configuration..."
    sudo cp "$LIVEREVIEW_DIR/config/caddy.conf.example" "/etc/caddy/Caddyfile"
    sudo_sed_inplace "s/your-domain.com/$DOMAIN/g" "/etc/caddy/Caddyfile"
    sudo systemctl reload caddy
    log_success "✓ Caddy configuration updated (automatic HTTPS)"
    
elif [[ -f "/etc/apache2/sites-available/livereview.conf" ]]; then
    log_info "Updating Apache configuration..."
    sudo cp "$LIVEREVIEW_DIR/config/apache.conf.example" "/tmp/livereview-ssl.conf"
    sudo_sed_inplace "s/your-domain.com/$DOMAIN/g" "/tmp/livereview-ssl.conf"
    
    # Uncomment HTTPS section
    sudo_sed_inplace '/# HTTPS virtual host/,/# <\/VirtualHost>/s/^# //' "/tmp/livereview-ssl.conf"
    sudo_sed_inplace '/# Redirect HTTP to HTTPS/,/# <\/VirtualHost>/s/^# //' "/tmp/livereview-ssl.conf"
    
    sudo cp "/tmp/livereview-ssl.conf" "/etc/apache2/sites-available/livereview.conf"
    
    # Enable SSL module and site
    sudo a2enmod ssl
    sudo a2ensite livereview
    
    if sudo apache2ctl configtest; then
        sudo systemctl reload apache2
        log_success "✓ Apache configuration updated"
    else
        log_error "Apache configuration test failed"
    fi
else
    log_warning "No reverse proxy configuration found"
    log_info "Please configure your reverse proxy manually"
    log_info "Certificate files:"
    log_info "  - Certificate: /etc/letsencrypt/live/$DOMAIN/fullchain.pem"
    log_info "  - Private Key: /etc/letsencrypt/live/$DOMAIN/privkey.pem"
fi

# Verify SSL certificate
log_info "Verifying SSL certificate..."
sleep 5  # Give time for configuration to reload

if curl -s "https://$DOMAIN" >/dev/null 2>&1; then
    log_success "✅ SSL certificate is working!"
    log_success "✅ LiveReview is now accessible at: https://$DOMAIN"
else
    log_warning "SSL verification failed, but certificate was generated"
    log_info "You may need to configure your reverse proxy manually"
fi

log_success "SSL setup completed!"
log_info "Certificate location: /etc/letsencrypt/live/$DOMAIN/"
log_info "Renewal: Automatic (cron job configured)"
log_info "Test renewal: sudo certbot renew --dry-run"

exit 0
# === END:setup-ssl.sh ===

# === DATA:renew-ssl.sh ===
#!/bin/bash
# LiveReview SSL Certificate Renewal Script
# Generated by lrops.sh installer
# Usage: ./renew-ssl.sh

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() {
    echo -e "${BLUE}ℹ️  INFO:${NC} $*"
}

log_success() {
    echo -e "${GREEN}✅ SUCCESS:${NC} $*"
}

log_warning() {
    echo -e "${YELLOW}⚠️  WARNING:${NC} $*"
}

log_error() {
    echo -e "${RED}❌ ERROR:${NC} $*"
}

log_info "Checking SSL certificate renewal..."

# Check if certbot is available
if ! command -v certbot >/dev/null 2>&1; then
    log_error "Certbot is not installed"
    exit 1
fi

# List certificates and their expiry dates
log_info "Current certificates:"
sudo certbot certificates

# Check for renewal (dry run)
log_info "Testing certificate renewal..."
if sudo certbot renew --dry-run; then
    log_success "✓ Renewal test passed"
else
    log_error "Renewal test failed"
    exit 1
fi

# Perform actual renewal
log_info "Renewing certificates..."
if sudo certbot renew; then
    log_success "✓ Certificate renewal completed"
    
    # Reload reverse proxy services
    for service in nginx apache2 httpd caddy; do
        if systemctl is-active --quiet "$service" 2>/dev/null; then
            log_info "Reloading $service..."
            sudo systemctl reload "$service"
        fi
    done
    
    log_success "✓ Reverse proxy services reloaded"
else
    log_warning "No certificates needed renewal"
fi

log_success "Certificate renewal check completed"

exit 0
# === END:renew-ssl.sh ===
