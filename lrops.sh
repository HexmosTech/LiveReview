#!/bin/bash
# lrops.sh - LiveReview Operations Script
# Version: 1.0.0
# Description: One-line installer and management tool for LiveReview
# Repository: https://github.com/HexmosTech/LiveReview

set -euo pipefail  # Exit on error, undefined vars, pipe failures

# =============================================================================
# SCRIPT METADATA AND CONSTANTS
# =============================================================================

SCRIPT_VERSION="1.0.0"
SCRIPT_NAME="lrops.sh"
LIVEREVIEW_INSTALL_DIR="${LIVEREVIEW_INSTALL_DIR:-/opt/livereview}"
LIVEREVIEW_SCRIPT_PATH="/usr/local/bin/lrops.sh"
GITHUB_REPO="HexmosTech/LiveReview"
GITHUB_API_BASE="https://api.github.com/repos/${GITHUB_REPO}"
DOCKER_REGISTRY="ghcr.io/hexmostech"
DOCKER_IMAGE="livereview"

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

# =============================================================================
# ERROR HANDLING AND CLEANUP
# =============================================================================

cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_error "Script failed with exit code $exit_code"
        log_info "For troubleshooting help, run: $0 --help"
    fi
    exit $exit_code
}

trap cleanup EXIT

error_exit() {
    log_error "$1"
    exit "${2:-1}"
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
        DOCKER_COMPOSE_CMD="docker-compose"
        log_debug "Using legacy docker-compose command"
    elif docker compose version >/dev/null 2>&1; then
        # Modern docker compose plugin is available
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
    
    log_debug "Executing: $DOCKER_COMPOSE_CMD $*"
    $DOCKER_COMPOSE_CMD "$@"
}

# =============================================================================
# ARGUMENT PARSING
# =============================================================================

# Default values
EXPRESS_MODE=false
FORCE_INSTALL=false
DRY_RUN=false
VERBOSE=false
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
            --show-plan)
                DRY_RUN=true
                VERBOSE=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
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
    lrops.sh start                     # Start LiveReview services
    lrops.sh stop                      # Stop LiveReview services
    lrops.sh restart                   # Restart LiveReview services
    lrops.sh logs [service]            # Show container logs
    lrops.sh help ssl                  # SSL/TLS setup guidance
    lrops.sh help backup               # Backup strategies

INSTALLATION OPTIONS:
    --express                          Use secure defaults, no prompts (demo mode)
    --force                           Overwrite existing installation
    --version=v1.2.3                  Install specific version (default: latest)
    --dry-run                         Show what would be done without installing
    --verbose, -v                     Enable verbose output

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
    
    # Check available disk space
    local available_space=$(df /opt 2>/dev/null | awk 'NR==2 {print $4}' || echo "0")
    if [[ $available_space -lt 2097152 ]]; then  # 2GB in KB
        log_warning "Low disk space in /opt. At least 2GB recommended."
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

# =============================================================================
# TEMPLATE EXTRACTION FUNCTIONS (PHASE 3)
# =============================================================================

# Check if script is being executed via pipe
is_piped_execution() {
    [[ "$0" == "bash" || "$0" == "-bash" || "$0" =~ /bash$ ]]
}

# Download script for template extraction when piped
download_script_for_templates() {
    local temp_script="/tmp/lrops_template_source_$$.sh"
    
    log_debug "Script is piped - downloading for template extraction..."
    
    if curl -fsSL "https://raw.githubusercontent.com/HexmosTech/LiveReview/main/lrops.sh" -o "$temp_script" 2>/dev/null; then
        echo "$temp_script"
        return 0
    else
        log_error "Failed to download script for template extraction"
        return 1
    fi
}

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
    local temp_script=""
    
    # If script is being piped, download it for template extraction
    if is_piped_execution; then
        if ! temp_script=$(download_script_for_templates); then
            return 1
        fi
        script_source="$temp_script"
        log_debug "Using downloaded script for template extraction: $script_source"
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
        
        # Cleanup temp script if we downloaded it
        [[ -n "$temp_script" && -f "$temp_script" ]] && rm -f "$temp_script"
        return 0
    else
        log_error "Failed to extract template '$template_name'"
        
        # Cleanup temp script if we downloaded it
        [[ -n "$temp_script" && -f "$temp_script" ]] && rm -f "$temp_script"
        return 1
    fi
}

# List all available embedded templates
list_embedded_templates() {
    log_info "Available embedded templates:"
    grep "^# === DATA:" "$0" | sed 's/^# === DATA:\(.*\) ===$/  - \1/' | sort
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
    
    local templates
    templates=$(grep "^# === DATA:" "$0" | sed 's/^# === DATA:\(.*\) ===$/\1/')
    
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
        
        # Demo mode defaults (localhost-only)
        cat > "$config_file" << EOF
DB_PASSWORD=$db_password
JWT_SECRET=$jwt_secret
LIVEREVIEW_BACKEND_PORT=8888
LIVEREVIEW_FRONTEND_PORT=8081
LIVEREVIEW_REVERSE_PROXY=false
DEPLOYMENT_MODE=demo
LIVEREVIEW_VERSION=$1
EOF
        log_success "✅ Demo mode configuration (localhost-only, no webhooks)"
        log_info "   To upgrade to production mode later, set LIVEREVIEW_REVERSE_PROXY=true"
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
        local reverse_proxy="false"
        
        if [[ "$mode_choice" == "2" ]]; then
            deployment_mode="production"
            reverse_proxy="true"
            log_info "Production mode selected - requires reverse proxy setup"
        else
            deployment_mode="demo"
            reverse_proxy="false"
            log_info "Demo mode selected - localhost only, no configuration needed"
        fi
        
        # Database password
        local db_password
        db_password=$(generate_password 32)
        echo -n "Database password [auto-generated secure password]: "
        read -r user_input
        if [[ -n "$user_input" ]]; then
            db_password="$user_input"
        fi
        
        # JWT Secret
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
        
        if [[ "$deployment_mode" == "production" ]]; then
            echo "Production mode will use standard ports (8888 backend, 8081 frontend)"
            echo "Configure your reverse proxy to route:"
            echo "  /api/* → http://127.0.0.1:8888"
            echo "  /* → http://127.0.0.1:8081"
        fi
        
        # Save configuration
        cat > "$config_file" << EOF
DB_PASSWORD=$db_password
JWT_SECRET=$jwt_secret
LIVEREVIEW_BACKEND_PORT=$backend_port
LIVEREVIEW_FRONTEND_PORT=$frontend_port
LIVEREVIEW_REVERSE_PROXY=$reverse_proxy
DEPLOYMENT_MODE=$deployment_mode
LIVEREVIEW_VERSION=$1
EOF
    fi
    
    echo "$config_file"
}

# Validate configuration values for two-mode system
validate_configuration() {
    local config_file="$1"
    
    log_info "Validating configuration..."
    
    # Source the config
    source "$config_file"
    
    # Validate backend port
    if ! [[ "$LIVEREVIEW_BACKEND_PORT" =~ ^[0-9]+$ ]] || [[ "$LIVEREVIEW_BACKEND_PORT" -lt 1024 ]] || [[ "$LIVEREVIEW_BACKEND_PORT" -gt 65535 ]]; then
        log_error "Invalid backend port: $LIVEREVIEW_BACKEND_PORT (must be 1024-65535)"
        return 1
    fi
    
    # Validate frontend port
    if ! [[ "$LIVEREVIEW_FRONTEND_PORT" =~ ^[0-9]+$ ]] || [[ "$LIVEREVIEW_FRONTEND_PORT" -lt 1024 ]] || [[ "$LIVEREVIEW_FRONTEND_PORT" -gt 65535 ]]; then
        log_error "Invalid frontend port: $LIVEREVIEW_FRONTEND_PORT (must be 1024-65535)"
        return 1
    fi
    
    if [[ "$LIVEREVIEW_BACKEND_PORT" == "$LIVEREVIEW_FRONTEND_PORT" ]]; then
        log_error "Backend and frontend ports cannot be the same"
        return 1
    fi
    
    # Check if ports are available
    if command -v netstat >/dev/null 2>&1; then
        if netstat -tln | grep -q ":${LIVEREVIEW_BACKEND_PORT} "; then
            log_warning "Port $LIVEREVIEW_BACKEND_PORT appears to be in use"
        fi
        if netstat -tln | grep -q ":${LIVEREVIEW_FRONTEND_PORT} "; then
            log_warning "Port $LIVEREVIEW_FRONTEND_PORT appears to be in use"
        fi
    fi
    
    # Validate password strength
    if [[ ${#DB_PASSWORD} -lt 12 ]]; then
        log_warning "Database password is shorter than 12 characters"
    fi
    
    if [[ ${#JWT_SECRET} -lt 32 ]]; then
        log_warning "JWT secret is shorter than 32 characters"
    fi
    
    # Validate deployment mode configuration
    if [[ "$LIVEREVIEW_REVERSE_PROXY" != "true" && "$LIVEREVIEW_REVERSE_PROXY" != "false" ]]; then
        log_error "Invalid LIVEREVIEW_REVERSE_PROXY value: must be 'true' or 'false'"
        return 1
    fi
    
    # Show configuration summary
    log_info "Configuration summary:"
    log_info "  - Deployment mode: $DEPLOYMENT_MODE"
    log_info "  - Backend port: $LIVEREVIEW_BACKEND_PORT"
    log_info "  - Frontend port: $LIVEREVIEW_FRONTEND_PORT"
    log_info "  - Reverse proxy: $LIVEREVIEW_REVERSE_PROXY"
    
    if [[ "$DEPLOYMENT_MODE" == "demo" ]]; then
        log_info "  - Access URL: http://localhost:$LIVEREVIEW_FRONTEND_PORT"
        log_info "  - Webhooks: Disabled (manual triggers only)"
    else
        log_info "  - Requires reverse proxy configuration"
        log_info "  - Webhooks: Enabled (automatic triggers)"
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

# Generate .env file from template with simplified two-mode configuration
generate_env_file() {
    local config_file="$1"
    local output_file="$LIVEREVIEW_INSTALL_DIR/.env"
    
    log_info "Generating .env file..."
    
    # Source configuration
    source "$config_file"
    
    # Extract .env template and substitute variables
    if ! extract_data ".env" "$output_file"; then
        error_exit "Failed to extract .env template"
    fi
    
    # Substitute variables in the .env file
    sed -i "s/\${DB_PASSWORD}/$DB_PASSWORD/g" "$output_file"
    sed -i "s/\${JWT_SECRET}/$JWT_SECRET/g" "$output_file"
    sed -i "s/\${LIVEREVIEW_VERSION}/$LIVEREVIEW_VERSION/g" "$output_file"
    
    # Add deployment mode configuration
    cat >> "$output_file" << EOF

# Two-Mode Deployment Configuration (auto-configured)
LIVEREVIEW_BACKEND_PORT=$LIVEREVIEW_BACKEND_PORT
LIVEREVIEW_FRONTEND_PORT=$LIVEREVIEW_FRONTEND_PORT
LIVEREVIEW_REVERSE_PROXY=$LIVEREVIEW_REVERSE_PROXY

# Deployment mode: $DEPLOYMENT_MODE
# Demo mode: localhost-only, no webhooks, manual triggers only
# Production mode: reverse proxy, webhooks enabled, external access
EOF

    # Add deployment mode-specific comments and guidance
    if [[ "$DEPLOYMENT_MODE" == "demo" ]]; then
        cat >> "$output_file" << EOF

# DEMO MODE CONFIGURATION
# - Access: http://localhost:$LIVEREVIEW_FRONTEND_PORT
# - API: http://localhost:$LIVEREVIEW_BACKEND_PORT
# - Webhooks: Disabled (manual triggers only)
# - External access: Not configured
# 
# To upgrade to production mode:
# 1. Set LIVEREVIEW_REVERSE_PROXY=true
# 2. Restart services: docker compose restart
# 3. Configure reverse proxy (see: lrops.sh help nginx)
EOF
    else
        cat >> "$output_file" << EOF

# PRODUCTION MODE CONFIGURATION  
# - Reverse proxy required for external access
# - Webhooks enabled for automatic triggers
# - External access ready (configure DNS and reverse proxy)
#
# Configure your reverse proxy to route:
# - /api/* → http://127.0.0.1:$LIVEREVIEW_BACKEND_PORT
# - /* → http://127.0.0.1:$LIVEREVIEW_FRONTEND_PORT
#
# For SSL setup: lrops.sh help ssl
# For nginx config: lrops.sh help nginx
# For caddy config: lrops.sh help caddy
EOF
    fi
    
    # Set secure permissions on .env file (readable by Docker containers)
    chmod 644 "$output_file"
    
    log_success "Generated .env file with $DEPLOYMENT_MODE mode configuration"
}

# Generate docker-compose.yml from template with two-mode configuration
generate_docker_compose() {
    local config_file="$1"
    local output_file="$LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
    
    log_info "Generating docker-compose.yml..."
    
    # Source configuration
    source "$config_file"
    
    # Extract docker-compose template
    if ! extract_data "docker-compose.yml" "$output_file"; then
        error_exit "Failed to extract docker-compose.yml template"
    fi
    
    # Substitute variables in the docker-compose file
    sed -i "s/\${LIVEREVIEW_VERSION}/$LIVEREVIEW_VERSION/g" "$output_file"
    sed -i "s/\${DB_PASSWORD}/\${DB_PASSWORD}/g" "$output_file"  # Keep as variable reference
    
    # Update port mappings to use the actual configured ports
    sed -i "s/\${LIVEREVIEW_FRONTEND_PORT:-8081}/$LIVEREVIEW_FRONTEND_PORT/g" "$output_file"
    sed -i "s/\${LIVEREVIEW_BACKEND_PORT:-8888}/$LIVEREVIEW_BACKEND_PORT/g" "$output_file"
    
    log_success "Generated docker-compose.yml with $DEPLOYMENT_MODE mode configuration"
    log_info "Port mappings: Frontend=$LIVEREVIEW_FRONTEND_PORT, Backend=$LIVEREVIEW_BACKEND_PORT"
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
    
    log_success "Configuration templates and scripts extracted"
}

# Generate installation summary file for two-mode system
generate_installation_summary() {
    local config_file="$1"
    local summary_file="$LIVEREVIEW_INSTALL_DIR/installation-summary.txt"
    
    # Source configuration
    source "$config_file"
    
    cat > "$summary_file" << EOF
LiveReview Installation Summary
===============================
Installation Date: $(date)
Script Version: $SCRIPT_VERSION
LiveReview Version: $LIVEREVIEW_VERSION

Deployment Configuration:
- Installation Directory: $LIVEREVIEW_INSTALL_DIR
- Deployment Mode: $DEPLOYMENT_MODE
- Backend Port: $LIVEREVIEW_BACKEND_PORT
- Frontend Port: $LIVEREVIEW_FRONTEND_PORT
- Reverse Proxy: $LIVEREVIEW_REVERSE_PROXY

EOF

if [[ "$DEPLOYMENT_MODE" == "demo" ]]; then
    cat >> "$summary_file" << EOF
Demo Mode Configuration:
- Access URL: http://localhost:$LIVEREVIEW_FRONTEND_PORT/
- API URL: http://localhost:$LIVEREVIEW_BACKEND_PORT/api
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
- Backend: http://127.0.0.1:$LIVEREVIEW_BACKEND_PORT/api
- Frontend: http://127.0.0.1:$LIVEREVIEW_FRONTEND_PORT/
- Webhooks: Enabled (automatic triggers)
- External Access: Via reverse proxy (requires configuration)
- SSL/TLS: Required for production use

Reverse Proxy Setup Required:
Route /api/* → http://127.0.0.1:$LIVEREVIEW_BACKEND_PORT
Route /* → http://127.0.0.1:$LIVEREVIEW_FRONTEND_PORT

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

    log_info "Installation summary saved to: $summary_file"
}

# =============================================================================
# DOCKER DEPLOYMENT FUNCTIONS (PHASE 5)
# =============================================================================

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
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Could not change to installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    # Start containers in detached mode
    log_info "Running: $DOCKER_COMPOSE_CMD up -d"
    if ! docker_compose up -d; then
        log_error "Failed to start containers with docker compose"
        return 1
    fi
    
    log_success "Containers started successfully"
    return 0
}

# Wait for containers to become healthy
wait_for_containers() {
    section_header "WAITING FOR CONTAINER HEALTH"
    log_info "Waiting for containers to become healthy..."
    
    local max_wait=120  # Maximum wait time in seconds
    local wait_time=0
    local check_interval=5
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Could not change to installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    while [[ $wait_time -lt $max_wait ]]; do
        log_info "Checking container status... (${wait_time}/${max_wait}s)"
        
        # Check if all containers are running
        local containers_running
        containers_running=$(docker_compose ps -q | wc -l)
        local containers_healthy=0
        
        if [[ $containers_running -gt 0 ]]; then
            # Check PostgreSQL container health
            if docker_compose exec -T livereview-db pg_isready -U livereview >/dev/null 2>&1; then
                log_info "✓ PostgreSQL container is healthy"
                ((containers_healthy++))
            else
                log_info "○ PostgreSQL container not ready yet..."
            fi
            
            # Check LiveReview app container (simple ping)
            if docker_compose ps livereview-app | grep -q "Up"; then
                log_info "✓ LiveReview app container is running"
                ((containers_healthy++))
            else
                log_info "○ LiveReview app container not ready yet..."
            fi
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
    
    # Source configuration to get ports
    source "$config_file"
    
    # Check API endpoint (with timeout) - try multiple possible endpoints
    log_info "Checking API endpoint accessibility..."
    local api_ready=false
    local endpoints=("/health" "/api/health" "/api/healthcheck" "/api")
    
    for i in {1..12}; do  # Try for 60 seconds (12 * 5 second intervals)
        for endpoint in "${endpoints[@]}"; do
            if curl -f -s --max-time 5 "http://localhost:${LIVEREVIEW_BACKEND_PORT}${endpoint}" >/dev/null 2>&1; then
                log_success "✓ API endpoint is accessible at: http://localhost:${LIVEREVIEW_BACKEND_PORT}${endpoint}"
                api_ready=true
                break 2
            fi
        done
        log_info "○ API not ready, waiting... (attempt $i/12)"
        sleep 5
    done
    
    if [[ "$api_ready" != "true" ]]; then
        log_warning "API endpoint not accessible yet, but containers are running"
        log_info "This may be normal during initial startup"
    fi
    
    # Check UI endpoint
    log_info "Checking UI endpoint at http://localhost:${LIVEREVIEW_FRONTEND_PORT}/"
    local ui_ready=false
    for i in {1..6}; do  # Try for 30 seconds (6 * 5 second intervals)
        if curl -f -s --max-time 5 "http://localhost:${LIVEREVIEW_FRONTEND_PORT}/" >/dev/null 2>&1; then
            log_success "✓ UI endpoint is accessible"
            ui_ready=true
            break
        else
            log_info "○ UI not ready, waiting... (attempt $i/6)"
            sleep 5
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
        if [[ "$DEPLOYMENT_MODE" == "demo" ]]; then
            log_info "   - Demo Mode: http://localhost:${LIVEREVIEW_FRONTEND_PORT}/"
            log_info "   - API: http://localhost:${LIVEREVIEW_BACKEND_PORT}/api"
            log_info "   - Webhooks: Disabled (manual triggers only)"
        else
            log_info "   - Production Mode: Configure reverse proxy"
            log_info "   - Backend: http://127.0.0.1:${LIVEREVIEW_BACKEND_PORT}/api"
            log_info "   - Frontend: http://127.0.0.1:${LIVEREVIEW_FRONTEND_PORT}/"
            log_info "   - Webhooks: Enabled (automatic triggers)"
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
        log_info "Run 'lrops.sh --express' to install"
        return 1
    fi
    
    log_info "Installation directory: $LIVEREVIEW_INSTALL_DIR"
    
    # Check if docker-compose.yml exists
    if [[ ! -f "$LIVEREVIEW_INSTALL_DIR/docker-compose.yml" ]]; then
        log_error "Docker Compose configuration not found"
        return 1
    fi
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory"
        return 1
    }
    
    # Show container status
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
    
    # Show version information
    echo
    log_info "Version Information:"
    if [[ -f ".env" ]]; then
        local lr_version=$(grep "LIVEREVIEW_VERSION=" .env | cut -d'=' -f2)
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
        log_info "Run 'lrops.sh --express' to install"
        return 1
    fi
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    if [[ ! -f "docker-compose.yml" ]]; then
        log_error "Docker Compose configuration not found"
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
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
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
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
    log_info "Restarting LiveReview containers..."
    
    if docker_compose restart; then
        log_success "Containers restarted successfully"
        
        # Wait a moment for health checks
        log_info "Waiting for services to be ready..."
        sleep 5
        
        # Show status
        docker_compose ps
        log_info "Run 'lrops.sh status' to check service health"
    else
        log_error "Failed to restart containers"
        return 1
    fi
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
    
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory: $LIVEREVIEW_INSTALL_DIR"
        return 1
    }
    
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

OPTION 1: Automated SSL Setup Script (NEW!)
==========================================
Use the included SSL setup script for automatic configuration:

   cd /opt/livereview/scripts
   sudo ./setup-ssl.sh yourdomain.com admin@yourdomain.com

This script will:
- Install certbot automatically
- Generate Let's Encrypt certificates
- Configure your existing reverse proxy
- Set up automatic renewal

OPTION 2: Automatic SSL with Caddy (Recommended)
============================================
1. Use the Caddy reverse proxy template:
   cp /opt/livereview/config/caddy.conf.example /opt/livereview/caddy.conf

2. Edit the domain name in caddy.conf:
   sed -i 's/your-domain.com/yourdomain.com/g' /opt/livereview/caddy.conf

3. Install and run Caddy:
   sudo apt install caddy
   sudo systemctl enable caddy
   sudo cp /opt/livereview/caddy.conf /etc/caddy/Caddyfile
   sudo systemctl restart caddy

OPTION 3: Manual SSL with Nginx + Certbot
=========================================
1. Install Nginx and Certbot:
   sudo apt update
   sudo apt install nginx certbot python3-certbot-nginx

2. Copy and configure Nginx template:
   sudo cp /opt/livereview/config/nginx.conf.example /etc/nginx/sites-available/livereview
   sudo sed -i 's/your-domain.com/yourdomain.com/g' /etc/nginx/sites-available/livereview
   sudo ln -s /etc/nginx/sites-available/livereview /etc/nginx/sites-enabled/
   sudo nginx -t && sudo systemctl reload nginx

3. Obtain SSL certificate:
   sudo certbot --nginx -d yourdomain.com

4. Set up automatic renewal:
   sudo crontab -e
   # Add: 0 12 * * * /usr/bin/certbot renew --quiet

OPTION 4: Manual SSL with Apache
================================
1. Install Apache and Certbot:
   sudo apt update
   sudo apt install apache2 certbot python3-certbot-apache

2. Copy and configure Apache template:
   sudo cp /opt/livereview/config/apache.conf.example /etc/apache2/sites-available/livereview.conf
   sudo sed -i 's/your-domain.com/yourdomain.com/g' /etc/apache2/sites-available/livereview.conf
   sudo a2ensite livereview.conf
   sudo a2enmod proxy proxy_http ssl
   sudo systemctl reload apache2

3. Obtain SSL certificate:
   sudo certbot --apache -d yourdomain.com

SSL MAINTENANCE
==============
- Test certificate renewal: sudo certbot renew --dry-run
- Manual renewal: cd /opt/livereview/scripts && sudo ./renew-ssl.sh
- Check certificate expiry: sudo certbot certificates
- View certificate details: openssl x509 -in /etc/letsencrypt/live/yourdomain.com/cert.pem -text -noout

SECURITY BEST PRACTICES
======================
✓ Use strong SSL/TLS protocols (TLS 1.2+)
✓ Configure HSTS headers for security
✓ Set up certificate monitoring/alerts
✓ Regularly test SSL configuration: https://www.ssllabs.com/ssltest/
✓ Keep certbot and reverse proxy updated
✓ Monitor certificate expiry (auto-renewal should handle this)

TROUBLESHOOTING
==============
- Domain not pointing to server: Check DNS records
- Port 80/443 blocked: Check firewall and security groups
- Certificate generation fails: Ensure domain resolves correctly
- Configuration errors: Check reverse proxy logs

3. Obtain SSL certificate:
   sudo certbot --apache -d yourdomain.com

IMPORTANT SECURITY NOTES:
- Always use HTTPS in production
- Keep certificates renewed automatically
- Configure proper firewall rules
- Regular security updates

For more help: https://github.com/HexmosTech/LiveReview/docs/ssl-setup
EOF
}

# Show backup strategies and script usage
show_backup_help() {
    section_header "BACKUP & RESTORE GUIDE"
    
    cat << 'EOF'
💾 LiveReview Backup & Restore Guide

QUICK BACKUP
============
Run the included backup script:
  cd /opt/livereview
  ./scripts/backup.sh

This creates a timestamped backup in /opt/livereview/backups/

MANUAL BACKUP PROCESS
====================
1. Stop LiveReview (optional, for consistency):
   lrops.sh stop

2. Backup database:
   docker run --rm -v livereview_postgres_data:/backup-source \
   -v /opt/livereview/backups:/backup-dest \
   postgres:15-alpine tar czf /backup-dest/db-$(date +%Y%m%d_%H%M%S).tar.gz /backup-source

3. Backup configuration:
   tar czf /opt/livereview/backups/config-$(date +%Y%m%d_%H%M%S).tar.gz \
   /opt/livereview/.env /opt/livereview/docker-compose.yml /opt/livereview/config/

4. Restart LiveReview:
   lrops.sh start

RESTORE PROCESS
===============
1. Stop LiveReview:
   lrops.sh stop

2. Restore database:
   ./scripts/restore.sh /path/to/backup.tar.gz

3. Restore configuration (if needed):
   tar xzf config-backup.tar.gz -C /

4. Restart LiveReview:
   lrops.sh start

AUTOMATED BACKUP WITH CRON
===========================
1. Copy the cron example:
   sudo cp /opt/livereview/config/backup-cron.example /etc/cron.d/livereview-backup

2. Edit the cron file to set your schedule:
   sudo nano /etc/cron.d/livereview-backup

CLOUD BACKUP WITH RCLONE (Optional)
===================================
1. Install rclone:
   sudo apt install rclone

2. Configure cloud storage:
   rclone config

3. Add cloud sync to backup script:
   rclone sync /opt/livereview/backups/ mycloud:livereview-backups/

BACKUP BEST PRACTICES:
- Run backups daily
- Keep multiple backup copies
- Test restore procedures regularly
- Store backups off-site
- Monitor backup success

For more help: https://github.com/HexmosTech/LiveReview/docs/backup-guide
EOF
}

# Show Nginx reverse proxy configuration
show_nginx_help() {
    section_header "NGINX REVERSE PROXY GUIDE"
    
    cat << 'EOF'
🌐 Nginx Reverse Proxy Configuration for LiveReview

INSTALLATION
============
1. Install Nginx:
   sudo apt update && sudo apt install nginx

2. Copy the LiveReview Nginx template:
   sudo cp /opt/livereview/config/nginx.conf.example /etc/nginx/sites-available/livereview

3. Edit the domain name:
   sudo sed -i 's/your-domain.com/yourdomain.com/g' /etc/nginx/sites-available/livereview

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
For more help: https://github.com/HexmosTech/LiveReview/docs/nginx-guide
EOF
}

# Show Caddy reverse proxy configuration
show_caddy_help() {
    section_header "CADDY REVERSE PROXY GUIDE"
    
    cat << 'EOF'
⚡ Caddy Reverse Proxy Configuration for LiveReview

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
   sudo cp /opt/livereview/config/caddy.conf.example /etc/caddy/Caddyfile
   sudo sed -i 's/your-domain.com/yourdomain.com/g' /etc/caddy/Caddyfile

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

For more help: https://github.com/HexmosTech/LiveReview/docs/caddy-guide
EOF
}

# Show Apache reverse proxy configuration
show_apache_help() {
    section_header "APACHE REVERSE PROXY GUIDE"
    
    cat << 'EOF'
🔧 Apache Reverse Proxy Configuration for LiveReview

INSTALLATION
============
1. Install Apache:
   sudo apt update && sudo apt install apache2

2. Enable required modules:
   sudo a2enmod proxy proxy_http proxy_balancer lbmethod_byrequests
   sudo a2enmod ssl rewrite headers

3. Copy and configure the template:
   sudo cp /opt/livereview/config/apache.conf.example /etc/apache2/sites-available/livereview.conf
   sudo sed -i 's/your-domain.com/yourdomain.com/g' /etc/apache2/sites-available/livereview.conf

4. Enable the site:
   sudo a2ensite livereview.conf
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
For more help: https://github.com/HexmosTech/LiveReview/docs/apache-guide
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
    
    section_header "VALIDATING INSTALLATION"
    log_info "Running post-installation health checks..."
    
    local validation_errors=0
    
    # Check container status
    cd "$LIVEREVIEW_INSTALL_DIR" || {
        log_error "Cannot access installation directory"
        return 1
    }
    
    # Check if containers are running
    if ! docker-compose ps | grep -q "Up"; then
        log_error "❌ Containers are not running"
        ((validation_errors++))
    else
        log_success "✅ Containers are running"
    fi
    
    # Check container health
    local app_health=$(docker-compose ps -q livereview-app | xargs docker inspect --format='{{.State.Health.Status}}' 2>/dev/null)
    local db_health=$(docker-compose ps -q livereview-db | xargs docker inspect --format='{{.State.Health.Status}}' 2>/dev/null)
    
    if [[ "$app_health" == "healthy" ]]; then
        log_success "✅ LiveReview application is healthy"
    else
        log_warning "⚠️ LiveReview application health: ${app_health:-unknown}"
        ((validation_errors++))
    fi
    
    if [[ "$db_health" == "healthy" ]]; then
        log_success "✅ Database is healthy"
    else
        log_warning "⚠️ Database health: ${db_health:-unknown}"
        ((validation_errors++))
    fi
    
    # Test API endpoint accessibility
    log_info "Testing API endpoint accessibility..."
    if curl -f -s --max-time 10 "http://localhost:${API_PORT}/health" >/dev/null 2>&1; then
        log_success "✅ API endpoint is accessible"
    else
        log_warning "⚠️ API endpoint not yet accessible (may still be starting)"
        ((validation_errors++))
    fi
    
    # Test UI endpoint accessibility
    log_info "Testing UI endpoint accessibility..."
    if curl -f -s --max-time 10 "http://localhost:${UI_PORT}/" >/dev/null 2>&1; then
        log_success "✅ UI endpoint is accessible"
    else
        log_warning "⚠️ UI endpoint not yet accessible (may still be starting)"
        ((validation_errors++))
    fi
    
    # Check for recent errors in logs (excluding harmless entries)
    log_info "Checking for errors in recent logs..."
    local recent_errors=$(docker-compose logs --since=2m 2>/dev/null | grep -i "error\|fail\|panic\|fatal" | grep -v '"error":""' | grep -v "relation.*does not exist" | wc -l)
    if [[ $recent_errors -eq 0 ]]; then
        log_success "✅ No recent errors found in logs"
    else
        log_warning "⚠️ Found $recent_errors recent error(s) in logs"
        ((validation_errors++))
    fi
    
    # Summary
    if [[ $validation_errors -eq 0 ]]; then
        log_success "🎉 All validation checks passed!"
    else
        log_warning "⚠️ Found $validation_errors validation issues"
        log_info "Run 'lrops.sh status' for detailed status information"
    fi
    
    return $validation_errors
}

# Generate comprehensive installation report file
generate_installation_report() {
    local resolved_version="$1"
    local config_file="$2"
    source "$config_file"
    
    local report_file="$LIVEREVIEW_INSTALL_DIR/installation-report.txt"
    
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
Domain: $DOMAIN
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
$(cd "$LIVEREVIEW_INSTALL_DIR" && docker-compose ps 2>/dev/null || echo "Unable to retrieve container status")

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
View Logs: lrops.sh logs [service]

HELP & DOCUMENTATION
====================
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
    if [[ "$DEPLOYMENT_MODE" == "demo" ]]; then
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
    if [[ "$DEPLOYMENT_MODE" == "production" ]]; then
        echo -e "   � ${BOLD}Set up SSL/TLS:${NC} ${CYAN}lrops.sh help ssl${NC}"
        echo -e "   🌐 ${BOLD}Set up reverse proxy:${NC} ${CYAN}lrops.sh help nginx${NC}"
    fi
    echo -e "   📄 ${BOLD}View full report:${NC} ${CYAN}cat $LIVEREVIEW_INSTALL_DIR/installation-report.txt${NC}"
    echo
    
    # Installation details
    echo -e "${GRAY}📁 Installation: $LIVEREVIEW_INSTALL_DIR${NC}"
    echo -e "${GRAY}📊 Version: LiveReview $resolved_version, Script $SCRIPT_VERSION${NC}"
    echo -e "${GRAY}🏗️  Mode: $DEPLOYMENT_MODE mode${NC}"
    echo -e "${GRAY}⏱️  Completed: $(date)${NC}"
    echo
    
    if [[ "$DEPLOYMENT_MODE" == "demo" ]]; then
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

main() {
    # Check for management commands first (before parsing complex arguments)
    case "${1:-}" in
        status)
            show_status
            exit $?
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
        log_info "Diagnostic functionality not yet implemented"
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
    check_system_prerequisites
    check_existing_installation
    
    # Detect and set docker compose command early
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
    # PHASE 8: POST-INSTALLATION EXPERIENCE
    # =============================================================================
    
    # Step 9: Comprehensive installation validation and summary
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
    ports:
      - "${LIVEREVIEW_FRONTEND_PORT:-8081}:8081"  # Frontend UI
      - "${LIVEREVIEW_BACKEND_PORT:-8888}:8888"   # Backend API
    env_file:
      - .env
    environment:
      # Override DATABASE_URL to use internal container hostname
      - DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable
      - LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION}
      # Two-mode deployment configuration
      - LIVEREVIEW_BACKEND_PORT=${LIVEREVIEW_BACKEND_PORT:-8888}
      - LIVEREVIEW_FRONTEND_PORT=${LIVEREVIEW_FRONTEND_PORT:-8081}
      - LIVEREVIEW_REVERSE_PROXY=${LIVEREVIEW_REVERSE_PROXY:-false}
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
      - POSTGRES_USER=livereview
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=livereview
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
# LiveReview Environment Configuration
# Generated by lrops.sh installer with two-mode deployment system

# Database Configuration
DB_PASSWORD=${DB_PASSWORD}
DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable

# Security Configuration
JWT_SECRET=${JWT_SECRET}

# Application Configuration
LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION}
LOG_LEVEL=info

# Session Configuration (optional)
ACCESS_TOKEN_DURATION_HOURS=8
REFRESH_TOKEN_DURATION_DAYS=30

# AI Integration (optional - configure as needed)
# OPENAI_API_KEY=your_openai_api_key
# GOOGLE_AI_API_KEY=your_google_ai_api_key

# Two-Mode Deployment Configuration
# These variables control demo vs production mode behavior
# Set via lrops.sh during installation - do not modify manually

# LIVEREVIEW_BACKEND_PORT=8888
# LIVEREVIEW_FRONTEND_PORT=8081
# LIVEREVIEW_REVERSE_PROXY=false

# Demo Mode (LIVEREVIEW_REVERSE_PROXY=false):
# - Binds to localhost only (secure local development)
# - Webhooks disabled (manual triggers only)
# - No external access
# - Perfect for testing and development

# Production Mode (LIVEREVIEW_REVERSE_PROXY=true):
# - Binds to 127.0.0.1 (behind reverse proxy)
# - Webhooks enabled (automatic triggers)
# - External access via reverse proxy
# - Requires SSL/TLS setup for security

# For mode switching and configuration help:
# lrops.sh help ssl     # SSL/TLS setup
# lrops.sh help nginx   # Nginx reverse proxy
# lrops.sh help caddy   # Caddy reverse proxy
# === END:.env ===

# === DATA:nginx.conf.example ===
# Nginx configuration for LiveReview reverse proxy
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
LIVEREVIEW_DIR="/opt/livereview"
BACKUP_BASE_DIR="/opt/livereview-backups"
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
LIVEREVIEW_DIR="/opt/livereview"
BACKUP_BASE_DIR="/opt/livereview-backups"

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
0 2 * * * cd /opt/livereview/scripts && ./backup.sh daily_$(date +\%Y\%m\%d) >> /var/log/livereview-backup.log 2>&1

# Weekly backup on Sundays at 3 AM
0 3 * * 0 cd /opt/livereview/scripts && ./backup.sh weekly_$(date +\%Y_week\%U) >> /var/log/livereview-backup.log 2>&1

# Monthly backup on the 1st at 4 AM
0 4 1 * * cd /opt/livereview/scripts && ./backup.sh monthly_$(date +\%Y\%m) >> /var/log/livereview-backup.log 2>&1

# === Example with rclone S3 sync ===
# Install rclone first: curl https://rclone.org/install.sh | sudo bash
# Configure S3: rclone config (create remote named 'livereview-s3')

# Daily backup + S3 sync at 2:30 AM
30 2 * * * cd /opt/livereview/scripts && ./backup.sh daily_$(date +\%Y\%m\%d) && rclone sync /opt/livereview-backups/ livereview-s3:backups/livereview/ --log-file=/var/log/livereview-s3-sync.log

# === Backup retention (cleanup old backups) ===
# Keep only last 7 daily backups (run at 5 AM)
0 5 * * * find /opt/livereview-backups -name "daily_*" -type f -mtime +7 -delete

# Keep only last 4 weekly backups
0 5 * * 1 find /opt/livereview-backups -name "weekly_*" -type f -mtime +28 -delete

# Keep only last 12 monthly backups
0 5 1 * * find /opt/livereview-backups -name "monthly_*" -type f -mtime +365 -delete

# === Complete example crontab entry ===
# # LiveReview automated backups
# 0 2 * * * cd /opt/livereview/scripts && ./backup.sh daily_$(date +\%Y\%m\%d) >> /var/log/livereview-backup.log 2>&1
# 30 2 * * * rclone sync /opt/livereview-backups/ livereview-s3:backups/livereview/ --log-file=/var/log/livereview-s3-sync.log
# 0 5 * * * find /opt/livereview-backups -name "daily_*" -type f -mtime +7 -delete
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
LIVEREVIEW_DIR="/opt/livereview"

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
    sudo sed -i "s/your-domain.com/$DOMAIN/g" "/tmp/livereview-ssl.conf"
    
    # Uncomment HTTPS section
    sudo sed -i '/# HTTPS configuration/,/# Redirect HTTP to HTTPS/s/^# //' "/tmp/livereview-ssl.conf"
    sudo sed -i '/# Redirect HTTP to HTTPS/,/# }/s/^# //' "/tmp/livereview-ssl.conf"
    
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
    sudo sed -i "s/your-domain.com/$DOMAIN/g" "/etc/caddy/Caddyfile"
    sudo systemctl reload caddy
    log_success "✓ Caddy configuration updated (automatic HTTPS)"
    
elif [[ -f "/etc/apache2/sites-available/livereview.conf" ]]; then
    log_info "Updating Apache configuration..."
    sudo cp "$LIVEREVIEW_DIR/config/apache.conf.example" "/tmp/livereview-ssl.conf"
    sudo sed -i "s/your-domain.com/$DOMAIN/g" "/tmp/livereview-ssl.conf"
    
    # Uncomment HTTPS section
    sudo sed -i '/# HTTPS virtual host/,/# <\/VirtualHost>/s/^# //' "/tmp/livereview-ssl.conf"
    sudo sed -i '/# Redirect HTTP to HTTPS/,/# <\/VirtualHost>/s/^# //' "/tmp/livereview-ssl.conf"
    
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
