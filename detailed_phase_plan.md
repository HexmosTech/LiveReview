# LiveReview Phase 1: Detailed Step-by-Step Implementation Plan

## Overview
Phase 1 focuses on **incremental architectural separation** of the unified pipeline into three packages:

1. `internal/provider_input` – everything required to detect provider webhooks and fetch additional platform data ("fetch/input")
2. `internal/core_processor` – provider-agnostic processing logic ("process")
3. `internal/provider_output` – anything that posts data back to providers ("post/output")

Every change must end with a successful `make build` from the repo root. If a step fails, stop, revert that step, and adjust before proceeding. Each step group below is intentionally small to avoid the circular dependency traps we hit previously.

## Current State Analysis (ACTUAL STATE)
- ✅ Some refactor changes have been applied (see progress snapshot)
- ✅ `internal/core_processor/` contains the relocated unified types, context builder and processor (moved from `internal/api`)
- ✅ `internal/api/` exposes a small alias/bridge file (`internal/api/unified_types_alias.go`) to smooth the migration
- ✅ `internal/provider_input/` subfolders exist and already contain profile helpers for GitHub and GitLab
- ✅ GitHub provider logic now lives in `internal/provider_input/github`
- ✅ `internal/api/server.go` has been updated to call the new `provider_input` helpers and instantiate the GitHub provider from there
- ✅ `make build` currently succeeds from the repo root
- ❌ GitLab and Bitbucket providers still live in `internal/api/`
- ❌ GitHub “post” helpers are still bundled with the provider input package; `internal/provider_output/github` is empty
- ⚠️ Tests were updated in earlier steps, but full `make test` validation should be run after remaining provider moves

## Phase 1 Target Architecture
```
internal/
├── core_processor/          # Pure unified processing logic (zero platform awareness)
│   ├── unified_processor_v2.go    # Main processing logic
│   ├── unified_context_v2.go      # Context building
│   ├── unified_types.go           # Unified data types
│   └── unified_processing_test.go # Core processing tests
│
├── provider_input/          # All input-side provider logic
│   ├── github/
│   │   ├── github_provider_v2.go  # GitHub webhook handling + API fetching
│   │   └── github_profile.go      # GitHub profile management
│   ├── gitlab/
│   │   ├── gitlab_provider_v2.go  # GitLab webhook handling + API fetching
│   │   ├── gitlab_auth.go         # GitLab authentication
│   │   └── gitlab_profile.go      # GitLab profile management
│   ├── bitbucket/
│   │   ├── bitbucket_provider_v2.go # Bitbucket webhook handling + API fetching
│   │   └── bitbucket_profile.go     # Bitbucket profile management
│   └── registry.go          # Provider registry and coordination
│
├── provider_output/         # Future: Output-side provider logic
│   └── (prepared for Phase 2)
│
└── api/                     # HTTP API layer (orchestration only)
    ├── server.go                   # Main server
    ├── webhook_handler.go          # HTTP webhook endpoints
    ├── webhook_registry_v2.go      # Provider registry
    ├── webhook_orchestrator_v2.go  # Flow orchestration
    └── learning_processor_v2.go    # Learning extraction
```

---

## Step Group 1: Current State and Guardrails
**Goal**: Capture baseline state and reaffirm guardrails**  
**Risk**: 🟢 **ZERO** – Read-only analysis  
**Verification**: `make build`

### Step 1.1: Baseline Build
```bash
make build
# Must succeed before making any structural change
```

### Step 1.2: Snapshot of Files to Rehome (ACTUAL STATE)
All files listed below currently live in `internal/api/`:

**Core processor candidates (→ `internal/core_processor/`):**
- `unified_types.go`
- `unified_context_v2.go`
- `unified_processor_v2.go`
- `unified_processing_test.go`

**Provider input candidates (→ `internal/provider_input/<provider>/`):**
- `github_provider_v2.go`
- `gitlab_provider_v2.go`
- `bitbucket_provider_v2.go`
- `github_profile.go`
- `gitlab_profile.go`
- `gitlab_auth.go`
- `bitbucket_profile.go`

**Provider output candidates (→ `internal/provider_output/<provider>/`):**
- Posting helpers currently embedded in each `*_provider_v2.go` file (will be split out later in the phase)

**Stay in `internal/api/`:**
- `webhook_interfaces.go`
- `webhook_handler.go`
- `webhook_orchestrator_v2.go`
- `webhook_registry_v2.go`
- `learning_processor_v2.go`

### Step 1.3: Import Inventory (ACTUAL RESULTS)
There are 9 external packages importing `internal/api`, mainly inside the same module (`internal/api/users/...`, `internal/api/server.go`, etc.). This keeps the blast radius small but we must avoid introducing `internal/api` → `core_processor` → `internal/api` cycles while moving code.

---

## Step Group 2: Carve Out Core Types and Processor
**Goal**: Move the truly provider-agnostic pieces first so that providers can depend on them without pulling in `internal/api`.  
**Risk**: 🟡 **LOW** – compile-time enforced.  
**Verification**: `make build` after every bullet.

### Step 2.1: Create the `core_processor` package boundary
- Add `package coreprocessor` (temporary bridge) files that re-export the existing types as type aliases. This keeps callers compiling while we relocate actual files.  
- `make build`

> STATUS: COMPLETED — temporary alias/bridge approach was used and later trimmed. `internal/api/unified_types_alias.go` exists to keep callers compiling during migration.

### Step 2.2: Move `unified_types.go`
- Physically move `unified_types.go` to `internal/core_processor/` and switch its package declaration to `core_processor`.  
- Update imports in dependent files from `internal/api` to `internal/core_processor`.  
- Remove any temporary type aliases once all references compile.  
- `make build`

> STATUS: COMPLETED — `unified_types.go` moved and imports updated; aliasing kept compatibility while callers were adjusted.

### Step 2.3: Move `unified_context_v2.go`
- Relocate the file, adjust package, and point its imports at the new `core_processor` types.  
- Update all call sites (Go compiler ensures completeness).  
- `make build`

> STATUS: COMPLETED — `unified_context_v2.go` relocated and callers updated.

### Step 2.4: Move `unified_processor_v2.go` and test file
- Move both production and test files.  
- Update imports in the orchestrator, registry, or any other users.  
- `make build`

> STATUS: COMPLETED — processor and its tests were moved; tests were adjusted where needed.

### Step 2.5: Delete temporary bridge aliases (if any remain)
- Once everything compiles, drop the bridging file(s).  
- `make build`

> STATUS: PARTIAL — the bridge/alias file remains in `internal/api` for a short migration window. It can be removed after the final provider files and registry wiring are moved.

---

## Step Group 3: Stabilize `api` → `core_processor` Dependencies
**Goal**: Ensure all references point from `api` to `core_processor` with no cycles.  
**Risk**: 🟡 **LOW** – compiler guarantees catch missing changes.  
**Verification**: `make build`

- Use `go list -deps` or `go list ./...` to confirm there is no reverse import from `core_processor` back into `internal/api`.  
- Update any lingering helper references (e.g., logging wrappers) so `core_processor` depends only on standard library + `internal` packages that do **not** pull `api` back in.  
- `make build`

---

## GitHub Integration Verification Harness (BLOCKER)
**Goal**: Capture a single GitHub PR run end-to-end, persist every payload required to replay it, and build targeted tests that exercise each pipeline stage until the exact line-mapping defect is identified and fixed. No further provider moves until this passes.  
**Risk**: 🟠 **MEDIUM** – requires new tooling and fixtures but unlocks deterministic debugging.  
**Verification**: Dedicated regression tests replaying the captured payloads produce identical comment targets as the live GitHub API expects.

### Step G1: One-Time Live Capture
- Trigger one GitHub review manually (single PR) after confirming credentials.  
- Capture helpers now emit artifacts into `captures/github/<timestamp>/` by default (override with `LIVEREVIEW_GITHUB_CAPTURE_DIR` if needed).  
- Ensure sensitive data (tokens, secrets) are redacted before promoting fixtures into `testdata/github/live_capture/`.
- Capture artifacts fall into predictable families (all JSON):
    - `github-pr-details-*.json`: normalized output of `GetMergeRequestDetails` calls; includes owner/repo/pr metadata and diff refs.
    - `github-pr-files-*.json`: raw `/pulls/:number/files` responses listing filenames, status, and patches.
    - `github-pr-diffs-*.json`: parsed `models.CodeDiff` slices produced by our converter immediately after `github-pr-files` ingestion.
    - `github-webhook-<type>-body-*.json`: exact webhook payload bodies as received from GitHub for each event type (`issue_comment`, `pull_request_review`, `pull_request_review_comment`, `reviewer`).
    - `github-webhook-<type>-meta-*.json`: sanitized headers plus minimal metadata (event type, capture error string when conversion fails).
    - `github-webhook-<type>-unified-*.json`: unified structs returned by the converter stage (when conversion succeeds). Use these as golden inputs for replay tests.

### Step G2: Fixture Normalization
- Write a small sanitizer that strips volatile fields (timestamps, etags) and normalizes ordering to make the fixtures deterministic.  
- Store the cleaned payloads in `internal/provider_input/github/testdata/` with README notes describing origin and redaction steps.  
- Add a checksum or metadata file documenting the PR URL, commit SHA, and capture date.

### Step G3: Stage-by-Stage Replay Tests
- Build table-driven tests that load the fixtures and feed them into each conversion step:
    1. Provider input translation → unified change set
    2. Core processor batching/grouping → suggested comments
    3. Provider output mapping → GitHub API request structs
- At each step assert file paths, hunks, and line numbers match the captured GitHub diff (add expectations alongside fixtures).  
- Fail the test if any comment targets a line absent from the diff hunks.

#### Step G3A: GitHub Replay Harness (Completed 2025-10-11)
1. ✅ Promoted the sanitized MR-context capture from `captures/github/20251011-183834/` into `internal/providers/github/testdata/`, documenting any manual redactions.
2. ✅ Added README notes describing fixture origin and sanitization steps.
3. ✅ Implemented MR context regression tests (including threaded reply coverage) that replay unified webhook fixtures through the GitHub context builder and compare against golden timelines.
4. ✅ Ran `go test ./internal/providers/github` to confirm the regression harness passes prior to moving on to other providers.

### Step G4: Bug Isolation and Fix
- Use the replay tests to pinpoint whether the discrepancy comes from diff parsing, hunk stitching, or AI comment attribution.  
- Fix the offending logic (likely in the diff-to-unified mapping) and expand expectations to cover the previous failure cases.  
- Re-run tests to verify the corrected line mappings produce valid API requests.

### Step G5: Regression Gate
- Add a CI job (or Makefile target) `make github-fixture-test` that runs the replay suite.  
- Block further provider migrations until this job passes locally.  
- Document the harness in `processor_design.md`, including instructions for refreshing fixtures when needed.

## Step Group 4: Provider Input (Fetch) Extraction
**Goal**: Relocate provider detection + fetch code into independent packages without yet touching posting logic.  
**Risk**: 🟡 **LOW** – large files but mechanical moves.  
**Verification**: `make build` after **each provider**.

### Step 4.1: Introduce provider-specific packages
- Create `internal/provider_input/github` and `.../gitlab` packages with new package names (Bitbucket will be handled in the next phase).  
- Work one provider at a time to preserve linear progress.  
- For each provider: move `*_profile.go` first (pure helpers), adjust imports, run `make build`.  
- Then move the corresponding `*_provider_v2.go`, adjust imports, run `make build` again.

> STATUS: IN PROGRESS

- ✅ `github_profile.go` moved into `internal/provider_input/github`
- ✅ `gitlab_profile.go` moved into `internal/provider_input/gitlab`
- ✅ `internal/api/server.go` now calls the new profile helpers
- ✅ `github_provider_v2.go` relocated into `internal/provider_input/github`; registry/server wiring updated
- 🚧 GitLab provider files still reside in `internal/api` (current focus)
- ⏳ Bitbucket provider files deferred to Phase 2 after GitHub + GitLab parity

Immediate next step: finish the GitLab refactor end-to-end (input package, processing touchpoints, and output integration) before tackling Bitbucket in the subsequent phase.

#### Step 4.1A: GitLab extraction game plan (NEXT)
1. Input: Move `internal/api/gitlab_provider_v2.go` (and any helpers such as `gitlab_auth.go`) into `internal/provider_input/gitlab`, updating package declarations, imports, and registry wiring. Run `make build` after each move.
2. Processing: Audit `internal/core_processor` usage for GitLab-specific shims; ensure any remaining references to `internal/api` are replaced with the new `provider_input/gitlab` package or shared abstractions. Re-run targeted tests that exercise GitLab conversion/context building.
3. Output: Create `internal/provider_output/gitlab`, migrate posting/reply/learning helpers from the old provider file, and update the orchestrator so UI-triggered reviews, replies, and learning calls route through the new package. Provide lightweight fakes for tests.
4. End-to-end validation: Trigger a GitLab review from the UI (manual or scripted), confirm comment posting, threaded replies, and learning capture all succeed. Document any gaps uncovered for follow-up work.
5. Leave Bitbucket untouched for now; schedule its migration for the next phase once GitLab mirrors the completed GitHub structure.

### Step 4.2: Update registry wiring after each provider move
- Update `internal/api/webhook_registry_v2.go` (and other orchestrators) to import the new packages.  
- Ensure the new packages depend only on `core_processor` (and shared utilities), not `internal/api`.  
- `make build`

---

## Step Group 5: Provider Output Separation (Post)
**Goal**: Extract the posting logic into `internal/provider_output/<provider>` packages and validate full review flows.  
**Risk**: 🟡 **LOW** – but watch for shared helpers.  
**Verification**: `make build` per provider.

### Step 5.1: GitHub Output (End-to-End Validation)
- Create `internal/provider_output/github` with concrete poster/reaction helpers moved from the input package.
- Define lightweight interfaces consumed by the GitHub provider so `provider_input/github` depends only on abstractions.
- Update `internal/api/server.go` (and registry/orchestrator wiring) to construct both the input provider and the output implementation.
- Adjust tests with stubs for the new interfaces.
- `make build`
- **Quality gate**: trigger a GitHub review end-to-end (webhook → reply → learning capture) to verify behavior before moving on.

### Step 5.2: GitLab Output & Learning Hooks
- Mirror the GitHub pattern immediately for GitLab: move POST/reply/learning helpers into `internal/provider_output/gitlab`, expose interfaces, and update wiring so UI-triggered reviews, replies, and learning captures pass through the new package.
- Update shared abstractions so both GitHub and GitLab providers use the same contracts where possible.
- Validate with automated tests plus a manual GitLab UI run.

### Step 5.3: Bitbucket Output (Deferred)
- Repeat the same extraction and verification for Bitbucket in Phase 2 once GitHub and GitLab are stable.

---

## Step Group 6: Integration Hardening
**Goal**: Confirm the new package boundaries are stable and all consumers are updated.  
**Risk**: � **LOW**  
**Verification**: `make build`, `make test`.

- Search for lingering `internal/api` imports that reference moved symbols and update the import path.  
- Run `go list ./...` to catch any new circular dependency warnings.  
- Execute `make build` and `make test` after each batch of import fixes.  
- Verify `./livereview --help` still works.

---

## Step Group 7: Documentation + Final Checks
**Goal**: Capture the new structure and ensure the workspace reflects it.  
**Risk**: 🟢 **ZERO**  
**Verification**: `make build`

- Document the new boundaries in `internal/api/README.md` (and/or `processor_design.md`).  
- Snapshot final tree (`tree -L 2 internal/core_processor internal/provider_input internal/provider_output`).  
- `make build`

---

## Phase 1 Success Criteria

### ✅ Architecture Compliance
- [x] `internal/core_processor` contains the moved unified types/context/processor and is being consumed via the aliasing bridge during migration
- [ ] `internal/core_processor` exposes only provider-agnostic logic and imports no provider-specific packages.
- [ ] `internal/provider_input/<provider>` imports `internal/core_processor` but not `internal/api`.
- [ ] `internal/provider_output/<provider>` handles provider posting without referencing `internal/api`.
- [ ] No circular dependency cycles per `go list ./...`.

### ✅ Build Stability
- [x] `make build` currently passes from the repo root
- [ ] `make test` passes after Step Groups 2, 4, and 5.  
- [ ] CLI sanity checks (`./livereview --help`) continue to run.

### ✅ Functionality Preservation  
- [ ] Webhook detection + conversion remains unchanged (verified via tests or manual payloads).  
- [ ] Unified processor behavior matches baseline responses.  
- [ ] Posting flows still reach GitHub/GitLab/Bitbucket APIs via provider output packages.

### ✅ Maintainability Improvements
- [ ] Folder layout reflects fetch/process/post separation.  
- [ ] Interfaces document responsibilities between packages.  
- [ ] Adding a new provider follows the same three-package blueprint.  
- [ ] No fallback logic introduced.

---

## Risk Mitigation Strategies

### 🟢 Zero-Risk Steps (Analysis and Documentation)
- Can be executed safely without affecting functionality
- Provide valuable information for later steps
- Help identify potential issues before making changes

### 🟡 Low-Risk Steps (File Operations with Compile-Time Safety)
- **Compile-time verification**: Go compiler catches all import/interface issues
- **Incremental approach**: One file at a time, build after each change  
- **Rollback plan**: Git commits after each successful step group
- **Testing verification**: Run tests after each major change

### 🔴 Mitigation for Any Issues
- **Immediate rollback**: `git reset --hard HEAD~1` if step fails
- **Build verification**: Never proceed if `make build` fails
- **Test confirmation**: Run relevant tests after each step group
- **Documentation**: Keep detailed notes on changes made

## Next Phase Preparation

After Phase 1 completion, the codebase will be ready for:
- **Phase 2**: Clean up provider interfaces and remove platform-specific code from core processing
- **Phase 3**: Implement clean provider input/output boundaries  
- **Future phases**: Add new providers easily using established patterns

The Phase 1 restructuring provides the foundation for all subsequent architectural improvements while maintaining complete functionality throughout the process.
