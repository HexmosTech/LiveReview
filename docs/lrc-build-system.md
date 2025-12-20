# lrc CLI Build & Release System

This document describes the build and release process for the `lrc` command-line tool.

## Overview

The lrc build system provides:
- **Commit-based versioning**: Version is based on Git commit SHA (7 chars)
- **Cross-platform builds**: Compiles for Linux, macOS, Windows (amd64/arm64)
- **Version injection**: Build info embedded via ldflags at compile time
- **B2 distribution**: Automated upload to Backblaze B2 storage
- **Clean state enforcement**: Prevents builds with uncommitted changes

## Prerequisites

- Go 1.19+ 
- Python 3.7+ with `requests` library
- Clean working directory in `cmd/lrc/` (no uncommitted changes)
- For releases: B2 credentials (see Environment Variables section)

Install Python dependencies:
```bash
pip install requests
```

## Makefile Targets

### `make lrc-build`

Builds lrc for all platforms with version information injected.

**Output:**
- `dist/lrc/lrc-<commit>-linux-amd64`
- `dist/lrc/lrc-<commit>-linux-arm64`
- `dist/lrc/lrc-<commit>-darwin-amd64` (Intel Mac)
- `dist/lrc/lrc-<commit>-darwin-arm64` (Apple Silicon)
- `dist/lrc/lrc-<commit>-windows-amd64.exe`
- `dist/lrc/SHA256SUMS` (checksums file)

**Example:**
```bash
$ make lrc-build
🔨 Building lrc CLI for all platforms...
Building version aec6d7a
Starting cross-platform build...
Building linux/amd64...
  ✓ Built lrc-aec6d7a-linux-amd64 (SHA256: 73a4d6ce...)
Building linux/arm64...
  ✓ Built lrc-aec6d7a-linux-arm64 (SHA256: d75906c6...)
...
✓ Build complete! 6 files in /home/shrsv/bin/LiveReview/dist/lrc
```

### `make lrc-bump`

Creates a new version by committing changes in `cmd/lrc/`.

**Workflow:**
1. Make changes to files in `cmd/lrc/`
2. Run `make lrc-bump`
3. Enter commit message when prompted
4. Changes are committed and new version is created

**Example:**
```bash
# Make some changes
$ vim cmd/lrc/main.go

# Stage and commit
$ make lrc-bump
📝 Creating new lrc version...
Changes in cmd/lrc/:
 cmd/lrc/main.go | 5 +++--
 1 file changed, 3 insertions(+), 2 deletions(-)

Commit message: fix: improve error handling
✅ New version created: b3c4d5e
Run 'make lrc-build' to build this version
```

### `make lrc-release`

Builds and uploads lrc binaries to Backblaze B2.

**Requirements:**
- `B2_APP_KEY` environment variable must be set
- `B2_BUCKET_ID` environment variable must be set
- Clean working directory (no uncommitted changes in `cmd/lrc/`)

**Upload path:** `lrc/<commit-id>/lrc-<commit>-<os>-<arch>[.exe]`

**Example:**
```bash
$ export B2_APP_KEY="your_application_key_here"
$ export B2_BUCKET_ID="your_bucket_id_here"
$ make lrc-release
🚀 Building and releasing lrc...
Building version aec6d7a
Starting cross-platform build...
...
✓ Build complete! 6 files in /home/shrsv/bin/LiveReview/dist/lrc
Starting B2 upload...
✓ Authorized with B2
  ✓ Uploaded lrc-aec6d7a-linux-amd64
    URL: https://...backblazeb2.com/file/.../lrc/aec6d7a/lrc-aec6d7a-linux-amd64
...
✓ Upload complete! All files available at:
  https://your-bucket.s3.us-west-004.backblazeb2.com/lrc/aec6d7a/
```

### `make lrc-clean`

Removes build artifacts from `dist/lrc/`.

```bash
$ make lrc-clean
🧹 Cleaning lrc build artifacts...
✅ Clean complete
```

## Environment Variables

### For Releases (B2 Upload)

**B2_APP_KEY** (required for `lrc-release`)
- Your Backblaze B2 application key
- Keep this secret! Do not commit to version control
- Set with: `export B2_APP_KEY="your_key_here"`

**B2_BUCKET_ID** (required for `lrc-release`)
- The bucket ID where binaries will be uploaded
- Find in B2 dashboard: Buckets → Your Bucket → Settings
- Set with: `export B2_BUCKET_ID="your_bucket_id_here"`

**Example .env setup:**
```bash
# Add to ~/.bashrc or ~/.zshrc
export B2_APP_KEY="K005..."
export B2_BUCKET_ID="abc123..."
```

## Version Information

Version information is embedded at build time using Go ldflags:

```go
var (
    version   = "aec6d7a"            // Git commit SHA (short)
    buildTime = "2025-12-20T14:11:24Z" // UTC timestamp
    gitCommit = "aec6d7a"            // Full commit SHA
)
```

View version info:
```bash
$ ./lrc-aec6d7a-linux-amd64 version
lrc version aec6d7a
  Build time: 2025-12-20T14:11:24Z
  Git commit: aec6d7a

# Or use --version flag
$ ./lrc-aec6d7a-linux-amd64 --version
lrc version aec6d7a
```

## Versioning Strategy

Unlike LiveReview's tag-based versioning, lrc uses **commit-based versioning**:

1. **No uncommitted changes allowed**: Build will fail if `cmd/lrc/` has uncommitted changes
2. **Commit = Version**: The Git commit SHA becomes the version identifier
3. **Simple workflow**: Just commit changes, then build
4. **No manual version bumping**: Version is automatically derived from Git

**Why commit-based?**
- Simpler than tag management
- Every commit is a potential release
- Version directly traceable to source code
- No version conflicts or coordination needed

## Build Artifacts

### Binary Naming

Binaries follow the pattern: `lrc-<version>-<os>-<arch>[.exe]`

Examples:
- `lrc-aec6d7a-linux-amd64`
- `lrc-aec6d7a-darwin-arm64` 
- `lrc-aec6d7a-windows-amd64.exe`

### SHA256SUMS File

Contains checksums for all binaries:
```
73a4d6ce1081962d...  lrc-aec6d7a-linux-amd64
d75906c60226094d...  lrc-aec6d7a-linux-arm64
839e5eda36781c33...  lrc-aec6d7a-darwin-amd64
5d90900101b15cdc...  lrc-aec6d7a-darwin-arm64
88f9aac4bb78dae1...  lrc-aec6d7a-windows-amd64.exe
```

Verify downloaded binary:
```bash
sha256sum -c SHA256SUMS --ignore-missing
```

## Troubleshooting

### "cmd/lrc/ has uncommitted changes"

**Problem:** Build fails because there are uncommitted changes.

**Solution:** Commit or stash your changes:
```bash
# Option 1: Commit changes
git add cmd/lrc/
git commit -m "your message"

# Option 2: Stash changes temporarily
git stash push cmd/lrc/
make lrc-build
git stash pop
```

### "B2_APP_KEY not set"

**Problem:** Trying to run `lrc-release` without B2 credentials.

**Solution:** Export required environment variables:
```bash
export B2_APP_KEY="your_key"
export B2_BUCKET_ID="your_bucket_id"
```

### Python requests library not found

**Problem:** `ModuleNotFoundError: No module named 'requests'`

**Solution:** Install requests:
```bash
pip install requests
# or if using venv
source scripts/.venv/bin/activate
pip install requests
```

### Build fails on Windows cross-compilation

**Problem:** Windows binary build errors.

**Solution:** Ensure Go 1.19+ with proper Windows support:
```bash
go version  # Should be 1.19 or higher
go env GOOS GOARCH  # Verify cross-compilation support
```

## Implementation Details

### scripts/lrc_build.py

Python automation script providing:
- `check_lrc_clean()`: Verifies no uncommitted changes
- `get_commit_id()`: Extracts Git commit SHA
- `build_for_platform()`: Cross-compiles with version injection
- `upload_to_b2()`: Native B2 REST API upload (no boto3)

### B2 Upload Process

1. **Authorize**: POST to `b2_authorize_account` with keyID/appKey
2. **Get upload URL**: POST to `b2_get_upload_url` for bucket
3. **Upload file**: PUT binary with SHA1 header and metadata
4. **Repeat**: Fresh upload URL for each file (B2 best practice)

No AWS SDK or boto3 required - uses plain HTTP with `requests` library.

## Future Enhancements

Potential additions (not yet implemented):

- [ ] Homebrew tap for macOS distribution
- [ ] apt/deb packages for Ubuntu/Debian
- [ ] Chocolatey package for Windows
- [ ] GitHub Releases integration
- [ ] Auto-generated release notes from commits
- [ ] Binary signing/notarization
- [ ] Install scripts (install.sh, install.ps1)

## See Also

- [LiveReview Build System](../Makefile) - Main project build targets
- [scripts/lrops.py](../scripts/lrops.py) - LiveReview build automation
- [cmd/lrc/](../cmd/lrc/) - lrc source code
