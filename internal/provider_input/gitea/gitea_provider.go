package gitea

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/livereview/internal/capture"
	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/livereview/internal/webhookutils"
)

type (
	UnifiedTimelineV2      = coreprocessor.UnifiedTimelineV2
	UnifiedTimelineItemV2  = coreprocessor.UnifiedTimelineItemV2
	UnifiedCommitV2        = coreprocessor.UnifiedCommitV2
	UnifiedCommitAuthorV2  = coreprocessor.UnifiedCommitAuthorV2
	UnifiedBotUserInfoV2   = coreprocessor.UnifiedBotUserInfoV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
)

// GiteaOutputClient captures the outbound capabilities required by the provider.
type GiteaOutputClient interface {
	PostCommentReply(event *UnifiedWebhookEventV2, token, content string) error
	PostEmojiReaction(event *UnifiedWebhookEventV2, token, emoji string) error
	PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error
}

// GiteaV2Provider implements webhook provider behaviour for Gitea.
type GiteaV2Provider struct {
	db     *sql.DB
	output GiteaOutputClient
}

// NewGiteaV2Provider creates a Gitea provider with the required dependencies.
func NewGiteaV2Provider(db *sql.DB, output GiteaOutputClient) *GiteaV2Provider {
	if output == nil {
		panic("gitea output client is required")
	}
	return &GiteaV2Provider{db: db, output: output}
}

// ProviderName returns the provider name.
func (p *GiteaV2Provider) ProviderName() string {
	return "gitea"
}

// CanHandleWebhook checks if this provider can handle the webhook.
// Gitea webhooks include X-Gitea-Event header.
func (p *GiteaV2Provider) CanHandleWebhook(headers map[string]string, body []byte) bool {
	// If GitHub-specific headers are present, this is NOT a Gitea webhook
	// (GitHub and Gitea have similar payload structures, so we must check headers first)
	if _, exists := webhookutils.GetHeaderCaseInsensitive(headers, "X-GitHub-Event"); exists {
		return false
	}
	if _, exists := webhookutils.GetHeaderCaseInsensitive(headers, "X-GitHub-Delivery"); exists {
		return false
	}

	// Check for Gitea-specific headers using case-insensitive lookup
	if _, exists := webhookutils.GetHeaderCaseInsensitive(headers, "X-Gitea-Event"); exists {
		return true
	}
	if _, exists := webhookutils.GetHeaderCaseInsensitive(headers, "X-Gitea-Delivery"); exists {
		return true
	}
	if _, exists := webhookutils.GetHeaderCaseInsensitive(headers, "X-Gitea-Signature"); exists {
		return true
	}

	// Fallback: check payload structure (only if no GitHub headers detected above)
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		// Gitea webhooks have a "repository" object with specific fields
		if repo, exists := payload["repository"]; exists {
			if repoMap, ok := repo.(map[string]interface{}); ok {
				// Check for Gitea-specific fields
				if _, hasFullName := repoMap["full_name"]; hasFullName {
					if _, hasCloneURL := repoMap["clone_url"]; hasCloneURL {
						if _, hasOwner := repoMap["owner"]; hasOwner {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

// ConvertCommentEvent converts Gitea comment webhook to unified format.
func (p *GiteaV2Provider) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	eventType, _ := webhookutils.GetHeaderCaseInsensitive(headers, "X-Gitea-Event")
	log.Printf("[DEBUG] Gitea webhook event type: '%s'", eventType)
	log.Printf("[DEBUG] Available headers: %v", headers)

	canonicalType := canonicalGiteaEventType(eventType)
	var (
		event *UnifiedWebhookEventV2
		err   error
	)

	switch eventType {
	case "issue_comment":
		log.Printf("[DEBUG] Processing Gitea issue_comment event")
		event, err = ConvertGiteaIssueCommentEvent(body)
	case "pull_request_comment", "pull_request_review_comment":
		log.Printf("[DEBUG] Processing Gitea pull_request_review_comment event")
		event, err = ConvertGiteaPullRequestReviewCommentEvent(body)
	case "pull_request":
		log.Printf("[DEBUG] Processing Gitea pull_request event")
		event, err = ConvertGiteaPullRequestEvent(body)
	default:
		log.Printf("[WARN] Unsupported Gitea comment event type: '%s' (supported: issue_comment, pull_request_comment, pull_request)", eventType)
		err = fmt.Errorf("unsupported Gitea comment event type: '%s'", eventType)
	}

	if capture.Enabled() {
		recordGiteaWebhook(canonicalType, headers, body, event, err)
	}

	if err != nil {
		return nil, err
	}
	return event, nil
}

// ConvertReviewerEvent converts Gitea reviewer assignment webhook to unified format.
func (p *GiteaV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	// TODO: Implement reviewer event conversion for Gitea
	// Gitea may use pull_request events with action="review_requested"
	return nil, fmt.Errorf("reviewer events not yet implemented for Gitea")
}

// FetchMergeRequestData fetches additional MR data from Gitea API.
func (p *GiteaV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event")
	}

	_, baseURL, err := FindIntegrationTokenForGiteaRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Gitea token: %w", err)
	}

	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]
	prNumber := event.MergeRequest.Number

	// TODO: Implement Gitea API calls to fetch:
	// - PR commits: GET /api/v1/repos/{owner}/{repo}/pulls/{index}/commits
	// - PR comments: GET /api/v1/repos/{owner}/{repo}/pulls/{index}/comments
	// - Issue comments: GET /api/v1/repos/{owner}/{repo}/issues/{index}/comments

	log.Printf("[INFO] Fetching PR data for Gitea PR %s/%s#%d (base_url=%s)", owner, repo, prNumber, baseURL)

	// For now, just log and return success
	// Full implementation will be added in next iteration
	if event.MergeRequest.Metadata == nil {
		event.MergeRequest.Metadata = map[string]interface{}{}
	}
	event.MergeRequest.Metadata["repository_full_name"] = event.Repository.FullName
	event.MergeRequest.Metadata["pull_request_number"] = event.MergeRequest.Number
	event.MergeRequest.Metadata["base_url"] = baseURL

	return nil
}

// FindIntegrationTokenForRepo returns the integration token associated with the given repository.
func (p *GiteaV2Provider) FindIntegrationTokenForRepo(repoFullName string) (*IntegrationToken, error) {
	if p == nil {
		return nil, fmt.Errorf("gitea provider not initialised")
	}
	if p.db == nil {
		return nil, fmt.Errorf("gitea provider missing database handle")
	}

	token, _, err := FindIntegrationTokenForGiteaRepo(p.db, repoFullName)
	return token, err
}

// GetBotUserInfo fetches bot user information from Gitea API.
func (p *GiteaV2Provider) GetBotUserInfo(repository UnifiedRepositoryV2) (*UnifiedBotUserInfoV2, error) {
	if repository.FullName == "" {
		return nil, fmt.Errorf("missing repository full name")
	}

	token, baseURL, err := FindIntegrationTokenForGiteaRepo(p.db, repository.FullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Gitea token: %w", err)
	}

	// Call Gitea API /api/v1/user to get authenticated user info
	apiURL := fmt.Sprintf("%s/api/v1/user", baseURL)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", token.PatToken))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gitea user API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea user API error (status %d): %s", resp.StatusCode, string(body))
	}

	var userResp struct {
		ID       int64  `json:"id"`
		Login    string `json:"login"`
		Username string `json:"username"`
		FullName string `json:"full_name"`
		Email    string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&userResp); err != nil {
		return nil, fmt.Errorf("failed to decode Gitea user response: %w", err)
	}

	log.Printf("[INFO] Fetched Gitea bot user: id=%d login=%s username=%s",
		userResp.ID, userResp.Login, userResp.Username)

	// Use login field, fall back to username
	username := userResp.Login
	if username == "" {
		username = userResp.Username
	}

	return &UnifiedBotUserInfoV2{
		UserID:   fmt.Sprintf("%d", userResp.ID),
		Username: username,
		Name:     userResp.FullName,
		IsBot:    false, // Gitea doesn't have a bot flag
		Metadata: map[string]interface{}{
			"base_url": baseURL,
			"email":    userResp.Email,
		},
	}, nil
}

func recordGiteaWebhook(eventType string, headers map[string]string, body []byte, unified *UnifiedWebhookEventV2, err error) {
	if eventType == "" {
		eventType = "unknown"
	}

	if len(body) > 0 {
		capture.WriteBlob(fmt.Sprintf("gitea-webhook-%s-body", eventType), "json", body)
	}

	sanitized := sanitizeHeaders(headers)
	meta := map[string]interface{}{
		"event_type": eventType,
		"headers":    sanitized,
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	capture.WriteJSON(fmt.Sprintf("gitea-webhook-%s-meta", eventType), meta)

	if unified != nil && err == nil {
		capture.WriteJSON(fmt.Sprintf("gitea-webhook-%s-unified", eventType), unified)
	}
}

func sanitizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(headers))
	for k, v := range headers {
		if strings.EqualFold(k, "authorization") {
			continue
		}
		if strings.EqualFold(k, "x-gitea-signature") {
			continue
		}
		sanitized[k] = v
	}
	return sanitized
}

func canonicalGiteaEventType(eventType string) string {
	if eventType == "" {
		return "unknown"
	}
	return strings.ToLower(eventType)
}

// PostCommentReply posts a reply to a Gitea comment.
func (p *GiteaV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	log.Printf("[DIAG] GiteaV2Provider.PostCommentReply ENTRY: comment_id=%s, content_len=%d, repo=%s",
		event.Comment.ID, len(content), event.Repository.FullName)
	if event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	token, _, err := FindIntegrationTokenForGiteaRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Gitea token: %w", err)
	}

	// Ensure downstream output client has username/password for multipart fallback
	if event.MergeRequest.Metadata == nil {
		event.MergeRequest.Metadata = map[string]interface{}{}
	}
	if u, ok := token.Metadata["gitea_username"].(string); ok && u != "" {
		event.MergeRequest.Metadata["gitea_username"] = u
	}
	if pword, ok := token.Metadata["gitea_password"].(string); ok && pword != "" {
		event.MergeRequest.Metadata["gitea_password"] = pword
	}

	log.Printf("[DIAG] Calling p.output.PostCommentReply with token_len=%d, has_username=%v, has_password=%v",
		len(token.PatToken), event.MergeRequest.Metadata["gitea_username"] != nil, event.MergeRequest.Metadata["gitea_password"] != nil)
	return p.output.PostCommentReply(event, token.PatToken, content)
}

// PostEmojiReaction posts an emoji reaction to a Gitea comment.
func (p *GiteaV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	if event.Comment == nil {
		return fmt.Errorf("no comment in event for emoji reaction")
	}

	token, _, err := FindIntegrationTokenForGiteaRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Gitea token: %w", err)
	}

	return p.output.PostEmojiReaction(event, token.PatToken, emoji)
}

// PostFullReview posts a comprehensive review to a Gitea PR.
func (p *GiteaV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event for full review")
	}

	token, _, err := FindIntegrationTokenForGiteaRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Gitea token: %w", err)
	}

	if overallComment != "" {
		if err := p.output.PostCommentReply(event, token.PatToken, overallComment); err != nil {
			return fmt.Errorf("failed to post overall review comment: %w", err)
		}
	}

	return nil
}

// FetchMRTimeline fetches timeline data for a merge request.
func (p *GiteaV2Provider) FetchMRTimeline(mr UnifiedMergeRequestV2) (*UnifiedTimelineV2, error) {
	repoFullName, err := extractRepoFullNameFromMetadata(mr.Metadata)
	if err != nil {
		return nil, err
	}

	tokenObj, baseURL, err := FindIntegrationTokenForGiteaRepo(p.db, repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Gitea token: %w", err)
	}

	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]
	prNumber := mr.Number

	log.Printf("[INFO] Fetching timeline for Gitea PR %s/%s#%d (base_url=%s)", owner, repo, prNumber, baseURL)

	timeline := &UnifiedTimelineV2{
		Items: []UnifiedTimelineItemV2{},
	}

	// 1. Fetch issue comments (general PR comments)
	issueComments, err := p.fetchIssueComments(baseURL, tokenObj.PatToken, owner, repo, prNumber)
	if err != nil {
		log.Printf("[WARN] Failed to fetch issue comments: %v", err)
	} else {
		for _, comment := range issueComments {
			// Skip deleted comments (empty body)
			if strings.TrimSpace(comment.Body) == "" {
				continue
			}
			timeline.Items = append(timeline.Items, UnifiedTimelineItemV2{
				Type:      "comment",
				Timestamp: comment.CreatedAt,
				Comment: &UnifiedCommentV2{
					ID:        fmt.Sprintf("%d", comment.ID),
					Body:      comment.Body,
					CreatedAt: comment.CreatedAt,
					UpdatedAt: comment.UpdatedAt,
					WebURL:    comment.HTMLURL,
					Author: UnifiedUserV2{
						ID:       fmt.Sprintf("%d", comment.User.ID),
						Username: comment.User.Login,
						Name:     comment.User.FullName,
						Email:    comment.User.Email,
					},
					Metadata: map[string]interface{}{
						"comment_type": "issue_comment",
					},
				},
			})
		}
	}

	// 2. Fetch reviews
	reviews, err := p.fetchReviews(baseURL, tokenObj.PatToken, owner, repo, prNumber)
	if err != nil {
		log.Printf("[WARN] Failed to fetch reviews: %v", err)
	} else {
		for _, review := range reviews {
			// 3. Fetch comments for reviews that have comments
			if review.CommentsCount > 0 {
				comments, err := p.fetchReviewComments(baseURL, tokenObj.PatToken, owner, repo, prNumber, review.ID)
				if err != nil {
					log.Printf("[WARN] Failed to fetch comments for review %d: %v", review.ID, err)
					continue
				}

				for _, comment := range comments {
					// Skip deleted comments (empty body)
					if strings.TrimSpace(comment.Body) == "" {
						continue
					}

					// Build position if this is an inline comment
					var position *UnifiedPositionV2
					if comment.Path != "" {
						position = &UnifiedPositionV2{
							FilePath:   comment.Path,
							LineNumber: comment.Line,
							LineType:   comment.Side, // "LEFT" or "RIGHT"
							Metadata: map[string]interface{}{
								"commit_id": comment.CommitID,
							},
						}
					}

					timeline.Items = append(timeline.Items, UnifiedTimelineItemV2{
						Type:      "comment",
						Timestamp: comment.CreatedAt,
						Comment: &UnifiedCommentV2{
							ID:        fmt.Sprintf("%d", comment.ID),
							Body:      comment.Body,
							CreatedAt: comment.CreatedAt,
							UpdatedAt: comment.UpdatedAt,
							WebURL:    comment.HTMLURL,
							Position:  position,
							Author: UnifiedUserV2{
								ID:       fmt.Sprintf("%d", comment.User.ID),
								Username: comment.User.Login,
								Name:     comment.User.FullName,
								Email:    comment.User.Email,
							},
							Metadata: map[string]interface{}{
								"comment_type": "review_comment",
								"review_id":    review.ID,
								"review_state": review.State,
							},
						},
					})
				}
			}
		}
	}

	log.Printf("[INFO] Fetched %d timeline items for Gitea PR %s/%s#%d", len(timeline.Items), owner, repo, prNumber)
	return timeline, nil
}

// fetchIssueComments fetches general PR comments via /api/v1/repos/{owner}/{repo}/issues/{number}/comments
func (p *GiteaV2Provider) fetchIssueComments(baseURL, token, owner, repo string, prNumber int) ([]GiteaIssueComment, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments", baseURL, owner, repo, prNumber)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gitea API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea API error (status %d): %s", resp.StatusCode, string(body))
	}

	var comments []GiteaIssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return comments, nil
}

// fetchReviews fetches reviews via /api/v1/repos/{owner}/{repo}/pulls/{number}/reviews
func (p *GiteaV2Provider) fetchReviews(baseURL, token, owner, repo string, prNumber int) ([]GiteaReview, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", baseURL, owner, repo, prNumber)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gitea API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea API error (status %d): %s", resp.StatusCode, string(body))
	}

	var reviews []GiteaReview
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return reviews, nil
}

// fetchReviewComments fetches comments for a specific review via /api/v1/repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}/comments
func (p *GiteaV2Provider) fetchReviewComments(baseURL, token, owner, repo string, prNumber int, reviewID int64) ([]GiteaReviewComment, error) {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews/%d/comments", baseURL, owner, repo, prNumber, reviewID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("token %s", token))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Gitea API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gitea API error (status %d): %s", resp.StatusCode, string(body))
	}

	var comments []GiteaReviewComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return comments, nil
}

func extractRepoFullNameFromMetadata(metadata map[string]interface{}) (string, error) {
	if metadata == nil {
		return "", fmt.Errorf("metadata is nil")
	}
	fullName, ok := metadata["repository_full_name"].(string)
	if !ok || fullName == "" {
		return "", fmt.Errorf("repository_full_name not found in metadata")
	}
	return fullName, nil
}

// validateGiteaSignature validates the X-Gitea-Signature header using HMAC-SHA256.
// Gitea sends the signature as a hex-encoded HMAC-SHA256 hash of the payload.
func validateGiteaSignature(signature string, secret string, payload []byte) bool {
	if signature == "" || secret == "" {
		return false
	}

	// Compute HMAC-SHA256
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	// Compare signatures (constant time comparison to prevent timing attacks)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

// ValidateWebhookSignature validates the webhook signature using the secret from webhook_registry.
// This is called by the orchestrator before processing the webhook.
func (p *GiteaV2Provider) ValidateWebhookSignature(connectorID int64, headers map[string]string, body []byte) bool {
	signature := headers["X-Gitea-Signature"]
	if signature == "" {
		// Try lowercase header
		signature = headers["x-gitea-signature"]
	}

	if signature == "" {
		log.Printf("[WARN] Gitea webhook missing X-Gitea-Signature header for connector_id=%d", connectorID)
		// Allow webhooks without signature for backward compatibility and manual trigger mode
		return true
	}

	// Lookup webhook secret from webhook_registry
	secret, err := FindWebhookSecretByConnectorID(p.db, int(connectorID))
	if err != nil {
		log.Printf("[ERROR] Failed to lookup webhook secret for connector_id=%d: %v", connectorID, err)
		return false
	}

	if secret == "" {
		log.Printf("[WARN] No webhook secret configured for connector_id=%d, accepting webhook", connectorID)
		// No secret configured - accept webhook (manual trigger mode)
		return true
	}

	// Validate signature
	if !validateGiteaSignature(signature, secret, body) {
		log.Printf("[ERROR] Invalid Gitea webhook signature for connector_id=%d", connectorID)
		return false
	}

	log.Printf("[DEBUG] Gitea webhook signature validated successfully for connector_id=%d", connectorID)
	return true
}

// IntegrationToken represents a token from the database
type IntegrationToken struct {
	ID          int64
	Provider    string
	ProviderURL string
	PatToken    string
	OrgID       int64
	Metadata    map[string]interface{}
}
