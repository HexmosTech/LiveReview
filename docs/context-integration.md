## Reply Context Integration Plan

### We need to do MR modeling

Gitlab Sample MR: https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426

Github Sample MR: https://github.com/livereviewbot/glabmig/pull/2

Bitbucket Sample MR: https://bitbucket.org/contorted/fb_backends/pull-requests/1



1. Rewire `webhook_orchestrator_v2.go` to build the full review timeline and comment tree using the existing `internal/reviewmodel` helpers so every reply gets commit, diff, and thread context.
2. Replace the current plain-text reply prompt in `unified_processor_v2.go` with the historical XML+text scaffold (from the `mrmodel` tool) while keeping the Phase 7 learning instructions intact.
3. Ensure `buildCommentReplyPromptWithLearning` appends the learning section after the new XML context and still routes through `buildContextualResponseWithLearningV2`.
4. Update the prompt-focused unit tests in `internal/api/unified_processing_test.go` to assert the new structure and run `go test ./internal/api` to confirm everything stays green.
