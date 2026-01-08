package gitea

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	coreprocessor "github.com/livereview/internal/core_processor"
	giteautils "github.com/livereview/internal/providers/gitea"
)

type (
	UnifiedWebhookEventV2  = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2  = coreprocessor.UnifiedMergeRequestV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
	UnifiedPositionV2      = coreprocessor.UnifiedPositionV2
)

type apiError struct {
	StatusCode int
	Body       string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("Gitea API request failed with status %d: %s", e.StatusCode, e.Body)
}

// APIClient posts outbound Gitea content on behalf of the provider.
type APIClient struct {
	httpClient *http.Client
}

// NewAPIClient constructs a Gitea output client with sensible defaults.
func NewAPIClient() *APIClient {
	return &APIClient{httpClient: &http.Client{Timeout: 30 * time.Second}}
}

// PostCommentReply posts a reply to an existing Gitea comment thread.
func (c *APIClient) PostCommentReply(event *UnifiedWebhookEventV2, token, replyText string) error {
	log.Printf("[DEBUG] APIClient.PostCommentReply: comment_id=%s, has_position=%v, reply_len=%d",
		event.Comment.ID, event.Comment.Position != nil, len(replyText))

	if event == nil || event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	baseURL, err := extractBaseURL(event)
	if err != nil {
		return fmt.Errorf("failed to extract base URL: %w", err)
	}

	creds := giteautils.UnpackGiteaCredentials(token)
	if creds.Username == "" && event.MergeRequest != nil && event.MergeRequest.Metadata != nil {
		if u, ok := event.MergeRequest.Metadata["gitea_username"].(string); ok && u != "" {
			creds.Username = u
		}
		if p, ok := event.MergeRequest.Metadata["gitea_password"].(string); ok && p != "" {
			creds.Password = p
		}
	}
	token = creds.PAT
	baseURL = giteautils.NormalizeGiteaBaseURL(baseURL)

	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	// Extract review_id from metadata if present
	var reviewID int64
	if event.Comment.Metadata != nil {
		if rid, ok := event.Comment.Metadata["review_id"].(int64); ok && rid > 0 {
			reviewID = rid
		} else if ridFloat, ok := event.Comment.Metadata["review_id"].(float64); ok && ridFloat > 0 {
			reviewID = int64(ridFloat)
		}
	}

	// If webhook lacks review context but we have comment_id, fetch from API
	// Gitea webhooks for replies to inline comments don't include position/review_id
	if reviewID == 0 && event.Comment.ID != "" {
		log.Printf("[DEBUG] Webhook lacks review context (review_id=0), attempting metadata enrichment for comment_id=%s", event.Comment.ID)
		// Use username/password to create temporary PAT for API call if PAT is invalid
		enrichToken := token
		if creds.Username != "" && creds.Password != "" {
			log.Printf("[DEBUG] Creating temporary PAT for metadata enrichment (username=%s)", creds.Username)
			if tempPAT, err := c.createTemporaryPAT(baseURL, creds.Username, creds.Password); err != nil {
				log.Printf("[WARN] Failed to create temporary PAT: %v, using existing token", err)
			} else {
				enrichToken = tempPAT
				log.Printf("[DEBUG] Using temporary PAT for enrichment")
			}
		}
		if err := c.enrichCommentMetadata(baseURL, owner, repo, event, enrichToken); err != nil {
			log.Printf("[WARN] Metadata enrichment failed: %v (will proceed with general comment)", err)
		} else {
			log.Printf("[DEBUG] Metadata enrichment completed")
		}
		// Re-extract review_id after enrichment
		if event.Comment.Metadata != nil {
			if rid, ok := event.Comment.Metadata["review_id"].(int64); ok && rid > 0 {
				reviewID = rid
				log.Printf("[DEBUG] Extracted review_id after enrichment: %d", reviewID)
			} else if ridFloat, ok := event.Comment.Metadata["review_id"].(float64); ok && ridFloat > 0 {
				reviewID = int64(ridFloat)
				log.Printf("[DEBUG] Extracted review_id after enrichment (float): %d", reviewID)
			}
		}
	}

	log.Printf("[DEBUG] Routing decision: review_id=%d, has_position=%v → %s",
		reviewID, event.Comment.Position != nil,
		func() string {
			if reviewID > 0 && event.Comment.Position != nil {
				return "INLINE_MULTIPART"
			}
			return "GENERAL_COMMENT"
		}())

	// Route based on whether this is a review comment or general comment
	if reviewID > 0 && event.Comment.Position != nil {
		// Inline review comment - use multipart form
		return c.postInlineCommentReply(baseURL, owner, repo, event, replyText, reviewID, creds.Username, creds.Password)
	}

	// General comment on issue/PR
	return c.postGeneralComment(baseURL, owner, repo, event.MergeRequest.Number, token, replyText)
}

// postGeneralComment posts a general comment to an issue or PR
func (c *APIClient) postGeneralComment(baseURL, owner, repo string, prNumber int, token, replyText string) error {
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/%d/comments",
		baseURL, owner, repo, prNumber)

	requestBody := map[string]interface{}{
		"body": replyText,
	}

	log.Printf("[DEBUG] Gitea API call (general comment): %s", apiURL)
	log.Printf("[DEBUG] Gitea API payload: %+v", requestBody)

	return c.postToGiteaAPI(apiURL, token, requestBody)
}

// postInlineCommentReply posts a reply to an inline code review comment using multipart form.
func (c *APIClient) postInlineCommentReply(baseURL, owner, repo string, event *UnifiedWebhookEventV2, replyText string, reviewID int64, username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("inline comment reply requires username/password credentials")
	}
	if reviewID == 0 {
		return fmt.Errorf("inline comment reply requires review_id")
	}
	if event.Comment.Position == nil {
		return fmt.Errorf("inline comment reply requires position information")
	}
	if event.Comment.Position.FilePath == "" {
		return fmt.Errorf("inline comment reply requires file path")
	}
	if event.Comment.Position.LineNumber <= 0 {
		return fmt.Errorf("inline comment reply requires valid line number (got: %d)", event.Comment.Position.LineNumber)
	}

	return c.postInlineCommentReplyMultipart(baseURL, owner, repo, event.MergeRequest.Number, event, replyText, reviewID, username, password)
}

// enrichCommentMetadata fetches review context by scanning all reviews in the PR.
// Gitea webhooks for user replies to inline review comments lack position/review_id.
// We scan all reviews to find the inline comment being replied to and extract its context.
func (c *APIClient) enrichCommentMetadata(baseURL, owner, repo string, event *UnifiedWebhookEventV2, token string) error {
	prNumber := event.MergeRequest.Number
	log.Printf("[DIAG] enrichCommentMetadata ENTRY: pr=%d, comment_id=%s", prNumber, event.Comment.ID)

	// Fetch all reviews for this PR
	reviewsURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews", baseURL, owner, repo, prNumber)
	log.Printf("[DIAG] Fetching reviews from: %s", reviewsURL)

	req, err := http.NewRequest("GET", reviewsURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch reviews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ERROR] Failed to fetch reviews: status=%d, body=%s", resp.StatusCode, string(body)[:min(len(body), 300)])
		return fmt.Errorf("reviews API returned %d", resp.StatusCode)
	}

	var reviews []struct {
		ID            int64 `json:"id"`
		CommentsCount int64 `json:"comments_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return fmt.Errorf("failed to decode reviews: %w", err)
	}

	log.Printf("[DIAG] Found %d reviews, scanning for inline comments", len(reviews))

	// Scan each review's comments to find the most recent inline comment
	var latestComment map[string]interface{}
	latestTime := ""
	totalCommentsScanned := 0
	inlineCommentsFound := 0

	for _, review := range reviews {
		if review.CommentsCount == 0 {
			log.Printf("[DIAG] Review %d has no comments, skipping", review.ID)
			continue
		}

		commentsURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/reviews/%d/comments",
			baseURL, owner, repo, prNumber, review.ID)
		log.Printf("[DIAG] Fetching comments from review %d: %s", review.ID, commentsURL)

		creq, _ := http.NewRequest("GET", commentsURL, nil)
		creq.Header.Set("Authorization", "token "+token)
		creq.Header.Set("Accept", "application/json")

		cresp, err := client.Do(creq)
		if err != nil {
			log.Printf("[WARN] Failed to fetch review %d comments: %v", review.ID, err)
			continue
		}
		if cresp.StatusCode != 200 {
			log.Printf("[WARN] Review %d comments returned %d", review.ID, cresp.StatusCode)
			cresp.Body.Close()
			continue
		}

		var comments []map[string]interface{}
		if err := json.NewDecoder(cresp.Body).Decode(&comments); err != nil {
			log.Printf("[WARN] Failed to decode review %d comments: %v", review.ID, err)
			cresp.Body.Close()
			continue
		}
		cresp.Body.Close()

		log.Printf("[DIAG] Review %d has %d comments", review.ID, len(comments))
		totalCommentsScanned += len(comments)

		// Find latest inline comment (has path and position) - matching Python script logic
		for _, cmt := range comments {
			path, _ := cmt["path"].(string)
			position, _ := cmt["position"].(float64)
			created, _ := cmt["created_at"].(string)
			cmtID, _ := cmt["id"].(float64)

			log.Printf("[DIAG] Comment %.0f: path=%s, position=%.0f, created=%s", cmtID, path, position, created)

			// Python script: filter by path only, position is used for form submission
			if path != "" && position > 0 {
				inlineCommentsFound++
				if created > latestTime {
					log.Printf("[DIAG] New latest inline comment: %.0f (prev_time=%s, new_time=%s)", cmtID, latestTime, created)
					latestComment = cmt
					latestTime = created
				}
			} else {
				log.Printf("[DIAG] Comment %.0f is not inline (no path or position=0)", cmtID)
			}
		}
	}

	log.Printf("[DIAG] Scan complete: total_comments=%d, inline_comments=%d, latest_found=%v",
		totalCommentsScanned, inlineCommentsFound, latestComment != nil)

	if latestComment == nil {
		log.Printf("[DEBUG] No inline review comments found in PR %d", prNumber)
		return nil
	}

	// Populate metadata from the latest inline comment (use position field like Python script)
	reviewID, _ := latestComment["pull_request_review_id"].(float64)
	path, _ := latestComment["path"].(string)
	position, _ := latestComment["position"].(float64)
	cmtID, _ := latestComment["id"].(float64)

	log.Printf("[DIAG] Using latest inline comment: id=%.0f, review_id=%.0f, path=%s, position=%.0f",
		cmtID, reviewID, path, position)

	// Populate review_id into event metadata for routing decision
	if reviewID > 0 {
		if event.Comment.Metadata == nil {
			event.Comment.Metadata = make(map[string]interface{})
		}
		event.Comment.Metadata["review_id"] = int64(reviewID)
		log.Printf("[DIAG] Populated review_id in metadata: %.0f", reviewID)
	}

	if path != "" && position > 0 {
		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   path,
			LineNumber: int(position),
			LineType:   "new",
		}
		log.Printf("[DIAG] Populated position: path=%s, position=%.0f", path, position)
	}

	log.Printf("[DIAG] enrichCommentMetadata EXIT: SUCCESS (review_id=%.0f, has_position=%v)",
		reviewID, event.Comment.Position != nil)
	return nil
}

// fetchCommentFromEndpoint fetches comment details from a given API endpoint.
func (c *APIClient) fetchCommentFromEndpoint(apiURL, token string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	log.Printf("[DIAG] Executing fetch request...")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[DIAG] Fetch response: status=%d, content-type=%s, body_len=%d",
		resp.StatusCode, resp.Header.Get("Content-Type"), len(bodyBytes))

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("comment not found (404)")
	}

	if resp.StatusCode != 200 {
		log.Printf("[DEBUG] Fetch failed: status=%d, body=%s", resp.StatusCode, string(bodyBytes)[:min(len(bodyBytes), 300)])
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var comment map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &comment); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return comment, nil
}

// populateMetadataFromReviewComment populates event metadata from a pull request review comment.
func (c *APIClient) populateMetadataFromReviewComment(event *UnifiedWebhookEventV2, comment map[string]interface{}) error {
	reviewID, _ := comment["pull_request_review_id"].(float64)
	path, _ := comment["path"].(string)
	line, _ := comment["line"].(float64)
	commitID, _ := comment["commit_id"].(string)

	log.Printf("[DIAG] Parsed review comment: review_id=%.0f, path=%s, line=%.0f, commit=%s",
		reviewID, path, line, commitID[:min(len(commitID), 8)])

	if event.Comment.Metadata == nil {
		event.Comment.Metadata = make(map[string]interface{})
	}
	event.Comment.Metadata["review_id"] = int64(reviewID)
	log.Printf("[DIAG] Populated review_id in metadata: %.0f", reviewID)

	if path != "" && line > 0 {
		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   path,
			LineNumber: int(line),
			LineType:   "new",
		}
		log.Printf("[DIAG] Populated position: path=%s, line=%.0f", path, line)
	} else {
		log.Printf("[DEBUG] No position info in API response (path=%s, line=%.0f)", path, line)
	}

	log.Printf("[DIAG] enrichCommentMetadata EXIT: SUCCESS (review_id=%.0f, has_position=%v)",
		reviewID, event.Comment.Position != nil)
	return nil
}

// createTemporaryPAT creates a short-lived PAT for API operations using username/password.
func (c *APIClient) createTemporaryPAT(baseURL, username, password string) (string, error) {
	tokenName := fmt.Sprintf("livereview-temp-%d", time.Now().Unix())
	apiURL := fmt.Sprintf("%s/api/v1/users/%s/tokens", baseURL, username)

	log.Printf("[DIAG] Creating temporary PAT: url=%s, token_name=%s", apiURL, tokenName)

	payload := map[string]interface{}{
		"name":   tokenName,
		"scopes": []string{"write:repository"},
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	log.Printf("[DIAG] Executing PAT creation request...")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	log.Printf("[DIAG] PAT creation response: status=%d, body_len=%d", resp.StatusCode, len(bodyBytes))

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		log.Printf("[ERROR] PAT creation failed: %s", string(bodyBytes)[:min(len(bodyBytes), 500)])
		return "", fmt.Errorf("PAT creation returned %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		SHA1 string `json:"sha1"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.SHA1 == "" {
		return "", fmt.Errorf("PAT not found in response")
	}

	log.Printf("[DIAG] Temporary PAT created: %s...", result.SHA1[:min(len(result.SHA1), 12)])
	return result.SHA1, nil
}

// PostEmojiReaction posts an emoji reaction to a Gitea comment.
func (c *APIClient) PostEmojiReaction(event *UnifiedWebhookEventV2, token, reaction string) error {
	if event == nil || event.Comment == nil {
		return fmt.Errorf("invalid event for emoji reaction")
	}

	baseURL, err := extractBaseURL(event)
	if err != nil {
		return fmt.Errorf("failed to extract base URL: %w", err)
	}

	token = giteautils.UnpackGiteaPAT(token)
	baseURL = giteautils.NormalizeGiteaBaseURL(baseURL)

	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	commentID := event.Comment.ID

	// Gitea reactions endpoint: POST /api/v1/repos/{owner}/{repo}/issues/comments/{id}/reactions
	apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/issues/comments/%s/reactions",
		baseURL, owner, repo, commentID)

	// Map common reaction names to Gitea format
	giteaReaction := mapReactionToGitea(reaction)

	requestBody := map[string]string{
		"content": giteaReaction,
	}

	log.Printf("[DEBUG] Gitea API call (emoji reaction): %s", apiURL)
	log.Printf("[DEBUG] Gitea API payload: %+v", requestBody)

	return c.postToGiteaAPI(apiURL, token, requestBody)
}

// PostReviewComments posts the structured review comments collected during processing.
func (c *APIClient) PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error {
	if len(comments) == 0 {
		return nil
	}

	token = giteautils.UnpackGiteaPAT(token)

	// Extract base URL from metadata
	baseURL, ok := mr.Metadata["base_url"].(string)
	if !ok || baseURL == "" {
		return fmt.Errorf("base_url not found in merge request metadata")
	}
	baseURL = giteautils.NormalizeGiteaBaseURL(baseURL)

	repoFullName, ok := mr.Metadata["repository_full_name"].(string)
	if !ok || repoFullName == "" {
		return fmt.Errorf("repository_full_name not found in metadata")
	}

	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]

	headSHA, _ := mr.Metadata["head_sha"].(string)

	for _, comment := range comments {
		apiURL := fmt.Sprintf("%s/api/v1/repos/%s/%s/pulls/%d/comments",
			baseURL, owner, repo, mr.Number)

		// Convert line type to side from Position
		side := "RIGHT"
		if comment.Position != nil && comment.Position.LineType == "old" {
			side = "LEFT"
		}

		requestBody := map[string]interface{}{
			"body": fmt.Sprintf("**%s** (%s)\n\n%s", comment.Severity, comment.Category, comment.Content),
			"path": comment.FilePath,
			"line": comment.LineNumber,
			"side": side,
		}

		if headSHA != "" {
			requestBody["commit_id"] = headSHA
		}

		if err := c.postToGiteaAPI(apiURL, token, requestBody); err != nil {
			if apiErr, ok := err.(*apiError); ok && (apiErr.StatusCode == http.StatusUnprocessableEntity || apiErr.StatusCode == http.StatusBadRequest) {
				log.Printf("[WARN] Skipping Gitea review comment due to %d response (path=%s line=%d): %s",
					apiErr.StatusCode, comment.FilePath, comment.LineNumber, apiErr.Body)
				continue
			}
			return fmt.Errorf("failed to post review comment: %w", err)
		}
	}

	return nil
}

func (c *APIClient) postToGiteaAPI(apiURL, token string, requestBody interface{}) error {
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
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	log.Printf("[DEBUG] Making Gitea API request to: %s", apiURL)

	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] Gitea API request failed: %v", err)
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ERROR] Gitea API error response (status %d): %s", resp.StatusCode, string(body))
		return &apiError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	log.Printf("[INFO] Successfully posted to Gitea API: %s", apiURL)
	return nil
}

// postInlineCommentReplyMultipart emulates the browser form submission used by Gitea when replying inline.
// Requires username/password (from packed connector) to obtain session + CSRF.
func (c *APIClient) postInlineCommentReplyMultipart(baseURL, owner, repo string, prNumber int, event *UnifiedWebhookEventV2, replyText string, reviewID int64, username, password string) error {
	log.Printf("[DIAG] postInlineCommentReplyMultipart ENTRY: baseURL=%s, owner=%s, repo=%s, prNumber=%d, reviewID=%d, replyTextLen=%d",
		baseURL, owner, repo, prNumber, reviewID, len(replyText))
	log.Printf("[DIAG] Event comment ID=%s, author=%s", event.Comment.ID, event.Comment.Author.Username)
	if username == "" || password == "" {
		return fmt.Errorf("multipart fallback requires username/password in connector token")
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 30 * time.Second, Jar: jar}

	loginURL := fmt.Sprintf("%s/user/login", baseURL)
	log.Printf("[DIAG] Fetching login page: %s", loginURL)
	resp, err := client.Get(loginURL)
	if err != nil {
		return fmt.Errorf("failed to load login page: %w", err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	csrf := extractCSRFToken(string(bodyBytes))
	log.Printf("[DIAG] CSRF from login page HTML: %s (len=%d)", csrf[:min(len(csrf), 16)], len(csrf))
	if csrf == "" {
		// try cookie
		for _, ck := range resp.Cookies() {
			if ck.Name == "_csrf" {
				csrf = ck.Value
				log.Printf("[DIAG] CSRF from cookie: %s (len=%d)", csrf[:min(len(csrf), 16)], len(csrf))
				break
			}
		}
	}
	if csrf == "" {
		return fmt.Errorf("could not obtain CSRF token from login page")
	}

	form := url.Values{}
	form.Set("_csrf", csrf)
	form.Set("user_name", username)
	form.Set("password", password)
	form.Set("remember", "on")

	log.Printf("[DIAG] POSTing login with user=%s, csrf_len=%d", username, len(csrf))
	resp, err = client.PostForm(loginURL, form)
	if err != nil {
		return fmt.Errorf("login post failed: %w", err)
	}
	log.Printf("[DIAG] Login POST response: status=%d, location=%s", resp.StatusCode, resp.Header.Get("Location"))
	resp.Body.Close()

	// Refresh CSRF from cookies after login (Gitea rotates it)
	oldCSRF := csrf
	if newCSRF := extractCSRFFromCookies(client, baseURL); newCSRF != "" {
		csrf = newCSRF
		log.Printf("[DIAG] CSRF refreshed from cookie: old_len=%d, new_len=%d, changed=%v", len(oldCSRF), len(csrf), oldCSRF != csrf)
	} else {
		log.Printf("[DIAG] No CSRF cookie found after login, keeping old: len=%d", len(csrf))
	}
	if csrf == "" {
		return fmt.Errorf("missing CSRF token after login")
	}

	// Position validated in postInlineCommentReply
	line := strconv.Itoa(event.Comment.Position.LineNumber)
	path := event.Comment.Position.FilePath
	commit := ""
	if sha, ok := event.MergeRequest.Metadata["head_sha"].(string); ok {
		commit = sha
	}

	log.Printf("[DEBUG] Posting inline reply: line=%s, path=%s, reviewID=%d", line, path, reviewID)

	// Build multipart form mirroring the working curl
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fields := map[string]string{
		"_csrf":            csrf,
		"origin":           "timeline",
		"latest_commit_id": commit,
		"side":             "proposed",
		"line":             line,
		"path":             path,
		"diff_start_cid":   "",
		"diff_end_cid":     "",
		"diff_base_cid":    "",
		"content":          replyText,
		"reply":            strconv.FormatInt(reviewID, 10),
		"single_review":    "true",
	}
	for k, v := range fields {
		fw, ferr := mw.CreateFormField(k)
		if ferr != nil {
			return fmt.Errorf("failed to create form field %s: %w", k, ferr)
		}
		if _, ferr = fw.Write([]byte(v)); ferr != nil {
			return fmt.Errorf("failed to write form field %s: %w", k, ferr)
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	commentURL := fmt.Sprintf("%s/%s/%s/pulls/%d/files/reviews/comments", baseURL, owner, repo, prNumber)
	log.Printf("[DIAG] Multipart POST URL: %s", commentURL)
	log.Printf("[DIAG] Multipart body size: %d bytes", buf.Len())
	req, err := http.NewRequest("POST", commentURL, &buf)
	if err != nil {
		return fmt.Errorf("failed to create multipart request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "LiveReview-Bot")
	req.Header.Set("Referer", fmt.Sprintf("%s/%s/%s/pulls/%d/files", baseURL, owner, repo, prNumber))

	log.Printf("[DIAG] Multipart POST headers: Content-Type=%s, CSRF_len=%d, Referer=%s",
		mw.FormDataContentType(), len(csrf), req.Header.Get("Referer"))
	log.Printf("[DIAG] Executing multipart POST request...")
	resp, err = client.Do(req)
	if err != nil {
		return fmt.Errorf("multipart reply request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DIAG] Multipart POST response: status=%d, location=%s, content-type=%s, body_len=%d",
		resp.StatusCode, resp.Header.Get("Location"), resp.Header.Get("Content-Type"), len(body))
	log.Printf("[DIAG] Response body preview: %s", string(body)[:min(len(body), 500)])

	// Check for redirects
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		log.Printf("[DIAG] Redirect detected: %d -> %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("[ERROR] Multipart POST failed: status=%d, body=%s", resp.StatusCode, string(body)[:min(len(body), 1000)])
		return &apiError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	log.Printf("[DIAG] postInlineCommentReplyMultipart EXIT: SUCCESS")
	return nil
}

var csrfPattern = regexp.MustCompile(`name="_csrf"\s+value="([^"]+)"`)

func extractCSRFToken(html string) string {
	m := csrfPattern.FindStringSubmatch(html)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// extractCSRFFromCookies returns the _csrf cookie value if present after auth.
func extractCSRFFromCookies(client *http.Client, baseURL string) string {
	if client == nil || client.Jar == nil {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	for _, ck := range client.Jar.Cookies(u) {
		if ck.Name == "_csrf" && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

// extractBaseURL extracts the Gitea base URL from event metadata or repository URL
func extractBaseURL(event *UnifiedWebhookEventV2) (string, error) {
	// Try to get from merge request metadata first
	if event.MergeRequest != nil && event.MergeRequest.Metadata != nil {
		if baseURL, ok := event.MergeRequest.Metadata["base_url"].(string); ok && baseURL != "" {
			return baseURL, nil
		}
	}

	// Try to extract from repository web URL
	if event.Repository.WebURL != "" {
		// Extract base URL from web URL (e.g., https://gitea.hexmos.site/owner/repo -> https://gitea.hexmos.site)
		parts := strings.Split(event.Repository.WebURL, "/")
		if len(parts) >= 3 {
			// parts: [https:] [] [host] [owner] [repo]
			baseURL := strings.Join(parts[:3], "/")
			return baseURL, nil
		}
	}

	return "", fmt.Errorf("could not extract base URL from event")
}

// mapReactionToGitea maps common emoji names to Gitea reaction format
func mapReactionToGitea(reaction string) string {
	// Gitea supports: +1, -1, laugh, hooray, confused, heart, rocket, eyes
	reactionMap := map[string]string{
		"thumbsup":   "+1",
		"+1":         "+1",
		"thumbsdown": "-1",
		"-1":         "-1",
		"laugh":      "laugh",
		"hooray":     "hooray",
		"confused":   "confused",
		"heart":      "heart",
		"rocket":     "rocket",
		"eyes":       "eyes",
	}

	if mapped, ok := reactionMap[strings.ToLower(reaction)]; ok {
		return mapped
	}

	// Return as-is if not in map
	return reaction
}
