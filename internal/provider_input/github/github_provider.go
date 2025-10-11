package github

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	coreprocessor "github.com/livereview/internal/core_processor"
)

type (
	UnifiedTimelineV2      = coreprocessor.UnifiedTimelineV2
	UnifiedTimelineItemV2  = coreprocessor.UnifiedTimelineItemV2
	UnifiedCommitV2        = coreprocessor.UnifiedCommitV2
	UnifiedCommitAuthorV2  = coreprocessor.UnifiedCommitAuthorV2
	UnifiedBotUserInfoV2   = coreprocessor.UnifiedBotUserInfoV2
	UnifiedReviewCommentV2 = coreprocessor.UnifiedReviewCommentV2
)

// GitHubV2Provider implements webhook provider behaviour for GitHub.
type GitHubV2Provider struct {
	db *sql.DB
}

// NewGitHubV2Provider creates a GitHub provider with the required dependencies.
func NewGitHubV2Provider(db *sql.DB) *GitHubV2Provider {
	return &GitHubV2Provider{db: db}
}

// ProviderName returns the provider name.
func (p *GitHubV2Provider) ProviderName() string {
	return "github"
}

// CanHandleWebhook checks if this provider can handle the webhook.
func (p *GitHubV2Provider) CanHandleWebhook(headers map[string]string, body []byte) bool {
	if _, exists := headers["X-GitHub-Event"]; exists {
		return true
	}
	if _, exists := headers["X-GitHub-Delivery"]; exists {
		return true
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
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

// ConvertCommentEvent converts GitHub comment webhook to unified format.
func (p *GitHubV2Provider) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
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
		return ConvertIssueCommentEvent(body)
	case "pull_request_review_comment":
		log.Printf("[DEBUG] Processing GitHub pull_request_review_comment event")
		return ConvertPullRequestReviewCommentEvent(body)
	case "pull_request_review":
		log.Printf("[DEBUG] Processing GitHub pull_request_review event")
		return ConvertPullRequestReviewEvent(body)
	default:
		log.Printf("[WARN] Unsupported GitHub comment event type: '%s' (supported: issue_comment, pull_request_review_comment, pull_request_review)", eventType)
		return nil, fmt.Errorf("unsupported GitHub comment event type: '%s'", eventType)
	}
}

// ConvertReviewerEvent converts GitHub reviewer assignment webhook to unified format.
func (p *GitHubV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	return ConvertReviewerEvent(headers, body)
}

// FetchMergeRequestData fetches additional MR data from GitHub API.
func (p *GitHubV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event")
	}

	token, err := FindIntegrationTokenForGitHubRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	parts := strings.Split(event.Repository.FullName, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repository full name: %s", event.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]
	prNumber := fmt.Sprintf("%d", event.MergeRequest.Number)

	if _, err = FetchGitHubPRCommitsV2(owner, repo, prNumber, token.PatToken); err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	if _, err = FetchGitHubPRCommentsV2(owner, repo, prNumber, token.PatToken); err != nil {
		return fmt.Errorf("failed to get comments: %w", err)
	}

	log.Printf("[INFO] Successfully fetched PR data for GitHub PR %s", prNumber)
	return nil
}

// PostCommentReply posts a reply to a GitHub comment.
func (p *GitHubV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	if event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	token, err := FindIntegrationTokenForGitHubRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	return PostGitHubCommentReplyV2(event, token.PatToken, content)
}

// PostEmojiReaction posts an emoji reaction to a GitHub comment.
func (p *GitHubV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	if event.Comment == nil {
		return fmt.Errorf("no comment in event for emoji reaction")
	}

	token, err := FindIntegrationTokenForGitHubRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	return PostGitHubCommentReactionV2(event, token.PatToken, emoji)
}

// PostFullReview posts a comprehensive review to a GitHub PR.
func (p *GitHubV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event for full review")
	}

	token, err := FindIntegrationTokenForGitHubRepo(p.db, event.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

	if overallComment != "" {
		if err := PostGitHubCommentReplyV2(event, token.PatToken, overallComment); err != nil {
			return fmt.Errorf("failed to post overall review comment: %w", err)
		}
	}

	return nil
}

// FetchMRTimeline fetches timeline data for a merge request.
func (p *GitHubV2Provider) FetchMRTimeline(mr UnifiedMergeRequestV2) (*UnifiedTimelineV2, error) {
	token, err := FindIntegrationTokenForGitHubRepo(p.db, mr.Metadata["repository_full_name"].(string))
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	repoFullName := mr.Metadata["repository_full_name"].(string)
	parts := strings.Split(repoFullName, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repository full name: %s", repoFullName)
	}
	owner, repo := parts[0], parts[1]
	prNumber := fmt.Sprintf("%d", mr.Number)

	commits, err := FetchGitHubPRCommitsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch commits: %w", err)
	}

	comments, err := FetchGitHubPRCommentsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch comments: %w", err)
	}

	timeline := &UnifiedTimelineV2{Items: []UnifiedTimelineItemV2{}}

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

// FetchCodeContext fetches code context (diff hunks, file content) for a comment.
func (p *GitHubV2Provider) FetchCodeContext(comment UnifiedCommentV2) (string, error) {
	if comment.Position == nil {
		return "", nil
	}

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

// GetBotUserInfo gets bot user information for warrant checking.
func (p *GitHubV2Provider) GetBotUserInfo(repository UnifiedRepositoryV2) (*UnifiedBotUserInfoV2, error) {
	token, err := FindIntegrationTokenForGitHubRepo(p.db, repository.FullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

	botInfo, err := FetchGitHubBotUserInfo(token.PatToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub bot user info: %w", err)
	}

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

// PostReviewComments posts multiple review comments to a GitHub PR.
func (p *GitHubV2Provider) PostReviewComments(mr UnifiedMergeRequestV2, comments []UnifiedReviewCommentV2) error {
	if len(comments) == 0 {
		return nil
	}

	repoFullName := mr.Metadata["repository_full_name"].(string)
	token, err := FindIntegrationTokenForGitHubRepo(p.db, repoFullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token: %w", err)
	}

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

		if err := PostToGitHubAPIV2(apiURL, token.PatToken, requestBody); err != nil {
			return fmt.Errorf("failed to post review comment: %w", err)
		}
	}

	return nil
}
