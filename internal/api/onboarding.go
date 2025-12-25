package api

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ClearOnboardingAPIKey clears the onboarding API key for a user
func (s *Server) ClearOnboardingAPIKey(c echo.Context) error {
	userIDVal := c.Get("user_id")
	userID, ok := userIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "User context required",
		})
	}

	query := `UPDATE users SET onboarding_api_key = NULL WHERE id = $1`
	_, err := s.db.Exec(query, userID)
	if err != nil {
		log.Printf("Error clearing onboarding API key for user %d: %v", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to clear onboarding API key",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Onboarding API key cleared successfully",
	})
}

// TrackCLIUsage updates the last_cli_used_at timestamp for a user
func (s *Server) TrackCLIUsage(c echo.Context) error {
	// This endpoint uses API key authentication middleware which sets user_id in context
	userIDVal := c.Get("user_id")
	if userIDVal == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "API key required",
		})
	}

	userID, ok := userIDVal.(int64)
	if !ok {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Invalid user context",
		})
	}

	// Update last CLI used timestamp
	query := `UPDATE users SET last_cli_used_at = NOW() WHERE id = $1`
	_, err := s.db.Exec(query, userID)
	if err != nil {
		log.Printf("Error updating CLI usage for user %d: %v", userID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to track CLI usage",
		})
	}

	log.Printf("Successfully tracked CLI usage for user %d", userID)
	return c.JSON(http.StatusOK, map[string]string{
		"message": "CLI usage tracked successfully",
	})
}
