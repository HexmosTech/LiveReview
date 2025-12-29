# LiveReview Pre-Commit AI Code Review

## Overview

LiveReview now supports **automatic pre-commit AI code reviews** via Git hooks, providing hygiene-focused advisory feedback without blocking commits.

## Features

### Core Capabilities
- ✅ Reviews **only staged changes** before commit
- ✅ Never blocks commits (always exits 0)
- ✅ Can be skipped with **Ctrl-C**
- ✅ Can be bypassed with `git commit --no-verify`
- ✅ Records review participation via commit trailers
- ✅ Composes with existing Git hooks
- ✅ Works in TTY and non-TTY environments (CLI, VS Code, GitHub Desktop)

### Hook Behavior

#### Pre-Commit Hook
- Runs `lrc review --staged` before commit
- Detects TTY vs non-TTY mode (silent in GUIs)
- Handles Ctrl-C gracefully (exit 130 → skipped state)
- Uses atomic file operations for state management
- Implements stale lock cleanup (>5 minutes old)
- Always exits 0 to allow commit

#### Commit-Msg Hook
- Reads review state from `.git/livereview_state`
- Appends trailer: `LiveReview Pre-Commit Check: [ran|skipped]`
- Cleans up state file and lock directory
- Does nothing if no state file exists (hooks bypassed with `--no-verify`)

## Installation

### Install Hooks

```bash
cd /path/to/your/repo
lrc install-hooks
```

Output:
```
✅ Created pre-commit
✅ Created commit-msg
✅ LiveReview hooks installed successfully!

Pre-commit hook will:
  • Run 'lrc review --staged' before each commit
  • Never block commits (always exits 0)
  • Can be skipped with Ctrl-C
  • Can be bypassed with 'git commit --no-verify'

Commit-msg hook will:
  • Add 'LiveReview Pre-Commit Check: [ran|skipped]' trailer

To uninstall: lrc uninstall-hooks
```

### Install with Force Update

```bash
lrc install-hooks --force
```

Forces replacement of existing lrc-managed sections (preserves user content outside markers).

### Uninstall Hooks

```bash
lrc uninstall-hooks
```

Output:
```
✅ Removed lrc section from pre-commit
🗑️  Removed commit-msg (was empty after removing lrc section)
✅ Removed lrc hooks from 2 file(s)
```

## Hook Implementation Details

### Marker-Based Composition

Hooks use sentinel markers for safe coexistence:

```bash
#!/bin/sh
# BEGIN lrc managed section - DO NOT EDIT
# lrc_version: v0.0.5
# ... lrc hook code ...
# END lrc managed section

# User's existing hook content continues here
```

### State Management

**State File**: `.git/livereview_state`
- Format: `<status>:<pid>:<timestamp>`
- Status values: `ran`, `skipped`
- Created by pre-commit hook, read by commit-msg hook
- Atomically written via temp file + mv
- Cleaned up after commit-msg runs

**Lock Directory**: `.git/livereview_state.lock`
- POSIX-safe locking via `mkdir` (atomic on POSIX)
- 5-minute timeout for acquiring lock
- Automatic stale lock cleanup (>5 minutes old)
- Removed by commit-msg hook

### Backup System

**Backup Directory**: `.git/.lrc_backups/`
- Created on first hook installation
- Timestamped backups: `<hook-name>.<YYYYMMDD_HHMMSS>`
- Example: `pre-commit.20251229_142058`
- Backups created before any modification
- Manual cleanup (backups accumulate intentionally)

### TTY Detection

```bash
# Detect interactive terminal
if [ -t 0 ] && [ -t 1 ]; then
    LRC_INTERACTIVE=1  # TTY - show progress
else
    LRC_INTERACTIVE=0  # Non-TTY - silent mode
fi
```

**Behavior**:
- **TTY mode**: Shows "Running LiveReview pre-commit check..." and full output
- **Non-TTY mode**: Runs silently (`>/dev/null 2>&1`), suitable for GUIs

## Example Workflows

### Normal Commit (Review Runs)

```bash
git add .
git commit -m "feat: add new feature"
```

Output:
```
Running LiveReview pre-commit check...
Review submitted, ID: abc123
Waiting for review completion (poll every 2s, timeout 5m0s)...
Status: completed | elapsed: 12s

[... review results ...]

[main abc1234] feat: add new feature
 1 file changed, 10 insertions(+)
```

Commit message includes:
```
feat: add new feature

LiveReview Pre-Commit Check: ran
```

### Skip Review (Ctrl-C)

```bash
git add .
git commit -m "fix: quick fix"
# Press Ctrl-C during review
```

Output:
```
Running LiveReview pre-commit check...
Review submitted, ID: xyz789
^C
[main def5678] fix: quick fix
 1 file changed, 2 insertions(+)
```

Commit message includes:
```
fix: quick fix

LiveReview Pre-Commit Check: skipped
```

### Bypass Hooks Completely

```bash
git commit --no-verify -m "chore: bypass hooks"
```

No review runs, no trailer added. Clean commit message:
```
chore: bypass hooks
```

### Force Update Existing Hooks

```bash
lrc install-hooks --force
```

Updates lrc-managed sections while preserving user content outside markers.

## Troubleshooting

### "Could not acquire lock after 300s"

Lock file exists and is blocking. Check for stale locks:

```bash
ls -la .git/livereview_state.lock
rmdir .git/livereview_state.lock  # Manual cleanup if needed
```

The hook auto-removes stale locks (>5 minutes old).

### Review Runs in GUI But No Output

Expected behavior. Hooks run silently in non-TTY mode (VS Code, GitHub Desktop, etc.).

To see output, use CLI:
```bash
git commit -m "..."  # In terminal
```

### Hooks Don't Run

Check hook files exist and are executable:

```bash
ls -la .git/hooks/pre-commit .git/hooks/commit-msg
```

Reinstall if missing:
```bash
lrc install-hooks --force
```

### Existing Hooks Are Lost

Backups are in `.git/.lrc_backups/`:

```bash
ls -la .git/.lrc_backups/
cat .git/.lrc_backups/pre-commit.<timestamp>
```

Restore manually if needed:
```bash
cp .git/.lrc_backups/pre-commit.<timestamp> .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Architecture Notes

### POSIX Compliance

Hooks use `#!/bin/sh` for maximum compatibility:
- ✅ Linux (dash, bash)
- ✅ macOS (bash in POSIX mode)
- ✅ Windows (Git Bash)
- ❌ No bashisms (arrays, `[[`, etc.)
- ✅ Tested with `checkbashisms` (recommended)

### Atomic Operations

State file updates use atomic writes:
```bash
echo "ran:$$:$(date +%s)" > "${STATE_FILE}.tmp"
mv "${STATE_FILE}.tmp" "$STATE_FILE"
```

Lock directory creation is atomic on POSIX:
```bash
mkdir "$LOCK_DIR" 2>/dev/null  # Atomic test-and-set
```

### Signal Handling

Pre-commit hook traps signals:
```bash
trap cleanup_lock EXIT INT TERM
```

Ctrl-C (SIGINT) → exit 130 → state="skipped"

### Non-Goals (Explicitly NOT Implemented)

- ❌ No hard enforcement (hooks always exit 0)
- ❌ No CI gating on commit trailers
- ❌ No commit rewriting (trailers added during commit, not post-commit)
- ❌ No developer scoring
- ❌ No auto-install on clone
- ❌ No bypass of `--no-verify`
- ❌ No `core.hooksPath` usage (breaks existing hook setups)

## Security Considerations

### API Key Handling

Hooks call `lrc review --staged`, which reads API key from:
1. `~/.lrc.toml` (recommended, chmod 600)
2. `LRC_API_KEY` environment variable
3. `--api-key` flag (if hardcoded in hook)

**Recommendation**: Use `~/.lrc.toml` with proper permissions.

### State File Security

State files are in `.git/` (not tracked by Git):
- Not committed to repository
- Local to each developer's machine
- Cleaned up after each commit

## Future Enhancements (Not in Scope)

- Interactive prompt: `[Enter] Run / [Esc] Skip` (defer to Ctrl-C for now)
- Backup retention policy (keep last N backups)
- Synchronous review mode (shorter poll intervals for faster feedback)
- Pre-push hooks (review before push)

## Related Files

- Implementation: `cmd/lrc/main.go`
- Documentation: `cmd/lrc/README.md`
- Build: `Makefile` (target: `make lrc`)

## Version

Pre-commit hooks feature added in **lrc v0.0.5**

Hook version is embedded in markers:
```bash
# lrc_version: v0.0.5
```

Use `lrc install-hooks --force` to update hooks to latest version.
