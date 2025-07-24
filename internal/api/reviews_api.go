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
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/review"
)

// validateAndParseURL validates the input URL string, parses it, and returns the parsed URL and base URL.
func validateAndParseURL(rawURL string) (*url.URL, string, error) {
	log.Printf("[DEBUG] validateAndParseURL: Validating URL: %s", rawURL)
	if rawURL == "" {
		log.Printf("[DEBUG] validateAndParseURL: URL validation failed - URL is empty")
		return nil, "", fmt.Errorf("URL is required")
	}

	log.Printf("[DEBUG] validateAndParseURL: Parsing URL")
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		log.Printf("[DEBUG] validateAndParseURL: URL parsing failed: %v", err)
		return nil, "", fmt.Errorf("invalid URL format: %v", err)
	}
	log.Printf("[DEBUG] validateAndParseURL: URL parsed successfully. Scheme: %s, Host: %s, Path: %s",
		parsedURL.Scheme, parsedURL.Host, parsedURL.Path)

	baseURL := parsedURL.Scheme + "://" + parsedURL.Host
	log.Printf("[DEBUG] validateAndParseURL: Extracted base URL: %s", baseURL)
	return parsedURL, baseURL, nil
}

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

// IntegrationToken holds the token data retrieved from database
type IntegrationToken struct {
	ID           int64
	Provider     string
	AccessToken  string
	RefreshToken string
	ExpiresAt    sql.NullTime
	ClientID     string
	ClientSecret string
	ProviderURL  string
}

// maskToken safely masks a token for logging
func maskToken(token string) string {
	if len(token) <= 10 {
		return "[HIDDEN]"
	}
	return token[:5] + "..." + token[len(token)-5:]
}

// findIntegrationToken queries the database to find an integration token for the given base URL
func (s *Server) findIntegrationToken(baseURL string) (*IntegrationToken, error) {
	log.Printf("[DEBUG] findIntegrationToken: Looking for integration token with base URL: %s", baseURL)

	sqlQuery := `
		SELECT id, provider, access_token, refresh_token, expires_at, provider_app_id, client_secret, provider_url
		FROM integration_tokens
		WHERE provider_url LIKE $1
		ORDER BY created_at DESC
		LIMIT 1
	`
	queryPattern := "%" + baseURL + "%"
	log.Printf("[DEBUG] findIntegrationToken: Query pattern: %s", queryPattern)

	token := &IntegrationToken{}
	err := s.db.QueryRow(sqlQuery, queryPattern).Scan(
		&token.ID, &token.Provider, &token.AccessToken, &token.RefreshToken,
		&token.ExpiresAt, &token.ClientID, &token.ClientSecret, &token.ProviderURL,
	)

	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] findIntegrationToken: No integration token found for URL: %s", baseURL)
		s.logAvailableTokens() // Helper to show what tokens are available
		return nil, fmt.Errorf("URL is not from a connected Git provider. Please connect the provider first")
	} else if err != nil {
		log.Printf("[DEBUG] findIntegrationToken: Database error: %v", err)
		return nil, fmt.Errorf("database error: %v", err)
	}

	log.Printf("[DEBUG] findIntegrationToken: Found integration token. ID: %d, Provider: %s, Provider URL: %s",
		token.ID, token.Provider, token.ProviderURL)
	log.Printf("[DEBUG] findIntegrationToken: Token details - Access Token: %s, Has Refresh Token: %v, Expires At: %v",
		maskToken(token.AccessToken), token.RefreshToken != "", token.ExpiresAt)

	return token, nil
}

// logAvailableTokens logs all available tokens in the database for debugging
func (s *Server) logAvailableTokens() {
	var count int
	countErr := s.db.QueryRow("SELECT COUNT(*) FROM integration_tokens").Scan(&count)
	if countErr == nil {
		log.Printf("[DEBUG] logAvailableTokens: Found %d total integration tokens in database", count)

		rows, listErr := s.db.Query("SELECT id, provider, provider_url FROM integration_tokens")
		if listErr == nil {
			defer rows.Close()
			log.Printf("[DEBUG] logAvailableTokens: Available tokens:")
			for rows.Next() {
				var id int64
				var prov, url string
				if scanErr := rows.Scan(&id, &prov, &url); scanErr == nil {
					log.Printf("[DEBUG] logAvailableTokens:   - ID: %d, Provider: %s, URL: %s", id, prov, url)
				}
			}
		}
	}
}

// checkTokenNeedsRefresh determines if a token needs to be refreshed
func checkTokenNeedsRefresh(token *IntegrationToken, forceRefresh bool) bool {
	tokenNeedsRefresh := false

	if token.ExpiresAt.Valid {
		log.Printf("[DEBUG] checkTokenNeedsRefresh: Token expires at: %v", token.ExpiresAt.Time)
		timeUntilExpiry := time.Until(token.ExpiresAt.Time)
		log.Printf("[DEBUG] checkTokenNeedsRefresh: Time until expiry: %v", timeUntilExpiry)
		tokenNeedsRefresh = time.Now().Add(5 * time.Minute).After(token.ExpiresAt.Time)
		log.Printf("[DEBUG] checkTokenNeedsRefresh: Token needs refresh based on expiry: %v", tokenNeedsRefresh)
	} else {
		log.Printf("[DEBUG] checkTokenNeedsRefresh: Token has no expiry time")
	}

	if forceRefresh {
		log.Printf("[DEBUG] checkTokenNeedsRefresh: Forcing token refresh for debugging")
		tokenNeedsRefresh = true
	}

	log.Printf("[DEBUG] checkTokenNeedsRefresh: Final decision - needs refresh: %v", tokenNeedsRefresh)
	return tokenNeedsRefresh
}

// refreshTokenIfNeeded refreshes the token if it needs refreshing and updates the token data
func (s *Server) refreshTokenIfNeeded(token *IntegrationToken, forceRefresh bool) error {
	if !checkTokenNeedsRefresh(token, forceRefresh) {
		return nil
	}

	log.Printf("[DEBUG] refreshTokenIfNeeded: Token for integration ID %d needs refreshing...", token.ID)
	log.Printf("[DEBUG] refreshTokenIfNeeded: Refresh parameters - Client ID: %s, Client Secret length: %d, Provider URL: %s",
		token.ClientID, len(token.ClientSecret), token.ProviderURL)

	result := s.RefreshGitLabToken(token.ID, token.ClientID, token.ClientSecret)

	if result.Error != nil {
		log.Printf("[DEBUG] refreshTokenIfNeeded: Failed to refresh token: %v", result.Error)
		log.Printf("[DEBUG] refreshTokenIfNeeded: Will try to continue with existing token despite refresh failure")
		return fmt.Errorf("failed to refresh token: %v", result.Error)
	}

	// Update token with refreshed values
	log.Printf("[DEBUG] refreshTokenIfNeeded: Token refreshed successfully")
	token.AccessToken = result.TokenData.AccessToken
	token.RefreshToken = result.TokenData.RefreshToken

	// Safely log part of the new token
	maskedToken := "unknown"
	if len(token.AccessToken) > 10 {
		maskedToken = token.AccessToken[:5] + "..." + token.AccessToken[len(token.AccessToken)-5:]
	}
	log.Printf("[DEBUG] refreshTokenIfNeeded: Updated access token: %s (type: %s, length: %d)",
		maskedToken,
		strings.Split(token.AccessToken, "-")[0],
		len(token.AccessToken))

	return nil
}

// validateProvider checks if the provider is supported
func validateProvider(provider string) error {
	if provider != "gitlab" && provider != "github" {
		log.Printf("[DEBUG] validateProvider: Unsupported provider: %s", provider)
		return fmt.Errorf("unsupported provider type: %s. Currently, only GitLab and GitHub are supported", provider)
	}
	log.Printf("[DEBUG] validateProvider: Provider is supported: %s", provider)
	return nil
}

// ensureValidToken ensures we have a valid token, falling back to config file if needed
func ensureValidToken(token *IntegrationToken) string {
	accessToken := token.AccessToken

	// Check if the token from database looks valid
	if len(accessToken) < 20 || !strings.HasPrefix(accessToken, "glpat-") {
		log.Printf("[DEBUG] ensureValidToken: Token from database doesn't look like a valid GitLab PAT, trying config file...")

		// Try to load from config file as fallback
		cfg, err := config.LoadConfig("")
		if err == nil && cfg != nil {
			if gitlabConfig, ok := cfg.Providers["gitlab"]; ok {
				if configToken, ok := gitlabConfig["token"].(string); ok && configToken != "" {
					log.Printf("[DEBUG] ensureValidToken: Found token in config file: %s", maskToken(configToken))
					log.Printf("[DEBUG] ensureValidToken: Falling back to config file token")
					accessToken = configToken
				}
			}
		}
	}

	return accessToken
}

// createReviewService creates a new review service instance per request
// This allows for dynamic configuration based on request context or database state
func (s *Server) createReviewService(token *IntegrationToken) (*review.Service, error) {
	// Create factories for this specific request
	providerFactory := review.NewStandardProviderFactory()
	aiProviderFactory := review.NewStandardAIProviderFactory()

	// Build review configuration (could be customized based on database state)
	reviewConfig := review.DefaultReviewConfig()

	// Example: Could customize config based on database state
	// if token.Provider == "enterprise-gitlab" {
	//     reviewConfig.ReviewTimeout = 20 * time.Minute
	// }
	//
	// Future enhancement: Load per-user/org config from database
	// userConfig, err := s.loadUserReviewConfig(token.UserID)
	// if err == nil {
	//     reviewConfig = userConfig
	// }

	// Create review service for this specific request
	reviewService := review.NewService(providerFactory, aiProviderFactory, reviewConfig)

	return reviewService, nil
}

// buildReviewRequest creates a review request for the given parameters
func (s *Server) buildReviewRequest(
	token *IntegrationToken,
	requestURL, reviewID, accessToken string,
) (*review.ReviewRequest, error) {
	// Load configuration for building the request
	cfg, err := config.LoadConfig("")
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// Create configuration service
	configService := review.NewConfigurationService(cfg)

	// Build review request
	reviewRequest, err := configService.BuildReviewRequest(
		context.Background(),
		requestURL,
		reviewID,
		token.Provider,
		token.ProviderURL,
		accessToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build review request: %w", err)
	}

	return reviewRequest, nil
}

// processReviewInBackground handles the entire review process using the new decoupled architecture
// Creates a new review service instance per request for maximum flexibility
func (s *Server) processReviewInBackground(token *IntegrationToken, requestURL, reviewID string) {
	log.Printf("[DEBUG] processReviewInBackground: Starting background review process for URL: %s, ReviewID: %s", requestURL, reviewID)

	// Ensure we have a valid token (with config file fallback if needed)
	accessToken := ensureValidToken(token)

	// Create review service instance for this specific request
	log.Printf("[DEBUG] processReviewInBackground: Creating review service for request")
	reviewService, err := s.createReviewService(token)
	if err != nil {
		log.Printf("[DEBUG] processReviewInBackground: Failed to create review service: %v", err)
		return
	}

	// Build review request
	log.Printf("[DEBUG] processReviewInBackground: Building review request")
	reviewRequest, err := s.buildReviewRequest(token, requestURL, reviewID, accessToken)
	if err != nil {
		log.Printf("[DEBUG] processReviewInBackground: Failed to build review request: %v", err)
		return
	}

	// Set up completion callback
	completionCallback := func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] processReviewInBackground: Review %s completed successfully: %s (%d comments, took %v)",
				result.ReviewID, truncateString(result.Summary, 50),
				result.CommentsCount, result.Duration)
		} else {
			log.Printf("[ERROR] processReviewInBackground: Review %s failed: %v (took %v)",
				result.ReviewID, result.Error, result.Duration)
		}
	}

	// Process review synchronously (we're already in a goroutine)
	log.Printf("[DEBUG] processReviewInBackground: Processing review using request-scoped service")
	result := reviewService.ProcessReview(context.Background(), *reviewRequest)

	// Call completion callback
	completionCallback(result)

	log.Printf("[DEBUG] processReviewInBackground: Review process completed for URL: %s, ReviewID: %s", requestURL, reviewID)
} // truncateString truncates a string to the specified length

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// parseTriggerReviewRequest parses the request body into a TriggerReviewRequest and returns an error if parsing fails.
func parseTriggerReviewRequest(c echo.Context) (*TriggerReviewRequest, error) {
	req := new(TriggerReviewRequest)
	log.Printf("[DEBUG] TriggerReview: Parsing request body")
	if err := c.Bind(req); err != nil {
		log.Printf("[DEBUG] TriggerReview: Failed to parse request body: %v", err)
		return nil, err
	}
	log.Printf("[DEBUG] TriggerReview: Request body parsed successfully")
	return req, nil
}

// TriggerReview handles the request to trigger a code review from a URL
func (s *Server) TriggerReview(c echo.Context) error {
	log.Printf("[DEBUG] TriggerReview: Starting review request handling")

	// Authenticate admin user
	if err := s.authenticateAdmin(c); err != nil {
		return err
	}

	// Parse request body
	req, err := parseTriggerReviewRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}

	_, baseURL, err := validateAndParseURL(req.URL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Find and retrieve integration token from database
	token, err := s.findIntegrationToken(baseURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Validate that the provider is supported
	if err := validateProvider(token.Provider); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Check if token needs refresh and refresh if necessary
	forceRefresh := true // Set to true to force a token refresh for debugging
	if err := s.refreshTokenIfNeeded(token, forceRefresh); err != nil {
		// Log the error but continue with existing token as fallback
		log.Printf("[DEBUG] TriggerReview: Token refresh failed, continuing with existing token: %v", err)
	}

	// Create a reviewID
	reviewID := fmt.Sprintf("review-%d", time.Now().Unix())
	log.Printf("[DEBUG] TriggerReview: Generated review ID: %s", reviewID)

	// Trigger the review process in a goroutine
	log.Printf("[DEBUG] TriggerReview: Starting review process in background goroutine")
	go s.processReviewInBackground(token, req.URL, reviewID)

	// Return success response immediately
	log.Printf("[DEBUG] TriggerReview: Returning success response with reviewID: %s", reviewID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully. You will receive a notification when it's complete.",
		URL:      req.URL,
		ReviewID: reviewID,
	})
}
