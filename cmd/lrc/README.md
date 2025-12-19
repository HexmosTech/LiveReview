# lrc - LiveReview CLI

`lrc` is a command-line tool for submitting local code diffs to LiveReview for AI-powered code review.

## Installation

Build the binary:

```bash
make lrc
```

Or build directly:

```bash
go build -o lrc ./cmd/lrc
```

## Usage

### Basic Usage

Review staged changes:

```bash
lrc --api-key YOUR_API_KEY
```

### Diff Sources

- **Staged changes** (default):
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source staged
  ```

- **Working tree changes**:
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source working
  ```

- **Git range**:
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source range --range HEAD~1..HEAD
  ```

- **From file**:
  ```bash
  lrc --api-key YOUR_API_KEY --diff-source file --diff-file my-changes.diff
  ```

### Configuration

All flags can be set via environment variables:

```bash
export LRC_API_KEY="your-api-key"
export LRC_API_URL="http://localhost:8888"
export LRC_REPO_NAME="my-project"
export LRC_OUTPUT="pretty"  # or "json"

lrc --diff-source staged
```

### Flags

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--repo-name` | `LRC_REPO_NAME` | current dir basename | Repository name |
| `--diff-source` | `LRC_DIFF_SOURCE` | `staged` | Diff source: `staged`, `working`, `range`, or `file` |
| `--range` | `LRC_RANGE` | | Git range (e.g., `HEAD~1..HEAD`) for `range` mode |
| `--diff-file` | `LRC_DIFF_FILE` | | Path to diff file for `file` mode |
| `--api-url` | `LRC_API_URL` | `http://localhost:8888` | LiveReview API base URL |
| `--api-key` | `LRC_API_KEY` | *required* | API key for authentication |
| `--poll-interval` | `LRC_POLL_INTERVAL` | `2s` | Interval between status polls |
| `--timeout` | `LRC_TIMEOUT` | `5m` | Maximum wait time for review |
| `--output` | `LRC_OUTPUT` | `pretty` | Output format: `pretty` or `json` |
| `--verbose, -v` | `LRC_VERBOSE` | `false` | Enable verbose output |

## Examples

### Review uncommitted changes

```bash
# Review all staged changes
git add .
lrc --api-key YOUR_API_KEY

# Review working directory changes (unstaged)
lrc --api-key YOUR_API_KEY --diff-source working
```

### Review a specific commit

```bash
lrc --api-key YOUR_API_KEY --diff-source range --range HEAD~1..HEAD
```

### Review a saved diff

```bash
git diff main..feature-branch > changes.diff
lrc --api-key YOUR_API_KEY --diff-source file --diff-file changes.diff
```

### JSON output for scripting

```bash
lrc --api-key YOUR_API_KEY --output json > review-results.json
```

### Verbose mode for debugging

```bash
lrc --api-key YOUR_API_KEY --verbose
```

## Output Formats

### Pretty (default)

Human-readable output with file sections and colored severity levels:

```
================================================================================
LIVEREVIEW RESULTS
================================================================================

Summary:
The code looks good overall. Minor suggestions below.

2 file(s) with comments:

--------------------------------------------------------------------------------
FILE: src/main.go
--------------------------------------------------------------------------------

  [WARNING] Line 42 (best-practices)
    Consider using context.WithTimeout instead of time.Sleep

  [INFO] Line 89 (style)
    Variable name could be more descriptive

================================================================================
Review complete: 2 total comment(s)
================================================================================
```

### JSON

Machine-readable JSON output for automation:

```json
{
  "status": "completed",
  "summary": "The code looks good overall. Minor suggestions below.",
  "files": [
    {
      "file_path": "src/main.go",
      "hunks": [...],
      "comments": [
        {
          "line": 42,
          "content": "Consider using context.WithTimeout instead of time.Sleep",
          "severity": "warning",
          "category": "best-practices"
        }
      ]
    }
  ]
}
```

## API Requirements

The `lrc` tool requires:

1. A running LiveReview API server (default: `http://localhost:8888`)
2. A valid API key (obtain from your LiveReview account settings)

The tool communicates with these endpoints:

- `POST /api/v1/diff-review` - Submit diff for review
- `GET /api/v1/diff-review/:id` - Poll for review status/results

## Exit Codes

- `0` - Success
- `1` - Error (network failure, invalid input, review failed, timeout, etc.)

## Troubleshooting

### "no diff content collected"

Make sure you have uncommitted changes:

```bash
git status
git diff --staged  # Check if there are staged changes
```

### "API returned status 401"

Check your API key:

```bash
echo $LRC_API_KEY
# or
lrc --api-key YOUR_API_KEY --verbose
```

### "timeout waiting for review completion"

Increase the timeout:

```bash
lrc --api-key YOUR_API_KEY --timeout 10m
```

### Connection refused

Ensure the LiveReview API server is running:

```bash
# Start the API server
./livereview api

# Or check if it's already running
curl http://localhost:8888/health
```
