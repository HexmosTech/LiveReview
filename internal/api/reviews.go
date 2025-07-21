package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
)

// TriggerReviewRequest represents the request to trigger a new code review
type TriggerReviewRequest struct {
	URL string `json:"url"`
}

// TriggerReviewResponse represents the response from triggering a code review
type TriggerReviewResponse struct {
	Message  string `json:"message"`
	URL      string `json:"url"`
	ReviewID string `json:"reviewId"`
}

// TriggerReview handles the request to trigger a code review from a URL
func (s *Server) TriggerReview(c echo.Context) error {
	// Check if the user is authenticated
	password := c.Request().Header.Get("X-Admin-Password")
	if password == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
		})
	}

	// Get the stored hashed password
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check authentication: " + err.Error(),
		})
	}

	// Verify the provided password
	if !comparePasswords(hashedPassword, password) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Invalid authentication",
		})
	}

	// Parse request body
	req := new(TriggerReviewRequest)
	if err := c.Bind(req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}

	// Validate URL
	if req.URL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "URL is required",
		})
	}

	// Parse the URL to ensure it's valid
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid URL format: " + err.Error(),
		})
	}

	// Extract base URL for connector validation
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host

	// Query the database to find the correct integration token
	var (
		integrationID int64
		provider      string
		accessToken   string
		refreshToken  string
		expiresAt     sql.NullTime
		clientID      string
		clientSecret  string
		providerURL   string
	)

	err = s.db.QueryRow(`
		SELECT id, provider, access_token, refresh_token, expires_at, provider_app_id, client_secret, provider_url
		FROM integration_tokens
		WHERE provider_url LIKE $1
		ORDER BY created_at DESC
		LIMIT 1
	`, "%"+baseURL+"%").Scan(&integrationID, &provider, &accessToken, &refreshToken, &expiresAt, &clientID, &clientSecret, &providerURL)

	if err == sql.ErrNoRows {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "URL is not from a connected Git provider. Please connect the provider first.",
		})
	} else if err != nil {
		log.Printf("Database error: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Currently, we only support GitLab
	if provider != "gitlab" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Unsupported provider type: %s. Currently, only GitLab is supported.", provider),
		})
	}

	// Check if the token is about to expire (within 5 minutes) and refresh if needed
	if expiresAt.Valid && time.Now().Add(5*time.Minute).After(expiresAt.Time) {
		log.Printf("Token for integration ID %d is about to expire, refreshing...", integrationID)

		// Refresh the token
		result := s.RefreshGitLabToken(integrationID, clientID, clientSecret)

		if result.Error != nil {
			log.Printf("Failed to refresh token: %s", result.Error)
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "Failed to refresh authentication token: " + result.Error.Error(),
			})
		}

		// Update tokens with refreshed values
		accessToken = result.TokenData.AccessToken
		refreshToken = result.TokenData.RefreshToken
	}

	// Create a reviewID
	reviewID := fmt.Sprintf("review-%d", time.Now().Unix())

	// Trigger the review process in a goroutine
	go func() {
		// Create GitLab provider
		gitlabProvider, err := gitlab.New(gitlab.GitLabConfig{
			URL:   providerURL,
			Token: accessToken,
		})
		if err != nil {
			log.Printf("Error creating GitLab provider: %v", err)
			return
		}

		// Create AI provider (Gemini as default)
		// Normally this would come from configuration, but for now we'll hardcode Gemini
		geminiAPIKey := "" // This should come from your configuration
		aiProvider, err := gemini.New(gemini.GeminiConfig{
			APIKey:      geminiAPIKey,
			Model:       "gemini-pro",
			Temperature: 0.4,
		})
		if err != nil {
			log.Printf("Error creating AI provider: %v", err)
			return
		}

		// Create a context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Run the review process
		// This is a simplified version - you'll need to adapt runReviewProcess for your API
		log.Printf("Starting review process for URL: %s", req.URL)

		// Get MR details
		mrDetails, err := gitlabProvider.GetMergeRequestDetails(ctx, req.URL)
		if err != nil {
			log.Printf("Failed to get merge request details: %v", err)
			return
		}

		log.Printf("Got MR details: ID=%s, Title=%s", mrDetails.ID, mrDetails.Title)

		// Get MR changes
		changes, err := gitlabProvider.GetMergeRequestChanges(ctx, mrDetails.ID)
		if err != nil {
			log.Printf("Failed to get code changes: %v", err)
			return
		}

		log.Printf("Got %d changed files", len(changes))

		// Review code
		result, err := aiProvider.ReviewCode(ctx, changes)
		if err != nil {
			log.Printf("Failed to review code: %v", err)
			return
		}

		log.Printf("AI Review completed successfully with %d comments", len(result.Comments))

		// Post comments
		// Post the summary as a general comment
		summaryComment := &models.ReviewComment{
			FilePath: "", // Empty for MR-level comment
			Line:     0,  // 0 for MR-level comment
			Content:  fmt.Sprintf("# AI Review Summary\n\n%s", result.Summary),
			Severity: models.SeverityInfo,
			Category: "summary",
		}

		if err := gitlabProvider.PostComment(ctx, mrDetails.ID, summaryComment); err != nil {
			log.Printf("Failed to post summary comment: %v", err)
			return
		}

		log.Printf("Posted summary comment to merge request")

		// Post specific comments
		if len(result.Comments) > 0 {
			log.Printf("Posting %d individual comments to merge request...", len(result.Comments))
			err = gitlabProvider.PostComments(ctx, mrDetails.ID, result.Comments)
			if err != nil {
				log.Printf("Failed to post comments: %v", err)
				return
			}
			log.Printf("Successfully posted all comments")
		}

		log.Printf("Review process completed for URL: %s", req.URL)
	}()

	// Return success response immediately
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully. You will receive a notification when it's complete.",
		URL:      req.URL,
		ReviewID: reviewID,
	})
}
