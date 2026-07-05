package azuredevops

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	coreprocessor "github.com/livereview/internal/core_processor"
	azuredevopsutils "github.com/livereview/internal/providers/azuredevops"
	"github.com/livereview/pkg/models"
)

type (
	UnifiedWebhookEventV2  = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2  = coreprocessor.UnifiedMergeRequestV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
)

// APIClient posts outbound Azure DevOps content on behalf of the provider.
type APIClient struct{}

// NewAPIClient constructs an Azure DevOps output client.
func NewAPIClient() *APIClient {
	return &APIClient{}
}

func newProvider(orgURL, token string) (*azuredevopsutils.Provider, error) {
	return azuredevopsutils.NewProvider(azuredevopsutils.Config{BaseURL: orgURL, Token: token})
}

// buildMRID reconstructs the "org/project/repo/id" composite id used by the
// Azure DevOps provider package, from event/mr metadata populated during
// FetchMergeRequestData.
func buildMRID(orgURL, repoFullName string, prNumber int) (string, error) {
	org, err := azuredevopsutils.OrgNameFromURL(orgURL)
	if err != nil {
		return "", fmt.Errorf("failed to derive org name from url %q: %w", orgURL, err)
	}
	parts := strings.SplitN(repoFullName, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid Azure DevOps repository full name: %s", repoFullName)
	}
	return fmt.Sprintf("%s/%s/%s/%d", org, parts[0], parts[1], prNumber), nil
}

// PostCommentReply posts a reply within the thread that the triggering comment belongs to.
func (c *APIClient) PostCommentReply(event *UnifiedWebhookEventV2, token, replyText string) error {
	if event == nil || event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	orgURL, ok := event.MergeRequest.Metadata["base_url"].(string)
	if !ok || orgURL == "" {
		return fmt.Errorf("base_url not found in merge request metadata")
	}

	threadID, ok := extractThreadIDFromMetadata(event.Comment.Metadata)
	if !ok {
		return fmt.Errorf("thread_id not found in comment metadata; cannot route reply")
	}

	parentCommentID, err := strconv.ParseInt(event.Comment.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid comment id %q: %w", event.Comment.ID, err)
	}

	mrID, err := buildMRID(orgURL, event.Repository.FullName, event.MergeRequest.Number)
	if err != nil {
		return err
	}

	provider, err := newProvider(orgURL, token)
	if err != nil {
		return fmt.Errorf("failed to construct azure devops provider: %w", err)
	}

	log.Printf("[DEBUG] AzureDevOps APIClient.PostCommentReply: thread_id=%d, parent_comment_id=%d, reply_len=%d",
		threadID, parentCommentID, len(replyText))

	return provider.PostThreadReply(context.Background(), mrID, threadID, parentCommentID, replyText)
}

// PostEmojiReaction is a no-op: Azure DevOps has no reaction API on PR thread
// comments, so we avoid polluting threads with a fallback text comment.
func (c *APIClient) PostEmojiReaction(event *UnifiedWebhookEventV2, token, reaction string) error {
	log.Printf("[DEBUG] Azure DevOps has no comment reaction API; skipping reaction %q", reaction)
	return nil
}

// PostReviewComments posts the structured review comments collected during full-review processing.
func (c *APIClient) PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error {
	if len(comments) == 0 {
		return nil
	}

	orgURL, ok := mr.Metadata["base_url"].(string)
	if !ok || orgURL == "" {
		return fmt.Errorf("base_url not found in merge request metadata")
	}
	repoFullName, ok := mr.Metadata["repository_full_name"].(string)
	if !ok || repoFullName == "" {
		return fmt.Errorf("repository_full_name not found in metadata")
	}

	mrID, err := buildMRID(orgURL, repoFullName, mr.Number)
	if err != nil {
		return err
	}

	provider, err := newProvider(orgURL, token)
	if err != nil {
		return fmt.Errorf("failed to construct azure devops provider: %w", err)
	}

	ctx := context.Background()
	for _, comment := range comments {
		reviewComment := &models.ReviewComment{
			FilePath:      comment.FilePath,
			Line:          comment.LineNumber,
			Content:       comment.Content,
			Severity:      models.CommentSeverity(comment.Severity),
			Confidence:    comment.Confidence,
			Type:          comment.Type,
			Category:      comment.Category,
			Subcategory:   comment.Subcategory,
			IsDeletedLine: comment.Position != nil && comment.Position.LineType == "old",
		}
		if err := provider.PostComment(ctx, mrID, reviewComment); err != nil {
			return fmt.Errorf("failed to post review comment: %w", err)
		}
	}

	return nil
}

func extractThreadIDFromMetadata(metadata map[string]any) (int, bool) {
	if metadata == nil {
		return 0, false
	}
	switch v := metadata["thread_id"].(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}
