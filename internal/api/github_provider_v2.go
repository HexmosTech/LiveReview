package api

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

	githubinput "github.com/livereview/internal/provider_input/github"
)

type (
	GitHubV2WebhookPayload                         = githubinput.GitHubV2WebhookPayload
	GitHubV2PullRequest                            = githubinput.GitHubV2PullRequest
	GitHubV2Repository                             = githubinput.GitHubV2Repository
	GitHubV2User                                   = githubinput.GitHubV2User
	GitHubV2Team                                   = githubinput.GitHubV2Team
	GitHubV2Branch                                 = githubinput.GitHubV2Branch
	GitHubV2ReviewerChangeInfo                     = githubinput.GitHubV2ReviewerChangeInfo
	GitHubV2ReviewerBotUserInfo                    = githubinput.GitHubV2ReviewerBotUserInfo
	GitHubV2IssueCommentWebhookPayload             = githubinput.GitHubV2IssueCommentWebhookPayload
	GitHubV2PullRequestReviewCommentWebhookPayload = githubinput.GitHubV2PullRequestReviewCommentWebhookPayload
	GitHubV2Comment                                = githubinput.GitHubV2Comment
	GitHubV2ReviewComment                          = githubinput.GitHubV2ReviewComment
	GitHubV2Issue                                  = githubinput.GitHubV2Issue
	GitHubV2Label                                  = githubinput.GitHubV2Label
	GitHubV2Milestone                              = githubinput.GitHubV2Milestone
	GitHubV2IssuePRRef                             = githubinput.GitHubV2IssuePRRef
	GitHubV2BotUserInfo                            = githubinput.GitHubV2BotUserInfo
	GitHubV2CommitInfo                             = githubinput.GitHubV2CommitInfo
	GitHubV2CommitAuthor                           = githubinput.GitHubV2CommitAuthor
	GitHubV2CommentInfo                            = githubinput.GitHubV2CommentInfo
)

// GitHubV2Provider implements WebhookProviderV2 interface for GitHub
type GitHubV2Provider struct {
	server *Server
}

// NewGitHubV2Provider creates a new GitHub V2 provider
func NewGitHubV2Provider(server *Server) *GitHubV2Provider {
	return &GitHubV2Provider{server: server}
}

// ProviderName returns the provider name
func (p *GitHubV2Provider) ProviderName() string {
	return "github"
}

// CanHandleWebhook checks if this provider can handle the webhook
func (p *GitHubV2Provider) CanHandleWebhook(headers map[string]string, body []byte) bool {
	// Check for GitHub-specific headers
	if _, exists := headers["X-GitHub-Event"]; exists {
		return true
	}
	if _, exists := headers["X-GitHub-Delivery"]; exists {
		return true
	}

	// Check for GitHub-specific content in body
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		// GitHub webhooks typically have "sender" field with GitHub user structure
		if sender, exists := payload["sender"]; exists {
			if senderMap, ok := sender.(map[string]interface{}); ok {
				if _, hasLogin := senderMap["login"]; hasLogin {
					if _, hasHTMLURL := senderMap["html_url"]; hasHTMLURL {
						return true
					}
				}
			}
		}
	}

	return false
}

// ConvertCommentEvent converts GitHub comment webhook to unified format
func (p *GitHubV2Provider) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	// GitHub uses X-GitHub-Event header, try different case variations
	eventType := headers["X-GitHub-Event"]
	if eventType == "" {
		eventType = headers["X-Github-Event"]
	}
	if eventType == "" {
		eventType = headers["x-github-event"]
	}
	log.Printf("[DEBUG] GitHub webhook event type: '%s'", eventType)
	log.Printf("[DEBUG] Available headers: %v", headers)

	switch eventType {
	case "issue_comment":
		log.Printf("[DEBUG] Processing GitHub issue_comment event")
		return p.convertIssueCommentEventV2(body)
	case "pull_request_review_comment":
		log.Printf("[DEBUG] Processing GitHub pull_request_review_comment event")
		return p.convertPullRequestReviewCommentEventV2(body)
	case "pull_request_review":
		log.Printf("[DEBUG] Processing GitHub pull_request_review event")
		return p.convertPullRequestReviewEventV2(body)
	default:
		log.Printf("[WARN] Unsupported GitHub comment event type: '%s' (supported: issue_comment, pull_request_review_comment, pull_request_review)", eventType)
		return nil, fmt.Errorf("unsupported GitHub comment event type: '%s'", eventType)
	}
}

// convertIssueCommentEventV2 converts GitHub issue comment webhook to unified format
func (p *GitHubV2Provider) convertIssueCommentEventV2(body []byte) (*UnifiedWebhookEventV2, error) {
	return githubinput.ConvertIssueCommentEvent(body)
}

// convertPullRequestReviewCommentEventV2 converts GitHub PR review comment webhook to unified format
func (p *GitHubV2Provider) convertPullRequestReviewCommentEventV2(body []byte) (*UnifiedWebhookEventV2, error) {
	return githubinput.ConvertPullRequestReviewCommentEvent(body)
}

// convertPullRequestReviewEventV2 converts GitHub pull request review webhook to unified format
func (p *GitHubV2Provider) convertPullRequestReviewEventV2(body []byte) (*UnifiedWebhookEventV2, error) {
	return githubinput.ConvertPullRequestReviewEvent(body)
}

// ConvertReviewerEvent converts GitHub reviewer assignment webhook to unified format
func (p *GitHubV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	return githubinput.ConvertReviewerEvent(headers, body)
}

// FetchMergeRequestData fetches additional MR data from GitHub API - simplified version
func (p *GitHubV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event")
	}

	// Get GitHub token
	token, err := p.findIntegrationTokenForGitHubRepoV2(event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Extract owner, repo, and PR number
	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]
	prNumber := fmt.Sprintf("%d", event.MergeRequest.Number)

	// Fetch commits and comments for future use
	_, err = p.fetchGitHubPRCommitsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	_, err = p.fetchGitHubPRCommentsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return fmt.Errorf("failed to get comments: %w", err)
	}

	log.Printf("[INFO] Successfully fetched PR data for GitHub PR %s", prNumber)
	return nil
}

// PostCommentReply posts a reply to a GitHub comment
func (p *GitHubV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	if event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	// Get GitHub token
	token, err := p.findIntegrationTokenForGitHubRepoV2(event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	return p.postGitHubCommentReplyV2(event, token.PatToken, content)
}

// PostEmojiReaction posts an emoji reaction to a GitHub comment
func (p *GitHubV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	if event.Comment == nil {
		return fmt.Errorf("no comment in event for emoji reaction")
	}

	// Get GitHub token
	token, err := p.findIntegrationTokenForGitHubRepoV2(event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	return p.postGitHubCommentReactionV2(event, token.PatToken, emoji)
}

// PostFullReview posts a comprehensive review to a GitHub PR - simplified version
func (p *GitHubV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event for full review")
	}

	// Get GitHub token
	token, err := p.findIntegrationTokenForGitHubRepoV2(event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Post overall review comment
	if overallComment != "" {
		if err := p.postGitHubCommentReplyV2(event, token.PatToken, overallComment); err != nil {
			return fmt.Errorf("failed to post overall review comment: %w", err)
		}
	}

	return nil
}

// GitHub V2 API Methods - Updated versions of existing GitHub API methods

// findIntegrationTokenForGitHubRepoV2 finds integration token for GitHub repository
func (p *GitHubV2Provider) findIntegrationTokenForGitHubRepoV2(repoFullName string) (*IntegrationToken, error) {
	// Query database for GitHub token
	query := `
		SELECT id, provider, provider_url, pat_token, metadata
		FROM integration_tokens 
		WHERE provider = 'github' 
		AND (provider_url = 'https://github.com' OR provider_url = 'https://api.github.com')
		LIMIT 1
	`

	var token IntegrationToken
	var metadataJSON []byte

	err := p.server.db.QueryRow(query).Scan(
		&token.ID, &token.Provider, &token.ProviderURL,
		&token.PatToken, &metadataJSON)
	if err != nil {
		return nil, fmt.Errorf("no GitHub integration token found: %w", err)
	}

	// Parse metadata if present
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &token.Metadata); err != nil {
			log.Printf("[WARN] Failed to parse token metadata: %v", err)
		}
	}

	return &token, nil
}

// getFreshGitHubBotUserInfoV2 gets fresh bot user information via GitHub API
func (p *GitHubV2Provider) getFreshGitHubBotUserInfoV2(repoFullName string) (*GitHubV2BotUserInfo, error) {
	// Get integration token for the repository
	token, err := p.findIntegrationTokenForGitHubRepoV2(repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Make API call to get current user (the bot)
	apiURL := "https://api.github.com/user"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token.PatToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	var user GitHubV2BotUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub user response: %w", err)
	}

	return &user, nil
}

// GitHub V2 Posting Methods

// postGitHubCommentReactionV2 posts a reaction to a GitHub comment
func (p *GitHubV2Provider) postGitHubCommentReactionV2(event *UnifiedWebhookEventV2, token, reaction string) error {
	var apiURL string
	if event.Comment.Position != nil {
		// Review comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/comments/%s/reactions",
			event.Repository.FullName, event.Comment.ID)
	} else {
		// Issue comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/comments/%s/reactions",
			event.Repository.FullName, event.Comment.ID)
	}

	requestBody := map[string]string{
		"content": reaction,
	}

	return p.postToGitHubAPIV2(apiURL, token, requestBody)
}

// postGitHubCommentReplyV2 posts a reply to a GitHub comment
func (p *GitHubV2Provider) postGitHubCommentReplyV2(event *UnifiedWebhookEventV2, token, replyText string) error {
	var apiURL string
	var requestBody map[string]interface{}

	// Based on V1 working implementation - handle replies properly
	if event.Comment.Position != nil && event.Comment.InReplyToID != nil && *event.Comment.InReplyToID != "" {
		// Reply to review comment - use review comments API with in_reply_to
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/%d/comments",
			event.Repository.FullName, event.MergeRequest.Number)

		// Convert string ID to integer for GitHub API (like V1)
		inReplyToInt, err := strconv.Atoi(*event.Comment.InReplyToID)
		if err != nil {
			log.Printf("[WARN] Failed to convert in_reply_to ID to integer: %v, falling back to issue comment", err)
			// Fall back to issue comment
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
		// General PR comment or no reply context - use issue comment API
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments",
			event.Repository.FullName, event.MergeRequest.Number)
		requestBody = map[string]interface{}{
			"body": replyText,
		}
	}

	log.Printf("[DEBUG] GitHub API call: %s", apiURL)
	log.Printf("[DEBUG] GitHub API payload: %+v", requestBody)

	return p.postToGitHubAPIV2(apiURL, token, requestBody)
}

// postToGitHubAPIV2 makes a POST request to GitHub API
func (p *GitHubV2Provider) postToGitHubAPIV2(apiURL, token string, requestBody interface{}) error {
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

// GitHub V2 Data Fetching Methods

// fetchGitHubPRCommitsV2 fetches commits for a GitHub PR
func (p *GitHubV2Provider) fetchGitHubPRCommitsV2(owner, repo, prNumber, token string) ([]GitHubV2CommitInfo, error) {
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

// fetchGitHubPRCommentsV2 fetches comments for a GitHub PR
func (p *GitHubV2Provider) fetchGitHubPRCommentsV2(owner, repo, prNumber, token string) ([]GitHubV2CommentInfo, error) {
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

// Missing WebhookProviderV2 Interface Methods

// FetchMRTimeline fetches timeline data for a merge request
func (p *GitHubV2Provider) FetchMRTimeline(mr UnifiedMergeRequestV2) (*UnifiedTimelineV2, error) {
	// Get integration token for the repository
	token, err := p.findIntegrationTokenForGitHubRepoV2(mr.Metadata["repository_full_name"].(string))
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Extract owner, repo, and PR number from metadata
	repoFullName := mr.Metadata["repository_full_name"].(string)
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]
	prNumber := fmt.Sprintf("%d", mr.Number)

	// Fetch commits and comments in parallel
	commits, err := p.fetchGitHubPRCommitsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commits: %w", err)
	}

	comments, err := p.fetchGitHubPRCommentsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}

	// Build unified timeline
	timeline := &UnifiedTimelineV2{
		Items: []UnifiedTimelineItemV2{},
	}

	// Add commits to timeline
	for _, commit := range commits {
		timeline.Items = append(timeline.Items, UnifiedTimelineItemV2{
			Type:      "commit",
			Timestamp: commit.Author.Date,
			Commit: &UnifiedCommitV2{
				SHA:     commit.SHA,
				Message: commit.Message,
				Author: UnifiedCommitAuthorV2{
					Name:  commit.Author.Name,
					Email: commit.Author.Email,
				},
				Timestamp: commit.Author.Date,
				WebURL:    commit.URL,
			},
		})
	}

	// Add comments to timeline
	for _, comment := range comments {
		timeline.Items = append(timeline.Items, UnifiedTimelineItemV2{
			Type:      "comment",
			Timestamp: comment.CreatedAt,
			Comment: &UnifiedCommentV2{
				ID:        fmt.Sprintf("%d", comment.ID),
				Body:      comment.Body,
				CreatedAt: comment.CreatedAt,
				UpdatedAt: comment.UpdatedAt,
				Author: UnifiedUserV2{
					ID:       fmt.Sprintf("%d", comment.User.ID),
					Username: comment.User.Login,
					Name:     comment.User.Name,
					WebURL:   comment.User.HTMLURL,
				},
			},
		})
	}

	return timeline, nil
}

// FetchCodeContext fetches code context (diff hunks, file content) for a comment
func (p *GitHubV2Provider) FetchCodeContext(comment UnifiedCommentV2) (string, error) {
	// For GitHub, if the comment has position info, we can get the diff context
	if comment.Position == nil {
		return "", nil // No code context for general comments
	}

	// This is a simplified implementation - in a full implementation,
	// you would fetch the actual diff hunks from the GitHub API
	context := fmt.Sprintf("File: %s\nLine: %d",
		comment.Position.FilePath,
		comment.Position.LineNumber)

	if comment.Position.StartLine != nil {
		context += fmt.Sprintf("\nLines: %d-%d",
			*comment.Position.StartLine,
			comment.Position.LineNumber)
	}

	return context, nil
}

// GetBotUserInfo gets bot user information for warrant checking
func (p *GitHubV2Provider) GetBotUserInfo(repository UnifiedRepositoryV2) (*UnifiedBotUserInfoV2, error) {
	// Get fresh bot user info from GitHub API
	botInfo, err := p.getFreshGitHubBotUserInfoV2(repository.FullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub bot user info: %w", err)
	}

	// Convert to unified format
	return &UnifiedBotUserInfoV2{
		UserID:   fmt.Sprintf("%d", botInfo.ID),
		Username: botInfo.Login,
		Name:     botInfo.Name,
		IsBot:    botInfo.Type == "Bot",
		Metadata: map[string]interface{}{
			"html_url":   botInfo.HTMLURL,
			"avatar_url": botInfo.AvatarURL,
			"provider":   "github",
			"user_type":  botInfo.Type,
		},
	}, nil
}

// PostReviewComments posts multiple review comments to a GitHub PR
func (p *GitHubV2Provider) PostReviewComments(mr UnifiedMergeRequestV2, comments []UnifiedReviewCommentV2) error {
	if len(comments) == 0 {
		return nil
	}

	// Get integration token
	repoFullName := mr.Metadata["repository_full_name"].(string)
	token, err := p.findIntegrationTokenForGitHubRepoV2(repoFullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	// Extract owner and repo
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]

	// Post each comment
	for _, comment := range comments {
		// For GitHub, we need to post as individual PR review comments
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d/comments",
			owner, repo, mr.Number)

		requestBody := map[string]interface{}{
			"body": fmt.Sprintf("**%s** (%s)\n\n%s",
				comment.Severity, comment.Category, comment.Content),
			"path":      comment.FilePath,
			"line":      comment.LineNumber,
			"side":      "RIGHT",
			"commit_id": mr.Metadata["head_sha"], // Would need the actual commit SHA
		}

		if err := p.postToGitHubAPIV2(apiURL, token.PatToken, requestBody); err != nil {
			return fmt.Errorf("failed to post review comment: %w", err)
		}
	}

	return nil
}
