package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	Model    string `json:"model,omitempty"`
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

	// Log the validation attempt without exposing the API key
	log.Info().
		Str("provider", req.Provider).
		Msg("Validating AI provider API key")

	// Validate the API key
	valid, err := aiconnectors.ValidateAPIKey(
		context.Background(),
		aiconnectors.Provider(req.Provider),
		req.APIKey,
		req.BaseURL,
		req.Model,
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
	OrgID         int64  `json:"org_id"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	APIKeyPreview string `json:"api_key_preview"`
	BaseURL       string `json:"base_url,omitempty"`
	SelectedModel string `json:"selected_model,omitempty"`
	APIKey        string `json:"api_key,omitempty"` // Full API key for editing (only when requested)
}

// FetchOllamaModelsRequest represents the request for fetching Ollama models
type FetchOllamaModelsRequest struct {
	BaseURL  string `json:"base_url"`
	JWTToken string `json:"jwt_token,omitempty"`
}

// CreateAIConnector handles requests to create a new AI connector
func (s *Server) CreateAIConnector(c echo.Context) error {
	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
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

	ctx := c.Request().Context()

	// Get the current max display order and increment it
	maxOrder, err := storage.GetMaxDisplayOrder(ctx, orgID)
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

	// Use provided model or provider default
	selectedModel := req.SelectedModel
	if selectedModel == "" && req.ProviderName == string(aiconnectors.ProviderOpenRouter) {
		selectedModel = aiconnectors.GetDefaultModel(aiconnectors.ProviderOpenRouter)
	}

	// Create a connector record
	connector := &aiconnectors.ConnectorRecord{
		ProviderName:  req.ProviderName,
		ApiKey:        req.APIKey,
		ConnectorName: req.ConnectorName,
		BaseURL:       sql.NullString{String: req.BaseURL, Valid: req.BaseURL != ""},
		SelectedModel: sql.NullString{String: selectedModel, Valid: selectedModel != ""},
		DisplayOrder:  nextOrder, // Auto-assign next order
		OrgID:         orgID,
	}

	// Save the connector to the database
	if err := storage.CreateConnector(ctx, orgID, connector); err != nil {
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
		OrgID:         connector.OrgID,
		CreatedAt:     connector.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     connector.UpdatedAt.Format(time.RFC3339),
		APIKeyPreview: getMaskedKey(connector.ApiKey),
	})
}

// GetAIConnectors handles requests to get all AI connectors
func (s *Server) GetAIConnectors(c echo.Context) error {
	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
		})
	}

	// Create a storage instance
	storage := aiconnectors.NewStorage(s.db)

	ctx := c.Request().Context()

	// Get all connectors
	connectors, err := storage.GetAllConnectors(ctx, orgID)
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
			OrgID:         connector.OrgID,
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
	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
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
	ctx := c.Request().Context()

	existingConnector, err := storage.GetConnectorByID(ctx, orgID, connectorID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "Connector not found",
		})
	}

	// Update connector fields
	existingConnector.ProviderName = req.ProviderName
	existingConnector.ApiKey = req.APIKey
	existingConnector.ConnectorName = req.ConnectorName
	existingConnector.DisplayOrder = req.DisplayOrder
	existingConnector.OrgID = orgID

	// Update provider-specific fields if provided
	if req.BaseURL != "" {
		existingConnector.BaseURL = sql.NullString{String: req.BaseURL, Valid: true}
	}

	selectedModel := req.SelectedModel
	if selectedModel == "" && req.ProviderName == string(aiconnectors.ProviderOpenRouter) {
		selectedModel = aiconnectors.GetDefaultModel(aiconnectors.ProviderOpenRouter)
	}
	if selectedModel != "" {
		existingConnector.SelectedModel = sql.NullString{String: selectedModel, Valid: true}
	}

	// Save updated connector
	if err := storage.UpdateConnector(ctx, existingConnector); err != nil {
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
		OrgID:         existingConnector.OrgID,
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
	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
		})
	}

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
	ctx := c.Request().Context()

	if err := storage.UpdateDisplayOrders(ctx, orgID, updates); err != nil {
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
	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
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

	// Delete the connector
	ctx := c.Request().Context()

	if err := storage.DeleteConnector(ctx, orgID, connectorID); err != nil {
		log.Error().Err(err).Int64("id", connectorID).Msg("Failed to delete connector")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete connector: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Connector deleted successfully",
	})
}

// FetchOllamaModels handles requests to fetch available models from an Ollama instance
func (s *Server) FetchOllamaModels(c echo.Context) error {
	orgIDVal := c.Get("org_id")
	if _, ok := orgIDVal.(int64); !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required",
		})
	}

	var req FetchOllamaModelsRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	if strings.TrimSpace(req.BaseURL) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Base URL is required",
		})
	}

	log.Info().
		Str("base_url", req.BaseURL).
		Msg("Fetching models from Ollama instance")

	ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
	defer cancel()

	models, err := aiconnectors.FetchOllamaModels(ctx, req.BaseURL, req.JWTToken)
	if err != nil {
		log.Error().Err(err).
			Str("base_url", req.BaseURL).
			Msg("Failed to fetch models from Ollama")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Failed to fetch models: %v", err),
		})
	}

	var modelNames []string
	for _, model := range models {
		modelNames = append(modelNames, model.Name)
	}

	log.Info().
		Str("base_url", req.BaseURL).
		Int("model_count", len(modelNames)).
		Msg("Successfully fetched Ollama models")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"models": modelNames,
		"count":  len(modelNames),
	})
}
