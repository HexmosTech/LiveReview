package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// GitLabTokenRequest represents a request to get GitLab access token
type GitLabTokenRequest struct {
	Code               string `json:"code" query:"code"`
	GitlabURL          string `json:"gitlab_url" query:"gitlab_url"`
	GitlabClientID     string `json:"gitlab_client_id" query:"gitlab_client_id"`
	GitlabClientSecret string `json:"gitlab_client_secret" query:"gitlab_client_secret"`
	RedirectURI        string `json:"redirect_uri" query:"redirect_uri"`
	ConnectionName     string `json:"connection_name" query:"connection_name"`
}

// GitLabTokenResponse is the response from GitLab OAuth token endpoint
type GitLabTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	CreatedAt    int    `json:"created_at"`
}

// GitLabUserResponse is the response from GitLab user info endpoint
type GitLabUserResponse struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// GitLabHandleCodeExchange handles the exchange of authorization code for access token
func (s *Server) GitLabHandleCodeExchange(c echo.Context) error {
	// Parse request parameters
	var req GitLabTokenRequest
	if err := c.Bind(&req); err != nil {
		log.Printf("Failed to bind request: %v", err)
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate required parameters
	if req.Code == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Missing code parameter",
		})
	}
	if req.GitlabURL == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Missing gitlab_url parameter",
		})
	}
	if req.GitlabClientID == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Missing gitlab_client_id parameter",
		})
	}
	if req.GitlabClientSecret == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Missing gitlab_client_secret parameter",
		})
	}

	// Construct token endpoint URL
	tokenEndpoint := fmt.Sprintf("%s/oauth/token", strings.TrimSuffix(req.GitlabURL, "/"))

	// Prepare the form data for the token request
	form := url.Values{}
	form.Add("client_id", req.GitlabClientID)
	form.Add("client_secret", req.GitlabClientSecret)
	form.Add("code", req.Code)
	form.Add("grant_type", "authorization_code")
	if req.RedirectURI != "" {
		form.Add("redirect_uri", req.RedirectURI)
	}

	// Make request to GitLab token endpoint
	httpClient := &http.Client{Timeout: 10 * time.Second}
	tokenReq, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		log.Printf("Failed to create token request: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to create token request: " + err.Error(),
		})
	}

	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	log.Printf("Making GitLab token request to: %s with params: %+v", tokenReq.URL.String(), form)

	tokenResp, err := httpClient.Do(tokenReq)
	if err != nil {
		log.Printf("Failed to send token request: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to send token request: " + err.Error(),
		})
	}
	defer tokenResp.Body.Close()

	// Read and parse the response
	tokenRespBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		log.Printf("Failed to read token response: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to read token response: " + err.Error(),
		})
	}

	if tokenResp.StatusCode != http.StatusOK {
		log.Printf("GitLab token request failed with status %d: %s",
			tokenResp.StatusCode, string(tokenRespBody))
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("GitLab token request failed with status %d: %s",
				tokenResp.StatusCode, string(tokenRespBody)),
		})
	}

	log.Printf("GitLab token response successful (status %d), parsing response", tokenResp.StatusCode)

	var tokenData GitLabTokenResponse
	if err := json.Unmarshal(tokenRespBody, &tokenData); err != nil {
		log.Printf("Failed to parse token response: %v - Response body: %s", err, string(tokenRespBody))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to parse token response: " + err.Error(),
		})
	}

	log.Printf("Successfully parsed token response. Token type: %s, Expires in: %d", tokenData.TokenType, tokenData.ExpiresIn)

	// Fetch user details from GitLab
	userInfo, err := fetchGitLabUserInfo(req.GitlabURL, tokenData.AccessToken)
	if err != nil {
		log.Printf("Failed to fetch GitLab user info: %v", err)
		// Continue with the process even if user info fails
	}

	// Calculate token expiration
	var expiresAt *time.Time
	if tokenData.ExpiresIn > 0 {
		expTime := time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
		expiresAt = &expTime
	}

	// Prepare metadata
	metadata := map[string]interface{}{
		"token_type": tokenData.TokenType,
		"scope":      tokenData.Scope,
		"created_at": tokenData.CreatedAt,
	}

	if userInfo != nil {
		metadata["user_id"] = userInfo.ID
		metadata["username"] = userInfo.Username
		metadata["email"] = userInfo.Email
		metadata["name"] = userInfo.Name
		metadata["avatar_url"] = userInfo.AvatarURL
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		log.Printf("Failed to marshal metadata: %v", err)
		// Continue with empty metadata if marshaling fails
		metadataJSON = []byte("{}")
	}

	// Use a transaction to ensure data consistency
	tx, err := s.db.Begin()
	if err != nil {
		log.Printf("Failed to begin transaction: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	// Determine connection name if not provided
	connectionName := req.ConnectionName
	if connectionName == "" {
		if strings.Contains(req.GitlabURL, "gitlab.com") {
			connectionName = "GitLab.com"
		} else {
			// Extract domain from URL for self-hosted instances
			u, err := url.Parse(req.GitlabURL)
			if err == nil {
				connectionName = u.Hostname()
			} else {
				connectionName = "GitLab"
			}
		}
	}

	// Store token in database
	var integrationTokenID int64
	var expiresAtSQL interface{} = nil
	if expiresAt != nil {
		expiresAtSQL = expiresAt.Format(time.RFC3339)
	}

	// Check if token already exists for this provider and app ID
	var existingID int64
	err = tx.QueryRow(`
		SELECT id FROM integration_tokens 
		WHERE provider = 'gitlab' AND provider_app_id = $1 AND connection_name = $2
	`, req.GitlabClientID, connectionName).Scan(&existingID)

	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to check for existing token: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Prepare for insert/update with proper gitlab URL
	// If token has gitlabURL, include it in the operations
	metadataWithURL := make(map[string]interface{})
	for k, v := range metadata {
		metadataWithURL[k] = v
	}

	if err == sql.ErrNoRows {
		// Insert new token
		err = tx.QueryRow(`
			INSERT INTO integration_tokens 
			(provider, provider_app_id, access_token, refresh_token, token_type, scope, 
			 expires_at, metadata, code, connection_name, provider_url, client_secret)
			VALUES ('gitlab', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id
		`, req.GitlabClientID, tokenData.AccessToken, tokenData.RefreshToken,
			tokenData.TokenType, tokenData.Scope, expiresAtSQL, metadataJSON,
			req.Code, connectionName, req.GitlabURL, req.GitlabClientSecret).Scan(&integrationTokenID)
	} else {
		// Update existing token
		err = tx.QueryRow(`
			UPDATE integration_tokens 
			SET access_token = $1, refresh_token = $2, token_type = $3, scope = $4, 
			    expires_at = $5, metadata = $6, code = $7, updated_at = CURRENT_TIMESTAMP,
				provider_url = $8, client_secret = $9
			WHERE id = $10
			RETURNING id
		`, tokenData.AccessToken, tokenData.RefreshToken, tokenData.TokenType,
			tokenData.Scope, expiresAtSQL, metadataJSON, req.Code, req.GitlabURL,
			req.GitlabClientSecret, existingID).Scan(&integrationTokenID)

		if err == nil {
			integrationTokenID = existingID
		}
	}

	if err != nil {
		log.Printf("Failed to store token in database: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to store token: " + err.Error(),
		})
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Trigger automatic webhook installation in background
	if s.autoWebhookInstaller != nil {
		s.autoWebhookInstaller.TriggerAutoInstallation(int(integrationTokenID))
	}

	// Prepare the response
	username := ""
	if userInfo != nil {
		username = userInfo.Username
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":         "GitLab integration successful",
		"integration_id":  integrationTokenID,
		"username":        username,
		"connection_name": connectionName,
	})
}

// fetchGitLabUserInfo fetches user information from GitLab
func fetchGitLabUserInfo(gitlabURL, accessToken string) (*GitLabUserResponse, error) {
	userEndpoint := fmt.Sprintf("%s/api/v4/user", strings.TrimSuffix(gitlabURL, "/"))

	httpClient := &http.Client{Timeout: 10 * time.Second}
	userReq, err := http.NewRequest("GET", userEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := httpClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send user info request: %w", err)
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(userResp.Body)
		return nil, fmt.Errorf("GitLab user info request failed with status %d: %s",
			userResp.StatusCode, string(body))
	}

	var userInfo GitLabUserResponse
	if err := json.NewDecoder(userResp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %w", err)
	}

	return &userInfo, nil
}

// GitLabRefreshTokenResult contains the result of a GitLab token refresh operation
type GitLabRefreshTokenResult struct {
	TokenData     GitLabTokenResponse
	IntegrationID int64
	Error         error
}

// RefreshGitLabToken refreshes an expired GitLab token and updates it in the database
func (s *Server) RefreshGitLabToken(integrationID int64, clientID, clientSecret string) GitLabRefreshTokenResult {
	result := GitLabRefreshTokenResult{
		IntegrationID: integrationID,
	}

	// Retrieve token data
	var gitlabURL, refreshToken, storedClientSecret string
	err := s.db.QueryRow(`
		SELECT refresh_token, provider_url, client_secret
		FROM integration_tokens
		WHERE id = $1 AND provider = 'gitlab'
	`, integrationID).Scan(&refreshToken, &gitlabURL, &storedClientSecret)

	if err != nil {
		result.Error = fmt.Errorf("failed to retrieve token data: %w", err)
		return result
	}

	if refreshToken == "" {
		result.Error = fmt.Errorf("no refresh token available")
		return result
	}

	// If client ID wasn't provided, try to get it from the database
	if clientID == "" {
		err := s.db.QueryRow(`
			SELECT provider_app_id FROM integration_tokens WHERE id = $1
		`, integrationID).Scan(&clientID)

		if err != nil {
			result.Error = fmt.Errorf("failed to retrieve client ID: %w", err)
			return result
		}
	}

	// If client secret wasn't provided, use the one from the database
	if clientSecret == "" && storedClientSecret != "" {
		clientSecret = storedClientSecret
	}

	// Cannot proceed without client secret
	if clientSecret == "" {
		result.Error = fmt.Errorf("missing client secret parameter")
		return result
	}

	// Construct token endpoint URL
	tokenEndpoint := fmt.Sprintf("%s/oauth/token", strings.TrimSuffix(gitlabURL, "/"))

	// Prepare the form data for the token refresh request
	form := url.Values{}
	form.Add("client_id", clientID)
	form.Add("client_secret", clientSecret)
	form.Add("refresh_token", refreshToken)
	form.Add("grant_type", "refresh_token")

	// Make request to GitLab token endpoint
	httpClient := &http.Client{Timeout: 10 * time.Second}
	tokenReq, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		result.Error = fmt.Errorf("failed to create token request: %w", err)
		return result
	}

	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := httpClient.Do(tokenReq)
	if err != nil {
		result.Error = fmt.Errorf("failed to send token request: %w", err)
		return result
	}
	defer tokenResp.Body.Close()

	// Read and parse the response
	tokenRespBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		result.Error = fmt.Errorf("failed to read token response: %w", err)
		return result
	}

	fmt.Printf("GitLab token refresh response status: %d\n", tokenResp.StatusCode)
	fmt.Printf("GitLab token refresh response body: %s\n", string(tokenRespBody))

	if tokenResp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("GitLab token refresh failed with status %d: %s",
			tokenResp.StatusCode, string(tokenRespBody))
		return result
	}

	if err := json.Unmarshal(tokenRespBody, &result.TokenData); err != nil {
		result.Error = fmt.Errorf("failed to parse token response: %w", err)
		return result
	}

	// Calculate token expiration
	var expiresAt *time.Time
	if result.TokenData.ExpiresIn > 0 {
		expTime := time.Now().Add(time.Duration(result.TokenData.ExpiresIn) * time.Second)
		expiresAt = &expTime
	}

	// Update token in database
	var expiresAtSQL interface{} = nil
	if expiresAt != nil {
		expiresAtSQL = expiresAt.Format(time.RFC3339)
	}

	_, err = s.db.Exec(`
		UPDATE integration_tokens 
		SET access_token = $1, refresh_token = $2, token_type = $3, scope = $4, 
		    expires_at = $5, updated_at = CURRENT_TIMESTAMP
		WHERE id = $6
	`, result.TokenData.AccessToken, result.TokenData.RefreshToken, result.TokenData.TokenType,
		result.TokenData.Scope, expiresAtSQL, integrationID)

	if err != nil {
		result.Error = fmt.Errorf("failed to update token: %w", err)
		return result
	}

	return result
}

// GitLabRefreshToken refreshes an expired GitLab token
func (s *Server) GitLabRefreshToken(c echo.Context) error {
	// Parse request parameters
	var req struct {
		IntegrationID      int64  `json:"integration_id" query:"integration_id"`
		GitlabClientID     string `json:"gitlab_client_id" query:"gitlab_client_id"`
		GitlabClientSecret string `json:"gitlab_client_secret" query:"gitlab_client_secret"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate required parameters
	if req.IntegrationID <= 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Missing or invalid integration_id parameter",
		})
	}

	// Call the extracted function to refresh the token
	result := s.RefreshGitLabToken(req.IntegrationID, req.GitlabClientID, req.GitlabClientSecret)

	if result.Error != nil {
		// Handle different error types
		if strings.Contains(result.Error.Error(), "failed to retrieve token data: sql: no rows") {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Token not found",
			})
		}
		if strings.Contains(result.Error.Error(), "no refresh token available") {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "No refresh token available",
			})
		}
		if strings.Contains(result.Error.Error(), "missing client secret parameter") {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "Client secret not found in request or database",
			})
		}

		// For all other errors, return an internal server error
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: result.Error.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":        "GitLab token refreshed successfully",
		"integration_id": req.IntegrationID,
		"expires_in":     result.TokenData.ExpiresIn,
	})
}
