package gitlab

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/capture"
	coreprocessor "github.com/livereview/internal/core_processor"
)

type (
	UnifiedWebhookEventV2   = coreprocessor.UnifiedWebhookEventV2
	UnifiedMergeRequestV2   = coreprocessor.UnifiedMergeRequestV2
	UnifiedCommentV2        = coreprocessor.UnifiedCommentV2
	UnifiedUserV2           = coreprocessor.UnifiedUserV2
	UnifiedRepositoryV2     = coreprocessor.UnifiedRepositoryV2
	UnifiedPositionV2       = coreprocessor.UnifiedPositionV2
	UnifiedTimelineV2       = coreprocessor.UnifiedTimelineV2
	UnifiedTimelineItemV2   = coreprocessor.UnifiedTimelineItemV2
	UnifiedCommitV2         = coreprocessor.UnifiedCommitV2
	UnifiedCommitAuthorV2   = coreprocessor.UnifiedCommitAuthorV2
	UnifiedBotUserInfoV2    = coreprocessor.UnifiedBotUserInfoV2
	UnifiedReviewCommentV2  = coreprocessor.UnifiedReviewCommentV2
	UnifiedReviewerChangeV2 = coreprocessor.UnifiedReviewerChangeV2
	ResponseScenarioV2      = coreprocessor.ResponseScenarioV2
)

// GitLabOutputClient captures the outbound actions required by the provider.
type GitLabOutputClient interface {
	PostCommentReply(event *UnifiedWebhookEventV2, accessToken, gitlabInstanceURL, content string) error
	PostEmojiReaction(event *UnifiedWebhookEventV2, accessToken, gitlabInstanceURL, emoji string) error
	PostFullReview(event *UnifiedWebhookEventV2, accessToken, gitlabInstanceURL, overallComment string) error
}

// GitLab V2 Types - All GitLab-specific types with V2 naming to avoid conflicts

// GitLabV2WebhookPayload represents a GitLab webhook payload
type GitLabV2WebhookPayload struct {
	ObjectKind       string                   `json:"object_kind"`
	User             GitLabV2User             `json:"user"`
	Project          GitLabV2Project          `json:"project"`
	Repository       GitLabV2Repository       `json:"repository"`
	MergeRequest     GitLabV2MergeRequest     `json:"merge_request"`
	ObjectAttributes GitLabV2ObjectAttributes `json:"object_attributes"`
	Changes          GitLabV2Changes          `json:"changes"`
}

// GitLabV2NoteWebhookPayload represents a GitLab note webhook payload
type GitLabV2NoteWebhookPayload struct {
	ObjectKind       string                       `json:"object_kind"`
	User             GitLabV2User                 `json:"user"`
	Project          GitLabV2Project              `json:"project"`
	Repository       GitLabV2Repository           `json:"repository"`
	ObjectAttributes GitLabV2NoteObjectAttributes `json:"object_attributes"`
	MergeRequest     GitLabV2MergeRequest         `json:"merge_request"`
	Issue            GitLabV2Issue                `json:"issue"`
	Snippet          GitLabV2Snippet              `json:"snippet"`
	Commit           GitLabV2Commit               `json:"commit"`
}

// GitLabV2NoteObjectAttributes represents note-specific attributes
type GitLabV2NoteObjectAttributes struct {
	ID           int                   `json:"id"`
	Note         string                `json:"note"`
	NoteableType string                `json:"noteable_type"`
	AuthorID     int                   `json:"author_id"`
	CreatedAt    string                `json:"created_at"`
	UpdatedAt    string                `json:"updated_at"`
	ProjectID    int                   `json:"project_id"`
	Attachment   string                `json:"attachment"`
	LineCode     string                `json:"line_code"`
	CommitID     string                `json:"commit_id"`
	NoteableID   int                   `json:"noteable_id"`
	System       bool                  `json:"system"`
	StDiff       string                `json:"st_diff"`
	URL          string                `json:"url"`
	DiscussionID string                `json:"discussion_id"`
	Type         string                `json:"type"`
	Position     *GitLabV2NotePosition `json:"position"`
}

// GitLabV2Repository represents a GitLab repository
type GitLabV2Repository struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Description     string `json:"description"`
	Homepage        string `json:"homepage"`
	GitHTTPURL      string `json:"git_http_url"`
	GitSSHURL       string `json:"git_ssh_url"`
	VisibilityLevel int    `json:"visibility_level"`
}

// GitLabV2Issue represents a GitLab issue
type GitLabV2Issue struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	AssigneeID  int          `json:"assignee_id"`
	AuthorID    int          `json:"author_id"`
	ProjectID   int          `json:"project_id"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	Position    int          `json:"position"`
	BranchName  string       `json:"branch_name"`
	Description string       `json:"description"`
	MilestoneID int          `json:"milestone_id"`
	State       string       `json:"state"`
	IID         int          `json:"iid"`
	URL         string       `json:"url"`
	Action      string       `json:"action"`
	Assignee    GitLabV2User `json:"assignee"`
	Author      GitLabV2User `json:"author"`
}

// GitLabV2Commit represents a GitLab commit
type GitLabV2Commit struct {
	ID        string         `json:"id"`
	Message   string         `json:"message"`
	Timestamp string         `json:"timestamp"`
	URL       string         `json:"url"`
	Author    GitLabV2Author `json:"author"`
	Added     []string       `json:"added"`
	Modified  []string       `json:"modified"`
	Removed   []string       `json:"removed"`
}

// GitLabV2Author represents a GitLab commit author
type GitLabV2Author struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GitLabV2Snippet represents a GitLab snippet
type GitLabV2Snippet struct {
	ID              int          `json:"id"`
	Title           string       `json:"title"`
	Content         string       `json:"content"`
	AuthorID        int          `json:"author_id"`
	ProjectID       int          `json:"project_id"`
	CreatedAt       string       `json:"created_at"`
	UpdatedAt       string       `json:"updated_at"`
	FileName        string       `json:"file_name"`
	ExpiresAt       string       `json:"expires_at"`
	Type            string       `json:"type"`
	VisibilityLevel int          `json:"visibility_level"`
	Author          GitLabV2User `json:"author"`
}

// GitLabV2User represents a GitLab user
type GitLabV2User struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	Email     string `json:"email"`
}

// GitLabV2Project represents a GitLab project
type GitLabV2Project struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	WebURL            string `json:"web_url"`
	AvatarURL         string `json:"avatar_url"`
	GitSSHURL         string `json:"git_ssh_url"`
	GitHTTPURL        string `json:"git_http_url"`
	Namespace         string `json:"namespace"`
	VisibilityLevel   int    `json:"visibility_level"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
	Homepage          string `json:"homepage"`
	URL               string `json:"url"`
	SSHURL            string `json:"ssh_url"`
	HTTPURL           string `json:"http_url"`
}

// GitLabV2ObjectAttributes represents merge request attributes
type GitLabV2ObjectAttributes struct {
	ID                        int                      `json:"id"`
	TargetBranch              string                   `json:"target_branch"`
	SourceBranch              string                   `json:"source_branch"`
	SourceProjectID           int                      `json:"source_project_id"`
	AuthorID                  int                      `json:"author_id"`
	AssigneeID                int                      `json:"assignee_id"`
	Title                     string                   `json:"title"`
	CreatedAt                 string                   `json:"created_at"`
	UpdatedAt                 string                   `json:"updated_at"`
	MilestoneID               int                      `json:"milestone_id"`
	State                     string                   `json:"state"`
	MergeStatus               string                   `json:"merge_status"`
	TargetProjectID           int                      `json:"target_project_id"`
	IID                       int                      `json:"iid"`
	Description               string                   `json:"description"`
	Position                  int                      `json:"position"`
	LockedAt                  string                   `json:"locked_at"`
	UpdatedByID               int                      `json:"updated_by_id"`
	MergeError                string                   `json:"merge_error"`
	MergeParams               map[string]interface{}   `json:"merge_params"`
	MergeWhenPipelineSucceeds bool                     `json:"merge_when_pipeline_succeeds"`
	MergeUserID               int                      `json:"merge_user_id"`
	MergeCommitSHA            string                   `json:"merge_commit_sha"`
	DeletedAt                 string                   `json:"deleted_at"`
	InProgressMergeCommitSHA  string                   `json:"in_progress_merge_commit_sha"`
	LockVersion               int                      `json:"lock_version"`
	ApprovalsBeforeMerge      int                      `json:"approvals_before_merge"`
	RebaseCommitSHA           string                   `json:"rebase_commit_sha"`
	TimeEstimate              int                      `json:"time_estimate"`
	Squash                    bool                     `json:"squash"`
	Source                    GitLabV2ProjectReference `json:"source"`
	Target                    GitLabV2ProjectReference `json:"target"`
	LastCommit                GitLabV2Commit           `json:"last_commit"`
	WorkInProgress            bool                     `json:"work_in_progress"`
	URL                       string                   `json:"url"`
	Action                    string                   `json:"action"`
	OldRev                    string                   `json:"oldrev"`
	NewRev                    string                   `json:"newrev"`
	Ref                       string                   `json:"ref"`
	UserID                    int                      `json:"user_id"`
	UserName                  string                   `json:"user_name"`
	UserUsername              string                   `json:"user_username"`
	UserEmail                 string                   `json:"user_email"`
	UserAvatar                string                   `json:"user_avatar"`
	ProjectID                 int                      `json:"project_id"`
	Message                   string                   `json:"message"`
	Timestamp                 string                   `json:"timestamp"`
	CheckoutSHA               string                   `json:"checkout_sha"`
}

// GitLabV2ProjectReference represents a project reference in merge request
type GitLabV2ProjectReference struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	WebURL            string `json:"web_url"`
	AvatarURL         string `json:"avatar_url"`
	GitSSHURL         string `json:"git_ssh_url"`
	GitHTTPURL        string `json:"git_http_url"`
	Namespace         string `json:"namespace"`
	VisibilityLevel   int    `json:"visibility_level"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
}

// GitLabV2Changes represents changes in a merge request
type GitLabV2Changes struct {
	ReviewerIds GitLabV2ReviewerChanges `json:"reviewer_ids"`
}

// GitLabV2ReviewerChanges represents reviewer changes
type GitLabV2ReviewerChanges struct {
	Previous []int `json:"previous"`
	Current  []int `json:"current"`
}

// GitLabV2MergeRequest represents a GitLab merge request
type GitLabV2MergeRequest struct {
	ID                        int                    `json:"id"`
	IID                       int                    `json:"iid"`
	ProjectID                 int                    `json:"project_id"`
	Title                     string                 `json:"title"`
	Description               string                 `json:"description"`
	State                     string                 `json:"state"`
	CreatedAt                 string                 `json:"created_at"`
	UpdatedAt                 string                 `json:"updated_at"`
	TargetBranch              string                 `json:"target_branch"`
	SourceBranch              string                 `json:"source_branch"`
	AuthorID                  int                    `json:"author_id"`
	AssigneeID                int                    `json:"assignee_id"`
	SourceProjectID           int                    `json:"source_project_id"`
	TargetProjectID           int                    `json:"target_project_id"`
	Labels                    []string               `json:"labels"`
	WorkInProgress            bool                   `json:"work_in_progress"`
	MilestoneID               int                    `json:"milestone_id"`
	MergeWhenPipelineSucceeds bool                   `json:"merge_when_pipeline_succeeds"`
	MergeStatus               string                 `json:"merge_status"`
	MergeCommitSHA            string                 `json:"merge_commit_sha"`
	Squash                    bool                   `json:"squash"`
	WebURL                    string                 `json:"web_url"`
	TimeStats                 map[string]interface{} `json:"time_stats"`
	Assignee                  GitLabV2User           `json:"assignee"`
	Author                    GitLabV2User           `json:"author"`
}

// GitLabV2ReviewerChangeInfo represents reviewer change information
type GitLabV2ReviewerChangeInfo struct {
	Previous []int `json:"previous"`
	Current  []int `json:"current"`
}

// GitLabV2BotUserInfo represents bot user information from GitLab API
type GitLabV2BotUserInfo struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
	Email     string `json:"email"`
}

// GitLabV2HTTPClient wraps HTTP client for GitLab API operations
type GitLabV2HTTPClient struct {
	baseURL     string
	accessToken string
	client      *http.Client
}

// GitLabV2Discussion represents a GitLab discussion thread
type GitLabV2Discussion struct {
	ID    string         `json:"id"`
	Notes []GitLabV2Note `json:"notes"`
}

// GitLabV2Note represents a single note in GitLab
type GitLabV2Note struct {
	ID           int                   `json:"id"`
	Type         string                `json:"type"`
	Body         string                `json:"body"`
	Author       GitLabV2User          `json:"author"`
	CreatedAt    string                `json:"created_at"`
	UpdatedAt    string                `json:"updated_at"`
	System       bool                  `json:"system"`
	NoteableID   int                   `json:"noteable_id"`
	NoteableType string                `json:"noteable_type"`
	Position     *GitLabV2NotePosition `json:"position"`
	Resolvable   bool                  `json:"resolvable"`
	Resolved     bool                  `json:"resolved"`
	ResolvedBy   GitLabV2User          `json:"resolved_by"`
}

// GitLabV2NotePosition represents the position of a code comment
type GitLabV2NotePosition struct {
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	OldPath      string `json:"old_path"`
	NewPath      string `json:"new_path"`
	PositionType string `json:"position_type"`
	OldLine      *int   `json:"old_line"`
	NewLine      *int   `json:"new_line"`
	LineRange    *struct {
		Start struct {
			LineCode string `json:"line_code"`
			Type     string `json:"type"`
			OldLine  *int   `json:"old_line"`
			NewLine  *int   `json:"new_line"`
		} `json:"start"`
		End struct {
			LineCode string `json:"line_code"`
			Type     string `json:"type"`
			OldLine  *int   `json:"old_line"`
			NewLine  *int   `json:"new_line"`
		} `json:"end"`
	} `json:"line_range"`
}

// GitLabV2TimelineItem represents an item in the chronological timeline
type GitLabV2TimelineItem struct {
	Kind      string          `json:"kind"` // "commit" or "comment"
	CreatedAt time.Time       `json:"created_at"`
	Commit    *GitLabV2Commit `json:"commit,omitempty"`
	Comment   *GitLabV2Note   `json:"comment,omitempty"`
	NoteID    string          `json:"note_id,omitempty"`
}

// GitLabV2CommentContext represents context around a comment
type GitLabV2CommentContext struct {
	BeforeCommits  []string `json:"before_commits"`
	BeforeComments []string `json:"before_comments"`
	AfterCommits   []string `json:"after_commits"`
	AfterComments  []string `json:"after_comments"`
}

// GitLabV2Provider implements WebhookProviderV2 interface for GitLab.
type GitLabV2Provider struct {
	db     *sql.DB
	output GitLabOutputClient
}

// NewGitLabV2Provider creates a new GitLab V2 provider.
func NewGitLabV2Provider(db *sql.DB, output GitLabOutputClient) *GitLabV2Provider {
	if db == nil {
		panic("gitlab provider requires database handle")
	}
	if output == nil {
		panic("gitlab provider requires output client")
	}
	return &GitLabV2Provider{db: db, output: output}
}

// ProviderName returns the provider name
func (p *GitLabV2Provider) ProviderName() string {
	return "gitlab"
}

// CanHandleWebhook checks if this provider can handle the webhook
func (p *GitLabV2Provider) CanHandleWebhook(headers map[string]string, body []byte) bool {
	// Check for GitLab-specific headers
	if _, exists := headers["X-Gitlab-Event"]; exists {
		return true
	}
	if _, exists := headers["X-Gitlab-Token"]; exists {
		return true
	}

	// Check for GitLab-specific content in body
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err == nil {
		if objectKind, exists := payload["object_kind"]; exists {
			switch objectKind {
			case "merge_request", "note", "push", "tag_push", "issue", "wiki_page":
				return true
			}
		}
	}

	return false
}

// ConvertCommentEvent converts GitLab comment webhook to unified format
func (p *GitLabV2Provider) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	eventType := canonicalGitLabEventType(headers["X-Gitlab-Event"])

	var payload GitLabV2NoteWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		if capture.Enabled() {
			recordGitLabWebhook(eventType, headers, body, nil, err)
		}
		return nil, fmt.Errorf("failed to parse GitLab note webhook: %w", err)
	}

	if eventType == "unknown" && payload.ObjectKind != "" {
		eventType = canonicalGitLabEventType(payload.ObjectKind)
	}

	// Convert to unified format
	unifiedEvent := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "gitlab",
		Timestamp: payload.ObjectAttributes.CreatedAt,
		Comment: &UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", payload.ObjectAttributes.ID),
			Body:      payload.ObjectAttributes.Note,
			CreatedAt: payload.ObjectAttributes.CreatedAt,
			UpdatedAt: payload.ObjectAttributes.UpdatedAt,
			WebURL:    payload.ObjectAttributes.URL,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.User.ID),
				Username:  payload.User.Username,
				Name:      payload.User.Name,
				WebURL:    payload.User.WebURL,
				AvatarURL: payload.User.AvatarURL,
			},
		},
		MergeRequest: &UnifiedMergeRequestV2{
			ID:           fmt.Sprintf("%d", payload.MergeRequest.ID),
			Number:       payload.MergeRequest.IID,
			Title:        payload.MergeRequest.Title,
			Description:  payload.MergeRequest.Description,
			State:        payload.MergeRequest.State,
			CreatedAt:    payload.MergeRequest.CreatedAt,
			UpdatedAt:    payload.MergeRequest.UpdatedAt,
			WebURL:       payload.MergeRequest.WebURL,
			TargetBranch: payload.MergeRequest.TargetBranch,
			SourceBranch: payload.MergeRequest.SourceBranch,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.MergeRequest.Author.ID),
				Username:  payload.MergeRequest.Author.Username,
				Name:      payload.MergeRequest.Author.Name,
				WebURL:    payload.MergeRequest.Author.WebURL,
				AvatarURL: payload.MergeRequest.Author.AvatarURL,
			},
		},
		Repository: UnifiedRepositoryV2{
			ID:       fmt.Sprintf("%d", payload.Project.ID),
			Name:     payload.Project.Name,
			FullName: payload.Project.PathWithNamespace,
			WebURL:   payload.Project.WebURL,
			Owner: UnifiedUserV2{
				Username: strings.Split(payload.Project.PathWithNamespace, "/")[0],
			},
		},
		Actor: UnifiedUserV2{
			ID:        fmt.Sprintf("%d", payload.User.ID),
			Username:  payload.User.Username,
			Name:      payload.User.Name,
			WebURL:    payload.User.WebURL,
			AvatarURL: payload.User.AvatarURL,
		},
	}

	// Add position information if this is a code comment
	if payload.ObjectAttributes.Position != nil {
		unifiedEvent.Comment.Position = &UnifiedPositionV2{
			FilePath: payload.ObjectAttributes.Position.NewPath,
		}

		if payload.ObjectAttributes.Position.NewLine != nil {
			unifiedEvent.Comment.Position.LineNumber = *payload.ObjectAttributes.Position.NewLine
		}
	} // Add discussion/thread information
	if payload.ObjectAttributes.DiscussionID != "" {
		unifiedEvent.Comment.DiscussionID = &payload.ObjectAttributes.DiscussionID
	}

	if capture.Enabled() {
		recordGitLabWebhook(eventType, headers, body, unifiedEvent, nil)
	}

	return unifiedEvent, nil
}

// ConvertReviewerEvent converts GitLab reviewer assignment webhook to unified format
func (p *GitLabV2Provider) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	eventType := canonicalGitLabEventType(headers["X-Gitlab-Event"])
	var payload GitLabV2WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		if capture.Enabled() {
			recordGitLabWebhook(eventType, headers, body, nil, err)
		}
		return nil, fmt.Errorf("failed to parse GitLab webhook: %w", err)
	}

	// Check if this is a reviewer assignment event
	if payload.ObjectKind != "merge_request" {
		return nil, fmt.Errorf("not a merge request event")
	}

	if eventType == "unknown" && payload.ObjectKind != "" {
		eventType = canonicalGitLabEventType(payload.ObjectKind)
	}

	// Convert to unified format
	unifiedEvent := &UnifiedWebhookEventV2{
		EventType: "reviewer_assigned",
		Provider:  "gitlab",
		Timestamp: payload.ObjectAttributes.UpdatedAt,
		MergeRequest: &UnifiedMergeRequestV2{
			ID:           fmt.Sprintf("%d", payload.ObjectAttributes.ID),
			Number:       payload.ObjectAttributes.IID,
			Title:        payload.ObjectAttributes.Title,
			Description:  payload.ObjectAttributes.Description,
			State:        payload.ObjectAttributes.State,
			CreatedAt:    payload.ObjectAttributes.CreatedAt,
			UpdatedAt:    payload.ObjectAttributes.UpdatedAt,
			WebURL:       payload.ObjectAttributes.URL,
			TargetBranch: payload.ObjectAttributes.TargetBranch,
			SourceBranch: payload.ObjectAttributes.SourceBranch,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", payload.User.ID),
				Username:  payload.User.Username,
				Name:      payload.User.Name,
				WebURL:    payload.User.WebURL,
				AvatarURL: payload.User.AvatarURL,
			},
		},
		Repository: UnifiedRepositoryV2{
			ID:       fmt.Sprintf("%d", payload.Project.ID),
			Name:     payload.Project.Name,
			FullName: payload.Project.PathWithNamespace,
			WebURL:   payload.Project.WebURL,
			Owner: UnifiedUserV2{
				Username: strings.Split(payload.Project.PathWithNamespace, "/")[0],
			},
		},
		Actor: UnifiedUserV2{
			ID:        fmt.Sprintf("%d", payload.User.ID),
			Username:  payload.User.Username,
			Name:      payload.User.Name,
			WebURL:    payload.User.WebURL,
			AvatarURL: payload.User.AvatarURL,
		},
	}

	// Add reviewer change information if available
	if len(payload.Changes.ReviewerIds.Current) > 0 {
		unifiedEvent.ReviewerChange = &UnifiedReviewerChangeV2{
			Action:            "added",
			PreviousReviewers: convertGitLabUsersToUnifiedV2(payload.Changes.ReviewerIds.Previous, payload),
			CurrentReviewers:  convertGitLabUsersToUnifiedV2(payload.Changes.ReviewerIds.Current, payload),
		}
	}

	if capture.Enabled() {
		recordGitLabWebhook(eventType, headers, body, unifiedEvent, nil)
	}

	return unifiedEvent, nil
}

// Helper function to convert GitLab user IDs to unified users
func convertGitLabUsersToUnifiedV2(userIDs []int, payload GitLabV2WebhookPayload) []UnifiedUserV2 {
	var users []UnifiedUserV2
	for _, userID := range userIDs {
		// For now, we only have basic user ID information
		// In a full implementation, we might need to fetch user details from GitLab API
		users = append(users, UnifiedUserV2{
			ID: fmt.Sprintf("%d", userID),
		})
	}
	return users
}

func canonicalGitLabEventType(eventType string) string {
	if eventType == "" {
		return "unknown"
	}
	canonical := strings.ToLower(eventType)
	canonical = strings.ReplaceAll(canonical, " ", "_")
	canonical = strings.ReplaceAll(canonical, "-", "_")
	if strings.HasSuffix(canonical, "_hook") {
		canonical = strings.TrimSuffix(canonical, "_hook")
	}
	return canonical
}

func sanitizeGitLabHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	sanitized := make(map[string]string, len(headers))
	for k, v := range headers {
		lower := strings.ToLower(k)
		switch lower {
		case "authorization", "x-gitlab-token":
			continue
		}
		sanitized[k] = v
	}
	return sanitized
}

func recordGitLabWebhook(eventType string, headers map[string]string, body []byte, unified *UnifiedWebhookEventV2, err error) {
	if eventType == "" {
		eventType = "unknown"
	}
	if len(body) > 0 {
		capture.WriteBlobForNamespace("gitlab", fmt.Sprintf("gitlab-webhook-%s-body", eventType), "json", body)
	}
	meta := map[string]interface{}{
		"event_type": eventType,
		"headers":    sanitizeGitLabHeaders(headers),
	}
	if err != nil {
		meta["error"] = err.Error()
	}
	capture.WriteJSONForNamespace("gitlab", fmt.Sprintf("gitlab-webhook-%s-meta", eventType), meta)
	if unified != nil && err == nil {
		capture.WriteJSONForNamespace("gitlab", fmt.Sprintf("gitlab-webhook-%s-unified", eventType), unified)
	}
}

// FetchMergeRequestData fetches additional MR data from GitLab API - simplified version
func (p *GitLabV2Provider) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event")
	}

	// Extract GitLab instance URL from project web URL
	gitlabInstanceURL := extractGitLabInstanceURLV2(event.Repository.WebURL)

	// Get access token
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Create HTTP client
	httpClient := &GitLabV2HTTPClient{
		baseURL:     gitlabInstanceURL,
		accessToken: accessToken,
		client:      &http.Client{Timeout: 60 * time.Second},
	}

	projectID, _ := strconv.Atoi(event.Repository.ID)
	mrIID := event.MergeRequest.Number

	// Fetch commits, discussions, and notes for future use
	_, err = httpClient.GetMergeRequestCommitsV2(projectID, mrIID)
	if err != nil {
		return fmt.Errorf("failed to get commits: %w", err)
	}

	_, err = httpClient.GetMergeRequestDiscussionsV2(projectID, mrIID)
	if err != nil {
		return fmt.Errorf("failed to get discussions: %w", err)
	}

	_, err = httpClient.GetMergeRequestNotesV2(projectID, mrIID)
	if err != nil {
		return fmt.Errorf("failed to get standalone notes: %w", err)
	}

	log.Printf("[INFO] Successfully fetched MR data for GitLab MR %d", mrIID)
	return nil
}

// PostCommentReply posts a reply to a GitLab comment
func (p *GitLabV2Provider) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	if event.Comment == nil || event.MergeRequest == nil {
		return fmt.Errorf("invalid event for comment reply")
	}

	gitlabInstanceURL := extractGitLabInstanceURLV2(event.Repository.WebURL)
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	return p.output.PostCommentReply(event, accessToken, gitlabInstanceURL, content)
}

// PostEmojiReaction posts an emoji reaction to a GitLab comment
func (p *GitLabV2Provider) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	if event.Comment == nil {
		return fmt.Errorf("no comment in event for emoji reaction")
	}

	gitlabInstanceURL := extractGitLabInstanceURLV2(event.Repository.WebURL)
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	return p.output.PostEmojiReaction(event, accessToken, gitlabInstanceURL, emoji)
}

// PostFullReview posts a comprehensive review to a GitLab MR - simplified version
func (p *GitLabV2Provider) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	if event.MergeRequest == nil {
		return fmt.Errorf("no merge request in event for full review")
	}

	gitlabInstanceURL := extractGitLabInstanceURLV2(event.Repository.WebURL)
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	return p.output.PostFullReview(event, accessToken, gitlabInstanceURL, overallComment)
}

// GitLab V2 API Methods - Updated versions of existing GitLab API methods

// getGitLabAccessTokenV2 gets access token for GitLab instance
func (p *GitLabV2Provider) getGitLabAccessTokenV2(gitlabInstanceURL string) (string, error) {
	// Use SQL to normalize URLs by trimming trailing slashes for flexible matching
	query := `
		SELECT pat_token FROM integration_tokens 
		WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') 
		AND RTRIM(provider_url, '/') = RTRIM($1, '/')
		LIMIT 1
	`

	var token string
	err := p.db.QueryRow(query, gitlabInstanceURL).Scan(&token)
	if err != nil {
		// If the SQL approach fails, try the manual approach as fallback
		normalizedURL := normalizeGitLabURLV2(gitlabInstanceURL)
		fallbackQuery := `
			SELECT pat_token FROM integration_tokens 
			WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') 
			AND (RTRIM(provider_url, '/') = $1 OR provider_url = $2 OR provider_url = $3)
			LIMIT 1
		`

		err = p.db.QueryRow(fallbackQuery, normalizedURL, normalizedURL+"/", gitlabInstanceURL).Scan(&token)
		if err != nil {
			return "", fmt.Errorf("no access token found for GitLab instance %s (tried flexible URL matching): %w", gitlabInstanceURL, err)
		}
	}

	return token, nil
}

// extractGitLabInstanceURLV2 extracts GitLab instance URL from project web URL
func extractGitLabInstanceURLV2(projectWebURL string) string {
	if projectWebURL == "" {
		return ""
	}

	// Parse the URL to extract scheme and host
	if strings.HasPrefix(projectWebURL, "http://") || strings.HasPrefix(projectWebURL, "https://") {
		parts := strings.Split(projectWebURL, "/")
		if len(parts) >= 3 {
			return parts[0] + "//" + parts[2] // scheme + "//" + host
		}
	}

	return projectWebURL
}

// normalizeGitLabURLV2 normalizes GitLab URLs for consistent comparison
func normalizeGitLabURLV2(url string) string {
	return strings.TrimSuffix(strings.TrimSpace(url), "/")
}

// matchesGitLabURLV2 checks if two GitLab URLs match, handling trailing slash variations
func matchesGitLabURLV2(url1, url2 string) bool {
	return normalizeGitLabURLV2(url1) == normalizeGitLabURLV2(url2)
}

// getFreshBotUserInfoV2 gets fresh bot user information via GitLab API
func (p *GitLabV2Provider) getFreshBotUserInfoV2(gitlabInstanceURL string) (*GitLabV2BotUserInfo, error) {
	// Get access token for this GitLab instance
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Call GitLab API to get current user info (the bot user)
	apiURL := fmt.Sprintf("%s/api/v4/user", gitlabInstanceURL)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var botUser GitLabV2BotUserInfo
	err = json.NewDecoder(resp.Body).Decode(&botUser)
	if err != nil {
		return nil, fmt.Errorf("failed to decode GitLab user response: %w", err)
	}

	return &botUser, nil
}

// GetFreshBotUserInfo fetches the bot account metadata for the specified GitLab instance.
func (p *GitLabV2Provider) GetFreshBotUserInfo(gitlabInstanceURL string) (*GitLabV2BotUserInfo, error) {
	return p.getFreshBotUserInfoV2(gitlabInstanceURL)
}

// GitLabHTTPClient V2 Methods

// GetMergeRequestCommitsV2 fetches commits for a merge request
func (c *GitLabV2HTTPClient) GetMergeRequestCommitsV2(projectID, mrIID int) ([]GitLabV2Commit, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/commits", c.baseURL, projectID, mrIID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var commits []GitLabV2Commit
	err = json.NewDecoder(resp.Body).Decode(&commits)
	if err != nil {
		return nil, err
	}
	if capture.Enabled() {
		category := fmt.Sprintf("gitlab-mr-%d-%d-commits", projectID, mrIID)
		capture.WriteJSONForNamespace("gitlab", category, commits)
	}
	return commits, nil
}

// GetMergeRequestDiscussionsV2 fetches discussions for a merge request
func (c *GitLabV2HTTPClient) GetMergeRequestDiscussionsV2(projectID, mrIID int) ([]GitLabV2Discussion, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions", c.baseURL, projectID, mrIID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var discussions []GitLabV2Discussion
	err = json.NewDecoder(resp.Body).Decode(&discussions)
	if err != nil {
		return nil, err
	}
	if capture.Enabled() {
		category := fmt.Sprintf("gitlab-mr-%d-%d-discussions", projectID, mrIID)
		capture.WriteJSONForNamespace("gitlab", category, discussions)
	}
	return discussions, nil
}

// GetMergeRequestNotesV2 fetches standalone notes for a merge request
func (c *GitLabV2HTTPClient) GetMergeRequestNotesV2(projectID, mrIID int) ([]GitLabV2Note, error) {
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/notes", c.baseURL, projectID, mrIID)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var notes []GitLabV2Note
	err = json.NewDecoder(resp.Body).Decode(&notes)
	if err != nil {
		return nil, err
	}
	if capture.Enabled() {
		category := fmt.Sprintf("gitlab-mr-%d-%d-notes", projectID, mrIID)
		capture.WriteJSONForNamespace("gitlab", category, notes)
	}
	return notes, nil
}

// GitLab V2 Helper Methods

// buildTimelineV2 creates a chronological timeline of commits and comments
func (p *GitLabV2Provider) buildTimelineV2(commits []GitLabV2Commit, discussions []GitLabV2Discussion, standaloneNotes []GitLabV2Note) []GitLabV2TimelineItem {
	var timeline []GitLabV2TimelineItem

	// Add commits to timeline
	for _, commit := range commits {
		createdAt := parseTimeBestEffortV2(commit.Timestamp)
		timeline = append(timeline, GitLabV2TimelineItem{
			Kind:      "commit",
			CreatedAt: createdAt,
			Commit:    &commit,
		})
	}

	// Add discussion notes to timeline
	for _, d := range discussions {
		for _, note := range d.Notes {
			if note.System {
				continue // Skip system notes
			}
			createdAt := parseTimeBestEffortV2(note.CreatedAt)
			timeline = append(timeline, GitLabV2TimelineItem{
				Kind:      "comment",
				CreatedAt: createdAt,
				Comment:   &note,
				NoteID:    fmt.Sprintf("%d", note.ID),
			})
		}
	}

	// Add standalone notes to timeline
	for _, note := range standaloneNotes {
		if note.System {
			continue // Skip system notes
		}
		createdAt := parseTimeBestEffortV2(note.CreatedAt)
		timeline = append(timeline, GitLabV2TimelineItem{
			Kind:      "comment",
			CreatedAt: createdAt,
			Comment:   &note,
			NoteID:    fmt.Sprintf("%d", note.ID),
		})
	}

	// Sort timeline by creation time
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].CreatedAt.Before(timeline[j].CreatedAt)
	})

	return timeline
}

// Conversion helper functions - simplified versions

// convertGitLabCommitsToUnifiedV2 converts GitLab commits to unified format - placeholder
func convertGitLabCommitsToUnifiedV2(commits []GitLabV2Commit) []UnifiedCommitV2 {
	var unifiedCommits []UnifiedCommitV2
	for _, commit := range commits {
		unifiedCommits = append(unifiedCommits, UnifiedCommitV2{
			SHA:       commit.ID,
			Message:   commit.Message,
			Timestamp: commit.Timestamp,
			WebURL:    commit.URL,
			Author: UnifiedCommitAuthorV2{
				Name:  commit.Author.Name,
				Email: commit.Author.Email,
			},
		})
	}
	return unifiedCommits
}

// convertGitLabCommentsToUnifiedV2 converts GitLab comments to unified format - placeholder
func convertGitLabCommentsToUnifiedV2(discussions []GitLabV2Discussion, standaloneNotes []GitLabV2Note) []UnifiedCommentV2 {
	var unifiedComments []UnifiedCommentV2

	// Convert discussion notes
	for _, discussion := range discussions {
		for _, note := range discussion.Notes {
			if note.System {
				continue
			}

			unifiedComment := UnifiedCommentV2{
				ID:           fmt.Sprintf("%d", note.ID),
				Body:         note.Body,
				CreatedAt:    note.CreatedAt,
				UpdatedAt:    note.UpdatedAt,
				DiscussionID: &discussion.ID,
				Author: UnifiedUserV2{
					ID:        fmt.Sprintf("%d", note.Author.ID),
					Username:  note.Author.Username,
					Name:      note.Author.Name,
					WebURL:    note.Author.WebURL,
					AvatarURL: note.Author.AvatarURL,
				},
			}

			if note.Position != nil {
				unifiedComment.Position = &UnifiedPositionV2{
					FilePath: note.Position.NewPath,
				}
				if note.Position.NewLine != nil {
					unifiedComment.Position.LineNumber = *note.Position.NewLine
				}
			}

			unifiedComments = append(unifiedComments, unifiedComment)
		}
	}

	// Convert standalone notes
	for _, note := range standaloneNotes {
		if note.System {
			continue
		}

		unifiedComment := UnifiedCommentV2{
			ID:        fmt.Sprintf("%d", note.ID),
			Body:      note.Body,
			CreatedAt: note.CreatedAt,
			UpdatedAt: note.UpdatedAt,
			Author: UnifiedUserV2{
				ID:        fmt.Sprintf("%d", note.Author.ID),
				Username:  note.Author.Username,
				Name:      note.Author.Name,
				WebURL:    note.Author.WebURL,
				AvatarURL: note.Author.AvatarURL,
			},
		}

		if note.Position != nil {
			unifiedComment.Position = &UnifiedPositionV2{
				FilePath: note.Position.NewPath,
			}
			if note.Position.NewLine != nil {
				unifiedComment.Position.LineNumber = *note.Position.NewLine
			}
		}

		unifiedComments = append(unifiedComments, unifiedComment)
	}

	return unifiedComments
}

// convertGitLabTimelineToUnifiedV2 converts GitLab timeline to unified format - placeholder
func convertGitLabTimelineToUnifiedV2(timeline []GitLabV2TimelineItem) []UnifiedTimelineItemV2 {
	var unifiedTimeline []UnifiedTimelineItemV2
	for _, item := range timeline {
		unifiedItem := UnifiedTimelineItemV2{
			Type:      item.Kind,
			Timestamp: item.CreatedAt.Format(time.RFC3339),
		}

		if item.Commit != nil {
			unifiedItem.Commit = &UnifiedCommitV2{
				SHA:       item.Commit.ID,
				Message:   item.Commit.Message,
				Timestamp: item.Commit.Timestamp,
				WebURL:    item.Commit.URL,
				Author: UnifiedCommitAuthorV2{
					Name:  item.Commit.Author.Name,
					Email: item.Commit.Author.Email,
				},
			}
		}

		if item.Comment != nil {
			unifiedItem.Comment = &UnifiedCommentV2{
				ID:        fmt.Sprintf("%d", item.Comment.ID),
				Body:      item.Comment.Body,
				CreatedAt: item.Comment.CreatedAt,
				UpdatedAt: item.Comment.UpdatedAt,
				Author: UnifiedUserV2{
					ID:        fmt.Sprintf("%d", item.Comment.Author.ID),
					Username:  item.Comment.Author.Username,
					Name:      item.Comment.Author.Name,
					WebURL:    item.Comment.Author.WebURL,
					AvatarURL: item.Comment.Author.AvatarURL,
				},
			}
		}

		unifiedTimeline = append(unifiedTimeline, unifiedItem)
	}
	return unifiedTimeline
} // Utility functions

// parseTimeBestEffortV2 parses timestamp with best effort
func parseTimeBestEffortV2(timestamp string) time.Time {
	if timestamp == "" {
		return time.Time{}
	}

	// Try different time formats
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02 15:04:05 UTC",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timestamp); err == nil {
			return t
		}
	}

	// If all parsing fails, return zero time
	log.Printf("[WARN] Failed to parse timestamp: %s", timestamp)
	return time.Time{}
}

// parseIntFromString safely parses integer from string
func parseIntFromString(s string) int {
	if s == "" {
		return 0
	}

	// Simple string to int conversion for IIDs
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

// shortSHAV2 returns a short version of a commit SHA
func shortSHAV2(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

// Advanced GitLab V2 Operations - Context Building and Analysis

// buildContextualAIResponseV2 creates a rich, contextual response using MR analysis
func (p *GitLabV2Provider) buildContextualAIResponseV2(ctx context.Context, event *UnifiedWebhookEventV2, scenario ResponseScenarioV2) (string, error) {
	if event.Comment == nil || event.MergeRequest == nil {
		return "", fmt.Errorf("invalid event for contextual response")
	}

	// Get GitLab access token
	gitlabInstanceURL := extractGitLabInstanceURLV2(event.Repository.WebURL)
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return "", fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Create GitLab HTTP client wrapper
	httpClient := &GitLabV2HTTPClient{
		baseURL:     gitlabInstanceURL,
		accessToken: accessToken,
		client:      &http.Client{Timeout: 60 * time.Second},
	}

	projectID, _ := strconv.Atoi(event.Repository.ID)
	mrIID := event.MergeRequest.Number
	targetNoteID, _ := strconv.Atoi(event.Comment.ID)
	discussionID := ""
	if event.Comment.DiscussionID != nil {
		discussionID = *event.Comment.DiscussionID
	}

	// Fetch MR details, commits, and discussions
	log.Printf("[DEBUG] Building contextual response for note %d in MR %d", targetNoteID, mrIID)

	// Get commits, discussions, and standalone notes
	commits, err := httpClient.GetMergeRequestCommitsV2(projectID, mrIID)
	if err != nil {
		return "", fmt.Errorf("failed to get commits: %w", err)
	}

	discussions, err := httpClient.GetMergeRequestDiscussionsV2(projectID, mrIID)
	if err != nil {
		return "", fmt.Errorf("failed to get discussions: %w", err)
	}

	standaloneNotes, err := httpClient.GetMergeRequestNotesV2(projectID, mrIID)
	if err != nil {
		return "", fmt.Errorf("failed to get standalone notes: %w", err)
	}

	// Find the target comment and its context
	targetComment, _, err := p.findTargetCommentV2(targetNoteID, discussionID, discussions, standaloneNotes)
	if err != nil {
		return "", fmt.Errorf("failed to find target comment: %w", err)
	}

	// Build timeline and extract context around the target comment
	timeline := p.buildTimelineV2(commits, discussions, standaloneNotes)
	beforeContext, afterContext := p.extractCommentContextV2(timeline, targetNoteID, targetComment.CreatedAt)

	// Get code context if this is a code comment
	var codeExcerpt, focusedDiff string
	if targetComment.Position != nil {
		codeExcerpt, focusedDiff, err = p.getCodeContextV2(httpClient, projectID, targetComment.Position)
		if err != nil {
			log.Printf("[WARN] Failed to get code context: %v", err)
		}
	}

	// Build enhanced prompt using the system from main.go
	prompt := p.buildGeminiPromptEnhancedV2(
		event.Comment.Author.Username,
		event.Comment.Body,
		targetComment.Position,
		beforeContext,
		afterContext,
		codeExcerpt,
		focusedDiff,
	)

	// For now, use a sophisticated fallback response based on context
	// TODO: Integrate actual AI provider (Gemini) in Phase 5
	return p.synthesizeContextualResponseV2(prompt, event, targetComment, scenario), nil
}

// findTargetCommentV2 locates the target comment in discussions or standalone notes
func (p *GitLabV2Provider) findTargetCommentV2(targetNoteID int, discussionID string, discussions []GitLabV2Discussion, standaloneNotes []GitLabV2Note) (*GitLabV2Note, *GitLabV2Discussion, error) {
	// First search in discussions
	for _, d := range discussions {
		if discussionID != "" && d.ID != discussionID {
			continue
		}
		for _, n := range d.Notes {
			if n.ID == targetNoteID {
				return &n, &d, nil
			}
		}
	}
	// Then search in standalone notes
	for _, n := range standaloneNotes {
		if n.ID == targetNoteID {
			return &n, nil, nil
		}
	}
	return nil, nil, fmt.Errorf("target comment %d not found", targetNoteID)
}

// extractCommentContextV2 extracts before/after context around target comment
func (p *GitLabV2Provider) extractCommentContextV2(timeline []GitLabV2TimelineItem, targetNoteID int, targetCreatedAt string) (GitLabV2CommentContext, GitLabV2CommentContext) {
	targetTime := parseTimeBestEffortV2(targetCreatedAt)
	var beforeContext, afterContext GitLabV2CommentContext

	for _, item := range timeline {
		if item.Kind == "commit" && item.Commit != nil {
			commitLine := fmt.Sprintf("%s — %s", shortSHAV2(item.Commit.ID), item.Commit.Message)
			if targetTime.IsZero() || !item.CreatedAt.After(targetTime) {
				beforeContext.BeforeCommits = append(beforeContext.BeforeCommits, commitLine)
			} else {
				afterContext.AfterCommits = append(afterContext.AfterCommits, commitLine)
			}
		} else if item.Kind == "comment" && item.Comment != nil {
			commentLine := fmt.Sprintf("[%s] %s: %s",
				item.CreatedAt.Format(time.RFC3339),
				item.Comment.Author.Name,
				item.Comment.Body)

			if item.Comment.ID == targetNoteID {
				beforeContext.BeforeComments = append(beforeContext.BeforeComments, commentLine)
			} else if targetTime.IsZero() || !item.CreatedAt.After(targetTime) {
				beforeContext.BeforeComments = append(beforeContext.BeforeComments, commentLine)
			} else {
				afterContext.AfterComments = append(afterContext.AfterComments, commentLine)
			}
		}
	}

	return beforeContext, afterContext
}

// getCodeContextV2 retrieves code excerpts and diffs for a positioned comment
func (p *GitLabV2Provider) getCodeContextV2(httpClient *GitLabV2HTTPClient, projectID int, position *GitLabV2NotePosition) (string, string, error) {
	// Simplified version - in full implementation would fetch actual file content and diffs
	codeExcerpt := fmt.Sprintf("Code context for %s at line %d",
		firstNonEmptyV2(position.NewPath, position.OldPath),
		getLineNumberV2(position.NewLine, position.OldLine))

	focusedDiff := fmt.Sprintf("Diff context for %s (%s)",
		firstNonEmptyV2(position.NewPath, position.OldPath),
		shortSHAV2(position.HeadSHA))

	return codeExcerpt, focusedDiff, nil
}

// buildGeminiPromptEnhancedV2 creates a rich prompt for AI response generation
func (p *GitLabV2Provider) buildGeminiPromptEnhancedV2(author, message string, position *GitLabV2NotePosition, beforeContext, afterContext GitLabV2CommentContext, codeExcerpt, focusedDiff string) string {
	var b strings.Builder

	b.WriteString("ROLE: You are a senior/principal engineer doing a contextual MR review.\n\n")
	b.WriteString("GOAL: Provide a specific, correct, and helpful reply to the latest message in the thread, grounded in the actual code and diff.\n\n")

	// Comment needing response
	b.WriteString("=== Comment needing response ===\n")
	b.WriteString(fmt.Sprintf("Author: %s\n", author))
	b.WriteString("Message:\n\n")
	b.WriteString("> ")
	b.WriteString(message)
	b.WriteString("\n\n")

	// Add position info if available
	if position != nil {
		b.WriteString(fmt.Sprintf("Location: %s (line %d)\n\n",
			firstNonEmptyV2(position.NewPath, position.OldPath),
			getLineNumberV2(position.NewLine, position.OldLine)))
	}

	// Add code context
	if codeExcerpt != "" {
		b.WriteString("=== Code context ===\n")
		b.WriteString(codeExcerpt)
		b.WriteString("\n\n")
	}

	if focusedDiff != "" {
		b.WriteString("=== Focused diff ===\n")
		b.WriteString(focusedDiff)
		b.WriteString("\n\n")
	}

	// Add thread context
	if len(beforeContext.BeforeComments) > 0 {
		b.WriteString("=== Thread context (before) ===\n")
		for _, msg := range beforeContext.BeforeComments {
			b.WriteString(msg)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(afterContext.AfterComments) > 0 {
		b.WriteString("=== Thread context (after) ===\n")
		for _, msg := range afterContext.AfterComments {
			b.WriteString(msg)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Add commit context
	if len(beforeContext.BeforeCommits) > 0 {
		b.WriteString("=== Recent commits ===\n")
		for _, commit := range beforeContext.BeforeCommits {
			b.WriteString("- ")
			b.WriteString(commit)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// synthesizeContextualResponseV2 generates a contextual response based on the built prompt and collected context
func (p *GitLabV2Provider) synthesizeContextualResponseV2(prompt string, event *UnifiedWebhookEventV2, targetComment *GitLabV2Note, scenario ResponseScenarioV2) string {
	// Use the sophisticated analysis approach
	commentBody := event.Comment.Body
	author := event.Comment.Author.Username

	var response strings.Builder

	// Start with acknowledgment
	response.WriteString(fmt.Sprintf("Thanks for the comment, @%s! ", author))

	// Analyze the comment type and respond appropriately
	commentLower := strings.ToLower(commentBody)

	if strings.Contains(commentLower, "question") || strings.Contains(commentLower, "?") {
		response.WriteString("Looking into this question. ")
		if targetComment.Position != nil {
			response.WriteString(fmt.Sprintf("For the code at %s, ",
				firstNonEmptyV2(targetComment.Position.NewPath, targetComment.Position.OldPath)))
		}
		response.WriteString("I'll need to analyze the context and get back to you with a detailed explanation.")
	} else if strings.Contains(commentLower, "suggestion") || strings.Contains(commentLower, "recommend") {
		response.WriteString("Good suggestion! ")
		response.WriteString("I'll consider this improvement and implement it in the next revision.")
	} else if strings.Contains(commentLower, "issue") || strings.Contains(commentLower, "problem") || strings.Contains(commentLower, "bug") {
		response.WriteString("Thanks for catching this! ")
		response.WriteString("I'll investigate and fix this issue.")
	} else {
		response.WriteString("I appreciate your feedback. ")
		response.WriteString("I'll review this and make any necessary adjustments.")
	}

	return response.String()
}

// checkAIResponseWarrantV2 determines if a comment warrants an AI response and how to respond
func (p *GitLabV2Provider) checkAIResponseWarrantV2(event *UnifiedWebhookEventV2, gitlabInstanceURL string) (bool, ResponseScenarioV2) {
	if event.Comment == nil {
		return false, ResponseScenarioV2{}
	}

	log.Printf("[DEBUG] Checking AI response warrant for comment by %s", event.Comment.Author.Username)
	log.Printf("[DEBUG] Comment content: %s", event.Comment.Body)

	// Early anti-loop protection: Check for common bot usernames before API calls
	commonBotUsernames := []string{"livereviewbot", "LiveReviewBot", "ai-bot", "codebot", "reviewbot"}
	for _, botUsername := range commonBotUsernames {
		if strings.EqualFold(event.Comment.Author.Username, botUsername) {
			log.Printf("[DEBUG] Comment author %s appears to be a bot user (early detection), skipping (anti-loop protection)", event.Comment.Author.Username)
			return false, ResponseScenarioV2{}
		}
	}

	// Get fresh bot user information via GitLab API
	botUserInfo, err := p.getFreshBotUserInfoV2(gitlabInstanceURL)
	if err != nil {
		log.Printf("[ERROR] Failed to get fresh bot user info for GitLab instance %s: %v", gitlabInstanceURL, err)
		return false, ResponseScenarioV2{}
	}

	if botUserInfo == nil {
		log.Printf("[DEBUG] No bot user configured for GitLab instance %s", gitlabInstanceURL)
		return false, ResponseScenarioV2{}
	}

	log.Printf("[DEBUG] Fresh bot user info: username=%s, id=%d, name=%s",
		botUserInfo.Username, botUserInfo.ID, botUserInfo.Name)

	// Anti-loop protection: Never respond to bot accounts
	if event.Comment.Author.Username == botUserInfo.Username {
		log.Printf("[DEBUG] Comment author %s is the bot user, skipping (anti-loop protection)", event.Comment.Author.Username)
		return false, ResponseScenarioV2{}
	}

	// Check for direct mentions of the bot
	botMentions := []string{
		"@" + botUserInfo.Username,
		botUserInfo.Username,
	}

	for _, mention := range botMentions {
		if strings.Contains(strings.ToLower(event.Comment.Body), strings.ToLower(mention)) {
			log.Printf("[DEBUG] Found direct mention of bot user: %s", mention)
			return true, ResponseScenarioV2{
				Type:       "direct_mention",
				Reason:     "Bot mentioned directly in comment",
				Confidence: 0.9,
			}
		}
	}

	// Check for thread replies - if this is a reply to a bot comment
	if event.Comment.DiscussionID != nil && *event.Comment.DiscussionID != "" {
		log.Printf("[DEBUG] Comment is part of discussion: %s", *event.Comment.DiscussionID)

		// Check if the parent discussion has bot comments
		isReplyToBot, err := p.checkIfReplyingToBotCommentV2(event, botUserInfo, gitlabInstanceURL)
		if err != nil {
			log.Printf("[WARN] Failed to check if reply is to bot comment: %v", err)
		} else if isReplyToBot {
			log.Printf("[DEBUG] Comment is a reply to bot comment, triggering response")
			return true, ResponseScenarioV2{
				Type:       "thread_reply",
				Reason:     "Reply to bot comment in thread",
				Confidence: 0.8,
			}
		}
	}

	// Content analysis - detect questions and help requests
	contentLower := strings.ToLower(event.Comment.Body)
	questionIndicators := []string{
		"what", "how", "why", "when", "where", "which", "who",
		"?", "can you", "could you", "would you", "help",
		"explain", "clarify", "understand",
	}

	for _, indicator := range questionIndicators {
		if strings.Contains(contentLower, indicator) {
			log.Printf("[DEBUG] Found question indicator '%s' in comment", indicator)
			return true, ResponseScenarioV2{
				Type:       "content_analysis",
				Reason:     fmt.Sprintf("Question detected: %s", indicator),
				Confidence: 0.7,
			}
		}
	}

	// For now, only respond to direct mentions, thread replies, and questions
	log.Printf("[DEBUG] No trigger found (no mention, not a reply to bot, no question indicators)")
	return false, ResponseScenarioV2{}
}

// checkIfReplyingToBotCommentV2 checks if the current comment is replying to a bot's previous comment
func (p *GitLabV2Provider) checkIfReplyingToBotCommentV2(event *UnifiedWebhookEventV2, botUserInfo *GitLabV2BotUserInfo, gitlabInstanceURL string) (bool, error) {
	// If this comment is not part of a discussion/thread, it can't be a reply
	if event.Comment.DiscussionID == nil || *event.Comment.DiscussionID == "" {
		log.Printf("[DEBUG] Comment has no discussion_id, not a thread reply")
		return false, nil
	}

	log.Printf("[DEBUG] Checking if comment is reply to bot in discussion: %s", *event.Comment.DiscussionID)

	// Get access token for GitLab API calls
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return false, fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Get discussion details from GitLab API
	projectID, _ := strconv.Atoi(event.Repository.ID)
	mrIID := event.MergeRequest.Number
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions/%s",
		gitlabInstanceURL, projectID, mrIID, *event.Comment.DiscussionID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[ERROR] GitLab API request failed with status %d: %s", resp.StatusCode, string(body))
		return false, nil // Don't fail the entire process, just skip this check
	}

	// Parse the discussion response to find previous comments
	var discussion struct {
		Notes []struct {
			ID     int `json:"id"`
			Author struct {
				Username string `json:"username"`
				ID       int    `json:"id"`
			} `json:"author"`
			CreatedAt string `json:"created_at"`
		} `json:"notes"`
	}

	err = json.NewDecoder(resp.Body).Decode(&discussion)
	if err != nil {
		log.Printf("[ERROR] Failed to decode discussion response: %v", err)
		return false, nil
	}

	// Check if any previous note in this discussion was authored by the bot
	currentNoteID, _ := strconv.Atoi(event.Comment.ID)
	for _, note := range discussion.Notes {
		// Skip the current note and check only previous ones
		if note.ID >= currentNoteID {
			continue
		}

		if note.Author.Username == botUserInfo.Username {
			log.Printf("[DEBUG] Found previous bot comment in discussion by %s (note_id=%d)",
				note.Author.Username, note.ID)
			return true, nil
		}
	}

	log.Printf("[DEBUG] No previous bot comments found in discussion")
	return false, nil
}

// Helper functions for GitLab V2 operations

// firstNonEmptyV2 returns the first non-empty string
func firstNonEmptyV2(strings ...string) string {
	for _, s := range strings {
		if s != "" {
			return s
		}
	}
	return ""
}

// getLineNumberV2 gets the line number from position, preferring new line over old line
func getLineNumberV2(newLine, oldLine *int) int {
	if newLine != nil {
		return *newLine
	}
	if oldLine != nil {
		return *oldLine
	}
	return 0
}

// High-Level GitLab V2 Response Generation Functions

// GenerateAndPostResponseV2 generates an AI response and posts it to GitLab
func (p *GitLabV2Provider) GenerateAndPostResponseV2(ctx context.Context, event *UnifiedWebhookEventV2, scenario ResponseScenarioV2, gitlabInstanceURL string) error {
	log.Printf("[INFO] Generating AI response for scenario: %s -> %s", scenario.Type, scenario.Reason)

	// Handle different response types
	switch scenario.Type {
	case "emoji_only":
		return p.postGitLabEmojiReactionV2(ctx, event, scenario, gitlabInstanceURL)
	case "brief_acknowledgment", "detailed_response", "diplomatic_response":
		return p.postGitLabTextResponseV2(ctx, event, scenario, gitlabInstanceURL)
	default:
		log.Printf("[WARN] Unknown response type: %s, defaulting to detailed response", scenario.Type)
		return p.postGitLabTextResponseV2(ctx, event, scenario, gitlabInstanceURL)
	}
}

// postGitLabEmojiReactionV2 posts an emoji reaction to a GitLab comment
func (p *GitLabV2Provider) postGitLabEmojiReactionV2(ctx context.Context, event *UnifiedWebhookEventV2, scenario ResponseScenarioV2, gitlabInstanceURL string) error {
	log.Printf("[INFO] Posting emoji reaction for %s content", scenario.Type)

	// Get the access token for this GitLab instance
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Choose appropriate emoji based on content type
	emoji := "thumbsup" // Default emoji
	if strings.Contains(scenario.Reason, "appreciation") {
		emoji = "heart" // ❤️ for thanks/appreciation
	} else if strings.Contains(scenario.Reason, "question") {
		emoji = "point_up" // 👆 for questions
	}

	// Post emoji reaction using GitLab API
	return p.output.PostEmojiReaction(event, accessToken, gitlabInstanceURL, emoji)
}

// postGitLabTextResponseV2 generates and posts a text response to GitLab
func (p *GitLabV2Provider) postGitLabTextResponseV2(ctx context.Context, event *UnifiedWebhookEventV2, scenario ResponseScenarioV2, gitlabInstanceURL string) error {
	log.Printf("[INFO] Generating text response for %s content type", scenario.Type)

	// Get the access token for this GitLab instance
	accessToken, err := p.getGitLabAccessTokenV2(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Generate response content based on response type
	var responseContent string
	switch scenario.Type {
	case "brief_acknowledgment":
		responseContent = p.generateBriefAcknowledgmentV2(scenario.Reason)
	case "detailed_response":
		// For detailed responses, we build MR context and use AI
		contextualResponse, err := p.buildContextualAIResponseV2(ctx, event, scenario)
		if err != nil {
			log.Printf("[ERROR] Failed to build contextual response, using fallback: %v", err)
			// Fallback to simple response if context building fails
			responseContent = fmt.Sprintf("Thanks for mentioning me, @%s! I encountered an issue building full context, but I'm here to help with your question about '%s'.",
				event.Comment.Author.Username, event.MergeRequest.Title)
		} else {
			responseContent = contextualResponse
		}
	case "diplomatic_response":
		responseContent = p.generateDiplomaticResponseV2(scenario.Reason, event.Comment.Body)
	default:
		responseContent = p.generateBriefAcknowledgmentV2(scenario.Reason)
	}

	// LEARNING EXTRACTION: Augment response with learning metadata detection
	augmentedResponse, learningAck := p.augmentResponseWithLearningMetadataV2(ctx, responseContent, event)
	finalResponse := augmentedResponse
	if learningAck != "" {
		finalResponse = augmentedResponse + "\n\n" + learningAck
	}

	// Post the response to GitLab
	if event.Comment.DiscussionID != nil && *event.Comment.DiscussionID != "" {
		return p.output.PostCommentReply(event, accessToken, gitlabInstanceURL, finalResponse)
	}

	mentionedResponse := fmt.Sprintf("@%s %s", event.Comment.Author.Username, finalResponse)
	return p.output.PostCommentReply(event, accessToken, gitlabInstanceURL, mentionedResponse)
}

// Response generation helper methods

// generateBriefAcknowledgmentV2 generates brief acknowledgment responses
func (p *GitLabV2Provider) generateBriefAcknowledgmentV2(reason string) string {
	responses := map[string][]string{
		"appreciation": {
			"You're welcome! Happy to help.",
			"Glad I could assist! Let me know if you need anything else.",
			"No problem! Feel free to ask if you have more questions.",
		},
		"question": {
			"I'll look into that for you.",
			"Good question! Let me check that.",
			"Thanks for asking! I'll investigate.",
		},
		"general": {
			"Thanks for the feedback!",
			"I appreciate your input.",
			"Good point! I'll take that into consideration.",
		},
	}

	// Determine response category based on reason
	category := "general"
	if strings.Contains(strings.ToLower(reason), "appreciation") {
		category = "appreciation"
	} else if strings.Contains(strings.ToLower(reason), "question") {
		category = "question"
	}

	responseList, exists := responses[category]
	if !exists {
		responseList = responses["general"]
	}

	// Return a simple selection (in full implementation, could be random)
	return responseList[0]
}

// generateDiplomaticResponseV2 generates diplomatic responses for sensitive situations
func (p *GitLabV2Provider) generateDiplomaticResponseV2(reason, originalComment string) string {
	if strings.Contains(strings.ToLower(originalComment), "disagree") {
		return "I understand your perspective. Let's discuss this further to find the best approach."
	} else if strings.Contains(strings.ToLower(originalComment), "wrong") {
		return "Thank you for pointing that out. I'll review this carefully and make corrections if needed."
	} else if strings.Contains(strings.ToLower(originalComment), "issue") || strings.Contains(strings.ToLower(originalComment), "problem") {
		return "I appreciate you bringing this to my attention. I'll investigate and address any issues."
	}

	return "Thank you for your feedback. I value your input and will consider it carefully."
}

// augmentResponseWithLearningMetadataV2 augments response with learning metadata detection
func (p *GitLabV2Provider) augmentResponseWithLearningMetadataV2(ctx context.Context, responseContent string, event *UnifiedWebhookEventV2) (string, string) {
	// Simplified learning metadata detection
	// In full implementation, this would integrate with the learning system

	commentLower := strings.ToLower(event.Comment.Body)
	var learningAck string

	// Detect learning opportunities
	if strings.Contains(commentLower, "learn") || strings.Contains(commentLower, "documentation") {
		learningAck = "*I've noted this as a learning opportunity for future reference.*"
	} else if strings.Contains(commentLower, "best practice") || strings.Contains(commentLower, "pattern") {
		learningAck = "*This feedback has been recorded for pattern recognition.*"
	}

	return responseContent, learningAck
}
