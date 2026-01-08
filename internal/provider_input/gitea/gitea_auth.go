package gitea

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	giteautils "github.com/livereview/internal/providers/gitea"
)

// FindIntegrationTokenForGiteaRepo finds the integration token for a Gitea repository.
// Returns the token, normalized base URL, and any error.
func FindIntegrationTokenForGiteaRepo(db *sql.DB, repoFullName string) (*IntegrationToken, string, error) {
	if db == nil {
		return nil, "", fmt.Errorf("database connection is nil")
	}
	if repoFullName == "" {
		return nil, "", fmt.Errorf("repository full name is empty")
	}

	// Query integration_tokens table for Gitea provider matching repo
	// We need to find the connector that can handle this repository
	// This requires matching against provider_url since we don't have direct repo mapping yet
	query := `
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}') as metadata
		FROM integration_tokens
		WHERE provider = 'gitea'
		AND org_id IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT 10
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query integration tokens: %w", err)
	}
	defer rows.Close()

	var tokens []IntegrationToken
	for rows.Next() {
		var token IntegrationToken
		var metadataJSON string

		err := rows.Scan(
			&token.ID,
			&token.Provider,
			&token.ProviderURL,
			&token.PatToken,
			&token.OrgID,
			&metadataJSON,
		)
		if err != nil {
			continue
		}

		// Parse metadata JSON
		if metadataJSON != "" {
			_ = json.Unmarshal([]byte(metadataJSON), &token.Metadata)
		}
		if token.Metadata == nil {
			token.Metadata = make(map[string]interface{})
		}

		// Unpack PAT/creds if packed
		creds := giteautils.UnpackGiteaCredentials(token.PatToken)
		token.PatToken = creds.PAT
		if creds.Username != "" {
			token.Metadata["gitea_username"] = creds.Username
		}
		if creds.Password != "" {
			token.Metadata["gitea_password"] = creds.Password
		}

		// Normalize the provider URL
		token.ProviderURL = giteautils.NormalizeGiteaBaseURL(token.ProviderURL)

		tokens = append(tokens, token)
	}

	if len(tokens) == 0 {
		return nil, "", fmt.Errorf("no Gitea integration token found")
	}

	// For now, return the most recently updated token
	// TODO: In the future, we should match against webhook_registry using project_full_name
	// or extract base URL from repository web URL to match provider_url
	token := tokens[0]

	return &token, token.ProviderURL, nil
}

// FindIntegrationTokenByConnectorID finds the integration token by connector ID.
func FindIntegrationTokenByConnectorID(db *sql.DB, connectorID int) (*IntegrationToken, string, error) {
	if db == nil {
		return nil, "", fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}') as metadata
		FROM integration_tokens
		WHERE id = $1 AND provider = 'gitea'
	`

	var token IntegrationToken
	var metadataJSON string

	err := db.QueryRow(query, connectorID).Scan(
		&token.ID,
		&token.Provider,
		&token.ProviderURL,
		&token.PatToken,
		&token.OrgID,
		&metadataJSON,
	)
	if err == sql.ErrNoRows {
		return nil, "", fmt.Errorf("no Gitea connector found with id %d", connectorID)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to query integration token: %w", err)
	}

	// Parse metadata JSON
	if metadataJSON != "" {
		_ = json.Unmarshal([]byte(metadataJSON), &token.Metadata)
	}
	if token.Metadata == nil {
		token.Metadata = make(map[string]interface{})
	}

	// Unpack PAT/creds if packed
	creds := giteautils.UnpackGiteaCredentials(token.PatToken)
	token.PatToken = creds.PAT
	if creds.Username != "" {
		token.Metadata["gitea_username"] = creds.Username
	}
	if creds.Password != "" {
		token.Metadata["gitea_password"] = creds.Password
	}

	// Normalize the provider URL
	token.ProviderURL = giteautils.NormalizeGiteaBaseURL(token.ProviderURL)

	return &token, token.ProviderURL, nil
}

// ExtractGiteaBaseURLFromWebURL extracts the base URL from a Gitea repository web URL.
// Example: https://gitea.hexmos.site/megaorg/livereview -> https://gitea.hexmos.site
func ExtractGiteaBaseURLFromWebURL(webURL string) (string, error) {
	if webURL == "" {
		return "", fmt.Errorf("web URL is empty")
	}

	// Remove trailing slash
	webURL = strings.TrimSuffix(webURL, "/")

	// Find the second-to-last slash (before owner/repo)
	// Example: https://gitea.hexmos.site/megaorg/livereview
	//          -> https://gitea.hexmos.site
	parts := strings.Split(webURL, "/")
	if len(parts) < 5 {
		return "", fmt.Errorf("invalid Gitea web URL format: %s", webURL)
	}

	// Reconstruct: scheme://host[:port][/subpath]
	// Parts: [https:] [] [host] [owner] [repo]
	// We want everything up to (but not including) owner/repo
	baseURL := strings.Join(parts[:len(parts)-2], "/")

	return giteautils.NormalizeGiteaBaseURL(baseURL), nil
}

// FindWebhookSecretByConnectorID finds the webhook secret for a connector from webhook_registry.
func FindWebhookSecretByConnectorID(db *sql.DB, connectorID int) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT webhook_secret
		FROM webhook_registry
		WHERE integration_token_id = $1 AND provider = 'gitea'
		ORDER BY updated_at DESC
		LIMIT 1
	`

	var secret sql.NullString
	err := db.QueryRow(query, connectorID).Scan(&secret)
	if err == sql.ErrNoRows {
		// No webhook registered yet - this is okay for manual trigger mode
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query webhook secret: %w", err)
	}

	if !secret.Valid || secret.String == "" {
		return "", nil
	}

	return secret.String, nil
}
