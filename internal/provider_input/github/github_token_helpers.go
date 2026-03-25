package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	networkgithub "github.com/livereview/network/providers/github"
	storagegithub "github.com/livereview/storage/providers/github"
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
	token := &IntegrationToken{Metadata: make(map[string]interface{})}
	store := storagegithub.NewTokenStore(db)
	rec, err := store.GetLatestGitHubToken()
	if err != nil {
		return nil, fmt.Errorf("no GitHub integration token found: %w", err)
	}
	token.ID = rec.ID
	token.Provider = rec.Provider
	token.ProviderURL = rec.ProviderURL
	token.PatToken = rec.PatToken
	metadataJSON := rec.Metadata

	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &token.Metadata); err != nil {
			log.Printf("[WARN] Failed to parse GitHub token metadata: %v", err)
		}
	}

	return token, nil
}

// FetchGitHubBotUserInfo retrieves the authenticated bot user using the supplied PAT.
func FetchGitHubBotUserInfo(pat string) (*GitHubV2BotUserInfo, error) {
	req, err := networkgithub.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+pat)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := networkgithub.NewHTTPClient(10 * time.Second)
	resp, err := networkgithub.Do(client, req)
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
