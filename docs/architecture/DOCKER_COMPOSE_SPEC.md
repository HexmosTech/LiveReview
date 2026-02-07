# LiveReview Operations Management with `lrops.py`

## Overview
`lrops.py` is a unified Python script that handles everything from development to production deployment and customer operations. It's self-contained, requires only Python standard library, and provides a single interface for all LiveReview operations.

## Core Philosophy

**One script to rule them all** - `lrops.py`:
- Replaces Makefile targets for development
- Manages Docker builds and deployments  
- Enables customer self-service operations
- Works identically for developers and customers
- Zero external dependencies (Python stdlib only)

## Script Architecture

```python
#!/usr/bin/env python3
"""
LiveReview Operations Script (lrops.py)
Unified script for development, deployment, and operations.
"""

# ====== PREAMBLE: VERSION & METADATA ======
# Version management is git-tag based:
# - "tip": Current development (no git tag)
# - "v1.2.3": Release versions (git tags)
# - Migrations determined by git tag timestamp vs migration file timestamps

DOCKER_IMAGE_TAG = "livereview"
SUPPORTED_PLATFORMS = ["linux/amd64", "linux/arm64"]
MIN_DOCKER_VERSION = "20.10"
MIN_COMPOSE_VERSION = "2.0"

# ====== IMPLEMENTATION ======
import os, sys, json, subprocess, argparse
from datetime import datetime
from pathlib import Path

class LiveReviewOps:
    def __init__(self):
        self.base_dir = Path.cwd()
        self.data_dir = self.base_dir / "data"
        self.backup_dir = self.data_dir / "backups"
    
    def get_current_version(self):
        """Get current version from git tags or return 'tip'"""
        try:
            result = subprocess.run(['git', 'describe', '--tags', '--exact-match', 'HEAD'], 
                                  capture_output=True, text=True, check=True)
            return result.stdout.strip()
        except subprocess.CalledProcessError:
            return 'tip'
    
    def get_available_versions(self):
        """Get all available versions from git tags"""
        try:
            result = subprocess.run(['git', 'tag', '-l', 'v*', '--sort=-version:refname'], 
                                  capture_output=True, text=True, check=True)
            return [v for v in result.stdout.strip().split('\n') if v.startswith('v')]
        except subprocess.CalledProcessError:
            return []
```

## Command Structure

### Development Commands ("tip" - current code)
```bash
python3 lrops.py dev build         # go build -o livereview
python3 lrops.py dev run           # ./livereview api (with air reload)
python3 lrops.py dev test          # go test -v ./...
python3 lrops.py dev ui-dev        # npm run start (port 8081)
python3 lrops.py dev ui-build      # npm run build
python3 lrops.py dev db-start      # Start local PostgreSQL
python3 lrops.py dev db-migrate    # Run dbmate migrations
python3 lrops.py dev clean         # Clean build artifacts
```

### Build & Release Commands
```bash
# Docker operations
python3 lrops.py build tip         # Build Docker image as "livereview:tip"
python3 lrops.py build compose     # Generate docker-compose.yml
python3 lrops.py build test        # Test Docker build locally

# Release management
python3 lrops.py release create v1.2.3    # Create git tag + Docker image
python3 lrops.py release push v1.2.3      # Push to registry
python3 lrops.py version list             # List available versions
python3 lrops.py version migrations v1.2.3  # Show migrations in version
```

### Operations Commands
```bash
# Installation & management
python3 lrops.py install [version]    # Install specific or latest version
python3 lrops.py start/stop/restart   # Service control
python3 lrops.py upgrade [version]    # Upgrade with auto-backup & migration

# Monitoring & maintenance
python3 lrops.py status            # System status + current version
python3 lrops.py logs [service]    # View logs
python3 lrops.py backup [name]     # Create backup
python3 lrops.py restore [backup]  # Restore from backup
python3 lrops.py health            # Health check
python3 lrops.py config init/validate/edit  # Configuration management
```

## Docker Architecture

### Container Structure
- **livereview-app**: Backend (Go:8888) + Frontend (React:8081) with embedded UI
- **livereview-db**: PostgreSQL 15 database (internal port 5432)

### Generated Files (`lrops.py` creates and manages these)

#### Dockerfile (Multi-stage build)
```dockerfile
# Stage 1: Build UI
FROM node:18-alpine AS ui-builder
WORKDIR /app/ui
COPY ui/package*.json ./
RUN npm ci --only=production && npm run build

# Stage 2: Build Go binary with version injection
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
RUN apk add --no-cache curl git
RUN curl -fsSL -o /usr/local/bin/dbmate \
    https://github.com/amacneil/dbmate/releases/latest/download/dbmate-linux-amd64 && chmod +x /usr/local/bin/dbmate

COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/ui/dist ./ui/dist

# Version info injected by lrops.py via ldflags
RUN CGO_ENABLED=0 GOOS=linux go build -o livereview .

# Stage 3: Runtime
FROM alpine:latest
RUN apk --no-cache add ca-certificates curl python3
COPY --from=go-builder /usr/local/bin/dbmate /usr/local/bin/dbmate
COPY --from=go-builder /app/livereview /root/livereview
COPY --from=go-builder /app/livereview.toml /root/livereview.toml
COPY --from=go-builder /app/db/migrations /root/db/migrations/
COPY lrops.py /root/lrops.py

WORKDIR /root
EXPOSE 8888 8081
ENTRYPOINT ["python3", "/root/lrops.py", "container"]
CMD ["start"]
```

#### docker-compose.yml
```yaml
version: '3.8'
services:
  livereview-app:
    image: livereview:${LIVEREVIEW_VERSION}
    container_name: livereview-app
    ports: ["8888:8888", "8081:8081"]
    env_file: [.env]
    environment:
      - DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable
      - LIVEREVIEW_VERSION=${LIVEREVIEW_VERSION}
    depends_on:
      livereview-db: {condition: service_healthy}
    volumes: ["./livereview.toml:/root/livereview.toml:ro"]
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "python3", "/root/lrops.py", "container", "health"]
      interval: 30s

  livereview-db:
    image: postgres:15-alpine
    container_name: livereview-db
    environment:
      - POSTGRES_USER=livereview
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=livereview
    volumes: ["livereview_data:/var/lib/postgresql/data"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U livereview -d livereview"]
      interval: 10s
    restart: unless-stopped

volumes:
  livereview_data: {name: livereview_data}
```

## Configuration Management

### Environment Files
**Required**: `.env` file (application and dbmate dependency)
- `DATABASE_URL`: PostgreSQL connection string
- `DB_PASSWORD`: Database password (generated by lrops.py)
- `LIVEREVIEW_VERSION`: Current version

**Strategy**: 
- Template-based generation from `.env.example`
- Runtime validation (container fails if missing/invalid)
- Never committed to version control

### Application Config
**livereview.toml**: AI providers, GitLab tokens, batch settings
- Mounted into container as read-only
- Environment variables can override settings

## Version Management

### Git-Tag Based Versioning
**Philosophy**: Versions are git tags, no hardcoded values

- **"tip"**: Development (no git tag) - includes all migrations
- **"v1.2.3"**: Release (git tag) - includes migrations up to tag timestamp

### Migration Logic
```python
def get_migrations_for_version(self, version):
    if version == 'tip':
        return all_migrations()  # Development gets everything
    
    # For releases, include migrations that existed when tag was created
    tag_timestamp = get_git_tag_timestamp(version)
    return [m for m in all_migrations() if migration_date(m) <= tag_timestamp]
```

### Release Workflow
```bash
# 1. Develop on tip
python3 lrops.py dev build && python3 lrops.py dev test

# 2. Create release (auto-determines migrations)
python3 lrops.py release create v1.2.3

# 3. Customer installation gets exact version
python3 lrops.py install v1.2.3
```

## Version Information Flow

### Binary Version Injection Strategy
**Problem**: Binary needs version for API endpoints, frontend display, and runtime validation.

**Solution**: Build-time injection via Go ldflags.

**Go Application Setup**:
```go
// livereview.go
var (
    version   = "development"  // Set by ldflags
    buildTime = "unknown"      // Set by ldflags
)

func main() {
    app := &cli.App{
        Version: version,
        Commands: []*cli.Command{
            cmd.VersionCommand(version, buildTime),
            // ... other commands
        },
    }
}
```

**API Endpoint** (internal/api/version.go):
```go
type VersionResponse struct {
    Version     string `json:"version"`
    BuildTime   string `json:"build_time"`
    Environment string `json:"environment"`
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
    response := VersionResponse{
        Version:     s.version,
        BuildTime:   s.buildTime,
        Environment: getEnvironment(s.version),
    }
    json.NewEncoder(w).Encode(response)
}
```

**Frontend Component** (ui/src/components/VersionDisplay.tsx):
```typescript
const VersionDisplay: React.FC = () => {
    const [version, setVersion] = useState<VersionInfo | null>(null);
    
    useEffect(() => {
        fetch('/api/version').then(res => res.json()).then(setVersion);
    }, []);
    
    return version ? (
        <div className="version-info">
            <span className={`badge ${version.environment}`}>
                {version.environment === 'development' ? '🚧' : '🚀'} 
                LiveReview {version.version}
            </span>
        </div>
    ) : null;
};
```

**Build Process** (in lrops.py):
```python
def build_go_binary(self, version):
    if version == 'tip':
        git_commit = self._get_git_commit()[:8]
        version_string = f"tip-{git_commit}"
    else:
        version_string = version
    
    ldflags = f"-X main.version={version_string} -X main.buildTime={datetime.now().isoformat()}"
    subprocess.run(['go', 'build', '-ldflags', ldflags, '-o', 'livereview'], check=True)
``` 
    ↓
lrops.py detects version
    ↓  
Go build with -ldflags version injection
    ↓
Binary knows its version at runtime
    ↓
API endpoint /api/version returns version info
    ↓
Frontend fetches and displays version
```

### Development vs Production Versions

```bash
# Development (tip)
python3 lrops.py dev build
# Binary version: "tip-abc1234" (tip + git commit)
# API returns: {"version": "tip-abc1234", "environment": "development"}

# Production (release)
python3 lrops.py release create v1.2.3
# Binary version: "v1.2.3"
# API returns: {"version": "v1.2.3", "environment": "production"}
```

This approach ensures that:
1. ✅ **Binary knows its version** - Injected at build time via ldflags
2. ✅ **API can return version** - Version info passed to server and exposed via /api/version
3. ✅ **Frontend shows version** - Fetches from API and displays in UI
4. ✅ **No hardcoded versions** - Everything derived from git tags
5. ✅ **Development vs production** - Clear distinction between tip and releases

### Customer Receives
1. **Single file**: `lrops.py` (self-contained, no dependencies)
2. **Simple instructions**: "Download lrops.py and run `python3 lrops.py install`"

### Installation Process
```bash
# Customer runs this on their server
curl -o lrops.py https://releases.livereview.com/v1.2.3/lrops.py
python3 lrops.py install

# lrops.py automatically:
# 1. Checks system requirements (Docker, Python)
# 2. Downloads required Docker images 
# 3. Generates configuration (.env, docker-compose.yml)
# 4. Creates data directories
# 5. Starts services
# 6. Runs health checks
# 7. Shows access URLs and credentials
```

### Customer Operations
```bash
# Day-to-day operations
python3 lrops.py status          # Check system health
python3 lrops.py logs            # View application logs  
python3 lrops.py backup          # Create backup
python3 lrops.py restart         # Restart services

# Maintenance
python3 lrops.py upgrade         # Check for updates
python3 lrops.py upgrade v1.2.4  # Upgrade to specific version
python3 lrops.py restore backup-20250802  # Restore from backup

# Configuration
python3 lrops.py config edit     # Safely edit configuration
python3 lrops.py config validate # Validate current config
```

## Developer Workflow with `lrops.py`

### Daily Development
```bash
# Replace make commands with lrops.py
python3 lrops.py dev build       # Instead of: make build
python3 lrops.py dev run         # Instead of: make develop  
python3 lrops.py dev test        # Instead of: make test
python3 lrops.py dev ui-dev      # Instead of: cd ui && npm start
python3 lrops.py dev db-start    # Instead of: ./pgctl.sh start
```

### Build & Release Process
```bash
# Build tip version for testing
python3 lrops.py build tip
python3 lrops.py build test      # Test the build locally

# Create release
python3 lrops.py release create v1.2.4
python3 lrops.py release push v1.2.4    # Push to registry

# Generate customer distribution
python3 lrops.py release package v1.2.4  # Creates standalone lrops.py
```

### Testing Customer Experience
```bash
# Test customer workflow locally
python3 lrops.py install --test-mode
python3 lrops.py upgrade v1.2.4 --dry-run
```

## Advanced `lrops.py` Features

### Configuration Management
```python
# lrops.py handles all configuration through embedded templates
def generate_env_file(self, db_password=None):
    """Generate .env file from embedded template"""
    if not db_password:
        db_password = self._generate_secure_password()
    
    env_content = ENV_TEMPLATE.format(
        db_password=db_password,
        livereview_version=self.version
    )
    
    with open('.env', 'w') as f:
        f.write(env_content)

def generate_compose_file(self):
    """Generate docker-compose.yml from embedded template"""
    compose_content = DOCKER_COMPOSE_TEMPLATE.format(
        livereview_version=self.version,
        image_tag=DOCKER_IMAGE_TAG
    )
    
    with open('docker-compose.yml', 'w') as f:
        f.write(compose_content)
```

### Smart Migration System
```python
def check_migration_needed(self, target_version):
    """Check if database migration is needed for target version"""
    try:
        # Get current database version
        result = subprocess.run([
            'docker', 'exec', 'livereview-db', 
            'psql', '-U', 'livereview', '-d', 'livereview', '-tAc',
            "SELECT value FROM livereview_metadata WHERE key='version'"
        ], capture_output=True, text=True)
        
        current_db_version = result.stdout.strip()
        
        # Get migrations required for target version
        required_migrations = self.get_migrations_for_version(target_version)
        current_migrations = self.get_migrations_for_version(current_db_version)
        
        # Check if we need new migrations
        new_migrations = set(required_migrations) - set(current_migrations)
        return len(new_migrations) > 0, list(new_migrations)
    except:
        return True, []  # Assume migration needed if check fails

def run_migrations(self, target_version):
    """Run database migrations if needed for target version"""
    migration_needed, new_migrations = self.check_migration_needed(target_version)
    
    if migration_needed:
        print(f"🔄 Running {len(new_migrations)} new migrations for {target_version}...")
        subprocess.run([
            'docker', 'exec', 'livereview-app',
            'dbmate', 'up'
        ])
        self._update_db_version(target_version)
    else:
        print("✅ Database is up to date")

def _update_db_version(self, version):
    """Update database version metadata"""
    subprocess.run([
        'docker', 'exec', 'livereview-db',
        'psql', '-U', 'livereview', '-d', 'livereview', '-c',
        f"UPDATE livereview_metadata SET value='{version}' WHERE key='version'"
    ])
```

### Git-Based Version Management
```python
def create_release(self, version):
    """Create a new release version using git tags"""
    # Validate version format
    if not version.startswith('v') or not self._is_valid_semver(version[1:]):
        raise ValueError(f"Invalid version format: {version}. Use vX.Y.Z format.")
    
    # Check if tag already exists
    existing_tags = self.get_available_versions()
    if version in existing_tags:
        raise ValueError(f"Version {version} already exists")
    
    # Create git tag
    print(f"🏷️  Creating git tag: {version}")
    subprocess.run(['git', 'tag', version], check=True)
    
    # Build Docker image with version tag
    print(f"🐳 Building Docker image: livereview:{version}")
    subprocess.run([
        'docker', 'build', '-t', f'livereview:{version}', '.'
    ], check=True)
    
    print(f"✅ Release {version} created successfully")
    print(f"📋 Includes {len(self.get_migrations_for_version(version))} migrations")

def upgrade(self, target_version=None):
    """Upgrade to specified version with git-tag based logic"""
    if not target_version:
        available = self.get_available_versions()
        if available:
            target_version = available[0]  # Latest version
        else:
            print("No versions available for upgrade")
            return
    
    current_version = self.get_current_version()
    print(f"🚀 Upgrading from {current_version} to {target_version}")
    
    # 1. Create backup
    backup_name = f"pre-upgrade-{target_version}"
    self.create_backup(backup_name)
    
    # 2. Check what migrations are needed
    migration_needed, new_migrations = self.check_migration_needed(target_version)
    if migration_needed:
        print(f"📋 Will apply {len(new_migrations)} new migrations")
    
    # 3. Pull/build new version
    if target_version != 'tip':
        subprocess.run(['docker', 'pull', f'livereview:{target_version}'])
    
    # 4. Update compose file to use new version
    self.generate_compose_file(target_version)
    
    # 5. Restart with new version
    subprocess.run(['docker-compose', 'up', '-d'])
    
    # 6. Run migrations if needed
    self.run_migrations(target_version)
    
    # 7. Health check
    if self.health_check():
        print(f"✅ Successfully upgraded to {target_version}")
    else:
        print("❌ Upgrade failed, consider rollback")

def list_versions_with_details(self):
    """List all available versions with migration info"""
    versions = self.get_available_versions()
    current = self.get_current_version()
    
    print(f"📋 Available LiveReview versions:")
    print(f"   Current: {current}")
    print()
    
    for version in versions:
        migrations = self.get_migrations_for_version(version)
        tag_date = self._get_tag_date(version)
        
        marker = "→" if version == current else " "
        print(f"{marker} {version:12} {tag_date:12} ({len(migrations):2d} migrations)")
```

### Backup and Restore System
```python
def create_backup(self, name=None):
    """Create full system backup"""
    if not name:
        name = f"backup-{datetime.now().strftime('%Y%m%d-%H%M%S')}"
    
    backup_path = self.backup_dir / f"{name}.tar.gz"
    
    # 1. Backup database
    db_backup = f"{name}-db.sql"
    subprocess.run([
        'docker', 'exec', 'livereview-db',
        'pg_dump', '-U', 'livereview', '-d', 'livereview',
        '--clean', '--if-exists'
    ], stdout=open(db_backup, 'w'))
    
    # 2. Backup configuration
    config_files = ['.env', 'livereview.toml', 'docker-compose.yml']
## lrops.py Implementation

### Core Methods (Sample)
```python
class LiveReviewOps:
    def create_release(self, version):
        """Create git tag and build versioned Docker image"""
        # Validate format, create tag, build image
        subprocess.run(['git', 'tag', version], check=True)
        subprocess.run(['docker', 'build', '-t', f'livereview:{version}', '.'], check=True)
    
    def upgrade(self, target_version=None):
        """Safe upgrade with automatic backup"""
        self.create_backup(f"pre-upgrade-{target_version}")
        self.generate_compose_file(target_version)
        subprocess.run(['docker-compose', 'up', '-d'], check=True)
        self.run_migrations()
    
    def create_backup(self, name):
        """Create backup of database and configuration"""
        db_backup = f"backup-{name}.sql"
        subprocess.run(['docker-compose', 'exec', 'postgres', 'pg_dump', '-U', 'livereview', 'livereview'], 
                      stdout=open(db_backup, 'w'))
        return f"backup-{name}.tar.gz"
```

### Customer Workflow
```bash
# Installation
curl -o lrops.py https://releases.livereview.com/v1.2.3/lrops.py
python3 lrops.py install v1.2.3

# Operations
python3 lrops.py status              # Check system health
python3 lrops.py backup create       # Manual backup
python3 lrops.py upgrade            # Upgrade to latest
python3 lrops.py logs app           # View logs
```

### File Structure
**Development Environment**:
```
LiveReview/
├── lrops.py ← Master operations script
├── livereview.go, go.mod/sum
├── ui/ ← React frontend
├── db/migrations/
├── .env.example
└── livereview.toml
```

**Customer Environment**:
```
/opt/livereview/
├── lrops.py ← Self-contained script
├── .env ← Generated config
├── docker-compose.yml ← Generated
├── livereview.toml
└── data/ ← Volumes and backups
```

### Benefits Summary
- **Developers**: Unified workflow, consistent environments, easy testing
- **Customers**: Zero setup, self-service operations, safe upgrades
- **Operations**: Reduced support, consistent deployments, clear troubleshooting

---

*This specification provides a complete blueprint for transforming LiveReview into a simple, customer-friendly solution managed entirely through the self-contained `lrops.py` script.*
  livereview-app:
    build: .
    ports:
      - "8888:8888"
      - "8081:8081"
    env_file:
      - .env
    environment:
      # Override DATABASE_URL to use internal container hostname
      - DATABASE_URL=postgres://livereview:${DB_PASSWORD}@livereview-db:5432/livereview?sslmode=disable
    depends_on:
      livereview-db:
        condition: service_healthy
    restart: unless-stopped

  livereview-db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=livereview
      - POSTGRES_PASSWORD=${DB_PASSWORD}
      - POSTGRES_DB=livereview
    volumes:
      - livereview_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U livereview -d livereview"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

volumes:
  livereview_data:
```

### .env File Validation
The docker-compose.yml **requires** a `.env` file with DB_PASSWORD:
```bash
# .env file must contain:
DB_PASSWORD=your_secure_password_here
```

### Migration Integration
- Install dbmate in the Dockerfile
- Copy migrations into the container during build
- Run migrations in entrypoint script before starting the app
- Use DATABASE_URL from .env file for dbmate
- **Fail fast**: If .env is missing or DATABASE_URL is invalid, container fails to start

### Configuration Management
- **Mandatory .env file** - Container fails without it
- Use existing `livereview.toml` for application configuration  
- Load .env variables into container environment
- Validate required variables at build time and runtime
- Never include actual .env in Docker image or version control

## Versioning Strategy

### Semantic Versioning
- Use git tags for version management
- Docker images tagged with version numbers
- Full stack upgrades with single version bump

### Release Process
1. Update version in code
2. Create git tag
3. Build new Docker images
4. Update docker-compose.yml with new tags
5. Deploy new stack

## Security Considerations

### Database Security
- Generate strong random password for PostgreSQL
- Use environment variables for credentials
- Database not exposed to host network
- Internal communication only

### Application Security
- Run as non-root user in container
- Minimal Alpine base image
- Only expose necessary ports
- Use .dockerignore to exclude sensitive files
- **Never commit .env files** to version control
- Validate .env file existence and content at multiple stages

## Performance Optimizations

### Build Optimizations
- Multi-stage builds to reduce final image size
- Layer caching for faster rebuilds
- Minimal base images (Alpine)
- Pre-built dependencies

### Runtime Optimizations
- Named volumes for database persistence
- Restart policies for automatic recovery
- Health checks for containers

## Requirements Clarification ✅

1. **Startup Process**: ✅ Both UI (8081) and backend (8888) run simultaneously from same `livereview` binary
2. **Migration Timing**: ✅ Run migrations only on upgrades (need smart version detection)
3. **Configuration**: ✅ DATABASE_URL sufficient for now 
4. **Monitoring**: ✅ Include health checks and monitoring endpoints
5. **Development vs Production**: ✅ Single docker-compose file for simplicity
6. **Build Process**: ✅ Webpack builds static assets (HTML/JS/CSS) copied to Go embed
7. **Data Persistence**: ✅ Database + `livereview.toml` config file
8. **External Dependencies**: ✅ Outgoing API calls (Gemini/OpenAI) - no impact on Docker setup

## Key Operational Challenges Identified

The main complexity is around **upgrade management**:
- How to detect when migrations are needed
- Version tracking and rollback capability  
- Data backup before upgrades
- Simple operational commands
- Release management

## Simple Operations Proposal 🚀

### The `livereview-ops` Script
Create a single script that handles ALL operations:

```bash
./livereview-ops [command]

Commands:
  build              # Build new "tip" version for testing
  test               # Test the current build
  release [version]  # Tag current build as version (e.g., v1.2.3)
  upgrade [version]  # Upgrade to specific version (with backup)
  status             # Show current version, available versions
  backup             # Manual backup of data
  restore [backup]   # Restore from backup
  logs               # Show application logs
  health             # Health check of services
  data               # Show data storage locations and sizes
```

### Version Management Strategy

#### 1. **Build & Test** ("tip" development)
```bash
# Build latest code as "tip" version
./livereview-ops build
# 🔧 Builds Docker image tagged as "livereview:tip"
# 🔧 Creates test environment with temporary database
# 🔧 Runs automated tests

# Test the build
./livereview-ops test
# 🧪 Starts temporary stack on different ports (8889/8082)
# 🧪 Runs health checks and integration tests
# 🧪 Shows test results and cleanup
```

#### 2. **Release Management**
```bash
# Create a release from current tip
./livereview-ops release v1.2.3
# 🏷️  Tags Docker image: livereview:tip → livereview:v1.2.3
# 🏷️  Updates version in go binary and toml
# 🏷️  Creates git tag
# 🏷️  Updates available versions list
```

#### 3. **Smart Upgrades**
```bash
# Upgrade to specific version
./livereview-ops upgrade v1.2.3
# 📦 Downloads/pulls new version if needed
# 💾 Creates automatic backup before upgrade
# 🔍 Compares database schema versions
# 🔄 Runs only necessary migrations
# 🚀 Performs rolling upgrade (minimal downtime)
# ✅ Validates upgrade success
```

#### 4. **Status & Monitoring**
```bash
# Show current status
./livereview-ops status
# 📊 Current version: v1.2.2
# 📊 Available versions: v1.2.3, v1.2.4
# 📊 Database status: healthy, 1.2GB
# 📊 Last backup: 2025-08-01 10:30:00
# 📊 Uptime: 5 days, 2 hours
```

### Data Management Strategy

#### **Storage Locations**
```
/var/livereview/
├── data/
│   ├── postgres/          # Database files (Docker volume)
│   ├── backups/           # Automated backups
│   │   ├── 2025-08-01-v1.2.2.sql.gz
│   │   └── 2025-08-02-v1.2.3.sql.gz
│   └── config/
│       └── livereview.toml  # Persistent config
├── logs/
├── docker-compose.yml
└── .env
```

#### **Backup Strategy**
```bash
# Automatic backups before upgrades
./livereview-ops backup
# 💾 Creates timestamped backup: YYYY-MM-DD-vX.X.X.sql.gz
# 💾 Includes database dump + config files
# 💾 Keeps last 10 backups, auto-cleanup old ones
# 💾 Stores backup metadata (version, size, timestamp)

# Restore from backup
./livereview-ops restore 2025-08-01-v1.2.2
# 🔄 Stops current services
# 🔄 Restores database from backup
# 🔄 Restores config files
# 🔄 Starts services with restored version
```

### Migration Intelligence

#### **Version-Based Migration Detection**
```sql
-- Add to schema.sql
CREATE TABLE livereview_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Track installed version
INSERT INTO livereview_metadata (key, value) VALUES ('version', '1.2.2');
INSERT INTO livereview_metadata (key, value) VALUES ('last_migration', '20250801150601');
```

#### **Smart Migration Logic**
```bash
# In livereview-ops upgrade
current_version=$(docker exec livereview-db psql -U livereview -d livereview -tAc "SELECT value FROM livereview_metadata WHERE key='version'")
target_version="v1.2.3"

if [ "$current_version" != "$target_version" ]; then
    echo "🔄 Migration needed: $current_version → $target_version"
    # Run only new migrations since last version
    dbmate up
    # Update version in database
    docker exec livereview-db psql -U livereview -d livereview -c "UPDATE livereview_metadata SET value='$target_version' WHERE key='version'"
else
    echo "✅ No migration needed"
fi
```

### File Structure for Operations
```
.
├── livereview-ops          # Master operations script
├── docker-compose.yml      # Main compose file
├── Dockerfile
├── .env.example
├── scripts/
│   ├── build.sh           # Build logic
│   ├── test.sh            # Testing logic  
│   ├── backup.sh          # Backup/restore logic
│   ├── upgrade.sh         # Upgrade logic
│   └── health.sh          # Health check logic
└── ops/
    ├── versions.json      # Available versions registry
    └── templates/         # Config templates
```

### Example Operations Workflow

#### **Development Cycle**
```bash
# 1. Build new version
./livereview-ops build
# Builds Docker image, runs tests

# 2. Test the build  
./livereview-ops test
# Starts test environment, validates functionality

# 3. Create release
./livereview-ops release v1.2.4
# Tags and registers new version

# 4. Upgrade production
./livereview-ops upgrade v1.2.4
# Backup → Migrate → Deploy → Validate
```

#### **Monitoring & Maintenance**
```bash
# Check system status
./livereview-ops status

# Manual backup
./livereview-ops backup

# View logs
./livereview-ops logs

# Health check
./livereview-ops health
```

### Zero-Downtime Upgrade Strategy

#### **Rolling Upgrade Process**
1. **Pre-upgrade validation**
   - Check disk space
   - Verify new version availability
   - Test database connectivity

2. **Backup phase**
   - Create full backup
   - Verify backup integrity

3. **Migration phase** (if needed)
   - Run database migrations
   - Validate schema changes

4. **Deployment phase**
   - Start new container alongside old one
   - Health check new container
   - Switch traffic to new container
   - Stop old container

5. **Post-upgrade validation**
   - Verify all services healthy
   - Check application functionality
   - Update version tracking

This approach makes operations **dead simple**:
- Single script for everything
- Automatic backups before changes
- Smart migration detection
- Clear status reporting
- Easy rollback capability

Would you like me to implement this `livereview-ops` script approach?
