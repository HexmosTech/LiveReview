package aiconnectors

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// ValidateAPIKeyRequest represents the request for API key validation
type ValidateAPIKeyRequest struct {
	Provider Provider `json:"provider"`
	APIKey   string   `json:"api_key"`
	BaseURL  string   `json:"base_url,omitempty"`
	Model    string   `json:"model,omitempty"`
}

// ValidateAPIKeyResponse represents the response for API key validation
type ValidateAPIKeyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// CreateConnectorRequest represents the request for creating a connector
type CreateConnectorRequest struct {
	ProviderName  string `json:"provider_name"`
	APIKey        string `json:"api_key"`
	ConnectorName string `json:"connector_name"`
	BaseURL       string `json:"base_url,omitempty"`
	SelectedModel string `json:"selected_model,omitempty"`
	DisplayOrder  int    `json:"display_order"`
}

// CreateConnectorResponse represents the response for creating a connector
type CreateConnectorResponse struct {
	ID            int64     `json:"id"`
	ProviderName  string    `json:"provider_name"`
	ConnectorName string    `json:"connector_name"`
	DisplayOrder  int       `json:"display_order"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// RegisterHandlers registers all aiconnectors API handlers to the given router
func RegisterHandlers(e *echo.Echo) {
	e.POST("/api/v1/aiconnectors/validate-key", validateAPIKeyHandler)
	e.POST("/api/v1/aiconnectors", createConnectorHandler)
	e.POST("/api/v1/aiconnectors/ollama/models", fetchOllamaModelsHandler)
}

// ValidateAPIKeyHandler handles requests to validate an API key
func validateAPIKeyHandler(c echo.Context) error {
	var req ValidateAPIKeyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ValidateAPIKeyResponse{
			Valid:   false,
			Message: "Invalid request body",
		})
	}

	if req.Provider == "" {
		return c.JSON(http.StatusBadRequest, ValidateAPIKeyResponse{
			Valid:   false,
			Message: "Provider is required",
		})
	}

	if req.APIKey == "" {
		return c.JSON(http.StatusBadRequest, ValidateAPIKeyResponse{
			Valid:   false,
			Message: "API key is required",
		})
	}

	// Log the validation attempt
	log.Info().
		Str("provider", string(req.Provider)).
		Str("api_key_masked", maskAPIKey(req.APIKey)).
		Msg("Validating API key")

	// Validate the API key
	valid, err := ValidateAPIKey(context.Background(), req.Provider, req.APIKey, req.BaseURL, req.Model)
	if err != nil {
		log.Error().Err(err).Msg("Error validating API key")
		return c.JSON(http.StatusInternalServerError, ValidateAPIKeyResponse{
			Valid:   false,
			Message: fmt.Sprintf("Error validating API key: %v", err),
		})
	}

	message := "API key is valid"
	if !valid {
		message = "API key is invalid"
	}

	return c.JSON(http.StatusOK, ValidateAPIKeyResponse{
		Valid:   valid,
		Message: message,
	})
}

// createConnectorHandler handles requests to create a new AI connector
func createConnectorHandler(c echo.Context) error {
	db := c.Get("db").(*sql.DB)
	if db == nil {
		log.Error().Msg("Database connection not available")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Database connection not available",
		})
	}

	orgIDVal := c.Get("org_id")
	orgID, ok := orgIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Organization context required (create connector handler)",
		})
	}

	var req CreateConnectorRequest
	if err := c.Bind(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind create connector request")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	log.Info().
		Str("provider_name", req.ProviderName).
		Str("connector_name", req.ConnectorName).
		Str("base_url", req.BaseURL).
		Str("selected_model", req.SelectedModel).
		Msg("Creating AI connector")

	storage := NewStorage(db)

	// Create a connector record
	connector := &ConnectorRecord{
		ProviderName:  req.ProviderName,
		ApiKey:        req.APIKey,
		ConnectorName: req.ConnectorName,
		BaseURL:       sql.NullString{String: req.BaseURL, Valid: req.BaseURL != ""},
		SelectedModel: sql.NullString{String: req.SelectedModel, Valid: req.SelectedModel != ""},
		DisplayOrder:  req.DisplayOrder,
		OrgID:         orgID,
	}

	// Save the connector to the database
	if err := storage.CreateConnector(context.Background(), orgID, connector); err != nil {
		log.Error().Err(err).Msg("Failed to create connector")
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("Failed to create connector: %v", err),
		})
	}

	// Return the created connector
	return c.JSON(http.StatusCreated, CreateConnectorResponse{
		ID:            connector.ID,
		ProviderName:  connector.ProviderName,
		ConnectorName: connector.ConnectorName,
		DisplayOrder:  connector.DisplayOrder,
		CreatedAt:     connector.CreatedAt,
		UpdatedAt:     connector.UpdatedAt,
	})
}

// FetchOllamaModelsRequest represents the request for fetching Ollama models
type FetchOllamaModelsRequest struct {
	BaseURL  string `json:"base_url"`
	JWTToken string `json:"jwt_token,omitempty"`
}

// fetchOllamaModelsHandler handles requests to fetch models from an Ollama instance
func fetchOllamaModelsHandler(c echo.Context) error {
	var req FetchOllamaModelsRequest
	if err := c.Bind(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind fetch Ollama models request")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Validate required fields
	if req.BaseURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Base URL is required",
		})
	}

	log.Info().
		Str("base_url", req.BaseURL).
		Msg("Fetching models from Ollama instance")

	// Fetch models from Ollama
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	models, err := FetchOllamaModels(ctx, req.BaseURL, req.JWTToken)
	if err != nil {
		log.Error().Err(err).
			Str("base_url", req.BaseURL).
			Msg("Failed to fetch models from Ollama")
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("Failed to fetch models: %v", err),
		})
	}

	// Transform models for frontend response
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

// GetProviderModels returns the available models for a provider
func GetProviderModels(provider Provider) []string {
	switch provider {
	case ProviderOpenRouter:
		return []string{
			"deepseek/deepseek-r1-0528:free",
		}
	case ProviderOpenAI:
		return []string{
			"gpt-3.5-turbo",
			"gpt-4",
			"gpt-4-turbo",
			"gpt-4o",
		}
	case ProviderGemini:
		return []string{
			"gemini-2.5-flash",
			"gemini-2.5-flash-lite",
			"gemini-2.5-pro",
			"gemini-2.0-flash",
			"gemini-2.0-flash-lite",
		}
	case ProviderClaude:
		return []string{
			"claude-3-opus-20240229",
			"claude-3-sonnet-20240229",
			"claude-3-haiku-20240307",
		}
	case ProviderCohere:
		return []string{
			"command",
			"command-light",
			"command-r",
			"command-r-plus",
		}
	case ProviderOllama:
		return []string{
			"llama3",
			"mistral",
			"codellama",
			"neural-chat",
		}
	default:
		return []string{}
	}
}

// GetDefaultModel returns the default model for a provider
func GetDefaultModel(provider Provider) string {
	switch provider {
	case ProviderOpenRouter:
		return "deepseek/deepseek-r1-0528:free"
	case ProviderOpenAI:
		return "gpt-4"
	case ProviderGemini:
		return "gemini-2.5-flash"
	case ProviderClaude:
		return "claude-3-sonnet-20240229"
	case ProviderCohere:
		return "command-r"
	case ProviderOllama:
		return "llama3"
	default:
		return ""
	}
}
