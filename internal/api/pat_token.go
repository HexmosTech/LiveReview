package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
)

// Handler for creating PAT integration token
func HandleCreatePATIntegrationToken(db *sql.DB, c echo.Context) error {
	type reqBody struct {
		Name     string                 `json:"name"` // connector_name
		Type     string                 `json:"type"` // provider
		URL      string                 `json:"url"`  // provider_url
		PATToken string                 `json:"pat_token"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	var body reqBody
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
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
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid metadata format"})
		}
	} else {
		metadataJSON = []byte("{}")
	}

	// Set expires_at to NULL for PAT tokens
	query := `INSERT INTO integration_tokens (provider, provider_app_id, token_type, pat_token, access_token, connection_name, provider_url, metadata, expires_at, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now()) RETURNING id`
	var id int64
	err = db.QueryRow(query,
		body.Type,     // provider
		"",            // provider_app_id (empty for manual PAT)
		"PAT",         // token_type
		body.PATToken, // pat_token
		"NA",          // access_token
		body.Name,     // connection_name
		body.URL,      // provider_url
		metadataJSON,  // metadata
		nil,           // expires_at
	).Scan(&id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]interface{}{"id": id})
}
