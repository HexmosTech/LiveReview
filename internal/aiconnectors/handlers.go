package aiconnectors

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// ValidateAPIKeyRequest represents the request for API key validation
type ValidateAPIKeyRequest struct {
	Provider Provider `json:"provider"`
	APIKey   string   `json:"api_key"`
	BaseURL  string   `json:"base_url,omitempty"`
}

// ValidateAPIKeyResponse represents the response for API key validation
type ValidateAPIKeyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// RegisterHandlers registers all aiconnectors API handlers to the given router
func RegisterHandlers(e *echo.Echo) {
	apiGroup := e.Group("/api/v1")

	// API key validation endpoint
	apiGroup.POST("/aiconnectors/validate-key", validateAPIKeyHandler)
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
		Str("api_key_prefix", req.APIKey[:min(5, len(req.APIKey))]+"...").
		Msg("Validating API key")

	// Validate the API key
	valid, err := ValidateAPIKey(context.Background(), req.Provider, req.APIKey, req.BaseURL)
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

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetProviderModels returns the available models for a provider
func GetProviderModels(provider Provider) []string {
	switch provider {
	case ProviderOpenAI:
		return []string{
			"gpt-3.5-turbo",
			"gpt-4",
			"gpt-4-turbo",
			"gpt-4o",
		}
	case ProviderGemini:
		return []string{
			"gemini-pro",
			"gemini-pro-vision",
			"gemini-ultra",
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
	case ProviderOpenAI:
		return "gpt-4"
	case ProviderGemini:
		return "gemini-pro"
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
