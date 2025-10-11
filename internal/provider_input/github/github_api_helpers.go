package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// PostGitHubCommentReactionV2 posts a reaction to a GitHub comment.
func PostGitHubCommentReactionV2(event *UnifiedWebhookEventV2, token, reaction string) error {
	if event == nil || event.Comment == nil {
		return fmt.Errorf("invalid event for emoji reaction")
	}

	var apiURL string
	if event.Comment.Position != nil {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/comments/%s/reactions",
			event.Repository.FullName, event.Comment.ID)
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/comments/%s/reactions",
			event.Repository.FullName, event.Comment.ID)
	}

	requestBody := map[string]string{
		"content": reaction,
	}

	return PostToGitHubAPIV2(apiURL, token, requestBody)
}

// PostGitHubCommentReplyV2 posts a reply to a GitHub comment.
func PostGitHubCommentReplyV2(event *UnifiedWebhookEventV2, token, replyText string) error {
	if event == nil || event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	var apiURL string
	var requestBody map[string]interface{}

	if event.Comment.Position != nil && event.Comment.InReplyToID != nil && *event.Comment.InReplyToID != "" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d/comments",
			event.Repository.FullName, event.MergeRequest.Number)

		inReplyToInt, err := strconv.Atoi(*event.Comment.InReplyToID)
		if err != nil {
			log.Printf("[WARN] Failed to convert in_reply_to ID to integer: %v, falling back to issue comment", err)
			apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments",
				event.Repository.FullName, event.MergeRequest.Number)
			requestBody = map[string]interface{}{
				"body": replyText,
			}
		} else {
			requestBody = map[string]interface{}{
				"body":        replyText,
				"in_reply_to": inReplyToInt,
			}
		}
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments",
			event.Repository.FullName, event.MergeRequest.Number)
		requestBody = map[string]interface{}{
			"body": replyText,
		}
	}

	log.Printf("[DEBUG] GitHub API call: %s", apiURL)
	log.Printf("[DEBUG] GitHub API payload: %+v", requestBody)

	return PostToGitHubAPIV2(apiURL, token, requestBody)
}

// PostToGitHubAPIV2 makes a POST request to the GitHub API.
func PostToGitHubAPIV2(apiURL, token string, requestBody interface{}) error {
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
	client := &http.Client{Timeout: 30 * time.Second}
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

// FetchGitHubPRCommitsV2 fetches commits for a GitHub PR.
func FetchGitHubPRCommitsV2(owner, repo, prNumber, token string) ([]GitHubV2CommitInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/commits", owner, repo, prNumber)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
	}

	var commits []GitHubV2CommitInfo
	err = json.NewDecoder(resp.Body).Decode(&commits)
	return commits, err
}

// FetchGitHubPRCommentsV2 fetches comments for a GitHub PR.
func FetchGitHubPRCommentsV2(owner, repo, prNumber, token string) ([]GitHubV2CommentInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", owner, repo, prNumber)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
	}

	var comments []GitHubV2CommentInfo
	err = json.NewDecoder(resp.Body).Decode(&comments)
	return comments, err
}
