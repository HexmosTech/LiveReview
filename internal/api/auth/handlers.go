package auth

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"os"
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
	// onboardingKeyGenerator creates a user's onboarding API key within an existing transaction.
	// Injected from package api (via SetOnboardingKeyGenerator) rather than imported directly,
	// since internal/api already imports internal/api/auth and a reverse import would cycle.
	// Optional: nil just means EnsureCloudUser skips eager generation (dashboard's own lazy
	// fallback in collectOnboardingData still covers it).
	onboardingKeyGenerator func(tx *sql.Tx, userID, orgID int64) (string, error)
}

// NewAuthHandlers creates a new authentication handlers instance
func NewAuthHandlers(tokenService *TokenService, db *sql.DB) *AuthHandlers {
	return &AuthHandlers{
		tokenService: tokenService,
		db:           db,
	}
}

// SetOnboardingKeyGenerator wires up eager onboarding-API-key creation for EnsureCloudUser, so a
// brand-new cloud signup gets its key during account provisioning instead of lazily on the
// user's first dashboard load (see collectOnboardingData in internal/api/dashboard.go).
func (h *AuthHandlers) SetOnboardingKeyGenerator(gen func(tx *sql.Tx, userID, orgID int64) (string, error)) {
	h.onboardingKeyGenerator = gen
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
	ID               int64      `json:"id"`
	Email            string     `json:"email"`
	Name             string     `json:"name,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	PlanType         string     `json:"plan_type,omitempty"`
	LicenseExpiresAt *time.Time `json:"license_expires_at,omitempty"`
	DefaultOrgID     *int64     `json:"default_org_id,omitempty"`
}

// userName returns "first last" from the users table, or "" when no name is set.
func (h *AuthHandlers) userName(userID int64) string {
	var first, last sql.NullString
	err := h.db.QueryRow(`SELECT first_name, last_name FROM users WHERE id = $1`, userID).Scan(&first, &last)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("[auth] failed to load name for user %d: %v", userID, err)
	}
	return strings.TrimSpace(first.String + " " + last.String)
}

// OrgInfo represents organization information for the user
type OrgInfo struct {
	ID               int64      `json:"id"`
	Name             string     `json:"name"`
	Role             string     `json:"role"` // super_admin, owner, member
	PlanType         *string    `json:"plan_type,omitempty"`
	LicenseExpiresAt *time.Time `json:"license_expires_at,omitempty"`
	CreatedByUserID  *int64     `json:"created_by_user_id,omitempty"`
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
		SELECT id, email, password_hash, default_org_id, created_at, updated_at
		FROM users WHERE email = $1
	`, req.Email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.DefaultOrgID, &user.CreatedAt, &user.UpdatedAt)

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

	// Get plan information from user_roles (only in cloud mode)
	var planType string
	var licenseExpiresAt *time.Time
	if isCloudMode() && len(organizations) > 0 {
		err = h.db.QueryRow(`
			SELECT obs.current_plan_code, obs.billing_period_end
			FROM org_billing_state obs
			WHERE obs.org_id = $1`,
			organizations[0].ID,
		).Scan(&planType, &licenseExpiresAt)
		if err != nil && err != sql.ErrNoRows {
			// Log error but don't fail - just use default free plan
			planType = "free"
		}
	}
	if planType == "" {
		planType = "free"
	}

	// Build response
	response := LoginResponse{
		User: &UserInfo{
			ID:               user.ID,
			Email:            user.Email,
			Name:             h.userName(user.ID),
			CreatedAt:        user.CreatedAt,
			UpdatedAt:        user.UpdatedAt,
			PlanType:         planType,
			LicenseExpiresAt: licenseExpiresAt,
			DefaultOrgID:     user.DefaultOrgID,
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

	// Get plan information from user_roles (only in cloud mode)
	var planType string
	var licenseExpiresAt *time.Time
	if isCloudMode() && len(organizations) > 0 {
		err = h.db.QueryRow(`
			SELECT obs.current_plan_code, obs.billing_period_end
			FROM org_billing_state obs
			WHERE obs.org_id = $1`,
			organizations[0].ID,
		).Scan(&planType, &licenseExpiresAt)
		if err != nil && err != sql.ErrNoRows {
			// Log error but don't fail - just use default free plan
			planType = "free"
		}
	}
	if planType == "" {
		planType = "free"
	}

	response := struct {
		User          *UserInfo `json:"user"`
		Organizations []OrgInfo `json:"organizations"`
	}{
		User: &UserInfo{
			ID:               user.ID,
			Email:            user.Email,
			Name:             h.userName(user.ID),
			CreatedAt:        user.CreatedAt,
			UpdatedAt:        user.UpdatedAt,
			PlanType:         planType,
			LicenseExpiresAt: licenseExpiresAt,
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

	// Create admin user first
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

	// Create default organization with created_by_user_id
	var orgID int64
	err = tx.QueryRow(`
		INSERT INTO orgs (name, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id
	`, req.OrgName, userID).Scan(&orgID)

	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to create organization",
		})
	}

	_, err = tx.Exec(`
		UPDATE users
		SET default_org_id = $1
		WHERE id = $2
	`, orgID, userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user's default organization",
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
		SELECT o.id, o.name, r.name, obs.current_plan_code, obs.billing_period_end, o.created_by_user_id
		FROM orgs o
		JOIN user_roles ur ON o.id = ur.org_id
		JOIN roles r ON ur.role_id = r.id
		LEFT JOIN org_billing_state obs ON o.id = obs.org_id
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
		var createdBy sql.NullInt64
		err := rows.Scan(&org.ID, &org.Name, &org.Role, &org.PlanType, &org.LicenseExpiresAt, &createdBy)
		if err != nil {
			return nil, err
		}
		if createdBy.Valid {
			org.CreatedByUserID = &createdBy.Int64
		}
		organizations = append(organizations, org)
	}

	return organizations, nil
}

// sendSignupDiscordNotification sends a notification to Discord when a new user signs up.
// It runs asynchronously and never fails the signup flow.
func sendSignupDiscordNotification(email, source string) {
	webhookURL := os.Getenv("DISCORD_SIGNUP_WEBHOOK_URL")
	if webhookURL == "" {
		return
	}

	if source == "" {
		source = "unknown"
	}

	content := fmt.Sprintf("🆕 **New signup** — `%s` (via %s) at %s",
		email, source, time.Now().UTC().Format(time.RFC3339))

	go func() {
		if err := postDiscordWebhook(webhookURL, map[string]string{"content": content}); err != nil {
			log.Printf("[discord-notify] %v", err)
		}
	}()
}

// postDiscordWebhook sends a JSON payload to a Discord webhook and reports any failure.
func postDiscordWebhook(webhookURL string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

// EnterpriseEnquiryRequest is the "Get Enterprise" form from the dashboard navbar.
type EnterpriseEnquiryRequest struct {
	Name       string `json:"name"`
	Company    string `json:"company"`
	Email      string `json:"email"`
	JobTitle   string `json:"job_title"`
	Developers string `json:"developers"`
	Country    string `json:"country"`
	Message    string `json:"message"`
}

// EnterpriseEnquiry posts the enquiry to the signup Discord channel, in the hexmos.com pricing-form format.
func (h *AuthHandlers) EnterpriseEnquiry(c echo.Context) error {
	webhookURL := os.Getenv("DISCORD_SIGNUP_WEBHOOK_URL")
	if webhookURL == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Enquiries are not configured"})
	}

	var req EnterpriseEnquiryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	// Discord rejects embed field values longer than 1024 characters.
	clip := func(v string) string {
		v = strings.TrimSpace(v)
		if r := []rune(v); len(r) > 1024 {
			return string(r[:1021]) + "..."
		}
		return v
	}
	required := []struct{ name, value string }{
		{"Name", clip(req.Name)},
		{"Company", clip(req.Company)},
		{"Work Email", clip(req.Email)},
		{"Job Title", clip(req.JobTitle)},
		{"Number of Developers", clip(req.Developers)},
	}
	fields := []map[string]any{}
	for _, f := range required {
		if f.value == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Please fill in all the required fields"})
		}
		fields = append(fields, map[string]any{"name": f.name, "value": f.value, "inline": true})
	}
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != strings.TrimSpace(req.Email) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Please enter a valid work email address"})
	}
	if country := clip(req.Country); country != "" {
		fields = append(fields, map[string]any{"name": "Country", "value": country, "inline": true})
	}
	if message := clip(req.Message); message != "" {
		fields = append(fields, map[string]any{"name": "How Can We Help", "value": message, "inline": false})
	}
	fields = append(fields, map[string]any{"name": "Source", "value": "LiveReview dashboard", "inline": false})

	payload := map[string]any{
		"username": "LiveReview Pricing",
		"embeds": []map[string]any{{
			"title":  "New Enterprise Enquiry",
			"color":  0x8b5cf6,
			"fields": fields,
			"footer": map[string]string{"text": "Submitted " + time.Now().UTC().Format("2006-01-02 15:04:05 UTC")},
		}},
	}
	if err := postDiscordWebhook(webhookURL, payload); err != nil {
		log.Printf("[enterprise-enquiry] %v", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "Could not send your enquiry"})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Note: Onboarding API keys are now created using the standard APIKeyManager from the api package
// This is called after transaction commit to avoid import cycles
