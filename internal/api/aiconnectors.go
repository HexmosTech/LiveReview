package api

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/rs/zerolog/log"
)

// AIConnectorKeyValidationRequest represents the request for API key validation
type AIConnectorKeyValidationRequest struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url,omitempty"`
}

// AIConnectorKeyValidationResponse represents the response for API key validation
type AIConnectorKeyValidationResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// ValidateAIConnectorKey handles requests to validate an AI provider API key
func (s *Server) ValidateAIConnectorKey(c echo.Context) error {
	var req AIConnectorKeyValidationRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, AIConnectorKeyValidationResponse{
			Valid:   false,
			Message: "Invalid request body",
		})
	}

	// Validate required fields
	if req.Provider == "" {
		return c.JSON(http.StatusBadRequest, AIConnectorKeyValidationResponse{
			Valid:   false,
			Message: "Provider is required",
		})
	}

	if req.APIKey == "" {
		return c.JSON(http.StatusBadRequest, AIConnectorKeyValidationResponse{
			Valid:   false,
			Message: "API key is required",
		})
	}

	// Log the validation attempt
	log.Info().
		Str("provider", req.Provider).
		Str("api_key_prefix", getMaskedKey(req.APIKey)).
		Msg("Validating AI provider API key")

	// Validate the API key
	valid, err := aiconnectors.ValidateAPIKey(
		context.Background(),
		aiconnectors.Provider(req.Provider),
		req.APIKey,
		req.BaseURL,
	)

	if err != nil {
		log.Error().Err(err).Msg("Error validating API key")
		return c.JSON(http.StatusInternalServerError, AIConnectorKeyValidationResponse{
			Valid:   false,
			Message: "Error validating API key: " + err.Error(),
		})
	}

	message := "API key is valid"
	if !valid {
		message = "API key is invalid"
	}

	return c.JSON(http.StatusOK, AIConnectorKeyValidationResponse{
		Valid:   valid,
		Message: message,
	})
}

// Helper function to mask API key for logging
func getMaskedKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

// AIConnectorCreateRequest represents the request for creating an AI connector
type AIConnectorCreateRequest struct {
	ProviderName  string `json:"provider_name"`
	APIKey        string `json:"api_key"`
	ConnectorName string `json:"connector_name"`
	BaseURL       string `json:"base_url,omitempty"`
	DisplayOrder  int    `json:"display_order"`
}

// AIConnectorResponse represents the response for AI connector operations
type AIConnectorResponse struct {
	ID            int64  `json:"id"`
	ProviderName  string `json:"provider_name"`
	ConnectorName string `json:"connector_name"`
	DisplayOrder  int    `json:"display_order"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	APIKeyPreview string `json:"api_key_preview"`
}

// CreateAIConnector handles requests to create a new AI connector
func (s *Server) CreateAIConnector(c echo.Context) error {
	var req AIConnectorCreateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.ProviderName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Provider name is required",
		})
	}

	if req.APIKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "API key is required",
		})
	}

	if req.ConnectorName == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Connector name is required",
		})
	}

	// Create a storage instance
	storage := aiconnectors.NewStorage(s.db)

	// Create a connector record
	connector := &aiconnectors.ConnectorRecord{
		ProviderName:  req.ProviderName,
		ApiKey:        req.APIKey,
		ConnectorName: req.ConnectorName,
		DisplayOrder:  req.DisplayOrder,
	}

	// Save the connector to the database
	if err := storage.CreateConnector(context.Background(), connector); err != nil {
		log.Error().Err(err).Msg("Failed to create connector")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create connector: " + err.Error(),
		})
	}

	// Return the created connector
	return c.JSON(http.StatusCreated, AIConnectorResponse{
		ID:            connector.ID,
		ProviderName:  connector.ProviderName,
		ConnectorName: connector.ConnectorName,
		DisplayOrder:  connector.DisplayOrder,
		CreatedAt:     connector.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     connector.UpdatedAt.Format(time.RFC3339),
		APIKeyPreview: getMaskedKey(connector.ApiKey),
	})
}

// GetAIConnectors handles requests to get all AI connectors
func (s *Server) GetAIConnectors(c echo.Context) error {
	// Create a storage instance
	storage := aiconnectors.NewStorage(s.db)

	// Get all connectors
	connectors, err := storage.GetAllConnectors(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get connectors")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get connectors: " + err.Error(),
		})
	}

	// Convert to response format
	var response []AIConnectorResponse
	for _, connector := range connectors {
		response = append(response, AIConnectorResponse{
			ID:            connector.ID,
			ProviderName:  connector.ProviderName,
			ConnectorName: connector.ConnectorName,
			DisplayOrder:  connector.DisplayOrder,
			CreatedAt:     connector.CreatedAt.Format(time.RFC3339),
			UpdatedAt:     connector.UpdatedAt.Format(time.RFC3339),
			APIKeyPreview: getMaskedKey(connector.ApiKey),
		})
	}

	return c.JSON(http.StatusOK, response)
}
