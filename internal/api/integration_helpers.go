package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func (s *Server) findIntegrationTokenForGitHubRepo(repoFullName string) (*IntegrationToken, error) {
	return s.findIntegrationTokenForRepo(repoFullName, "github")
}

func (s *Server) findIntegrationTokenForBitbucketRepo(repoFullName string) (*IntegrationToken, error) {
	return s.findIntegrationTokenForRepo(repoFullName, "bitbucket")
}

func (s *Server) findIntegrationTokenForRepo(repoFullName, providerPrefix string) (*IntegrationToken, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("server database handle unavailable")
	}

	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token, it.org_id, COALESCE(it.metadata, '{}')
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE wr.project_full_name = $1
		LIMIT 1
	`

	token := &IntegrationToken{Metadata: make(map[string]interface{})}
	var metadataJSON []byte
	err := s.db.QueryRow(query, repoFullName).Scan(
		&token.ID,
		&token.Provider,
		&token.ProviderURL,
		&token.PatToken,
		&token.OrgID,
		&metadataJSON,
	)
	if err != nil {
		fallbackQuery := `
			SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}')
			FROM integration_tokens
			WHERE provider LIKE $1
			ORDER BY created_at DESC
			LIMIT 1
		`

		err = s.db.QueryRow(fallbackQuery, providerPrefix+"%").Scan(
			&token.ID,
			&token.Provider,
			&token.ProviderURL,
			&token.PatToken,
			&token.OrgID,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("no %s integration token found for %s: %w", providerPrefix, repoFullName, err)
		}
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &token.Metadata); err != nil {
			log.Printf("[WARN] Failed to parse integration token metadata for %s: %v", repoFullName, err)
		}
	}

	if token.Metadata == nil {
		token.Metadata = make(map[string]interface{})
	}

	return token, nil
}

// GitHubBotUserInfo represents the subset of bot info fields the orchestrator requires.
type GitHubBotUserInfo struct {
	Login     string `json:"login"`
	ID        int    `json:"id"`
	Name      string `json:"name"`
	HTMLURL   string `json:"html_url"`
	AvatarURL string `json:"avatar_url"`
	Type      string `json:"type"`
}

func (s *Server) getFreshGitHubBotUserInfo(repoFullName string) (*GitHubBotUserInfo, error) {
	token, err := s.findIntegrationTokenForGitHubRepo(repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to locate GitHub integration token: %w", err)
	}

	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct GitHub user request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token.PatToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error status %d: %s", resp.StatusCode, string(body))
	}

	var botUser GitHubBotUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&botUser); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub bot user response: %w", err)
	}

	return &botUser, nil
}

// BitbucketUserInfo captures the minimal bot fields we rely on for comparisons.
type BitbucketUserInfo struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AccountID   string `json:"account_id"`
	Type        string `json:"type"`
}

func (s *Server) getFreshBitbucketBotUserInfo(repoFullName string) (*BitbucketUserInfo, error) {
	token, err := s.findIntegrationTokenForBitbucketRepo(repoFullName)
	if err != nil {
		return nil, fmt.Errorf("failed to locate Bitbucket integration token: %w", err)
	}

	email, _ := token.Metadata["email"].(string)
	if email == "" {
		return nil, fmt.Errorf("bitbucket token metadata missing email")
	}

	req, err := http.NewRequest("GET", "https://api.bitbucket.org/2.0/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct Bitbucket user request: %w", err)
	}

	req.SetBasicAuth(email, token.PatToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call Bitbucket API: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Bitbucket API error status %d: %s", resp.StatusCode, string(body))
	}

	var user BitbucketUserInfo
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("failed to decode Bitbucket bot user response: %w", err)
	}

	return &user, nil
}
