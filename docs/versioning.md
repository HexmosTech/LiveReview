# LiveReview Versioning System Specification

## Overview

This document specifies the versioning system for LiveReview, including automated version management, binary version embedding, and Docker image tagging. The system uses semantic versioning (MAJOR.MINOR.PATCH) with Git-based automation.

## Requirements

### 1. Version Creation and Management
- **Interactive version bumping**: Script asks user for patch/minor/major increment
- **Git tag creation**: Automatically creates properly formatted Git tags
- **Version validation**: Ensures version follows semantic versioning rules
- **CI-friendly mode**: Non-interactive mode for automated environments

### 2. Binary Version Embedding
- **Build-time injection**: Version information embedded during Go build process
- **CLI version flag**: `./livereview --version` returns current version
- **API endpoint**: REST API endpoint returns version in JSON format
- **Development builds**: Special handling for non-tagged commits

### 3. Development Version Identification
- **Clean builds**: Shows Git commit hash (6 characters) for untagged commits
- **Modified builds**: Appends `-modified` suffix for uncommitted changes
- **Branch awareness**: Different handling for main/master vs feature branches

### 4. Docker Integration
- **Automated tagging**: Docker images tagged with same version as Git tags
- **Latest tag management**: Automatically updates `latest` tag for releases
- **Multi-registry support**: Can push to multiple Docker registries
- **Build metadata**: Docker images include version and build metadata

## Command Specifications

### 1. Version Management Commands

#### `lrops.py bump [--type TYPE] [--dry-run] [--ci]`
**Purpose**: Create a new version and Git tag

**Interactive Mode** (default):
```bash
python scripts/lrops.py bump
# Prompts:
# Current version: 1.2.3
# Detail: Would create tag v1.2.4 from current commit abc123f
# Select increment type:
# [1] Patch (1.2.4)
# [2] Minor (1.3.0) 
# [3] Major (2.0.0)
# Enter choice (1-3): _
```

**Non-interactive Mode**:
```bash
python scripts/lrops.py bump --type patch    # 1.2.3 -> 1.2.4
python scripts/lrops.py bump --type minor    # 1.2.3 -> 1.3.0
python scripts/lrops.py bump --type major    # 1.2.3 -> 2.0.0
python scripts/lrops.py bump --ci            # Auto-detects from commit messages
```

**Behavior**:
1. Check Git status (must be clean for release tags)
2. Get current version from latest Git tag
3. Calculate new version based on increment type
4. Create annotated Git tag with version
5. Push tag to remote repository
6. Generate release notes from commits since last tag

#### `lrops.py version [--json] [--build-info]`
**Purpose**: Display current version information

```bash
python scripts/lrops.py version
# Output: v1.2.3

python scripts/lrops.py version --json
# Output: {"version": "1.2.3", "commit": "abc123f", "dirty": false, "tag": "v1.2.3"}

python scripts/lrops.py version --build-info
# Output: v1.2.3 (commit abc123f, built 2025-08-26T10:30:00Z)
```

#### `lrops.py build [--version VERSION] [--docker] [--push]`
**Purpose**: Build binary with embedded version information

```bash
python scripts/lrops.py build                    # Build with auto-detected version
python scripts/lrops.py build --version v1.2.3  # Build with specific version
python scripts/lrops.py build --docker           # Build Docker image with version
python scripts/lrops.py build --docker --push   # Build and push Docker image
```

### 2. Makefile Integration

Add new targets to existing Makefile:

```makefile
# Version management targets
.PHONY: version version-bump version-patch version-minor version-major build-versioned docker-build-versioned

version:
	@python scripts/lrops.py version

version-bump:
	@python scripts/lrops.py bump

version-patch:
	@python scripts/lrops.py bump --type patch

version-minor:
	@python scripts/lrops.py bump --type minor

version-major:
	@python scripts/lrops.py bump --type major

build-versioned:
	@python scripts/lrops.py build

docker-build-versioned:
	@python scripts/lrops.py build --docker

docker-build-push:
	@python scripts/lrops.py build --docker --push
```

## Implementation Details

### 1. Version Detection Logic

#### For Tagged Commits (Releases)
```
git describe --exact-match --tags HEAD 2>/dev/null
→ v1.2.3
```

#### For Development Builds
```
# Clean working directory
git status --porcelain | wc -l == 0
git rev-parse --short=6 HEAD
→ abc123 

# Modified working directory  
git status --porcelain | wc -l > 0
git rev-parse --short=6 HEAD
→ abc123-modified
```

#### For CI/Development Builds from Tags
```
git describe --tags --always --dirty=-modified
→ v1.2.3-5-gabc123f-modified
```

### 2. Go Build Integration

#### Current Code Modification
**File**: `livereview.go`

**Current**:
```go
const (
    version = "0.1.0"
)
```

**New**:
```go
var (
    version   = "development"  // Set by -ldflags during build
    buildTime = "unknown"      // Set by -ldflags during build  
    gitCommit = "unknown"      // Set by -ldflags during build
)
```

#### Build Command Template
```bash
go build -ldflags="-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" -o livereview .
```

### 3. API Version Endpoint

Add to existing API routes:

**Endpoint**: `GET /api/version`

**Response**:
```json
{
  "version": "1.2.3",
  "gitCommit": "abc123f", 
  "buildTime": "2025-08-26T10:30:00Z",
  "dirty": false
}
```

### 4. Docker Integration

#### Dockerfile Modifications
**Current build stage** needs build args:
```dockerfile
# Build arguments for version injection
ARG VERSION=development
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

# Build command with version injection
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT}" \
    -v -o livereview .
```

#### Docker Build Command Template
```bash
docker build \
  --build-arg VERSION=${VERSION} \
  --build-arg BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
  --build-arg GIT_COMMIT=$(git rev-parse --short HEAD) \
  -t ${REGISTRY}/${IMAGE_NAME}:${VERSION} \
  -t ${REGISTRY}/${IMAGE_NAME}:latest \
  .
```

## File Structure

```
scripts/
├── lrops.py              # Main versioning script
└── version_utils.py      # Utility functions (optional)

.github/workflows/         # CI integration
├── release.yml           # Release workflow
└── build.yml            # Build workflow with versioning
```

## Python Script Requirements

### Dependencies
- **Built-in only**: No external dependencies for CI compatibility
- **Modules used**: `subprocess`, `json`, `argparse`, `datetime`, `re`, `sys`, `os`

### Key Features
- **Git operations**: Tag creation, status checking, commit parsing
- **Semantic versioning**: Version parsing and increment logic
- **Cross-platform**: Works on Linux, macOS, Windows
- **Error handling**: Proper error messages and exit codes
- **Logging**: Verbose mode for debugging

### Configuration
- **Registry URLs**: Configurable Docker registries
- **Image names**: Configurable image naming
- **Tag patterns**: Configurable tag naming conventions
- **Branch rules**: Different behavior for different branches

## CI/CD Integration

### GitHub Actions Example
```yaml
name: Release
on:
  workflow_dispatch:
    inputs:
      version_type:
        description: 'Version increment type'
        required: true
        default: 'patch'
        type: choice
        options:
        - patch
        - minor  
        - major

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      
      - name: Create Release
        run: python scripts/lrops.py bump --type ${{ inputs.version_type }} --ci
        
      - name: Build and Push Docker
        run: python scripts/lrops.py build --docker --push
```

## Error Handling

### Common Error Scenarios
1. **Dirty working directory**: Prevent release tags on uncommitted changes
2. **No Git repository**: Graceful handling when not in Git repo
3. **No existing tags**: Handle initial version (start with v0.1.0)
4. **Permission issues**: Clear error messages for Git/Docker permissions
5. **Network issues**: Retry logic for Docker registry pushes
6. **Invalid version format**: Validation of version strings

### Exit Codes
- `0`: Success
- `1`: General error
- `2`: Git repository error  
- `3`: Version format error
- `4`: Docker operation error
- `5`: Network/registry error

## Security Considerations

### Git Operations
- **Signed tags**: Support for GPG-signed tags
- **Protected branches**: Respect branch protection rules
- **Authentication**: Handle Git authentication properly

### Docker Operations  
- **Registry authentication**: Secure credential handling
- **Image scanning**: Integration with security scanners
- **Multi-stage builds**: Minimize attack surface

## Future Enhancements

### Phase 2 Features
- **Changelog generation**: Automatic changelog from commit messages
- **Release notes**: GitHub/GitLab release creation
- **Artifact uploading**: Binary uploads to release pages
- **Notification integration**: Slack/Discord notifications
- **Rollback capability**: Easy version rollback mechanisms

### Integration Possibilities
- **IDE integration**: VS Code extension for version management
- **Git hooks**: Pre-commit hooks for version validation
- **Package managers**: Integration with Go module versioning
- **Monitoring**: Version tracking in application monitoring

---

## Implementation Phases

### Phase 1: Core Functionality
1. Create `scripts/lrops.py` with basic version detection and bumping
2. Modify `livereview.go` for version embedding
3. Add API version endpoint
4. Update Makefile with version targets
5. Test manually with different scenarios

### Phase 2: Docker Integration  
1. Modify Dockerfile for version injection
2. Add Docker build/push functionality to script
3. Test Docker image version embedding
4. Add registry configuration

### Phase 3: CI/CD Integration
1. Create GitHub Actions workflows
2. Add automated testing for version script
3. Add release automation
4. Documentation and team training

This specification provides a complete foundation for implementing the LiveReview versioning system with minimal dependencies and maximum automation.