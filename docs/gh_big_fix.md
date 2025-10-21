# GitHub Learning Flow Fix Follow-Up

## Current Architecture Snapshot
- **Unified pipeline (V2)** — `internal/api/webhook_orchestrator_v2.go` drives replies via `UnifiedProcessorV2` and `LearningProcessorV2`, adds acknowledgments, and posts through provider adapters.
- **Legacy GitHub path** — `internal/providers/github/github.go` (and related review service code) still posts comments directly, bypassing the orchestrator abstractions; we only patched acknowledgments locally.
- **Provider input split** — GitHub has `internal/provider_input/github/github_provider.go` for the newer webhook plumbing while older review triggers still lean on `internal/review/service.go` + legacy provider.
- **Learning metadata sources** — `LearningProcessorV2` writes rich metadata (org, repo, source_context) but only V2 orchestrator consumed it end‑to‑end before the stopgap.
- **Tests / fixtures** — Coverage is skewed toward V2; legacy flow relies on manual QA and integration tests that pre-date the learning block work.

## Pain Points Identified
- **Dual posting pipelines**: Reply flows and triggered reviews can hit different stacks (legacy provider vs. unified orchestrator), duplicating logic for formatting, retries, learning extraction, and error handling.
- **Formatter forks**: Until now each flow hand‑rolled the learning block; other concerns (prompt hygiene, context trimming, citation handling) risk similar divergence.
- **Mixed data plumbing**: Legacy path lacks canonical source_context metadata (e.g., repo URL) unless we keep shimming it in, making features like per-repo learnings harder.
- **Hard to reason about warrants**: UnifiedProcessorV2 encodes warrant policy, but legacy review triggers dodge it entirely, so responding rules differ between auto replies and manual triggers.
- **Testing gaps**: Unit tests assert the V2 acknowledgment, but no regression suite covers the legacy provider, meaning every fix is a bespoke patch.
- **Operational confusion**: Engineers need to know which switches (e.g., Make targets, environment flags) route traffic through which path; documentation is thin.

## Recommended Resolution Strategy
1. **Collapse onto UnifiedPipeline for GitHub**
	- Swap legacy review posting in `internal/review/service.go` to invoke the V2 orchestrator/provider adapter instead of `internal/providers/github` directly.
	- Ensure `UnifiedProcessorV2` exposes a simple entrypoint for both webhook replies and manual review triggers (shared code for warrant checks, learning extraction).
2. **Deprecate legacy `internal/providers/github`**
	- After routing is unified, migrate remaining consumers (CLI tools, tests) and delete the file to prevent backslide.
	- Retire duplicate config structs (`review.ProviderConfig` vs integration token plumbing) so GitHub setup lives in one place.
3. **Centralize formatting & metadata helpers**
	- Keep the new `internal/learnings/acknowledgment` package as the single markdown renderer.
	- Add shared helpers for stripping learning JSON, context summarization, and prompt annotations to avoid future drift.
4. **Strengthen tests across flows**
	- Add end‑to‑end tests that simulate a GitHub webhook and a manual review trigger, confirming they run through the same code and emit identical acknowledgment blocks.
	- Capture fixture differences (real vs stub payloads) in `internal/api/tests` so both paths share the same data.
5. **Document the flow**
	- Update developer docs describing the request journey (webhook → unified orchestrator → provider adapter) with callouts for learning persistence and acknowledgment.
	- Include migration notes while the legacy provider is still around so ops knows which flags toggle the unified flow.

## Suggested Next Steps (Incremental)
- Draft an ADR clarifying that GitHub must use UnifiedProcessorV2 for all comment generation.
- Build a feature flag (temporary) in review service to switch between legacy and unified posting; run controlled rollout to ensure no regression in comment placement.
- Track cleanup tasks in a single ticket (or this doc) and close out once legacy provider files are deleted and docs/tests reflect the new single path.
