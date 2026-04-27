# Gitea Issues

This doc tracks Gitea integration issues and their implemented solutions.

## Resolved Issues

### 1. Missing Severity Level in Initial Comments
**Issue:** 
When the AI posted code reviews on a Gitea PR, the severity level (e.g., `**Severity: critical**`) was missing from the comment block.
**Solution:** 
The Gitea provider previously posted raw comment content. This was fixed by introducing `formatGiteaComment` in `internal/providers/gitea/gitea_provider.go`. This function standardizes the format to match the GitHub/GitLab providers, ensuring that severity and suggestions are properly injected and sanitized before the comment is posted.

### 2. General Replies Displacing Inline Threading
**Issue:** 
When a user replied to an inline review comment on Gitea, the bot's subsequent reply broke out of the inline discussion and was posted at the very bottom of the PR timeline as a quoted general comment.
**Solution:** 
Gitea webhooks for replies often omit the exact `position` line data while retaining the `review_id`. The reply routing logic in `internal/provider_output/gitea/api_client.go` was updated to aggressively trigger metadata enrichment whenever an inline reply lacks `position`. It now dynamically fetches the parent comment's line coordinates to ensure the bot responds properly within the inline thread rather than falling back to a general quote block.

### 3. Inline Comments Disregarded as PR Requests
**Issue:** 
Whenever a user tried to comment inline on the code, Gitea would send a webhook that the system incorrectly disregarded as a general PR request, completely ignoring the comment content.
**Solution:** 
This occurred because Gitea assigns the `reviewed` action to `pull_request_review_comment` webhooks when an inline comment is created. The webhook parser (`internal/provider_input/gitea/gitea_conversion.go`) was strictly filtering out any action other than `created`. The logic was updated to explicitly process `reviewed` actions and intelligently fall back to the `Review` object if the raw `Comment` body was omitted in the payload.
