package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/config"
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
	log.Printf("[DEBUG] TriggerReview: Starting review request handling")

	// Check if the user is authenticated
	password := c.Request().Header.Get("X-Admin-Password")
	log.Printf("[DEBUG] TriggerReview: Authentication header present: %v", password != "")
	if password == "" {
		log.Printf("[DEBUG] TriggerReview: Authentication failed - no password provided")
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
		})
	}

	// Get the stored hashed password
	var hashedPassword string
	log.Printf("[DEBUG] TriggerReview: Querying database for admin password")
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		log.Printf("[DEBUG] TriggerReview: Database error retrieving password: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check authentication: " + err.Error(),
		})
	}
	log.Printf("[DEBUG] TriggerReview: Retrieved hashed password from database")

	// Verify the provided password
	log.Printf("[DEBUG] TriggerReview: Verifying password")
	if !comparePasswords(hashedPassword, password) {
		log.Printf("[DEBUG] TriggerReview: Authentication failed - invalid password")
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Invalid authentication",
		})
	}
	log.Printf("[DEBUG] TriggerReview: Password verification successful")

	// Parse request body
	req := new(TriggerReviewRequest)
	log.Printf("[DEBUG] TriggerReview: Parsing request body")
	if err := c.Bind(req); err != nil {
		log.Printf("[DEBUG] TriggerReview: Failed to parse request body: %v", err)
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}
	log.Printf("[DEBUG] TriggerReview: Request body parsed successfully")

	// Validate URL
	log.Printf("[DEBUG] TriggerReview: Validating URL: %s", req.URL)
	if req.URL == "" {
		log.Printf("[DEBUG] TriggerReview: URL validation failed - URL is empty")
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "URL is required",
		})
	}

	// Parse the URL to ensure it's valid
	log.Printf("[DEBUG] TriggerReview: Parsing URL")
	parsedURL, err := url.Parse(req.URL)
	if err != nil {
		log.Printf("[DEBUG] TriggerReview: URL parsing failed: %v", err)
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid URL format: " + err.Error(),
		})
	}
	log.Printf("[DEBUG] TriggerReview: URL parsed successfully. Scheme: %s, Host: %s, Path: %s",
		parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

	// Extract base URL for connector validation
	baseURL := parsedURL.Scheme + "://" + parsedURL.Host
	log.Printf("[DEBUG] TriggerReview: Extracted base URL: %s", baseURL)

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

	// Debug the query we'll use to find the token
	sqlQuery := `
		SELECT id, provider, access_token, refresh_token, expires_at, provider_app_id, client_secret, provider_url
		FROM integration_tokens
		WHERE provider_url LIKE $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	queryPattern := "%" + baseURL + "%"
	log.Printf("[DEBUG] TriggerReview: Querying database for integration token with base URL: %s, pattern: %s", baseURL, queryPattern)
	log.Printf("[DEBUG] TriggerReview: SQL Query: %s", sqlQuery)

	err = s.db.QueryRow(sqlQuery, queryPattern).Scan(&integrationID, &provider, &accessToken, &refreshToken, &expiresAt, &clientID, &clientSecret, &providerURL)

	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] TriggerReview: No integration token found for URL: %s", baseURL)

		// Let's check what tokens we do have in the database
		var count int
		countErr := s.db.QueryRow("SELECT COUNT(*) FROM integration_tokens").Scan(&count)
		if countErr == nil {
			log.Printf("[DEBUG] TriggerReview: Found %d total integration tokens in database", count)

			// Query for all tokens to see what provider_urls we have
			rows, listErr := s.db.Query("SELECT id, provider, provider_url FROM integration_tokens")
			if listErr == nil {
				defer rows.Close()
				log.Printf("[DEBUG] TriggerReview: Available tokens:")
				for rows.Next() {
					var id int64
					var prov, url string
					if scanErr := rows.Scan(&id, &prov, &url); scanErr == nil {
						log.Printf("[DEBUG] TriggerReview:   - ID: %d, Provider: %s, URL: %s", id, prov, url)
					}
				}
			}
		}

		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "URL is not from a connected Git provider. Please connect the provider first.",
		})
	} else if err != nil {
		log.Printf("[DEBUG] TriggerReview: Database error retrieving integration token: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Log token details (safely)
	maskToken := func(token string) string {
		if len(token) <= 10 {
			return "[HIDDEN]"
		}
		return token[:5] + "..." + token[len(token)-5:]
	}

	log.Printf("[DEBUG] TriggerReview: Found integration token. ID: %d, Provider: %s, Provider URL: %s",
		integrationID, provider, providerURL)
	log.Printf("[DEBUG] TriggerReview: Token details - Access Token: %s, Has Refresh Token: %v, Expires At: %v",
		maskToken(accessToken), refreshToken != "", expiresAt) // Currently, we only support GitLab
	if provider != "gitlab" {
		log.Printf("[DEBUG] TriggerReview: Unsupported provider: %s", provider)
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Unsupported provider type: %s. Currently, only GitLab is supported.", provider),
		})
	}
	log.Printf("[DEBUG] TriggerReview: Provider is supported: %s", provider)

	// Check if the token is about to expire (within 5 minutes) and refresh if needed
	tokenNeedsRefresh := false
	if expiresAt.Valid {
		log.Printf("[DEBUG] TriggerReview: Token expires at: %v", expiresAt.Time)
		timeUntilExpiry := time.Until(expiresAt.Time)
		log.Printf("[DEBUG] TriggerReview: Time until expiry: %v", timeUntilExpiry)
		tokenNeedsRefresh = time.Now().Add(5 * time.Minute).After(expiresAt.Time)
		log.Printf("[DEBUG] TriggerReview: Token needs refresh: %v", tokenNeedsRefresh)
	} else {
		log.Printf("[DEBUG] TriggerReview: Token has no expiry time")
	}

	// Force token refresh for debugging if needed
	forceRefresh := true // Set to true to force a token refresh for debugging
	if forceRefresh {
		log.Printf("[DEBUG] TriggerReview: Forcing token refresh for debugging")
		tokenNeedsRefresh = true
	}

	if tokenNeedsRefresh {
		log.Printf("[DEBUG] TriggerReview: Token for integration ID %d is about to expire, refreshing...", integrationID)
		log.Printf("[DEBUG] TriggerReview: Refresh parameters - Client ID: %s, Client Secret length: %d, Provider URL: %s",
			clientID, len(clientSecret), providerURL)

		// Refresh the token
		result := s.RefreshGitLabToken(integrationID, clientID, clientSecret)

		if result.Error != nil {
			log.Printf("[DEBUG] TriggerReview: Failed to refresh token: %v", result.Error)
			log.Printf("[DEBUG] TriggerReview: Error details: %s", result.Error.Error())

			// Try using the existing token anyway as fallback
			log.Printf("[DEBUG] TriggerReview: Will try to continue with existing token despite refresh failure")
		} else {
			// Update tokens with refreshed values
			log.Printf("[DEBUG] TriggerReview: Token refreshed successfully")
			accessToken = result.TokenData.AccessToken
			refreshToken = result.TokenData.RefreshToken

			// Safely log part of the new token
			maskedToken := "unknown"
			if len(accessToken) > 10 {
				maskedToken = accessToken[:5] + "..." + accessToken[len(accessToken)-5:]
			}
			log.Printf("[DEBUG] TriggerReview: Updated access token: %s (type: %s, length: %d)",
				maskedToken,
				strings.Split(accessToken, "-")[0],
				len(accessToken))
		}
	}

	// Create a reviewID
	reviewID := fmt.Sprintf("review-%d", time.Now().Unix())
	log.Printf("[DEBUG] TriggerReview: Generated review ID: %s", reviewID)

	// Trigger the review process in a goroutine
	log.Printf("[DEBUG] TriggerReview: Starting review process in background goroutine")
	go func() {
		log.Printf("[DEBUG] TriggerReview-goroutine: Starting background review process for URL: %s, ReviewID: %s", req.URL, reviewID)

		// Create GitLab provider
		log.Printf("[DEBUG] TriggerReview-goroutine: Creating GitLab provider with URL: %s", providerURL)
		log.Printf("[DEBUG] TriggerReview-goroutine: Using GitLab token: %s", maskToken(accessToken))

		// Check if the token from database looks valid
		if len(accessToken) < 20 || !strings.HasPrefix(accessToken, "glpat-") {
			log.Printf("[DEBUG] TriggerReview-goroutine: Token from database doesn't look like a valid GitLab PAT, trying config file...")

			// Try to load from config file as fallback
			cfg, err := config.LoadConfig("")
			if err == nil && cfg != nil {
				if gitlabConfig, ok := cfg.Providers["gitlab"]; ok {
					if configToken, ok := gitlabConfig["token"].(string); ok && configToken != "" {
						log.Printf("[DEBUG] TriggerReview-goroutine: Found token in config file: %s", maskToken(configToken))
						log.Printf("[DEBUG] TriggerReview-goroutine: Falling back to config file token")
						accessToken = configToken
					}
				}
			}
		}

		gitlabProvider, err := gitlab.New(gitlab.GitLabConfig{
			URL:   providerURL,
			Token: accessToken,
		})
		if err != nil {
			log.Printf("[DEBUG] TriggerReview-goroutine: Error creating GitLab provider: %v", err)
			return
		}
		log.Printf("[DEBUG] TriggerReview-goroutine: GitLab provider created successfully")

		// Create AI provider (Gemini as default)
		log.Printf("[DEBUG] TriggerReview-goroutine: Creating AI provider (Gemini)")
		aiProvider, err := gemini.New(gemini.GeminiConfig{
			APIKey: "AIzaSyDEaJ5eRAn4PLeCI5-kKDjgZMrxTbx00NA",
			// APIKey:      geminiAPIKey,
			Model:       "gemini-2.5-flash",
			Temperature: 0.4,
		})
		if err != nil {
			log.Printf("[DEBUG] TriggerReview-goroutine: Error creating AI provider: %v", err)
			return
		}
		log.Printf("[DEBUG] TriggerReview-goroutine: AI provider created successfully with model: gemini-pro, temperature: 0.4")

		// Create a context with timeout
		log.Printf("[DEBUG] TriggerReview-goroutine: Creating context with 10-minute timeout")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Run the review process
		log.Printf("[DEBUG] TriggerReview-goroutine: Starting review process execution for URL: %s", req.URL)

		// Get MR details
		log.Printf("[DEBUG] TriggerReview-goroutine: Fetching merge request details for URL: %s", req.URL)
		mrDetails, err := gitlabProvider.GetMergeRequestDetails(ctx, req.URL)
		if err != nil {
			log.Printf("[DEBUG] TriggerReview-goroutine: Failed to get merge request details: %v", err)
			return
		}
		log.Printf("[DEBUG] TriggerReview-goroutine: Retrieved MR details successfully. ID: %s, Project ID: %s, Title: %s, State: %s, Created: %s",
			mrDetails.ID, mrDetails.ProjectID, mrDetails.Title, mrDetails.State, mrDetails.CreatedAt)

		// Get MR changes
		log.Printf("[DEBUG] TriggerReview-goroutine: Fetching merge request changes for MR ID: %s", mrDetails.ID)
		changes, err := gitlabProvider.GetMergeRequestChanges(ctx, mrDetails.ID)
		if err != nil {
			log.Printf("[DEBUG] TriggerReview-goroutine: Failed to get code changes: %v", err)
			return
		}
		log.Printf("[DEBUG] TriggerReview-goroutine: Retrieved %d changed files", len(changes))

		// Log details about each changed file
		for i, change := range changes {
			changeType := "modified"
			if change.IsNew {
				changeType = "added"
			} else if change.IsDeleted {
				changeType = "deleted"
			} else if change.IsRenamed {
				changeType = "renamed"
			}

			log.Printf("[DEBUG] TriggerReview-goroutine: Change #%d - Path: %s, Type: %s, Hunks: %d, FileType: %s",
				i+1, change.FilePath, changeType, len(change.Hunks), change.FileType)
		}

		// Review code
		log.Printf("[DEBUG] TriggerReview-goroutine: Sending code to AI for review, total files: %d", len(changes))
		result, err := aiProvider.ReviewCode(ctx, changes)
		if err != nil {
			log.Printf("[DEBUG] TriggerReview-goroutine: Failed to review code: %v", err)
			return
		}
		log.Printf("[DEBUG] TriggerReview-goroutine: AI Review completed successfully with %d comments", len(result.Comments))
		log.Printf("[DEBUG] TriggerReview-goroutine: Summary length: %d characters", len(result.Summary))

		// Post comments
		// Post the summary as a general comment
		log.Printf("[DEBUG] TriggerReview-goroutine: Creating summary comment")
		summaryComment := &models.ReviewComment{
			FilePath: "", // Empty for MR-level comment
			Line:     0,  // 0 for MR-level comment
			Content:  fmt.Sprintf("# AI Review Summary\n\n%s", result.Summary),
			Severity: models.SeverityInfo,
			Category: "summary",
		}

		log.Printf("[DEBUG] TriggerReview-goroutine: Posting summary comment to MR ID: %s", mrDetails.ID)
		if err := gitlabProvider.PostComment(ctx, mrDetails.ID, summaryComment); err != nil {
			log.Printf("[DEBUG] TriggerReview-goroutine: Failed to post summary comment: %v", err)
			return
		}
		log.Printf("[DEBUG] TriggerReview-goroutine: Posted summary comment successfully")

		// Post specific comments
		if len(result.Comments) > 0 {
			log.Printf("[DEBUG] TriggerReview-goroutine: Posting %d individual comments to merge request...", len(result.Comments))

			// Log details about each comment
			for i, comment := range result.Comments {
				log.Printf("[DEBUG] TriggerReview-goroutine: Comment #%d - File: %s, Line: %d, Severity: %s, Category: %s, Length: %d chars",
					i+1, comment.FilePath, comment.Line, comment.Severity, comment.Category, len(comment.Content))
			}

			err = gitlabProvider.PostComments(ctx, mrDetails.ID, result.Comments)
			if err != nil {
				log.Printf("[DEBUG] TriggerReview-goroutine: Failed to post comments: %v", err)
				return
			}
			log.Printf("[DEBUG] TriggerReview-goroutine: Successfully posted all %d comments", len(result.Comments))
		} else {
			log.Printf("[DEBUG] TriggerReview-goroutine: No individual comments to post")
		}

		log.Printf("[DEBUG] TriggerReview-goroutine: Review process completed successfully for URL: %s, ReviewID: %s", req.URL, reviewID)
	}()

	// Return success response immediately
	log.Printf("[DEBUG] TriggerReview: Returning success response with reviewID: %s", reviewID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully. You will receive a notification when it's complete.",
		URL:      req.URL,
		ReviewID: reviewID,
	})
}
