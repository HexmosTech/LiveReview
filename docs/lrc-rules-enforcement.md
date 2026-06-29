# `.lrc/` Rules Enforcement for Webhook-Triggered Reviews

## Problem

When `git-lrc` (the local CLI) runs a review, it reads `.lrc/` from the local filesystem, bundles the rules, and uploads them in a zip to LiveReview. The server-side pipeline applies ignore patterns and injects rules into the AI prompt via `prompts.WithRepoRules`.

But when LiveReview receives a **webhook** (PR opened, or a bot-mention comment in a PR thread), there is no local filesystem. The `.lrc/` rules were silently skipped, meaning teams using webhook-driven reviews got no benefit from their per-repo AI instructions.

---

## Solution: API-Based Fetching

Each git host has a REST API for fetching file/directory contents. LiveReview already holds auth tokens for every connected repo — no new credentials needed, no git binary, no temp dirs.

The `.lrc/` folder is tiny (≤ 10 small `.md` files). Fetching blob-by-blob via API is fast and correct at this scale.

**No persistent caching** — fetched fresh on every review event. Simple, no invalidation complexity.

---

## Branch Selection: Target Branch

Use `mrDetails.TargetBranch` (the PR's base branch, usually `main`/`master`).

**Security**: If the source branch were used, any PR author could inject arbitrary AI instructions by modifying `.lrc/rules/` on their feature branch before the review runs. Target branch is maintainer-controlled and represents actual team policy.

**Fallback**: if `TargetBranch` is empty, fall back to `SourceBranch`.

---

## Architecture

### New Package: `internal/lrcfetch`

A dependency-free package that breaks an import cycle. Providers implement this interface:

```go
package lrcfetch

import "context"

type Provider interface {
    GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (files map[string][]byte, ok bool, err error)
}
```

Returns `map[string][]byte` (not `lrcconfig.Bundle`) so providers don't import `lrcconfig` and avoid the cycle:
`provider_input/github` → `lrcconfig` → `cmd/mrmodel/lib` → `provider_input/github`

Call sites wrap the result: `lrcconfig.BundleFromFiles(files)`.

### Integration Points

1. **Review flow** (`internal/review/service.go`) — after building diffs, before calling AI
2. **Webhook comment/query flow** (`internal/api/webhook_orchestrator_v2.go`) — after `FetchMergeRequestData`, before unified processor

---

## API Reference Per Provider

### GitHub

**Source**: https://docs.github.com/en/rest/repos/contents

#### List `.lrc/` directory

```
GET https://api.github.com/repos/{owner}/{repo}/contents/.lrc?ref={branch}
Authorization: token {pat}
Accept: application/vnd.github+json
```

Response — array of objects:
```json
[
  { "type": "file", "name": "ignore",  "path": ".lrc/ignore" },
  { "type": "dir",  "name": "rules",   "path": ".lrc/rules"  }
]
```

`type` is `"file"` or `"dir"`. When `rules/` appears as a dir, make a second call.

**404** → `.lrc/` does not exist → return `ok=false, err=nil`.

#### List `.lrc/rules/` (second call)

```
GET https://api.github.com/repos/{owner}/{repo}/contents/.lrc/rules?ref={branch}
Authorization: token {pat}
Accept: application/vnd.github+json
```

Filter: `type=="file"`, `.md` extension, no nested `/` after `rules/` (direct children only).

#### Fetch file content (raw, no base64)

```
GET https://api.github.com/repos/{owner}/{repo}/contents/{path}?ref={branch}
Authorization: token {pat}
Accept: application/vnd.github.raw+json
```

With `Accept: application/vnd.github.raw+json` the response body is raw bytes — no base64 decoding needed.

**Total API calls**: 2 directory lists + N file fetches.

---

### GitLab

**Source**: https://docs.gitlab.com/api/repositories/ and https://docs.gitlab.com/api/repository_files/

#### List `.lrc/` tree (recursive — one call)

```
GET {instanceURL}/api/v4/projects/{url.PathEscape(repoFullName)}/repository/tree?path=.lrc&ref={branch}&recursive=true&per_page=100
Authorization: Bearer {token}
User-Agent: LiveReview-Bot
```

Response — array of tree entries:
```json
[
  { "type": "blob", "name": "ignore",          "path": ".lrc/ignore" },
  { "type": "tree", "name": "rules",           "path": ".lrc/rules" },
  { "type": "blob", "name": "INSTRUCTIONS.md", "path": ".lrc/rules/INSTRUCTIONS.md" },
  { "type": "blob", "name": "design.md",       "path": ".lrc/rules/design.md" }
]
```

`type`: `"blob"` = file, `"tree"` = directory. With `recursive=true`, all nested blobs are returned in one call.

**404** → `.lrc/` does not exist. GitLab <17.7 returns 200 + empty array for non-existent paths — handle both.

Filter blobs where `path` matches `.lrc/rules/*.md` (direct child: no `/` in segment after `rules/`) or `.lrc/ignore`.

#### Fetch file content (raw)

```
GET {instanceURL}/api/v4/projects/{encodedProject}/repository/files/{url.PathEscape(filePath)}/raw?ref={branch}
Authorization: Bearer {token}
```

Response body = raw bytes. No encoding to decode.

**Total API calls**: 1 tree list + N file fetches.

**Instance URL**: For self-hosted GitLab, the instance URL is injected via context using `gitlabinput.WithInstanceURL(ctx, url)`. The webhook orchestrator extracts it from `event.Repository.WebURL` via `gitlabinput.ExtractGitLabInstanceURL(webURL)`.

**Token lookup**: Queries `integration_tokens` table matching `provider_url`. Falls back to any GitLab token if no URL match.

---

### Bitbucket

**Source**: https://developer.atlassian.com/cloud/bitbucket/rest/api-group-source/

#### List `.lrc/` directory

```
GET https://api.bitbucket.org/2.0/repositories/{workspace}/{repo_slug}/src/{ref}/.lrc/?pagelen=100
Authorization: Basic {base64(email:app_password)}
```

`{ref}` = branch name (e.g., `main`). `workspace` and `repo_slug` come from `event.Repository.FullName`.

Response:
```json
{
  "values": [
    { "type": "commit_file",      "path": ".lrc/ignore",
      "links": { "self": { "href": "..." } } },
    { "type": "commit_directory", "path": ".lrc/rules",
      "links": { "self": { "href": "..." } } }
  ]
}
```

`type`: `"commit_file"` = file, `"commit_directory"` = directory.

**404** → `.lrc/` does not exist → return `ok=false, err=nil`.

#### List `.lrc/rules/` (second call)

```
GET https://api.bitbucket.org/2.0/repositories/{workspace}/{repo}/src/{ref}/.lrc/rules/?pagelen=100
```

Filter: `type=="commit_file"`, `.md` extension, no nested separator after `rules/`.

#### Fetch file content (raw)

```
GET https://api.bitbucket.org/2.0/repositories/{workspace}/{repo}/src/{ref}/{relative_path}
```

**Important**: Bitbucket returns file content as **raw bytes** directly — NOT base64 encoded.

**Auth**: Basic Auth using `email` from `token.Metadata["email"]` and `token.PatToken` as the password.

**Total API calls**: 2 directory lists + N file fetches.

---

### Gitea

**Source**: https://docs.gitea.com/api/ (Swagger at `/swagger`)

Gitea uses a GitHub-compatible REST API structure.

#### List `.lrc/` directory

```
GET {baseURL}/api/v1/repos/{owner}/{repo}/contents/.lrc?ref={branch}
Authorization: token {pat}
```

Response — array of objects (same structure as GitHub):
```json
[
  { "type": "file", "name": "ignore", "path": ".lrc/ignore" },
  { "type": "dir",  "name": "rules",  "path": ".lrc/rules"  }
]
```

**404** → `.lrc/` does not exist → return `ok=false, err=nil`.

#### List `.lrc/rules/` (second call)

```
GET {baseURL}/api/v1/repos/{owner}/{repo}/contents/.lrc/rules?ref={branch}
Authorization: token {pat}
```

#### Fetch file content (base64-encoded)

```
GET {baseURL}/api/v1/repos/{owner}/{repo}/contents/{path}?ref={branch}
Authorization: token {pat}
```

Response (single file):
```json
{
  "type": "file",
  "name": "design.md",
  "path": ".lrc/rules/design.md",
  "content": "IyBEZXNpZ24gUnVsZXMKCi0gVXNlIFJFU1QgQVBJcwo=",
  "encoding": "base64"
}
```

**`content`** is base64-encoded with embedded newlines — strip `\n` before decoding:
```go
cleaned := strings.ReplaceAll(entry.Content, "\n", "")
data, _ := base64.StdEncoding.DecodeString(cleaned)
```

Unlike GitHub, Gitea does not support `Accept: application/vnd.github.raw+json`. Pagination max is 50 items per page.

**baseURL** comes from `FindIntegrationTokenForGiteaRepo` (returns token + instance base URL).

**Total API calls**: 2 directory lists + N file fetches.

---

## Files Changed

| Action | File | Purpose |
|--------|------|---------|
| Create | `internal/lrcfetch/provider.go` | Cycle-breaking interface |
| Modify | `internal/lrcconfig/provider.go` | Added `BundleFromFiles` helper |
| Modify | `internal/lrcconfig/lrcconfig.go` | Added `FilterCodeDiffs` for `[]*models.CodeDiff` |
| Create | `internal/provider_input/github/lrc_fetch.go` | GitHub implementation |
| Create | `internal/provider_input/gitlab/lrc_fetch.go` | GitLab implementation + `WithInstanceURL`, `ExtractGitLabInstanceURL` |
| Create | `internal/provider_input/bitbucket/lrc_fetch.go` | Bitbucket implementation |
| Create | `internal/provider_input/gitea/lrc_fetch.go` | Gitea implementation |
| Modify | `internal/review/service.go` | Activated TODO block; fetches `.lrc/` using target branch |
| Modify | `internal/api/webhook_orchestrator_v2.go` | `injectLRCRules` after `FetchMergeRequestData` |
| Modify | `internal/api/unified_processor_v2.go` | Passes repo rules section into comment reply prompt |
| Modify | `internal/api/unified_processing_test.go` | Fixed signature of `buildCommentReplyPromptWithLearning` |

---

## Key Design Decisions

### Import Cycle Fix

`internal/lrcfetch` is a standalone package with no dependencies. The `Provider` interface returns `map[string][]byte` instead of `lrcconfig.Bundle`, so provider packages don't need to import `lrcconfig`. Call sites do: `lrcconfig.BundleFromFiles(files)`.

### FilterCodeDiffs vs FilterDiffs

`lrcconfig.FilterDiffs` takes `[]lib.LocalCodeDiff` (CLI type). The server review flow uses `[]*models.CodeDiff`. Added `lrcconfig.FilterCodeDiffs` as a separate function.

### GitLab Instance URL

For self-hosted GitLab, the API base URL must match the instance. The webhook orchestrator extracts it from `event.Repository.WebURL` and stores it in context via `gitlabinput.WithInstanceURL`. The GitLab provider reads it back via `instanceURLFromContext`. Falls back to `https://gitlab.com`.

---

## Security

- **Target branch only**: `.lrc/` is always fetched from the PR's target branch (usually `main`), not the source/feature branch. PR authors cannot inject rules by modifying `.lrc/` on their branch.
- **Existing tokens**: No new credentials are introduced. Each provider reuses its stored integration token.
- **Non-fatal**: A missing or inaccessible `.lrc/` is the common case and is silently skipped (logged at WARN if a fetch error occurs, not an error).

---

## Verification Checklist

- [ ] Open a PR on a repo with `.lrc/rules/INSTRUCTIONS.md` on `main` → verify rules appear in the AI review prompt
- [ ] Post `@livereviewbot` comment on same PR → verify bot respects repo rules
- [ ] Security test: PR where source branch has `.lrc/rules/manipulation.md` but target branch (`main`) does not → verify injected rules do NOT include the manipulation file
- [ ] Repo with no `.lrc/` folder → PR review and bot comments complete without error
