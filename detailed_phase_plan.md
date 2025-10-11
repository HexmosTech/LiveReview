# LiveReview Phase 1: Detailed Step-by-Step Implementation Plan

## Overview
Phase 1 focuses on **incremental architectural separation** of the unified pipeline into three packages:

1. `internal/provider_input` – everything required to detect provider webhooks and fetch additional platform data ("fetch/input")
2. `internal/core_processor` – provider-agnostic processing logic ("process")
3. `internal/provider_output` – anything that posts data back to providers ("post/output")

Every change must end with a successful `make build` from the repo root. If a step fails, stop, revert that step, and adjust before proceeding. Each step group below is intentionally small to avoid the circular dependency traps we hit previously.

## Current State Analysis (ACTUAL STATE)
- ❌ All refactor changes have been reverted
- ❌ `internal/core_processor/` folder exists but is EMPTY
- ❌ `internal/provider_input/` subfolders exist but are EMPTY  
- ❌ ALL files are currently in `internal/api/` - this is our starting point
- ❌ No architectural separation exists yet - this is Phase 1's job

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

### Step 2.2: Move `unified_types.go`
- Physically move `unified_types.go` to `internal/core_processor/` and switch its package declaration to `core_processor`.  
- Update imports in dependent files from `internal/api` to `internal/core_processor`.  
- Remove any temporary type aliases once all references compile.  
- `make build`

### Step 2.3: Move `unified_context_v2.go`
- Relocate the file, adjust package, and point its imports at the new `core_processor` types.  
- Update all call sites (Go compiler ensures completeness).  
- `make build`

### Step 2.4: Move `unified_processor_v2.go` and test file
- Move both production and test files.  
- Update imports in the orchestrator, registry, or any other users.  
- `make build`

### Step 2.5: Delete temporary bridge aliases (if any remain)
- Once everything compiles, drop the bridging file(s).  
- `make build`

---

## Step Group 3: Stabilize `api` → `core_processor` Dependencies
**Goal**: Ensure all references point from `api` to `core_processor` with no cycles.  
**Risk**: 🟡 **LOW** – compiler guarantees catch missing changes.  
**Verification**: `make build`

- Use `go list -deps` or `go list ./...` to confirm there is no reverse import from `core_processor` back into `internal/api`.  
- Update any lingering helper references (e.g., logging wrappers) so `core_processor` depends only on standard library + `internal` packages that do **not** pull `api` back in.  
- `make build`

---

## Step Group 4: Provider Input (Fetch) Extraction
**Goal**: Relocate provider detection + fetch code into independent packages without yet touching posting logic.  
**Risk**: 🟡 **LOW** – large files but mechanical moves.  
**Verification**: `make build` after **each provider**.

### Step 4.1: Introduce provider-specific packages
- Create `internal/provider_input/github`, `.../gitlab`, `.../bitbucket` packages with new package names.  
- Start with **single provider at a time** (GitHub → GitLab → Bitbucket).  
- For each provider: move `*_profile.go` first (pure helpers), adjust imports, run `make build`.  
- Then move the corresponding `*_provider_v2.go`, adjust imports, run `make build` again.

### Step 4.2: Update registry wiring after each provider move
- Update `internal/api/webhook_registry_v2.go` (and other orchestrators) to import the new packages.  
- Ensure the new packages depend only on `core_processor` (and shared utilities), not `internal/api`.  
- `make build`

---

## Step Group 5: Provider Output Separation (Post)
**Goal**: Extract the posting logic into `internal/provider_output/<provider>` packages.  
**Risk**: 🟡 **LOW** – but watch for shared helpers.  
**Verification**: `make build` per provider.

- For each provider, move functions that perform HTTP POSTs/API calls into a new `provider_output` package.  
- Wire the provider input package to depend on provider output via interfaces (defined in `internal/api/webhook_interfaces.go` or a new shared location) to avoid import cycles.  
- Update orchestrator/registry to construct both input + output components.  
- `make build`

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
- [ ] `internal/core_processor` exposes only provider-agnostic logic and imports no provider-specific packages.
- [ ] `internal/provider_input/<provider>` imports `internal/core_processor` but not `internal/api`.
- [ ] `internal/provider_output/<provider>` handles provider posting without referencing `internal/api`.
- [ ] No circular dependency cycles per `go list ./...`.

### ✅ Build Stability
- [ ] `make build` passes after each bullet above.  
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
