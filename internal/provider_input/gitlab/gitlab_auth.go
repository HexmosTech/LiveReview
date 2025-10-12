package gitlab

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
)

type (
	GitLabTokenRequest struct {
		Code               string
		GitlabURL          string
		GitlabClientID     string
		GitlabClientSecret string
		RedirectURI        string
		ConnectionName     string
	}
	GitLabTokenResponse struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		CreatedAt    int    `json:"created_at"`
	}
	GitLabUserResponse struct {
		ID        int    `json:"id"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
	}
	GitLabRefreshTokenResult struct {
		TokenData     GitLabTokenResponse
		IntegrationID int64
		Error         error
	}
)

// AuthError captures a failure with an associated HTTP-like status code.
type AuthError struct {
	Status  int
	Message string
	err     error
}

func (e *AuthError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.err)
	}
	return e.Message
}

func (e *AuthError) Unwrap() error {
	return e.err
}

// AuthExchangeResult contains the outcome of a successful code exchange.
type AuthExchangeResult struct {
	IntegrationID  int64
	Username       string
	ConnectionName string
}

// AuthService encapsulates GitLab authentication workflows backed by the DB.
type AuthService struct {
	db                  *sql.DB
	triggerInstallation func(int)
	httpClient          *http.Client
	now                 func() time.Time
}

// NewAuthService constructs a GitLab auth service.
func NewAuthService(db *sql.DB, trigger func(int)) *AuthService {
	client := &http.Client{Timeout: 10 * time.Second}
	if trigger == nil {
		trigger = func(int) {}
	}
	return &AuthService{
		db:                  db,
		triggerInstallation: trigger,
		httpClient:          client,
		now:                 time.Now,
	}
}

// ExchangeCode stores GitLab tokens exchanged via OAuth and returns integration info.
func (s *AuthService) ExchangeCode(req GitLabTokenRequest) (*AuthExchangeResult, error) {
	if req.Code == "" {
		return nil, &AuthError{Status: http.StatusBadRequest, Message: "Missing code parameter"}
	}
	if req.GitlabURL == "" {
		return nil, &AuthError{Status: http.StatusBadRequest, Message: "Missing gitlab_url parameter"}
	}
	if req.GitlabClientID == "" {
		return nil, &AuthError{Status: http.StatusBadRequest, Message: "Missing gitlab_client_id parameter"}
	}
	if req.GitlabClientSecret == "" {
		return nil, &AuthError{Status: http.StatusBadRequest, Message: "Missing gitlab_client_secret parameter"}
	}

	tokenEndpoint := fmt.Sprintf("%s/oauth/token", strings.TrimSuffix(req.GitlabURL, "/"))

	form := url.Values{}
	form.Add("client_id", req.GitlabClientID)
	form.Add("client_secret", req.GitlabClientSecret)
	form.Add("code", req.Code)
	form.Add("grant_type", "authorization_code")
	if req.RedirectURI != "" {
		form.Add("redirect_uri", req.RedirectURI)
	}

	log.Printf("Making GitLab token request to: %s with params: %+v", tokenEndpoint, form)
	tokenReq, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to create token request", err: err}
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := s.httpClient.Do(tokenReq)
	if err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to send token request", err: err}
	}
	defer tokenResp.Body.Close()

	tokenRespBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to read token response", err: err}
	}

	if tokenResp.StatusCode != http.StatusOK {
		log.Printf("GitLab token request failed with status %d: %s", tokenResp.StatusCode, string(tokenRespBody))
		return nil, &AuthError{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("GitLab token request failed with status %d: %s", tokenResp.StatusCode, string(tokenRespBody)),
		}
	}

	var tokenData GitLabTokenResponse
	if err := json.Unmarshal(tokenRespBody, &tokenData); err != nil {
		log.Printf("Failed to parse token response: %v - Response body: %s", err, string(tokenRespBody))
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to parse token response", err: err}
	}

	log.Printf("Successfully parsed token response. Token type: %s, Expires in: %d", tokenData.TokenType, tokenData.ExpiresIn)

	userInfo, err := s.fetchGitLabUserInfo(req.GitlabURL, tokenData.AccessToken)
	if err != nil {
		log.Printf("Failed to fetch GitLab user info: %v", err)
	}

	var expiresAt *time.Time
	if tokenData.ExpiresIn > 0 {
		expTime := s.now().Add(time.Duration(tokenData.ExpiresIn) * time.Second)
		expiresAt = &expTime
	}

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
		metadataJSON = []byte("{}")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Database error", err: err}
	}

	rollback := true
	defer func() {
		if rollback {
			tx.Rollback()
		}
	}()

	connectionName := req.ConnectionName
	if connectionName == "" {
		if strings.Contains(req.GitlabURL, "gitlab.com") {
			connectionName = "GitLab.com"
		} else if u, err := url.Parse(req.GitlabURL); err == nil {
			connectionName = u.Hostname()
		} else {
			connectionName = "GitLab"
		}
	}

	var expiresAtSQL interface{}
	if expiresAt != nil {
		expiresAtSQL = expiresAt.Format(time.RFC3339)
	}

	var existingID int64
	err = tx.QueryRow(`
		SELECT id FROM integration_tokens 
		WHERE provider = 'gitlab' AND provider_app_id = $1 AND connection_name = $2
	`, req.GitlabClientID, connectionName).Scan(&existingID)

	if err != nil && err != sql.ErrNoRows {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Database error", err: err}
	}

	var integrationTokenID int64
	if err == sql.ErrNoRows {
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
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to store token", err: err}
	}

	if err := tx.Commit(); err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Database error", err: err}
	}
	rollback = false

	s.triggerInstallation(int(integrationTokenID))

	username := ""
	if userInfo != nil {
		username = userInfo.Username
	}

	return &AuthExchangeResult{
		IntegrationID:  integrationTokenID,
		Username:       username,
		ConnectionName: connectionName,
	}, nil
}

// RefreshToken refreshes an expired GitLab token and updates it in the database.
func (s *AuthService) RefreshToken(integrationID int64, clientID, clientSecret string) GitLabRefreshTokenResult {
	result := GitLabRefreshTokenResult{IntegrationID: integrationID}

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

	if clientID == "" {
		if err := s.db.QueryRow(`
			SELECT provider_app_id FROM integration_tokens WHERE id = $1
		`, integrationID).Scan(&clientID); err != nil {
			result.Error = fmt.Errorf("failed to retrieve client ID: %w", err)
			return result
		}
	}

	if clientSecret == "" && storedClientSecret != "" {
		clientSecret = storedClientSecret
	}

	if clientSecret == "" {
		result.Error = fmt.Errorf("missing client secret parameter")
		return result
	}

	tokenEndpoint := fmt.Sprintf("%s/oauth/token", strings.TrimSuffix(gitlabURL, "/"))

	form := url.Values{}
	form.Add("client_id", clientID)
	form.Add("client_secret", clientSecret)
	form.Add("refresh_token", refreshToken)
	form.Add("grant_type", "refresh_token")

	tokenReq, err := http.NewRequest("POST", tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		result.Error = fmt.Errorf("failed to create token request: %w", err)
		return result
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := s.httpClient.Do(tokenReq)
	if err != nil {
		result.Error = fmt.Errorf("failed to send token request: %w", err)
		return result
	}
	defer tokenResp.Body.Close()

	tokenRespBody, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		result.Error = fmt.Errorf("failed to read token response: %w", err)
		return result
	}

	log.Printf("GitLab token refresh response status: %d", tokenResp.StatusCode)
	log.Printf("GitLab token refresh response body: %s", string(tokenRespBody))

	if tokenResp.StatusCode != http.StatusOK {
		result.Error = fmt.Errorf("GitLab token refresh failed with status %d: %s", tokenResp.StatusCode, string(tokenRespBody))
		return result
	}

	if err := json.Unmarshal(tokenRespBody, &result.TokenData); err != nil {
		result.Error = fmt.Errorf("failed to parse token response: %w", err)
		return result
	}

	var expiresAt *time.Time
	if result.TokenData.ExpiresIn > 0 {
		expTime := s.now().Add(time.Duration(result.TokenData.ExpiresIn) * time.Second)
		expiresAt = &expTime
	}

	var expiresAtSQL interface{}
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
	}

	return result
}

func (s *AuthService) fetchGitLabUserInfo(gitlabURL, accessToken string) (*GitLabUserResponse, error) {
	userEndpoint := fmt.Sprintf("%s/api/v4/user", strings.TrimSuffix(gitlabURL, "/"))

	userReq, err := http.NewRequest("GET", userEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	userReq.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send user info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab user info request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var userInfo GitLabUserResponse
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %w", err)
	}

	return &userInfo, nil
}
