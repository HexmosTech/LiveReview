# GitHub Learning Flow Fix Follow-Up

## Scope Clarification
This note focuses **only on reply flow (flow #2)**—the path that handles GitHub webhooks (issue comments, review comments) and produces AI replies. The code that powers full review triggers (flow #1) lives under `internal/review` and is tracked separately.

At its simplest the reply pipeline should be:

```
github_input → core_processor → github_output
```

- **github_input**: webhook ingestion + provider-specific parsing (`internal/provider_input/github`).
- **core_processor**: warrant checks, prompt orchestration, learning application (`internal/api/webhook_orchestrator_v2.go`, `UnifiedProcessorV2`, `LearningProcessorV2`).
- **github_output**: posting replies back to GitHub (`internal/provider_output/github`).
	- The concrete poster is `internal/provider_output/github/output_client.go` (APIClient), which should be the only GitHub-specific HTTP surface we call from reply logic.

Any path that deviates from this straight-through pipeline is legacy debt we need to retire.

## Current Reply Architecture Snapshot
- **Unified reply pipeline (production)** — `internal/api/webhook_orchestrator_v2.go` and the provider registry route every GitHub webhook reply through `UnifiedProcessorV2`, apply learnings with `LearningProcessorV2`, and post via `internal/provider_input/github/github_provider.go` + `internal/provider_output/github/output_client.go`.
- **Legacy manual-review pipeline** — The review service in `internal/review/service.go` and CLI tooling still create comments by calling `internal/providers/github/github.go` directly, outside the unified orchestrator abstractions. Recent changes only added acknowledgment rendering here; warrant checks and posting remain bespoke.
- **Learning metadata sources** — `LearningProcessorV2` produces rich metadata consumed by the unified reply flow; the manual-review path still needs shims to keep metadata aligned.
- **Tests / fixtures** — Unified replies have focused tests; manual-review posting does not, so regressions land unnoticed unless we explicitly exercise that code.

### Manual Review Trigger Path (legacy)
1. Review workloads call directly into `internal/providers/github/github.go` to post summary and inline comments, formatting content locally.
2. Learning acknowledgments are appended by helper code immediately before posting.
3. This path predates `UnifiedProcessorV2`; it skips warrant logic, duplicate prompt handling, and the unified output client.

### Unified Reply Path (active path)
1. GitHub webhook hits the generic handler and is routed by `internal/api/webhook_orchestrator_v2.go` through the provider registry.
2. The orchestrator invokes `UnifiedProcessorV2` for warrant checks, prompt construction, and reply generation.
3. Learnings are applied through `LearningProcessorV2` and acknowledgments rendered via `internal/learnings/acknowledgment`.
4. Replies are posted using the V2 provider adapter `internal/provider_input/github/github_provider.go`, which delegates to `internal/provider_output/github/output_client.go`.

Today only the unified reply path handles GitHub webhook responses. The remaining operational complexity comes from the manual-review tooling still running on the legacy provider stack.

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

## Ideal End State (target flow)
1. **All GitHub comment generation**—whether webhook driven or manual trigger—calls into `UnifiedProcessorV2` for warranting, prompting, and learning capture.
2. `LearningProcessorV2` remains the single persistence layer, and `internal/learnings/acknowledgment` renders acknowledgments everywhere.
3. Posting happens exclusively through the V2 provider adapter (`internal/provider_input/github/github_provider.go`), which understands threads, diffs, and auth tokens.
4. Configuration lives in one place: integration tokens feed both orchestrated and manual flows without duplicate PAT handling code.
5. Tests cover the unified path end-to-end, with fixtures shared across webhook + manual scenarios so regressions surface immediately.
