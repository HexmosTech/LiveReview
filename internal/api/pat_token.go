package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/providers/gitea"
)

// CreatePATRequest represents the request to create a PAT integration token
type CreatePATRequest struct {
	Name     string                 `json:"name" jsonschema:"description=Display name for the connection (e.g. 'My GitHub')"`
	Type     string                 `json:"type" jsonschema:"description=Provider type (github, gitlab, bitbucket, gitea)"`
	URL      string                 `json:"url" jsonschema:"description=Provider base URL (e.g. 'https://github.com')"`
	PATToken string                 `json:"pat_token" jsonschema:"description=Personal Access Token from the provider"`
	Metadata map[string]interface{} `json:"metadata,omitempty" jsonschema:"description=Optional provider-specific metadata"`
}

// Handler for creating PAT integration token
func HandleCreatePATIntegrationToken(db *sql.DB, c echo.Context) error {
	connectorID, _, err := CreatePATIntegrationToken(db, c)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": connectorID})
}

// CreatePATIntegrationToken creates a PAT integration token and returns the ID
func CreatePATIntegrationToken(db *sql.DB, c echo.Context) (int64, int64, error) {
	var body CreatePATRequest
	if err := c.Bind(&body); err != nil {
		return 0, 0, fmt.Errorf("invalid request body: %w", err)
	}

	// Extract org_id from context (set by BuildOrgContextFromHeader middleware)
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return 0, 0, fmt.Errorf("organization context not found - missing org_id")
	}

	// Remove username from metadata if present (should be connector_name)
	if body.Metadata != nil {
		delete(body.Metadata, "username")
	}

	// Marshal metadata to JSON
	var metadataJSON []byte
	var err error
	if body.Metadata != nil {
		metadataJSON, err = json.Marshal(body.Metadata)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid metadata format: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	// Normalize provider URL for certain providers (e.g., strip swagger path for Gitea)
	providerURL := body.URL
	if strings.HasPrefix(body.Type, "gitea") {
		providerURL = gitea.NormalizeGiteaBaseURL(body.URL)
	}

	// Set expires_at to NULL for PAT tokens and include org_id
	query := `INSERT INTO integration_tokens (provider, provider_app_id, token_type, pat_token, access_token, connection_name, provider_url, metadata, expires_at, org_id, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now()) RETURNING id`
	var id int64
	err = db.QueryRow(query,
		body.Type,     // provider
		"",            // provider_app_id (empty for manual PAT)
		"PAT",         // token_type
		body.PATToken, // pat_token
		"NA",          // access_token
		body.Name,     // connection_name (from frontend 'name')
		providerURL,   // provider_url (normalized)
		metadataJSON,  // metadata
		nil,           // expires_at
		orgID,         // org_id
	).Scan(&id)
	if err != nil {
		return 0, 0, fmt.Errorf("database error: %w", err)
	}
	return id, orgID, nil
}
