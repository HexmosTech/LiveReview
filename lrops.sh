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
LIVEREVIEW_INSTALL_DIR="/opt/livereview"
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
# MAIN EXECUTION LOGIC
# =============================================================================

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
        log_info "List embedded data functionality not yet implemented"
        exit 0
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
    
    # Show current implementation status
    section_header "IMPLEMENTATION STATUS"
    log_success "✅ Phase 1: Script foundation implemented"
    log_success "✅ Phase 2: Version Management & GitHub Integration"
    log_info "   - GitHub Container Registry API integration"
    log_info "   - Semantic version filtering and validation"
    log_info "   - Latest version resolution"
    log_info "   - Version pinning capabilities"
    echo
    log_info "🚧 Next phases to implement:"
    log_info "   - Phase 3: Embedded Data System"
    log_info "   - Phase 4: Installation Core Logic"
    log_info "   - Phase 5: Docker Deployment"
    echo
    log_info "Run with --dry-run to see installation plan"
    log_info "Run with --help to see all options"
}

# =============================================================================
# SCRIPT ENTRY POINT
# =============================================================================

# Only run main if script is executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi

# =============================================================================
# EMBEDDED DATA SECTION (TO BE IMPLEMENTED IN PHASE 3)
# =============================================================================

# Future: Embedded docker-compose.yml, configuration templates, and scripts
# will be added here using the data extraction framework