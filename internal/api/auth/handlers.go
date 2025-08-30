package auth

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandlers contains the authentication handler methods
type AuthHandlers struct {
	tokenService *TokenService
	db           *sql.DB
}

// NewAuthHandlers creates a new authentication handlers instance
func NewAuthHandlers(tokenService *TokenService, db *sql.DB) *AuthHandlers {
	return &AuthHandlers{
		tokenService: tokenService,
		db:           db,
	}
}

// LoginRequest represents the login request body
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse represents the login response
type LoginResponse struct {
	User          *UserInfo  `json:"user"`
	TokenPair     *TokenPair `json:"tokens"`
	Organizations []OrgInfo  `json:"organizations"`
}

// UserInfo represents basic user information (no sensitive data)
type UserInfo struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// OrgInfo represents organization information for the user
type OrgInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"` // super_admin, owner, member
}

// RefreshRequest represents the token refresh request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// ChangePasswordRequest represents change password request (for temp passwords)
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

// SetupAdminRequest represents initial admin setup request
type SetupAdminRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8"`
	OrgName  string `json:"org_name" validate:"required"`
}

// Login handles user authentication with email/password
func (h *AuthHandlers) Login(c echo.Context) error {
	var req LoginRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Get user by email
	user := &models.User{}
	err := h.db.QueryRow(`
		SELECT id, email, password_hash, created_at, updated_at
		FROM users WHERE email = $1
	`, req.Email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Invalid email or password",
		})
	}
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Database error",
		})
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Invalid email or password",
		})
	}

	// Get user agent and IP for token tracking
	userAgent := c.Request().Header.Get("User-Agent")
	ipAddress := c.RealIP()

	// Create token pair
	tokenPair, err := h.tokenService.CreateTokenPair(user, userAgent, ipAddress)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create session",
		})
	}

	// Get user's organizations and roles
	organizations, err := h.getUserOrganizations(user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get user organizations",
		})
	}

	// Build response
	response := LoginResponse{
		User: &UserInfo{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
		TokenPair:     tokenPair,
		Organizations: organizations,
	}

	return c.JSON(http.StatusOK, response)
}

// Logout handles user logout (revokes tokens)
func (h *AuthHandlers) Logout(c echo.Context) error {
	// Get the access token from the request
	authHeader := c.Request().Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Authorization header required",
		})
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate token to get the token hash
	user, err := h.tokenService.ValidateAccessToken(tokenString)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Invalid token",
		})
	}

	// Parse JWT to get token hash (we need this to revoke the specific session)
	claims, err := h.tokenService.parseTokenClaims(tokenString)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Invalid token",
		})
	}

	// Check if refresh token is provided for single-session logout
	var req struct {
		RefreshToken string `json:"refresh_token,omitempty"`
		LogoutAll    bool   `json:"logout_all,omitempty"`
	}
	c.Bind(&req)

	if req.LogoutAll {
		// Logout from all devices
		err = h.tokenService.RevokeAllUserTokens(user.ID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to logout from all devices",
			})
		}
		return c.JSON(http.StatusOK, map[string]string{
			"message": "Logged out from all devices",
		})
	} else {
		// Single session logout - revoke current access token
		err = h.tokenService.RevokeToken(claims.TokenHash, "session")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to revoke session",
			})
		}

		// Also revoke the refresh token if provided
		if req.RefreshToken != "" {
			refreshTokenHash := h.tokenService.hashToken(req.RefreshToken)
			h.tokenService.RevokeToken(refreshTokenHash, "refresh")
		}

		return c.JSON(http.StatusOK, map[string]string{
			"message": "Logged out successfully",
		})
	}
}

// Me returns information about the currently authenticated user
func (h *AuthHandlers) Me(c echo.Context) error {
	// Get user from context (set by middleware)
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "User not found in context",
		})
	}

	// Get user's organizations and roles
	organizations, err := h.getUserOrganizations(user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get user organizations",
		})
	}

	response := struct {
		User          *UserInfo `json:"user"`
		Organizations []OrgInfo `json:"organizations"`
	}{
		User: &UserInfo{
			ID:        user.ID,
			Email:     user.Email,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
		Organizations: organizations,
	}

	return c.JSON(http.StatusOK, response)
}

// RefreshToken handles token refresh using a valid refresh token
func (h *AuthHandlers) RefreshToken(c echo.Context) error {
	var req RefreshRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Get user agent and IP for token tracking
	userAgent := c.Request().Header.Get("User-Agent")
	ipAddress := c.RealIP()

	// Refresh the token pair
	tokenPair, err := h.tokenService.RefreshTokenPair(req.RefreshToken, userAgent, ipAddress)
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Invalid or expired refresh token",
		})
	}

	return c.JSON(http.StatusOK, tokenPair)
}

// SetupAdmin handles initial admin user setup (replaces legacy password system)
func (h *AuthHandlers) SetupAdmin(c echo.Context) error {
	var req SetupAdminRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Check if any users already exist
	var userCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Database error",
		})
	}

	if userCount > 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Admin user already exists",
		})
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to hash password",
		})
	}

	// Start transaction
	tx, err := h.db.Begin()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Database transaction error",
		})
	}
	defer tx.Rollback()

	// Create default organization
	var orgID int64
	err = tx.QueryRow(`
		INSERT INTO orgs (name, created_at, updated_at)
		VALUES ($1, NOW(), NOW())
		RETURNING id
	`, req.OrgName).Scan(&orgID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create organization",
		})
	}

	// Create admin user
	var userID int64
	err = tx.QueryRow(`
		INSERT INTO users (email, password_hash, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id
	`, req.Email, string(hashedPassword)).Scan(&userID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create user",
		})
	}

	// Get the super_admin role ID
	var roleID int64
	err = tx.QueryRow(`
		SELECT id FROM roles WHERE name = 'super_admin'
	`).Scan(&roleID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to find super_admin role",
		})
	}

	// Add super_admin role to user
	_, err = tx.Exec(`
		INSERT INTO user_roles (user_id, org_id, role_id, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, userID, orgID, roleID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to assign role",
		})
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to commit transaction",
		})
	}

	// Create user object for token generation
	user := &models.User{
		ID:           userID,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Get user agent and IP for token tracking
	userAgent := c.Request().Header.Get("User-Agent")
	ipAddress := c.RealIP()

	// Create token pair for immediate login
	tokenPair, err := h.tokenService.CreateTokenPair(user, userAgent, ipAddress)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Admin created but failed to create session",
		})
	}

	// Get user's organizations and roles
	organizations, err := h.getUserOrganizations(userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Admin created but failed to get organizations",
		})
	}

	// Build response similar to login
	response := struct {
		Message       string     `json:"message"`
		User          *UserInfo  `json:"user"`
		TokenPair     *TokenPair `json:"tokens"`
		Organizations []OrgInfo  `json:"organizations"`
	}{
		Message: "Admin user created successfully",
		User: &UserInfo{
			ID:        userID,
			Email:     req.Email,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		},
		TokenPair:     tokenPair,
		Organizations: organizations,
	}

	return c.JSON(http.StatusOK, response)
}

// ChangePassword handles password changes (useful for temp passwords)
func (h *AuthHandlers) ChangePassword(c echo.Context) error {
	// Get user from context (set by middleware)
	user, ok := c.Get("user").(*models.User)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "User not found in context",
		})
	}

	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
	}

	// Verify current password
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword))
	if err != nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Current password is incorrect",
		})
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to hash password",
		})
	}

	// Update password
	_, err = h.db.Exec(`
		UPDATE users 
		SET password_hash = $1, updated_at = NOW()
		WHERE id = $2
	`, string(hashedPassword), user.ID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update password",
		})
	}

	// Optionally revoke all existing tokens to force re-login
	// This is a security best practice when passwords change
	err = h.tokenService.RevokeAllUserTokens(user.ID)
	if err != nil {
		// Log but don't fail - password change succeeded
		fmt.Printf("Warning: failed to revoke tokens after password change: %v\n", err)
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Password changed successfully",
	})
}

// CheckSetupStatus checks if initial setup is needed
func (h *AuthHandlers) CheckSetupStatus(c echo.Context) error {
	// Check if any users exist
	var userCount int
	err := h.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Database error",
		})
	}

	response := map[string]interface{}{
		"setup_required": userCount == 0,
		"user_count":     userCount,
	}

	return c.JSON(http.StatusOK, response)
}

// Helper method to get user's organizations and roles
func (h *AuthHandlers) getUserOrganizations(userID int64) ([]OrgInfo, error) {
	rows, err := h.db.Query(`
		SELECT o.id, o.name, r.name
		FROM orgs o
		JOIN user_roles ur ON o.id = ur.org_id
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY o.name
	`, userID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var organizations []OrgInfo
	for rows.Next() {
		var org OrgInfo
		err := rows.Scan(&org.ID, &org.Name, &org.Role)
		if err != nil {
			return nil, err
		}
		organizations = append(organizations, org)
	}

	return organizations, nil
}
