package azuredevops

import (
	"database/sql"
	"encoding/json"
	"fmt"

	azuredevopsutils "github.com/livereview/internal/providers/azuredevops"
)

// FindIntegrationTokenForAzureDevOpsRepo finds the integration token for an
// Azure DevOps repository identified by its "{project}/{repo}" full name.
// Returns the token and its normalized organization URL.
func FindIntegrationTokenForAzureDevOpsRepo(db *sql.DB, repoFullName string) (*IntegrationToken, string, error) {
	if db == nil {
		return nil, "", fmt.Errorf("database connection is nil")
	}
	if repoFullName == "" {
		return nil, "", fmt.Errorf("repository full name is empty")
	}

	// webhook_registry.project_full_name stores the same "{project}/{repo}" value
	// produced by DiscoverProjectsAzureDevOps, so join against it to disambiguate
	// between multiple Azure DevOps connectors that might share project names.
	query := `
		SELECT it.id, it.provider, it.provider_url, it.pat_token, it.org_id, COALESCE(it.metadata, '{}')
		FROM integration_tokens it
		JOIN webhook_registry wr ON wr.integration_token_id = it.id
		WHERE it.provider LIKE 'azuredevops%' AND wr.project_full_name = $1
		ORDER BY it.updated_at DESC
		LIMIT 1
	`
	token, err := scanAzureDevOpsToken(db.QueryRow(query, repoFullName))
	if err == nil {
		return token, token.ProviderURL, nil
	}

	// Fallback: no webhook_registry row yet (e.g. manual-trigger-only connector).
	// Only safe when exactly one Azure DevOps connector exists - guessing "most
	// recently updated" among several would risk silently picking the wrong
	// connector/PAT for this repo (this has happened in practice when two
	// connectors pointed at the same org), so fail loudly instead of guessing.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM integration_tokens WHERE provider LIKE 'azuredevops%'`).Scan(&count); err != nil {
		return nil, "", fmt.Errorf("failed to count Azure DevOps connectors: %w", err)
	}
	if count == 0 {
		return nil, "", fmt.Errorf("no Azure DevOps integration token found for repository %s", repoFullName)
	}
	if count > 1 {
		return nil, "", fmt.Errorf("multiple Azure DevOps connectors exist and no webhook_registry entry found for repository %s - cannot determine which connector owns this repo; run 'Enable Manual Trigger' for this repo under the correct connector first", repoFullName)
	}

	fallbackQuery := `
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE provider LIKE 'azuredevops%'
		LIMIT 1
	`
	token, ferr := scanAzureDevOpsToken(db.QueryRow(fallbackQuery))
	if ferr != nil {
		return nil, "", fmt.Errorf("no Azure DevOps integration token found for repository %s", repoFullName)
	}
	return token, token.ProviderURL, nil
}

// FindIntegrationTokenForAzureDevOpsOrg finds the integration token for an
// Azure DevOps organization URL directly. Used for webhook comment events,
// which carry no repository/project names (only GUIDs) - since every Azure
// DevOps connector is already scoped to one whole organization, matching on
// org URL alone is sufficient without needing repo full name resolution first.
func FindIntegrationTokenForAzureDevOpsOrg(db *sql.DB, orgURL string) (*IntegrationToken, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	orgURL = azuredevopsutils.NormalizeOrgURL(orgURL)
	if orgURL == "" {
		return nil, fmt.Errorf("organization URL is empty")
	}

	query := `
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE provider LIKE 'azuredevops%' AND provider_url = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`
	token, err := scanAzureDevOpsToken(db.QueryRow(query, orgURL))
	if err != nil {
		return nil, fmt.Errorf("no Azure DevOps integration token found for organization %s", orgURL)
	}
	return token, nil
}

// FindIntegrationTokenByConnectorID finds the Azure DevOps integration token by connector ID.
func FindIntegrationTokenByConnectorID(db *sql.DB, connectorID int64) (*IntegrationToken, string, error) {
	if db == nil {
		return nil, "", fmt.Errorf("database connection is nil")
	}

	query := `
		SELECT id, provider, provider_url, pat_token, org_id, COALESCE(metadata, '{}')
		FROM integration_tokens
		WHERE id = $1
	`
	token, err := scanAzureDevOpsToken(db.QueryRow(query, connectorID))
	if err != nil {
		return nil, "", fmt.Errorf("no Azure DevOps connector found with id %d: %w", connectorID, err)
	}
	return token, token.ProviderURL, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAzureDevOpsToken(row rowScanner) (*IntegrationToken, error) {
	var token IntegrationToken
	var metadataJSON string

	if err := row.Scan(&token.ID, &token.Provider, &token.ProviderURL, &token.PatToken, &token.OrgID, &metadataJSON); err != nil {
		return nil, err
	}

	token.Metadata = make(map[string]any)
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &token.Metadata); err != nil {
			return nil, fmt.Errorf("failed to parse azuredevops metadata for connector %d: %w", token.ID, err)
		}
	}

	token.ProviderURL = azuredevopsutils.NormalizeOrgURL(token.ProviderURL)
	return &token, nil
}

// FindWebhookSecretByConnectorID finds the webhook shared secret for a connector
// from webhook_registry. Returns an empty string (no error) when no webhook is
// registered yet, which is the expected state for manual-trigger-only connectors.
func FindWebhookSecretByConnectorID(db *sql.DB, connectorID int) (string, error) {
	if db == nil {
		return "", fmt.Errorf("database connection is nil")
	}

	var secret sql.NullString
	err := db.QueryRow(`
		SELECT webhook_secret FROM webhook_registry
		WHERE integration_token_id = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`, connectorID).Scan(&secret)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to query webhook secret: %w", err)
	}
	if !secret.Valid {
		return "", nil
	}
	return secret.String, nil
}
