package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	coreprocessor "github.com/livereview/internal/core_processor"
)

type (
	UnifiedWebhookEventV2  = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2  = coreprocessor.UnifiedMergeRequestV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
)

// APIClient posts outbound GitHub content on behalf of the provider.
type APIClient struct {
	httpClient *http.Client
}

// NewAPIClient constructs a GitHub output client with sensible defaults.
func NewAPIClient() *APIClient {
	return &APIClient{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// PostCommentReply posts a reply to an existing GitHub comment thread.
func (c *APIClient) PostCommentReply(event *UnifiedWebhookEventV2, token, replyText string) error {
	if event == nil || event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments",
		event.Repository.FullName, event.MergeRequest.Number)
	requestBody := map[string]interface{}{
		"body": replyText,
	}

	if event.Comment.Position != nil && event.Comment.InReplyToID != nil && *event.Comment.InReplyToID != "" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d/comments",
			event.Repository.FullName, event.MergeRequest.Number)

		inReplyToInt, err := strconv.Atoi(*event.Comment.InReplyToID)
		if err != nil {
			log.Printf("[WARN] Failed to convert in_reply_to ID to integer: %v, falling back to issue comment", err)
			apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments",
				event.Repository.FullName, event.MergeRequest.Number)
		} else {
			requestBody = map[string]interface{}{
				"body":        replyText,
				"in_reply_to": inReplyToInt,
			}
		}
	}

	log.Printf("[DEBUG] GitHub API call: %s", apiURL)
	log.Printf("[DEBUG] GitHub API payload: %+v", requestBody)

	return c.postToGitHubAPI(apiURL, token, requestBody)
}

// PostEmojiReaction posts an emoji reaction to a GitHub comment.
func (c *APIClient) PostEmojiReaction(event *UnifiedWebhookEventV2, token, reaction string) error {
	if event == nil || event.Comment == nil {
		return fmt.Errorf("invalid event for emoji reaction")
	}

	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/issues/comments/%s/reactions",
		event.Repository.FullName, event.Comment.ID)
	if event.Comment.Position != nil {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/comments/%s/reactions",
			event.Repository.FullName, event.Comment.ID)
	}

	requestBody := map[string]string{
		"content": reaction,
	}

	return c.postToGitHubAPI(apiURL, token, requestBody)
}

// PostReviewComments posts the structured review comments collected during processing.
func (c *APIClient) PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error {
	if len(comments) == 0 {
		return nil
	}

	repoFullName := mr.Metadata["repository_full_name"].(string)
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]

	for _, comment := range comments {
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/comments",
			owner, repo, mr.Number)

		requestBody := map[string]interface{}{
			"body": fmt.Sprintf("**%s** (%s)\n\n%s",
				comment.Severity, comment.Category, comment.Content),
			"path":      comment.FilePath,
			"line":      comment.LineNumber,
			"side":      "RIGHT",
			"commit_id": mr.Metadata["head_sha"],
		}

		if err := c.postToGitHubAPI(apiURL, token, requestBody); err != nil {
			return fmt.Errorf("failed to post review comment: %w", err)
		}
	}

	return nil
}

func (c *APIClient) postToGitHubAPI(apiURL, token string, requestBody interface{}) error {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	log.Printf("[DEBUG] Making GitHub API request to: %s", apiURL)

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] GitHub API request failed: %v", err)
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[INFO] Successfully posted to GitHub API: %s", apiURL)
	return nil
}
