package api

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/pkg/models"
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

// Onboard performs the user onboarding flow by validating an onboarding API key,
// revoking it, generating a new persistent API key, minting session tokens,
// and returning the details to the client.
func (s *Server) Onboard(c echo.Context) error {
	apiKey := c.Request().Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = c.QueryParam("api_key")
	}
	if apiKey == "" {
		// Try to read from JSON body
		var req struct {
			APIKey string `json:"api_key"`
		}
		if err := c.Bind(&req); err == nil && req.APIKey != "" {
			apiKey = req.APIKey
		}
	}

	if apiKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "API key required",
		})
	}

	manager := NewAPIKeyManager(s.db)
	keyRecord, _, err := manager.ValidateAPIKey(apiKey)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Invalid or expired onboarding API key",
		})
	}

	// Fetch user details
	user := &models.User{}
	var firstName, lastName *string
	err = s.db.QueryRowContext(c.Request().Context(), `
		SELECT id, email, password_hash, first_name, last_name, is_active, last_login_at, created_at, updated_at, created_by_user_id, password_reset_required, default_org_id
		FROM users WHERE id = $1
	`, keyRecord.UserID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &firstName, &lastName,
		&user.IsActive, &user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt,
		&user.CreatedByUserID, &user.PasswordResetRequired, &user.DefaultOrgID,
	)
	if err != nil {
		log.Printf("Onboard: failed to query user %d: %v", keyRecord.UserID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve user details",
		})
	}
	user.FirstName = firstName
	user.LastName = lastName

	// Fetch organization name
	var orgName string
	err = s.db.QueryRowContext(c.Request().Context(), `
		SELECT name FROM orgs WHERE id = $1
	`, keyRecord.OrgID).Scan(&orgName)
	if err != nil {
		log.Printf("Onboard: failed to query organization %d: %v", keyRecord.OrgID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve organization details",
		})
	}

	// Generate a new persistent API Key
	_, newKey, err := manager.CreateAPIKey(keyRecord.UserID, keyRecord.OrgID, "LRC CLI Key", []string{}, nil)
	if err != nil {
		log.Printf("Onboard: failed to generate persistent API key: %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to generate persistent API key",
		})
	}

	// Revoke the old onboarding API Key
	err = manager.RevokeAPIKey(keyRecord.ID, keyRecord.UserID, keyRecord.OrgID)
	if err != nil {
		log.Printf("Onboard: failed to revoke onboarding API key %d: %v", keyRecord.ID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to revoke onboarding API key",
		})
	}

	// Clear the onboarding_api_key in users table
	_, err = s.db.ExecContext(c.Request().Context(), `
		UPDATE users SET onboarding_api_key = NULL WHERE id = $1
	`, keyRecord.UserID)
	if err != nil {
		log.Printf("Onboard: failed to clear onboarding_api_key for user %d: %v", keyRecord.UserID, err)
	}

	// Generate JWT and refresh token
	userAgent := c.Request().UserAgent()
	ipAddress := c.RealIP()
	tokenPair, err := s.tokenService.CreateTokenPairWithOrg(user, keyRecord.OrgID, userAgent, ipAddress)
	if err != nil {
		log.Printf("Onboard: failed to create token pair for user %d: %v", keyRecord.UserID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to generate session tokens",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"api_key":       newKey,
		"org_id":        keyRecord.OrgID,
		"org_name":      orgName,
		"jwt":           tokenPair.AccessToken,
		"refresh_token": tokenPair.RefreshToken,
	})
}
