# Context Restoration Plan

## Primary Goals

- Re-enable the rich merge-request context pipeline (timeline, threaded comments, before/after diff snapshots, code excerpts) that existed prior to the recent refactors.
- Restore "learning" features from the historical implementation so that the system can surface past decisions, prior guidance, and resolved conversations.

## Work Items

1. **Recover historical reviewmodel package**
	- Reintroduce `internal/reviewmodel/model.go` and `internal/reviewmodel/builders.go` from commit `5cdb560a390ab1d34a3932765cff98627905c7f0` (or adjacent commits with identical logic).
	- Ensure the types (`TimelineItem`, `TimelineComment`, `CommentNode`, `ExportTimelineItem`, etc.) compile cleanly against the current provider layer.
	- Verify build without unused dependencies; update module imports if directory layout shifted since the refactor.

2. **Port timeline + comment tree orchestration**
	- Mirror the orchestration code from `cmd/mrmodel/main.go` in that same commit:
	  - Fetch MR commits and discussions via the provider interface.
	  - Call `BuildTimeline`, `BuildCommentTree`, and `BuildPrevCommitIndex` to assemble chronological context.
	  - Generate before/after commit summaries, thread slices, focused diffs, and code excerpts (logic around lines 300–520 in the legacy file).
	- Embed this pipeline inside the modern webhook orchestrator / responder flow so every auto-reply uses the restored context.
	- Add diagnostics (e.g., optional artifact dumps) behind a flag to aid debugging.

3. **Reconstruct prompt builders**
	- Bring back `buildGeminiPromptRich` / `buildGeminiPromptEnhanced` (or rename appropriately) so the LLM receives structured sections: target comment, focused diff, before & after context, current code excerpt, and commit history.
	- Update wording and output format instructions to match today’s product tone, but preserve the before/after emphasis and threaded context.

4. **Restore learning features**
	- Audit historical commits for "learning" logic that captured prior clarifications, resolved threads, or reviewer notes (e.g., search older `docs/mr/` guides and any `learning`-prefixed modules).
	- Reintroduce data structures and persistence (if applicable) that track lessons learned, resolution statuses, and follow-up actions.
	- Integrate these learnings into the prompt so the assistant can reference past decisions and avoid repeating resolved feedback.
	- Add a regression harness (unit test or fixture) to confirm learnings persist across runs and surface correctly in replies.

5. **Wire into provider-neutral flow**
	- Abstract provider-specific fetching (GitHub, GitLab) so both can supply commits, discussions, and metadata required by the reviewmodel helpers.
	- Ensure error handling is strict—no silent fallbacks—per current guidelines.

6. **Testing & validation**
	- Craft unit tests for `BuildTimeline`, `BuildCommentTree`, and any new learning-related stores using canned provider fixtures.
	- Add integration tests (or recorded fixtures) that simulate a full MR response, asserting the prompt includes timeline, before/after segments, and referenced learnings.
	- Run targeted tests via `go test ./...` (or the most specific packages) once wiring is complete.

7. **Documentation & follow-up**
	- Update developer docs to describe the restored pipeline, artifact outputs, and how learnings are captured/applied.
	- Create a migration checklist for future refactors so context/learning pieces are not accidentally dropped again.
	- Track any remaining gaps (e.g., provider parity, performance tuning) as follow-up issues.

## Notes

- Historical reference commits: `5cdb560a390ab1d34a3932765cff98627905c7f0`, `dd72c1fd761216916a53637b1d05cf223d4201df`, `7b58418cc8c6abb789da0ce341d50b958e5e0edc` (identical context logic).
- Confirm whether legacy "learning" data lived in dedicated storage or was synthesized on the fly; adapt restoration accordingly.
- Prefer incremental merges (bring back reviewmodel first, then orchestration, then learnings) to keep diffs reviewable.
