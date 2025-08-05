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

	// Generate a unique review ID
	reviewID := fmt.Sprintf("webhook-review-%d", time.Now().Unix())

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
		URL:      changeInfo.MergeRequest.URL,
		ReviewID: reviewID,
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

	// Process the review asynchronously
	reviewService.ProcessReviewAsync(ctx, request, func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] Review completed successfully for MR %d (ReviewID: %s)",
				changeInfo.MergeRequest.ID, result.ReviewID)
			log.Printf("[INFO] Posted %d comments", result.CommentsCount)
		} else {
			log.Printf("[ERROR] Review failed for MR %d (ReviewID: %s): %v",
				changeInfo.MergeRequest.ID, result.ReviewID, result.Error)
		}
	})

	log.Printf("[INFO] Review process started asynchronously for MR %d (ReviewID: %s)",
		changeInfo.MergeRequest.ID, reviewID)
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
		return "gemini-1.5-flash"
	case "openai":
		return "gpt-4"
	case "anthropic":
		return "claude-3-sonnet-20240229"
	default:
		return "gemini-1.5-flash" // Default fallback
	}
}
