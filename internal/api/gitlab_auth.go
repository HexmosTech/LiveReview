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

// GitLabGetAccessToken handles the exchange of authorization code for access token
func (s *Server) GitLabGetAccessToken(c echo.Context) error {
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

	log.Printf("Making GitLab token request to: %s", tokenReq.URL.String())

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

	var tokenData GitLabTokenResponse
	if err := json.Unmarshal(tokenRespBody, &tokenData); err != nil {
		log.Printf("Failed to parse token response: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to parse token response: " + err.Error(),
		})
	}

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
	var tokenID int64
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

	if err == sql.ErrNoRows {
		// Insert new token
		err = tx.QueryRow(`
			INSERT INTO integration_tokens 
			(provider, provider_app_id, access_token, refresh_token, token_type, scope, expires_at, metadata, code, connection_name)
			VALUES ('gitlab', $1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, req.GitlabClientID, tokenData.AccessToken, tokenData.RefreshToken,
			tokenData.TokenType, tokenData.Scope, expiresAtSQL, metadataJSON, req.Code, connectionName).Scan(&tokenID)
	} else {
		// Update existing token
		err = tx.QueryRow(`
			UPDATE integration_tokens 
			SET access_token = $1, refresh_token = $2, token_type = $3, scope = $4, 
			    expires_at = $5, metadata = $6, code = $7, updated_at = CURRENT_TIMESTAMP
			WHERE id = $8
			RETURNING id
		`, tokenData.AccessToken, tokenData.RefreshToken, tokenData.TokenType,
			tokenData.Scope, expiresAtSQL, metadataJSON, req.Code, existingID).Scan(&tokenID)

		if err == nil {
			tokenID = existingID
		}
	}

	if err != nil {
		log.Printf("Failed to store token in database: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to store token: " + err.Error(),
		})
	}

	// Check if integration record exists
	var integrationID int64
	err = tx.QueryRow(`
		SELECT id FROM integration_tables 
		WHERE provider = 'gitlab' AND provider_app_id = $1 AND connection_name = $2
	`, req.GitlabClientID, connectionName).Scan(&integrationID)

	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to check for existing integration: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	integrationMetadata := map[string]interface{}{
		"gitlab_url": req.GitlabURL,
	}

	if userInfo != nil {
		integrationMetadata["user_id"] = userInfo.ID
		integrationMetadata["username"] = userInfo.Username
	}

	integrationMetadataJSON, err := json.Marshal(integrationMetadata)
	if err != nil {
		log.Printf("Failed to marshal integration metadata: %v", err)
		integrationMetadataJSON = []byte("{}")
	}

	if err == sql.ErrNoRows {
		// Insert new integration record
		err = tx.QueryRow(`
			INSERT INTO integration_tables 
			(provider, provider_app_id, connection_name, metadata)
			VALUES ('gitlab', $1, $2, $3)
			RETURNING id
		`, req.GitlabClientID, connectionName, integrationMetadataJSON).Scan(&integrationID)
	} else {
		// Update existing integration record
		_, err = tx.Exec(`
			UPDATE integration_tables 
			SET metadata = $1, updated_at = CURRENT_TIMESTAMP
			WHERE id = $2
		`, integrationMetadataJSON, integrationID)
	}

	if err != nil {
		log.Printf("Failed to store integration data: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to store integration data: " + err.Error(),
		})
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Prepare the response
	username := ""
	if userInfo != nil {
		username = userInfo.Username
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":         "GitLab integration successful",
		"token_id":        tokenID,
		"integration_id":  integrationID,
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

// GitLabRefreshToken refreshes an expired GitLab token
func (s *Server) GitLabRefreshToken(c echo.Context) error {
	// Parse request parameters
	var req struct {
		TokenID            int64  `json:"token_id" query:"token_id"`
		GitlabClientID     string `json:"gitlab_client_id" query:"gitlab_client_id"`
		GitlabClientSecret string `json:"gitlab_client_secret" query:"gitlab_client_secret"`
	}

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate required parameters
	if req.TokenID <= 0 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Missing or invalid token_id parameter",
		})
	}

	// Retrieve token data
	var gitlabURL, refreshToken string
	err := s.db.QueryRow(`
		SELECT t.refresh_token, i.metadata->>'gitlab_url' 
		FROM integration_tokens t
		JOIN integration_tables i ON t.provider = i.provider AND t.provider_app_id = i.provider_app_id
		WHERE t.id = $1 AND t.provider = 'gitlab'
	`, req.TokenID).Scan(&refreshToken, &gitlabURL)

	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Token not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	if refreshToken == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "No refresh token available",
		})
	}

	// If client credentials weren't provided, try to get them from the database
	if req.GitlabClientID == "" || req.GitlabClientSecret == "" {
		err := s.db.QueryRow(`
			SELECT provider_app_id FROM integration_tokens WHERE id = $1
		`, req.TokenID).Scan(&req.GitlabClientID)

		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "Failed to retrieve client ID: " + err.Error(),
			})
		}

		// Note: We can't retrieve the client secret from the database as we don't store it
		if req.GitlabClientSecret == "" {
			return c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: "Missing gitlab_client_secret parameter and not stored in database",
			})
		}
	}

	// Construct token endpoint URL
	tokenEndpoint := fmt.Sprintf("%s/oauth/token", strings.TrimSuffix(gitlabURL, "/"))

	// Prepare the form data for the token refresh request
	form := url.Values{}
	form.Add("client_id", req.GitlabClientID)
	form.Add("client_secret", req.GitlabClientSecret)
	form.Add("refresh_token", refreshToken)
	form.Add("grant_type", "refresh_token")

	// Make request to GitLab token endpoint
	httpClient := &http.Client{Timeout: 10 * time.Second}
	tokenReq, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to create token request: " + err.Error(),
		})
	}

	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := httpClient.Do(tokenReq)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to send token request: " + err.Error(),
		})
	}
	defer tokenResp.Body.Close()

	// Read and parse the response
	tokenRespBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to read token response: " + err.Error(),
		})
	}

	if tokenResp.StatusCode != http.StatusOK {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("GitLab token refresh failed with status %d: %s",
				tokenResp.StatusCode, string(tokenRespBody)),
		})
	}

	var tokenData GitLabTokenResponse
	if err := json.Unmarshal(tokenRespBody, &tokenData); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to parse token response: " + err.Error(),
		})
	}

	// Calculate token expiration
	var expiresAt *time.Time
	if tokenData.ExpiresIn > 0 {
		expTime := time.Now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
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
	`, tokenData.AccessToken, tokenData.RefreshToken, tokenData.TokenType,
		tokenData.Scope, expiresAtSQL, req.TokenID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to update token: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "GitLab token refreshed successfully",
		"token_id":   req.TokenID,
		"expires_in": tokenData.ExpiresIn,
	})
}
