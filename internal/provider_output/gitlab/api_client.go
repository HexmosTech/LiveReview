package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	coreprocessor "github.com/livereview/internal/core_processor"
)

type (
	UnifiedWebhookEventV2 = coreprocessor.UnifiedWebhookEventV2
)

// APIClient posts outbound requests back to GitLab.
type APIClient struct {
	httpClient *http.Client
}

// NewAPIClient creates a GitLab output client with sane defaults.
func NewAPIClient() *APIClient {
	return &APIClient{httpClient: &http.Client{Timeout: 60 * time.Second}}
}

// PostCommentReply posts a reply or general note depending on the event context.
func (c *APIClient) PostCommentReply(event *UnifiedWebhookEventV2, accessToken, gitlabInstanceURL, content string) error {
	if event == nil || event.MergeRequest == nil {
		return fmt.Errorf("merge request required for comment reply")
	}
	if content == "" {
		return fmt.Errorf("comment reply content cannot be empty")
	}

	projectID, err := strconv.Atoi(event.Repository.ID)
	if err != nil {
		return fmt.Errorf("invalid project id: %w", err)
	}
	mrIID := event.MergeRequest.Number

	if event.Comment != nil && event.Comment.DiscussionID != nil && *event.Comment.DiscussionID != "" {
		return c.postReplyToDiscussion(context.Background(), accessToken, projectID, mrIID, *event.Comment.DiscussionID, content, gitlabInstanceURL)
	}
	return c.postGeneralComment(context.Background(), accessToken, projectID, mrIID, content, gitlabInstanceURL)
}

// PostEmojiReaction posts an emoji reaction to the target comment.
func (c *APIClient) PostEmojiReaction(event *UnifiedWebhookEventV2, accessToken, gitlabInstanceURL, emoji string) error {
	if event == nil || event.Comment == nil {
		return fmt.Errorf("comment event required for emoji reaction")
	}
	if emoji == "" {
		return fmt.Errorf("emoji value cannot be empty")
	}

	projectID, err := strconv.Atoi(event.Repository.ID)
	if err != nil {
		return fmt.Errorf("invalid project id: %w", err)
	}
	noteID, err := strconv.Atoi(event.Comment.ID)
	if err != nil {
		return fmt.Errorf("invalid note id: %w", err)
	}

	return c.postEmoji(context.Background(), accessToken, projectID, noteID, emoji, gitlabInstanceURL)
}

// PostFullReview posts an overall comment for the merge request when provided.
func (c *APIClient) PostFullReview(event *UnifiedWebhookEventV2, accessToken, gitlabInstanceURL, overallComment string) error {
	if event == nil || event.MergeRequest == nil {
		return fmt.Errorf("merge request required for full review")
	}
	if overallComment == "" {
		return nil
	}

	projectID, err := strconv.Atoi(event.Repository.ID)
	if err != nil {
		return fmt.Errorf("invalid project id: %w", err)
	}
	mrIID := event.MergeRequest.Number

	return c.postGeneralComment(context.Background(), accessToken, projectID, mrIID, overallComment, gitlabInstanceURL)
}

func (c *APIClient) postReplyToDiscussion(ctx context.Context, accessToken string, projectID, mrIID int, discussionID, content, gitlabInstanceURL string) error {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions/%s/notes", gitlabInstanceURL, projectID, mrIID, discussionID)
	body := map[string]string{"body": content}
	log.Printf("[DEBUG] Posting reply to GitLab discussion: %s", apiURL)
	return c.postToGitLabAPI(ctx, apiURL, accessToken, body)
}

func (c *APIClient) postGeneralComment(ctx context.Context, accessToken string, projectID, mrIID int, content, gitlabInstanceURL string) error {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/notes", gitlabInstanceURL, projectID, mrIID)
	body := map[string]string{"body": content}
	return c.postToGitLabAPI(ctx, apiURL, accessToken, body)
}

func (c *APIClient) postEmoji(ctx context.Context, accessToken string, projectID, noteID int, emoji, gitlabInstanceURL string) error {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/notes/%d/award_emoji", gitlabInstanceURL, projectID, noteID)
	body := map[string]string{"name": emoji}
	return c.postToGitLabAPI(ctx, apiURL, accessToken, body)
}

func (c *APIClient) postToGitLabAPI(ctx context.Context, apiURL, accessToken string, requestBody interface{}) error {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	requestCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] GitLab API request failed: %v", err)
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[INFO] Successfully posted to GitLab API: %s", apiURL)
	return nil
}
