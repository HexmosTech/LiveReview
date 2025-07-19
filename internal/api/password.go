package api

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
)

// PasswordRequest represents a request to set or reset admin password
type PasswordRequest struct {
	Password    string `json:"password"`
	OldPassword string `json:"old_password,omitempty"`
	Force       bool   `json:"force,omitempty"`
}

// PasswordVerifyRequest represents a request to verify an admin password
type PasswordVerifyRequest struct {
	Password string `json:"password"`
}

// PasswordResponse is the response for password operations
type PasswordResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// PasswordVerifyResponse is the response for password verification
type PasswordVerifyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// PasswordStatusResponse is the response for checking if a password is set
type PasswordStatusResponse struct {
	IsSet   bool   `json:"is_set"`
	Message string `json:"message"`
}

// ErrorResponse is a standard error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// hashPassword securely hashes a password using bcrypt
func hashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// comparePasswords checks if the provided password matches the hashed password
func comparePasswords(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// SetAdminPassword sets the admin password for the instance
// If an admin password already exists, it returns an error unless Force is true
func (s *Server) SetAdminPassword(c echo.Context) error {
	var req PasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate password
	if len(req.Password) < 8 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Password must be at least 8 characters long",
		})
	}

	// Check if admin password already exists (only if not forcing)
	if !req.Force {
		isSet, err := s.CheckAdminPasswordStatusDirectly()
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "Failed to check password status: " + err.Error(),
			})
		}

		if isSet {
			return c.JSON(http.StatusConflict, ErrorResponse{
				Error: "Admin password already set. Use reset endpoint to change it, or set force=true to override.",
			})
		}
	}

	// Hash the password
	hashedPassword, err := hashPassword(req.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to hash password",
		})
	}

	// Check if instance_details record exists
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM instance_details").Scan(&count)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check instance details: " + err.Error(),
		})
	}

	if count == 0 {
		// Insert a new record if none exists
		_, err = s.db.Exec(`
			INSERT INTO instance_details (livereview_prod_url, admin_password) 
			VALUES ('localhost', $1)
		`, hashedPassword)
	} else {
		// Update existing record
		_, err = s.db.Exec(`
			UPDATE instance_details 
			SET admin_password = $1, updated_at = CURRENT_TIMESTAMP 
			WHERE id = (SELECT id FROM instance_details LIMIT 1)
		`, hashedPassword)
	}

	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to set admin password: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, PasswordResponse{
		Success: true,
		Message: "Admin password has been set successfully",
	})
}

// ResetAdminPassword resets the admin password
// Requires the old password for verification and a new password
func (s *Server) ResetAdminPassword(c echo.Context) error {
	var req PasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Validate new password is provided
	if req.Password == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "New password is required",
		})
	}

	// Validate new password length
	if len(req.Password) < 8 {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "New password must be at least 8 characters long",
		})
	}

	// Validate old password is provided
	if req.OldPassword == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Old password is required",
		})
	}

	// Get the current hashed password
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, ErrorResponse{
				Error: "No admin password set. Use set endpoint to create one.",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to retrieve current password",
		})
	}

	// Verify old password
	if !comparePasswords(hashedPassword, req.OldPassword) {
		return c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error: "Old password is incorrect",
		})
	}

	// Hash the new password
	newHashedPassword, err := hashPassword(req.Password)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to hash new password",
		})
	}

	// Update the password
	_, err = s.db.Exec(`
		UPDATE instance_details 
		SET admin_password = $1, updated_at = CURRENT_TIMESTAMP 
		WHERE id = (SELECT id FROM instance_details LIMIT 1)
	`, newHashedPassword)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to update admin password",
		})
	}

	return c.JSON(http.StatusOK, PasswordResponse{
		Success: true,
		Message: "Admin password has been updated successfully",
	})
}

// SetAdminPasswordDirectly sets the admin password directly (for CLI use)
func (s *Server) SetAdminPasswordDirectly(password string, force bool) error {
	// Validate password
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	// Check if admin password already exists (only if not forcing)
	if !force {
		isSet, err := s.CheckAdminPasswordStatusDirectly()
		if err != nil {
			return fmt.Errorf("failed to check password status: %v", err)
		}

		if isSet {
			return fmt.Errorf("admin password already set; use --reset-admin-password-* to change it, or use --force to override")
		}
	}

	// Hash the password
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %v", err)
	}

	// Check if instance_details record exists
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM instance_details").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check instance details: %v", err)
	}

	var result sql.Result
	if count == 0 {
		// Insert a new record
		result, err = s.db.Exec(`
			INSERT INTO instance_details (livereview_prod_url, admin_password) 
			VALUES ('localhost', $1)
		`, hashedPassword)
	} else {
		// Update existing record
		result, err = s.db.Exec(`
			UPDATE instance_details 
			SET admin_password = $1, updated_at = CURRENT_TIMESTAMP 
			WHERE id = (SELECT id FROM instance_details LIMIT 1)
		`, hashedPassword)
	}

	if err != nil {
		return fmt.Errorf("failed to set admin password: %v", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no records were updated")
	}

	return nil
}

// ResetAdminPasswordDirectly resets the admin password directly (for CLI use)
func (s *Server) ResetAdminPasswordDirectly(oldPassword, newPassword string) error {
	// Validate passwords
	if len(newPassword) < 8 {
		return fmt.Errorf("new password must be at least 8 characters long")
	}

	if oldPassword == "" {
		return fmt.Errorf("old password is required")
	}

	// Get the current hashed password
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no admin password set. Use --set-admin-password to create one")
		}
		return fmt.Errorf("failed to retrieve current password: %v", err)
	}

	// Verify old password
	if !comparePasswords(hashedPassword, oldPassword) {
		return fmt.Errorf("old password is incorrect")
	}

	// Hash the new password
	newHashedPassword, err := hashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %v", err)
	}

	// Update the password
	_, err = s.db.Exec(`
		UPDATE instance_details 
		SET admin_password = $1, updated_at = CURRENT_TIMESTAMP 
		WHERE id = (SELECT id FROM instance_details LIMIT 1)
	`, newHashedPassword)
	if err != nil {
		return fmt.Errorf("failed to update admin password: %v", err)
	}

	return nil
}

// VerifyAdminPassword verifies if the provided password matches the stored admin password
func (s *Server) VerifyAdminPassword(c echo.Context) error {
	var req PasswordVerifyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format",
		})
	}

	// Check if password is provided
	if req.Password == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Password is required",
		})
	}

	// Check if admin password is set
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, PasswordVerifyResponse{
				Valid:   false,
				Message: "No admin password has been set yet",
			})
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to retrieve current password",
		})
	}

	// Verify password
	isValid := comparePasswords(hashedPassword, req.Password)

	if isValid {
		return c.JSON(http.StatusOK, PasswordVerifyResponse{
			Valid:   true,
			Message: "Password is valid",
		})
	} else {
		return c.JSON(http.StatusUnauthorized, PasswordVerifyResponse{
			Valid:   false,
			Message: "Password is invalid",
		})
	}
}

// VerifyAdminPasswordDirectly verifies admin password directly (for CLI use)
func (s *Server) VerifyAdminPasswordDirectly(password string) (bool, error) {
	// Check if password is provided
	if password == "" {
		return false, fmt.Errorf("password is required")
	}

	// Check if admin password is set
	var hashedPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&hashedPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("no admin password has been set yet")
		}
		return false, fmt.Errorf("failed to retrieve current password: %v", err)
	}

	// Verify password
	isValid := comparePasswords(hashedPassword, password)

	if !isValid {
		return false, fmt.Errorf("password is invalid")
	}

	return true, nil
}

// CheckAdminPasswordStatus checks if an admin password has been set
func (s *Server) CheckAdminPasswordStatus(c echo.Context) error {
	// Check if admin password is set in the database
	var adminPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&adminPassword)

	if err != nil {
		if err == sql.ErrNoRows {
			// No instance details record exists
			return c.JSON(http.StatusOK, PasswordStatusResponse{
				IsSet:   false,
				Message: "No admin password has been set yet",
			})
		}
		// Other database error
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to check password status: " + err.Error(),
		})
	}

	// Check if the password field is empty
	isSet := adminPassword != ""

	if isSet {
		return c.JSON(http.StatusOK, PasswordStatusResponse{
			IsSet:   true,
			Message: "Admin password is set",
		})
	} else {
		return c.JSON(http.StatusOK, PasswordStatusResponse{
			IsSet:   false,
			Message: "No admin password has been set yet",
		})
	}
}

// CheckAdminPasswordStatusDirectly checks if admin password is set (for CLI use)
func (s *Server) CheckAdminPasswordStatusDirectly() (bool, error) {
	// Check if admin password is set
	var adminPassword string
	err := s.db.QueryRow("SELECT admin_password FROM instance_details LIMIT 1").Scan(&adminPassword)

	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // No error, just not set
		}
		return false, fmt.Errorf("failed to check password status: %v", err)
	}

	// Check if the password field is empty
	isSet := adminPassword != ""

	return isSet, nil
}
