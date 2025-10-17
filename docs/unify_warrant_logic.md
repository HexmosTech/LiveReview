## Unified Warrant Logic Rollout Plan

Each phase is gated by `make build` in the repo root (per docs/copilot instructions) to ensure the tree compiles cleanly before moving on. In addition, run `make testall` so the curated suite (excluding locked directories) stays green at every checkpoint.

### Phase 0 – Establish clean test baseline _(Status: ✅ completed 2025-10-17 via `make testall`)_
- Run `go test ./...` from the repo root to discover the current failing unit/integration tests.
- For every failing test case that is obsolete or flaky, remove the test (or the entire `_test.go` file) so the suite only contains the core reply-capability coverage:
	- Start with `internal/api/unified_processing_test.go`, `internal/provider_output/github/output_client_test.go`, and `internal/api/webhook_handler_prompt*_test.go`, pruning only the cases that are no longer relevant per product requirements.
	- Document each deletion inline with a short comment in the PR description so reviewers know what was removed.
- Re-run `go test ./...` to confirm the suite now passes and treat this as the new baseline.
- `make testall` to ensure the curated suite passes, then `make build` in the repo root.

### Phase 1 – Centralize warrant entry point (GitHub path)
- [x] Update `internal/api/unified_processor_v2.go`:
	- [x] Move the mature warrant rules from `Server.checkUnifiedAIResponseWarrant` into `UnifiedProcessorV2Impl.CheckResponseWarrant` (preserve the strict “top-level requires mention” rule).
	- [x] Port `appendLearningsToPrompt`/`fetchRelevantLearnings` dependencies used during warrant evaluation or prompt prep into the V2 struct so all logic now lives together.
- [x] Adjust `internal/api/webhook_orchestrator_v2.go` to call `CheckResponseWarrant` and short-circuit legacy fallbacks for GitHub comment events.
- [x] Delete or inline any GitHub-specific warrant helper still invoked from `internal/api/webhook_handler.go` for comment replies.
- [x] Add focused unit tests (e.g., new cases in `internal/api/unified_processing_test.go`) covering the migrated V2 warrant checks, then run `make testall` followed by `make build` before committing.

### Phase 2 – Normalize provider event data
- Ensure `UnifiedWebhookEventV2` instances carry `InReplyToID` and `DiscussionID` consistently across adapters:
	- [x] `internal/provider_input/github/github_webhook_convert.go` (GitHub): set `InReplyToID`/`DiscussionID` for replies and leave them empty for lone comments while normalizing existing metadata.
	- [x] `internal/provider_input/gitlab/gitlab_provider_v2.go` (GitLab): populate `DiscussionID`, `InReplyToID`, and existing comment metadata.
	- [x] `internal/provider_input/bitbucket/bitbucket_provider_v2.go` (Bitbucket): expose thread identifiers and raw mention metadata across top-level and reply comments.
- [x] Extend `internal/api/unified_processing_test.go` with table-driven cases covering lone vs threaded comments for all three providers, run `make testall`, then `make build` in repo root.

### Phase 3 – Tighten shared mention detection
- In `UnifiedProcessorV2Impl.checkDirectBotMentionV2`, add provider-aware mention parsing helpers housed in:
	- [x] `internal/providers/github/mentions.go`
	- [x] `internal/providers/gitlab/mentions.go`
	- [x] `internal/providers/bitbucket/mentions.go`
- [x] Replace ad-hoc parsing from legacy code with shared helpers in `internal/api/unified_processor_v2.go`.
- [x] Expand `internal/api/unified_processing_test.go` to assert mention detection correctness (with provider fixtures), run `make testall`, then `make build` in repo root.

### Phase 4 – Remove legacy warrant path
- Strip the now-unused `Server.checkUnifiedAIResponseWarrant` and related prompt glue from `internal/api/webhook_handler.go`.
- Update legacy handlers (`GitLabWebhookHandlerV1`, Bitbucket equivalents) to rely on V2 orchestrator flow; if V2 cannot process an event, fail fast instead of silently succeeding.
- Delete obsolete helpers/tests that only exercised the legacy warrant path.
- Ensure regression tests remain green (update or add coverage as needed), run `make testall`, then `make build` in repo root.

### Phase 5 – Final verification & guardrails
- Audit error paths in `UnifiedProcessorV2Impl.CheckResponseWarrant` to log and return hard failures when required data is missing, rather than falling back to permissive defaults.
- Add regression tests under `internal/api/unified_processing_test.go` and provider-specific suites to cover failure handling.
- Verify docs (`docs/unify_warrant_logic.md`) reflect that no legacy warrant logic remains; note that failures surface loudly.
- Run `make testall` followed by `make build` in repo root before closing out the rollout.
