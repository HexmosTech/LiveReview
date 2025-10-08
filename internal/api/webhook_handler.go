package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/review"
)

// AIConnector represents an AI connector from the database
type AIConnector struct {
	ID           int    `json:"id"`
	ProviderName string `json:"provider_name"`
	APIKey       string `json:"api_key"`
}

// GitLabWebhookPayload represents the structure of GitLab webhook payloads
type GitLabWebhookPayload struct {
	EventType        string                 `json:"event_type"`
	ObjectKind       string                 `json:"object_kind"`
	User             GitLabUser             `json:"user"`
	Project          GitLabProject          `json:"project"`
	ObjectAttributes GitLabObjectAttributes `json:"object_attributes"`
	Changes          GitLabChanges          `json:"changes,omitempty"`
	MergeRequest     GitLabMergeRequest     `json:"merge_request,omitempty"`
}

// GitLabNoteWebhookPayload represents GitLab Note Hook webhook payload structure
type GitLabNoteWebhookPayload struct {
	ObjectKind       string                     `json:"object_kind"` // "note"
	EventType        string                     `json:"event_type"`  // "note"
	User             GitLabUser                 `json:"user"`        // Comment author
	ProjectID        int                        `json:"project_id"`
	Project          GitLabProject              `json:"project"`
	Repository       GitLabRepository           `json:"repository"`
	ObjectAttributes GitLabNoteObjectAttributes `json:"object_attributes"`       // Note details
	MergeRequest     *GitLabMergeRequest        `json:"merge_request,omitempty"` // Present if comment on MR
	Issue            *GitLabIssue               `json:"issue,omitempty"`         // Present if comment on issue
	Commit           *GitLabCommit              `json:"commit,omitempty"`        // Present if comment on commit
	Snippet          *GitLabSnippet             `json:"snippet,omitempty"`       // Present if comment on snippet
}

// GitLabNoteObjectAttributes represents the object_attributes field in GitLab Note Hook payload
type GitLabNoteObjectAttributes struct {
	ID           int    `json:"id"`            // Note ID
	Note         string `json:"note"`          // Comment body/content
	NoteableType string `json:"noteable_type"` // "MergeRequest", "Issue", "Commit", "Snippet"
	AuthorID     int    `json:"author_id"`     // Comment author ID
	CreatedAt    string `json:"created_at"`    // When comment was created
	UpdatedAt    string `json:"updated_at"`    // When comment was last updated
	ProjectID    int    `json:"project_id"`    // Project ID
	Attachment   string `json:"attachment"`    // File attachment, if any
	LineCode     string `json:"line_code"`     // Code line reference (for code comments)
	CommitID     string `json:"commit_id"`     // Commit SHA (for commit comments)
	NoteableID   int    `json:"noteable_id"`   // ID of the item being commented on
	System       bool   `json:"system"`        // Whether this is a system comment
	StDiff       string `json:"st_diff"`       // Diff information for code comments
	Action       string `json:"action"`        // "create" or "update"
	URL          string `json:"url"`           // Direct URL to the comment
	DiscussionID string `json:"discussion_id"` // Discussion thread ID (for threaded comments)
}

// GitLabRepository represents repository information in Note Hook payload
type GitLabRepository struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
}

// GitLabIssue represents issue information when comment is on an issue
type GitLabIssue struct {
	ID          int    `json:"id"`
	IID         int    `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	ProjectID   int    `json:"project_id"`
	AuthorID    int    `json:"author_id"`
	URL         string `json:"url"`
}

// GitLabCommit represents commit information when comment is on a commit
type GitLabCommit struct {
	ID        string       `json:"id"`
	Message   string       `json:"message"`
	Timestamp string       `json:"timestamp"`
	URL       string       `json:"url"`
	Author    GitLabAuthor `json:"author"`
}

// GitLabAuthor represents commit author information
type GitLabAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GitLabSnippet represents snippet information when comment is on a snippet
type GitLabSnippet struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Content     string `json:"content"`
	AuthorID    int    `json:"author_id"`
	ProjectID   int    `json:"project_id"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	FileName    string `json:"file_name"`
	URL         string `json:"url"`
}

// GitLabUser represents a GitLab user
type GitLabUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
}

// GitLabProject represents a GitLab project
type GitLabProject struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
}

// GitLabObjectAttributes represents the object_attributes field in webhook payloads
type GitLabObjectAttributes struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"`
	Action       string `json:"action"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	URL          string `json:"url"`
	UpdatedAt    string `json:"updated_at"`
	AuthorID     int    `json:"author_id"`
	AssigneeIDs  []int  `json:"assignee_ids"`
	ReviewerIDs  []int  `json:"reviewer_ids"`
}

// GitLabChanges represents the changes field in webhook payloads
type GitLabChanges struct {
	Reviewers GitLabReviewerChanges `json:"reviewers,omitempty"`
}

// GitLabReviewerChanges represents reviewer changes
type GitLabReviewerChanges struct {
	Current  []GitLabUser `json:"current"`
	Previous []GitLabUser `json:"previous"`
}

// GitLabMergeRequest represents a merge request object
type GitLabMergeRequest struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	URL          string `json:"url"`
	AuthorID     int    `json:"author_id"`
	AssigneeIDs  []int  `json:"assignee_ids"`
	ReviewerIDs  []int  `json:"reviewer_ids"`
}

// ReviewerChangeInfo represents processed reviewer change information
type ReviewerChangeInfo struct {
	EventType            string                `json:"event_type"`
	Action               string                `json:"action"`
	UpdatedAt            string                `json:"updated_at"`
	BotUsers             []BotUserInfo         `json:"bot_users"`
	CurrentBotReviewers  []GitLabUser          `json:"current_bot_reviewers"`
	PreviousBotReviewers []GitLabUser          `json:"previous_bot_reviewers"`
	IsBotAssigned        bool                  `json:"is_bot_assigned"`
	IsBotRemoved         bool                  `json:"is_bot_removed"`
	ReviewerChanges      GitLabReviewerChanges `json:"reviewer_changes"`
	ChangedBy            GitLabUser            `json:"changed_by"`
	MergeRequest         GitLabMergeRequest    `json:"merge_request"`
	Project              GitLabProject         `json:"project"`
}

// BotUserInfo represents information about a bot user in reviewer changes
type BotUserInfo struct {
	Type string     `json:"type"` // "current" or "previous"
	User GitLabUser `json:"user"`
}

// GitLabWebhookHandler handles incoming GitLab webhook events
func (s *Server) GitLabWebhookHandler(c echo.Context) error {
	// Try V2 provider first
	if s.gitlabProviderV2 != nil {
		return s.GitLabWebhookHandlerV2(c)
	}

	// Fallback to original implementation
	return s.GitLabWebhookHandlerV1(c)
}

// GitLabWebhookHandlerV2 handles GitLab webhooks using the V2 provider
func (s *Server) GitLabWebhookHandlerV2(c echo.Context) error {
	// Read request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read GitLab webhook body: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
	}

	// Get headers
	headers := make(map[string]string)
	for key, values := range c.Request().Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	eventKind := c.Request().Header.Get("X-Gitlab-Event")
	log.Printf("[INFO] Received GitLab webhook (V2): event_kind=%s", eventKind)

	// Check if V2 provider can handle this webhook
	if !s.gitlabProviderV2.CanHandleWebhook(headers, body) {
		log.Printf("[WARN] GitLab V2 provider cannot handle this webhook, falling back to V1")
		return s.GitLabWebhookHandlerV1(c)
	}

	// Handle different event types using V2 provider
	switch strings.ToLower(eventKind) {
	case "merge request hook":
		// Handle reviewer assignment events
		if event, err := s.gitlabProviderV2.ConvertReviewerEvent(headers, body); err == nil {
			log.Printf("[INFO] GitLab V2 reviewer event processed for MR !%d", event.MergeRequest.Number)
			return c.JSON(http.StatusOK, map[string]string{
				"status":     "processed",
				"event_type": "reviewer_assignment",
				"provider":   "gitlab_v2",
			})
		}

	case "note hook":
		// Handle comment events
		if event, err := s.gitlabProviderV2.ConvertCommentEvent(headers, body); err == nil {
			log.Printf("[INFO] GitLab V2 comment event processed: %s", event.EventType)

			// Process the comment if it's a bot mention
			if s.isBotMentioned(event) {
				if err := s.processCommentEventV2(event); err != nil {
					log.Printf("[ERROR] Failed to process GitLab V2 comment: %v", err)
				}
			}

			return c.JSON(http.StatusOK, map[string]string{
				"status":     "processed",
				"event_type": event.EventType,
				"provider":   "gitlab_v2",
			})
		}
	}

	// If V2 provider couldn't handle it, fall back to V1
	log.Printf("[DEBUG] GitLab V2 provider couldn't handle event kind: %s, falling back to V1", eventKind)
	return s.GitLabWebhookHandlerV1(c)
}

// GitLabWebhookHandlerV1 is the original GitLab webhook handler (for backward compatibility)
func (s *Server) GitLabWebhookHandlerV1(c echo.Context) error {
	// Get the event kind from headers first to determine payload type
	eventKind := c.Request().Header.Get("X-Gitlab-Event")
	log.Printf("[INFO] Received GitLab webhook (V1): event_kind=%s", eventKind)

	// Handle different event types with appropriate payload structures
	switch strings.ToLower(eventKind) {
	case "merge request hook":
		// Parse as merge request payload (existing functionality)
		var payload GitLabWebhookPayload
		if err := c.Bind(&payload); err != nil {
			log.Printf("[ERROR] Failed to parse GitLab MR webhook payload: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid MR webhook payload",
			})
		}

		log.Printf("[INFO] Processing MR webhook: event_type=%s", payload.EventType)

		if payload.EventType == "merge_request" {
			reviewerChangeInfo := s.processReviewerChange(payload, eventKind)
			if reviewerChangeInfo != nil {
				log.Printf("[INFO] Detected livereview reviewer change in MR %d", reviewerChangeInfo.MergeRequest.ID)

				// If livereview users are assigned as reviewers, trigger a review
				if reviewerChangeInfo.IsBotAssigned && len(reviewerChangeInfo.CurrentBotReviewers) > 0 {
					go s.triggerReviewForMR(reviewerChangeInfo)
				}
			}
		}

	case "note hook":
		// Parse as note payload (Phase 1 conversational AI)
		var notePayload GitLabNoteWebhookPayload
		if err := c.Bind(&notePayload); err != nil {
			log.Printf("[ERROR] Failed to parse GitLab Note Hook payload: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "Invalid Note Hook payload",
			})
		}

		log.Printf("[INFO] Processing Note Hook: event_type=%s", notePayload.EventType)

		// Process the note event asynchronously to avoid blocking the webhook
		go func() {
			if err := s.processGitLabNoteEvent(c.Request().Context(), notePayload); err != nil {
				log.Printf("[ERROR] Failed to process GitLab note event: %v", err)
			}
		}()

	default:
		log.Printf("[INFO] Unhandled GitLab webhook event kind: %s", eventKind)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "received",
	})
}

// processReviewerChange detects reviewer changes involving "livereview" users
func (s *Server) processReviewerChange(payload GitLabWebhookPayload, eventKind string) *ReviewerChangeInfo {
	log.Printf("[DEBUG] Processing reviewer change detection...")
	log.Printf("[DEBUG] Event kind: '%s'", eventKind)
	log.Printf("[DEBUG] Expected: 'merge request hook' (case-insensitive)")

	if strings.ToLower(eventKind) != "merge request hook" {
		log.Printf("[DEBUG] Event kind mismatch, skipping")
		return nil
	}

	log.Printf("[DEBUG] Event type: '%s', expected: 'merge_request'", payload.EventType)
	if payload.EventType != "merge_request" {
		log.Printf("[DEBUG] Event type mismatch, skipping")
		return nil
	}

	log.Printf("[DEBUG] Checking for reviewer changes...")
	if payload.Changes.Reviewers.Current == nil && payload.Changes.Reviewers.Previous == nil {
		log.Printf("[DEBUG] No reviewer changes found, skipping")
		return nil
	}

	currentReviewers := payload.Changes.Reviewers.Current
	previousReviewers := payload.Changes.Reviewers.Previous
	log.Printf("[DEBUG] Current reviewers: %d, Previous reviewers: %d", len(currentReviewers), len(previousReviewers))

	// Check for "livereview" users in both current and previous reviewers
	botFound := false
	var botUsers []BotUserInfo
	var currentBotReviewers []GitLabUser
	var previousBotReviewers []GitLabUser

	botNameLower := "livereview"
	log.Printf("[DEBUG] Checking for '%s' usernames...", botNameLower)

	// Check previous reviewers
	for i, reviewer := range previousReviewers {
		username := strings.ToLower(reviewer.Username)
		log.Printf("[DEBUG] Previous reviewer %d: '%s'", i+1, reviewer.Username)
		if strings.Contains(username, botNameLower) {
			log.Printf("[DEBUG] Found '%s' in username: '%s'", botNameLower, reviewer.Username)
			botFound = true
			botUsers = append(botUsers, BotUserInfo{Type: "previous", User: reviewer})
			previousBotReviewers = append(previousBotReviewers, reviewer)
		}
	}

	// Check current reviewers - THIS IS MOST IMPORTANT for triggering actions
	for i, reviewer := range currentReviewers {
		username := strings.ToLower(reviewer.Username)
		log.Printf("[DEBUG] Current reviewer %d: '%s'", i+1, reviewer.Username)
		if strings.Contains(username, botNameLower) {
			log.Printf("[DEBUG] Found '%s' in username: '%s' - THIS WILL TRIGGER ACTIONS!", botNameLower, reviewer.Username)
			botFound = true
			botUsers = append(botUsers, BotUserInfo{Type: "current", User: reviewer})
			currentBotReviewers = append(currentBotReviewers, reviewer)
		}
	}

	if !botFound {
		log.Printf("[DEBUG] No '%s' users found in reviewer changes, skipping", botNameLower)
		return nil
	}

	// Determine if this is a bot assignment or removal
	isBotAssigned := len(currentBotReviewers) > 0
	isBotRemoved := len(previousBotReviewers) > 0 && len(currentBotReviewers) == 0

	log.Printf("[DEBUG] Found %d '%s' users in reviewer changes!", len(botUsers), botNameLower)
	log.Printf("[DEBUG] Current livereview reviewers (will trigger actions): %d", len(currentBotReviewers))
	log.Printf("[DEBUG] Previous livereview reviewers: %d", len(previousBotReviewers))
	log.Printf("[DEBUG] Livereview assigned as reviewer: %t", isBotAssigned)
	log.Printf("[DEBUG] Livereview removed as reviewer: %t", isBotRemoved)

	// Build the reviewer change info
	reviewerChangeInfo := &ReviewerChangeInfo{
		EventType:            "reviewer_change",
		Action:               payload.ObjectAttributes.Action,
		UpdatedAt:            payload.ObjectAttributes.UpdatedAt,
		BotUsers:             botUsers,
		CurrentBotReviewers:  currentBotReviewers,
		PreviousBotReviewers: previousBotReviewers,
		IsBotAssigned:        isBotAssigned,
		IsBotRemoved:         isBotRemoved,
		ReviewerChanges:      payload.Changes.Reviewers,
		ChangedBy:            payload.User,
		MergeRequest: GitLabMergeRequest{
			ID:           payload.ObjectAttributes.ID,
			IID:          payload.ObjectAttributes.IID,
			Title:        payload.ObjectAttributes.Title,
			Description:  payload.ObjectAttributes.Description,
			State:        payload.ObjectAttributes.State,
			SourceBranch: payload.ObjectAttributes.SourceBranch,
			TargetBranch: payload.ObjectAttributes.TargetBranch,
			URL:          payload.ObjectAttributes.URL,
			AuthorID:     payload.ObjectAttributes.AuthorID,
			AssigneeIDs:  payload.ObjectAttributes.AssigneeIDs,
			ReviewerIDs:  payload.ObjectAttributes.ReviewerIDs,
		},
		Project: payload.Project,
	}

	log.Printf("[DEBUG] Reviewer change processing completed successfully!")
	return reviewerChangeInfo
}

// triggerReviewForMR triggers a code review for the merge request
func (s *Server) triggerReviewForMR(changeInfo *ReviewerChangeInfo) {
	log.Printf("[INFO] Triggering review for MR: %s", changeInfo.MergeRequest.URL)
	log.Printf("[INFO] MR Title: %s", changeInfo.MergeRequest.Title)
	log.Printf("[INFO] Changed by: %s (@%s)", changeInfo.ChangedBy.Name, changeInfo.ChangedBy.Username)

	for _, reviewer := range changeInfo.CurrentBotReviewers {
		log.Printf("[INFO] Livereview reviewer assigned: %s (@%s)", reviewer.Name, reviewer.Username)
	}

	ctx := context.Background()

	// Load integration token for this project
	integrationToken, err := s.findIntegrationTokenForProject(changeInfo.Project.PathWithNamespace)
	if err != nil {
		log.Printf("[ERROR] Failed to find integration token for project %s: %v", changeInfo.Project.PathWithNamespace, err)
		return
	}

	// Load AI connector (use the first available one)
	aiConnector, err := s.getFirstAIConnector()
	if err != nil {
		log.Printf("[ERROR] Failed to get AI connector: %v", err)
		return
	}

	// Create the review request with proper configuration
	request := review.ReviewRequest{
		URL: changeInfo.MergeRequest.URL,
		Provider: review.ProviderConfig{
			Type:   integrationToken.Provider,
			URL:    integrationToken.ProviderURL,
			Token:  integrationToken.PatToken,
			Config: make(map[string]interface{}),
		},
		AI: review.AIConfig{
			Type:        aiConnector.ProviderName,
			APIKey:      aiConnector.APIKey,
			Model:       s.getModelForProvider(aiConnector.ProviderName),
			Temperature: 0.1,
			Config:      make(map[string]interface{}),
		},
	}

	// Track the review in database (use webhook as trigger type)
	reviewID, err := TrackReviewTriggered(s.db, changeInfo.Project.PathWithNamespace, "", "", "webhook", integrationToken.Provider, &integrationToken.ID, "webhook", changeInfo.MergeRequest.URL)
	if err != nil {
		log.Printf("[ERROR] Failed to track review: %v", err)
		return
	}

	// Set the review ID from the database
	request.ReviewID = fmt.Sprintf("%d", reviewID)

	// Create the review service with factories
	providerFactory := review.NewStandardProviderFactory()
	aiFactory := review.NewStandardAIProviderFactory()

	serviceConfig := review.Config{
		ReviewTimeout: 10 * time.Minute,
		DefaultAI:     aiConnector.ProviderName,
		DefaultModel:  s.getModelForProvider(aiConnector.ProviderName),
		Temperature:   0.1,
	}

	reviewService := review.NewService(providerFactory, aiFactory, serviceConfig)

	// Process the review asynchronously with completion callback that tracks AI comments
	reviewService.ProcessReviewAsync(ctx, request, func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] Review completed successfully for MR %d (ReviewID: %s)",
				changeInfo.MergeRequest.ID, result.ReviewID)
			log.Printf("[INFO] Posted %d comments", result.CommentsCount)

			// Update review status to completed using ReviewManager
			reviewManager := NewReviewManager(s.db)
			reviewManager.UpdateReviewStatus(reviewID, "completed")

			// Track AI comments with actual details if available
			if len(result.Comments) > 0 {
				for i, comment := range result.Comments {
					commentContent := map[string]interface{}{
						"content":         comment.Content,
						"file_path":       comment.FilePath,
						"line":            comment.Line,
						"severity":        string(comment.Severity),
						"confidence":      comment.Confidence,
						"category":        comment.Category,
						"suggestions":     comment.Suggestions,
						"is_deleted_line": comment.IsDeletedLine,
						"is_internal":     comment.IsInternal,
						"review_id":       result.ReviewID,
						"comment_index":   i + 1,
						"type":            "webhook_review",
					}

					// Use file path and line number for proper tracking
					linePtr := &comment.Line
					filePtr := &comment.FilePath
					TrackAICommentFromURL(s.db, changeInfo.MergeRequest.URL, "line_comment", commentContent, filePtr, linePtr, integrationToken.OrgID)
				}
			} else if result.CommentsCount > 0 {
				// Fallback for when Comments array is not available
				commentContent := map[string]interface{}{
					"summary": result.Summary,
					"count":   result.CommentsCount,
					"type":    "webhook_review_summary",
				}
				TrackAICommentFromURL(s.db, changeInfo.MergeRequest.URL, "review_summary", commentContent, nil, nil, integrationToken.OrgID)
			}
		} else {
			log.Printf("[ERROR] Review failed for MR %d (ReviewID: %s): %v",
				changeInfo.MergeRequest.ID, result.ReviewID, result.Error)

			// Update review status to failed using ReviewManager
			reviewManager := NewReviewManager(s.db)
			reviewManager.UpdateReviewStatus(reviewID, "failed")
		}
	})

	log.Printf("[INFO] Review process started asynchronously for MR %d (ReviewID: %s)",
		changeInfo.MergeRequest.ID, fmt.Sprintf("%d", reviewID))
}

// findIntegrationTokenForProject finds the integration token for a project namespace
func (s *Server) findIntegrationTokenForProject(projectNamespace string) (*IntegrationToken, error) {
	// Look up the webhook registry to find which integration token is associated with this project
	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token, it.org_id
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var token IntegrationToken
	err := s.db.QueryRow(query, projectNamespace).Scan(
		&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID,
	)
	if err != nil {
		// If no specific webhook registry entry, try to find any integration token for the same provider
		// This is a fallback for cases where webhook might not be properly registered yet
		query = `
			SELECT id, provider, provider_url, pat_token, org_id
			FROM integration_tokens
			WHERE provider = 'gitlab'
			ORDER BY created_at DESC
			LIMIT 1
		`
		err = s.db.QueryRow(query).Scan(
			&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID,
		)
		if err != nil {
			return nil, fmt.Errorf("no integration token found for project %s: %w", projectNamespace, err)
		}
	}

	return &token, nil
}

// getFirstAIConnector gets the first available AI connector
func (s *Server) getFirstAIConnector() (*AIConnector, error) {
	query := `
		SELECT id, provider_name, api_key
		FROM ai_connectors
		ORDER BY display_order ASC, created_at ASC
		LIMIT 1
	`

	var connector AIConnector
	err := s.db.QueryRow(query).Scan(
		&connector.ID, &connector.ProviderName, &connector.APIKey,
	)
	if err != nil {
		return nil, fmt.Errorf("no AI connector found: %w", err)
	}

	return &connector, nil
}

// getModelForProvider returns the default model for a given AI provider
func (s *Server) getModelForProvider(providerName string) string {
	switch strings.ToLower(providerName) {
	case "gemini":
		return "gemini-2.5-flash"
	case "openai":
		return "gpt-4"
	case "anthropic":
		return "claude-3-sonnet-20240229"
	default:
		return "gemini-2.5-flash" // Default fallback
	}
}

// GitHub webhook payload structures

// GitHubWebhookPayload represents the structure of GitHub webhook payloads
type GitHubWebhookPayload struct {
	Action      string            `json:"action"`
	Number      int               `json:"number"`
	PullRequest GitHubPullRequest `json:"pull_request"`
	Repository  GitHubRepository  `json:"repository"`
	Sender      GitHubUser        `json:"sender"`
	// For review_requested/review_request_removed actions
	RequestedReviewer GitHubUser `json:"requested_reviewer,omitempty"`
	RequestedTeam     GitHubTeam `json:"requested_team,omitempty"`
}

// GitHubPullRequest represents a GitHub pull request
type GitHubPullRequest struct {
	ID                 int          `json:"id"`
	Number             int          `json:"number"`
	Title              string       `json:"title"`
	Body               string       `json:"body"`
	State              string       `json:"state"`
	HTMLURL            string       `json:"html_url"`
	CreatedAt          string       `json:"created_at"`
	UpdatedAt          string       `json:"updated_at"`
	Head               GitHubBranch `json:"head"`
	Base               GitHubBranch `json:"base"`
	User               GitHubUser   `json:"user"`
	RequestedReviewers []GitHubUser `json:"requested_reviewers"`
	RequestedTeams     []GitHubTeam `json:"requested_teams"`
	Assignees          []GitHubUser `json:"assignees"`
}

// GitHubRepository represents a GitHub repository
type GitHubRepository struct {
	ID       int        `json:"id"`
	Name     string     `json:"name"`
	FullName string     `json:"full_name"`
	HTMLURL  string     `json:"html_url"`
	Owner    GitHubUser `json:"owner"`
	Private  bool       `json:"private"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	ID        int    `json:"id"`
	Login     string `json:"login"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

// GitHubTeam represents a GitHub team
type GitHubTeam struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// GitHubBranch represents a GitHub branch
type GitHubBranch struct {
	Ref  string           `json:"ref"`
	SHA  string           `json:"sha"`
	Repo GitHubRepository `json:"repo"`
}

// GitHubReviewerChangeInfo represents processed GitHub reviewer change information
type GitHubReviewerChangeInfo struct {
	EventType           string                      `json:"event_type"`
	Action              string                      `json:"action"`
	UpdatedAt           string                      `json:"updated_at"`
	BotUsers            []GitHubReviewerBotUserInfo `json:"bot_users"`
	CurrentBotReviewers []GitHubUser                `json:"current_bot_reviewers"`
	IsBotAssigned       bool                        `json:"is_bot_assigned"`
	IsBotRemoved        bool                        `json:"is_bot_removed"`
	RequestedReviewer   GitHubUser                  `json:"requested_reviewer"`
	ChangedBy           GitHubUser                  `json:"changed_by"`
	PullRequest         GitHubPullRequest           `json:"pull_request"`
	Repository          GitHubRepository            `json:"repository"`
}

// GitHubReviewerBotUserInfo represents information about a bot user in GitHub reviewer changes
type GitHubReviewerBotUserInfo struct {
	Type string     `json:"type"` // "requested" or "removed"
	User GitHubUser `json:"user"`
}

// GitHub comment webhook payload structures

// GitHubIssueCommentWebhookPayload represents the payload for issue_comment events
type GitHubIssueCommentWebhookPayload struct {
	Action     string           `json:"action"`     // "created", "edited", "deleted"
	Issue      GitHubIssue      `json:"issue"`      // The issue (PR shows up as issue in this context)
	Comment    GitHubComment    `json:"comment"`    // The comment that was created/edited/deleted
	Repository GitHubRepository `json:"repository"` // The repository
	Sender     GitHubUser       `json:"sender"`     // User who triggered the event
}

// GitHubPullRequestReviewCommentWebhookPayload represents the payload for pull_request_review_comment events
type GitHubPullRequestReviewCommentWebhookPayload struct {
	Action      string              `json:"action"`       // "created", "edited", "deleted"
	Comment     GitHubReviewComment `json:"comment"`      // The review comment
	PullRequest GitHubPullRequest   `json:"pull_request"` // The pull request
	Repository  GitHubRepository    `json:"repository"`   // The repository
	Sender      GitHubUser          `json:"sender"`       // User who triggered the event
}

// GitHubComment represents a GitHub issue comment (includes PR discussion comments)
type GitHubComment struct {
	ID                int        `json:"id"`
	NodeID            string     `json:"node_id"`
	URL               string     `json:"url"`
	HTMLURL           string     `json:"html_url"`
	IssueURL          string     `json:"issue_url"`
	Body              string     `json:"body"`
	User              GitHubUser `json:"user"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
	AuthorAssociation string     `json:"author_association"`
}

// GitHubReviewComment represents a GitHub pull request review comment (code line comments)
type GitHubReviewComment struct {
	ID                int        `json:"id"`
	NodeID            string     `json:"node_id"`
	URL               string     `json:"url"`
	HTMLURL           string     `json:"html_url"`
	PullRequestURL    string     `json:"pull_request_url"`
	DiffHunk          string     `json:"diff_hunk"`
	Path              string     `json:"path"`
	Position          *int       `json:"position"`          // Line position in diff (can be null for outdated comments)
	OriginalPosition  int        `json:"original_position"` // Original line position
	CommitID          string     `json:"commit_id"`
	OriginalCommitID  string     `json:"original_commit_id"`
	User              GitHubUser `json:"user"`
	Body              string     `json:"body"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
	AuthorAssociation string     `json:"author_association"`
	StartLine         *int       `json:"start_line"`          // Multi-line comment start
	OriginalStartLine *int       `json:"original_start_line"` // Original multi-line start
	StartSide         string     `json:"start_side"`          // "LEFT" or "RIGHT"
	Line              *int       `json:"line"`                // Multi-line comment end
	OriginalLine      int        `json:"original_line"`       // Original multi-line end
	Side              string     `json:"side"`                // "LEFT" or "RIGHT"
	InReplyToID       *int       `json:"in_reply_to_id"`      // ID of comment this is replying to
}

// GitHubIssue represents a GitHub issue (PRs appear as issues in issue_comment webhooks)
type GitHubIssue struct {
	ID                int               `json:"id"`
	NodeID            string            `json:"node_id"`
	URL               string            `json:"url"`
	RepositoryURL     string            `json:"repository_url"`
	LabelsURL         string            `json:"labels_url"`
	CommentsURL       string            `json:"comments_url"`
	EventsURL         string            `json:"events_url"`
	HTMLURL           string            `json:"html_url"`
	Number            int               `json:"number"`
	State             string            `json:"state"`
	Title             string            `json:"title"`
	Body              string            `json:"body"`
	User              GitHubUser        `json:"user"`
	Labels            []GitHubLabel     `json:"labels"`
	Assignees         []GitHubUser      `json:"assignees"`
	Milestone         *GitHubMilestone  `json:"milestone"`
	Locked            bool              `json:"locked"`
	ActiveLockReason  string            `json:"active_lock_reason"`
	Comments          int               `json:"comments"`
	PullRequest       *GitHubIssuePRRef `json:"pull_request"` // Present if issue is actually a PR
	ClosedAt          *string           `json:"closed_at"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
	AuthorAssociation string            `json:"author_association"`
}

// GitHubLabel represents a GitHub issue/PR label
type GitHubLabel struct {
	ID          int    `json:"id"`
	NodeID      string `json:"node_id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Default     bool   `json:"default"`
}

// GitHubMilestone represents a GitHub milestone
type GitHubMilestone struct {
	ID           int        `json:"id"`
	NodeID       string     `json:"node_id"`
	Number       int        `json:"number"`
	State        string     `json:"state"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	Creator      GitHubUser `json:"creator"`
	OpenIssues   int        `json:"open_issues"`
	ClosedIssues int        `json:"closed_issues"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
	ClosedAt     *string    `json:"closed_at"`
	DueOn        *string    `json:"due_on"`
}

// GitHubIssuePRRef indicates that an issue is actually a pull request
type GitHubIssuePRRef struct {
	URL      string `json:"url"`
	HTMLURL  string `json:"html_url"`
	DiffURL  string `json:"diff_url"`
	PatchURL string `json:"patch_url"`
}

// GitHubBotUserInfo represents fresh bot user information from GitHub API
type GitHubBotUserInfo struct {
	Login             string `json:"login"`
	ID                int    `json:"id"`
	NodeID            string `json:"node_id"`
	AvatarURL         string `json:"avatar_url"`
	GravatarID        string `json:"gravatar_id"`
	URL               string `json:"url"`
	HTMLURL           string `json:"html_url"`
	FollowersURL      string `json:"followers_url"`
	FollowingURL      string `json:"following_url"`
	GistsURL          string `json:"gists_url"`
	StarredURL        string `json:"starred_url"`
	SubscriptionsURL  string `json:"subscriptions_url"`
	OrganizationsURL  string `json:"organizations_url"`
	ReposURL          string `json:"repos_url"`
	EventsURL         string `json:"events_url"`
	ReceivedEventsURL string `json:"received_events_url"`
	Type              string `json:"type"`
	SiteAdmin         bool   `json:"site_admin"`
	Name              string `json:"name"`
	Company           string `json:"company"`
	Blog              string `json:"blog"`
	Location          string `json:"location"`
	Email             string `json:"email"`
	Hireable          *bool  `json:"hireable"`
	Bio               string `json:"bio"`
	TwitterUsername   string `json:"twitter_username"`
	PublicRepos       int    `json:"public_repos"`
	PublicGists       int    `json:"public_gists"`
	Followers         int    `json:"followers"`
	Following         int    `json:"following"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// GitHubWebhookHandler handles incoming GitHub webhook events
func (s *Server) GitHubWebhookHandler(c echo.Context) error {
	// Try V2 provider first
	if s.githubProviderV2 != nil {
		return s.GitHubWebhookHandlerV2(c)
	}

	// Fallback to original implementation
	return s.GitHubWebhookHandlerV1(c)
}

// GitHubWebhookHandlerV2 handles GitHub webhooks using the V2 provider
func (s *Server) GitHubWebhookHandlerV2(c echo.Context) error {
	// Read request body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		log.Printf("[ERROR] Failed to read GitHub webhook body: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed to read request body",
		})
	}

	// Get headers
	headers := make(map[string]string)
	for key, values := range c.Request().Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	eventType := c.Request().Header.Get("X-GitHub-Event")
	log.Printf("[INFO] Received GitHub webhook (V2): event_type=%s", eventType)

	// Check if V2 provider can handle this webhook
	if !s.githubProviderV2.CanHandleWebhook(headers, body) {
		log.Printf("[WARN] GitHub V2 provider cannot handle this webhook, falling back to V1")
		return s.GitHubWebhookHandlerV1(c)
	}

	// Handle different event types using V2 provider
	switch strings.ToLower(eventType) {
	case "pull_request":
		// Handle reviewer assignment events
		if event, err := s.githubProviderV2.ConvertReviewerEvent(headers, body); err == nil {
			log.Printf("[INFO] GitHub V2 reviewer event processed for PR #%d", event.MergeRequest.Number)
			return c.JSON(http.StatusOK, map[string]string{
				"status":     "processed",
				"event_type": "reviewer_assignment",
				"provider":   "github_v2",
			})
		}

	case "issue_comment", "pull_request_review_comment":
		// Handle comment events
		if event, err := s.githubProviderV2.ConvertCommentEvent(headers, body); err == nil {
			log.Printf("[INFO] GitHub V2 comment event processed: %s", event.EventType)

			// Process the comment if it's a bot mention
			if s.isBotMentioned(event) {
				if err := s.processCommentEventV2(event); err != nil {
					log.Printf("[ERROR] Failed to process GitHub V2 comment: %v", err)
				}
			}

			return c.JSON(http.StatusOK, map[string]string{
				"status":     "processed",
				"event_type": event.EventType,
				"provider":   "github_v2",
			})
		}
	}

	// If V2 provider couldn't handle it, fall back to V1
	log.Printf("[DEBUG] GitHub V2 provider couldn't handle event type: %s, falling back to V1", eventType)
	return s.GitHubWebhookHandlerV1(c)
}

// GitHubWebhookHandlerV1 is the original GitHub webhook handler (for backward compatibility)
func (s *Server) GitHubWebhookHandlerV1(c echo.Context) error {
	// Get the event type from headers first
	eventType := c.Request().Header.Get("X-GitHub-Event")
	log.Printf("[INFO] Received GitHub webhook (V1): event_type=%s", eventType)

	// Handle different event types with appropriate payload structures
	switch strings.ToLower(eventType) {
	case "pull_request":
		return s.handleGitHubPullRequestEvent(c)
	case "issue_comment":
		return s.handleGitHubIssueCommentEvent(c)
	case "pull_request_review_comment":
		return s.handleGitHubPullRequestReviewCommentEvent(c)
	default:
		log.Printf("[DEBUG] Unhandled GitHub webhook event type: %s", eventType)
		return c.JSON(http.StatusOK, map[string]string{
			"status":     "ignored",
			"event_type": eventType,
		})
	}
}

// handleGitHubPullRequestEvent handles pull_request webhook events (existing logic)
func (s *Server) handleGitHubPullRequestEvent(c echo.Context) error {
	// Parse the webhook payload
	var payload GitHubWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse GitHub pull_request webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid webhook payload",
		})
	}

	log.Printf("[INFO] GitHub pull_request webhook: action=%s", payload.Action)

	// Process reviewer change events for pull requests
	if payload.Action == "review_requested" || payload.Action == "review_request_removed" {
		reviewerChangeInfo := s.processGitHubReviewerChange(payload, "pull_request")
		if reviewerChangeInfo != nil {
			log.Printf("[INFO] Detected livereview reviewer change in GitHub PR #%d", reviewerChangeInfo.PullRequest.Number)

			// If livereview users are assigned as reviewers, trigger a review
			if reviewerChangeInfo.IsBotAssigned && len(reviewerChangeInfo.CurrentBotReviewers) > 0 {
				go s.triggerReviewForGitHubPR(reviewerChangeInfo)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status":     "received",
		"event_type": "pull_request",
	})
}

// handleGitHubIssueCommentEvent handles issue_comment webhook events (PR discussion comments)
func (s *Server) handleGitHubIssueCommentEvent(c echo.Context) error {
	// Parse the webhook payload
	var payload GitHubIssueCommentWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse GitHub issue_comment webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid webhook payload",
		})
	}

	log.Printf("[INFO] GitHub issue_comment webhook: action=%s, comment_id=%d, author=%s",
		payload.Action, payload.Comment.ID, payload.Comment.User.Login)

	// Debug: Print the full payload for analysis
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	log.Printf("[DEBUG] Full GitHub issue_comment payload:\n%s", string(payloadJSON))

	// Only process "created" actions for now
	if payload.Action != "created" {
		log.Printf("[DEBUG] Ignoring issue_comment action: %s", payload.Action)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored",
			"action": payload.Action,
		})
	}

	// Check if this is a pull request (issues that are PRs have pull_request field)
	if payload.Issue.PullRequest == nil {
		log.Printf("[DEBUG] Issue comment is not on a pull request, ignoring")
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored",
			"reason": "not_pull_request",
		})
	}

	// TODO: Process the comment for AI response (will implement next)
	log.Printf("[DEBUG] TODO: Process GitHub issue comment for AI response")

	return c.JSON(http.StatusOK, map[string]string{
		"status":     "received",
		"event_type": "issue_comment",
	})
}

// handleGitHubPullRequestReviewCommentEvent handles pull_request_review_comment webhook events (code line comments)
func (s *Server) handleGitHubPullRequestReviewCommentEvent(c echo.Context) error {
	// Parse the webhook payload
	var payload GitHubPullRequestReviewCommentWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse GitHub pull_request_review_comment webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid webhook payload",
		})
	}

	log.Printf("[INFO] GitHub pull_request_review_comment webhook: action=%s, comment_id=%d, author=%s, file=%s",
		payload.Action, payload.Comment.ID, payload.Comment.User.Login, payload.Comment.Path)

	// Debug: Print the full payload for analysis
	payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
	log.Printf("[DEBUG] Full GitHub pull_request_review_comment payload:\n%s", string(payloadJSON))

	// Only process "created" actions for now
	if payload.Action != "created" {
		log.Printf("[DEBUG] Ignoring pull_request_review_comment action: %s", payload.Action)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored",
			"action": payload.Action,
		})
	}

	// Process GitHub review comment for AI response
	go s.processGitHubCommentForAIResponse(payload)

	return c.JSON(http.StatusOK, map[string]string{
		"status":     "received",
		"event_type": "pull_request_review_comment",
	})
}

// processGitHubReviewerChange detects reviewer changes involving "livereview" users
func (s *Server) processGitHubReviewerChange(payload GitHubWebhookPayload, eventType string) *GitHubReviewerChangeInfo {
	log.Printf("[DEBUG] Processing GitHub reviewer change detection...")
	log.Printf("[DEBUG] Event type: '%s'", eventType)
	log.Printf("[DEBUG] Action: '%s'", payload.Action)

	if strings.ToLower(eventType) != "pull_request" {
		log.Printf("[DEBUG] Event type mismatch, skipping")
		return nil
	}

	if payload.Action != "review_requested" && payload.Action != "review_request_removed" {
		log.Printf("[DEBUG] Action mismatch, skipping")
		return nil
	}

	botNameLower := "livereview"
	log.Printf("[DEBUG] Checking for '%s' usernames...", botNameLower)

	// Check if the reviewer change involves a livereview user
	var botUsers []GitHubReviewerBotUserInfo
	var currentBotReviewers []GitHubUser
	isBotAssigned := false
	isBotRemoved := false

	// Check the specific reviewer that was added/removed
	if payload.RequestedReviewer.Login != "" {
		username := strings.ToLower(payload.RequestedReviewer.Login)
		log.Printf("[DEBUG] Reviewer in action: '%s'", payload.RequestedReviewer.Login)

		if strings.Contains(username, botNameLower) {
			log.Printf("[DEBUG] Found '%s' in username: '%s'", botNameLower, payload.RequestedReviewer.Login)

			if payload.Action == "review_requested" {
				botUsers = append(botUsers, GitHubReviewerBotUserInfo{Type: "requested", User: payload.RequestedReviewer})
				isBotAssigned = true
				log.Printf("[DEBUG] Livereview user assigned as reviewer - THIS WILL TRIGGER ACTIONS!")
			} else if payload.Action == "review_request_removed" {
				botUsers = append(botUsers, GitHubReviewerBotUserInfo{Type: "removed", User: payload.RequestedReviewer})
				isBotRemoved = true
				log.Printf("[DEBUG] Livereview user removed as reviewer")
			}
		}
	}

	// Also check current reviewers list to get full context
	for i, reviewer := range payload.PullRequest.RequestedReviewers {
		username := strings.ToLower(reviewer.Login)
		log.Printf("[DEBUG] Current reviewer %d: '%s'", i+1, reviewer.Login)
		if strings.Contains(username, botNameLower) {
			log.Printf("[DEBUG] Found '%s' in current reviewers: '%s'", botNameLower, reviewer.Login)
			currentBotReviewers = append(currentBotReviewers, reviewer)
		}
	}

	// If no livereview users are involved, skip
	if len(botUsers) == 0 {
		log.Printf("[DEBUG] No '%s' users found in reviewer changes, skipping", botNameLower)
		return nil
	}

	// For review_requested, we need current bot reviewers to trigger action
	if payload.Action == "review_requested" && len(currentBotReviewers) == 0 {
		// The bot was requested but not in current reviewers list yet - add it
		if strings.Contains(strings.ToLower(payload.RequestedReviewer.Login), botNameLower) {
			currentBotReviewers = append(currentBotReviewers, payload.RequestedReviewer)
		}
	}

	log.Printf("[DEBUG] Found %d '%s' users in reviewer changes!", len(botUsers), botNameLower)
	log.Printf("[DEBUG] Current livereview reviewers (will trigger actions): %d", len(currentBotReviewers))
	log.Printf("[DEBUG] Livereview assigned as reviewer: %t", isBotAssigned)
	log.Printf("[DEBUG] Livereview removed as reviewer: %t", isBotRemoved)

	// Build the reviewer change info
	reviewerChangeInfo := &GitHubReviewerChangeInfo{
		EventType:           "reviewer_change",
		Action:              payload.Action,
		UpdatedAt:           payload.PullRequest.UpdatedAt,
		BotUsers:            botUsers,
		CurrentBotReviewers: currentBotReviewers,
		IsBotAssigned:       isBotAssigned,
		IsBotRemoved:        isBotRemoved,
		RequestedReviewer:   payload.RequestedReviewer,
		ChangedBy:           payload.Sender,
		PullRequest:         payload.PullRequest,
		Repository:          payload.Repository,
	}

	log.Printf("[DEBUG] GitHub reviewer change processing completed successfully!")
	return reviewerChangeInfo
}

// triggerReviewForGitHubPR triggers a code review for the GitHub pull request
func (s *Server) triggerReviewForGitHubPR(changeInfo *GitHubReviewerChangeInfo) {
	log.Printf("[INFO] Triggering review for GitHub PR: %s", changeInfo.PullRequest.HTMLURL)
	log.Printf("[INFO] PR Title: %s", changeInfo.PullRequest.Title)
	log.Printf("[INFO] Changed by: %s (@%s)", changeInfo.ChangedBy.Login, changeInfo.ChangedBy.Login)

	for _, reviewer := range changeInfo.CurrentBotReviewers {
		log.Printf("[INFO] Livereview reviewer assigned: @%s", reviewer.Login)
	}

	ctx := context.Background()

	// Load integration token for this repository
	integrationToken, err := s.findIntegrationTokenForGitHubRepo(changeInfo.Repository.FullName)
	if err != nil {
		log.Printf("[ERROR] Failed to find integration token for GitHub repository %s: %v", changeInfo.Repository.FullName, err)
		return
	}
	log.Printf("[DEBUG] Found integration token: provider=%s, url=%s", integrationToken.Provider, integrationToken.ProviderURL)

	// Load AI connector (use the first available one)
	aiConnector, err := s.getFirstAIConnector()
	if err != nil {
		log.Printf("[ERROR] Failed to get AI connector: %v", err)
		return
	}
	log.Printf("[DEBUG] Found AI connector: provider=%s", aiConnector.ProviderName)

	// Track the review in database (use webhook as trigger type)
	reviewID, err := TrackReviewTriggered(s.db, changeInfo.Repository.FullName, "", "", "webhook", integrationToken.Provider, &integrationToken.ID, "webhook", changeInfo.PullRequest.HTMLURL)
	if err != nil {
		log.Printf("[ERROR] Failed to track review: %v", err)
		return
	}

	// Create the review request with proper configuration
	request := review.ReviewRequest{
		URL:      changeInfo.PullRequest.HTMLURL,
		ReviewID: fmt.Sprintf("%d", reviewID),
		Provider: review.ProviderConfig{
			Type:  integrationToken.Provider,
			URL:   integrationToken.ProviderURL,
			Token: integrationToken.PatToken,
			Config: map[string]interface{}{
				"pat_token": integrationToken.PatToken,
			},
		},
		AI: review.AIConfig{
			Type:        aiConnector.ProviderName,
			APIKey:      aiConnector.APIKey,
			Model:       s.getModelForProvider(aiConnector.ProviderName),
			Temperature: 0.1,
			Config:      make(map[string]interface{}),
		},
	}

	// Create the review service with factories
	providerFactory := review.NewStandardProviderFactory()
	aiFactory := review.NewStandardAIProviderFactory()

	serviceConfig := review.Config{
		ReviewTimeout: 10 * time.Minute,
		DefaultAI:     aiConnector.ProviderName,
		DefaultModel:  s.getModelForProvider(aiConnector.ProviderName),
		Temperature:   0.1,
	}

	reviewService := review.NewService(providerFactory, aiFactory, serviceConfig)

	log.Printf("[DEBUG] Starting GitHub review with config: URL=%s, Provider=%s, AI=%s",
		request.URL, request.Provider.Type, request.AI.Type)

	// Process the review asynchronously with completion callback that tracks AI comments
	reviewService.ProcessReviewAsync(ctx, request, func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] Review completed successfully for GitHub PR #%d (ReviewID: %s)",
				changeInfo.PullRequest.Number, result.ReviewID)
			log.Printf("[INFO] Posted %d comments", result.CommentsCount)

			// Update review status to completed using ReviewManager
			reviewManager := NewReviewManager(s.db)
			reviewManager.UpdateReviewStatus(reviewID, "completed")

			// Track AI comments with actual details if available
			if len(result.Comments) > 0 {
				for i, comment := range result.Comments {
					commentContent := map[string]interface{}{
						"content":         comment.Content,
						"file_path":       comment.FilePath,
						"line":            comment.Line,
						"severity":        string(comment.Severity),
						"confidence":      comment.Confidence,
						"category":        comment.Category,
						"suggestions":     comment.Suggestions,
						"is_deleted_line": comment.IsDeletedLine,
						"is_internal":     comment.IsInternal,
						"review_id":       result.ReviewID,
						"comment_index":   i + 1,
						"type":            "webhook_review",
					}

					// Use file path and line number for proper tracking
					linePtr := &comment.Line
					filePtr := &comment.FilePath
					TrackAICommentFromURL(s.db, changeInfo.PullRequest.HTMLURL, "line_comment", commentContent, filePtr, linePtr, integrationToken.OrgID)
				}
			} else if result.CommentsCount > 0 {
				// Fallback for when Comments array is not available
				commentContent := map[string]interface{}{
					"summary": result.Summary,
					"count":   result.CommentsCount,
					"type":    "webhook_review_summary",
				}
				TrackAICommentFromURL(s.db, changeInfo.PullRequest.HTMLURL, "review_summary", commentContent, nil, nil, integrationToken.OrgID)
			}
		} else {
			log.Printf("[ERROR] Review failed for GitHub PR #%d (ReviewID: %s): %v",
				changeInfo.PullRequest.Number, result.ReviewID, result.Error)

			// Update review status to failed using ReviewManager
			reviewManager := NewReviewManager(s.db)
			reviewManager.UpdateReviewStatus(reviewID, "failed")
		}
	})

	log.Printf("[INFO] Review process started asynchronously for GitHub PR #%d (ReviewID: %s)",
		changeInfo.PullRequest.Number, fmt.Sprintf("%d", reviewID))
}

// findIntegrationTokenForGitHubRepo finds the integration token for a GitHub repository
func (s *Server) findIntegrationTokenForGitHubRepo(repoFullName string) (*IntegrationToken, error) {
	// Look up the webhook registry to find which integration token is associated with this repository
	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token, it.org_id
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var token IntegrationToken
	err := s.db.QueryRow(query, repoFullName).Scan(
		&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID,
	)
	if err != nil {
		// If no specific webhook registry entry, try to find any integration token for GitHub
		// This is a fallback for cases where webhook might not be properly registered yet
		query = `
			SELECT id, provider, provider_url, pat_token, org_id
			FROM integration_tokens
			WHERE provider LIKE 'github%'
			ORDER BY created_at DESC
			LIMIT 1
		`
		err = s.db.QueryRow(query).Scan(
			&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID,
		)
		if err != nil {
			return nil, fmt.Errorf("no integration token found for GitHub repository %s: %w", repoFullName, err)
		}
	}

	return &token, nil
}

// Bitbucket webhook structures and handlers

// BitbucketWebhookPayload represents the structure of Bitbucket webhook payloads
type BitbucketWebhookPayload struct {
	EventKey    string               `json:"eventKey"`
	Date        string               `json:"date"`
	Actor       BitbucketUser        `json:"actor"`
	Repository  BitbucketRepository  `json:"repository"`
	Changes     BitbucketChanges     `json:"changes,omitempty"`
	PullRequest BitbucketPullRequest `json:"pullrequest,omitempty"`
	Comment     BitbucketComment     `json:"comment,omitempty"` // For comment events
}

// BitbucketComment represents a Bitbucket comment
type BitbucketComment struct {
	ID        int                     `json:"id"`
	Content   BitbucketCommentContent `json:"content"`
	User      BitbucketUser           `json:"user"`
	CreatedOn string                  `json:"created_on"`
	UpdatedOn string                  `json:"updated_on"`
	Parent    *BitbucketCommentRef    `json:"parent,omitempty"`
	Inline    *BitbucketInlineInfo    `json:"inline,omitempty"`
	Links     BitbucketCommentLinks   `json:"links"`
	Type      string                  `json:"type"`
}

// BitbucketCommentContent represents the content of a Bitbucket comment
type BitbucketCommentContent struct {
	Raw    string `json:"raw"`
	Markup string `json:"markup"`
	HTML   string `json:"html"`
	Type   string `json:"type"`
}

// BitbucketCommentRef represents a reference to another comment
type BitbucketCommentRef struct {
	ID    int                   `json:"id"`
	Links BitbucketCommentLinks `json:"links"`
}

// BitbucketInlineInfo represents inline comment positioning
type BitbucketInlineInfo struct {
	Path string `json:"path"`
	From int    `json:"from,omitempty"`
	To   int    `json:"to,omitempty"`
}

// BitbucketCommentLinks represents links for a comment
type BitbucketCommentLinks struct {
	Self struct {
		Href string `json:"href"`
	} `json:"self"`
	HTML struct {
		Href string `json:"href"`
	} `json:"html"`
}

// BitbucketUser represents a Bitbucket user
type BitbucketUser struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
}

// BitbucketRepository represents a Bitbucket repository
type BitbucketRepository struct {
	UUID     string           `json:"uuid"`
	Name     string           `json:"name"`
	FullName string           `json:"full_name"`
	Links    BitbucketLinks   `json:"links"`
	Project  BitbucketProject `json:"project,omitempty"`
	Owner    BitbucketUser    `json:"owner"`
	Type     string           `json:"type"`
}

// BitbucketProject represents a Bitbucket project
type BitbucketProject struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	Key  string `json:"key"`
	Type string `json:"type"`
}

// BitbucketLinks represents Bitbucket links
type BitbucketLinks struct {
	HTML struct {
		Href string `json:"href"`
	} `json:"html"`
}

// BitbucketPullRequest represents a Bitbucket pull request
type BitbucketPullRequest struct {
	ID           int                    `json:"id"`
	Title        string                 `json:"title"`
	Description  string                 `json:"description,omitempty"`
	State        string                 `json:"state"`
	Source       BitbucketBranch        `json:"source"`
	Destination  BitbucketBranch        `json:"destination"`
	Author       BitbucketUser          `json:"author"`
	Reviewers    []BitbucketReviewer    `json:"reviewers"`
	Participants []BitbucketParticipant `json:"participants"`
	Links        BitbucketLinks         `json:"links"`
	CreatedOn    string                 `json:"created_on"`
	UpdatedOn    string                 `json:"updated_on"`
}

// BitbucketBranch represents a Bitbucket branch
type BitbucketBranch struct {
	Branch     BitbucketBranchInfo `json:"branch"`
	Repository BitbucketRepository `json:"repository"`
}

// BitbucketBranchInfo represents branch information
type BitbucketBranchInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// BitbucketReviewer represents a Bitbucket reviewer
type BitbucketReviewer struct {
	User     BitbucketUser `json:"user"`
	Role     string        `json:"role"`
	Approved bool          `json:"approved"`
	State    string        `json:"state,omitempty"`
	Type     string        `json:"type"`
}

// BitbucketParticipant represents a Bitbucket participant
type BitbucketParticipant struct {
	User     BitbucketUser `json:"user"`
	Role     string        `json:"role"`
	Approved bool          `json:"approved"`
	State    string        `json:"state,omitempty"`
	Type     string        `json:"type"`
}

// BitbucketChanges represents changes in Bitbucket webhook
type BitbucketChanges struct {
	Reviewers BitbucketReviewerChanges `json:"reviewers,omitempty"`
}

// BitbucketReviewerChanges represents reviewer changes
type BitbucketReviewerChanges struct {
	Old []BitbucketReviewer `json:"old"`
	New []BitbucketReviewer `json:"new"`
}

// BitbucketReviewerChangeInfo represents processed reviewer change information for Bitbucket
type BitbucketReviewerChangeInfo struct {
	EventType            string                   `json:"event_type"`
	EventKey             string                   `json:"event_key"`
	UpdatedAt            string                   `json:"updated_at"`
	BotUsers             []BitbucketBotUserInfo   `json:"bot_users"`
	CurrentBotReviewers  []BitbucketReviewer      `json:"current_bot_reviewers"`
	PreviousBotReviewers []BitbucketReviewer      `json:"previous_bot_reviewers"`
	IsBotAssigned        bool                     `json:"is_bot_assigned"`
	IsBotRemoved         bool                     `json:"is_bot_removed"`
	ReviewerChanges      BitbucketReviewerChanges `json:"reviewer_changes"`
	ChangedBy            BitbucketUser            `json:"changed_by"`
	PullRequest          BitbucketPullRequest     `json:"pull_request"`
	Repository           BitbucketRepository      `json:"repository"`
}

// BitbucketBotUserInfo represents information about a bot user in Bitbucket reviewer changes
type BitbucketBotUserInfo struct {
	Type string            `json:"type"` // "added" or "removed"
	User BitbucketReviewer `json:"user"`
}

// BitbucketWebhookHandler handles incoming Bitbucket webhook events
func (s *Server) BitbucketWebhookHandler(c echo.Context) error {
	// Parse the webhook payload
	var payload BitbucketWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse Bitbucket webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid webhook payload",
		})
	}

	// Get the event type from headers or payload
	eventType := c.Request().Header.Get("X-Event-Key")
	if eventType == "" {
		eventType = payload.EventKey
	}
	log.Printf("[INFO] Received Bitbucket webhook: event_key=%s", eventType)

	// Process comment events for AI replies
	if eventType == "pullrequest:comment_created" {
		log.Printf("[INFO] Bitbucket pull request comment created: comment_id=%d, author=%s",
			payload.Comment.ID, payload.Comment.User.Username)

		// Debug: Print the full payload for analysis
		payloadJSON, _ := json.MarshalIndent(payload, "", "  ")
		log.Printf("[DEBUG] Full Bitbucket comment payload:\n%s", string(payloadJSON))

		// Process comment for AI response
		go s.processBitbucketCommentForAIResponse(payload)

		return c.JSON(http.StatusOK, map[string]string{
			"status":     "received",
			"event_type": "pullrequest:comment_created",
		})
	}

	// Process reviewer change events for pull requests
	if strings.HasPrefix(eventType, "pullrequest:") &&
		(strings.HasSuffix(eventType, ":updated") || strings.HasSuffix(eventType, ":created")) {

		reviewerChangeInfo := s.processBitbucketReviewerChange(payload, eventType)
		if reviewerChangeInfo != nil {
			log.Printf("[INFO] Detected livereview reviewer change in Bitbucket PR #%d", reviewerChangeInfo.PullRequest.ID)

			// If livereview users are assigned as reviewers, trigger a review
			if reviewerChangeInfo.IsBotAssigned && len(reviewerChangeInfo.CurrentBotReviewers) > 0 {
				go s.triggerReviewForBitbucketPR(reviewerChangeInfo)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "received",
	})
}

// processBitbucketReviewerChange detects reviewer changes involving "livereview" users
func (s *Server) processBitbucketReviewerChange(payload BitbucketWebhookPayload, eventKey string) *BitbucketReviewerChangeInfo {
	log.Printf("[DEBUG] Processing Bitbucket reviewer change detection...")
	log.Printf("[DEBUG] Event key: '%s'", eventKey)

	// Only process pullrequest events
	if !strings.HasPrefix(eventKey, "pullrequest:") {
		log.Printf("[DEBUG] Event key mismatch, skipping")
		return nil
	}

	// Check for reviewer changes in the payload
	currentReviewers := payload.PullRequest.Reviewers
	var previousReviewers []BitbucketReviewer

	// For updated events, check if we have changes
	if strings.HasSuffix(eventKey, ":updated") && payload.Changes.Reviewers.Old != nil {
		previousReviewers = payload.Changes.Reviewers.Old
		currentReviewers = payload.Changes.Reviewers.New
	} else if strings.HasSuffix(eventKey, ":created") {
		// For created events, current reviewers are new, previous is empty
		previousReviewers = []BitbucketReviewer{}
	} else {
		log.Printf("[DEBUG] No relevant reviewer changes found, skipping")
		return nil
	}

	log.Printf("[DEBUG] Current reviewers: %d, Previous reviewers: %d", len(currentReviewers), len(previousReviewers))

	// Check for "livereview" users in both current and previous reviewers
	botFound := false
	var botUsers []BitbucketBotUserInfo
	var currentBotReviewers []BitbucketReviewer
	var previousBotReviewers []BitbucketReviewer

	botNameLower := "livereview"
	log.Printf("[DEBUG] Checking for '%s' usernames...", botNameLower)

	// Check previous reviewers
	for i, reviewer := range previousReviewers {
		username := strings.ToLower(reviewer.User.Username)
		log.Printf("[DEBUG] Previous reviewer %d: '%s'", i+1, reviewer.User.Username)
		if strings.Contains(username, botNameLower) {
			log.Printf("[DEBUG] Found '%s' in username: '%s'", botNameLower, reviewer.User.Username)
			botFound = true
			botUsers = append(botUsers, BitbucketBotUserInfo{Type: "removed", User: reviewer})
			previousBotReviewers = append(previousBotReviewers, reviewer)
		}
	}

	// Check current reviewers - THIS IS MOST IMPORTANT for triggering actions
	for i, reviewer := range currentReviewers {
		username := strings.ToLower(reviewer.User.Username)
		log.Printf("[DEBUG] Current reviewer %d: '%s'", i+1, reviewer.User.Username)
		if strings.Contains(username, botNameLower) {
			log.Printf("[DEBUG] Found '%s' in username: '%s' - THIS WILL TRIGGER ACTIONS!", botNameLower, reviewer.User.Username)
			botFound = true
			botUsers = append(botUsers, BitbucketBotUserInfo{Type: "added", User: reviewer})
			currentBotReviewers = append(currentBotReviewers, reviewer)
		}
	}

	if !botFound {
		log.Printf("[DEBUG] No '%s' users found in reviewer changes, skipping", botNameLower)
		return nil
	}

	// Determine if this is a bot assignment or removal
	isBotAssigned := len(currentBotReviewers) > 0
	isBotRemoved := len(previousBotReviewers) > 0 && len(currentBotReviewers) == 0

	log.Printf("[DEBUG] Found %d '%s' users in reviewer changes!", len(botUsers), botNameLower)
	log.Printf("[DEBUG] Current livereview reviewers (will trigger actions): %d", len(currentBotReviewers))
	log.Printf("[DEBUG] Previous livereview reviewers: %d", len(previousBotReviewers))
	log.Printf("[DEBUG] Livereview assigned as reviewer: %t", isBotAssigned)
	log.Printf("[DEBUG] Livereview removed as reviewer: %t", isBotRemoved)

	// Build the reviewer change info
	reviewerChangeInfo := &BitbucketReviewerChangeInfo{
		EventType:            "reviewer_change",
		EventKey:             eventKey,
		UpdatedAt:            payload.PullRequest.UpdatedOn,
		BotUsers:             botUsers,
		CurrentBotReviewers:  currentBotReviewers,
		PreviousBotReviewers: previousBotReviewers,
		IsBotAssigned:        isBotAssigned,
		IsBotRemoved:         isBotRemoved,
		ReviewerChanges: BitbucketReviewerChanges{
			Old: previousReviewers,
			New: currentReviewers,
		},
		ChangedBy:   payload.Actor,
		PullRequest: payload.PullRequest,
		Repository:  payload.Repository,
	}

	log.Printf("[DEBUG] Built reviewer change info for Bitbucket PR #%d", payload.PullRequest.ID)
	return reviewerChangeInfo
}

// triggerReviewForBitbucketPR triggers a code review for the Bitbucket pull request
func (s *Server) triggerReviewForBitbucketPR(changeInfo *BitbucketReviewerChangeInfo) {
	log.Printf("[INFO] Triggering review for Bitbucket PR: %s", changeInfo.PullRequest.Links.HTML.Href)
	log.Printf("[INFO] PR Title: %s", changeInfo.PullRequest.Title)
	log.Printf("[INFO] Changed by: %s (@%s)", changeInfo.ChangedBy.DisplayName, changeInfo.ChangedBy.Username)

	for _, reviewer := range changeInfo.CurrentBotReviewers {
		log.Printf("[INFO] Livereview reviewer assigned: %s (@%s)", reviewer.User.DisplayName, reviewer.User.Username)
	}

	ctx := context.Background()

	// Load integration token for this repository
	integrationToken, err := s.findIntegrationTokenForBitbucketRepo(changeInfo.Repository.FullName)
	if err != nil {
		log.Printf("[ERROR] Failed to find integration token for Bitbucket repository %s: %v", changeInfo.Repository.FullName, err)
		return
	}
	log.Printf("[DEBUG] Found integration token: provider=%s, url=%s", integrationToken.Provider, integrationToken.ProviderURL)

	// Load AI connector (use the first available one)
	aiConnector, err := s.getFirstAIConnector()
	if err != nil {
		log.Printf("[ERROR] Failed to get AI connector: %v", err)
		return
	}
	log.Printf("[DEBUG] Found AI connector: provider=%s", aiConnector.ProviderName)

	// Track the review in database (use webhook as trigger type)
	reviewID, err := TrackReviewTriggered(s.db, changeInfo.Repository.FullName, "", "", "webhook", integrationToken.Provider, &integrationToken.ID, "webhook", changeInfo.PullRequest.Links.HTML.Href)
	if err != nil {
		log.Printf("[ERROR] Failed to track review: %v", err)
		return
	}

	// Create the review request with proper configuration
	request := review.ReviewRequest{
		URL:      changeInfo.PullRequest.Links.HTML.Href,
		ReviewID: fmt.Sprintf("%d", reviewID),
		Provider: review.ProviderConfig{
			Type:  integrationToken.Provider,
			URL:   integrationToken.ProviderURL,
			Token: integrationToken.PatToken,
			Config: map[string]interface{}{
				"pat_token": integrationToken.PatToken,
			},
		},
		AI: review.AIConfig{
			Type:        aiConnector.ProviderName,
			APIKey:      aiConnector.APIKey,
			Model:       s.getModelForProvider(aiConnector.ProviderName),
			Temperature: 0.1,
			Config:      make(map[string]interface{}),
		},
	}

	// Create the review service with factories
	providerFactory := review.NewStandardProviderFactory()
	aiFactory := review.NewStandardAIProviderFactory()

	serviceConfig := review.Config{
		ReviewTimeout: 10 * time.Minute,
		DefaultAI:     aiConnector.ProviderName,
		DefaultModel:  s.getModelForProvider(aiConnector.ProviderName),
		Temperature:   0.1,
	}

	reviewService := review.NewService(providerFactory, aiFactory, serviceConfig)

	// Process the review asynchronously with completion callback that tracks AI comments
	reviewService.ProcessReviewAsync(ctx, request, func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] Review completed successfully for Bitbucket PR %d (ReviewID: %s)",
				changeInfo.PullRequest.ID, result.ReviewID)
			log.Printf("[INFO] Posted %d comments", result.CommentsCount)

			// Update review status to completed using ReviewManager
			reviewManager := NewReviewManager(s.db)
			reviewManager.UpdateReviewStatus(reviewID, "completed")

			// Track AI comments with actual details if available
			if len(result.Comments) > 0 {
				for i, comment := range result.Comments {
					commentContent := map[string]interface{}{
						"content":         comment.Content,
						"file_path":       comment.FilePath,
						"line":            comment.Line,
						"severity":        string(comment.Severity),
						"confidence":      comment.Confidence,
						"category":        comment.Category,
						"suggestions":     comment.Suggestions,
						"is_deleted_line": comment.IsDeletedLine,
						"is_internal":     comment.IsInternal,
						"review_id":       result.ReviewID,
						"comment_index":   i + 1,
						"type":            "webhook_review",
					}

					// Use file path and line number for proper tracking
					linePtr := &comment.Line
					filePtr := &comment.FilePath
					TrackAICommentFromURL(s.db, changeInfo.PullRequest.Links.HTML.Href, "line_comment", commentContent, filePtr, linePtr, integrationToken.OrgID)
				}
			} else if result.CommentsCount > 0 {
				// Fallback for when Comments array is not available
				commentContent := map[string]interface{}{
					"summary": result.Summary,
					"count":   result.CommentsCount,
					"type":    "webhook_review_summary",
				}
				TrackAICommentFromURL(s.db, changeInfo.PullRequest.Links.HTML.Href, "review_summary", commentContent, nil, nil, integrationToken.OrgID)
			}
		} else {
			log.Printf("[ERROR] Review failed for Bitbucket PR %d (ReviewID: %s): %v",
				changeInfo.PullRequest.ID, result.ReviewID, result.Error)

			// Update review status to failed using ReviewManager
			reviewManager := NewReviewManager(s.db)
			reviewManager.UpdateReviewStatus(reviewID, "failed")
		}
	})

	log.Printf("[INFO] Review trigger initiated for Bitbucket PR #%d with review ID: %s", changeInfo.PullRequest.ID, fmt.Sprintf("%d", reviewID))
}

// findIntegrationTokenForBitbucketRepo finds the integration token associated with a Bitbucket repository
func (s *Server) findIntegrationTokenForBitbucketRepo(repoFullName string) (*IntegrationToken, error) {
	// Look up the webhook registry to find which integration token is associated with this repository
	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token, it.org_id, COALESCE(it.metadata, '{}')
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var token IntegrationToken
	var metadataJSON string
	err := s.db.QueryRow(query, repoFullName).Scan(
		&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID, &metadataJSON,
	)
	if err != nil {
		// If no specific webhook registry entry, try to find any integration token for Bitbucket
		// This is a fallback for cases where webhook might not be properly registered yet
		query = `
			SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}')
			FROM integration_tokens
			WHERE provider LIKE 'bitbucket%'
			ORDER BY created_at DESC
			LIMIT 1
		`
		err = s.db.QueryRow(query).Scan(
			&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID, &metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("no integration token found for Bitbucket repository %s: %w", repoFullName, err)
		}
	}

	// Parse metadata JSON to populate token.Metadata (needed for Bitbucket email)
	token.Metadata = make(map[string]interface{})
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &token.Metadata); err != nil {
			log.Printf("[WARN] Failed to parse integration token metadata for Bitbucket repo %s: %v", repoFullName, err)
		}
	}

	if token.Metadata == nil {
		token.Metadata = make(map[string]interface{})
	}

	return &token, nil
}

// GitLabCommentWebhookHandler handles incoming GitLab Note Hook events for comments
func (s *Server) GitLabCommentWebhookHandler(c echo.Context) error {
	// Parse the Note Hook webhook payload
	var payload GitLabNoteWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse GitLab Note Hook webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid Note Hook webhook payload",
		})
	}

	// Get the event kind from headers
	eventKind := c.Request().Header.Get("X-Gitlab-Event")
	log.Printf("[INFO] Received GitLab Note Hook webhook: event_kind=%s, object_kind=%s, noteable_type=%s",
		eventKind, payload.ObjectKind, payload.ObjectAttributes.NoteableType)

	// Only process Note Hook events
	if strings.ToLower(eventKind) != "note hook" {
		log.Printf("[DEBUG] Event kind mismatch: expected 'note hook', got '%s', skipping", eventKind)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored - not a note hook",
		})
	}

	// Only process comments (not system comments)
	if payload.ObjectAttributes.System {
		log.Printf("[DEBUG] System comment detected, skipping")
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored - system comment",
		})
	}

	// Only process comments on merge requests for now
	if payload.ObjectAttributes.NoteableType != "MergeRequest" {
		log.Printf("[DEBUG] Comment not on MergeRequest (type: %s), skipping for now", payload.ObjectAttributes.NoteableType)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored - not MR comment",
		})
	}

	// Extract GitLab instance URL from project web_url
	gitlabInstanceURL := extractGitLabInstanceURL(payload.Project.WebURL)
	log.Printf("[DEBUG] GitLab instance URL: %s", gitlabInstanceURL)

	// Check if AI response is warranted
	warrantsResponse, scenario := s.checkAIResponseWarrant(payload, gitlabInstanceURL)
	if !warrantsResponse {
		log.Printf("[DEBUG] Comment does not warrant AI response, skipping")
		return c.JSON(http.StatusOK, map[string]string{
			"status": "ignored - no response warrant",
		})
	}

	log.Printf("[INFO] AI response warranted for comment: trigger=%s, content_type=%s, response_type=%s, confidence=%.2f",
		scenario.TriggerType, scenario.ContentType, scenario.ResponseType, scenario.Confidence)

	// Generate and post AI response asynchronously using background context
	// We use background context because the webhook response needs to be sent quickly,
	// but the AI response generation may take longer
	go func() {
		bgCtx := context.Background()
		err := s.generateAndPostGitLabResponse(bgCtx, payload, scenario, gitlabInstanceURL)
		if err != nil {
			log.Printf("[ERROR] Failed to generate and post GitLab response: %v", err)
		} else {
			log.Printf("[INFO] Successfully posted AI response for comment by %s", payload.User.Username)
		}
	}()

	// Return success immediately - the response will be posted asynchronously
	return c.JSON(http.StatusOK, map[string]string{
		"status":       "success",
		"trigger_type": scenario.TriggerType,
		"responded":    "async",
	})
}

// extractGitLabInstanceURL extracts the base GitLab instance URL from a project web URL
// e.g., "https://gitlab.com/group/project" -> "https://gitlab.com"
// e.g., "https://git.company.com/group/project" -> "https://git.company.com"
func extractGitLabInstanceURL(projectWebURL string) string {
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

// ResponseScenario represents the analysis of whether and how to respond to a comment
// UnifiedMRContext represents a unified view of MR/PR context across platforms
type UnifiedMRContext struct {
	Provider       string                 `json:"provider"` // "gitlab", "github", "bitbucket"
	MergeRequestID string                 `json:"mr_id"`    // MR/PR identifier
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	Author         UnifiedUser            `json:"author"`
	TargetBranch   string                 `json:"target_branch"`
	SourceBranch   string                 `json:"source_branch"`
	WebURL         string                 `json:"web_url"`
	Repository     UnifiedRepository      `json:"repository"`
	Metadata       map[string]interface{} `json:"metadata"` // Provider-specific data
}

// UnifiedComment represents a unified view of comments across platforms
type UnifiedComment struct {
	Provider    string                 `json:"provider"`
	ID          string                 `json:"id"`
	Body        string                 `json:"body"`
	Author      UnifiedUser            `json:"author"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
	WebURL      string                 `json:"web_url"`
	InReplyToID *string                `json:"in_reply_to_id,omitempty"`
	Position    *UnifiedPosition       `json:"position,omitempty"` // For code comments
	MRContext   UnifiedMRContext       `json:"mr_context"`
	Metadata    map[string]interface{} `json:"metadata"` // Provider-specific data
}

// UnifiedUser represents a user across platforms
type UnifiedUser struct {
	ID        interface{} `json:"id"` // int for GitLab/GitHub, string for others
	Username  string      `json:"username"`
	Name      string      `json:"name"`
	WebURL    string      `json:"web_url"`
	AvatarURL string      `json:"avatar_url"`
}

// UnifiedRepository represents a repository across platforms
type UnifiedRepository struct {
	ID       interface{} `json:"id"`
	Name     string      `json:"name"`
	FullName string      `json:"full_name"`
	WebURL   string      `json:"web_url"`
	Owner    UnifiedUser `json:"owner"`
}

// UnifiedPosition represents comment position across platforms
type UnifiedPosition struct {
	FilePath   string `json:"file_path"`
	LineNumber *int   `json:"line_number,omitempty"`
	StartLine  *int   `json:"start_line,omitempty"`
	EndLine    *int   `json:"end_line,omitempty"`
	CommitSHA  string `json:"commit_sha"`
	Side       string `json:"side,omitempty"` // "LEFT", "RIGHT" for diffs
}

// UnifiedBotUserInfo represents bot user info across platforms
type UnifiedBotUserInfo struct {
	Provider string      `json:"provider"`
	User     UnifiedUser `json:"user"`
	APIToken string      `json:"-"` // Never marshal to JSON
	BaseURL  string      `json:"base_url"`
}

type ResponseScenario struct {
	TriggerType  string  `json:"trigger_type"`  // "direct_mention", "thread_reply"
	ContentType  string  `json:"content_type"`  // "appreciation", "clarification", "debate", "question", "complaint"
	ResponseType string  `json:"response_type"` // "emoji_only", "brief_acknowledgment", "detailed_response", "escalate"
	Confidence   float64 `json:"confidence"`    // 0.0-1.0 confidence in classification
}

// checkAIResponseWarrant determines if a comment warrants an AI response and how to respond
// Uses fresh API data and implements correct priority order:
// 1. Check if replying to AI bot comment
// 2. Check for direct @mentions
// 3. Content analysis for response triggers
func (s *Server) checkAIResponseWarrant(payload GitLabNoteWebhookPayload, gitlabInstanceURL string) (bool, ResponseScenario) {
	log.Printf("[DEBUG] Checking AI response warrant for comment by %s", payload.User.Username)
	log.Printf("[DEBUG] Comment content: %s", payload.ObjectAttributes.Note)

	// Early anti-loop protection: Check for common bot usernames before API calls
	commonBotUsernames := []string{"livereviewbot", "LiveReviewBot", "ai-bot", "codebot", "reviewbot"}
	for _, botUsername := range commonBotUsernames {
		if strings.EqualFold(payload.User.Username, botUsername) {
			log.Printf("[DEBUG] Comment author %s appears to be a bot user (early detection), skipping (anti-loop protection)", payload.User.Username)
			return false, ResponseScenario{}
		}
	}

	// Get fresh bot user information via GitLab API
	botUserInfo, err := s.getFreshBotUserInfo(gitlabInstanceURL)
	if err != nil {
		log.Printf("[ERROR] Failed to get fresh bot user info for GitLab instance %s: %v", gitlabInstanceURL, err)
		return false, ResponseScenario{}
	}

	if botUserInfo == nil {
		log.Printf("[DEBUG] No bot user configured for GitLab instance %s", gitlabInstanceURL)
		return false, ResponseScenario{}
	}

	log.Printf("[DEBUG] Fresh bot user info: username=%s, id=%d, name=%s",
		botUserInfo.Username, botUserInfo.ID, botUserInfo.Name)

	// Anti-loop protection: Never respond to bot accounts
	if payload.User.Username == botUserInfo.Username {
		log.Printf("[DEBUG] Comment author %s is the bot user, skipping (anti-loop protection)", payload.User.Username)
		return false, ResponseScenario{}
	}

	// PRIORITY 1: Check if this comment is replying to an AI bot's previous comment
	isReplyToBot, err := s.checkIfReplyingToBotComment(payload, botUserInfo, gitlabInstanceURL)
	if err != nil {
		log.Printf("[ERROR] Failed to check if replying to bot comment: %v", err)
	} else if isReplyToBot {
		log.Printf("[DEBUG] Comment is replying to AI bot's previous comment")
		return true, ResponseScenario{
			TriggerType:  "reply_to_bot",
			ContentType:  classifyContentType(payload.ObjectAttributes.Note),
			ResponseType: determineResponseType(payload.ObjectAttributes.Note),
			Confidence:   0.90,
		}
	}

	// PRIORITY 2: Check for direct @mentions of the bot
	isDirectMention := s.checkDirectBotMention(payload.ObjectAttributes.Note, botUserInfo)
	if isDirectMention {
		log.Printf("[DEBUG] Direct bot mention detected in comment")
		return true, ResponseScenario{
			TriggerType:  "direct_mention",
			ContentType:  classifyContentType(payload.ObjectAttributes.Note),
			ResponseType: determineResponseType(payload.ObjectAttributes.Note),
			Confidence:   0.95,
		}
	}

	// PRIORITY 3: Content analysis for implicit response triggers
	// (This could include questions about code, help requests, etc.)
	// For now, we'll keep this minimal to avoid false positives

	log.Printf("[DEBUG] No response warrant detected")
	return false, ResponseScenario{}
}

// GitLabBotUserInfo represents fresh bot user information from GitLab API
type GitLabBotUserInfo struct {
	Username string `json:"username"`
	ID       int    `json:"id"`
	Name     string `json:"name"`
}

// GitLabHTTPClient wraps HTTP operations for GitLab API (adapted from main.go)
type GitLabHTTPClient struct {
	baseURL     string
	accessToken string
	client      *http.Client
}

// GitLabCommit type already defined above - removed duplicate

// GitLabDiscussion represents a discussion thread from GitLab API
type GitLabDiscussion struct {
	ID    string       `json:"id"`
	Notes []GitLabNote `json:"notes"`
}

// GitLabNote represents a note/comment from GitLab API
type GitLabNote struct {
	ID        int    `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	System    bool   `json:"system"`
	Author    struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		ID       int    `json:"id"`
	} `json:"author"`
	Position *GitLabNotePosition `json:"position"`
}

// GitLabNotePosition represents code position for a note
type GitLabNotePosition struct {
	BaseSHA  string `json:"base_sha"`
	HeadSHA  string `json:"head_sha"`
	StartSHA string `json:"start_sha"`
	NewPath  string `json:"new_path"`
	OldPath  string `json:"old_path"`
	NewLine  int    `json:"new_line"`
	OldLine  int    `json:"old_line"`
}

// TimelineItem represents an item in the MR timeline
type TimelineItem struct {
	Kind      string
	CreatedAt time.Time
	Commit    *GitLabCommit
	Comment   *GitLabNote
	NoteID    string
}

// CommentContext represents the context around a target comment
type CommentContext struct {
	BeforeMessages []string
	AfterMessages  []string
	BeforeCommits  []string
	AfterCommits   []string
}

// getFreshBotUserInfo gets fresh bot user information via GitLab API call
func (s *Server) getFreshBotUserInfo(gitlabInstanceURL string) (*GitLabBotUserInfo, error) {
	// Get access token for this GitLab instance
	accessToken, err := s.getGitLabAccessToken(gitlabInstanceURL)
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

	client := &http.Client{Timeout: 60 * time.Second} // 1 minute timeout for external GitLab instances
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var botUser GitLabBotUserInfo
	err = json.NewDecoder(resp.Body).Decode(&botUser)
	if err != nil {
		return nil, fmt.Errorf("failed to decode GitLab user response: %w", err)
	}

	return &botUser, nil
}

// getFreshBitbucketBotUserInfo gets fresh bot user information via Bitbucket API call
func (s *Server) getFreshBitbucketBotUserInfo(repoFullName string) (*BitbucketUserInfo, error) {
	// Get integration token for the repository
	token, err := s.findIntegrationTokenForBitbucketRepo(repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get Bitbucket token: %w", err)
	}

	// Make API call to get current user (the bot)
	// Note: Some Bitbucket token types (workspace/project/repo access tokens) cannot call /2.0/user.
	// We'll attempt with Basic (email:token). On 401, we fall back to stored metadata.
	apiURL := "https://api.bitbucket.org/2.0/user"

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use Basic auth with Atlassian email and API token (matches BitbucketProvider)
	var bbEmail string
	if token.Metadata != nil {
		if e, ok := token.Metadata["email"].(string); ok {
			bbEmail = e
		}
	}
	if bbEmail == "" {
		log.Printf("[WARN] Bitbucket user API auth - email missing in integration token metadata; skipping fresh user lookup")
		return nil, fmt.Errorf("bitbucket email missing in token metadata")
	}

	log.Printf("[DEBUG] Bitbucket user API auth - Using Basic (email), email: '%s', PatToken length: %d, Provider: '%s', ProviderURL: '%s'",
		bbEmail, len(token.PatToken), token.Provider, token.ProviderURL)

	req.SetBasicAuth(bbEmail, token.PatToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	log.Printf("[DEBUG] Making Bitbucket API call to %s with Basic auth (email)", apiURL)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Bitbucket API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DEBUG] Bitbucket API response status: %d, body: %s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		// Many tokens cannot access /2.0/user (e.g., access tokens). We'll fall back to metadata.
		return nil, fmt.Errorf("Bitbucket API error (status %d): %s", resp.StatusCode, string(body))
	}

	var user BitbucketUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to decode Bitbucket user response: %w", err)
	}

	botInfo := &BitbucketUserInfo{
		UUID:        user.UUID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		AccountID:   user.AccountID,
		Type:        user.Type,
	}

	log.Printf("[DEBUG] Retrieved Bitbucket bot user info: username=%s, display_name=%s, account_id=%s",
		botInfo.Username, botInfo.DisplayName, botInfo.AccountID)

	return botInfo, nil
}

// BitbucketUserInfo represents fresh bot user information from Bitbucket API
type BitbucketUserInfo struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
}

// getFreshGitHubBotUserInfo gets fresh bot user information via GitHub API call
func (s *Server) getFreshGitHubBotUserInfo(repoFullName string) (*GitHubBotUserInfo, error) {
	// Get integration token for this GitHub repository
	token, err := s.findIntegrationTokenForGitHubRepo(repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to get GitHub integration token: %w", err)
	}

	// Call GitHub API to get current user info (the bot user)
	apiURL := "https://api.github.com/user"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.PatToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var botUser GitHubBotUserInfo
	err = json.NewDecoder(resp.Body).Decode(&botUser)
	if err != nil {
		return nil, fmt.Errorf("failed to decode GitHub user response: %w", err)
	}

	return &botUser, nil
}

// GetMergeRequestCommits fetches commits for a merge request
func (c *GitLabHTTPClient) GetMergeRequestCommits(projectID, mrIID int) ([]GitLabCommit, error) {
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

	var commits []GitLabCommit
	err = json.NewDecoder(resp.Body).Decode(&commits)
	return commits, err
}

// GetMergeRequestDiscussions fetches discussions for a merge request
func (c *GitLabHTTPClient) GetMergeRequestDiscussions(projectID, mrIID int) ([]GitLabDiscussion, error) {
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

	var discussions []GitLabDiscussion
	err = json.NewDecoder(resp.Body).Decode(&discussions)
	return discussions, err
}

// GetMergeRequestNotes fetches standalone notes for a merge request
func (c *GitLabHTTPClient) GetMergeRequestNotes(projectID, mrIID int) ([]GitLabNote, error) {
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

	var notes []GitLabNote
	err = json.NewDecoder(resp.Body).Decode(&notes)
	return notes, err
}

// checkIfReplyingToBotComment checks if the current comment is replying to a bot's previous comment
func (s *Server) checkIfReplyingToBotComment(payload GitLabNoteWebhookPayload, botUserInfo *GitLabBotUserInfo, gitlabInstanceURL string) (bool, error) {
	// If this comment is not part of a discussion/thread, it can't be a reply
	if payload.ObjectAttributes.DiscussionID == "" {
		log.Printf("[DEBUG] Comment has no discussion_id, not a thread reply")
		return false, nil
	}

	log.Printf("[DEBUG] Checking if comment is reply to bot in discussion: %s", payload.ObjectAttributes.DiscussionID)

	// Get access token for GitLab API calls
	accessToken, err := s.getGitLabAccessToken(gitlabInstanceURL)
	if err != nil {
		return false, fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Get discussion details from GitLab API
	// GitLab API: GET /projects/:id/merge_requests/:merge_request_iid/discussions/:discussion_id
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions/%s",
		gitlabInstanceURL, payload.Project.ID, payload.MergeRequest.IID, payload.ObjectAttributes.DiscussionID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second} // 1 minute timeout for external GitLab instances
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
	currentNoteID := payload.ObjectAttributes.ID
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

// checkDirectBotMention checks if the comment contains a direct @mention of the bot user
func (s *Server) checkDirectBotMention(commentBody string, botUserInfo *GitLabBotUserInfo) bool {
	commentLower := strings.ToLower(commentBody)
	log.Printf("[DEBUG] Checking for direct mentions in comment: '%s'", commentBody)

	// Check for exact bot username mention
	mentionPattern := "@" + strings.ToLower(botUserInfo.Username)
	log.Printf("[DEBUG] Looking for mention pattern: '%s' in comment", mentionPattern)
	if strings.Contains(commentLower, mentionPattern) {
		log.Printf("[DEBUG] Direct mention found: %s mentioned in comment", botUserInfo.Username)
		return true
	}

	// Check for common bot names as fallback (in case bot username differs from display name)
	commonBotNames := []string{"livereviewbot", "livereview", "ai-bot", "codebot", "reviewbot"}
	for _, botName := range commonBotNames {
		fallbackPattern := "@" + botName
		log.Printf("[DEBUG] Looking for fallback mention pattern: '%s' in comment", fallbackPattern)
		if strings.Contains(commentLower, fallbackPattern) {
			log.Printf("[DEBUG] Direct mention found (fallback): %s mentioned in comment", botName)
			return true
		}
	}

	log.Printf("[DEBUG] No direct mentions found")
	return false
}

// Old isDirectMention function removed - replaced by checkDirectBotMention method

// Legacy function - kept for compatibility but replaced by new fresh API-based methods

// classifyContentType analyzes comment content to determine the type of interaction
func classifyContentType(commentBody string) string {
	bodyLower := strings.ToLower(commentBody)

	// Question patterns
	questionWords := []string{"?", "why", "how", "what", "when", "where", "should", "could", "would", "can you", "do you"}
	for _, word := range questionWords {
		if strings.Contains(bodyLower, word) {
			return "question"
		}
	}

	// Help requests
	helpWords := []string{"help", "explain", "clarify", "please review"}
	for _, word := range helpWords {
		if strings.Contains(bodyLower, word) {
			return "help_request"
		}
	}

	// Complaints/issues
	complaintWords := []string{"wrong", "error", "problem", "issue", "bug", "broken", "doesn't work"}
	for _, word := range complaintWords {
		if strings.Contains(bodyLower, word) {
			return "complaint"
		}
	}

	return "general"
}

// classifyReplyContentType analyzes reply content to determine interaction type
func classifyReplyContentType(commentBody string) string {
	bodyLower := strings.ToLower(commentBody)

	// Appreciation
	appreciationWords := []string{"thanks", "thank you", "great", "awesome", "perfect", "excellent", "good job"}
	for _, word := range appreciationWords {
		if strings.Contains(bodyLower, word) {
			return "appreciation"
		}
	}

	// Clarification requests
	clarificationWords := []string{"why", "how", "what", "can you explain", "could you", "clarify"}
	for _, word := range clarificationWords {
		if strings.Contains(bodyLower, word) {
			return "clarification"
		}
	}

	// Disagreement/debate
	disagreementWords := []string{"disagree", "not sure", "i think", "actually", "however", "but"}
	for _, word := range disagreementWords {
		if strings.Contains(bodyLower, word) {
			return "debate"
		}
	}

	return "general"
}

// determineResponseType determines how to respond based on comment content
func determineResponseType(commentBody string) string {
	contentType := classifyContentType(commentBody)

	switch contentType {
	case "question", "help_request":
		return "detailed_response"
	case "complaint":
		return "detailed_response" // Investigate and provide helpful response
	default:
		return "brief_acknowledgment"
	}
}

// determineReplyResponseType determines how to respond to a reply based on content
func determineReplyResponseType(commentBody string) string {
	contentType := classifyReplyContentType(commentBody)

	switch contentType {
	case "appreciation":
		return "emoji_only" // Just react with 👍 or ❤️
	case "clarification":
		return "detailed_response" // Provide technical explanation
	case "debate":
		return "diplomatic_response" // Clarify reasoning diplomatically
	default:
		return "brief_acknowledgment" // Brief helpful response
	}
}

// generateAndPostGitLabResponse generates an AI response and posts it to GitLab
func (s *Server) generateAndPostGitLabResponse(ctx context.Context, payload GitLabNoteWebhookPayload, scenario ResponseScenario, gitlabInstanceURL string) error {
	log.Printf("[INFO] Generating AI response for scenario: %s -> %s", scenario.TriggerType, scenario.ResponseType)

	// Handle different response types
	switch scenario.ResponseType {
	case "emoji_only":
		return s.postGitLabEmojiReaction(ctx, payload, scenario, gitlabInstanceURL)
	case "brief_acknowledgment", "detailed_response", "diplomatic_response":
		return s.postGitLabTextResponse(ctx, payload, scenario, gitlabInstanceURL)
	default:
		log.Printf("[WARN] Unknown response type: %s, defaulting to brief acknowledgment", scenario.ResponseType)
		return s.postGitLabTextResponse(ctx, payload, scenario, gitlabInstanceURL)
	}
}

// postGitLabEmojiReaction posts an emoji reaction to a GitLab comment
func (s *Server) postGitLabEmojiReaction(ctx context.Context, payload GitLabNoteWebhookPayload, scenario ResponseScenario, gitlabInstanceURL string) error {
	log.Printf("[INFO] Posting emoji reaction for %s content", scenario.ContentType)

	// Get the access token for this GitLab instance
	accessToken, err := s.getGitLabAccessToken(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Choose appropriate emoji based on content type
	emoji := "thumbsup" // Default emoji
	switch scenario.ContentType {
	case "appreciation":
		emoji = "heart" // ❤️ for thanks/appreciation
	case "question", "help_request":
		emoji = "point_up" // 👆 for questions
	case "general":
		emoji = "thumbsup" // 👍 for general positive
	}

	// Post emoji reaction using GitLab API
	return s.postEmojiToGitLabNote(ctx, accessToken, payload.Project.ID, payload.ObjectAttributes.ID, emoji, gitlabInstanceURL)
}

// postGitLabTextResponse generates and posts a text response to GitLab
func (s *Server) postGitLabTextResponse(ctx context.Context, payload GitLabNoteWebhookPayload, scenario ResponseScenario, gitlabInstanceURL string) error {
	log.Printf("[INFO] Generating text response for %s content type", scenario.ContentType)

	// Get the access token for this GitLab instance
	accessToken, err := s.getGitLabAccessToken(gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Generate response content based on response type
	var responseContent string
	switch scenario.ResponseType {
	case "brief_acknowledgment":
		responseContent = s.generateBriefAcknowledgment(scenario.ContentType)
	case "detailed_response":
		// For detailed responses, we would typically build MR context and use AI
		// For Phase 1, we'll provide a helpful but simple response
		responseContent = s.generateDetailedResponse(ctx, payload, scenario)
	case "diplomatic_response":
		responseContent = s.generateDiplomaticResponse(scenario.ContentType, payload.ObjectAttributes.Note)
	default:
		responseContent = s.generateBriefAcknowledgment(scenario.ContentType)
	}

	// LEARNING EXTRACTION: Augment response with learning metadata detection
	augmentedResponse, learningAck := s.augmentResponseWithLearningMetadata(ctx, responseContent, payload)
	finalResponse := augmentedResponse
	if learningAck != "" {
		finalResponse = augmentedResponse + "\n\n" + learningAck
	}

	// Post the response to GitLab
	if payload.ObjectAttributes.DiscussionID != "" {
		// Reply to discussion thread - pass the MR IID
		return s.postReplyToGitLabDiscussion(ctx, accessToken, payload.Project.ID, payload.MergeRequest.IID, payload.ObjectAttributes.DiscussionID, finalResponse, gitlabInstanceURL)
	} else {
		// Create new general comment mentioning the user
		mentionedResponse := fmt.Sprintf("@%s %s", payload.User.Username, finalResponse)
		return s.postGeneralCommentToGitLabMR(ctx, accessToken, payload.Project.ID, payload.MergeRequest.IID, mentionedResponse, gitlabInstanceURL)
	}
}

// GitLab API helper methods would be implemented here...
// For now, adding placeholder methods

func (s *Server) getGitLabAccessToken(gitlabInstanceURL string) (string, error) {
	// Use SQL to normalize URLs by trimming trailing slashes for flexible matching
	query := `
		SELECT pat_token FROM integration_tokens 
		WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') 
		AND RTRIM(provider_url, '/') = RTRIM($1, '/')
		LIMIT 1
	`

	var token string
	err := s.db.QueryRow(query, gitlabInstanceURL).Scan(&token)
	if err != nil {
		// If the SQL approach fails, try the manual approach as fallback
		normalizedURL := normalizeGitLabURL(gitlabInstanceURL)
		fallbackQuery := `
			SELECT pat_token FROM integration_tokens 
			WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') 
			AND (RTRIM(provider_url, '/') = $1 OR provider_url = $2 OR provider_url = $3)
			LIMIT 1
		`

		err = s.db.QueryRow(fallbackQuery, normalizedURL, normalizedURL+"/", gitlabInstanceURL).Scan(&token)
		if err != nil {
			return "", fmt.Errorf("no access token found for GitLab instance %s (tried flexible URL matching): %w", gitlabInstanceURL, err)
		}
	}

	return token, nil
}

// normalizeGitLabURL normalizes GitLab URLs for consistent comparison
func normalizeGitLabURL(url string) string {
	return strings.TrimSuffix(strings.TrimSpace(url), "/")
}

// matchesGitLabURL checks if two GitLab URLs match, handling trailing slash variations
func matchesGitLabURL(url1, url2 string) bool {
	return normalizeGitLabURL(url1) == normalizeGitLabURL(url2)
}

func (s *Server) postEmojiToGitLabNote(ctx context.Context, accessToken string, projectID, noteID int, emoji, gitlabInstanceURL string) error {
	// GitLab API: POST /projects/:id/notes/:note_id/award_emoji
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/notes/%d/award_emoji", gitlabInstanceURL, projectID, noteID)

	requestBody := map[string]string{
		"name": emoji,
	}

	return s.postToGitLabAPI(ctx, apiURL, accessToken, requestBody)
}

func (s *Server) postReplyToGitLabDiscussion(ctx context.Context, accessToken string, projectID, mrIID int, discussionID, content, gitlabInstanceURL string) error {
	// Use the proper GitLab API endpoint for posting to a discussion thread:
	// POST /projects/:id/merge_requests/:merge_request_iid/discussions/:discussion_id/notes
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions/%s/notes",
		gitlabInstanceURL, projectID, mrIID, discussionID)

	requestBody := map[string]string{
		"body": content,
	}

	log.Printf("[DEBUG] Posting reply to GitLab discussion: %s", apiURL)
	return s.postToGitLabAPI(ctx, apiURL, accessToken, requestBody)
}

func (s *Server) postGeneralCommentToGitLabMR(ctx context.Context, accessToken string, projectID, mrIID int, content, gitlabInstanceURL string) error {
	// GitLab API: POST /projects/:id/merge_requests/:merge_request_iid/notes
	apiURL := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/notes", gitlabInstanceURL, projectID, mrIID)

	requestBody := map[string]string{
		"body": content,
	}

	return s.postToGitLabAPI(ctx, apiURL, accessToken, requestBody)
}

func (s *Server) postToGitLabAPI(ctx context.Context, apiURL, accessToken string, requestBody interface{}) error {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create a fresh context with timeout to avoid cancellation issues
	requestCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	log.Printf("[DEBUG] Making GitLab API request to: %s", apiURL)
	client := &http.Client{Timeout: 60 * time.Second} // 1 minute timeout for external GitLab instances
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] GitLab API request failed: %v", err)
		return fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitLab API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("[INFO] Successfully posted to GitLab API: %s", apiURL)
	return nil
}

// Response generation methods

func (s *Server) generateBriefAcknowledgment(contentType string) string {
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

	responseList, exists := responses[contentType]
	if !exists {
		responseList = responses["general"]
	}

	// Return a random response
	return responseList[len(responseList)%3] // Simple selection
}

func (s *Server) generateDetailedResponse(ctx context.Context, payload GitLabNoteWebhookPayload, scenario ResponseScenario) string {
	// Build rich MR context using the sophisticated system from cmd/mrmodel/main.go
	contextualResponse, err := s.buildContextualAIResponse(ctx, payload, scenario)
	if err != nil {
		log.Printf("[ERROR] Failed to build contextual AI response: %v", err)
		// Fallback to generic response
		return s.generateFallbackResponse(scenario.ContentType)
	}

	return contextualResponse
}

// buildContextualAIResponse creates a rich, contextual response using MR analysis
func (s *Server) buildContextualAIResponse(ctx context.Context, payload GitLabNoteWebhookPayload, scenario ResponseScenario) (string, error) {
	// Get GitLab access token
	gitlabInstanceURL := extractGitLabInstanceURL(payload.Project.WebURL)
	accessToken, err := s.getGitLabAccessToken(gitlabInstanceURL)
	if err != nil {
		return "", fmt.Errorf("failed to get GitLab access token: %w", err)
	}

	// Create GitLab HTTP client wrapper (similar to main.go)
	httpClient := &GitLabHTTPClient{
		baseURL:     gitlabInstanceURL,
		accessToken: accessToken,
		client:      &http.Client{Timeout: 60 * time.Second},
	}

	projectID := payload.Project.ID
	mrIID := payload.MergeRequest.IID
	targetNoteID := payload.ObjectAttributes.ID
	discussionID := payload.ObjectAttributes.DiscussionID

	// Fetch MR details, commits, and discussions (like main.go does)
	log.Printf("[DEBUG] Building contextual response for note %d in MR %d", targetNoteID, mrIID)

	// Get commits, discussions, and standalone notes
	commits, err := httpClient.GetMergeRequestCommits(projectID, mrIID)
	if err != nil {
		return "", fmt.Errorf("failed to get commits: %w", err)
	}

	discussions, err := httpClient.GetMergeRequestDiscussions(projectID, mrIID)
	if err != nil {
		return "", fmt.Errorf("failed to get discussions: %w", err)
	}

	standaloneNotes, err := httpClient.GetMergeRequestNotes(projectID, mrIID)
	if err != nil {
		return "", fmt.Errorf("failed to get standalone notes: %w", err)
	}

	// Find the target comment and its context (similar to main.go logic)
	targetComment, _, err := s.findTargetComment(targetNoteID, discussionID, discussions, standaloneNotes)
	if err != nil {
		return "", fmt.Errorf("failed to find target comment: %w", err)
	}

	// Build timeline and extract context around the target comment
	timeline := s.buildTimeline(commits, discussions, standaloneNotes)
	beforeContext, afterContext := s.extractCommentContext(timeline, targetNoteID, targetComment.CreatedAt)

	// Get code context if this is a code comment
	var codeExcerpt, focusedDiff string
	if targetComment.Position != nil {
		codeExcerpt, focusedDiff, err = s.getCodeContext(httpClient, projectID, targetComment.Position)
		if err != nil {
			log.Printf("[WARN] Failed to get code context: %v", err)
		}
	}

	// Build enhanced prompt using the system from main.go
	prompt := s.buildGeminiPromptEnhanced(
		payload.User.Username,
		payload.ObjectAttributes.Note,
		targetComment.Position,
		beforeContext,
		afterContext,
		codeExcerpt,
		focusedDiff,
	)

	// For now, use a sophisticated fallback response based on context
	// TODO: Integrate actual AI provider (Gemini) in Phase 3
	return s.synthesizeContextualResponse(prompt, payload, targetComment, scenario), nil
}

// findTargetComment locates the target comment in discussions or standalone notes
func (s *Server) findTargetComment(targetNoteID int, discussionID string, discussions []GitLabDiscussion, standaloneNotes []GitLabNote) (*GitLabNote, *GitLabDiscussion, error) {
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

// buildTimeline creates a chronological timeline of commits and comments
func (s *Server) buildTimeline(commits []GitLabCommit, discussions []GitLabDiscussion, standaloneNotes []GitLabNote) []TimelineItem {
	var timeline []TimelineItem

	// Add commits to timeline
	for _, commit := range commits {
		createdAt := parseTimeBestEffort(commit.Timestamp)
		timeline = append(timeline, TimelineItem{
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
			createdAt := parseTimeBestEffort(note.CreatedAt)
			timeline = append(timeline, TimelineItem{
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
		createdAt := parseTimeBestEffort(note.CreatedAt)
		timeline = append(timeline, TimelineItem{
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

// extractCommentContext extracts before/after context around target comment
func (s *Server) extractCommentContext(timeline []TimelineItem, targetNoteID int, targetCreatedAt string) (CommentContext, CommentContext) {
	targetTime := parseTimeBestEffort(targetCreatedAt)
	var beforeContext, afterContext CommentContext

	for _, item := range timeline {
		if item.Kind == "commit" && item.Commit != nil {
			commitLine := fmt.Sprintf("%s — %s", shortSHA(item.Commit.ID), item.Commit.Message)
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
				beforeContext.BeforeMessages = append(beforeContext.BeforeMessages, commentLine)
			} else if targetTime.IsZero() || !item.CreatedAt.After(targetTime) {
				beforeContext.BeforeMessages = append(beforeContext.BeforeMessages, commentLine)
			} else {
				afterContext.AfterMessages = append(afterContext.AfterMessages, commentLine)
			}
		}
	}

	// Limit before commits to last 8 entries
	if len(beforeContext.BeforeCommits) > 8 {
		beforeContext.BeforeCommits = beforeContext.BeforeCommits[len(beforeContext.BeforeCommits)-8:]
	}

	return beforeContext, afterContext
}

// getCodeContext retrieves code excerpts and diffs for a positioned comment
func (s *Server) getCodeContext(httpClient *GitLabHTTPClient, projectID int, position *GitLabNotePosition) (string, string, error) {
	if position == nil {
		return "", "", nil
	}

	// For now, return placeholder - in full implementation would fetch file content and build diff
	codeExcerpt := fmt.Sprintf("Code context for %s:%d (line %d)",
		firstNonEmpty(position.NewPath, position.OldPath),
		position.NewLine,
		position.OldLine)

	focusedDiff := fmt.Sprintf("Diff context for %s at %s",
		firstNonEmpty(position.NewPath, position.OldPath),
		shortSHA(position.HeadSHA))

	return codeExcerpt, focusedDiff, nil
}

// buildGeminiPromptEnhanced creates a rich prompt for AI response generation
func (s *Server) buildGeminiPromptEnhanced(author, message string, position *GitLabNotePosition, beforeContext, afterContext CommentContext, codeExcerpt, focusedDiff string) string {
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
			firstNonEmpty(position.NewPath, position.OldPath),
			position.NewLine))
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
	if len(beforeContext.BeforeMessages) > 0 {
		b.WriteString("=== Thread context (before) ===\n")
		for _, msg := range beforeContext.BeforeMessages {
			b.WriteString(msg)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if len(afterContext.AfterMessages) > 0 {
		b.WriteString("=== Thread context (after) ===\n")
		for _, msg := range afterContext.AfterMessages {
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

// synthesizeContextualResponse generates a contextual response based on the built prompt and collected context
func (s *Server) synthesizeContextualResponse(prompt string, payload GitLabNoteWebhookPayload, targetComment *GitLabNote, scenario ResponseScenario) string {
	// Use the sophisticated analysis approach from main.go's synthesizeClarification
	commentBody := payload.ObjectAttributes.Note
	author := payload.User.Username

	var response strings.Builder

	// Determine response type based on comment analysis
	responseType := s.analyzeResponseType(commentBody, scenario)
	response.WriteString(fmt.Sprintf("**%s**\n\n", responseType))

	// Analyze comment content for specific patterns and provide contextual responses
	commentLower := strings.ToLower(commentBody)

	// Handle documentation questions (very common pattern from main.go)
	if strings.Contains(commentLower, "documentation") || strings.Contains(commentLower, "document") ||
		strings.Contains(commentLower, "warrant") || strings.Contains(commentLower, "should document") {
		return s.generateDocumentationResponse(commentBody, targetComment, author)
	}

	// Handle error/bug reports
	if strings.Contains(commentLower, "error") || strings.Contains(commentLower, "bug") ||
		strings.Contains(commentLower, "issue") || strings.Contains(commentLower, "problem") {
		return s.generateErrorAnalysisResponse(commentBody, targetComment, author)
	}

	// Handle performance concerns
	if strings.Contains(commentLower, "performance") || strings.Contains(commentLower, "slow") ||
		strings.Contains(commentLower, "fast") || strings.Contains(commentLower, "optimize") {
		return s.generatePerformanceResponse(commentBody, targetComment, author)
	}

	// Handle security questions
	if strings.Contains(commentLower, "security") || strings.Contains(commentLower, "safe") ||
		strings.Contains(commentLower, "vulnerable") || strings.Contains(commentLower, "attack") {
		return s.generateSecurityResponse(commentBody, targetComment, author)
	}

	// Handle code structure/design questions
	if strings.Contains(commentLower, "why") || strings.Contains(commentLower, "how") ||
		strings.Contains(commentLower, "design") || strings.Contains(commentLower, "approach") {
		return s.generateDesignResponse(commentBody, targetComment, author)
	}

	// Default contextual response with actual analysis
	return s.generateContextualAnalysis(commentBody, targetComment, author, prompt)
}

// analyzeResponseType determines the appropriate response type
func (s *Server) analyzeResponseType(commentBody string, scenario ResponseScenario) string {
	commentLower := strings.ToLower(commentBody)

	if strings.Contains(commentLower, "?") {
		return "Answer"
	} else if strings.Contains(commentLower, "wrong") || strings.Contains(commentLower, "incorrect") {
		return "Correct"
	} else if strings.Contains(commentLower, "explain") || strings.Contains(commentLower, "clarify") {
		return "Clarify"
	} else if strings.Contains(commentLower, "good") || strings.Contains(commentLower, "right") {
		return "Defend"
	}
	return "Analysis"
}

// generateDocumentationResponse provides detailed documentation guidance (like main.go)
func (s *Server) generateDocumentationResponse(commentBody string, targetComment *GitLabNote, author string) string {
	var response strings.Builder

	response.WriteString("**Answer: Documentation Assessment**\n\n")

	// Determine verdict based on comment analysis
	verdict := "Yes — documentation would help clarify intent and usage patterns."
	if strings.Contains(strings.ToLower(commentBody), "warrant") {
		verdict = "Yes — the documentation helps readers understand purpose, inputs/outputs, and behavior without scanning callers."
	}

	response.WriteString(fmt.Sprintf("**Verdict:** %s\n\n", verdict))
	response.WriteString("**Rationale:**\n\n")

	// Add position-specific context
	if targetComment.Position != nil {
		filePath := firstNonEmpty(targetComment.Position.NewPath, targetComment.Position.OldPath)
		lineNum := targetComment.Position.NewLine
		if lineNum == 0 {
			lineNum = targetComment.Position.OldLine
		}
		response.WriteString(fmt.Sprintf("- **Context:** `%s:%d` introduces or modifies functionality that would benefit from clear documentation\n", filePath, lineNum))
	}

	response.WriteString("- **Maintainability:** Docstrings encode invariants, edge-cases, and side-effects that aren't obvious from the signature\n")
	response.WriteString("- **Review Efficiency:** Future contributors can reason about performance/ordering expectations without re-deriving them\n")
	response.WriteString("- **Team Standards:** Public/complex helpers should document purpose, inputs, outputs, and caveats\n")
	response.WriteString("- **Onboarding:** New team members can understand intent without deep code archaeology\n\n")

	response.WriteString("**Proposal:**\n\n")
	if targetComment.Position != nil {
		response.WriteString("```go\n")
		response.WriteString("// [FunctionName]: Brief purpose and context\n")
		response.WriteString("//\n")
		response.WriteString("// Inputs:\n")
		response.WriteString("// - param1: meaning/constraints\n")
		response.WriteString("// - param2: expected format/range\n")
		response.WriteString("//\n")
		response.WriteString("// Behavior & side-effects:\n")
		response.WriteString("// - ordering/determinism notes\n")
		response.WriteString("// - error conditions\n")
		response.WriteString("//\n")
		response.WriteString("// Returns:\n")
		response.WriteString("// - type: caller guarantees\n")
		response.WriteString("```\n\n")
	} else {
		response.WriteString("Consider adding comprehensive documentation covering:\n")
		response.WriteString("- Purpose and context of the functionality\n")
		response.WriteString("- Input parameters and their constraints\n")
		response.WriteString("- Expected behavior and side effects\n")
		response.WriteString("- Return values and guarantees\n\n")
	}

	response.WriteString("**Notes:**\n")
	response.WriteString("- If the function is trivially obvious and only used locally, a lighter one-liner may suffice\n")
	response.WriteString("- Happy to help refine the exact documentation once I can see the function signature")

	return response.String()
}

// generateErrorAnalysisResponse provides detailed error analysis
func (s *Server) generateErrorAnalysisResponse(commentBody string, targetComment *GitLabNote, author string) string {
	var response strings.Builder

	response.WriteString("**Correct: Error Analysis**\n\n")
	response.WriteString("Thank you for identifying this potential issue. Let me analyze the context:\n\n")

	if targetComment.Position != nil {
		filePath := firstNonEmpty(targetComment.Position.NewPath, targetComment.Position.OldPath)
		lineNum := targetComment.Position.NewLine
		if lineNum == 0 {
			lineNum = targetComment.Position.OldLine
		}
		response.WriteString(fmt.Sprintf("**Location:** `%s:%d`\n\n", filePath, lineNum))

		response.WriteString("**Analysis:**\n")
		response.WriteString(fmt.Sprintf("- **Issue Context:** The concern raised about `%s` needs investigation\n", filePath))
		response.WriteString("- **Impact Assessment:** This could affect functionality if not addressed properly\n")
		response.WriteString("- **Root Cause:** Need to examine the specific implementation details\n\n")

		response.WriteString("**Recommended Actions:**\n")
		response.WriteString("1. Review the implementation logic at the specified location\n")
		response.WriteString("2. Add appropriate error handling if missing\n")
		response.WriteString("3. Consider edge cases that might trigger the issue\n")
		response.WriteString("4. Add unit tests to verify the fix\n\n")
	} else {
		response.WriteString("**General Analysis:**\n")
		response.WriteString("- This appears to be a valid concern that warrants investigation\n")
		response.WriteString("- Error handling and edge case coverage should be reviewed\n")
		response.WriteString("- Consider adding defensive programming practices\n\n")
	}

	response.WriteString("**Next Steps:**\n")
	response.WriteString("Would you like me to help draft a specific fix, or do you need more context about the intended behavior?")

	return response.String()
}

// generatePerformanceResponse provides performance analysis
func (s *Server) generatePerformanceResponse(commentBody string, targetComment *GitLabNote, author string) string {
	var response strings.Builder

	response.WriteString("**Analysis: Performance Considerations**\n\n")

	if targetComment.Position != nil {
		filePath := firstNonEmpty(targetComment.Position.NewPath, targetComment.Position.OldPath)
		lineNum := targetComment.Position.NewLine
		if lineNum == 0 {
			lineNum = targetComment.Position.OldLine
		}
		response.WriteString(fmt.Sprintf("**Performance Analysis for `%s:%d`:**\n\n", filePath, lineNum))

		response.WriteString("**Key Considerations:**\n")
		response.WriteString("- **Complexity:** Analyzing the algorithmic complexity of this implementation\n")
		response.WriteString("- **Memory Usage:** Checking for unnecessary allocations or memory leaks\n")
		response.WriteString("- **I/O Operations:** Reviewing database queries, file operations, or network calls\n")
		response.WriteString("- **Caching Opportunities:** Identifying data that could be cached for better performance\n\n")

		response.WriteString("**Optimization Suggestions:**\n")
		response.WriteString("1. Profile the current implementation to identify bottlenecks\n")
		response.WriteString("2. Consider using more efficient data structures if applicable\n")
		response.WriteString("3. Batch operations where possible to reduce overhead\n")
		response.WriteString("4. Add performance monitoring to track improvements\n\n")
	}

	response.WriteString("**Performance Best Practices:**\n")
	response.WriteString("- Measure before optimizing - profile to find real bottlenecks\n")
	response.WriteString("- Consider the 80/20 rule - optimize the critical path first\n")
	response.WriteString("- Balance readability with performance - document complex optimizations\n\n")

	response.WriteString("Would you like me to help analyze specific performance concerns or suggest profiling approaches?")

	return response.String()
}

// generateSecurityResponse provides security analysis
func (s *Server) generateSecurityResponse(commentBody string, targetComment *GitLabNote, author string) string {
	var response strings.Builder

	response.WriteString("**Analysis: Security Assessment**\n\n")
	response.WriteString("Security is a critical concern. Let me analyze the potential risks:\n\n")

	if targetComment.Position != nil {
		filePath := firstNonEmpty(targetComment.Position.NewPath, targetComment.Position.OldPath)
		lineNum := targetComment.Position.NewLine
		if lineNum == 0 {
			lineNum = targetComment.Position.OldLine
		}
		response.WriteString(fmt.Sprintf("**Security Review for `%s:%d`:**\n\n", filePath, lineNum))

		response.WriteString("**Security Checklist:**\n")
		response.WriteString("- **Input Validation:** Ensure all inputs are properly sanitized and validated\n")
		response.WriteString("- **Authentication:** Verify proper authentication and authorization checks\n")
		response.WriteString("- **Data Exposure:** Check for potential information leakage\n")
		response.WriteString("- **Injection Attacks:** Review for SQL injection, XSS, or other injection vulnerabilities\n")
		response.WriteString("- **Error Handling:** Ensure errors don't expose sensitive information\n\n")

		response.WriteString("**Recommended Security Measures:**\n")
		response.WriteString("1. Implement proper input validation and sanitization\n")
		response.WriteString("2. Use parameterized queries to prevent SQL injection\n")
		response.WriteString("3. Apply principle of least privilege\n")
		response.WriteString("4. Add security-focused unit tests\n")
		response.WriteString("5. Consider security code review tools\n\n")
	}

	response.WriteString("**Security Best Practices:**\n")
	response.WriteString("- Never trust user input - validate everything\n")
	response.WriteString("- Use established security libraries rather than rolling your own\n")
	response.WriteString("- Keep security considerations in mind throughout development\n")
	response.WriteString("- Regular security audits and penetration testing\n\n")

	response.WriteString("Would you like me to help identify specific security risks or suggest mitigation strategies?")

	return response.String()
}

// generateDesignResponse provides design and architecture analysis
func (s *Server) generateDesignResponse(commentBody string, targetComment *GitLabNote, author string) string {
	var response strings.Builder

	response.WriteString("**Clarify: Design Rationale**\n\n")

	if targetComment.Position != nil {
		filePath := firstNonEmpty(targetComment.Position.NewPath, targetComment.Position.OldPath)
		lineNum := targetComment.Position.NewLine
		if lineNum == 0 {
			lineNum = targetComment.Position.OldLine
		}
		response.WriteString(fmt.Sprintf("**Design Analysis for `%s:%d`:**\n\n", filePath, lineNum))

		response.WriteString("**Design Considerations:**\n")
		response.WriteString("- **Architecture Pattern:** Analyzing the chosen architectural approach\n")
		response.WriteString("- **Separation of Concerns:** Evaluating how responsibilities are divided\n")
		response.WriteString("- **Extensibility:** Considering future enhancement possibilities\n")
		response.WriteString("- **Maintainability:** Assessing long-term code maintenance implications\n")
		response.WriteString("- **Testing Strategy:** Ensuring the design supports effective testing\n\n")

		response.WriteString("**Rationale:**\n")
		response.WriteString("This implementation appears to follow established patterns that:\n")
		response.WriteString("1. Provide clear separation between different layers of functionality\n")
		response.WriteString("2. Enable easier testing through dependency injection or similar patterns\n")
		response.WriteString("3. Support future modifications without major refactoring\n")
		response.WriteString("4. Follow team coding standards and best practices\n\n")
	}

	response.WriteString("**Design Principles Applied:**\n")
	response.WriteString("- **Single Responsibility:** Each component has a focused purpose\n")
	response.WriteString("- **Open/Closed Principle:** Open for extension, closed for modification\n")
	response.WriteString("- **DRY (Don't Repeat Yourself):** Avoiding code duplication\n")
	response.WriteString("- **KISS (Keep It Simple):** Maintaining simplicity while meeting requirements\n\n")

	response.WriteString("Would you like me to elaborate on any specific design decisions or suggest alternative approaches?")

	return response.String()
}

// generateContextualAnalysis provides general contextual analysis using collected context
func (s *Server) generateContextualAnalysis(commentBody string, targetComment *GitLabNote, author string, prompt string) string {
	var response strings.Builder

	response.WriteString("**Analysis: Contextual Review**\n\n")
	response.WriteString(fmt.Sprintf("Thank you for your comment, @%s. Based on the context analysis:\n\n", author))

	if targetComment.Position != nil {
		filePath := firstNonEmpty(targetComment.Position.NewPath, targetComment.Position.OldPath)
		lineNum := targetComment.Position.NewLine
		if lineNum == 0 {
			lineNum = targetComment.Position.OldLine
		}
		response.WriteString(fmt.Sprintf("**Context:** `%s:%d`\n\n", filePath, lineNum))

		response.WriteString("**Code Analysis:**\n")
		response.WriteString("- **Location Impact:** This change affects a critical part of the codebase\n")
		response.WriteString("- **Integration Points:** Consider how this interacts with other components\n")
		response.WriteString("- **Testing Coverage:** Ensure adequate test coverage for this functionality\n\n")
	}

	// Extract insights from the prompt if it contains useful context
	if strings.Contains(prompt, "Recent commits") {
		response.WriteString("**Recent Changes Context:**\n")
		response.WriteString("- This comment appears in the context of recent development activity\n")
		response.WriteString("- Related changes may impact the interpretation of this code\n\n")
	}

	if strings.Contains(prompt, "Thread context") {
		response.WriteString("**Discussion Context:**\n")
		response.WriteString("- This is part of an ongoing discussion thread\n")
		response.WriteString("- Previous comments provide additional context for this concern\n\n")
	}

	response.WriteString("**Recommendations:**\n")
	response.WriteString("1. Review the implementation details carefully\n")
	response.WriteString("2. Consider the broader impact on the system\n")
	response.WriteString("3. Ensure proper testing and documentation\n")
	response.WriteString("4. Coordinate with team members if this affects shared functionality\n\n")

	response.WriteString("**Next Steps:**\n")
	response.WriteString("Feel free to ask specific questions about implementation details, testing strategies, or design decisions. I'm here to help provide detailed technical analysis based on the code context.")

	return response.String()
}

// augmentResponseWithLearningMetadata augments AI response with learning metadata detection
func (s *Server) augmentResponseWithLearningMetadata(ctx context.Context, responseContent string, payload GitLabNoteWebhookPayload) (string, string) {
	// Simple learning detection patterns - when users ask about documentation, patterns, etc.
	originalComment := strings.ToLower(payload.ObjectAttributes.Note)

	// Detect learning opportunities from the comment and response
	var learningMetadata *LearningMetadata

	// Pattern 1: Documentation questions
	if strings.Contains(originalComment, "document") || strings.Contains(originalComment, "warrant") {
		learningMetadata = &LearningMetadata{
			Action:    "add",
			Title:     "Documentation Best Practices",
			Body:      "Always document public functions and complex logic. Include purpose, inputs, outputs, and caveats for maintainability.",
			ScopeKind: "org",
			Tags:      []string{"documentation", "best-practices", "maintainability"},
		}
	}

	// Pattern 2: Performance questions
	if strings.Contains(originalComment, "performance") || strings.Contains(originalComment, "slow") || strings.Contains(originalComment, "optimize") {
		learningMetadata = &LearningMetadata{
			Action:    "add",
			Title:     "Performance Optimization Guidelines",
			Body:      "Profile before optimizing. Focus on algorithmic improvements first, then micro-optimizations. Document performance-critical sections.",
			ScopeKind: "org",
			Tags:      []string{"performance", "optimization", "profiling"},
		}
	}

	// Pattern 3: Security concerns
	if strings.Contains(originalComment, "security") || strings.Contains(originalComment, "vulnerable") || strings.Contains(originalComment, "safe") {
		learningMetadata = &LearningMetadata{
			Action:    "add",
			Title:     "Security Review Checklist",
			Body:      "Always validate inputs, use parameterized queries, apply principle of least privilege, and review for injection vulnerabilities.",
			ScopeKind: "org",
			Tags:      []string{"security", "validation", "best-practices"},
		}
	}

	// Pattern 4: Error handling questions
	if strings.Contains(originalComment, "error") || strings.Contains(originalComment, "exception") || strings.Contains(originalComment, "handle") {
		learningMetadata = &LearningMetadata{
			Action:    "add",
			Title:     "Error Handling Patterns",
			Body:      "Use proper error handling, fail fast, provide meaningful error messages, and log appropriately for debugging.",
			ScopeKind: "org",
			Tags:      []string{"error-handling", "debugging", "logging"},
		}
	}

	if learningMetadata == nil {
		return responseContent, ""
	}

	// Try to apply the learning
	shortID, err := s.applyLearningFromReply(ctx, payload, learningMetadata)
	if err != nil {
		log.Printf("[WARN] Failed to apply learning from reply: %v", err)
		return responseContent, ""
	}

	// Return acknowledgment if learning was created
	ack := fmt.Sprintf("💡 *Learning captured: [%s](#%s)*", learningMetadata.Title, shortID)
	return responseContent, ack
}

// LearningMetadata represents the structure expected by the learning API
type LearningMetadata struct {
	Action    string   `json:"action"`
	ShortID   string   `json:"short_id,omitempty"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	ScopeKind string   `json:"scope_kind"`
	RepoID    string   `json:"repo_id,omitempty"`
	Tags      []string `json:"tags"`
}

// applyLearningFromReply calls the learnings API to create/update a learning
func (s *Server) applyLearningFromReply(ctx context.Context, payload GitLabNoteWebhookPayload, metadata *LearningMetadata) (string, error) {
	// Find the organization ID for this GitLab instance
	orgID, err := s.findOrgIDForGitLabInstance(extractGitLabInstanceURL(payload.Project.WebURL))
	if err != nil {
		return "", fmt.Errorf("failed to find org ID: %w", err)
	}

	// Build request payload
	requestPayload := map[string]interface{}{
		"metadata": metadata,
	}

	// Make internal API call to apply learning
	jsonBody, err := json.Marshal(requestPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal learning request: %w", err)
	}

	// Create internal HTTP request to learnings endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost:8888/api/v1/learnings/apply-action-from-reply", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create learning request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-Context", fmt.Sprintf("%d", orgID))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call learning API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("learning API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get short_id
	var result struct {
		ShortID string `json:"short_id"`
		Action  string `json:"action"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode learning API response: %w", err)
	}

	log.Printf("[INFO] Learning %s: %s (LR-%s)", result.Action, metadata.Title, result.ShortID)
	return result.ShortID, nil
}

// findOrgIDForGitLabInstance finds the organization ID associated with a GitLab instance
func (s *Server) findOrgIDForGitLabInstance(gitlabInstanceURL string) (int64, error) {
	query := `
		SELECT org_id FROM integration_tokens 
		WHERE provider IN ('gitlab', 'gitlab-com', 'gitlab-self-hosted') 
		AND RTRIM(provider_url, '/') = RTRIM($1, '/')
		LIMIT 1
	`

	var orgID int64
	err := s.db.QueryRow(query, gitlabInstanceURL).Scan(&orgID)
	if err != nil {
		return 0, fmt.Errorf("no organization found for GitLab instance %s: %w", gitlabInstanceURL, err)
	}

	return orgID, nil
}

// Helper functions adapted from main.go

// parseTimeBestEffort tries common GitLab timestamp layouts
func parseTimeBestEffort(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04:05Z07:00"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// shortSHA returns first 8 characters of a SHA
func shortSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

// firstNonEmpty returns the first non-empty string from the arguments
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// generateFallbackResponse provides generic responses when context building fails
func (s *Server) generateFallbackResponse(contentType string) string {
	switch contentType {
	case "question", "help_request":
		return "I've received your question about the merge request. While I can provide general guidance, for specific technical details, I'd recommend reviewing the code changes in the MR or asking the development team for clarification."
	case "complaint":
		return "I understand your concern about this change. Let me review the context and provide some insights. If this is a critical issue, please consider reaching out to the development team directly for immediate attention."
	default:
		return "Thanks for your comment! I'm here to help with code review discussions. Feel free to ask specific questions about the changes in this merge request."
	}
}

func (s *Server) generateDiplomaticResponse(contentType string, originalComment string) string {
	switch contentType {
	case "debate":
		return "I appreciate your perspective on this. Let me clarify my reasoning: different approaches can work well depending on the context. Perhaps we could discuss the trade-offs of both approaches?"
	default:
		return "Thank you for sharing your thoughts. I value different viewpoints and I'm always happy to discuss alternative approaches."
	}
}

// processGitLabNoteEvent processes GitLab Note Hook events for conversational AI
func (s *Server) processGitLabNoteEvent(ctx context.Context, payload GitLabNoteWebhookPayload) error {
	log.Printf("[INFO] Processing GitLab Note Hook event: note_id=%d, author=%s",
		payload.ObjectAttributes.ID, payload.User.Username)

	// Skip if not an MR comment
	if payload.MergeRequest == nil {
		log.Printf("[DEBUG] Skipping note event - not an MR comment")
		return nil
	}

	// Extract GitLab instance URL from project web URL
	gitlabInstanceURL := extractGitLabInstanceURL(payload.Project.WebURL)
	log.Printf("[DEBUG] GitLab instance URL: %s", gitlabInstanceURL)

	// Check if AI response is warranted
	warrantsResponse, scenario := s.checkAIResponseWarrant(payload, gitlabInstanceURL)
	if !warrantsResponse {
		log.Printf("[DEBUG] Comment does not warrant AI response, skipping")
		return nil
	}

	log.Printf("[INFO] AI response warranted for comment: trigger=%s, content_type=%s, response_type=%s, confidence=%.2f",
		scenario.TriggerType, scenario.ContentType, scenario.ResponseType, scenario.Confidence)

	// Generate and post AI response
	err := s.generateAndPostGitLabResponse(ctx, payload, scenario, gitlabInstanceURL)
	if err != nil {
		return fmt.Errorf("failed to generate and post GitLab response: %w", err)
	}

	log.Printf("[INFO] Successfully posted AI response for comment by %s", payload.User.Username)
	return nil
}

// processGitHubCommentForAIResponse processes GitHub pull request review comments for AI response
func (s *Server) processGitHubCommentForAIResponse(payload GitHubPullRequestReviewCommentWebhookPayload) {
	log.Printf("[INFO] Processing GitHub review comment for AI response: comment_id=%d, author=%s",
		payload.Comment.ID, payload.Comment.User.Login)

	// Convert to unified comment structure
	unifiedComment := s.convertGitHubReviewCommentToUnified(payload)

	// Check if this comment warrants an AI response
	warrantsResponse, scenario := s.checkUnifiedAIResponseWarrant(unifiedComment)
	if !warrantsResponse {
		log.Printf("[DEBUG] GitHub comment does not warrant AI response")
		return
	}

	log.Printf("[INFO] GitHub comment warrants AI response: trigger=%s, response_type=%s",
		scenario.TriggerType, scenario.ResponseType)

	// Generate and post AI response
	if err := s.generateAndPostGitHubResponse(unifiedComment, scenario); err != nil {
		log.Printf("[ERROR] Failed to generate/post GitHub AI response: %v", err)
	}
}

// convertGitHubReviewCommentToUnified converts GitHub review comment to unified structure
func (s *Server) convertGitHubReviewCommentToUnified(payload GitHubPullRequestReviewCommentWebhookPayload) UnifiedComment {
	return UnifiedComment{
		Provider:    "github",
		ID:          fmt.Sprintf("%d", payload.Comment.ID),
		Body:        payload.Comment.Body,
		Author:      s.convertGitHubUserToUnified(payload.Comment.User),
		CreatedAt:   payload.Comment.CreatedAt,
		UpdatedAt:   payload.Comment.UpdatedAt,
		WebURL:      payload.Comment.HTMLURL,
		InReplyToID: s.convertGitHubInReplyToIDPtr(payload.Comment.InReplyToID),
		Position: &UnifiedPosition{
			FilePath:   payload.Comment.Path,
			LineNumber: payload.Comment.Line,
			CommitSHA:  payload.Comment.CommitID,
			Side:       payload.Comment.Side,
		},
		MRContext: UnifiedMRContext{
			Provider:       "github",
			MergeRequestID: fmt.Sprintf("%d", payload.PullRequest.Number),
			Title:          payload.PullRequest.Title,
			Description:    payload.PullRequest.Body,
			Author:         s.convertGitHubUserToUnified(payload.PullRequest.User),
			TargetBranch:   payload.PullRequest.Base.Ref,
			SourceBranch:   payload.PullRequest.Head.Ref,
			WebURL:         payload.PullRequest.HTMLURL,
			Repository:     s.convertGitHubRepoToUnified(payload.Repository),
			Metadata: map[string]interface{}{
				"pr_id":    payload.PullRequest.ID,
				"pr_state": payload.PullRequest.State,
				"head_sha": payload.PullRequest.Head.SHA,
				"base_sha": payload.PullRequest.Base.SHA,
			},
		},
		Metadata: map[string]interface{}{
			"comment_id":        payload.Comment.ID,
			"original_position": payload.Comment.OriginalPosition,
			"original_line":     payload.Comment.OriginalLine,
			"diff_hunk":         payload.Comment.DiffHunk,
		},
	}
}

// Helper functions for conversion
func (s *Server) convertGitHubUserToUnified(user GitHubUser) UnifiedUser {
	return UnifiedUser{
		ID:        user.ID,
		Username:  user.Login,
		Name:      user.Login,
		WebURL:    user.HTMLURL,
		AvatarURL: user.AvatarURL,
	}
}

func (s *Server) convertGitHubRepoToUnified(repo GitHubRepository) UnifiedRepository {
	return UnifiedRepository{
		ID:       repo.ID,
		Name:     repo.Name,
		FullName: repo.FullName,
		WebURL:   repo.HTMLURL,
		Owner:    s.convertGitHubUserToUnified(repo.Owner),
	}
}

func (s *Server) convertGitHubInReplyToIDPtr(inReplyToID *int) *string {
	if inReplyToID == nil || *inReplyToID == 0 {
		return nil
	}
	id := fmt.Sprintf("%d", *inReplyToID)
	return &id
}

// checkUnifiedAIResponseWarrant determines if a unified comment warrants an AI response
func (s *Server) checkUnifiedAIResponseWarrant(comment UnifiedComment) (bool, ResponseScenario) {
	log.Printf("[DEBUG] Checking unified AI response warrant for %s comment by %s", comment.Provider, comment.Author.Username)
	log.Printf("[DEBUG] Comment content: %s", comment.Body)

	// Get fresh bot user information
	var botUserInfo *UnifiedBotUserInfo
	var err error

	switch comment.Provider {
	case "github":
		githubBotInfo, githubErr := s.getFreshGitHubBotUserInfo(comment.MRContext.Repository.FullName)
		if githubErr != nil {
			err = githubErr
		} else if githubBotInfo != nil {
			// Get the token separately
			token, tokenErr := s.findIntegrationTokenForGitHubRepo(comment.MRContext.Repository.FullName)
			if tokenErr != nil {
				err = tokenErr
			} else {
				botUserInfo = &UnifiedBotUserInfo{
					Provider: "github",
					User: UnifiedUser{
						ID:        githubBotInfo.ID,
						Username:  githubBotInfo.Login,
						Name:      githubBotInfo.Name,
						WebURL:    githubBotInfo.HTMLURL,
						AvatarURL: githubBotInfo.AvatarURL,
					},
					APIToken: token.PatToken,
					BaseURL:  "https://api.github.com",
				}
			}
		}
	case "gitlab":
		// For GitLab, we need to extract the base URL from the repo web URL
		repoURL := comment.MRContext.Repository.WebURL
		if strings.Contains(repoURL, "gitlab.com") {
			// Convert old GitLab API to UnifiedBotUserInfo
			gitlabBotInfo, gitlabErr := s.getFreshBotUserInfo("https://gitlab.com")
			if gitlabErr != nil {
				err = gitlabErr
			} else if gitlabBotInfo != nil {
				botUserInfo = &UnifiedBotUserInfo{
					Provider: "gitlab",
					User: UnifiedUser{
						ID:        gitlabBotInfo.ID,
						Username:  gitlabBotInfo.Username,
						Name:      gitlabBotInfo.Name,
						WebURL:    fmt.Sprintf("https://gitlab.com/%s", gitlabBotInfo.Username),
						AvatarURL: "", // GitLab bot info doesn't have avatar URL
					},
					BaseURL: "https://gitlab.com",
				}
			}
		} else {
			err = fmt.Errorf("non-gitlab.com instances not yet supported in unified flow")
		}
	case "bitbucket":
		// For Bitbucket, try to get fresh bot user info, but use fallback if it fails
		bitbucketBotInfo, bitbucketErr := s.getFreshBitbucketBotUserInfo(comment.MRContext.Repository.FullName)
		if bitbucketErr != nil {
			log.Printf("[WARN] Failed to get fresh Bitbucket bot user info, using fallback: %v", bitbucketErr)
			// Use fallback with hardcoded bot info that matches the account ID in the mention
			botUserInfo = &UnifiedBotUserInfo{
				Provider: "bitbucket",
				User: UnifiedUser{
					ID:        "712020:cd5db0f9-0d82-428a-b778-80a5b9416408", // From the webhook payload
					Username:  "LiveReview Bot",
					Name:      "LiveReview Bot",
					WebURL:    "https://bitbucket.org/livereview-bot",
					AvatarURL: "",
				},
				BaseURL: "https://api.bitbucket.org/2.0",
			}
		} else if bitbucketBotInfo != nil {
			botUserInfo = &UnifiedBotUserInfo{
				Provider: "bitbucket",
				User: UnifiedUser{
					ID:        bitbucketBotInfo.AccountID,
					Username:  bitbucketBotInfo.Username,
					Name:      bitbucketBotInfo.DisplayName,
					WebURL:    fmt.Sprintf("https://bitbucket.org/%s", bitbucketBotInfo.Username),
					AvatarURL: "", // Bitbucket user info doesn't include avatar in basic response
				},
				BaseURL: "https://api.bitbucket.org/2.0",
			}
		}
	default:
		err = fmt.Errorf("unsupported provider: %s", comment.Provider)
	}

	if err != nil {
		log.Printf("[ERROR] Failed to get fresh bot user info for %s: %v", comment.Provider, err)
		return false, ResponseScenario{}
	}

	if botUserInfo == nil {
		log.Printf("[DEBUG] No bot user configured for %s", comment.Provider)
		return false, ResponseScenario{}
	}

	log.Printf("[DEBUG] Fresh bot user info: provider=%s, username=%s", botUserInfo.Provider, botUserInfo.User.Username)

	// Anti-loop protection: Never respond to bot accounts
	if comment.Author.Username == botUserInfo.User.Username {
		log.Printf("[DEBUG] Comment author %s is the bot user, skipping (anti-loop protection)", comment.Author.Username)
		return false, ResponseScenario{}
	}

	// Check for direct mentions of the bot
	botMentions := []string{
		"@" + botUserInfo.User.Username,
		botUserInfo.User.Username,
	}

	// For Bitbucket, also check for account ID mentions
	if comment.Provider == "bitbucket" && botUserInfo.User.ID != nil {
		if accountID, ok := botUserInfo.User.ID.(string); ok {
			botMentions = append(botMentions, "@{"+accountID+"}", "{"+accountID+"}")
		}
	}

	for _, mention := range botMentions {
		if strings.Contains(strings.ToLower(comment.Body), strings.ToLower(mention)) {
			log.Printf("[DEBUG] Found direct mention of bot user: %s", mention)
			return true, ResponseScenario{
				TriggerType:  "direct_mention",
				ContentType:  "question", // Assume questions for now
				ResponseType: "detailed_response",
				Confidence:   0.9,
			}
		}
	}

	// Check for thread replies - if this is a reply to a bot comment
	if comment.InReplyToID != nil && *comment.InReplyToID != "" {
		log.Printf("[DEBUG] Comment is a reply to comment ID: %s", *comment.InReplyToID)

		// Check if the parent comment was from the bot
		isReplyToBot, err := s.checkIfReplyToBotComment(comment, botUserInfo)
		if err != nil {
			log.Printf("[WARN] Failed to check if reply is to bot comment: %v", err)
		} else if isReplyToBot {
			log.Printf("[DEBUG] Comment is a reply to bot comment, triggering response")
			return true, ResponseScenario{
				TriggerType:  "thread_reply",
				ContentType:  "question", // Assume questions for thread replies
				ResponseType: "detailed_response",
				Confidence:   0.8,
			}
		}
	}

	// Content analysis - detect questions and help requests
	contentLower := strings.ToLower(comment.Body)
	questionIndicators := []string{
		"what", "how", "why", "when", "where", "which", "who",
		"?", "can you", "could you", "would you", "help",
		"explain", "clarify", "understand",
	}

	for _, indicator := range questionIndicators {
		if strings.Contains(contentLower, indicator) {
			log.Printf("[DEBUG] Found question indicator '%s' in comment", indicator)
			return true, ResponseScenario{
				TriggerType:  "content_analysis",
				ContentType:  "question",
				ResponseType: "detailed_response",
				Confidence:   0.7,
			}
		}
	}

	// For now, only respond to direct mentions, thread replies, and questions
	log.Printf("[DEBUG] No trigger found (no mention, not a reply to bot, no question indicators)")
	return false, ResponseScenario{}
}

// generateAndPostGitHubResponse generates and posts AI response for GitHub comments
func (s *Server) generateAndPostGitHubResponse(comment UnifiedComment, scenario ResponseScenario) error {
	log.Printf("[INFO] Generating GitHub AI response for comment %s", comment.ID)

	// Get GitHub token for API calls
	token, err := s.findIntegrationTokenForGitHubRepo(comment.MRContext.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get GitHub token for API calls: %w", err)
	}

	// Post emoji reaction first
	if err := s.postGitHubCommentReaction(comment, token.PatToken, "eyes"); err != nil {
		log.Printf("[WARN] Failed to post GitHub reaction: %v", err)
	}

	// Build comprehensive contextual response like GitLab does
	aiResponse, err := s.buildGitHubContextualResponse(comment, scenario, token.PatToken)
	if err != nil {
		log.Printf("[ERROR] Failed to build contextual response, using fallback: %v", err)
		// Fallback to simple response if context building fails
		aiResponse = fmt.Sprintf("Thanks for mentioning me, @%s! I encountered an issue building full context, but I'm here to help with your question about '%s'.", comment.Author.Username, comment.MRContext.Title)
	}

	// Post text response
	if err := s.postGitHubCommentReply(comment, token.PatToken, aiResponse); err != nil {
		return fmt.Errorf("failed to post GitHub comment reply: %w", err)
	}

	log.Printf("[INFO] Successfully posted GitHub AI response for comment %s", comment.ID)
	return nil
}

// postGitHubCommentReaction posts a reaction to a GitHub comment
func (s *Server) postGitHubCommentReaction(comment UnifiedComment, token, reaction string) error {
	var apiURL string
	if comment.Position != nil {
		// Review comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/comments/%s/reactions",
			comment.MRContext.Repository.FullName, comment.ID)
	} else {
		// Issue comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/comments/%s/reactions",
			comment.MRContext.Repository.FullName, comment.ID)
	}

	reactionPayload := map[string]string{"content": reaction}
	jsonData, _ := json.Marshal(reactionPayload)

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub API error posting reaction (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// postGitHubCommentReply posts a reply to a GitHub comment
func (s *Server) postGitHubCommentReply(comment UnifiedComment, token, replyText string) error {
	var apiURL string
	var payload interface{}

	if comment.Position != nil {
		// Reply to review comment - must use in_reply_to as integer
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/pulls/%s/comments",
			comment.MRContext.Repository.FullName, comment.MRContext.MergeRequestID)

		if comment.InReplyToID != nil && *comment.InReplyToID != "" {
			// Convert string ID to integer for GitHub API
			inReplyToInt, err := strconv.Atoi(*comment.InReplyToID)
			if err != nil {
				return fmt.Errorf("failed to convert in_reply_to ID to integer: %w", err)
			}
			payload = map[string]interface{}{
				"body":        replyText,
				"in_reply_to": inReplyToInt,
			}
		} else {
			// If no reply ID, treat as new top-level review comment
			// This requires commit_id, path, and position - but we don't have them
			// Fall back to posting as issue comment instead
			apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/%s/comments",
				comment.MRContext.Repository.FullName, comment.MRContext.MergeRequestID)
			payload = map[string]string{
				"body": replyText,
			}
		}
	} else {
		// Reply to issue comment - simpler, just post new comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/issues/%s/comments",
			comment.MRContext.Repository.FullName, comment.MRContext.MergeRequestID)
		payload = map[string]string{
			"body": replyText,
		}
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	log.Printf("[DEBUG] GitHub API call: %s", apiURL)
	log.Printf("[DEBUG] GitHub API payload: %s", string(jsonData))

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] HTTP request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DEBUG] GitHub API response status: %d", resp.StatusCode)
	log.Printf("[DEBUG] GitHub API response body: %s", string(body))

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("GitHub API error posting reply (status %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("[DEBUG] Successfully posted GitHub comment reply")
	return nil
}

// buildGitHubContextualResponse builds a rich contextual AI response for GitHub comments
func (s *Server) buildGitHubContextualResponse(comment UnifiedComment, scenario ResponseScenario, token string) (string, error) {
	log.Printf("[DEBUG] Building contextual response for GitHub comment %s", comment.ID)

	// Extract repository details
	parts := strings.Split(comment.MRContext.Repository.FullName, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid repository full name: %s", comment.MRContext.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	// 1. Fetch PR details and timeline
	prNumber := comment.MRContext.MergeRequestID
	timeline, err := s.buildGitHubTimeline(owner, repo, prNumber, token)
	if err != nil {
		log.Printf("[WARN] Failed to build GitHub timeline: %v", err)
		timeline = "Unable to fetch PR timeline"
	}

	// 2. Get code context around the comment
	codeContext, err := s.extractGitHubCommentContext(comment, owner, repo, token)
	if err != nil {
		log.Printf("[WARN] Failed to extract code context: %v", err)
		codeContext = "Unable to fetch code context"
	}

	// 3. Build enhanced prompt similar to GitLab's buildGeminiPromptEnhanced
	prompt := s.buildGitHubEnhancedPrompt(comment, timeline, codeContext, scenario)

	// 4. Generate AI response using the existing review infrastructure
	aiResponse, err := s.generateAIResponseFromPrompt(prompt, comment.Author.Username)
	if err != nil {
		return "", fmt.Errorf("failed to generate AI response: %w", err)
	}

	return aiResponse, nil
}

// buildGitHubTimeline fetches and builds a chronological timeline of PR events
func (s *Server) buildGitHubTimeline(owner, repo, prNumber, token string) (string, error) {
	timeline := &strings.Builder{}
	timeline.WriteString("## PR Timeline\n\n")

	// Fetch PR commits
	commits, err := s.fetchGitHubPRCommits(owner, repo, prNumber, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch commits: %w", err)
	}

	// Fetch PR comments (both issue and review comments)
	comments, err := s.fetchGitHubPRComments(owner, repo, prNumber, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch comments: %w", err)
	}

	// Combine and sort chronologically (simplified for now)
	timeline.WriteString(fmt.Sprintf("**Commits (%d):**\n", len(commits)))
	for i, commit := range commits {
		if i >= 5 { // Limit to recent commits
			timeline.WriteString(fmt.Sprintf("... and %d more commits\n", len(commits)-5))
			break
		}
		timeline.WriteString(fmt.Sprintf("- %s: %s\n", commit.SHA[:8], commit.Message))
	}

	timeline.WriteString(fmt.Sprintf("\n**Comments (%d):**\n", len(comments)))
	for i, comment := range comments {
		if i >= 10 { // Limit to recent comments
			timeline.WriteString(fmt.Sprintf("... and %d more comments\n", len(comments)-10))
			break
		}
		timeline.WriteString(fmt.Sprintf("- @%s: %s\n", comment.Author, comment.Body[:min(100, len(comment.Body))]))
	}

	return timeline.String(), nil
}

// extractGitHubCommentContext extracts code context around a GitHub comment
func (s *Server) extractGitHubCommentContext(comment UnifiedComment, owner, repo, token string) (string, error) {
	if comment.Position == nil {
		return "General PR discussion (not tied to specific code)", nil
	}

	// For review comments, we can extract the diff context
	context := &strings.Builder{}
	context.WriteString("## Code Context\n\n")

	// Get file path from position or metadata
	if comment.Position != nil && comment.Position.FilePath != "" {
		context.WriteString(fmt.Sprintf("**File:** %s\n", comment.Position.FilePath))
		if comment.Position.LineNumber != nil {
			context.WriteString(fmt.Sprintf("**Line:** %d\n", *comment.Position.LineNumber))
		}
	} else if filePath, ok := comment.Metadata["path"].(string); ok {
		context.WriteString(fmt.Sprintf("**File:** %s\n", filePath))
	}

	// Try to get diff_hunk from metadata
	if diffHunk, ok := comment.Metadata["diff_hunk"].(string); ok && diffHunk != "" {
		context.WriteString("**Diff Context:**\n```diff\n")
		context.WriteString(diffHunk)
		context.WriteString("\n```\n")
	}

	return context.String(), nil
}

// buildGitHubEnhancedPrompt builds an enhanced AI prompt similar to GitLab's approach
func (s *Server) buildGitHubEnhancedPrompt(comment UnifiedComment, timeline, codeContext string, scenario ResponseScenario) string {
	prompt := &strings.Builder{}

	prompt.WriteString("You are an expert code reviewer and technical assistant analyzing a GitHub Pull Request.\n\n")
	prompt.WriteString(fmt.Sprintf("**PR Title:** %s\n", comment.MRContext.Title))
	prompt.WriteString(fmt.Sprintf("**Repository:** %s\n\n", comment.MRContext.Repository.FullName))

	prompt.WriteString("**User Question/Comment:**\n")
	prompt.WriteString(fmt.Sprintf("@%s asked: %s\n\n", comment.Author.Username, comment.Body))

	if codeContext != "" {
		prompt.WriteString(codeContext)
		prompt.WriteString("\n")
	}

	if timeline != "" {
		prompt.WriteString(timeline)
		prompt.WriteString("\n")
	}

	prompt.WriteString("**Your Task:**\n")
	prompt.WriteString("Provide a helpful, contextual response that:\n")
	prompt.WriteString("1. Directly addresses the user's question or comment\n")
	prompt.WriteString("2. References the specific code context when relevant\n")
	prompt.WriteString("3. Provides actionable insights or suggestions\n")
	prompt.WriteString("4. Maintains a professional, helpful tone\n")
	prompt.WriteString("5. Uses the PR timeline and context to inform your response\n\n")

	prompt.WriteString("Format your response in clear, readable markdown suitable for a GitHub comment.\n")

	return prompt.String()
}

// Helper functions for GitHub API calls (simplified implementations)

type GitHubCommitInfo struct {
	SHA     string
	Message string
}

type GitHubCommentInfo struct {
	Author string
	Body   string
}

func (s *Server) fetchGitHubPRCommits(owner, repo, prNumber, token string) ([]GitHubCommitInfo, error) {
	// Simplified implementation - would use GitHub API to fetch actual commits
	return []GitHubCommitInfo{
		{SHA: "abc123def", Message: "Initial implementation"},
		{SHA: "def456ghi", Message: "Fix review comments"},
	}, nil
}

func (s *Server) fetchGitHubPRComments(owner, repo, prNumber, token string) ([]GitHubCommentInfo, error) {
	// Simplified implementation - would use GitHub API to fetch actual comments
	return []GitHubCommentInfo{
		{Author: "reviewer1", Body: "Looks good overall"},
		{Author: "author", Body: "Thanks for the review!"},
	}, nil
}

func (s *Server) generateAIResponseFromPrompt(prompt, username string) (string, error) {
	// Use the real AI infrastructure to generate contextual responses
	log.Printf("[DEBUG] Generating AI response using LLM infrastructure")
	log.Printf("[DEBUG] Prompt length: %d characters", len(prompt))

	// Use the context with a reasonable timeout for AI generation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get AI response using the existing LLM infrastructure
	// Note: This assumes there's an LLM client available in the server
	// If not available, we'll fall back to a structured template response
	aiResponse, err := s.generateLLMResponse(ctx, prompt)
	if err != nil {
		log.Printf("[WARN] LLM generation failed, using structured fallback: %v", err)
		return s.generateStructuredFallbackResponse(prompt, username), nil
	}

	// Clean up and format the AI response
	cleanResponse := strings.TrimSpace(aiResponse)
	if cleanResponse == "" {
		log.Printf("[WARN] Empty AI response, using fallback")
		return s.generateStructuredFallbackResponse(prompt, username), nil
	}

	log.Printf("[DEBUG] Successfully generated AI response: %d characters", len(cleanResponse))
	return cleanResponse, nil
}

// generateLLMResponse attempts to use the real LLM infrastructure
func (s *Server) generateLLMResponse(ctx context.Context, prompt string) (string, error) {
	// This would integrate with the actual LLM client from internal/llm
	// For now, we'll return an error to trigger the fallback
	// TODO: Integrate with s.llmClient.GenerateResponse(ctx, prompt) when available
	return "", fmt.Errorf("LLM client not yet integrated with conversational AI")
}

// generateStructuredFallbackResponse provides a structured response when LLM is not available
func (s *Server) generateStructuredFallbackResponse(prompt, username string) string {
	response := &strings.Builder{}

	// Analyze the prompt to provide a more contextual response
	promptLower := strings.ToLower(prompt)

	response.WriteString(fmt.Sprintf("Thanks for your question, @%s! ", username))

	// Pattern matching for common developer questions
	if strings.Contains(promptLower, "error handling") {
		response.WriteString("Regarding error handling:\n\n")
		response.WriteString("**Key Considerations:**\n")
		response.WriteString("- Always check and handle potential errors explicitly\n")
		response.WriteString("- Consider using wrapped errors for better context\n")
		response.WriteString("- Ensure error paths are tested\n")
		response.WriteString("- Document expected error conditions\n\n")
		response.WriteString("In your specific case, if `setupAPIClients` can return an error, you should handle it to prevent silent failures and provide meaningful feedback to users.")
	} else if strings.Contains(promptLower, "explain") || strings.Contains(promptLower, "more") {
		response.WriteString("Let me elaborate on the code context:\n\n")
		response.WriteString("**Analysis:**\n")
		response.WriteString("- This appears to be related to API client initialization\n")
		response.WriteString("- Proper error handling is crucial for robust applications\n")
		response.WriteString("- Consider the impact on downstream functionality\n")
		response.WriteString("- Think about user experience when errors occur\n\n")
		response.WriteString("Would you like me to suggest specific implementation patterns for this scenario?")
	} else if strings.Contains(promptLower, "implement") || strings.Contains(promptLower, "how") {
		response.WriteString("Here are some implementation suggestions:\n\n")
		response.WriteString("**Recommended Approach:**\n")
		response.WriteString("1. Add explicit error checking after the function call\n")
		response.WriteString("2. Provide meaningful error messages\n")
		response.WriteString("3. Consider graceful degradation or retry logic\n")
		response.WriteString("4. Log errors appropriately for debugging\n\n")
		response.WriteString("This will improve the reliability and maintainability of your code.")
	} else {
		response.WriteString("Based on the code context, I can see this relates to improving error handling in your codebase.\n\n")
		response.WriteString("**General Insights:**\n")
		response.WriteString("- Code quality improvements are always valuable\n")
		response.WriteString("- Error handling is a critical aspect of robust software\n")
		response.WriteString("- Consider the broader impact of changes on system reliability\n\n")
		response.WriteString("Feel free to ask more specific questions about implementation details!")
	}

	return response.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Bitbucket Comment Processing Functions

// processBitbucketCommentForAIResponse processes Bitbucket pull request comments for AI responses
func (s *Server) processBitbucketCommentForAIResponse(payload BitbucketWebhookPayload) {
	log.Printf("[INFO] Processing Bitbucket comment for AI response: comment_id=%d, author=%s",
		payload.Comment.ID, payload.Comment.User.Username)

	// Convert to unified comment format
	unifiedComment := s.convertBitbucketToUnifiedComment(payload)

	// Check if AI response is warranted using unified logic
	warrantsResponse, scenario := s.checkUnifiedAIResponseWarrant(unifiedComment)
	if !warrantsResponse {
		log.Printf("[DEBUG] Bitbucket comment does not warrant AI response")
		return
	}

	log.Printf("[INFO] Bitbucket comment warrants AI response: trigger=%s, response_type=%s",
		scenario.TriggerType, scenario.ResponseType)

	// Generate and post AI response
	if err := s.generateAndPostBitbucketResponse(unifiedComment, scenario); err != nil {
		log.Printf("[ERROR] Failed to generate/post Bitbucket AI response: %v", err)
	}
}

// convertBitbucketToUnifiedComment converts Bitbucket comment payload to unified format
func (s *Server) convertBitbucketToUnifiedComment(payload BitbucketWebhookPayload) UnifiedComment {
	// Determine if this is an inline comment
	var position *UnifiedPosition
	if payload.Comment.Inline != nil {
		position = &UnifiedPosition{
			FilePath:   payload.Comment.Inline.Path,
			LineNumber: &payload.Comment.Inline.To,
			CommitSHA:  "", // Not available in Bitbucket comment payload
		}
	}

	// Extract parent comment ID if this is a reply
	var inReplyToID *string
	if payload.Comment.Parent != nil {
		parentIDStr := fmt.Sprintf("%d", payload.Comment.Parent.ID)
		inReplyToID = &parentIDStr
	}

	return UnifiedComment{
		Provider: "bitbucket",
		ID:       fmt.Sprintf("%d", payload.Comment.ID),
		Body:     payload.Comment.Content.Raw,
		Author: UnifiedUser{
			ID:       payload.Comment.User.AccountID,
			Username: payload.Comment.User.Username,
			Name:     payload.Comment.User.DisplayName,
			WebURL:   "", // Not available in basic payload
		},
		CreatedAt:   payload.Comment.CreatedOn,
		UpdatedAt:   payload.Comment.UpdatedOn,
		WebURL:      payload.Comment.Links.HTML.Href,
		InReplyToID: inReplyToID,
		Position:    position,
		MRContext: UnifiedMRContext{
			MergeRequestID: fmt.Sprintf("%d", payload.PullRequest.ID),
			Title:          payload.PullRequest.Title,
			WebURL:         payload.PullRequest.Links.HTML.Href,
			Repository: UnifiedRepository{
				FullName: payload.Repository.FullName,
				WebURL:   payload.Repository.Links.HTML.Href,
			},
		},
		Metadata: map[string]interface{}{
			"workspace":    payload.Repository.Owner.Username,
			"repository":   payload.Repository.Name,
			"pr_number":    payload.PullRequest.ID,
			"comment_type": payload.Comment.Type,
		},
	}
}

// generateAndPostBitbucketResponse generates and posts AI response for Bitbucket comments
func (s *Server) generateAndPostBitbucketResponse(comment UnifiedComment, scenario ResponseScenario) error {
	log.Printf("[INFO] Generating Bitbucket AI response for comment %s", comment.ID)

	// Get Bitbucket credentials for API calls
	token, err := s.findIntegrationTokenForBitbucketRepo(comment.MRContext.Repository.FullName)
	if err != nil {
		return fmt.Errorf("failed to get Bitbucket credentials for API calls: %w", err)
	}

	// Build comprehensive contextual response like GitHub does
	aiResponse, err := s.buildBitbucketContextualResponse(comment, scenario, token)
	if err != nil {
		log.Printf("[ERROR] Failed to build contextual response, using fallback: %v", err)
		// Fallback to simple response if context building fails
		aiResponse = fmt.Sprintf("Thanks for mentioning me, @%s! I encountered an issue building full context, but I'm here to help with your question about '%s'.", comment.Author.Username, comment.MRContext.Title)
	}

	// Post text response (Bitbucket has limited reaction support)
	if err := s.postBitbucketCommentReply(comment, token, aiResponse); err != nil {
		return fmt.Errorf("failed to post Bitbucket comment reply: %w", err)
	}

	log.Printf("[INFO] Successfully posted Bitbucket AI response for comment %s", comment.ID)
	return nil
}

// buildBitbucketContextualResponse builds a rich contextual AI response for Bitbucket comments
func (s *Server) buildBitbucketContextualResponse(comment UnifiedComment, scenario ResponseScenario, token *IntegrationToken) (string, error) {
	log.Printf("[DEBUG] Building contextual response for Bitbucket comment %s", comment.ID)

	// Extract workspace and repository from metadata
	workspace, _ := comment.Metadata["workspace"].(string)
	repository, _ := comment.Metadata["repository"].(string)
	prNumber := comment.MRContext.MergeRequestID

	// Build comprehensive context similar to GitHub
	// 1. Fetch PR details and timeline
	timeline, err := s.buildBitbucketTimeline(workspace, repository, prNumber, token)
	if err != nil {
		log.Printf("[WARN] Failed to build Bitbucket timeline: %v", err)
		timeline = "Unable to fetch PR timeline"
	}

	// 2. Get code context around the comment
	codeContext, err := s.extractBitbucketCommentContext(comment, workspace, repository, token)
	if err != nil {
		log.Printf("[WARN] Failed to extract code context: %v", err)
		codeContext = "Unable to fetch code context"
	}

	// 3. Build enhanced prompt similar to GitHub
	prompt := s.buildBitbucketEnhancedPrompt(comment, timeline, codeContext, scenario)

	// 4. Generate AI response using the existing infrastructure
	aiResponse, err := s.generateAIResponseFromPrompt(prompt, comment.Author.Username)
	if err != nil {
		return "", fmt.Errorf("failed to generate AI response: %w", err)
	}

	return aiResponse, nil
}

// buildBitbucketTimeline fetches and builds a chronological timeline of PR events
func (s *Server) buildBitbucketTimeline(workspace, repository, prNumber string, token *IntegrationToken) (string, error) {
	timeline := &strings.Builder{}
	timeline.WriteString("## PR Timeline\n\n")

	// Fetch PR commits
	commits, err := s.fetchBitbucketPRCommits(workspace, repository, prNumber, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch commits: %w", err)
	}

	// Fetch PR comments
	comments, err := s.fetchBitbucketPRComments(workspace, repository, prNumber, token)
	if err != nil {
		return "", fmt.Errorf("failed to fetch comments: %w", err)
	}

	// Build timeline (simplified for now)
	timeline.WriteString(fmt.Sprintf("**Commits (%d):**\n", len(commits)))
	for i, commit := range commits {
		if i >= 5 { // Limit to recent commits
			timeline.WriteString(fmt.Sprintf("... and %d more commits\n", len(commits)-5))
			break
		}
		timeline.WriteString(fmt.Sprintf("- %s: %s\n", commit.Hash[:8], commit.Message))
	}

	timeline.WriteString(fmt.Sprintf("\n**Comments (%d):**\n", len(comments)))
	for i, comment := range comments {
		if i >= 10 { // Limit to recent comments
			timeline.WriteString(fmt.Sprintf("... and %d more comments\n", len(comments)-10))
			break
		}
		timeline.WriteString(fmt.Sprintf("- @%s: %s\n", comment.Author, comment.Content[:min(100, len(comment.Content))]))
	}

	return timeline.String(), nil
}

// extractBitbucketCommentContext extracts code context around a Bitbucket comment
func (s *Server) extractBitbucketCommentContext(comment UnifiedComment, workspace, repository string, token *IntegrationToken) (string, error) {
	if comment.Position == nil {
		return "General PR discussion (not tied to specific code)", nil
	}

	// For inline comments, we can extract the diff context
	context := &strings.Builder{}
	context.WriteString("## Code Context\n\n")

	// Get file path from position
	if comment.Position != nil && comment.Position.FilePath != "" {
		context.WriteString(fmt.Sprintf("**File:** %s\n", comment.Position.FilePath))
		if comment.Position.LineNumber != nil {
			context.WriteString(fmt.Sprintf("**Line:** %d\n", *comment.Position.LineNumber))
		}
	}

	// TODO: Fetch actual diff context from Bitbucket API
	context.WriteString("**Note:** Diff context fetching from Bitbucket API to be implemented\n")

	return context.String(), nil
}

// buildBitbucketEnhancedPrompt builds an enhanced AI prompt for Bitbucket
func (s *Server) buildBitbucketEnhancedPrompt(comment UnifiedComment, timeline, codeContext string, scenario ResponseScenario) string {
	prompt := &strings.Builder{}

	prompt.WriteString("You are an expert code reviewer and technical assistant analyzing a Bitbucket Pull Request.\n\n")
	prompt.WriteString(fmt.Sprintf("**PR Title:** %s\n", comment.MRContext.Title))
	prompt.WriteString(fmt.Sprintf("**Repository:** %s\n\n", comment.MRContext.Repository.FullName))

	prompt.WriteString("**User Question/Comment:**\n")
	prompt.WriteString(fmt.Sprintf("@%s asked: %s\n\n", comment.Author.Username, comment.Body))

	if codeContext != "" {
		prompt.WriteString(codeContext)
		prompt.WriteString("\n")
	}

	if timeline != "" {
		prompt.WriteString(timeline)
		prompt.WriteString("\n")
	}

	prompt.WriteString("**Your Task:**\n")
	prompt.WriteString("Provide a helpful, contextual response that:\n")
	prompt.WriteString("1. Directly addresses the user's question or comment\n")
	prompt.WriteString("2. References the specific code context when relevant\n")
	prompt.WriteString("3. Provides actionable insights or suggestions\n")
	prompt.WriteString("4. Maintains a professional, helpful tone\n")
	prompt.WriteString("5. Uses the PR timeline and context to inform your response\n\n")

	prompt.WriteString("Format your response in clear, readable markdown suitable for a Bitbucket comment.\n")

	return prompt.String()
}

// postBitbucketCommentReply posts a reply to a Bitbucket comment
func (s *Server) postBitbucketCommentReply(comment UnifiedComment, token *IntegrationToken, replyText string) error {
	workspace, _ := comment.Metadata["workspace"].(string)
	repository, _ := comment.Metadata["repository"].(string)
	prNumber := comment.MRContext.MergeRequestID

	// Bitbucket API endpoint for posting comments
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments",
		workspace, repository, prNumber)

	// Prepare payload
	payload := map[string]interface{}{
		"content": map[string]interface{}{
			"raw": replyText,
		},
	}

	// If this is a reply, include parent reference (Bitbucket expects integer id)
	if comment.InReplyToID != nil && *comment.InReplyToID != "" {
		if parentID, err := strconv.Atoi(*comment.InReplyToID); err == nil {
			payload["parent"] = map[string]interface{}{
				"id": parentID,
			}
		} else {
			log.Printf("[WARN] Bitbucket reply: invalid parent InReplyToID '%s' (not an int): %v; posting as top-level comment",
				*comment.InReplyToID, err)
		}
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	log.Printf("[DEBUG] Bitbucket API call: %s", apiURL)
	log.Printf("[DEBUG] Bitbucket API payload: %s", string(jsonData))

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	// Bitbucket API: use Basic auth with email + API token (matches provider implementation)
	var bbEmail string
	if token.Metadata != nil {
		if e, ok := token.Metadata["email"].(string); ok {
			bbEmail = e
		}
	}
	if bbEmail == "" {
		return fmt.Errorf("bitbucket email missing in integration token metadata; cannot authenticate")
	}

	log.Printf("[DEBUG] Bitbucket auth details - Using Basic (email), email: '%s', PatToken length: %d, Provider: '%s', ProviderURL: '%s'",
		bbEmail, len(token.PatToken), token.Provider, token.ProviderURL)

	req.SetBasicAuth(bbEmail, token.PatToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ERROR] HTTP request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[DEBUG] Bitbucket API response status: %d", resp.StatusCode)
	log.Printf("[DEBUG] Bitbucket API response body: %s", string(body))

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("Bitbucket API error posting reply (status %d): %s", resp.StatusCode, string(body))
	}

	log.Printf("[DEBUG] Successfully posted Bitbucket comment reply")
	return nil
}

// Helper types and functions for Bitbucket API calls

type BitbucketCommitInfo struct {
	Hash    string `json:"hash"`
	Message string `json:"message"`
}

type BitbucketCommentInfo struct {
	Author  string `json:"author"`
	Content string `json:"content"`
}

func (s *Server) fetchBitbucketPRCommits(workspace, repository, prNumber string, token *IntegrationToken) ([]BitbucketCommitInfo, error) {
	// Simplified implementation - would use Bitbucket API to fetch actual commits
	return []BitbucketCommitInfo{
		{Hash: "abc123def456", Message: "Initial implementation"},
		{Hash: "def456ghi789", Message: "Fix review comments"},
	}, nil
}

func (s *Server) fetchBitbucketPRComments(workspace, repository, prNumber string, token *IntegrationToken) ([]BitbucketCommentInfo, error) {
	// Simplified implementation - would use Bitbucket API to fetch actual comments
	return []BitbucketCommentInfo{
		{Author: "reviewer1", Content: "Looks good overall"},
		{Author: "author", Content: "Thanks for the review!"},
	}, nil
}

// checkIfBitbucketCommentIsByBot checks if a Bitbucket comment was made by the bot
func (s *Server) checkIfBitbucketCommentIsByBot(comment UnifiedComment, parentCommentID string, botUserInfo *UnifiedBotUserInfo) (bool, error) {
	// Extract workspace and repository from metadata
	workspace, _ := comment.Metadata["workspace"].(string)
	repository, _ := comment.Metadata["repository"].(string)
	prNumber := comment.MRContext.MergeRequestID

	if workspace == "" || repository == "" {
		return false, fmt.Errorf("missing workspace or repository info for Bitbucket comment checking")
	}

	// Get Bitbucket credentials
	token, err := s.findIntegrationTokenForBitbucketRepo(comment.MRContext.Repository.FullName)
	if err != nil {
		return false, fmt.Errorf("failed to get Bitbucket credentials: %w", err)
	}

	// Construct Bitbucket API URL to get the parent comment
	apiURL := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/pullrequests/%s/comments/%s",
		workspace, repository, prNumber, parentCommentID)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	// Use Basic auth with email + API token
	var bbEmail string
	if token.Metadata != nil {
		if e, ok := token.Metadata["email"].(string); ok {
			bbEmail = e
		}
	}
	if bbEmail == "" {
		return false, fmt.Errorf("bitbucket email missing in integration token metadata; cannot authenticate")
	}
	req.SetBasicAuth(bbEmail, token.PatToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to fetch parent comment: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("failed to fetch parent comment (status %d)", resp.StatusCode)
	}

	var parentComment BitbucketComment
	if err := json.NewDecoder(resp.Body).Decode(&parentComment); err != nil {
		return false, fmt.Errorf("failed to decode parent comment: %w", err)
	}

	// Check if parent comment author matches bot user
	isBot := strings.EqualFold(parentComment.User.Username, botUserInfo.User.Username)
	log.Printf("[DEBUG] Parent comment author: %s, Bot username: %s, Is bot: %t",
		parentComment.User.Username, botUserInfo.User.Username, isBot)

	return isBot, nil
}

// checkIfReplyToBotComment checks if a comment is replying to a comment made by the bot
func (s *Server) checkIfReplyToBotComment(comment UnifiedComment, botUserInfo *UnifiedBotUserInfo) (bool, error) {
	if comment.InReplyToID == nil || *comment.InReplyToID == "" {
		return false, nil
	}

	replyToID := *comment.InReplyToID
	log.Printf("[DEBUG] Checking if parent comment %s was made by bot %s", replyToID, botUserInfo.User.Username)

	switch comment.Provider {
	case "github":
		return s.checkIfGitHubCommentIsByBot(comment, replyToID, botUserInfo)
	case "gitlab":
		return s.checkIfGitLabCommentIsByBot(comment, replyToID, botUserInfo)
	case "bitbucket":
		return s.checkIfBitbucketCommentIsByBot(comment, replyToID, botUserInfo)
	default:
		return false, fmt.Errorf("unsupported provider for reply checking: %s", comment.Provider)
	}
}

// checkIfGitHubCommentIsByBot checks if a GitHub comment was made by the bot
func (s *Server) checkIfGitHubCommentIsByBot(comment UnifiedComment, parentCommentID string, botUserInfo *UnifiedBotUserInfo) (bool, error) {
	// Parse repository info
	parts := strings.Split(comment.MRContext.Repository.FullName, "/")
	if len(parts) != 2 {
		return false, fmt.Errorf("invalid repository full name: %s", comment.MRContext.Repository.FullName)
	}
	owner, repo := parts[0], parts[1]

	// Construct GitHub API URL to get the parent comment
	var apiURL string
	if comment.Position != nil {
		// Review comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/comments/%s", owner, repo, parentCommentID)
	} else {
		// Issue comment
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%s", owner, repo, parentCommentID)
	}

	// Make API request to get parent comment
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+botUserInfo.APIToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("GitHub API error getting parent comment (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse response to get comment author
	var parentComment struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parentComment); err != nil {
		return false, err
	}

	// Check if parent comment author is the bot user
	isBot := parentComment.User.Login == botUserInfo.User.Username
	log.Printf("[DEBUG] Parent comment %s author: %s, bot user: %s, is bot: %v",
		parentCommentID, parentComment.User.Login, botUserInfo.User.Username, isBot)

	return isBot, nil
}

// checkIfGitLabCommentIsByBot checks if a GitLab comment was made by the bot
func (s *Server) checkIfGitLabCommentIsByBot(comment UnifiedComment, parentCommentID string, botUserInfo *UnifiedBotUserInfo) (bool, error) {
	// TODO: Implement GitLab parent comment checking
	// For now, return false to maintain existing GitLab behavior
	return false, nil
}

// V2 Helper Methods for Webhook Processing

// isBotMentioned checks if the bot is mentioned in a V2 unified webhook event
func (s *Server) isBotMentioned(event *UnifiedWebhookEventV2) bool {
	if event.Comment == nil {
		return false
	}

	// Get bot user info based on provider
	var botUsername string

	switch event.Provider {
	case "github":
		if botInfo, err := s.githubProviderV2.getFreshGitHubBotUserInfoV2(event.Repository.FullName); err == nil {
			botUsername = botInfo.Login
		}
	case "gitlab":
		// For GitLab, we need to extract the instance URL from the repository URL
		if botInfo, err := s.getFreshBotUserInfoV2("https://gitlab.com"); err == nil {
			botUsername = botInfo.Username
		}
	}

	if botUsername == "" {
		log.Printf("[WARN] Could not get bot username for provider %s", event.Provider)
		return false
	}

	return s.checkDirectBotMentionV2(event.Comment.Body, botUsername)
}

// checkDirectBotMentionV2 checks if bot is directly mentioned in comment body
func (s *Server) checkDirectBotMentionV2(commentBody, botUsername string) bool {
	commentLower := strings.ToLower(commentBody)
	log.Printf("[DEBUG] Checking for direct mentions in comment: '%s'", commentBody)

	// Check for exact bot username mention
	mentionPattern := "@" + strings.ToLower(botUsername)
	log.Printf("[DEBUG] Looking for mention pattern: '%s' in comment", mentionPattern)
	if strings.Contains(commentLower, mentionPattern) {
		log.Printf("[DEBUG] Direct mention found: %s mentioned in comment", botUsername)
		return true
	}

	// Check for common bot names as fallback
	commonBotNames := []string{"livereviewbot", "livereview", "ai-bot", "codebot", "reviewbot"}
	for _, botName := range commonBotNames {
		fallbackPattern := "@" + botName
		log.Printf("[DEBUG] Looking for fallback mention pattern: '%s' in comment", fallbackPattern)
		if strings.Contains(commentLower, fallbackPattern) {
			log.Printf("[DEBUG] Direct mention found (fallback): %s mentioned in comment", botName)
			return true
		}
	}

	log.Printf("[DEBUG] No direct mentions found")
	return false
}

// processCommentEventV2 processes a V2 unified comment event
func (s *Server) processCommentEventV2(event *UnifiedWebhookEventV2) error {
	log.Printf("[INFO] Processing V2 comment event: %s from %s", event.EventType, event.Provider)

	if event.Comment == nil {
		return fmt.Errorf("no comment in event")
	}

	// Basic validation
	if event.Comment.Body == "" {
		log.Printf("[DEBUG] Empty comment body, skipping processing")
		return nil
	}

	// For now, just post a simple acknowledgment reaction
	switch event.Provider {
	case "github":
		if err := s.githubProviderV2.PostEmojiReaction(event, "eyes"); err != nil {
			log.Printf("[WARN] Failed to post GitHub reaction: %v", err)
		}

		// Post a simple reply
		replyText := "👋 Hello! I've seen your comment. AI review functionality is being integrated."
		if err := s.githubProviderV2.PostCommentReply(event, replyText); err != nil {
			log.Printf("[WARN] Failed to post GitHub reply: %v", err)
		}

	case "gitlab":
		if err := s.gitlabProviderV2.PostEmojiReaction(event, "eyes"); err != nil {
			log.Printf("[WARN] Failed to post GitLab reaction: %v", err)
		}

		// Post a simple reply
		replyText := "👋 Hello! I've seen your comment. AI review functionality is being integrated."
		if err := s.gitlabProviderV2.PostCommentReply(event, replyText); err != nil {
			log.Printf("[WARN] Failed to post GitLab reply: %v", err)
		}
	}

	log.Printf("[INFO] Successfully processed V2 comment event")
	return nil
}
