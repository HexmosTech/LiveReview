# LiveReview Operations (lrops) Multi-Architecture Build Setup Guide

This guide provides the complete setup and troubleshooting steps for configuring multi-architecture Docker builds for LiveReview. This documentation covers the full evolution from initial setup to the final working solution.

## Overview

The LiveReview build system uses Docker buildx for cross-compilation based multi-architecture builds. The approach:
- **Cross-compiles Go binaries** on the native platform (no QEMU emulation for speed)
- **Builds UI once** and reuses across all architectures  
- **Uses standard buildx patterns** for automatic manifest list creation
- **Includes retry logic** for network resilience during registry pushes

## Architecture

- **UI Build**: Node.js build happens once on `$BUILDPLATFORM` (native platform)
- **Go Cross-Compilation**: All Go binaries (main app, river, riverui, dbmate) cross-compiled for target architectures
- **Runtime Assembly**: Final Alpine-based images assembled per-architecture with cross-compiled binaries
- **Manifest Creation**: Buildx automatically creates and pushes manifest lists for multi-arch images

## Prerequisites

### Remote Build Server Setup (SSH alias: `gitlab`)

The remote server requires binfmt support and a properly configured buildx builder:

```bash
# Install binfmt support for multi-platform emulation (required even for cross-compilation)
docker run --privileged --rm tonistiigi/binfmt --install all

# Create buildx builder with docker-container driver (required for multi-platform builds)
docker buildx create --name gitlab-multiarch --driver docker-container --use

# Bootstrap and verify multi-platform support
docker buildx inspect --bootstrap
# Output should show linux/arm64, linux/amd64 among supported platforms
```

### Local Development Machine Setup

```bash
# Remove any conflicting builders
docker buildx rm gitlab 2>/dev/null || true

# Create local builder matching remote name (optional, for consistency)
docker buildx create --name gitlab-multiarch --driver docker-container --use

# Verify builder works
docker buildx inspect --bootstrap
```

### Docker Context Setup

Ensure you have a working Docker context named `gitlab` that connects to your remote build server:

```bash
# Verify context exists and works
docker context ls
docker --context gitlab info

# If needed, create context (replace with your actual server details)
docker context create gitlab --docker "host=ssh://gitlab"
```

## Build Commands

### Multi-Architecture Build and Push
```bash
make docker-multiarch-push
```

### Multi-Architecture Build (No Push)
```bash
make docker-multiarch
```

### Include ARM v7 Support
```bash
make docker-multiarch-push ARGS="--architectures amd64 arm64 arm/v7"
```

## Key Technical Decisions & Lessons Learned

### 1. Cross-Compilation vs QEMU Emulation

**Problem**: Initial approach used QEMU emulation which was extremely slow for ARM builds.

**Solution**: Implemented cross-compilation approach:
- UI builds once on native platform with `--platform=$BUILDPLATFORM`
- Go binaries cross-compiled using `GOOS`, `GOARCH`, `GOARM` environment variables
- Tools (river, riverui) built using `go build` with cross-compilation rather than `go install`

**Critical**: Added `--platform=$BUILDPLATFORM` to both UI builder and Go builder stages in Dockerfile.

### 2. Standard Buildx vs Manual Manifest Creation

**Problem**: Initially created per-architecture tags (`-amd64`, `-arm64`) and manually created manifests, which caused GitLab registry issues ("invalid tag: missing manifest digest").

**Solution**: Adopted standard buildx pattern:
```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --tag registry.example.com/image:tag \
  --push
```

**Key insight**: When using `--platform` with multiple architectures and `--push`, buildx automatically:
- Builds for all specified platforms
- Creates manifest lists
- Pushes everything with the same tag

### 3. Builder Configuration Issues

**Problem**: Multiple builder configuration issues:
- Default Docker driver doesn't support multi-platform builds
- Builders referencing incorrect endpoints (`docker.example.com`)
- Context/builder name mismatches

**Solution**:
- Use `docker-container` driver for buildx builders
- Explicitly specify builder with `--builder gitlab-multiarch`
- Ensure builder is created on correct Docker context

### 4. Network Resilience

**Problem**: Large multi-arch images often failed during push due to network timeouts.

**Solution**: Added retry logic with exponential backoff:
- 3 retry attempts for push operations
- Exponential backoff: 2s, 4s, 8s delays
- Clear manual command instructions when retries fail
- Applied to both main build and manifest push operations

### 5. GitLab Registry Compatibility

**Problem**: GitLab registry complained about manifest digest issues with manual manifest creation.

**Solution**: Let buildx handle manifest creation automatically by using standard multi-platform build pattern.

## Dockerfile Structure

The `Dockerfile.crosscompile` uses a 3-stage approach:

```dockerfile
# Stage 1: UI Builder (pinned to build platform)
FROM --platform=$BUILDPLATFORM node:18-alpine AS ui-builder
# ... builds UI once, outputs to ui/dist

# Stage 2: Go Cross-Compiler (pinned to build platform)  
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
# ... cross-compiles all Go binaries for TARGETOS/TARGETARCH

# Stage 3: Runtime Assembly (per target platform)
FROM alpine:3.18
# ... copies architecture-specific binaries from builder
```

## Build Script Architecture

The `lrops.py` script orchestrates the build process:

### Standard Buildx Approach
```python
cmd = [
    'docker', '--context', 'gitlab', 'buildx', 'build',
    '--builder', 'gitlab-multiarch',
    '--platform', platforms,  # e.g., "linux/amd64,linux/arm64"
    '--tag', version_tag,
    '--push',
    '.'
]
```

### Retry Logic
```python
def _run_command_with_retries(self, cmd, max_retries=3):
    for attempt in range(max_retries + 1):
        try:
            return self._run_command(cmd)
        except GitError as e:
            if attempt == max_retries:
                # Print manual command and troubleshooting tips
                raise e
            else:
                time.sleep(2 ** attempt)  # Exponential backoff
```

## Troubleshooting

### Build Failures

1. **"Multi-platform build is not supported for the docker driver"**
   - Solution: Use `--driver docker-container` when creating buildx builder

2. **"no active session" or timeout errors**
   - Solution: Retry logic will handle this, or run manually with provided command

3. **"invalid tag: missing manifest digest" (GitLab)**
   - Solution: Use standard buildx pattern, don't create manual per-arch tags

4. **QEMU processes during build**
   - Solution: Ensure UI and Go builder stages use `--platform=$BUILDPLATFORM`

5. **"go: cannot install cross-compiled binaries when GOBIN is set"**
   - Solution: Use `go build` instead of `go install` for cross-compilation

### Network Issues

If builds fail due to network timeouts:
1. The retry logic will automatically attempt 3 times
2. Manual command will be printed for manual execution
3. Consider using faster network or increasing Docker timeout settings

### Registry Issues

For GitLab registry compatibility:
- Don't create separate architecture tags (`-amd64`, `-arm64`)
- Let buildx create manifest lists automatically
- Use single tag with multiple platforms

## Performance Notes

- **Cross-compilation build time**: ~3-5 minutes for both amd64 and arm64
- **Previous QEMU approach**: ~15-20 minutes per architecture
- **UI build time**: ~2 minutes (shared across all architectures)
- **Network upload time**: Varies by connection (largest bottleneck)

## Commands Reference

### Verify Builder Status
```bash
docker --context gitlab buildx ls
docker --context gitlab buildx inspect gitlab-multiarch
```

### Manual Build Commands
```bash
# Single architecture
docker --context gitlab buildx build \
  --builder gitlab-multiarch \
  --platform linux/amd64 \
  --tag your-registry/livereview:test-amd64 \
  -f Dockerfile.crosscompile \
  --push .

# Multi-architecture
docker --context gitlab buildx build \
  --builder gitlab-multiarch \
  --platform linux/amd64,linux/arm64 \
  --tag your-registry/livereview:test \
  -f Dockerfile.crosscompile \
  --push .
```

### Debug Manifest Issues
```bash
# Inspect manifest
docker --context gitlab manifest inspect your-registry/livereview:tag

# Remove problematic manifest
docker --context gitlab manifest rm your-registry/livereview:tag
```

This approach provides a robust, fast, and reliable multi-architecture build system that leverages modern Docker buildx capabilities while maintaining compatibility with GitLab registry requirements.
