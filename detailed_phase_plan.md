# LiveReview Phase 1: Detailed Step-by-Step Implementation Plan

## Overview
Phase 1 focuses on **Architectural Folder Restructuring** to achieve clean separation of concerns and avoid circular imports. Each step group ensures `make build` continues to work, providing an incremental, safe refactoring path.

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

## Step Group 1: Current State Analysis and Build Verification
**Goal**: Understand what files need to be moved and verify current build state  
**Risk**: 🟢 **ZERO** - Read-only analysis  
**Verification**: `make build` (should work unchanged)

### Step 1.1: Verify Current Build State
```bash
make build
# Should succeed - this is our baseline
```

### Step 1.2: Analyze Files That Need to Move (ACTUAL ANALYSIS)
**Current files confirmed to exist in `internal/api/`**:

**Core Processing Files (→ internal/core_processor/)**:
- ✅ `unified_processor_v2.go` (26,686 bytes) - Main unified processing logic  
- ✅ `unified_context_v2.go` (14,159 bytes) - Context building for processing
- ✅ `unified_types.go` (5,729 bytes) - Unified data types
- ✅ `unified_processing_test.go` (13,074 bytes) - Core processing tests

**Provider Files (→ internal/provider_input/)**:
- ✅ `github_provider_v2.go` (41,050 bytes) → `internal/provider_input/github/`
- ✅ `gitlab_provider_v2.go` (65,833 bytes) → `internal/provider_input/gitlab/`  
- ✅ `bitbucket_provider_v2.go` (24,286 bytes) → `internal/provider_input/bitbucket/`
- ✅ `github_profile.go` (1,828 bytes) → `internal/provider_input/github/`
- ✅ `gitlab_profile.go` (1,742 bytes) → `internal/provider_input/gitlab/`
- ✅ `gitlab_auth.go` (16,634 bytes) → `internal/provider_input/gitlab/`
- ✅ `bitbucket_profile.go` (4,034 bytes) → `internal/provider_input/bitbucket/`

**Additional Core Files That Should Stay in API**:
- `webhook_interfaces.go` - Interface definitions for orchestration
- `learning_processor_v2.go` - Learning extraction (stays in API layer)
- `webhook_orchestrator_v2.go` - Flow orchestration (stays in API layer)  
- `webhook_registry_v2.go` - Provider registry (stays in API layer)

### Step 1.3: Identify Import Dependencies (ACTUAL RESULTS)
**Current internal/api imports found: 9 total imports in 7 files**

**Files that currently import from internal/api**:
- `internal/api/users/handlers.go`
- `internal/api/users/profile_handlers.go`  
- `internal/api/prompts.go`
- `internal/api/organizations/org_handlers.go`
- `internal/api/tests/connectors_test.go`
- `internal/api/server.go`
- `internal/api/handlers/prompts.go`

**Risk Assessment**: Only 7 files have internal/api imports, making this refactor low-risk since most dependencies are contained within the api package itself.

---

## Step Group 2: Move Core Processing Files 
**Goal**: Move unified processing files to core_processor package  
**Risk**: � **LOW** - File movement with package/import updates  
**Verification**: `make build` after each file move

### Step 2.1: Move Unified Types (Foundation)
Move types first since everything depends on them:
```bash
# Move the file
mv internal/api/unified_types.go internal/core_processor/

# Update package declaration
sed -i 's/^package api$/package core_processor/' internal/core_processor/unified_types.go
```

**Update imports in moved file**:
- Remove any `github.com/livereview/internal/api` self-imports
- Keep external imports unchanged

**Verification**:
```bash
make build
# May fail due to other files still importing from api - that's expected
```

### Step 2.2: Move Unified Context Builder
```bash
mv internal/api/unified_context_v2.go internal/core_processor/
sed -i 's/^package api$/package core_processor/' internal/core_processor/unified_context_v2.go
```

**Update imports in moved file**:
- Change `"github.com/livereview/internal/api"` to `"github.com/livereview/internal/core_processor"` if needed

### Step 2.3: Move Unified Processor
```bash
mv internal/api/unified_processor_v2.go internal/core_processor/
sed -i 's/^package api$/package core_processor/' internal/core_processor/unified_processor_v2.go
```

### Step 2.4: Move Core Processing Tests
```bash
mv internal/api/unified_processing_test.go internal/core_processor/
sed -i 's/^package api$/package core_processor/' internal/core_processor/unified_processing_test.go
```

**Verification**:
```bash
make build
# Will likely fail - need to update imports next
```

---

## Step Group 3: Update Imports for Core Processor Files
**Goal**: Fix all import statements to use core_processor package  
**Risk**: 🟡 **LOW** - Compile-time safety catches all issues  
**Verification**: `make build` succeeds

### Step 3.1: Update Files That Import Core Processing Types
Find and update all files that import core processing components:
```bash
# Find files that need import updates
grep -r "github.com/livereview/internal/api" internal/ --include="*.go" -l
```

**For each file found, update imports**:
```go
// OLD import (remove or update):
"github.com/livereview/internal/api"

// NEW import (add if needed):
"github.com/livereview/internal/core_processor"
```

**Key files that will need updates**:
- `internal/api/webhook_orchestrator_v2.go`
- `internal/api/learning_processor_v2.go`
- `internal/api/webhook_registry_v2.go`
- `internal/api/server.go`
- Any other files using `UnifiedWebhookEventV2`, `UnifiedProcessorV2`, etc.

### Step 3.2: Update Type References
For each file updated in 3.1, change type references:
```go
// OLD type references:
api.UnifiedWebhookEventV2
api.UnifiedProcessorV2
api.UnifiedContextBuilderV2

// NEW type references:
core_processor.UnifiedWebhookEventV2
core_processor.UnifiedProcessorV2
core_processor.UnifiedContextBuilderV2
```

**Verification**:
```bash
make build
# Should succeed with core_processor files properly imported
```

---

## Step Group 4: Move Provider Files to provider_input
**Goal**: Move provider-specific files to their own packages  
**Risk**: 🟡 **LOW** - File movement with package/import updates  
**Verification**: `make build` after each provider

### Step 4.1: Move GitHub Provider Files
```bash
# Move GitHub provider files
mv internal/api/github_provider_v2.go internal/provider_input/github/
mv internal/api/github_profile.go internal/provider_input/github/

# Update package declarations
sed -i 's/^package api$/package github/' internal/provider_input/github/github_provider_v2.go
sed -i 's/^package api$/package github/' internal/provider_input/github/github_profile.go
```

**Update imports in moved files**:
- Change `"github.com/livereview/internal/api"` to `"github.com/livereview/internal/core_processor"` for core types
- Keep other imports unchanged

### Step 4.2: Move GitLab Provider Files  
```bash
# Move GitLab provider files
mv internal/api/gitlab_provider_v2.go internal/provider_input/gitlab/
mv internal/api/gitlab_profile.go internal/provider_input/gitlab/
mv internal/api/gitlab_auth.go internal/provider_input/gitlab/

# Update package declarations
sed -i 's/^package api$/package gitlab/' internal/provider_input/gitlab/gitlab_provider_v2.go
sed -i 's/^package api$/package gitlab/' internal/provider_input/gitlab/gitlab_profile.go
sed -i 's/^package api$/package gitlab/' internal/provider_input/gitlab/gitlab_auth.go
```

### Step 4.3: Move Bitbucket Provider Files
```bash
# Move Bitbucket provider files
mv internal/api/bitbucket_provider_v2.go internal/provider_input/bitbucket/
mv internal/api/bitbucket_profile.go internal/provider_input/bitbucket/

# Update package declarations  
sed -i 's/^package api$/package bitbucket/' internal/provider_input/bitbucket/bitbucket_provider_v2.go
sed -i 's/^package api$/package bitbucket/' internal/provider_input/bitbucket/bitbucket_profile.go
```

**Verification**:
```bash
make build
# Will likely fail - need to update imports next
```

---

## Step Group 5: Update Imports for Provider Files
**Goal**: Fix all import statements to use new provider packages  
**Risk**: 🟡 **LOW** - Compile-time safety catches all issues  
**Verification**: `make build` succeeds

### Step 5.1: Update API Files That Import Providers
Find and update files that import provider components:

**Files to update**:
- `internal/api/webhook_registry_v2.go`
- `internal/api/webhook_orchestrator_v2.go`
- `internal/api/server.go`

**For each file, update imports**:
```go
// OLD imports (remove these):
// No specific provider imports from api package

// NEW imports (add these):
"github.com/livereview/internal/provider_input/github"
"github.com/livereview/internal/provider_input/gitlab"
"github.com/livereview/internal/provider_input/bitbucket"
```

### Step 5.2: Update Provider Constructor Calls
In files like `webhook_registry_v2.go`, update provider instantiation:
```go
// OLD constructor calls:
NewGitHubV2Provider(server)
NewGitLabV2Provider(server)  
NewBitbucketV2Provider(server)

// NEW constructor calls:
github.NewGitHubV2Provider(server)
gitlab.NewGitLabV2Provider(server)
bitbucket.NewBitbucketV2Provider(server)
```

### Step 5.3: Update Type References
Update any direct references to provider types:
```go
// OLD type references:
*GitHubV2Provider
*GitLabV2Provider
*BitbucketV2Provider

// NEW type references:
*github.GitHubV2Provider
*gitlab.GitLabV2Provider  
*bitbucket.BitbucketV2Provider
```

**Verification**:
```bash
make build
# Should succeed with all provider files properly imported
```

---

## Step Group 6: Final Integration and Testing
**Goal**: Ensure everything works together and no circular imports exist  
**Risk**: 🟡 **LOW** - Final integration testing  
**Verification**: `make build` + `make test` + functionality verification

### Step 6.1: Update Remaining Import References
Search for any remaining references to old import paths:
```bash
# Find any remaining api imports for moved files
grep -r "github.com/livereview/internal/api" internal/ --include="*.go" | grep -E "(Unified|GitHub|GitLab|Bitbucket)" || echo "No remaining imports found"
```

Fix any found imports following the patterns from previous steps.

### Step 6.2: Verify No Circular Imports
```bash
# Check for circular import issues
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./internal/... | grep -E "(core_processor|provider_input)" || echo "No circular imports found"
```

### Step 6.3: Run Full Build and Test Suite
```bash
make build
# Should succeed with new architecture

make test
# Should pass all tests
```

### Step 6.4: Test Key Functionality
```bash
# Test that the binary runs
./livereview --help

# Test basic API functionality (if possible)
./livereview api --help
```

---

## Step Group 7: Documentation and Verification
**Goal**: Document new structure and verify everything works  
**Risk**: 🟢 **ZERO** - Documentation and verification only  
**Verification**: Complete Phase 1 success criteria

### Step 7.1: Verify Final File Structure
Confirm the target architecture is achieved:
```bash
# Check that files are in correct locations
ls internal/core_processor/
ls internal/provider_input/github/
ls internal/provider_input/gitlab/
ls internal/provider_input/bitbucket/
```

Expected output:
```
internal/core_processor/:
unified_processor_v2.go
unified_context_v2.go  
unified_types.go
unified_processing_test.go

internal/provider_input/github/:
github_provider_v2.go
github_profile.go

internal/provider_input/gitlab/:
gitlab_provider_v2.go
gitlab_profile.go
gitlab_auth.go

internal/provider_input/bitbucket/:
bitbucket_provider_v2.go
bitbucket_profile.go
```

### Step 7.2: Update Documentation
Update `internal/api/README.md` to reflect new structure:
```markdown
# API Package Structure

This package contains the HTTP API layer for LiveReview. It orchestrates
requests between the core processing engine and provider input layer.

## Dependencies:
- `internal/core_processor/` - Core processing logic (zero platform awareness)
- `internal/provider_input/` - Provider-specific input handling
- External HTTP framework (echo)

## Responsibilities:
- HTTP endpoint definitions
- Request/response serialization  
- Flow orchestration between layers
- Authentication and authorization
```

### Step 7.3: Final Build and Functionality Verification
```bash
make build
./livereview --version
# Should build and run successfully with new architecture
```

---

## Phase 1 Success Criteria

### ✅ Architecture Compliance
- [ ] Core processing has zero platform-specific imports
- [ ] Provider logic is contained within provider folders
- [ ] No circular imports exist between packages
- [ ] Clear separation of concerns maintained

### ✅ Build Stability
- [ ] `make build` succeeds at every step
- [ ] No compile-time errors or warnings
- [ ] All tests pass with new structure
- [ ] Integration tests work with restructured code

### ✅ Functionality Preservation  
- [ ] All webhook processing continues to work
- [ ] Provider detection and routing functions correctly
- [ ] Core processing generates responses as before
- [ ] Profile management APIs remain functional

### ✅ Maintainability Improvements
- [ ] Clear folder structure makes adding new providers easy
- [ ] Interface boundaries are well-defined and documented
- [ ] Dependencies flow in one direction (no cycles)
- [ ] Code is easier to understand and modify

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
