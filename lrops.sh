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
    --express                          Use secure defaults, no prompts
    --force                           Overwrite existing installation
    --version=v1.2.3                  Install specific version (default: latest)
    --dry-run                         Show what would be done without installing
    --verbose, -v                     Enable verbose output

MANAGEMENT OPTIONS:
    --help, -h                        Show this help message
    --version                         Show script version
    --diagnose                        Run diagnostic checks

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
    # Quick start
    lrops.sh --express

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
    local required_commands=("curl" "docker" "docker-compose" "jq")
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
    
    # Check Docker Compose
    if command -v docker-compose &> /dev/null; then
        local compose_version=$(docker-compose --version | cut -d' ' -f3 | sed 's/,//')
        log_success "Docker Compose is available (version: $compose_version)"
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
    
    # Extract data between markers, excluding the marker lines themselves
    if sed -n "/^# === DATA:${template_name} ===/,/^# === END:${template_name} ===/p" "$0" \
        | grep -v "^# === " > "$output_file"; then
        
        # Set appropriate permissions for script files
        case "$template_name" in
            *.sh)
                chmod +x "$output_file"
                ;;
        esac
        
        log_debug "Extracted template '$template_name' to '$output_file'"
        return 0
    else
        log_error "Failed to extract template '$template_name'"
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

# Interactive configuration prompts
gather_configuration() {
    local config_file="/tmp/lrops_config_$$"
    
    section_header "CONFIGURATION"
    
    if [[ "$EXPRESS_MODE" == "true" ]]; then
        log_info "Express mode: Using secure defaults"
        
        # Generate secure defaults
        local db_password
        db_password=$(generate_password 32)
        local jwt_secret
        jwt_secret=$(generate_jwt_secret)
        
        # Set configuration values
        cat > "$config_file" << EOF
DB_PASSWORD=$db_password
JWT_SECRET=$jwt_secret
API_PORT=8888
UI_PORT=8081
DOMAIN=localhost
LIVEREVIEW_VERSION=$1
EOF
    else
        log_info "Interactive configuration mode"
        log_info "Press Enter to use defaults shown in [brackets]"
        echo
        
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
        
        # API Port
        local api_port=8888
        echo -n "API port [8888]: "
        read -r user_input
        if [[ -n "$user_input" ]]; then
            api_port="$user_input"
        fi
        
        # UI Port
        local ui_port=8081
        echo -n "UI port [8081]: "
        read -r user_input
        if [[ -n "$user_input" ]]; then
            ui_port="$user_input"
        fi
        
        # Domain
        local domain="localhost"
        echo -n "Domain/hostname [localhost]: "
        read -r user_input
        if [[ -n "$user_input" ]]; then
            domain="$user_input"
        fi
        
        # Save configuration
        cat > "$config_file" << EOF
DB_PASSWORD=$db_password
JWT_SECRET=$jwt_secret
API_PORT=$api_port
UI_PORT=$ui_port
DOMAIN=$domain
LIVEREVIEW_VERSION=$1
EOF
    fi
    
    echo "$config_file"
}

# Validate configuration values
validate_configuration() {
    local config_file="$1"
    
    log_info "Validating configuration..."
    
    # Source the config
    source "$config_file"
    
    # Validate ports
    if ! [[ "$API_PORT" =~ ^[0-9]+$ ]] || [[ "$API_PORT" -lt 1024 ]] || [[ "$API_PORT" -gt 65535 ]]; then
        log_error "Invalid API port: $API_PORT (must be 1024-65535)"
        return 1
    fi
    
    if ! [[ "$UI_PORT" =~ ^[0-9]+$ ]] || [[ "$UI_PORT" -lt 1024 ]] || [[ "$UI_PORT" -gt 65535 ]]; then
        log_error "Invalid UI port: $UI_PORT (must be 1024-65535)"
        return 1
    fi
    
    if [[ "$API_PORT" == "$UI_PORT" ]]; then
        log_error "API and UI ports cannot be the same"
        return 1
    fi
    
    # Check if ports are available
    if command -v netstat >/dev/null 2>&1; then
        if netstat -tln | grep -q ":${API_PORT} "; then
            log_warning "Port $API_PORT appears to be in use"
        fi
        if netstat -tln | grep -q ":${UI_PORT} "; then
            log_warning "Port $UI_PORT appears to be in use"
        fi
    fi
    
    # Validate password strength
    if [[ ${#DB_PASSWORD} -lt 12 ]]; then
        log_warning "Database password is shorter than 12 characters"
    fi
    
    if [[ ${#JWT_SECRET} -lt 32 ]]; then
        log_warning "JWT secret is shorter than 32 characters"
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

# Generate .env file from template with user configuration
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
    
    # Add additional configuration
    cat >> "$output_file" << EOF

# Port configuration (auto-configured)
API_PORT=$API_PORT
UI_PORT=$UI_PORT

# Domain configuration
DOMAIN=$DOMAIN
LIVEREVIEW_API_URL=http://$DOMAIN:$API_PORT
EOF
    
    # Set secure permissions on .env file
    chmod 600 "$output_file"
    
    log_success "Generated .env file with secure permissions"
}

# Generate docker-compose.yml from template with user configuration
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
    
    # Update port mappings if not using defaults
    if [[ "$API_PORT" != "8888" ]] || [[ "$UI_PORT" != "8081" ]]; then
        log_info "Updating port mappings: API=$API_PORT, UI=$UI_PORT"
        sed -i "s/\"8888:8888\"/\"$API_PORT:8888\"/g" "$output_file"
        sed -i "s/\"8081:8081\"/\"$UI_PORT:8081\"/g" "$output_file"
    fi
    
    log_success "Generated docker-compose.yml with custom configuration"
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
    
    # Set executable permissions on scripts
    chmod +x "$LIVEREVIEW_INSTALL_DIR/scripts/"*.sh 2>/dev/null || true
    
    log_success "Configuration templates and scripts extracted"
}

# Generate installation summary file
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

Configuration:
- Installation Directory: $LIVEREVIEW_INSTALL_DIR
- API Port: $API_PORT
- UI Port: $UI_PORT
- Domain: $DOMAIN

Access URLs:
- API: http://$DOMAIN:$API_PORT/api
- Web UI: http://$DOMAIN:$UI_PORT

Important Files:
- Docker Compose: $LIVEREVIEW_INSTALL_DIR/docker-compose.yml
- Environment: $LIVEREVIEW_INSTALL_DIR/.env
- Configuration Templates: $LIVEREVIEW_INSTALL_DIR/config/
- Helper Scripts: $LIVEREVIEW_INSTALL_DIR/scripts/

Management Commands:
- Start: lrops.sh start
- Stop: lrops.sh stop
- Status: lrops.sh status
- Logs: lrops.sh logs

Configuration Help:
- SSL Setup: lrops.sh help ssl
- Backup Setup: lrops.sh help backup
- Nginx Config: lrops.sh help nginx
- Caddy Config: lrops.sh help caddy

For support, visit: https://github.com/HexmosTech/LiveReview
EOF

    log_info "Installation summary saved to: $summary_file"
}

main() {
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
    
    # Step 8: Installation complete (Phase 5 will add Docker deployment)
    section_header "INSTALLATION COMPLETE (Phase 4)"
    log_success "✅ Phase 1: Script foundation"
    log_success "✅ Phase 2: Version Management & GitHub Integration"
    log_success "✅ Phase 3: Embedded Templates & Configuration Files"
    log_success "✅ Phase 4: Installation Core Logic"
    log_info "   - Interactive/Express configuration ✓"
    log_info "   - Directory structure creation ✓"
    log_info "   - Configuration file generation ✓"
    log_info "   - Template and script extraction ✓"
    echo
    log_info "🚧 Next phase to implement:"
    log_info "   - Phase 5: Docker Deployment"
    echo
    log_info "Files created:"
    log_info "   - $LIVEREVIEW_INSTALL_DIR/.env"
    log_info "   - $LIVEREVIEW_INSTALL_DIR/docker-compose.yml"
    log_info "   - $LIVEREVIEW_INSTALL_DIR/config/ (templates)"
    log_info "   - $LIVEREVIEW_INSTALL_DIR/scripts/ (backup/restore)"
    log_info "   - $LIVEREVIEW_INSTALL_DIR/installation-summary.txt"
    echo
    log_info "To start LiveReview manually:"
    log_info "   cd $LIVEREVIEW_INSTALL_DIR && docker-compose up -d"
    
    # Cleanup
    rm -f "$config_file"
}

# =============================================================================
# SCRIPT ENTRY POINT
# =============================================================================

# Only run main if script is executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
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
      - "8888:8888"  # Backend API
      - "8081:8081"  # Frontend UI
    env_file:
      - .env
    environment:
      # Override DATABASE_URL to use internal container hostname
      - DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable
      - LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION}
      # API URL for frontend configuration (defaults to localhost if not set)
      - LIVEREVIEW_API_URL=${LIVEREVIEW_API_URL:-http://localhost:8888}
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
      test: ["CMD", "curl", "-f", "http://localhost:8888/api/health"]
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
# Generated by lrops.sh installer

# Database password (auto-generated secure password)
DB_PASSWORD=${DB_PASSWORD}

# Database connection string (auto-configured for Docker)
DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable

# JWT Secret Key (auto-generated secure key)
JWT_SECRET=${JWT_SECRET}

# LiveReview version
LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION}

# Log level
LOG_LEVEL=info

# Optional: Session token durations
ACCESS_TOKEN_DURATION_HOURS=8
REFRESH_TOKEN_DURATION_DAYS=30

# Optional: API configuration (uncomment and set if needed)
# OPENAI_API_KEY=your_openai_api_key
# GOOGLE_AI_API_KEY=your_google_ai_api_key

# API URL for frontend (auto-configured for localhost)
LIVEREVIEW_API_URL=http://localhost:8888
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
