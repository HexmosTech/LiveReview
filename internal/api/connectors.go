package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// ConnectorResponse represents a single connector record returned by the API
type ConnectorResponse struct {
	ID             int64           `json:"id"`
	Provider       string          `json:"provider"`
	ProviderAppID  string          `json:"provider_app_id"`
	ConnectionName string          `json:"connection_name"`
	ProviderURL    string          `json:"provider_url"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// CompleteConnectorResponse represents a connector with all fields including sensitive data
// This should only be used internally and not exposed directly via APIs
type CompleteConnectorResponse struct {
	ID             int64           `json:"id"`
	Provider       string          `json:"provider"`
	ProviderAppID  string          `json:"provider_app_id"`
	AccessToken    string          `json:"access_token"`
	RefreshToken   sql.NullString  `json:"refresh_token"`
	TokenType      sql.NullString  `json:"token_type"`
	Scope          sql.NullString  `json:"scope"`
	ExpiresAt      sql.NullTime    `json:"expires_at"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Code           sql.NullString  `json:"code"`
	ConnectionName string          `json:"connection_name"`
	ProviderURL    string          `json:"provider_url"`
	ClientSecret   sql.NullString  `json:"client_secret"`
}

// GetConnectorByURL retrieves a connector by matching the provider URL
func (s *Server) GetConnectorByURL(providerURL string) (*ConnectorResponse, error) {
	// Query the database for a matching integration token
	row := s.db.QueryRow(`
	SELECT id, provider, provider_app_id, connection_name, provider_url, metadata, created_at, updated_at
	FROM integration_tokens
	WHERE provider_url = $1
	ORDER BY created_at DESC
	LIMIT 1
	`, providerURL)

	var connector ConnectorResponse
	var metadataRaw []byte

	// Scan the results
	err := row.Scan(
		&connector.ID,
		&connector.Provider,
		&connector.ProviderAppID,
		&connector.ConnectionName,
		&connector.ProviderURL,
		&metadataRaw,
		&connector.CreatedAt,
		&connector.UpdatedAt,
	)
	if err != nil {
		log.Printf("Failed to find connector with URL %s: %v", providerURL, err)
		return nil, err
	}

	// Parse the metadata JSON
	if metadataRaw != nil {
		var metadata map[string]interface{}
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
			log.Printf("Failed to parse metadata JSON: %v", err)
			connector.Metadata = json.RawMessage("{}")
		} else {
			// Remove sensitive info like tokens
			delete(metadata, "access_token")
			delete(metadata, "refresh_token")

			// Re-marshal the filtered metadata
			filteredMetadata, err := json.Marshal(metadata)
			if err != nil {
				log.Printf("Failed to marshal filtered metadata: %v", err)
				connector.Metadata = json.RawMessage("{}")
			} else {
				connector.Metadata = filteredMetadata
			}
		}
	} else {
		connector.Metadata = json.RawMessage("{}")
	}

	return &connector, nil
}

// GetCompleteConnectorByURL retrieves all fields from a connector by matching the provider URL
// This includes sensitive data like tokens and should be used with caution
func (s *Server) GetCompleteConnectorByURL(providerURL string) (*CompleteConnectorResponse, error) {
	// Query the database for a matching integration token with ALL fields
	row := s.db.QueryRow(`
	SELECT id, provider, provider_app_id, access_token, refresh_token, 
	       token_type, scope, expires_at, metadata, created_at, updated_at,
	       code, connection_name, provider_url, client_secret
	FROM integration_tokens
	WHERE provider_url = $1
	ORDER BY created_at DESC
	LIMIT 1
	`, providerURL)

	var connector CompleteConnectorResponse
	var metadataRaw []byte

	// Scan all fields into our struct
	err := row.Scan(
		&connector.ID,
		&connector.Provider,
		&connector.ProviderAppID,
		&connector.AccessToken,
		&connector.RefreshToken,
		&connector.TokenType,
		&connector.Scope,
		&connector.ExpiresAt,
		&metadataRaw,
		&connector.CreatedAt,
		&connector.UpdatedAt,
		&connector.Code,
		&connector.ConnectionName,
		&connector.ProviderURL,
		&connector.ClientSecret,
	)
	if err != nil {
		log.Printf("Failed to find complete connector with URL %s: %v", providerURL, err)
		return nil, err
	}

	// Store the raw metadata
	connector.Metadata = metadataRaw

	return &connector, nil
}

// GetConnectorByProviderURL returns all fields for the first row matching the provider URL
func (s *Server) GetConnectorByProviderURL(providerURL string) (*CompleteConnectorResponse, error) {
	row := s.db.QueryRow(`
		SELECT id, provider, provider_app_id, access_token, refresh_token, token_type, scope, expires_at, metadata, created_at, updated_at, code, connection_name, provider_url, client_secret
		FROM integration_tokens
		WHERE provider_url = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, providerURL)

	var connector CompleteConnectorResponse
	var metadataRaw []byte
	if err := row.Scan(
		&connector.ID,
		&connector.Provider,
		&connector.ProviderAppID,
		&connector.AccessToken,
		&connector.RefreshToken,
		&connector.TokenType,
		&connector.Scope,
		&connector.ExpiresAt,
		&metadataRaw,
		&connector.CreatedAt,
		&connector.UpdatedAt,
		&connector.Code,
		&connector.ConnectionName,
		&connector.ProviderURL,
		&connector.ClientSecret,
	); err != nil {
		return nil, err
	}
	connector.Metadata = metadataRaw
	return &connector, nil
}

// GetConnectors returns all integration tokens (connectors)
func (s *Server) GetConnectors(c echo.Context) error {
	// Check if the user is authenticated
	password := c.Request().Header.Get("X-Admin-Password")
	if password == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
		})
	}

	// Get the stored hashed password
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check authentication: " + err.Error(),
		})
	}

	// Verify the provided password
	if !comparePasswords(hashedPassword, password) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Invalid authentication",
		})
	}

	// Query the database for all integration tokens
	rows, err := s.db.Query(`
	SELECT id, provider, provider_app_id, connection_name, provider_url, metadata, created_at, updated_at
	FROM integration_tokens
	ORDER BY created_at DESC
	`)
	if err != nil {
		log.Printf("Failed to query integration tokens: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}
	defer rows.Close()

	// Parse the results
	var connectors []ConnectorResponse
	for rows.Next() {
		var connector ConnectorResponse
		var metadataRaw []byte

		if err := rows.Scan(
			&connector.ID,
			&connector.Provider,
			&connector.ProviderAppID,
			&connector.ConnectionName,
			&connector.ProviderURL,
			&metadataRaw,
			&connector.CreatedAt,
			&connector.UpdatedAt,
		); err != nil {
			log.Printf("Failed to scan integration token row: %v", err)
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "Database error: " + err.Error(),
			})
		}

		// Parse the metadata JSON
		if metadataRaw != nil {
			var metadata map[string]interface{}
			if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
				log.Printf("Failed to parse metadata JSON: %v", err)
				connector.Metadata = json.RawMessage("{}")
			} else {
				// Remove sensitive info like tokens
				delete(metadata, "access_token")
				delete(metadata, "refresh_token")

				// Re-marshal the filtered metadata
				filteredMetadata, err := json.Marshal(metadata)
				if err != nil {
					log.Printf("Failed to marshal filtered metadata: %v", err)
					connector.Metadata = json.RawMessage("{}")
				} else {
					connector.Metadata = filteredMetadata
				}
			}
		} else {
			connector.Metadata = json.RawMessage("{}")
		}

		connectors = append(connectors, connector)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Error iterating over integration token rows: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Return empty array if no connectors found
	if connectors == nil {
		connectors = []ConnectorResponse{}
	}

	return c.JSON(http.StatusOK, connectors)
}

// GetConnector handles fetching a single git provider connection by ID
func (s *Server) GetConnector(c echo.Context) error {
	// Check if the user is authenticated
	password := c.Request().Header.Get("X-Admin-Password")
	if password == "" {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Authentication required",
		})
	}

	// Get the stored hashed password
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check authentication: " + err.Error(),
		})
	}

	// Verify the provided password
	if !comparePasswords(hashedPassword, password) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Invalid authentication",
		})
	}

	// Get connector ID from URL parameter
	connectorIDStr := c.Param("id")
	connectorID, err := strconv.Atoi(connectorIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid connector ID",
		})
	}

	// Query the database for the specific integration token
	var connector ConnectorResponse
	var metadataRaw []byte

	err = s.db.QueryRow(`
		SELECT id, provider, provider_app_id, connection_name, provider_url, metadata, created_at, updated_at
		FROM integration_tokens
		WHERE id = $1
	`, connectorID).Scan(
		&connector.ID,
		&connector.Provider,
		&connector.ProviderAppID,
		&connector.ConnectionName,
		&connector.ProviderURL,
		&metadataRaw,
		&connector.CreatedAt,
		&connector.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "Connector not found",
			})
		}
		log.Printf("Database error: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Parse the metadata JSON
	if metadataRaw != nil {
		var metadata map[string]interface{}
		if err := json.Unmarshal(metadataRaw, &metadata); err != nil {
			log.Printf("Failed to parse metadata JSON: %v", err)
			connector.Metadata = json.RawMessage("{}")
		} else {
			// Remove sensitive info like tokens
			delete(metadata, "access_token")
			delete(metadata, "refresh_token")

			// Re-marshal the filtered metadata
			filteredMetadata, err := json.Marshal(metadata)
			if err != nil {
				log.Printf("Failed to marshal filtered metadata: %v", err)
				connector.Metadata = json.RawMessage("{}")
			} else {
				connector.Metadata = filteredMetadata
			}
		}
	} else {
		connector.Metadata = json.RawMessage("{}")
	}

	return c.JSON(http.StatusOK, connector)
}

// DeleteConnector handles deletion of a git provider connection
func (s *Server) DeleteConnector(c echo.Context) error {
	id := c.Param("id")

	// Execute the delete query
	result, err := s.db.Exec(`
		DELETE FROM integration_tokens 
		WHERE id = $1
	`, id)

	if err != nil {
		log.Printf("Failed to delete connector with ID %s: %v", id, err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	// Check if any rows were affected
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Failed to get rows affected for connector deletion: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Database error: " + err.Error(),
		})
	}

	if rowsAffected == 0 {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error: "Connector not found",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Connector deleted successfully",
	})
}
