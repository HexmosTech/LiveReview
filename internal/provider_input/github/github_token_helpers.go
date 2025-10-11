package github

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// IntegrationToken holds GitHub-specific token data needed for API calls.
type IntegrationToken struct {
	ID          int64
	Provider    string
	ProviderURL string
	PatToken    string
	Metadata    map[string]interface{}
}

// FindIntegrationTokenForGitHubRepo looks up a GitHub integration token.
func FindIntegrationTokenForGitHubRepo(db *sql.DB, repoFullName string) (*IntegrationToken, error) {
	query := `
		SELECT id, provider, provider_url, pat_token, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE provider = 'github'
		  AND (provider_url = 'https://github.com' OR provider_url = 'https://api.github.com')
		ORDER BY created_at DESC
		LIMIT 1
	`

	token := &IntegrationToken{Metadata: make(map[string]interface{})}
	var metadataJSON []byte
	if err := db.QueryRow(query).Scan(&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &metadataJSON); err != nil {
		return nil, fmt.Errorf("no GitHub integration token found: %w", err)
	}

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &token.Metadata); err != nil {
			log.Printf("[WARN] Failed to parse GitHub token metadata: %v", err)
		}
	}

	return token, nil
}

// FetchGitHubBotUserInfo retrieves the authenticated bot user using the supplied PAT.
func FetchGitHubBotUserInfo(pat string) (*GitHubV2BotUserInfo, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API error (status %d): %s", resp.StatusCode, string(body))
	}

	var user GitHubV2BotUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("failed to decode GitHub user response: %w", err)
	}

	return &user, nil
}
