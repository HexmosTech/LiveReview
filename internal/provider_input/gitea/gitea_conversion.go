package gitea

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	coreprocessor "github.com/livereview/internal/core_processor"
)

// Type aliases for unified types
type (
	UnifiedWebhookEventV2   = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2   = coreprocessor.UnifiedMergeRequestV2
	UnifiedCommentV2        = coreprocessor.UnifiedCommentV2
	UnifiedUserV2           = coreprocessor.UnifiedUserV2
	UnifiedRepositoryV2     = coreprocessor.UnifiedRepositoryV2
	UnifiedPositionV2       = coreprocessor.UnifiedPositionV2
	UnifiedReviewerChangeV2 = coreprocessor.UnifiedReviewerChangeV2
)

// ConvertGiteaIssueCommentEvent converts a Gitea issue_comment webhook to unified format
// Handles: issue_comment events (created, edited) on issues and pull requests
func ConvertGiteaIssueCommentEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload GiteaV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Gitea issue comment webhook: %w", err)
	}

	if payload.Action != "created" && payload.Action != "edited" {
		log.Printf("[DEBUG] Ignoring Gitea issue_comment action: %s", payload.Action)
		return nil, fmt.Errorf("issue_comment event ignored (action=%s)", payload.Action)
	}

	if payload.Comment == nil {
		return nil, fmt.Errorf("comment is nil in payload")
	}
	if payload.Repository == nil {
		return nil, fmt.Errorf("repository is nil in payload")
	}
	if payload.Sender == nil {
		return nil, fmt.Errorf("sender is nil in payload")
	}

	event := &UnifiedWebhookEventV2{
		EventType:  "comment_created",
		Provider:   "gitea",
		Timestamp:  payload.Comment.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Comment:    convertGiteaCommentToUnified(payload.Comment),
		Repository: convertGiteaRepositoryToUnified(payload.Repository),
		Actor:      convertGiteaUserToUnified(payload.Sender),
	}

	// Check if this is a comment on a PR (not just an issue)
	if payload.Issue != nil && payload.Issue.PullRequest != nil {
		// This is a PR comment - create minimal MR data
		event.MergeRequest = &UnifiedMergeRequestV2{
			ID:          strconv.FormatInt(payload.Issue.ID, 10),
			Number:      int(payload.Issue.Number),
			Title:       payload.Issue.Title,
			Description: payload.Issue.Body,
			State:       payload.Issue.State,
			WebURL:      payload.Issue.HTMLURL,
			Author:      convertGiteaUserToUnified(payload.Issue.User),
			CreatedAt:   payload.Issue.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   payload.Issue.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Metadata:    map[string]interface{}{},
		}
	}

	return event, nil
}

// ConvertGiteaPullRequestReviewCommentEvent converts a pull_request review comment webhook
func ConvertGiteaPullRequestReviewCommentEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload GiteaV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Gitea PR review comment webhook: %w", err)
	}

	if payload.Action != "created" {
		log.Printf("[DEBUG] Ignoring Gitea pull_request_review_comment action: %s", payload.Action)
		return nil, fmt.Errorf("pull_request_review_comment event ignored (action=%s)", payload.Action)
	}

	if payload.Comment == nil {
		return nil, fmt.Errorf("comment is nil in payload")
	}
	if payload.PullRequest == nil {
		return nil, fmt.Errorf("pull_request is nil in payload")
	}
	if payload.Repository == nil {
		return nil, fmt.Errorf("repository is nil in payload")
	}
	if payload.Sender == nil {
		return nil, fmt.Errorf("sender is nil in payload")
	}

	event := &UnifiedWebhookEventV2{
		EventType:    "comment_created",
		Provider:     "gitea",
		Timestamp:    payload.Comment.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Comment:      convertGiteaCommentToUnified(payload.Comment),
		MergeRequest: convertGiteaPullRequestToUnified(payload.PullRequest),
		Repository:   convertGiteaRepositoryToUnified(payload.Repository),
		Actor:        convertGiteaUserToUnified(payload.Sender),
	}

	return event, nil
}

// ConvertGiteaPullRequestEvent converts a pull_request webhook to unified format
func ConvertGiteaPullRequestEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload GiteaV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Gitea pull_request webhook: %w", err)
	}

	if payload.PullRequest == nil {
		return nil, fmt.Errorf("pull_request is nil in payload")
	}
	if payload.Repository == nil {
		return nil, fmt.Errorf("repository is nil in payload")
	}
	if payload.Sender == nil {
		return nil, fmt.Errorf("sender is nil in payload")
	}

	event := &UnifiedWebhookEventV2{
		EventType:    "mr_updated",
		Provider:     "gitea",
		Timestamp:    payload.PullRequest.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		MergeRequest: convertGiteaPullRequestToUnified(payload.PullRequest),
		Repository:   convertGiteaRepositoryToUnified(payload.Repository),
		Actor:        convertGiteaUserToUnified(payload.Sender),
	}

	return event, nil
}

// Helper functions to convert Gitea types to unified types

func convertGiteaCommentToUnified(comment *GiteaV2Comment) *UnifiedCommentV2 {
	if comment == nil {
		return nil
	}

	unified := &UnifiedCommentV2{
		ID:        strconv.FormatInt(comment.ID, 10),
		Body:      comment.Body,
		Author:    convertGiteaUserToUnified(comment.User),
		CreatedAt: comment.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: comment.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		WebURL:    comment.HTMLURL,
		Metadata:  map[string]interface{}{},
	}

	// Check if this is a review comment (inline comment on code)
	if comment.Path != "" {
		unified.Position = &UnifiedPositionV2{
			FilePath:   comment.Path,
			LineNumber: comment.Line,
			LineType:   convertGiteaSideToLineType(comment.Side),
			Metadata:   map[string]interface{}{},
		}

		if comment.StartLine > 0 {
			startLine := comment.StartLine
			unified.Position.StartLine = &startLine
		}

		if comment.DiffHunk != "" {
			unified.Position.Metadata["diff_hunk"] = comment.DiffHunk
		}
		if comment.CommitID != "" {
			unified.Position.Metadata["head_commit_sha"] = comment.CommitID
		}
		if comment.OriginalCommitID != "" {
			unified.Position.Metadata["base_commit_sha"] = comment.OriginalCommitID
		}

		unified.Metadata["comment_type"] = "review_comment"
	} else {
		unified.Metadata["comment_type"] = "issue_comment"
	}

	// Thread information
	if comment.InReplyTo > 0 {
		inReplyTo := strconv.FormatInt(comment.InReplyTo, 10)
		unified.InReplyToID = &inReplyTo
	}

	// Review context
	if comment.ReviewID > 0 {
		unified.Metadata["review_id"] = comment.ReviewID
	}

	return unified
}

func convertGiteaPullRequestToUnified(pr *GiteaV2PullRequest) *UnifiedMergeRequestV2 {
	if pr == nil {
		return nil
	}

	unified := &UnifiedMergeRequestV2{
		ID:          strconv.FormatInt(pr.ID, 10),
		Number:      int(pr.Number),
		Title:       pr.Title,
		Description: pr.Body,
		State:       pr.State,
		WebURL:      pr.HTMLURL,
		Author:      convertGiteaUserToUnified(pr.User),
		CreatedAt:   pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   pr.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Metadata:    map[string]interface{}{},
	}

	if pr.Head != nil {
		unified.SourceBranch = pr.Head.Ref
		unified.Metadata["head_sha"] = pr.Head.SHA
	}

	if pr.Base != nil {
		unified.TargetBranch = pr.Base.Ref
		unified.Metadata["base_sha"] = pr.Base.SHA
	}

	unified.Metadata["merged"] = pr.Merged
	if pr.MergedAt != nil {
		unified.Metadata["merged_at"] = pr.MergedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if pr.ClosedAt != nil {
		unified.Metadata["closed_at"] = pr.ClosedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	unified.Metadata["diff_url"] = pr.DiffURL
	unified.Metadata["patch_url"] = pr.PatchURL

	return unified
}

func convertGiteaRepositoryToUnified(repo *GiteaV2Repository) UnifiedRepositoryV2 {
	if repo == nil {
		return UnifiedRepositoryV2{}
	}

	return UnifiedRepositoryV2{
		ID:            strconv.FormatInt(repo.ID, 10),
		Name:          repo.Name,
		FullName:      repo.FullName,
		WebURL:        repo.HTMLURL,
		CloneURL:      repo.CloneURL,
		DefaultBranch: repo.DefaultBranch,
		Owner:         convertGiteaUserToUnified(repo.Owner),
		Metadata: map[string]interface{}{
			"private": repo.Private,
			"fork":    repo.Fork,
		},
	}
}

func convertGiteaUserToUnified(user *GiteaV2User) UnifiedUserV2 {
	if user == nil {
		return UnifiedUserV2{}
	}

	// Gitea may use 'login' or 'username' field
	username := user.Login
	if username == "" {
		username = user.Username
	}

	return UnifiedUserV2{
		ID:        strconv.FormatInt(user.ID, 10),
		Username:  username,
		Name:      user.FullName,
		Email:     user.Email,
		AvatarURL: user.AvatarURL,
		WebURL:    user.HTMLURL,
		Metadata:  map[string]interface{}{},
	}
}

// convertGiteaSideToLineType maps Gitea's side values to unified line type format
// Gitea uses "LEFT" (old/base) and "RIGHT" (new/head)
// Unified types use "old", "new", "context"
func convertGiteaSideToLineType(side string) string {
	switch strings.ToUpper(side) {
	case "LEFT":
		return "old"
	case "RIGHT":
		return "new"
	default:
		return "new" // Default to new side
	}
}
