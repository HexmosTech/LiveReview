package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// ConnectorResponse represents a single connector record returned by the API
type ConnectorResponse struct {
	ID             int64           `json:"id"`
	Provider       string          `json:"provider"`
	ProviderAppID  string          `json:"provider_app_id"`
	ConnectionName string          `json:"connection_name"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
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
		SELECT id, provider, provider_app_id, connection_name, metadata, created_at, updated_at
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
