package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
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
	// Parse the webhook payload
	var payload GitLabWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse GitLab webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid webhook payload",
		})
	}

	// Get the event kind from headers
	eventKind := c.Request().Header.Get("X-Gitlab-Event")
	log.Printf("[INFO] Received GitLab webhook: event_kind=%s, event_type=%s", eventKind, payload.EventType)

	// Process reviewer change events
	if strings.ToLower(eventKind) == "merge request hook" && payload.EventType == "merge_request" {
		reviewerChangeInfo := s.processReviewerChange(payload, eventKind)
		if reviewerChangeInfo != nil {
			log.Printf("[INFO] Detected livereview reviewer change in MR %d", reviewerChangeInfo.MergeRequest.ID)

			// If livereview users are assigned as reviewers, trigger a review
			if reviewerChangeInfo.IsBotAssigned && len(reviewerChangeInfo.CurrentBotReviewers) > 0 {
				go s.triggerReviewForMR(reviewerChangeInfo)
			}
		}
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

			// Track AI comments - if comments were posted, track them
			if result.CommentsCount > 0 {
				// Use a simple tracking approach since we don't have individual comment details
				commentContent := map[string]interface{}{
					"summary": result.Summary,
					"count":   result.CommentsCount,
					"type":    "webhook_review",
				}
				TrackAICommentFromURL(s.db, changeInfo.MergeRequest.URL, "review_summary", commentContent, nil, nil)
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
		changeInfo.MergeRequest.ID, request.ReviewID)
}

// findIntegrationTokenForProject finds the integration token for a project namespace
func (s *Server) findIntegrationTokenForProject(projectNamespace string) (*IntegrationToken, error) {
	// Look up the webhook registry to find which integration token is associated with this project
	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var token IntegrationToken
	err := s.db.QueryRow(query, projectNamespace).Scan(
		&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken,
	)
	if err != nil {
		// If no specific webhook registry entry, try to find any integration token for the same provider
		// This is a fallback for cases where webhook might not be properly registered yet
		query = `
			SELECT id, provider, provider_url, pat_token
			FROM integration_tokens
			WHERE provider = 'gitlab'
			ORDER BY created_at DESC
			LIMIT 1
		`
		err = s.db.QueryRow(query).Scan(
			&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken,
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
	EventType           string              `json:"event_type"`
	Action              string              `json:"action"`
	UpdatedAt           string              `json:"updated_at"`
	BotUsers            []GitHubBotUserInfo `json:"bot_users"`
	CurrentBotReviewers []GitHubUser        `json:"current_bot_reviewers"`
	IsBotAssigned       bool                `json:"is_bot_assigned"`
	IsBotRemoved        bool                `json:"is_bot_removed"`
	RequestedReviewer   GitHubUser          `json:"requested_reviewer"`
	ChangedBy           GitHubUser          `json:"changed_by"`
	PullRequest         GitHubPullRequest   `json:"pull_request"`
	Repository          GitHubRepository    `json:"repository"`
}

// GitHubBotUserInfo represents information about a bot user in GitHub reviewer changes
type GitHubBotUserInfo struct {
	Type string     `json:"type"` // "requested" or "removed"
	User GitHubUser `json:"user"`
}

// GitHubWebhookHandler handles incoming GitHub webhook events
func (s *Server) GitHubWebhookHandler(c echo.Context) error {
	// Parse the webhook payload
	var payload GitHubWebhookPayload
	if err := c.Bind(&payload); err != nil {
		log.Printf("[ERROR] Failed to parse GitHub webhook payload: %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid webhook payload",
		})
	}

	// Get the event type from headers
	eventType := c.Request().Header.Get("X-GitHub-Event")
	log.Printf("[INFO] Received GitHub webhook: event_type=%s, action=%s", eventType, payload.Action)

	// Process reviewer change events for pull requests
	if strings.ToLower(eventType) == "pull_request" &&
		(payload.Action == "review_requested" || payload.Action == "review_request_removed") {

		reviewerChangeInfo := s.processGitHubReviewerChange(payload, eventType)
		if reviewerChangeInfo != nil {
			log.Printf("[INFO] Detected livereview reviewer change in GitHub PR #%d", reviewerChangeInfo.PullRequest.Number)

			// If livereview users are assigned as reviewers, trigger a review
			if reviewerChangeInfo.IsBotAssigned && len(reviewerChangeInfo.CurrentBotReviewers) > 0 {
				go s.triggerReviewForGitHubPR(reviewerChangeInfo)
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "received",
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
	var botUsers []GitHubBotUserInfo
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
				botUsers = append(botUsers, GitHubBotUserInfo{Type: "requested", User: payload.RequestedReviewer})
				isBotAssigned = true
				log.Printf("[DEBUG] Livereview user assigned as reviewer - THIS WILL TRIGGER ACTIONS!")
			} else if payload.Action == "review_request_removed" {
				botUsers = append(botUsers, GitHubBotUserInfo{Type: "removed", User: payload.RequestedReviewer})
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

			// Track AI comments - if comments were posted, track them
			if result.CommentsCount > 0 {
				// Use a simple tracking approach since we don't have individual comment details
				commentContent := map[string]interface{}{
					"summary": result.Summary,
					"count":   result.CommentsCount,
					"type":    "webhook_review",
				}
				TrackAICommentFromURL(s.db, changeInfo.PullRequest.HTMLURL, "review_summary", commentContent, nil, nil)
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
		SELECT it.id, it.provider, it.provider_url, it.pat_token
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var token IntegrationToken
	err := s.db.QueryRow(query, repoFullName).Scan(
		&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken,
	)
	if err != nil {
		// If no specific webhook registry entry, try to find any integration token for GitHub
		// This is a fallback for cases where webhook might not be properly registered yet
		query = `
			SELECT id, provider, provider_url, pat_token
			FROM integration_tokens
			WHERE provider LIKE 'github%'
			ORDER BY created_at DESC
			LIMIT 1
		`
		err = s.db.QueryRow(query).Scan(
			&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken,
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

			// Track AI comments - if comments were posted, track them
			if result.CommentsCount > 0 {
				// Use a simple tracking approach since we don't have individual comment details
				commentContent := map[string]interface{}{
					"summary": result.Summary,
					"count":   result.CommentsCount,
					"type":    "webhook_review",
				}
				TrackAICommentFromURL(s.db, changeInfo.PullRequest.Links.HTML.Href, "review_summary", commentContent, nil, nil)
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
		SELECT it.id, it.provider, it.provider_url, it.pat_token
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	var token IntegrationToken
	err := s.db.QueryRow(query, repoFullName).Scan(
		&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken,
	)
	if err != nil {
		// If no specific webhook registry entry, try to find any integration token for Bitbucket
		// This is a fallback for cases where webhook might not be properly registered yet
		query = `
			SELECT id, provider, provider_url, pat_token
			FROM integration_tokens
			WHERE provider LIKE 'bitbucket%'
			ORDER BY created_at DESC
			LIMIT 1
		`
		err = s.db.QueryRow(query).Scan(
			&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken,
		)
		if err != nil {
			return nil, fmt.Errorf("no integration token found for Bitbucket repository %s: %w", repoFullName, err)
		}
	}

	return &token, nil
}
