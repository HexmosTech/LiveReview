package users

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

// ProfileHandlers contains the profile management handler methods
type ProfileHandlers struct {
	profileService *ProfileService
}

// NewProfileHandlers creates a new profile handlers instance
func NewProfileHandlers(profileService *ProfileService) *ProfileHandlers {
	return &ProfileHandlers{
		profileService: profileService,
	}
}

// GetProfile handles getting the current user's profile
func (ph *ProfileHandlers) GetProfile(c echo.Context) error {
	// Get user from auth context
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Get user profile
	profile, err := ph.profileService.GetUserProfile(user.ID)
	if err != nil {
		if err.Error() == "user not found" {
			return echo.NewHTTPError(http.StatusNotFound, "User profile not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user profile")
	}

	// Get user organizations
	organizations, err := ph.profileService.GetUserOrganizations(user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user organizations")
	}

	response := map[string]interface{}{
		"profile":       profile,
		"organizations": organizations,
	}

	return c.JSON(http.StatusOK, response)
}

// UpdateProfile handles updating the current user's profile
func (ph *ProfileHandlers) UpdateProfile(c echo.Context) error {
	// Get user from auth context
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Parse request
	var req UpdateProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Update profile
	profile, err := ph.profileService.UpdateUserProfile(user.ID, req)
	if err != nil {
		if err.Error() == "user not found" {
			return echo.NewHTTPError(http.StatusNotFound, "User profile not found")
		}
		if err.Error() != "" && err.Error()[:5] == "email" {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update user profile")
	}

	return c.JSON(http.StatusOK, profile)
}

// ChangePassword handles changing the current user's password
func (ph *ProfileHandlers) ChangePassword(c echo.Context) error {
	// Get user from auth context
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Parse request
	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Validate password requirements
	if len(req.NewPassword) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "New password must be at least 8 characters long")
	}

	// Change password
	err := ph.profileService.ChangePassword(user.ID, req)
	if err != nil {
		if err.Error() == "user not found" {
			return echo.NewHTTPError(http.StatusNotFound, "User not found")
		}
		if err.Error() == "current password is incorrect" {
			return echo.NewHTTPError(http.StatusUnauthorized, "Current password is incorrect")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to change password")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Password changed successfully. Please log in again with your new password.",
	})
}
