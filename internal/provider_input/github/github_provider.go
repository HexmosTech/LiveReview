package github

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/internal/capture"
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

// GitHubOutputClient captures the outbound capabilities required by the provider.
type GitHubOutputClient interface {
	PostCommentReply(event *UnifiedWebhookEventV2, token, content string) error
	PostEmojiReaction(event *UnifiedWebhookEventV2, token, emoji string) error
	PostReviewComments(mr UnifiedMergeRequestV2, token string, comments []UnifiedReviewCommentV2) error
}

// GitHubV2Provider implements webhook provider behaviour for GitHub.
type GitHubV2Provider struct {
	db     *sql.DB
	output GitHubOutputClient
}

// NewGitHubV2Provider creates a GitHub provider with the required dependencies.
func NewGitHubV2Provider(db *sql.DB, output GitHubOutputClient) *GitHubV2Provider {
	if output == nil {
		panic("github output client is required")
	}
	return &GitHubV2Provider{db: db, output: output}
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

	canonicalType := canonicalGitHubEventType(eventType)
	var (
		event *UnifiedWebhookEventV2
		err   error
	)

	switch eventType {
	case "issue_comment":
		log.Printf("[DEBUG] Processing GitHub issue_comment event")
		event, err = ConvertIssueCommentEvent(body)
	case "pull_request_review_comment":
		log.Printf("[DEBUG] Processing GitHub pull_request_review_comment event")
		event, err = ConvertPullRequestReviewCommentEvent(body)
	case "pull_request_review":
		log.Printf("[DEBUG] Processing GitHub pull_request_review event")
		event, err = ConvertPullRequestReviewEvent(body)
	default:
		log.Printf("[WARN] Unsupported GitHub comment event type: '%s' (supported: issue_comment, pull_request_review_comment, pull_request_review)", eventType)
		err = fmt.Errorf("unsupported GitHub comment event type: '%s'", eventType)
	}

	if capture.Enabled() {
		recordGitHubWebhook(canonicalType, headers, body, event, err)
	}

	if err != nil {
		return nil, err
	}
	return event, nil
}

// ConvertReviewerEvent converts GitHub reviewer assignment webhook to unified format.
func (p *GitHubV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	event, err := ConvertReviewerEvent(headers, body)
	if capture.Enabled() {
		recordGitHubWebhook("reviewer", headers, body, event, err)
	}
	return event, err
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

	commits, err := FetchGitHubPRCommitsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	issueComments, err := FetchGitHubPRCommentsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return fmt.Errorf("failed to get comments: %w", err)
	}

	reviewComments, err := FetchGitHubPRReviewCommentsV2(owner, repo, prNumber, token.PatToken)
	if err != nil {
		return fmt.Errorf("failed to get review comments: %w", err)
	}

	if event.MergeRequest.Metadata == nil {
		event.MergeRequest.Metadata = map[string]interface{}{}
	}

	unifiedCommits := convertGitHubCommitsToUnified(commits)
	unifiedComments := convertGitHubCommentsToUnified(issueComments, reviewComments)

	event.MergeRequest.Metadata["timeline_commits"] = unifiedCommits
	event.MergeRequest.Metadata["timeline_comments"] = unifiedComments
	event.MergeRequest.Metadata["repository_full_name"] = event.Repository.FullName
	event.MergeRequest.Metadata["pull_request_number"] = event.MergeRequest.Number

	log.Printf("[INFO] Successfully fetched PR data for GitHub PR %s (commits=%d, comments=%d, review_comments=%d)",
		prNumber, len(unifiedCommits), len(issueComments), len(reviewComments))
	return nil
}

// FindIntegrationTokenForRepo returns the integration token associated with the given repository.
func (p *GitHubV2Provider) FindIntegrationTokenForRepo(repoFullName string) (*IntegrationToken, error) {
	if p == nil {
		return nil, fmt.Errorf("github provider not initialised")
	}
	if p.db == nil {
		return nil, fmt.Errorf("github provider missing database handle")
	}

	return FindIntegrationTokenForGitHubRepo(p.db, repoFullName)
}

func recordGitHubWebhook(eventType string, headers map[string]string, body []byte, unified *UnifiedWebhookEventV2, err error) {
	if eventType == "" {
		eventType = "unknown"
	}

	if len(body) > 0 {
		capture.WriteBlob(fmt.Sprintf("github-webhook-%s-body", eventType), "json", body)
	}

	sanitized := sanitizeHeaders(headers)
	meta := map[string]interface{}{
		"event_type": eventType,
		"headers":    sanitized,
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	capture.WriteJSON(fmt.Sprintf("github-webhook-%s-meta", eventType), meta)

	if unified != nil && err == nil {
		capture.WriteJSON(fmt.Sprintf("github-webhook-%s-unified", eventType), unified)
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
		if strings.EqualFold(k, "x-hub-signature") {
			continue
		}
		if strings.EqualFold(k, "x-hub-signature-256") {
			continue
		}
		sanitized[k] = v
	}
	return sanitized
}

func canonicalGitHubEventType(eventType string) string {
	if eventType == "" {
		return "unknown"
	}
	return strings.ToLower(eventType)
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

	return p.output.PostCommentReply(event, token.PatToken, content)
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

	return p.output.PostEmojiReaction(event, token.PatToken, emoji)
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
		if err := p.output.PostCommentReply(event, token.PatToken, overallComment); err != nil {
			return fmt.Errorf("failed to post overall review comment: %w", err)
		}
	}

	return nil
}

// FetchMRTimeline fetches timeline data for a merge request.
func (p *GitHubV2Provider) FetchMRTimeline(mr UnifiedMergeRequestV2) (*UnifiedTimelineV2, error) {
	repoFullName, err := extractRepoFullNameFromMetadata(mr.Metadata)
	if err != nil {
		return nil, err
	}

	token, err := FindIntegrationTokenForGitHubRepo(p.db, repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub token: %w", err)
	}

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

	return p.output.PostReviewComments(mr, token.PatToken, comments)
}

func convertGitHubCommitsToUnified(commits []GitHubV2CommitInfo) []UnifiedCommitV2 {
	if len(commits) == 0 {
		return nil
	}

	unified := make([]UnifiedCommitV2, 0, len(commits))
	for _, commit := range commits {
		unified = append(unified, UnifiedCommitV2{
			SHA:     commit.SHA,
			Message: commit.Message,
			Author: UnifiedCommitAuthorV2{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
			},
			Timestamp: commit.Author.Date,
			WebURL:    commit.URL,
		})
	}
	return unified
}

func convertGitHubCommentsToUnified(issueComments []GitHubV2CommentInfo, reviewComments []GitHubV2ReviewComment) []UnifiedCommentV2 {
	total := len(issueComments) + len(reviewComments)
	if total == 0 {
		return nil
	}

	unified := make([]UnifiedCommentV2, 0, total)

	for _, comment := range issueComments {
		unified = append(unified, UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", comment.ID),
			Body:      comment.Body,
			Author:    UnifiedUserV2{ID: fmt.Sprintf("%d", comment.User.ID), Username: comment.User.Login, Name: comment.User.Name, WebURL: comment.User.HTMLURL, AvatarURL: comment.User.AvatarURL},
			CreatedAt: comment.CreatedAt,
			UpdatedAt: comment.UpdatedAt,
			Metadata: map[string]interface{}{
				"comment_type": "issue_comment",
			},
		})
	}

	for _, comment := range reviewComments {
		metadata := map[string]interface{}{
			"comment_type": "review_comment",
		}
		if comment.DiffHunk != "" {
			metadata["diff_hunk"] = comment.DiffHunk
		}
		if comment.CommitID != "" {
			metadata["commit_id"] = comment.CommitID
		}
		if comment.OriginalCommitID != "" {
			metadata["original_commit_id"] = comment.OriginalCommitID
		}
		if comment.PullRequestReviewID != 0 {
			metadata["pull_request_review_id"] = comment.PullRequestReviewID
			metadata["thread_id"] = fmt.Sprintf("%d", comment.PullRequestReviewID)
		}
		if comment.InReplyToID != nil {
			metadata["parent_id"] = fmt.Sprintf("%d", *comment.InReplyToID)
		}

		var position *UnifiedPositionV2
		if comment.Path != "" {
			lineNumber := comment.Line
			if lineNumber == 0 {
				lineNumber = comment.OriginalLine
			}

			lineType := strings.ToLower(comment.Side)
			switch lineType {
			case "right":
				lineType = "new"
			case "left":
				lineType = "old"
			}

			position = &UnifiedPositionV2{
				FilePath:   comment.Path,
				LineNumber: lineNumber,
				LineType:   lineType,
			}
			if comment.StartLine != nil {
				position.StartLine = comment.StartLine
			}
		}

		unifiedComment := UnifiedCommentV2{
			ID:   fmt.Sprintf("%d", comment.ID),
			Body: comment.Body,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", comment.User.ID),
				Username:  comment.User.Login,
				Name:      comment.User.Name,
				WebURL:    comment.User.HTMLURL,
				AvatarURL: comment.User.AvatarURL,
			},
			CreatedAt: comment.CreatedAt,
			UpdatedAt: comment.UpdatedAt,
			Metadata:  metadata,
			Position:  position,
		}

		if comment.InReplyToID != nil {
			replyID := fmt.Sprintf("%d", *comment.InReplyToID)
			unifiedComment.InReplyToID = &replyID
		}
		if comment.PullRequestReviewID != 0 {
			discussionID := fmt.Sprintf("%d", comment.PullRequestReviewID)
			unifiedComment.DiscussionID = &discussionID
		}

		unified = append(unified, unifiedComment)
	}

	return unified
}

func extractRepoFullNameFromMetadata(metadata map[string]interface{}) (string, error) {
	if metadata == nil {
		return "", fmt.Errorf("merge request metadata missing repository_full_name")
	}

	if value, ok := metadata["repository_full_name"]; ok {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return typed, nil
			}
		case fmt.Stringer:
			if s := strings.TrimSpace(typed.String()); s != "" {
				return s, nil
			}
		}
	}

	return "", fmt.Errorf("repository_full_name missing from metadata")
}
