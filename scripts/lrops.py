#!/usr/bin/env python3
"""
LiveReview Operations Script (lrops.py)
Handles versioning, building, and release operations for LiveReview.

This script provides automated version management using semantic versioning
with Git tags, build-time version injection, and Docker integration.
"""

import argparse
import json
import os
import re
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path


class GitError(Exception):
    """Git operation error"""
    pass


class VersionError(Exception):
    """Version format or operation error"""
    pass


class LiveReviewOps:
    def __init__(self, repo_root=None, verbose=False):
        self.verbose = verbose
        self.repo_root = Path(repo_root) if repo_root else self._find_repo_root()
        self._validate_git_repo()
        
    def _find_repo_root(self):
        """Find the Git repository root from current directory"""
        current = Path.cwd()
        while current != current.parent:
            if (current / '.git').exists():
                return current
            current = current.parent
        raise GitError("Not in a Git repository")
    
    def _validate_git_repo(self):
        """Validate we're in a proper Git repository"""
        if not (self.repo_root / '.git').exists():
            raise GitError(f"No Git repository found at {self.repo_root}")
    
    def _run_command(self, cmd, cwd=None, capture_output=True, check=True):
        """Run a shell command and return result"""
        if isinstance(cmd, str):
            cmd = cmd.split()
        
        # Always display the command prominently
        cmd_str = ' '.join(cmd)
        print(f"\n{'='*80}")
        print(f"🔧 EXECUTING COMMAND:")
        print(f"📁 Directory: {cwd or self.repo_root}")
        print(f"💻 Command: {cmd_str}")
        print(f"{'='*80}")
        
        if self.verbose:
            print(f"Running: {cmd_str}")
        
        try:
            result = subprocess.run(
                cmd,
                cwd=cwd or self.repo_root,
                capture_output=capture_output,
                text=True,
                check=check
            )
            
            if self.verbose and result.stdout:
                print(f"Output: {result.stdout.strip()}")
            
            return result
        except subprocess.CalledProcessError as e:
            print(f"❌ COMMAND FAILED: {cmd_str}")
            if self.verbose:
                print(f"Command failed: {e}")
                if e.stdout:
                    print(f"Stdout: {e.stdout}")
                if e.stderr:
                    print(f"Stderr: {e.stderr}")
            raise GitError(f"Command failed: {' '.join(cmd)}: {e.stderr}")
    
    def _display_build_plan(self, build_type, version, docker_version, architectures=None, multiarch=False, push=False, make_latest=False, registry=None, image_name=None, git_commit=None, build_time=None):
        """Display comprehensive build plan before execution"""
        print(f"\n{'🚀 BUILD EXECUTION PLAN':=^100}")
        print(f"\n📋 BUILD CONFIGURATION:")
        print(f"   • Build Type: {build_type}")
        print(f"   • Version: {version}")
        if docker_version:
            print(f"   • Docker Version Tag: {docker_version}")
        print(f"   • Multi-arch: {multiarch}")
        if multiarch and architectures:
            print(f"   • Architectures: {', '.join(architectures)}")
        print(f"   • Push to Registry: {push}")
        print(f"   • Tag as Latest: {make_latest}")
        if registry and image_name:
            print(f"   • Registry: {registry}")
            print(f"   • Image Name: {image_name}")
            print(f"   • Full Image: {registry}/{image_name}")
        
        print(f"\n📝 EXECUTION PHASES:")
        
        if build_type == "docker":
            print(f"   1️⃣  UI BUILD PHASE: inside Docker (shared across architectures)")
            print(f"       🧰 Handled by Dockerfile (ui-builder stage) — no local npm runs")
            
            if multiarch:
                print(f"   2️⃣  CROSS-COMPILATION MULTI-ARCH DOCKER BUILD PHASE:")
                print(f"       🔧 docker buildx use multiplatform-builder")
                print(f"       🚀 Using cross-compilation for faster ARM builds!")
                print(f"       📦 Reusing common UI assets from ui/dist for all platforms")
                
                full_image = f"{registry}/{image_name}"
                arch_tags = []
                for i, arch in enumerate(architectures or ['amd64', 'arm64'], 1):
                    suffix = arch.replace('/', '')  # arm/v7 -> armv7
                    arch_tag = f"{full_image}:{docker_version}-{suffix}"
                    arch_tags.append(arch_tag)
                    print(f"       🐳 Cross-compile {arch} image:")
                    # Map architecture for build args
                    target_arch = 'arm' if arch == 'arm/v7' else arch
                    goarm = '7' if arch == 'arm/v7' else None
                    build_cmd = [
                        'docker', '--context', 'gitlab', 'buildx', 'build',
                        '--platform', f'linux/{arch}',
                        '--build-arg', f'VERSION={version}',
                        '--build-arg', f'BUILD_TIME={build_time or "$(date -u +%Y-%m-%dT%H:%M:%SZ)"}',
                        '--build-arg', f'GIT_COMMIT={git_commit or "$(git rev-parse --short HEAD)"}',
                        '-f', 'Dockerfile.crosscompile',
                        '-t', arch_tag,
                        '--push' if push else '--load',
                        '.'
                    ]
                    print(f"          {' '.join(build_cmd)}")
                
                if push:
                    print(f"   3️⃣  MANIFEST LIST CREATION PHASE:")
                    version_tag = f"{full_image}:{docker_version}"
                    print(f"       📋 Create version manifest:")
                    manifest_cmd = ['docker', '--context', 'gitlab', 'manifest', 'create', version_tag] + arch_tags
                    print(f"          {' '.join(manifest_cmd)}")
                    
                    for arch in architectures or ['amd64', 'arm64']:
                        suffix = arch.replace('/', '')
                        arch_tag = f"{full_image}:{docker_version}-{suffix}"
                        if arch == 'arm/v7':
                            annotate_cmd = [
                                'docker', '--context', 'gitlab', 'manifest', 'annotate',
                                version_tag, arch_tag,
                                '--arch', 'arm', '--variant', 'v7'
                            ]
                        else:
                            annotate_cmd = [
                                'docker', '--context', 'gitlab', 'manifest', 'annotate',
                                version_tag, arch_tag,
                                '--arch', arch
                            ]
                        print(f"          {' '.join(annotate_cmd)}")
                    
                    push_cmd = ['docker', '--context', 'gitlab', 'manifest', 'push', version_tag]
                    print(f"          {' '.join(push_cmd)}")
                    
                    if make_latest:
                        latest_tag = f"{full_image}:latest"
                        print(f"       📋 Create latest manifest:")
                        latest_manifest_cmd = ['docker', '--context', 'gitlab', 'manifest', 'create', latest_tag] + arch_tags
                        print(f"          {' '.join(latest_manifest_cmd)}")
                        
                        for arch in architectures or ['amd64', 'arm64']:
                            suffix = arch.replace('/', '')
                            arch_tag = f"{full_image}:{docker_version}-{suffix}"
                            if arch == 'arm/v7':
                                annotate_cmd = [
                                    'docker', '--context', 'gitlab', 'manifest', 'annotate',
                                    latest_tag, arch_tag,
                                    '--arch', 'arm', '--variant', 'v7'
                                ]
                            else:
                                annotate_cmd = [
                                    'docker', '--context', 'gitlab', 'manifest', 'annotate',
                                    latest_tag, arch_tag,
                                    '--arch', arch
                                ]
                            print(f"          {' '.join(annotate_cmd)}")
                        
                        latest_push_cmd = ['docker', '--context', 'gitlab', 'manifest', 'push', latest_tag]
                        print(f"          {' '.join(latest_push_cmd)}")
            else:
                print(f"   2️⃣  SINGLE-ARCH DOCKER BUILD PHASE (cross-compile, reuse UI dist):")
                version_tag = f"{registry}/{image_name}:{docker_version}"
                build_cmd = [
                    'docker', '--context', 'gitlab', 'buildx', 'build',
                    '--platform', 'linux/amd64',
                    '--build-arg', f'VERSION={version}',
                    '--build-arg', f'BUILD_TIME={build_time or "$(date -u +%Y-%m-%dT%H:%M:%SZ)"}',
                    '--build-arg', f'GIT_COMMIT={git_commit or "$(git rev-parse --short HEAD)"}',
                    '--build-arg', 'UI_PLATFORM=linux/amd64',
                    '--build-arg', 'GO_BUILDER_PLATFORM=linux/amd64',
                    '--build-arg', 'TARGETPLATFORM=linux/amd64',
                    '--build-arg', 'TARGETARCH=amd64',
                    '--build-arg', 'TARGETOS=linux',
                    '-f', 'Dockerfile.crosscompile',
                    '-t', version_tag,
                    '--load',
                    '.'
                ]
                print(f"       🐳 {' '.join(build_cmd)}")
                
                if make_latest:
                    latest_tag = f"{registry}/{image_name}:latest"
                    tag_cmd = ['docker', 'tag', version_tag, latest_tag]
                    print(f"       🏷️  {' '.join(tag_cmd)}")
                
                if push:
                    print(f"   3️⃣  DOCKER PUSH PHASE:")
                    push_cmd = ['docker', '--context', 'gitlab', 'push', version_tag]
                    print(f"       📤 {' '.join(push_cmd)}")
                    
                    if make_latest:
                        latest_tag = f"{registry}/{image_name}:latest"
                        latest_push_cmd = ['docker', '--context', 'gitlab', 'push', latest_tag]
                        print(f"       📤 {' '.join(latest_push_cmd)}")
        
        elif build_type == "binary":
            print(f"   1️⃣  GO BINARY BUILD PHASE:")
            ldflags = [
                f"-X main.version={version}",
                f"-X main.buildTime={build_time or '$(date -u --iso-8601=seconds)'}",
                f"-X main.gitCommit={git_commit or '$(git rev-parse --short HEAD)'}"
            ]
            build_cmd = [
                'go', 'build',
                f'-ldflags={" ".join(ldflags)}',
                '-o', 'livereview',
                '.'
            ]
            print(f"       🔨 {' '.join(build_cmd)}")
            print(f"       ✅ ./livereview --version")
        
        print(f"\n⚠️  MANUAL TESTING COMMANDS:")
        print(f"   You can run these commands manually to debug issues:")
        print(f"   📁 cd {self.repo_root}")
        
        if build_type == "docker":
            print(f"   # UI Build: occurs inside Docker via ui-builder stage")
            
            if multiarch:
                print(f"   # Cross-compilation Multi-arch Docker Build (UI built once and reused):")
                print(f"   docker buildx use multiplatform-builder")
                for arch in architectures or ['amd64', 'arm64']:
                    suffix = arch.replace('/', '')
                    arch_tag = f"{registry}/{image_name}:{docker_version}-{suffix}"
                    build_cmd = [
                        'docker', '--context', 'gitlab', 'buildx', 'build',
                        '--platform', f'linux/{arch}',
                        '--build-arg', f'VERSION={version}',
                        '--build-arg', f'BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)',
                        '--build-arg', f'GIT_COMMIT=$(git rev-parse --short HEAD)',
                        '--build-arg', 'UI_PLATFORM=linux/amd64',
                        '--build-arg', 'GO_BUILDER_PLATFORM=linux/amd64',
                        '--build-arg', f'TARGETPLATFORM=linux/{arch}',
                        '--build-arg', f'TARGETARCH={arch}',
                        '--build-arg', f'TARGETOS=linux',
                        '-f', 'Dockerfile.crosscompile',
                        '-t', arch_tag,
                        '--push' if push else '--load',
                        '.'
                    ]
                    print(f"   {' '.join(build_cmd)}")
            else:
                print(f"   # Single-arch Docker Build (cross-compile, reuse UI dist):")
                version_tag = f"{registry}/{image_name}:{docker_version}"
                build_cmd = [
                    'docker', '--context', 'gitlab', 'buildx', 'build',
                    '--platform', 'linux/amd64',
                    '--build-arg', f'VERSION={version}',
                    '--build-arg', f'BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)',
                    '--build-arg', f'GIT_COMMIT=$(git rev-parse --short HEAD)',
                    '--build-arg', 'UI_PLATFORM=linux/amd64',
                    '--build-arg', 'GO_BUILDER_PLATFORM=linux/amd64',
                    '--build-arg', 'TARGETPLATFORM=linux/amd64',
                    '--build-arg', 'TARGETARCH=amd64',
                    '--build-arg', 'TARGETOS=linux',
                    '-f', 'Dockerfile.crosscompile',
                    '-t', version_tag,
                    '--load',
                    '.'
                ]
                print(f"   {' '.join(build_cmd)}")
        
        elif build_type == "binary":
            ldflags = [
                f"-X main.version={version}",
                f"-X main.buildTime=$(date -u --iso-8601=seconds)",
                f"-X main.gitCommit=$(git rev-parse --short HEAD)"
            ]
            build_cmd = [
                'go', 'build',
                f'-ldflags={" ".join(ldflags)}',
                '-o', 'livereview',
                '.'
            ]
            print(f"   {' '.join(build_cmd)}")
        
        print(f"{'='*100}\n")
        
        # Ask for confirmation
        try:
            confirm = input("❓ Do you want to proceed with this build plan? (y/n): ").strip().lower()
            if confirm not in ['y', 'yes']:
                print("❌ Build cancelled by user.")
                return False
        except KeyboardInterrupt:
            print("\n❌ Build cancelled by user.")
            return False
        
        return True

    def _parse_version(self, version_str):
        """Parse a semantic version string into components.
        Returns dict: {major, minor, patch, prerelease, build, full}
        Accepts optional leading 'v'.
        """
        # Remove 'v' prefix if present
        version_str = version_str.lstrip('v')

        pattern = r'^(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9\-\.]+))?(?:\+([a-zA-Z0-9\-\.]+))?$'
        match = re.match(pattern, version_str)
        if not match:
            raise VersionError(f"Invalid semantic version: {version_str}")

        major, minor, patch = map(int, match.groups()[:3])
        prerelease = match.group(4)
        build = match.group(5)

        return {
            'major': major,
            'minor': minor,
            'patch': patch,
            'prerelease': prerelease,
            'build': build,
            'full': version_str,
        }
    
    def _format_version(self, major, minor, patch, prerelease=None, build=None):
        """Format version components into semantic version string"""
        version = f"{major}.{minor}.{patch}"
        if prerelease:
            version += f"-{prerelease}"
        if build:
            version += f"+{build}"
        return version
    
    def _increment_version(self, current_version, increment_type):
        """Increment version based on type (patch, minor, major)"""
        parsed = self._parse_version(current_version)
        
        if increment_type == 'patch':
            return self._format_version(parsed['major'], parsed['minor'], parsed['patch'] + 1)
        elif increment_type == 'minor':
            return self._format_version(parsed['major'], parsed['minor'] + 1, 0)
        elif increment_type == 'major':
            return self._format_version(parsed['major'] + 1, 0, 0)
        else:
            raise VersionError(f"Invalid increment type: {increment_type}")
    
    def get_current_version_info(self):
        """Get comprehensive version information from Git"""
        info = {
            'version': None,
            'commit': None,
            'dirty': False,
            'tag': None,
            'is_tagged': False,
            'distance': 0
        }
        
        try:
            # Get current commit hash
            result = self._run_command('git rev-parse --short=6 HEAD')
            info['commit'] = result.stdout.strip()
            
            # Check if working directory is dirty
            result = self._run_command('git status --porcelain')
            info['dirty'] = bool(result.stdout.strip())
            
            # Try to get exact tag for current commit
            try:
                result = self._run_command('git describe --exact-match --tags HEAD')
                tag = result.stdout.strip()
                info['tag'] = tag
                info['version'] = tag.lstrip('v')
                info['is_tagged'] = True
            except GitError:
                # Not on a tagged commit, try to get closest tag
                try:
                    result = self._run_command('git describe --tags --always --dirty=-modified')
                    desc = result.stdout.strip()
                    
                    # Parse git describe output
                    # Format: v1.2.3-5-gabc123f or abc123f (if no tags)
                    if '-' in desc and desc.startswith('v'):
                        parts = desc.split('-')
                        if len(parts) >= 3:
                            info['tag'] = parts[0]
                            info['distance'] = int(parts[1])
                            commit_part = parts[2].lstrip('g')
                            if desc.endswith('-modified'):
                                info['dirty'] = True
                                commit_part = commit_part.replace('-modified', '')
                            info['commit'] = commit_part
                            info['version'] = f"{parts[0].lstrip('v')}-dev+{info['commit']}"
                        else:
                            # Just a commit hash
                            info['version'] = desc
                    else:
                        # No tags in repository yet
                        info['version'] = f"0.0.0-dev+{info['commit']}"
                        if desc.endswith('-modified'):
                            info['dirty'] = True
                            info['version'] += '-modified'
                except GitError:
                    # Fallback to just commit hash
                    info['version'] = f"0.0.0-dev+{info['commit']}"
            
            # Append -modified suffix if dirty
            if info['dirty'] and not info['version'].endswith('-modified'):
                info['version'] += '-modified'
                
        except GitError as e:
            raise GitError(f"Failed to get version info: {e}")
        
        return info
    
    def get_latest_tag(self):
        """Get the latest semantic version tag"""
        try:
            result = self._run_command(['git', 'tag', '-l', 'v*', '--sort=-version:refname'])
            tags = [line.strip() for line in result.stdout.strip().split('\n') if line.strip()]
            
            # Find first valid semantic version
            for tag in tags:
                try:
                    self._parse_version(tag)
                    return tag
                except VersionError:
                    continue
            
            # No valid semantic version tags found
            return None
        except GitError:
            return None
    
    def get_recent_tags(self, limit=10):
        """Get recent semantic version tags, sorted latest to oldest"""
        try:
            result = self._run_command(['git', 'tag', '-l', 'v*', '--sort=-version:refname'])
            tags = [line.strip() for line in result.stdout.strip().split('\n') if line.strip()]
            
            # Filter for valid semantic versions and limit
            valid_tags = []
            for tag in tags:
                try:
                    self._parse_version(tag)
                    valid_tags.append(tag)
                    if len(valid_tags) >= limit:
                        break
                except VersionError:
                    continue
            
            return valid_tags
        except GitError:
            return []
    
    def create_tag(self, version, message=None, push=True):
        """Create an annotated Git tag"""
        if not version.startswith('v'):
            version = f'v{version}'
        
        # Validate version format
        self._parse_version(version)
        
        # Check if tag already exists
        try:
            self._run_command(f'git rev-parse --verify {version}')
            raise VersionError(f"Tag {version} already exists")
        except GitError:
            pass  # Tag doesn't exist, which is what we want
        
        # Create annotated tag
        if not message:
            message = f"Release {version}"
        
        self._run_command(['git', 'tag', '-a', version, '-m', message])
        
        if push:
            self._run_command(['git', 'push', 'origin', version])
        
        return version
    
    def check_working_directory_clean(self):
        """Check if working directory is clean (no uncommitted changes)"""
        result = self._run_command('git status --porcelain')
        return not bool(result.stdout.strip())
    
    def build_binary(self, version=None, output_path=None):
        """Build the Go binary with version information embedded"""
        if not version:
            version_info = self.get_current_version_info()
            version = version_info['version']
            git_commit = version_info['commit']
        else:
            git_commit = self.get_current_version_info()['commit']
        
        build_time = datetime.now(timezone.utc).isoformat()
        
        if not output_path:
            output_path = self.repo_root / 'livereview'
        
        # Display build plan
        if not self._display_build_plan(
            build_type="binary",
            version=version,
            docker_version=None,
            git_commit=git_commit,
            build_time=build_time
        ):
            return None  # User cancelled
        
        # Build command with version injection
        ldflags = [
            f"-X main.version={version}",
            f"-X main.buildTime={build_time}", 
            f"-X main.gitCommit={git_commit}"
        ]
        
        cmd = [
            'go', 'build',
            f'-ldflags={" ".join(ldflags)}',
            '-o', str(output_path),
            '.'
        ]
        
        print(f"Building binary with version {version}...")
        self._run_command(cmd, capture_output=False)
        print(f"Binary built: {output_path}")
        
        return output_path
    
    def cmd_version(self, args):
        """Handle 'version' command"""
        try:
            version_info = self.get_current_version_info()
            
            if args.json:
                print(json.dumps(version_info, indent=2))
            elif args.build_info:
                build_time = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
                print(f"v{version_info['version']} (commit {version_info['commit']}, built {build_time})")
            else:
                print(f"v{version_info['version']}")
                
        except (GitError, VersionError) as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1
        
        return 0
    
    def cmd_bump(self, args):
        """Handle 'bump' command"""
        try:
            # Check if working directory is clean for release tags
            if not args.allow_dirty and not self.check_working_directory_clean():
                print("Error: Working directory has uncommitted changes. Use --allow-dirty to override.", file=sys.stderr)
                return 2
            
            # Get current version
            latest_tag = self.get_latest_tag()
            if latest_tag:
                current_version = latest_tag.lstrip('v')
            else:
                current_version = '0.0.0'
                print("No existing tags found, starting from v0.0.0")
            
            # Determine increment type
            if args.ci:
                # Auto-detect from commit messages (simple implementation)
                increment_type = 'patch'  # Default for CI
            elif args.type:
                increment_type = args.type
            else:
                # Interactive mode
                print(f"Current version: {current_version}")
                
                # Calculate possible new versions
                patch_version = self._increment_version(current_version, 'patch')
                minor_version = self._increment_version(current_version, 'minor')
                major_version = self._increment_version(current_version, 'major')
                
                print("Select increment type:")
                print(f"[1] Patch ({patch_version})")
                print(f"[2] Minor ({minor_version})")
                print(f"[3] Major ({major_version})")
                
                while True:
                    try:
                        choice = input("Enter choice (1-3): ").strip()
                        if choice == '1':
                            increment_type = 'patch'
                            break
                        elif choice == '2':
                            increment_type = 'minor'
                            break
                        elif choice == '3':
                            increment_type = 'major'
                            break
                        else:
                            print("Invalid choice. Please enter 1, 2, or 3.")
                    except KeyboardInterrupt:
                        print("\nAborted.")
                        return 1
            
            # Calculate new version
            new_version = self._increment_version(current_version, increment_type)
            new_tag = f'v{new_version}'
            
            if args.dry_run:
                commit_info = self.get_current_version_info()
                print(f"Would create tag {new_tag} from current commit {commit_info['commit']}")
                return 0
            
            # Create the tag
            print(f"Creating tag {new_tag}...")
            tag = self.create_tag(new_version, push=not args.no_push)
            print(f"Successfully created and pushed tag {tag}")
            
        except (GitError, VersionError) as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1
        except KeyboardInterrupt:
            print("\nAborted.", file=sys.stderr)
            return 1
        
        return 0
    
    def build_docker_image(self, version=None, registry=None, image_name=None, push=False, dry_run=False, make_latest=None, architectures=None, multiarch=False):
        """Build Docker image with version information and multi-architecture support"""
        if not version:
            version_info = self.get_current_version_info()
            version = version_info['version']
            git_commit = version_info['commit']
        else:
            git_commit = self.get_current_version_info()['commit']
        
        build_time = datetime.now(timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
        
        # Default values
        if not registry:
            registry = "git.apps.hexmos.com:5050/hexmos"
        if not image_name:
            image_name = "livereview"
        
        # Default architectures for multi-arch builds
        if not architectures:
            architectures = ['amd64', 'arm64']
        
        # Clean version for Docker tag (remove any dev/modified suffixes)
        if version.startswith('v'):
            version = version[1:]
        
        # For development versions, use a simpler tag
        if '-dev+' in version or '-modified' in version:
            docker_version = f"dev-{git_commit}"
            # Development versions should not be tagged as latest by default
            if make_latest is None:
                make_latest = False
        else:
            docker_version = version
            # Release versions can be tagged as latest
            if make_latest is None:
                make_latest = True
        
        full_image = f"{registry}/{image_name}"
        version_tag = f"{full_image}:{docker_version}"
        latest_tag = f"{full_image}:latest"
        
        # Display build plan
        if not self._display_build_plan(
            build_type="docker",
            version=version,
            docker_version=docker_version,
            architectures=architectures if multiarch else None,
            multiarch=multiarch,
            push=push,
            make_latest=make_latest,
            registry=registry,
            image_name=image_name,
            git_commit=git_commit,
            build_time=build_time
        ):
            return None  # User cancelled
        
        if dry_run:
            print(f"Would build Docker image with:")
            print(f"  Version: {version}")
            print(f"  Git Commit: {git_commit}")
            print(f"  Build Time: {build_time}")
            print(f"  Multi-arch: {multiarch}")
            if multiarch:
                print(f"  Architectures: {', '.join(architectures)}")
                for arch in architectures:
                    suffix = arch.replace('/', '')
                    arch_tag = f"{full_image}:{docker_version}-{suffix}"
                    print(f"  Arch Tag ({arch}): {arch_tag}")
            print(f"  Main Tag: {version_tag}")
            if make_latest:
                print(f"  Latest Tag: {latest_tag}")
            if push:
                print(f"  Would push to registry")
            return version_tag

        print(f"Building Docker image with version {version}...")
        # UI is built inside Docker (ui-builder stage); no local npm needed

        if multiarch:
            return self._build_multiarch_image(
                version, docker_version, git_commit, build_time,
                full_image, version_tag, latest_tag, architectures,
                make_latest, push
            )
        else:
            return self._build_single_arch_image(
                version, docker_version, git_commit, build_time,
                version_tag, latest_tag, make_latest, push
            )
    
    def _build_single_arch_image(self, version, docker_version, git_commit, build_time,
                                version_tag, latest_tag, make_latest, push):
        """Build single architecture Docker image via cross-compilation Dockerfile"""
        # Use buildx with the cross-compilation Dockerfile for consistency and UI reuse
        cmd = [
            'docker', '--context', 'gitlab', 'buildx', 'build',
            '--platform', 'linux/amd64',
            '--build-arg', f'VERSION={version}',
            '--build-arg', f'BUILD_TIME={build_time}',
            '--build-arg', f'GIT_COMMIT={git_commit}',
            '-f', 'Dockerfile.crosscompile',
            '-t', version_tag,
            '--load',
            '.'
        ]

        # No direct multi-tagging in a single buildx + --load step; tag after load if needed
        self._run_command(cmd, capture_output=False)
        
        print(f"Successfully built Docker image: {version_tag}")
        if make_latest:
            # Tag locally after load
            self._run_command(['docker', 'tag', version_tag, latest_tag], capture_output=False)
            print(f"Also tagged as: {latest_tag}")
        
        if push:
            print(f"Pushing Docker image: {version_tag}")
            self._run_command(['docker', '--context', 'gitlab', 'push', version_tag], capture_output=False)

            if make_latest:
                print(f"Pushing Docker image: {latest_tag}")
                self._run_command(['docker', '--context', 'gitlab', 'push', latest_tag], capture_output=False)
        
        return version_tag
    
    def _build_multiarch_image(self, version, docker_version, git_commit, build_time,
                              full_image, version_tag, latest_tag, architectures,
                              make_latest, push):
        """Build multi-architecture Docker images using cross-compilation for faster builds"""
        print(f"Building multi-architecture Docker image for architectures: {', '.join(architectures)}")
        print("🚀 Using cross-compilation approach for faster ARM builds!")

        # Build multi-architecture image in a single command
        # Create platform list and tags
        platforms = ','.join([f'linux/{arch}' for arch in architectures])
        arch_tags = []
        tags_args = []
        
        for arch in architectures:
            suffix = arch.replace('/', '')  # arm/v7 -> armv7
            arch_tag = f"{full_image}:{docker_version}-{suffix}"
            arch_tags.append(arch_tag)
            tags_args.extend(['-t', arch_tag])
        
        # Also tag with the main version tag
        tags_args.extend(['-t', version_tag])
        
        print(f"Building multi-architecture image for platforms: {platforms}")
        
        cmd = [
            'docker', '--context', 'gitlab', 'buildx', 'build',
            '--builder', 'gitlab-multiarch',
            '--platform', platforms,
            '--build-arg', f'VERSION={version}',
            '--build-arg', f'BUILD_TIME={build_time}',
            '--build-arg', f'GIT_COMMIT={git_commit}',
            '-f', 'Dockerfile.crosscompile',
        ] + tags_args + [
            '--push' if push else '--load',
            '.'
        ]
        
        self._run_command(cmd, capture_output=False)
        print(f"✅ Successfully built multi-architecture image with platforms: {platforms}")
        
        # Step 3: Create and push manifest list for latest tag if requested
        if push and make_latest:
            print(f"Creating manifest list for: {latest_tag}")
            self._create_and_push_manifest(latest_tag, arch_tags)
        
        print(f"🎉 Successfully built multi-architecture image using cross-compilation: {version_tag}")
        if make_latest:
            print(f"Also tagged as latest: {latest_tag}")
        
        return version_tag
    
    def _create_and_push_manifest(self, manifest_tag, arch_tags):
        """Create and push a Docker manifest list"""
        try:
            # Create manifest list
            cmd = ['docker', '--context', 'gitlab', 'manifest', 'create', manifest_tag] + arch_tags
            self._run_command(cmd, capture_output=False)
            
            # Annotate each architecture in the manifest
            for tag in arch_tags:
                # Extract suffix and map to arch/variant
                suffix = tag.split(':')[-1].split('-')[-1]
                if suffix == 'armv7':
                    print(f"Annotating manifest for arm/v7: {tag}")
                    self._run_command([
                        'docker', '--context', 'gitlab', 'manifest', 'annotate',
                        manifest_tag, tag,
                        '--arch', 'arm', '--variant', 'v7'
                    ], capture_output=False)
                elif suffix in ['amd64', 'arm64']:
                    print(f"Annotating manifest for {suffix}: {tag}")
                    self._run_command([
                        'docker', '--context', 'gitlab', 'manifest', 'annotate',
                        manifest_tag, tag,
                        '--arch', suffix
                    ], capture_output=False)
            
            # Push manifest list
            print(f"Pushing manifest list: {manifest_tag}")
            self._run_command(['docker', '--context', 'gitlab', 'manifest', 'push', manifest_tag], capture_output=False)
            
        except GitError as e:
            print(f"Warning: Failed to create/push manifest list for {manifest_tag}: {e}")
            print("Individual architecture images were still pushed successfully.")

    def cmd_build(self, args):
        """Handle 'build' command"""
        try:
            if args.docker:
                # Build Docker image
                image_tag = self.build_docker_image(
                    version=args.version,
                    registry=args.registry,
                    image_name=args.image_name,
                    push=args.push,
                    dry_run=args.dry_run,
                    make_latest=args.latest if hasattr(args, 'latest') else None,
                    architectures=args.architectures if hasattr(args, 'architectures') else None,
                    multiarch=args.multiarch if hasattr(args, 'multiarch') else False
                )
                if not args.dry_run:
                    print(f"Docker build completed: {image_tag}")
            else:
                if args.dry_run:
                    version_info = self.get_current_version_info()
                    version = args.version or version_info['version']
                    print(f"Would build binary with version: {version}")
                    return 0
                
                # Build binary
                binary_path = self.build_binary(version=args.version)
                
                # Test the binary
                print("Testing binary...")
                result = self._run_command([str(binary_path), '--version'])
                print(f"Binary version output: {result.stdout.strip()}")
            
        except (GitError, VersionError) as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1
        
        return 0
    
    def cmd_docker(self, args):
        """Handle 'docker' command with interactive tag selection"""
        try:
            # Determine which tag to use
            if args.tag:
                selected_tag = args.tag
                if not selected_tag.startswith('v'):
                    selected_tag = f'v{selected_tag}'
                # Validate the tag exists
                recent_tags = self.get_recent_tags(50)  # Check more tags for validation
                if selected_tag not in recent_tags:
                    print(f"Error: Tag {selected_tag} not found in repository")
                    return 1
            else:
                # Interactive tag selection
                recent_tags = self.get_recent_tags(10)
                if not recent_tags:
                    print("No version tags found in repository")
                    return 1
                
                print("Available tags (latest to oldest):")
                for i, tag in enumerate(recent_tags, 1):
                    print(f"[{i}] {tag}")
                
                while True:
                    try:
                        choice = input(f"Select tag (1-{len(recent_tags)}): ").strip()
                        tag_index = int(choice) - 1
                        if 0 <= tag_index < len(recent_tags):
                            selected_tag = recent_tags[tag_index]
                            break
                        else:
                            print(f"Invalid choice. Please enter 1-{len(recent_tags)}.")
                    except (ValueError, KeyboardInterrupt):
                        print("\nAborted.")
                        return 1
            
            # Determine if should tag as latest
            if hasattr(args, 'latest') and args.latest is not None:
                make_latest = args.latest
            else:
                # Interactive latest selection
                while True:
                    try:
                        latest_choice = input(f"Tag {selected_tag} as 'latest'? (y/n): ").strip().lower()
                        if latest_choice in ['y', 'yes']:
                            make_latest = True
                            break
                        elif latest_choice in ['n', 'no']:
                            make_latest = False
                            break
                        else:
                            print("Please enter 'y' or 'n'.")
                    except KeyboardInterrupt:
                        print("\nAborted.")
                        return 1
            
            # Determine multi-arch build preference
            multiarch = False
            if hasattr(args, 'multiarch') and args.multiarch is not None:
                multiarch = args.multiarch
            elif not args.tag:  # Only ask interactively if no tag specified
                while True:
                    try:
                        multiarch_choice = input("Build multi-architecture image (amd64 + arm64)? (y/n): ").strip().lower()
                        if multiarch_choice in ['y', 'yes']:
                            multiarch = True
                            break
                        elif multiarch_choice in ['n', 'no']:
                            multiarch = False
                            break
                        else:
                            print("Please enter 'y' or 'n'.")
                    except KeyboardInterrupt:
                        print("\nAborted.")
                        return 1
            
            # Build and optionally push
            image_tag = self.build_docker_image(
                version=selected_tag,
                registry=args.registry,
                image_name=args.image_name,
                make_latest=make_latest,
                push=args.push,
                dry_run=args.dry_run,
                multiarch=multiarch,
                architectures=args.architectures if hasattr(args, 'architectures') else None
            )
            
            if not args.dry_run:
                print(f"Successfully built: {image_tag}")
                if args.push:
                    print("Images pushed to registry")
            
        except (GitError, VersionError) as e:
            print(f"Error: {e}", file=sys.stderr)
            return 1
        except KeyboardInterrupt:
            print("\nAborted.", file=sys.stderr)
            return 1
        
        return 0


def main():
    parser = argparse.ArgumentParser(
        description='LiveReview Operations Script',
        formatter_class=argparse.RawDescriptionHelpFormatter
    )
    
    parser.add_argument('--verbose', '-v', action='store_true',
                      help='Enable verbose output')
    parser.add_argument('--repo-root', type=str,
                      help='Git repository root path (auto-detected if not specified)')
    
    subparsers = parser.add_subparsers(dest='command', help='Available commands')
    
    # Version command
    version_parser = subparsers.add_parser('version', help='Show version information')
    version_parser.add_argument('--json', action='store_true',
                               help='Output in JSON format')
    version_parser.add_argument('--build-info', action='store_true',
                               help='Include build information')
    
    # Bump command
    bump_parser = subparsers.add_parser('bump', help='Create new version tag')
    bump_parser.add_argument('--type', choices=['patch', 'minor', 'major'],
                            help='Version increment type (non-interactive)')
    bump_parser.add_argument('--dry-run', action='store_true',
                            help='Show what would be done without making changes')
    bump_parser.add_argument('--ci', action='store_true',
                            help='CI mode (non-interactive, auto-detect increment type)')
    bump_parser.add_argument('--allow-dirty', action='store_true',
                            help='Allow tagging with uncommitted changes')
    bump_parser.add_argument('--no-push', action='store_true',
                            help='Create tag locally but do not push to remote')
    
    # Build command
    build_parser = subparsers.add_parser('build', help='Build binary with version info')
    build_parser.add_argument('--version', type=str,
                             help='Specific version to embed (auto-detected if not specified)')
    build_parser.add_argument('--docker', action='store_true',
                             help='Build Docker image instead of binary')
    build_parser.add_argument('--push', action='store_true',
                             help='Push Docker image to registry (requires --docker)')
    build_parser.add_argument('--registry', type=str,
                             help='Docker registry URL (default: git.apps.hexmos.com:5050/hexmos)')
    build_parser.add_argument('--image-name', type=str,
                             help='Docker image name (default: livereview)')
    build_parser.add_argument('--dry-run', action='store_true',
                             help='Show what would be done without making changes')
    build_parser.add_argument('--latest', action='store_true',
                             help='Tag as latest (for --docker builds)')
    build_parser.add_argument('--multiarch', action='store_true',
                             help='Build multi-architecture image (amd64 + arm64)')
    build_parser.add_argument('--architectures', nargs='+', 
                             choices=['amd64', 'arm64', 'arm/v7'],
                             help='Specific architectures to build (default: amd64 arm64)')
    
    # Docker command (interactive tag selection)
    docker_parser = subparsers.add_parser('docker', help='Build and push Docker images with interactive tag selection')
    docker_parser.add_argument('--tag', type=str,
                              help='Specific tag to build (non-interactive)')
    docker_parser.add_argument('--latest', action='store_true',
                              help='Tag as latest (non-interactive)')
    docker_parser.add_argument('--no-latest', dest='latest', action='store_false',
                              help='Do not tag as latest (non-interactive)')
    docker_parser.add_argument('--push', action='store_true',
                              help='Push Docker image to registry')
    docker_parser.add_argument('--registry', type=str,
                              help='Docker registry URL (default: git.apps.hexmos.com:5050/hexmos)')
    docker_parser.add_argument('--image-name', type=str,
                              help='Docker image name (default: livereview)')
    docker_parser.add_argument('--dry-run', action='store_true',
                              help='Show what would be done without making changes')
    docker_parser.add_argument('--multiarch', action='store_true',
                              help='Build multi-architecture image (amd64 + arm64)')
    docker_parser.add_argument('--no-multiarch', dest='multiarch', action='store_false',
                              help='Build single architecture image only')
    docker_parser.add_argument('--architectures', nargs='+',
                              choices=['amd64', 'arm64', 'arm/v7'],
                              help='Specific architectures to build (default: amd64 arm64)')
    docker_parser.set_defaults(latest=None, multiarch=None)  # Allow None to trigger interactive mode
    
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        return 1
    
    try:
        ops = LiveReviewOps(repo_root=args.repo_root, verbose=args.verbose)
        
        if args.command == 'version':
            return ops.cmd_version(args)
        elif args.command == 'bump':
            return ops.cmd_bump(args)
        elif args.command == 'build':
            return ops.cmd_build(args)
        elif args.command == 'docker':
            return ops.cmd_docker(args)
        else:
            print(f"Unknown command: {args.command}", file=sys.stderr)
            return 1
            
    except (GitError, VersionError) as e:
        print(f"Error: {e}", file=sys.stderr)
        return 2
    except KeyboardInterrupt:
        print("\nAborted.", file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())