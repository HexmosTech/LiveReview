## Plan: Minimal diff-review for CLI prototype

Create `/api/v1/diff-review` that bypasses auth, accepts base64-encoded diff ZIP in JSON, reuses existing diff parser from [cmd/mrmodel/lib/diff_parser.go](cmd/mrmodel/lib/diff_parser.go), feeds into review pipeline, and returns structured JSON with comments embedded in diffs.

### Steps

1. **Add new public route** in [internal/api/server.go](internal/api/server.go#L443): Insert `v1.POST("/diff-review", s.DiffReview)` after line 443 (`/auth/setup`) in public routes block; handler validates `X-Bypass-Key` header equals `"lr-internal-2024"` before processing, returns 401 if missing/invalid.

2. **Create DiffReview handler** in new file `internal/api/diff_review.go`: Parse JSON `{"diff_zip_base64": "...", "repo_name": "optional"}` → decode base64 → write to temp file → extract ZIP to temp folder → call [lib.LocalParser.Parse()](cmd/mrmodel/lib/diff_parser.go#L18) on extracted diff file → convert `[]LocalCodeDiff` to `[]*models.CodeDiff` (map `LocalDiffHunk` to `models.DiffHunk`, join hunk lines to Content string) → create review record with `org_id=1`, `trigger_type="cli_diff"`, `status="processing"`, `repository=repo_name` → return `{"review_id": "..."}` immediately for async processing.

3. **Add PreloadedChanges field** to [ReviewRequest](internal/review/service.go#L44): Add `PreloadedChanges []*models.CodeDiff` field after line 49 (`AI AIConfig`); modify [executeReviewWorkflow](internal/review/service.go#L283) at line 308 to check `if request.PreloadedChanges != nil` → skip `GetMergeRequestDetails` and `GetMergeRequestChanges` (lines 308-417) → assign `changes = request.PreloadedChanges` → construct stub `mrDetails = &providers.MergeRequestDetails{ID: request.ReviewID, Title: "CLI Diff Review", ProviderType: "cli", URL: ""}` → jump to line 440 AI review.

4. **Skip provider post for cli type** in [ProcessReview](internal/review/service.go#L214): After line 206 AI review completion, check `if request.Provider.Type == "cli"` before calling `postReviewResults` (line 214) → if true, skip lines 214-238 and jump to line 264 success result assignment → still return full `ReviewResult` with comments.

5. **Create status polling endpoint** in `diff_review.go`: Add `GET /api/v1/diff-review/:review_id` (public, bypass key required) → query review record status from DB → if `status="processing"`, return `{"status": "processing", "review_id": "..."}` → if `status="completed"`, fetch `ReviewResult` from DB, build response `{"status": "completed", "review_id": "...", "summary": "...", "files": [...]}` where files array merges diffs with comments: iterate `PreloadedChanges`, for each file create `{"file_path": "...", "hunks": [...original hunks...], "comments": [{"line": N, "content": "...", "severity": "...", "category": "..."}]}` by matching `comment.FilePath == diff.FilePath` and embedding matched comments.

### Further Considerations

1. **LocalCodeDiff conversion**: Write helper `convertLocalToModelDiff(local LocalCodeDiff) *models.CodeDiff` that maps `LocalDiffHunk` fields (OldStartLine, NewStartLine, etc) to `models.DiffHunk`, concatenate `LocalDiffLine.Content` into hunk Content string with proper formatting.
2. **Comment line validation**: When embedding comments, validate `comment.Line` falls within hunk's `NewStartLine` to `NewStartLine+NewLineCount` range; log warning for out-of-range comments but include in response.
3. **Review result persistence**: Store serialized `ReviewResult` (summary + comments JSON) in review metadata column after completion for efficient polling retrieval without recomputing.
