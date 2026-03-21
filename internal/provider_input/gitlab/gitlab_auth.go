package gitlab

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	networkgitlab "github.com/livereview/network/providers/gitlab"
	storagegitlab "github.com/livereview/storage/providers/gitlab"
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
	tokenStore          *storagegitlab.AuthTokenStore
}

// NewAuthService constructs a GitLab auth service.
func NewAuthService(db *sql.DB, trigger func(int)) *AuthService {
	client := networkgitlab.NewHTTPClient(10 * time.Second)
	if trigger == nil {
		trigger = func(int) {}
	}
	return &AuthService{
		db:                  db,
		triggerInstallation: trigger,
		httpClient:          client,
		now:                 time.Now,
		tokenStore:          storagegitlab.NewAuthTokenStore(db),
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
	tokenReq, err := networkgitlab.NewRequest(http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to create token request", err: err}
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := networkgitlab.Do(s.httpClient, tokenReq)
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

	metadataJSON, err := storagegitlab.BuildGitLabTokenMetadata(
		tokenData.TokenType,
		tokenData.Scope,
		tokenData.CreatedAt,
		func() int {
			if userInfo != nil {
				return userInfo.ID
			}
			return 0
		}(),
		func() string {
			if userInfo != nil {
				return userInfo.Username
			}
			return ""
		}(),
		func() string {
			if userInfo != nil {
				return userInfo.Email
			}
			return ""
		}(),
		func() string {
			if userInfo != nil {
				return userInfo.Name
			}
			return ""
		}(),
		func() string {
			if userInfo != nil {
				return userInfo.AvatarURL
			}
			return ""
		}(),
	)
	if err != nil {
		log.Printf("Failed to marshal metadata: %v", err)
		metadataJSON = []byte("{}")
	}

	connectionName := req.ConnectionName
	if connectionName == "" {
		if strings.Contains(req.GitlabURL, "gitlab.com") {
			connectionName = "GitLab.com"
		} else if u, err := networkgitlab.ParseURL(req.GitlabURL); err == nil {
			connectionName = u.Hostname()
		} else {
			connectionName = "GitLab"
		}
	}

	integrationTokenID, err := s.tokenStore.UpsertGitLabIntegrationToken(storagegitlab.TokenUpsertInput{
		ProviderAppID:  req.GitlabClientID,
		AccessToken:    tokenData.AccessToken,
		RefreshToken:   tokenData.RefreshToken,
		TokenType:      tokenData.TokenType,
		Scope:          tokenData.Scope,
		ExpiresAt:      expiresAt,
		MetadataJSON:   metadataJSON,
		Code:           req.Code,
		ConnectionName: connectionName,
		ProviderURL:    req.GitlabURL,
		ClientSecret:   req.GitlabClientSecret,
	})
	if err != nil {
		return nil, &AuthError{Status: http.StatusInternalServerError, Message: "Failed to store token", err: err}
	}

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

	rec, err := s.tokenStore.GetGitLabTokenRecord(integrationID)
	if err != nil {
		result.Error = fmt.Errorf("failed to retrieve token data: %w", err)
		return result
	}

	gitlabURL := rec.ProviderURL
	refreshToken := rec.RefreshToken
	storedClientSecret := rec.StoredClientSecret

	if refreshToken == "" {
		result.Error = fmt.Errorf("no refresh token available")
		return result
	}

	if clientID == "" {
		if clientID, err = s.tokenStore.GetProviderAppID(integrationID); err != nil {
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

	requestCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokenReq, err := networkgitlab.NewRequestWithContext(requestCtx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		result.Error = fmt.Errorf("failed to create token request: %w", err)
		return result
	}
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("Accept", "application/json")

	tokenResp, err := networkgitlab.Do(s.httpClient, tokenReq)
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

	err = s.tokenStore.UpdateRefreshedToken(storagegitlab.TokenRefreshUpdateInput{
		IntegrationID: integrationID,
		AccessToken:   result.TokenData.AccessToken,
		RefreshToken:  result.TokenData.RefreshToken,
		TokenType:     result.TokenData.TokenType,
		Scope:         result.TokenData.Scope,
		ExpiresAt:     expiresAt,
	})

	if err != nil {
		result.Error = fmt.Errorf("failed to update token: %w", err)
	}

	return result
}

func (s *AuthService) fetchGitLabUserInfo(gitlabURL, accessToken string) (*GitLabUserResponse, error) {
	userEndpoint := fmt.Sprintf("%s/api/v4/user", strings.TrimSuffix(gitlabURL, "/"))

	userReq, err := networkgitlab.NewRequest(http.MethodGet, userEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create user info request: %w", err)
	}

	userReq.Header.Set("Authorization", "Bearer "+accessToken)
	userReq.Header.Set("Accept", "application/json")

	resp, err := networkgitlab.Do(s.httpClient, userReq)
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
