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

	"github.com/livereview/internal/githubapp"
	networkgithub "github.com/livereview/network/providers/github"
	storagegithub "github.com/livereview/storage/providers/github"
)

// IntegrationToken holds GitHub-specific token data needed for API calls.
type IntegrationToken struct {
	ID          int64
	OrgID       int64
	Provider    string
	ProviderURL string
	TokenType   string
	PatToken    string
	Metadata    map[string]interface{}
}

// connectorIDFromMetadata reads connector_id stamped by the webhook orchestrator. After a JSON
// round trip through the job queue it arrives as float64, so accept the numeric forms.
func connectorIDFromMetadata(meta map[string]interface{}) (int64, error) {
	switch v := meta["connector_id"].(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	}
	return 0, fmt.Errorf("connector_id missing from event metadata")
}

type connectorCtxKey struct{}

// WithConnectorID carries the receiving connector through a context, for provider interfaces
// (e.g. lrcfetch.Provider) whose signatures have no event to read it from.
func WithConnectorID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, connectorCtxKey{}, id)
}

func connectorIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(connectorCtxKey{}).(int64)
	return id, ok
}

// FindIntegrationTokenForGitHubConnector loads the GitHub connector that received the event.
func FindIntegrationTokenForGitHubConnector(db *sql.DB, connectorID int64) (*IntegrationToken, error) {
	token := &IntegrationToken{Metadata: make(map[string]interface{})}
	store := storagegithub.NewTokenStore(db)
	rec, err := store.GetGitHubTokenByConnectorID(connectorID)
	if err != nil {
		return nil, fmt.Errorf("no GitHub integration token found for connector %d: %w", connectorID, err)
	}
	token.ID = rec.ID
	token.OrgID = rec.OrgID
	token.Provider = rec.Provider
	token.ProviderURL = rec.ProviderURL
	token.TokenType = rec.TokenType

	// PAT connectors return pat_token as-is; GitHub App connectors return a fresh installation token.
	apiToken, err := githubapp.ConnectorToken(context.Background(), store, rec)
	if err != nil {
		return nil, fmt.Errorf("github connector %d: %w", connectorID, err)
	}
	token.PatToken = apiToken

	if len(rec.Metadata) > 0 {
		if err := json.Unmarshal(rec.Metadata, &token.Metadata); err != nil {
			log.Printf("[WARN] Failed to parse GitHub token metadata: %v", err)
		}
	}

	return token, nil
}

// FindIntegrationTokenForEvent resolves the connector stamped on a webhook event by the orchestrator.
func FindIntegrationTokenForEvent(db *sql.DB, event *UnifiedWebhookEventV2) (*IntegrationToken, error) {
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}
	if _, err := connectorIDFromMetadata(event.Repository.Metadata); err == nil {
		return findIntegrationTokenForMetadata(db, event.Repository.Metadata)
	}
	if event.MergeRequest != nil {
		return findIntegrationTokenForMetadata(db, event.MergeRequest.Metadata)
	}
	return nil, fmt.Errorf("connector_id missing from event metadata")
}

// findIntegrationTokenForMetadata resolves the connector stamped into an event's metadata.
func findIntegrationTokenForMetadata(db *sql.DB, meta map[string]interface{}) (*IntegrationToken, error) {
	connectorID, err := connectorIDFromMetadata(meta)
	if err != nil {
		return nil, err
	}
	return FindIntegrationTokenForGitHubConnector(db, connectorID)
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
