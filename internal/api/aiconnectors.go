package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
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

	// API key is optional for Ollama, required for other providers
	if req.APIKey == "" && req.Provider != "ollama" {
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
	SelectedModel string `json:"selected_model,omitempty"`
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
	BaseURL       string `json:"base_url,omitempty"`
	SelectedModel string `json:"selected_model,omitempty"`
	APIKey        string `json:"api_key,omitempty"` // Full API key for editing (only when requested)
}

// CreateAIConnector handles requests to create a new AI connector
func (s *Server) CreateAIConnector(c echo.Context) error {
	// Get org_id from context
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Organization ID not found in context",
		})
	}

	log.Info().Int64("org_id", orgID).Msg("CreateAIConnector: Retrieved org_id from context")

	// Verify organization exists
	var orgExists bool
	err := s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM orgs WHERE id = $1)", orgID).Scan(&orgExists)
	if err != nil {
		log.Error().Err(err).Int64("org_id", orgID).Msg("Failed to check if organization exists")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to verify organization: " + err.Error(),
		})
	}

	if !orgExists {
		log.Error().Int64("org_id", orgID).Msg("Organization does not exist in database")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Organization with ID %d does not exist. Please contact support.", orgID),
		})
	}

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

	// API key is optional for Ollama, required for other providers
	if req.APIKey == "" && req.ProviderName != "ollama" {
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

	// Get the current max display order and increment it
	maxOrder, err := storage.GetMaxDisplayOrder(context.Background())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get max display order")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get max display order: " + err.Error(),
		})
	}

	nextOrder := maxOrder + 1
	log.Debug().
		Int("max_order", maxOrder).
		Int("next_order", nextOrder).
		Int("req_display_order", req.DisplayOrder).
		Msg("Auto-incrementing display order for new connector")

	// Create a connector record
	connector := &aiconnectors.ConnectorRecord{
		ProviderName:  req.ProviderName,
		ApiKey:        req.APIKey,
		ConnectorName: req.ConnectorName,
		BaseURL:       sql.NullString{String: req.BaseURL, Valid: req.BaseURL != ""},
		SelectedModel: sql.NullString{String: req.SelectedModel, Valid: req.SelectedModel != ""},
		DisplayOrder:  nextOrder, // Auto-assign next order
		OrgID:         orgID,     // Set organization ID
	}

	log.Info().
		Int64("org_id", orgID).
		Str("provider_name", req.ProviderName).
		Str("connector_name", req.ConnectorName).
		Int("display_order", nextOrder).
		Msg("Creating AI connector with org_id")

	// Save the connector to the database
	if err := storage.CreateConnector(context.Background(), connector); err != nil {
		log.Error().Err(err).Int64("org_id", orgID).Msg("Failed to create connector")
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
	// Get org_id from context
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Organization ID not found in context",
		})
	}

	// Create a storage instance
	storage := aiconnectors.NewStorage(s.db)

	// Get all connectors for this organization
	connectors, err := storage.GetAllConnectors(context.Background(), orgID)
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
			BaseURL:       connector.GetBaseURL(),
			SelectedModel: connector.GetSelectedModel(),
			APIKey:        connector.ApiKey, // Include full API key for editing
		})
	}

	return c.JSON(http.StatusOK, response)
}

// UpdateAIConnector handles requests to update an existing AI connector
func (s *Server) UpdateAIConnector(c echo.Context) error {
	// Get org_id from context
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Organization ID not found in context",
		})
	}

	// Get connector ID from URL parameter
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Connector ID is required",
		})
	}

	// Parse connector ID
	connectorID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid connector ID",
		})
	}

	// Parse request body
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

	// API key is optional for Ollama, required for other providers
	if req.APIKey == "" && req.ProviderName != "ollama" {
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

	// Get existing connector to update
	existingConnector, err := storage.GetConnectorByID(context.Background(), connectorID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Connector not found",
		})
	}

	// Verify connector belongs to this organization
	if existingConnector.OrgID != orgID {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Access denied: connector belongs to different organization",
		})
	}

	// Update connector fields
	existingConnector.ProviderName = req.ProviderName
	existingConnector.ApiKey = req.APIKey
	existingConnector.ConnectorName = req.ConnectorName
	existingConnector.DisplayOrder = req.DisplayOrder

	// Update Ollama-specific fields if provided
	if req.BaseURL != "" {
		existingConnector.BaseURL = sql.NullString{String: req.BaseURL, Valid: true}
	}
	if req.SelectedModel != "" {
		existingConnector.SelectedModel = sql.NullString{String: req.SelectedModel, Valid: true}
	}

	// Save updated connector
	if err := storage.UpdateConnector(context.Background(), existingConnector); err != nil {
		log.Error().Err(err).Msg("Failed to update connector")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update connector: " + err.Error(),
		})
	}

	// Return success response
	return c.JSON(http.StatusOK, AIConnectorResponse{
		ID:            existingConnector.ID,
		ProviderName:  existingConnector.ProviderName,
		ConnectorName: existingConnector.ConnectorName,
		DisplayOrder:  existingConnector.DisplayOrder,
		CreatedAt:     existingConnector.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     existingConnector.UpdatedAt.Format(time.RFC3339),
		APIKeyPreview: getMaskedKey(existingConnector.ApiKey),
		BaseURL:       existingConnector.GetBaseURL(),
		SelectedModel: existingConnector.GetSelectedModel(),
		APIKey:        existingConnector.ApiKey,
	})
}

// ReorderAIConnectors handles requests to reorder AI connectors
func (s *Server) ReorderAIConnectors(c echo.Context) error {
	// Parse request body
	var updates []aiconnectors.DisplayOrderUpdate
	if err := c.Bind(&updates); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	if len(updates) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "No updates provided",
		})
	}

	// Create a storage instance
	storage := aiconnectors.NewStorage(s.db)

	// Update display orders
	if err := storage.UpdateDisplayOrders(context.Background(), updates); err != nil {
		log.Error().Err(err).Msg("Failed to update display orders")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update display orders: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Display orders updated successfully",
	})
}

// DeleteAIConnector handles requests to delete an AI connector by ID
func (s *Server) DeleteAIConnector(c echo.Context) error {
	// Get org_id from context
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Organization ID not found in context",
		})
	}

	// Get connector ID from URL parameter
	id := c.Param("id")
	if id == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Connector ID is required",
		})
	}

	// Convert ID string to int64
	connectorID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid connector ID format",
		})
	}

	// Create a storage instance
	storage := aiconnectors.NewStorage(s.db)

	// Delete the connector (will only delete if it belongs to this org)
	if err := storage.DeleteConnector(context.Background(), connectorID, orgID); err != nil {
		log.Error().Err(err).Int64("id", connectorID).Msg("Failed to delete connector")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete connector: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Connector deleted successfully",
	})
}
