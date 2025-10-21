package github

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	coreprocessor "github.com/livereview/internal/core_processor"
)

type (
	UnifiedWebhookEventV2   = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2   = coreprocessor.UnifiedMergeRequestV2
	UnifiedCommentV2        = coreprocessor.UnifiedCommentV2
	UnifiedUserV2           = coreprocessor.UnifiedUserV2
	UnifiedRepositoryV2     = coreprocessor.UnifiedRepositoryV2
	UnifiedPositionV2       = coreprocessor.UnifiedPositionV2
	UnifiedReviewerChangeV2 = coreprocessor.UnifiedReviewerChangeV2
)

// ConvertIssueCommentEvent transforms a GitHub issue_comment payload into a unified event.
func ConvertIssueCommentEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload GitHubV2IssueCommentWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub issue comment webhook: %w", err)
	}

	if payload.Action != "created" {
		log.Printf("[DEBUG] Ignoring GitHub issue_comment action: %s", payload.Action)
		return nil, fmt.Errorf("issue_comment event ignored (action=%s)", payload.Action)
	}

	event := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "github",
		Timestamp: payload.Comment.CreatedAt,
		Comment: &UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", payload.Comment.ID),
			Body:      payload.Comment.Body,
			CreatedAt: payload.Comment.CreatedAt,
			UpdatedAt: payload.Comment.UpdatedAt,
			WebURL:    payload.Comment.HTMLURL,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Comment.User.ID),
				Username:  payload.Comment.User.Login,
				Name:      payload.Comment.User.Name,
				WebURL:    payload.Comment.User.HTMLURL,
				AvatarURL: payload.Comment.User.AvatarURL,
			},
			Metadata: map[string]interface{}{
				"comment_type": "issue_comment",
			},
		},
		Repository: UnifiedRepositoryV2{
			ID:       fmt.Sprintf("%d", payload.Repository.ID),
			Name:     payload.Repository.Name,
			FullName: payload.Repository.FullName,
			WebURL:   payload.Repository.HTMLURL,
			Owner: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Repository.Owner.ID),
				Username:  payload.Repository.Owner.Login,
				Name:      payload.Repository.Owner.Name,
				WebURL:    payload.Repository.Owner.HTMLURL,
				AvatarURL: payload.Repository.Owner.AvatarURL,
			},
		},
		Actor: UnifiedUserV2{
			ID:        fmt.Sprintf("%d", payload.Sender.ID),
			Username:  payload.Sender.Login,
			Name:      payload.Sender.Name,
			WebURL:    payload.Sender.HTMLURL,
			AvatarURL: payload.Sender.AvatarURL,
		},
	}

	if payload.Issue.PullRequest != nil {
		event.MergeRequest = &UnifiedMergeRequestV2{
			ID:          fmt.Sprintf("%d", payload.Issue.ID),
			Number:      payload.Issue.Number,
			Title:       payload.Issue.Title,
			Description: payload.Issue.Body,
			State:       payload.Issue.State,
			CreatedAt:   payload.Issue.CreatedAt,
			UpdatedAt:   payload.Issue.UpdatedAt,
			WebURL:      payload.Issue.HTMLURL,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Issue.User.ID),
				Username:  payload.Issue.User.Login,
				Name:      payload.Issue.User.Name,
				WebURL:    payload.Issue.User.HTMLURL,
				AvatarURL: payload.Issue.User.AvatarURL,
			},
		}
	}

	return event, nil
}

// ConvertPullRequestReviewCommentEvent transforms a pull_request_review_comment payload into a unified event.
func ConvertPullRequestReviewCommentEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload GitHubV2PullRequestReviewCommentWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub PR review comment webhook: %w", err)
	}

	if payload.Action != "created" {
		log.Printf("[DEBUG] Ignoring GitHub pull_request_review_comment action: %s", payload.Action)
		return nil, fmt.Errorf("pull_request_review_comment event ignored (action=%s)", payload.Action)
	}

	event := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "github",
		Timestamp: payload.Comment.CreatedAt,
		Comment: &UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", payload.Comment.ID),
			Body:      payload.Comment.Body,
			CreatedAt: payload.Comment.CreatedAt,
			UpdatedAt: payload.Comment.UpdatedAt,
			WebURL:    payload.Comment.HTMLURL,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Comment.User.ID),
				Username:  payload.Comment.User.Login,
				Name:      payload.Comment.User.Name,
				WebURL:    payload.Comment.User.HTMLURL,
				AvatarURL: payload.Comment.User.AvatarURL,
			},
			Metadata: map[string]interface{}{
				"comment_type":           "review_comment",
				"pull_request_review_id": payload.Comment.PullRequestReviewID,
			},
		},
		MergeRequest: &UnifiedMergeRequestV2{
			ID:           fmt.Sprintf("%d", payload.PullRequest.ID),
			Number:       payload.PullRequest.Number,
			Title:        payload.PullRequest.Title,
			Description:  payload.PullRequest.Body,
			State:        payload.PullRequest.State,
			CreatedAt:    payload.PullRequest.CreatedAt,
			UpdatedAt:    payload.PullRequest.UpdatedAt,
			WebURL:       payload.PullRequest.HTMLURL,
			TargetBranch: payload.PullRequest.Base.Ref,
			SourceBranch: payload.PullRequest.Head.Ref,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.PullRequest.User.ID),
				Username:  payload.PullRequest.User.Login,
				Name:      payload.PullRequest.User.Name,
				WebURL:    payload.PullRequest.User.HTMLURL,
				AvatarURL: payload.PullRequest.User.AvatarURL,
			},
		},
		Repository: UnifiedRepositoryV2{
			ID:       fmt.Sprintf("%d", payload.Repository.ID),
			Name:     payload.Repository.Name,
			FullName: payload.Repository.FullName,
			WebURL:   payload.Repository.HTMLURL,
			Owner: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Repository.Owner.ID),
				Username:  payload.Repository.Owner.Login,
				Name:      payload.Repository.Owner.Name,
				WebURL:    payload.Repository.Owner.HTMLURL,
				AvatarURL: payload.Repository.Owner.AvatarURL,
			},
		},
		Actor: UnifiedUserV2{
			ID:        fmt.Sprintf("%d", payload.Sender.ID),
			Username:  payload.Sender.Login,
			Name:      payload.Sender.Name,
			WebURL:    payload.Sender.HTMLURL,
			AvatarURL: payload.Sender.AvatarURL,
		},
	}

	if payload.Comment.Path != "" {
		event.Comment.Position = &UnifiedPositionV2{
			FilePath:   payload.Comment.Path,
			LineNumber: payload.Comment.Line,
		}

		if payload.Comment.StartLine != nil && *payload.Comment.StartLine != 0 {
			event.Comment.Position.StartLine = payload.Comment.StartLine
		}
	}

	if payload.Comment.InReplyToID != nil {
		inReplyToStr := fmt.Sprintf("%d", *payload.Comment.InReplyToID)
		event.Comment.InReplyToID = &inReplyToStr
		discussionID := inReplyToStr
		event.Comment.DiscussionID = &discussionID
		if event.Comment.Metadata == nil {
			event.Comment.Metadata = map[string]interface{}{}
		}
		event.Comment.Metadata["parent_id"] = inReplyToStr
	} else if payload.Comment.PullRequestReviewID != 0 {
		discussionID := fmt.Sprintf("review-%d", payload.Comment.PullRequestReviewID)
		event.Comment.DiscussionID = &discussionID
	}

	if event.Comment.Metadata == nil {
		event.Comment.Metadata = map[string]interface{}{}
	}
	if payload.Comment.DiffHunk != "" {
		event.Comment.Metadata["diff_hunk"] = payload.Comment.DiffHunk
	}
	if payload.Comment.PullRequestReviewID != 0 {
		event.Comment.Metadata["pull_request_review_id"] = payload.Comment.PullRequestReviewID
		event.Comment.Metadata["thread_id"] = fmt.Sprintf("%d", payload.Comment.PullRequestReviewID)
	}

	return event, nil
}

// ConvertPullRequestReviewEvent handles pull_request_review payloads and emits comment events when possible.
func ConvertPullRequestReviewEvent(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload struct {
		Action string `json:"action"`
		Review struct {
			ID   int          `json:"id"`
			Body string       `json:"body"`
			User GitHubV2User `json:"user"`
		} `json:"review"`
		PullRequest GitHubV2PullRequest `json:"pull_request"`
		Repository  GitHubV2Repository  `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub pull_request_review webhook: %w", err)
	}

	if payload.Action != "submitted" || payload.Review.Body == "" {
		log.Printf("[DEBUG] Ignoring pull_request_review: action=%s, has_body=%t", payload.Action, payload.Review.Body != "")
		return nil, fmt.Errorf("pull_request_review event ignored (action=%s, no comment body)", payload.Action)
	}

	timestamp := time.Now().Format(time.RFC3339)

	event := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "github",
		Timestamp: timestamp,
		Comment: &UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", payload.Review.ID),
			Body:      payload.Review.Body,
			CreatedAt: timestamp,
			WebURL:    payload.PullRequest.HTMLURL,
			Author: UnifiedUserV2{
				ID:       fmt.Sprintf("%d", payload.Review.User.ID),
				Username: payload.Review.User.Login,
				Name:     payload.Review.User.Login,
				WebURL:   payload.Review.User.HTMLURL,
			},
		},
		MergeRequest: &UnifiedMergeRequestV2{
			ID:           fmt.Sprintf("%d", payload.PullRequest.ID),
			Number:       payload.PullRequest.Number,
			Title:        payload.PullRequest.Title,
			Description:  payload.PullRequest.Body,
			State:        payload.PullRequest.State,
			WebURL:       payload.PullRequest.HTMLURL,
			TargetBranch: payload.PullRequest.Base.Ref,
			SourceBranch: payload.PullRequest.Head.Ref,
			Author: UnifiedUserV2{
				ID:       fmt.Sprintf("%d", payload.PullRequest.User.ID),
				Username: payload.PullRequest.User.Login,
				Name:     payload.PullRequest.User.Login,
				WebURL:   payload.PullRequest.User.HTMLURL,
			},
		},
		Repository: UnifiedRepositoryV2{
			ID:       fmt.Sprintf("%d", payload.Repository.ID),
			Name:     payload.Repository.Name,
			FullName: payload.Repository.FullName,
			WebURL:   payload.Repository.HTMLURL,
			Owner: UnifiedUserV2{
				Username: payload.Repository.Owner.Login,
			},
		},
		Actor: UnifiedUserV2{
			ID:       fmt.Sprintf("%d", payload.Review.User.ID),
			Username: payload.Review.User.Login,
			Name:     payload.Review.User.Login,
			WebURL:   payload.Review.User.HTMLURL,
		},
	}

	return event, nil
}

// ConvertReviewerEvent transforms reviewer assignment payloads into a unified event.
func ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	eventType := headers["X-GitHub-Event"]
	if eventType == "" {
		eventType = headers["X-Github-Event"]
	}
	if eventType == "" {
		eventType = headers["x-github-event"]
	}
	log.Printf("[DEBUG] GitHub reviewer event type: '%s'", eventType)
	log.Printf("[DEBUG] Available headers: %v", headers)

	var payload GitHubV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub webhook: %w", err)
	}

	log.Printf("[DEBUG] GitHub webhook action: %s", payload.Action)

	if payload.Action != "review_requested" && payload.Action != "review_request_removed" {
		return nil, fmt.Errorf("not a reviewer assignment event: action=%s", payload.Action)
	}

	action := "added"
	if payload.Action == "review_request_removed" {
		action = "removed"
	}

	var currentReviewers []UnifiedUserV2
	for _, reviewer := range payload.PullRequest.RequestedReviewers {
		currentReviewers = append(currentReviewers, UnifiedUserV2{
			ID:        fmt.Sprintf("%d", reviewer.ID),
			Username:  reviewer.Login,
			Name:      reviewer.Name,
			WebURL:    reviewer.HTMLURL,
			AvatarURL: reviewer.AvatarURL,
		})
	}

	event := &UnifiedWebhookEventV2{
		EventType: "reviewer_assigned",
		Provider:  "github",
		Timestamp: payload.PullRequest.UpdatedAt,
		MergeRequest: &UnifiedMergeRequestV2{
			ID:           fmt.Sprintf("%d", payload.PullRequest.ID),
			Number:       payload.PullRequest.Number,
			Title:        payload.PullRequest.Title,
			Description:  payload.PullRequest.Body,
			State:        payload.PullRequest.State,
			CreatedAt:    payload.PullRequest.CreatedAt,
			UpdatedAt:    payload.PullRequest.UpdatedAt,
			WebURL:       payload.PullRequest.HTMLURL,
			TargetBranch: payload.PullRequest.Base.Ref,
			SourceBranch: payload.PullRequest.Head.Ref,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.PullRequest.User.ID),
				Username:  payload.PullRequest.User.Login,
				Name:      payload.PullRequest.User.Name,
				WebURL:    payload.PullRequest.User.HTMLURL,
				AvatarURL: payload.PullRequest.User.AvatarURL,
			},
		},
		Repository: UnifiedRepositoryV2{
			ID:       fmt.Sprintf("%d", payload.Repository.ID),
			Name:     payload.Repository.Name,
			FullName: payload.Repository.FullName,
			WebURL:   payload.Repository.HTMLURL,
			Owner: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Repository.Owner.ID),
				Username:  payload.Repository.Owner.Login,
				Name:      payload.Repository.Owner.Name,
				WebURL:    payload.Repository.Owner.HTMLURL,
				AvatarURL: payload.Repository.Owner.AvatarURL,
			},
		},
		Actor: UnifiedUserV2{
			ID:        fmt.Sprintf("%d", payload.Sender.ID),
			Username:  payload.Sender.Login,
			Name:      payload.Sender.Name,
			WebURL:    payload.Sender.HTMLURL,
			AvatarURL: payload.Sender.AvatarURL,
		},
		ReviewerChange: &UnifiedReviewerChangeV2{
			Action:           action,
			CurrentReviewers: currentReviewers,
			ChangedBy: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.Sender.ID),
				Username:  payload.Sender.Login,
				Name:      payload.Sender.Name,
				WebURL:    payload.Sender.HTMLURL,
				AvatarURL: payload.Sender.AvatarURL,
			},
		},
	}

	return event, nil
}
