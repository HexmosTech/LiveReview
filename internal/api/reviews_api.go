package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/aidefault"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/license"
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
	RefreshToken sql.NullString
	ExpiresAt    sql.NullTime
	ClientID     string
	ClientSecret sql.NullString
	ProviderURL  string
	TokenType    string
	PatToken     string
	Metadata     map[string]interface{}
	OrgID        int64
}

// maskToken safely masks a token for logging
func maskToken(token string) string {
	return token
	/*
		if len(token) <= 10 {
			return "[HIDDEN]"
		}
		return token[:5] + "..." + token[len(token)-5:]
	*/
}

// findIntegrationToken queries the database to find an integration token for the given base URL
func (s *Server) findIntegrationToken(baseURL string) (*IntegrationToken, error) {
	log.Printf("[DEBUG] findIntegrationToken: Looking for integration token with base URL: %s", baseURL)

	sqlQuery := `
		SELECT id, provider, access_token, refresh_token, expires_at, provider_app_id, client_secret, provider_url, token_type, pat_token, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE provider_url LIKE '%' || $1 || '%'
		ORDER BY created_at DESC
		LIMIT 1
	`
	log.Printf("[DEBUG] findIntegrationToken: Query base URL: %s", baseURL)

	token := &IntegrationToken{}
	var metadataJSON string
	err := s.db.QueryRow(sqlQuery, baseURL).Scan(
		&token.ID, &token.Provider, &token.AccessToken, &token.RefreshToken,
		&token.ExpiresAt, &token.ClientID, &token.ClientSecret, &token.ProviderURL,
		&token.TokenType, &token.PatToken, &metadataJSON,
	)

	if err == sql.ErrNoRows {
		log.Printf("[DEBUG] findIntegrationToken: No integration token found for URL: %s", baseURL)
		s.logAvailableTokens() // Helper to show what tokens are available
		return nil, fmt.Errorf("URL is not from a connected Git provider. Please connect the provider first")
	} else if err != nil {
		log.Printf("[DEBUG] findIntegrationToken: Database error: %v", err)
		return nil, fmt.Errorf("database error: %v", err)
	}

	// Parse metadata JSON
	token.Metadata = make(map[string]interface{})
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &token.Metadata); err != nil {
			log.Printf("[DEBUG] findIntegrationToken: Failed to parse metadata: %v", err)
			// Continue with empty metadata rather than failing
		}
	}

	log.Printf("[DEBUG] findIntegrationToken: Found integration token. ID: %d, Provider: %s, Provider URL: %s",
		token.ID, token.Provider, token.ProviderURL)
	log.Printf("[DEBUG] findIntegrationToken: Token details - Access Token: %s, Has Refresh Token: %v, Expires At: %v",
		maskToken(token.AccessToken), token.RefreshToken.Valid, token.ExpiresAt)

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
	if token.Provider == "github" {
		log.Printf("[DEBUG] checkTokenNeedsRefresh: Skipping refresh for GitHub PAT tokens")
		return false
	}
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
		token.ClientID, len(token.ClientSecret.String), token.ProviderURL)

	result := s.RefreshGitLabToken(token.ID, token.ClientID, token.ClientSecret.String)

	if result.Error != nil {
		log.Printf("[DEBUG] refreshTokenIfNeeded: Failed to refresh token: %v", result.Error)
		log.Printf("[DEBUG] refreshTokenIfNeeded: Will try to continue with existing token despite refresh failure")
		return fmt.Errorf("failed to refresh token: %v", result.Error)
	}

	// Update token with refreshed values
	log.Printf("[DEBUG] refreshTokenIfNeeded: Token refreshed successfully")
	token.AccessToken = result.TokenData.AccessToken
	token.RefreshToken = sql.NullString{String: result.TokenData.RefreshToken, Valid: result.TokenData.RefreshToken != ""}

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
	// Support GitLab variants (gitlab, gitlab-self-hosted, etc.), GitHub variants, Bitbucket variants, and Gitea
	if !strings.HasPrefix(provider, "gitlab") &&
		!strings.HasPrefix(provider, "github") &&
		!strings.HasPrefix(provider, "bitbucket") &&
		!strings.HasPrefix(provider, "gitea") {
		log.Printf("[DEBUG] validateProvider: Unsupported provider: %s", provider)
		return fmt.Errorf("unsupported provider type: %s. Currently, GitLab, GitHub, Bitbucket, and Gitea variants are supported", provider)
	}
	log.Printf("[DEBUG] validateProvider: Provider is supported: %s", provider)
	return nil
}

// ensureValidToken ensures we have a valid token, falling back to config file if needed
func ensureValidToken(token *IntegrationToken) string {
	// Handle GitHub PAT tokens
	if strings.HasPrefix(token.Provider, "github") && token.TokenType == "PAT" {
		log.Printf("[DEBUG] ensureValidToken: Using GitHub PAT from database: %s", maskToken(token.PatToken))
		return token.PatToken
	}

	// Handle GitLab variants (gitlab, gitlab-self-hosted, etc.)
	if strings.HasPrefix(token.Provider, "gitlab") {
		// Prefer PAT token if available
		if token.TokenType == "PAT" && token.PatToken != "" {
			log.Printf("[DEBUG] ensureValidToken: Using GitLab PAT from database: %s", maskToken(token.PatToken))
			return token.PatToken
		}

		// Fall back to access token
		accessToken := token.AccessToken
		if len(accessToken) < 20 || !strings.HasPrefix(accessToken, "glpat-") {
			log.Printf("[DEBUG] ensureValidToken: Token from database doesn't look like a valid GitLab PAT, trying config file...")
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

	// Handle Bitbucket variants
	if strings.HasPrefix(token.Provider, "bitbucket") {
		// Prefer PAT token if available
		if token.TokenType == "PAT" && token.PatToken != "" {
			log.Printf("[DEBUG] ensureValidToken: Using Bitbucket PAT from database: %s", maskToken(token.PatToken))
			return token.PatToken
		}

		// Fall back to access token
		accessToken := token.AccessToken
		log.Printf("[DEBUG] ensureValidToken: Using Bitbucket access token: %s", maskToken(accessToken))
		return accessToken
	}

	// Handle Gitea variants
	if strings.HasPrefix(token.Provider, "gitea") {
		if token.TokenType == "PAT" && token.PatToken != "" {
			pat, _, _ := decodePATPayload(token.PatToken)
			log.Printf("[DEBUG] ensureValidToken: Using Gitea PAT from database: %s", maskToken(pat))
			return pat
		}
		pat, _, _ := decodePATPayload(token.AccessToken)
		if pat == "" {
			pat = token.AccessToken
		}
		log.Printf("[DEBUG] ensureValidToken: Using Gitea access token: %s", maskToken(pat))
		return pat
	}

	// Default fallback
	return token.AccessToken
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

// getAIConfigFromDatabase retrieves AI configuration from ai_connectors table
func (s *Server) getAIConfigFromDatabase(ctx context.Context, orgID int64, planCode license.PlanType) (review.AIConfig, error) {
	// Create storage instance to query ai_connectors table
	storage := aiconnectors.NewStorage(s.db)

	// Get all connectors ordered by display_order
	connectors, err := storage.GetAllConnectors(ctx, orgID)
	if err != nil {
		return review.AIConfig{}, fmt.Errorf("failed to get AI connectors: %w", err)
	}

	if planCode == "" {
		planCode = license.PlanFree30K
	}

	// Free tier enforces BYOK strictly (system_managed AI is not available).
	if planCode == license.PlanFree30K {
		var byokConnector *aiconnectors.ConnectorRecord
		for _, c := range connectors {
			if c.ProviderName != "livereview-default-ai" && c.ApiKey != "system_managed" {
				byokConnector = c
				break
			}
		}

		if byokConnector == nil {
			return review.AIConfig{}, fmt.Errorf("the Free plan (PlanFree30K) requires you to configure your own Gemini/AI key (BYOK) for organization %d. Managed AI is not available in the Free tier.", orgID)
		}
		return buildBYOKAIConfig(byokConnector, "byok_required")
	}

	// Paid team defaults to hosted auto model when no BYOK connector is configured.
	if planCode == license.PlanTeam32USD {
		if len(connectors) > 0 {
			connector := connectors[0]
			if connector.ProviderName == "livereview-default-ai" {
				return buildDefaultAIConfig(ctx, s.db, connector)
			}
			return buildBYOKAIConfig(connector, "byok_override")
		}
		return buildHostedAutoAIConfig()
	}

	// Fallback for other plans: prefer BYOK if present, else hosted-auto.
	if len(connectors) > 0 {
		return buildBYOKAIConfig(connectors[0], "byok_optional")
	}
	return buildHostedAutoAIConfig()
}

func buildDefaultAIConfig(ctx context.Context, db *sql.DB, record *aiconnectors.ConnectorRecord) (review.AIConfig, error) {
	tier := record.GetSelectedModel()
	if tier == "" {
		tier = "default"
	}
	options, err := aidefault.ResolveConnectorOptions(ctx, db, tier)
	if err != nil {
		return review.AIConfig{}, fmt.Errorf("failed to resolve managed AI options for tier %s: %w", tier, err)
	}

	// Build AIConfig from resolved options
	configMap := map[string]interface{}{
		"provider_name":       record.ProviderName,
		"ai_provider_type":    string(options.Provider),
		"connector_name":      record.ConnectorName,
		"display_order":       record.DisplayOrder,
		"ai_execution_mode":   "managed_default",
		"ai_execution_source": "internal",
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      options.APIKey,
		Model:       options.ModelConfig.Model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

func buildBYOKAIConfig(connector *aiconnectors.ConnectorRecord, executionMode string) (review.AIConfig, error) {
	if connector == nil {
		return review.AIConfig{}, fmt.Errorf("connector is required for BYOK mode")
	}

	// Debug logging to see which connector is selected
	fmt.Printf("[AI CONFIG] Selected connector: %s (%s) with display_order: %d\n",
		connector.ConnectorName, connector.ProviderName, connector.DisplayOrder)

	// Map provider_name to AI type for langchain
	aiType := "langchain" // We always use langchain as the AI type

	// Determine model - use selectedModel if available, otherwise default
	var model string
	if connector.SelectedModel.Valid && connector.SelectedModel.String != "" {
		model = connector.SelectedModel.String
	} else {
		// Default models based on provider
		switch connector.ProviderName {
		case "ollama":
			model = "llama3.2:latest" // Default Ollama model
		case "gemini":
			model = "gemini-2.5-flash" // Default Gemini model
		case "openai":
			model = "o4-mini" // Default OpenAI model
		case "deepseek":
			model = "deepseek-chat" // Default DeepSeek model
		case "openrouter":
			model = "deepseek/deepseek-r1-0528:free" // Default OpenRouter model
		case "claude":
			model = "claude-haiku-4-5-20251001" // Default Anthropic model
		default:
			model = "gemini-2.5-flash" // Default fallback
		}
	}

	// Prepare configuration map with provider details
	configMap := map[string]interface{}{
		"provider_name":       connector.ProviderName,
		"connector_name":      connector.ConnectorName,
		"display_order":       connector.DisplayOrder,
		"ai_execution_mode":   executionMode,
		"ai_execution_source": "connector",
	}

	// Add base URL if available
	baseURL := ""
	if connector.BaseURL.Valid && connector.BaseURL.String != "" {
		baseURL = connector.BaseURL.String
	}
	baseURL = aiconnectors.ResolveBaseURLForProviderName(connector.ProviderName, baseURL)

	if baseURL != "" {
		configMap["base_url"] = baseURL
		fmt.Printf("[AI CONFIG] Using base URL: %s\n", baseURL)
	} else {
		fmt.Printf("[AI CONFIG] No base URL configured\n")
	}

	fmt.Printf("[AI CONFIG] Final model: %s\n", model)
	fmt.Printf("[AI CONFIG] API Key length: %d\n", len(connector.ApiKey))

	return review.AIConfig{
		Type:        aiType,
		APIKey:      connector.ApiKey,
		Model:       model,
		Temperature: 0.4, // Default temperature
		Config:      configMap,
	}, nil
}

func buildHostedAutoAIConfig() (review.AIConfig, error) {
	providerName := strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_PROVIDER"))
	if providerName == "" {
		providerName = "gemini"
	}

	model := strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_MODEL"))
	if model == "" {
		switch providerName {
		case "openai":
			model = "o4-mini"
		case "deepseek":
			model = "deepseek-chat"
		case "openrouter":
			model = "deepseek/deepseek-r1-0528:free"
		case "claude":
			model = "claude-haiku-4-5-20251001"
		case "ollama":
			model = "llama3.2:latest"
		default:
			model = "gemini-2.5-flash"
		}
	}

	apiKey := ""
	switch providerName {
	case "gemini":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_GEMINI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
		}
	case "openai":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_OPENAI_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		}
	case "deepseek":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_DEEPSEEK_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
		}
	case "openrouter":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_OPENROUTER_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
		}
	case "claude":
		apiKey = strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_CLAUDE_API_KEY"))
		if apiKey == "" {
			apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
		}
	case "ollama":
		// Ollama does not require API key.
	default:
		return review.AIConfig{}, fmt.Errorf("unsupported hosted auto provider: %s", providerName)
	}

	if providerName != "ollama" && apiKey == "" {
		return review.AIConfig{}, fmt.Errorf("hosted auto provider '%s' is configured without API key; set LIVEREVIEW_HOSTED_*_API_KEY", providerName)
	}

	configMap := map[string]interface{}{
		"provider_name":       providerName,
		"connector_name":      "Hosted Auto",
		"display_order":       -1,
		"ai_execution_mode":   "hosted_auto",
		"ai_execution_source": "platform",
	}

	baseURL := aiconnectors.ResolveBaseURLForProviderName(providerName, strings.TrimSpace(os.Getenv("LIVEREVIEW_HOSTED_AI_BASE_URL")))
	if baseURL != "" {
		configMap["base_url"] = baseURL
	}

	return review.AIConfig{
		Type:        "langchain",
		APIKey:      apiKey,
		Model:       model,
		Temperature: 0.4,
		Config:      configMap,
	}, nil
}

// buildReviewRequest creates a review request for the given parameters
func (s *Server) buildReviewRequest(
	token *IntegrationToken,
	requestURL, reviewID, accessToken string,
	orgID int64,
	planCode license.PlanType,
) (*review.ReviewRequest, error) {
	// Get AI configuration from database instead of config files
	aiConfig, err := s.getAIConfigFromDatabase(context.Background(), orgID, planCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI configuration from database: %w", err)
	}

	// Build provider configuration
	providerToken := accessToken
	providerConfigMap := map[string]interface{}{}
	if token.TokenType == "PAT" && token.PatToken != "" {
		if strings.HasPrefix(token.Provider, "github") {
			providerToken = token.PatToken
			providerConfigMap["pat_token"] = token.PatToken
		} else if strings.HasPrefix(token.Provider, "gitlab") {
			providerToken = token.PatToken
			providerConfigMap["pat_token"] = token.PatToken
		} else if strings.HasPrefix(token.Provider, "bitbucket") {
			providerToken = token.PatToken
			providerConfigMap["pat_token"] = token.PatToken
			// For Bitbucket, also need email from metadata if available
			if token.Metadata != nil {
				if email, ok := token.Metadata["email"].(string); ok {
					providerConfigMap["email"] = email
				}
			}
		} else if strings.HasPrefix(token.Provider, "gitea") {
			pat, user, pass := decodePATPayload(token.PatToken)
			if pat == "" {
				pat = token.PatToken
			}
			providerToken = pat
			providerConfigMap["pat_token"] = pat
			if user != "" {
				providerConfigMap["username"] = user
			}
			if pass != "" {
				providerConfigMap["password"] = pass
			}
		}
	}

	// Provide base URL to provider configs that need it
	if strings.HasPrefix(token.Provider, "gitea") {
		providerConfigMap["base_url"] = token.ProviderURL
		if _, ok := providerConfigMap["pat_token"]; !ok {
			providerConfigMap["pat_token"] = providerToken
		}
	}

	providerConfig := review.ProviderConfig{
		Type:   token.Provider,
		URL:    token.ProviderURL,
		Token:  providerToken,
		Config: providerConfigMap,
	}

	// Create review request directly without config service
	reviewRequest := &review.ReviewRequest{
		URL:      requestURL,
		ReviewID: reviewID,
		Provider: providerConfig,
		AI:       aiConfig,
	}

	return reviewRequest, nil
}

// truncateString truncates a string to the specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// decodePATPayload attempts to parse a JSON-encoded PAT payload that may include
// additional credentials for providers like Gitea. Expected format:
// {"pat":"...","username":"...","password":"..."}
// If parsing fails, returns empty strings allowing callers to fall back.
func decodePATPayload(raw string) (pat, username, password string) {
	var payload struct {
		Pat      string `json:"pat"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return "", "", ""
	}
	return payload.Pat, payload.Username, payload.Password
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
