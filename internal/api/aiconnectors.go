package api

import (
	"context"
	"net/http"

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
