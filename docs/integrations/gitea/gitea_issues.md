# Gitea Issues

This doc tracks Gitea integration issues and their implemented solutions.

## Resolved Issues

### 1. Missing Severity Level in Initial Comments
**Issue:** 
When the AI posted code reviews on a Gitea PR, the severity level (e.g., `**Severity: critical**`) was missing from the comment block.
**Solution:** 
The Gitea provider previously posted raw comment content. This was fixed by introducing `formatGiteaComment` in [internal/providers/gitea/gitea_provider.go#L274](internal/providers/gitea/gitea_provider.go#L274). This function standardizes the format to match the GitHub/GitLab providers, ensuring that severity and suggestions are properly injected and sanitized before the comment is posted.

### 2. General Replies Displacing Inline Threading
**Issue:** 
When a user replied to an inline review comment on Gitea, the bot's subsequent reply broke out of the inline discussion and was posted at the very bottom of the PR timeline as a quoted general comment.
**Solution:** 
Gitea webhooks for replies often omit the exact `position` line data while retaining the `review_id`. The reply routing logic in [internal/provider_output/gitea/api_client.go#L59](internal/provider_output/gitea/api_client.go#L59) was updated to aggressively trigger metadata enrichment whenever an inline reply lacks `position`. The [enrichCommentMetadata](internal/provider_output/gitea/api_client.go#L220) function now dynamically fetches the parent comment's line coordinates to ensure the bot responds properly within the inline thread rather than falling back to a general quote block.

### 3. Inline Comments Disregarded as PR Requests
**Issue:** 
Whenever a user tried to comment inline on the code, Gitea would send a webhook that the system incorrectly disregarded as a general PR request, completely ignoring the comment content.
**Solution:** 
This occurred because Gitea assigns the `reviewed` action to `pull_request_review_comment` webhooks when an inline comment is created. The webhook parser ([internal/provider_input/gitea/gitea_conversion.go#L83](internal/provider_input/gitea/gitea_conversion.go#L83)) was strictly filtering out any action other than `created`. The logic was updated to explicitly process `reviewed` actions and intelligently fall back to the `Review` object if the raw `Comment` body was omitted in the payload. The unified processor also handles this special case in [internal/api/unified_processor_v2.go#L88](internal/api/unified_processor_v2.go#L88).
## TODO

### 4. Race Condition Handling for Multiple Bot Comments
**Issue:** 
When livereview posts multiple comments in quick succession in response to a single user comment, the enrichment logic may select an earlier, incomplete comment instead of the most recent and comprehensive one. This can lead to processing outdated or less detailed responses.

**Current Status:** 
- ✅ **Implemented:** Enhanced enrichment logic to collect all suitable comments and prioritize by timestamp
- ✅ **Fixed:** Removed early break after finding first suitable comment  
- ✅ **Added:** Content comparison to handle duplicate comment bodies
- 🔄 **In Progress:** Testing and monitoring for race condition scenarios

**Solution Details:**
1. **Collect all suitable comments** instead of breaking after first match
2. **Prioritize by latest timestamp** using `UpdatedAt` field comparison
3. **Skip duplicate content** using `seenContents` map to prevent race processing
4. **Select most comprehensive** response when multiple comments contain bot mention

**Files Modified:**
- `internal/provider_input/gitea/gitea_provider.go` - Updated enrichment logic in `FetchMergeRequestData`
- `internal/provider_input/gitea/gitea_types.go` - Added `DiffHunk` field to `GiteaReviewComment` struct

**Next Steps:**
- Monitor webhook processing logs for race condition scenarios
- Verify that latest/most comprehensive comment is consistently selected
- Consider additional heuristics if timestamp-based selection proves insufficient