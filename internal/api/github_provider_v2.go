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
)

// GitHub V2 Types - All GitHub-specific types with V2 naming to avoid conflicts

// GitHubV2WebhookPayload represents a GitHub webhook payload
type GitHubV2WebhookPayload struct {
	Action      string              `json:"action"`
	Number      int                 `json:"number"`
	PullRequest GitHubV2PullRequest `json:"pull_request"`
	Repository  GitHubV2Repository  `json:"repository"`
	Sender      GitHubV2User        `json:"sender"`
	// For review_requested/review_request_removed actions
	RequestedReviewer GitHubV2User `json:"requested_reviewer,omitempty"`
	RequestedTeam     GitHubV2Team `json:"requested_team,omitempty"`
}

// GitHubV2PullRequest represents a GitHub pull request
type GitHubV2PullRequest struct {
	ID                 int            `json:"id"`
	Number             int            `json:"number"`
	Title              string         `json:"title"`
	Body               string         `json:"body"`
	State              string         `json:"state"`
	HTMLURL            string         `json:"html_url"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
	Head               GitHubV2Branch `json:"head"`
	Base               GitHubV2Branch `json:"base"`
	User               GitHubV2User   `json:"user"`
	RequestedReviewers []GitHubV2User `json:"requested_reviewers"`
	RequestedTeams     []GitHubV2Team `json:"requested_teams"`
	Assignees          []GitHubV2User `json:"assignees"`
}

// GitHubV2Repository represents a GitHub repository
type GitHubV2Repository struct {
	ID       int          `json:"id"`
	Name     string       `json:"name"`
	FullName string       `json:"full_name"`
	HTMLURL  string       `json:"html_url"`
	Owner    GitHubV2User `json:"owner"`
	Private  bool         `json:"private"`
}

// GitHubV2User represents a GitHub user
type GitHubV2User struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// GitHubV2Team represents a GitHub team
type GitHubV2Team struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}

// GitHubV2Branch represents a GitHub branch
type GitHubV2Branch struct {
	Ref  string             `json:"ref"`
	SHA  string             `json:"sha"`
	Repo GitHubV2Repository `json:"repo"`
}

// GitHubV2ReviewerChangeInfo represents reviewer change information
type GitHubV2ReviewerChangeInfo struct {
	Previous []int `json:"previous"`
	Current  []int `json:"current"`
}

// GitHubV2ReviewerBotUserInfo represents reviewer bot user information
type GitHubV2ReviewerBotUserInfo struct {
	ID    int    `json:"id"`
	Login string `json:"login"`
	Type  string `json:"type"`
}

// GitHubV2IssueCommentWebhookPayload represents a GitHub issue comment webhook payload
type GitHubV2IssueCommentWebhookPayload struct {
	Action     string             `json:"action"`
	Issue      GitHubV2Issue      `json:"issue"`
	Comment    GitHubV2Comment    `json:"comment"`
	Repository GitHubV2Repository `json:"repository"`
	Sender     GitHubV2User       `json:"sender"`
}

// GitHubV2PullRequestReviewCommentWebhookPayload represents a GitHub PR review comment webhook payload
type GitHubV2PullRequestReviewCommentWebhookPayload struct {
	Action      string                `json:"action"`
	Comment     GitHubV2ReviewComment `json:"comment"`
	PullRequest GitHubV2PullRequest   `json:"pull_request"`
	Repository  GitHubV2Repository    `json:"repository"`
	Sender      GitHubV2User          `json:"sender"`
}

// GitHubV2Comment represents a GitHub comment
type GitHubV2Comment struct {
	ID                    int          `json:"id"`
	HTMLURL               string       `json:"html_url"`
	IssueURL              string       `json:"issue_url"`
	User                  GitHubV2User `json:"user"`
	CreatedAt             string       `json:"created_at"`
	UpdatedAt             string       `json:"updated_at"`
	AuthorAssociation     string       `json:"author_association"`
	Body                  string       `json:"body"`
	Reactions             interface{}  `json:"reactions"`
	PerformedViaGitHubApp interface{}  `json:"performed_via_github_app"`
}

// GitHubV2ReviewComment represents a GitHub review comment
type GitHubV2ReviewComment struct {
	ID                    int          `json:"id"`
	DiffHunk              string       `json:"diff_hunk"`
	Path                  string       `json:"path"`
	Position              int          `json:"position"`
	OriginalPosition      int          `json:"original_position"`
	CommitID              string       `json:"commit_id"`
	OriginalCommitID      string       `json:"original_commit_id"`
	InReplyToID           *int         `json:"in_reply_to_id"`
	User                  GitHubV2User `json:"user"`
	Body                  string       `json:"body"`
	CreatedAt             string       `json:"created_at"`
	UpdatedAt             string       `json:"updated_at"`
	HTMLURL               string       `json:"html_url"`
	PullRequestURL        string       `json:"pull_request_url"`
	AuthorAssociation     string       `json:"author_association"`
	StartLine             *int         `json:"start_line"`
	OriginalStartLine     *int         `json:"original_start_line"`
	StartSide             string       `json:"start_side"`
	Line                  int          `json:"line"`
	OriginalLine          int          `json:"original_line"`
	Side                  string       `json:"side"`
	Reactions             interface{}  `json:"reactions"`
	PerformedViaGitHubApp interface{}  `json:"performed_via_github_app"`
}

// GitHubV2Issue represents a GitHub issue
type GitHubV2Issue struct {
	ID                int                 `json:"id"`
	Number            int                 `json:"number"`
	Title             string              `json:"title"`
	User              GitHubV2User        `json:"user"`
	Labels            []GitHubV2Label     `json:"labels"`
	State             string              `json:"state"`
	Locked            bool                `json:"locked"`
	Assignee          *GitHubV2User       `json:"assignee"`
	Assignees         []GitHubV2User      `json:"assignees"`
	Milestone         *GitHubV2Milestone  `json:"milestone"`
	Comments          int                 `json:"comments"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
	ClosedAt          *string             `json:"closed_at"`
	AuthorAssociation string              `json:"author_association"`
	ActiveLockReason  *string             `json:"active_lock_reason"`
	Body              string              `json:"body"`
	ClosedBy          *GitHubV2User       `json:"closed_by"`
	HTMLURL           string              `json:"html_url"`
	NodeID            string              `json:"node_id"`
	RepositoryURL     string              `json:"repository_url"`
	PullRequest       *GitHubV2IssuePRRef `json:"pull_request"`
}

// GitHubV2Label represents a GitHub label
type GitHubV2Label struct {
	ID          int     `json:"id"`
	NodeID      string  `json:"node_id"`
	URL         string  `json:"url"`
	Name        string  `json:"name"`
	Color       string  `json:"color"`
	Default     bool    `json:"default"`
	Description *string `json:"description"`
}

// GitHubV2Milestone represents a GitHub milestone
type GitHubV2Milestone struct {
	URL          string       `json:"url"`
	HTMLURL      string       `json:"html_url"`
	LabelsURL    string       `json:"labels_url"`
	ID           int          `json:"id"`
	NodeID       string       `json:"node_id"`
	Number       int          `json:"number"`
	Title        string       `json:"title"`
	Description  *string      `json:"description"`
	Creator      GitHubV2User `json:"creator"`
	OpenIssues   int          `json:"open_issues"`
	ClosedIssues int          `json:"closed_issues"`
	State        string       `json:"state"`
	CreatedAt    string       `json:"created_at"`
	UpdatedAt    string       `json:"updated_at"`
	DueOn        *string      `json:"due_on"`
	ClosedAt     *string      `json:"closed_at"`
}

// GitHubV2IssuePRRef represents a GitHub issue PR reference
type GitHubV2IssuePRRef struct {
	URL      string `json:"url"`
	HTMLURL  string `json:"html_url"`
	DiffURL  string `json:"diff_url"`
	PatchURL string `json:"patch_url"`
}

// GitHubV2BotUserInfo represents GitHub bot user information
type GitHubV2BotUserInfo struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// GitHubV2CommitInfo represents GitHub commit information for timeline
type GitHubV2CommitInfo struct {
	SHA       string               `json:"sha"`
	Message   string               `json:"message"`
	Author    GitHubV2CommitAuthor `json:"author"`
	Committer GitHubV2CommitAuthor `json:"committer"`
	URL       string               `json:"url"`
}

// GitHubV2CommitAuthor represents GitHub commit author
type GitHubV2CommitAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

// GitHubV2CommentInfo represents GitHub comment information for timeline
type GitHubV2CommentInfo struct {
	ID        int          `json:"id"`
	Body      string       `json:"body"`
	User      GitHubV2User `json:"user"`
	CreatedAt string       `json:"created_at"`
	UpdatedAt string       `json:"updated_at"`
}

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
	var payload GitHubV2IssueCommentWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub issue comment webhook: %w", err)
	}

	// Convert to unified format
	unifiedEvent := &UnifiedWebhookEventV2{
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

	// Check if this is a PR comment (issue with pull_request field)
	if payload.Issue.PullRequest != nil {
		unifiedEvent.MergeRequest = &UnifiedMergeRequestV2{
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

	return unifiedEvent, nil
}

// convertPullRequestReviewCommentEventV2 converts GitHub PR review comment webhook to unified format
func (p *GitHubV2Provider) convertPullRequestReviewCommentEventV2(body []byte) (*UnifiedWebhookEventV2, error) {
	var payload GitHubV2PullRequestReviewCommentWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub PR review comment webhook: %w", err)
	}

	// Convert to unified format
	unifiedEvent := &UnifiedWebhookEventV2{
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

	// Add position information for review comments
	if payload.Comment.Path != "" {
		unifiedEvent.Comment.Position = &UnifiedPositionV2{
			FilePath:   payload.Comment.Path,
			LineNumber: payload.Comment.Line,
		}

		if payload.Comment.StartLine != nil && *payload.Comment.StartLine != 0 {
			unifiedEvent.Comment.Position.StartLine = payload.Comment.StartLine
		}
	}

	// Add in_reply_to information
	if payload.Comment.InReplyToID != nil {
		inReplyToStr := fmt.Sprintf("%d", *payload.Comment.InReplyToID)
		unifiedEvent.Comment.InReplyToID = &inReplyToStr
	}

	return unifiedEvent, nil
}

// convertPullRequestReviewEventV2 converts GitHub pull request review webhook to unified format
func (p *GitHubV2Provider) convertPullRequestReviewEventV2(body []byte) (*UnifiedWebhookEventV2, error) {
	// For now, treat pull_request_review as a comment event if it has a comment body
	// This is a simplified approach - full implementation would need proper review handling
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

	// Only handle reviews with actual comment text
	if payload.Action != "submitted" || payload.Review.Body == "" {
		log.Printf("[DEBUG] Ignoring pull_request_review: action=%s, has_body=%t", payload.Action, payload.Review.Body != "")
		return nil, fmt.Errorf("pull_request_review event ignored (action=%s, no comment body)", payload.Action)
	}

	// Convert to unified event (similar to issue comment)
	unifiedEvent := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "github",
		Timestamp: time.Now().Format(time.RFC3339),
		Comment: &UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", payload.Review.ID),
			Body:      payload.Review.Body,
			CreatedAt: time.Now().Format(time.RFC3339),
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

	return unifiedEvent, nil
}

// ConvertReviewerEvent converts GitHub reviewer assignment webhook to unified format
func (p *GitHubV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	// GitHub uses X-GitHub-Event header, try different case variations
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

	// Check if this is a reviewer assignment event
	if payload.Action != "review_requested" && payload.Action != "review_request_removed" {
		return nil, fmt.Errorf("not a reviewer assignment event: action=%s", payload.Action)
	}

	// Convert to unified format
	unifiedEvent := &UnifiedWebhookEventV2{
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
	}

	// Add reviewer change information
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

	unifiedEvent.ReviewerChange = &UnifiedReviewerChangeV2{
		Action:           action,
		CurrentReviewers: currentReviewers,
		ChangedBy: UnifiedUserV2{
			ID:        fmt.Sprintf("%d", payload.Sender.ID),
			Username:  payload.Sender.Login,
			Name:      payload.Sender.Name,
			WebURL:    payload.Sender.HTMLURL,
			AvatarURL: payload.Sender.AvatarURL,
		},
	}

	return unifiedEvent, nil
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
