package bitbucket

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

// APIClient posts outbound requests to Bitbucket.
type APIClient struct {
	httpClient *http.Client
}

// NewAPIClient creates a Bitbucket output client.
func NewAPIClient() *APIClient {
	return &APIClient{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// PostCommentReply creates or replies to a pull request comment.
func (c *APIClient) PostCommentReply(workspace, repository, prNumber string, inReplyToID *string, replyText, email, patToken string) error {
	if workspace == "" || repository == "" || prNumber == "" {
		return fmt.Errorf("workspace, repository, and prNumber are required")
	}
	if replyText == "" {
		return fmt.Errorf("reply text cannot be empty")
	}
	if email == "" || patToken == "" {
		return fmt.Errorf("bitbucket credentials missing")
	}

	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments", workspace, repository, prNumber)
	payload := map[string]interface{}{
		"content": map[string]interface{}{
			"raw": replyText,
		},
	}
	if inReplyToID != nil && *inReplyToID != "" {
		if parentID, err := strconv.Atoi(*inReplyToID); err == nil {
			payload["parent"] = map[string]interface{}{"id": parentID}
		} else {
			log.Printf("[WARN] Bitbucket reply: invalid parent InReplyToID '%s' (not an int): %v; posting as top-level comment", *inReplyToID, err)
		}
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(email, patToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("bitbucket API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("bitbucket API error (status %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("[INFO] Successfully posted Bitbucket comment reply")
	return nil
}
