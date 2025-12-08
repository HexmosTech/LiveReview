package auth

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/livereview/pkg/models"
)

// isCloudMode checks if LiveReview is running in cloud mode
func isCloudMode() bool {
	valueStr := os.Getenv("LIVEREVIEW_IS_CLOUD")
	valueStr = strings.ToLower(strings.TrimSpace(valueStr))
	return valueStr == "true" || valueStr == "1"
}

// ContextKey represents keys for context values
type ContextKey string

const (
	// Context keys
	UserContextKey       ContextKey = "user"
	PermissionContextKey ContextKey = "permission_context"
	OrgContextKey        ContextKey = "organization"
)

// RequireAuth is a helper function that creates authentication middleware
// This can be used directly without creating an AuthMiddleware instance
func RequireAuth(tokenService *TokenService, db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract token from Authorization header
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Authorization header required")
			}

			// Check Bearer token format
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid authorization header format")
			}

			tokenString := tokenParts[1]

			// Validate token
			user, err := tokenService.ValidateAccessToken(tokenString)
			if err != nil {
				// Fallback: validate with CLOUD_JWT_SECRET for verification-stage tokens
				fallbackUser, ferr := validateWithCloudSecret(tokenString, db)
				if ferr != nil {
					return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or expired token")
				}
				// Add user to context and continue
				c.Set(string(UserContextKey), fallbackUser)
				return next(c)
			}

			// Add user to context
			c.Set(string(UserContextKey), user)

			return next(c)
		}
	}
}

// validateWithCloudSecret attempts to validate a JWT using CLOUD_JWT_SECRET without DB token checks.
// If valid, it resolves the user from DB using claims (by ID first, then email).
func validateWithCloudSecret(tokenString string, db *sql.DB) (*models.User, error) {
	secret := os.Getenv("CLOUD_JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("CLOUD_JWT_SECRET not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if err == nil {
			err = fmt.Errorf("invalid token")
		}
		return nil, err
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Try resolving user by ID first
	user := &models.User{}
	if claims.UserID != 0 {
		err = db.QueryRow(`
			SELECT id, email, password_hash, created_at, updated_at
			FROM users WHERE id = $1
		`, claims.UserID).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
		if err == nil {
			return user, nil
		}
		if err != sql.ErrNoRows {
			return nil, err
		}
	}

	// Fallback: resolve by email if present
	if strings.TrimSpace(claims.Email) != "" {
		err = db.QueryRow(`
			SELECT id, email, password_hash, created_at, updated_at
			FROM users WHERE email = $1
		`, claims.Email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
		if err == nil {
			return user, nil
		}
	}

	return nil, fmt.Errorf("user not found for cloud jwt")
}

// AuthMiddleware holds the dependencies for auth middleware
type AuthMiddleware struct {
	tokenService *TokenService
	db           *sql.DB
	planStmt     *sql.Stmt // Prepared statement for plan lookup
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(tokenService *TokenService, db *sql.DB) *AuthMiddleware {
	// Prepare statement for subscription plan lookups (performance optimization)
	stmt, err := db.Prepare(`
		SELECT plan_type, license_expires_at
		FROM user_roles
		WHERE user_id = $1 AND org_id = $2
	`)
	if err != nil {
		// Log warning but don't fail - will fall back to non-prepared queries
		fmt.Printf("[Warning] Failed to prepare plan query: %v\n", err)
	}

	return &AuthMiddleware{
		tokenService: tokenService,
		db:           db,
		planStmt:     stmt,
	}
}

// RequireAuth middleware validates that a valid JWT token is present
func (am *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return RequireAuth(am.tokenService, am.db)
}

// EnforceSubscriptionLimits checks subscription validity in cloud mode
func (am *AuthMiddleware) EnforceSubscriptionLimits() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Only enforce subscriptions in cloud mode
			if !isCloudMode() {
				// Self-hosted: skip subscription checks entirely
				return next(c)
			}

			// Cloud mode: load subscription data based on current org context
			userInterface := c.Get(string(UserContextKey))
			if userInterface == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "user not found in context")
			}
			user, ok := userInterface.(*models.User)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid user in context")
			}

			// Get org_id from context (set by BuildOrgContext or BuildOrgContextFromHeader)
			orgID, hasOrgID := GetOrgIDFromContext(c)
			if !hasOrgID {
				// If no org context, use default/first org for the user
				err := am.db.QueryRow(`
					SELECT org_id FROM user_roles 
					WHERE user_id = $1 
					ORDER BY created_at ASC LIMIT 1
				`, user.ID).Scan(&orgID)
				if err != nil {
					if err == sql.ErrNoRows {
						return echo.NewHTTPError(http.StatusForbidden, "no organization access")
					}
					return echo.NewHTTPError(http.StatusInternalServerError, "failed to get user organization")
				}
				c.Set("org_id", orgID)
			}

			// Query user's plan in this specific org using prepared statement if available
			var planType string
			var licenseExpiresAt sql.NullTime
			var err error

			if am.planStmt != nil {
				err = am.planStmt.QueryRow(user.ID, orgID).Scan(&planType, &licenseExpiresAt)
			} else {
				// Fallback to non-prepared query
				err = am.db.QueryRow(`
					SELECT plan_type, license_expires_at
					FROM user_roles
					WHERE user_id = $1 AND org_id = $2
				`, user.ID, orgID).Scan(&planType, &licenseExpiresAt)
			}

			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusForbidden, "no access to this organization")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to check subscription")
			}

			// Check license expiration
			if licenseExpiresAt.Valid && time.Now().After(licenseExpiresAt.Time) {
				return echo.NewHTTPError(http.StatusPaymentRequired, map[string]interface{}{
					"error":            "license expired",
					"expired_at":       licenseExpiresAt.Time,
					"upgrade_required": true,
				})
			}

			// Set plan info in context for downstream handlers
			c.Set("plan_type", planType)
			if planType == "free" {
				dailyLimit := 3
				c.Set("daily_review_limit", &dailyLimit)
			} else {
				c.Set("daily_review_limit", (*int)(nil)) // unlimited
			}

			return next(c)
		}
	}
}

// BuildOrgContextFromHeader middleware extracts org_id from X-Org-Context header and validates org exists
func (am *AuthMiddleware) BuildOrgContextFromHeader() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			orgIDStr := c.Request().Header.Get("X-Org-Context")
			if orgIDStr == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "Organization context header required")
			}

			orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "Invalid organization ID in header")
			}

			// Check if organization exists
			org := &models.Org{}
			err = am.db.QueryRow(`
				SELECT id, name, description, created_at, updated_at
				FROM orgs WHERE id = $1
			`, orgID).Scan(&org.ID, &org.Name, &org.Description, &org.CreatedAt, &org.UpdatedAt)

			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusNotFound, "Organization not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch organization")
			}

			// Store org in context and set org_id for handlers
			c.Set(string(OrgContextKey), org)
			c.Set("org_id", org.ID)
			return next(c)
		}
	}
}

// BuildOrgContextFromConnector middleware extracts connector_id from URL path parameter,
// queries the integration_tokens table to get the org_id, validates both connector and org exist,
// and sets connector_id and org_id in context for downstream handlers.
// This is used for webhook endpoints where the connector_id is in the URL path.
func (am *AuthMiddleware) BuildOrgContextFromConnector() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			connectorIDStr := c.Param("connector_id")
			if connectorIDStr == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "Connector ID required in URL path")
			}

			connectorID, err := strconv.ParseInt(connectorIDStr, 10, 64)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "Invalid connector ID")
			}

			// Query integration_tokens to get org_id for this connector
			var orgID int64
			err = am.db.QueryRow(`
				SELECT org_id FROM integration_tokens WHERE id = $1
			`, connectorID).Scan(&orgID)

			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusNotFound, "Connector not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch connector")
			}

			// Validate that the organization exists and is active
			org := &models.Org{}
			err = am.db.QueryRow(`
				SELECT id, name, description, created_at, updated_at
				FROM orgs WHERE id = $1
			`, orgID).Scan(&org.ID, &org.Name, &org.Description, &org.CreatedAt, &org.UpdatedAt)

			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusNotFound, "Organization not found for this connector")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch organization")
			}

			// Store both connector_id and org context for downstream handlers
			c.Set("connector_id", connectorID)
			c.Set(string(OrgContextKey), org)
			c.Set("org_id", org.ID)

			return next(c)
		}
	}
}

// BuildOrgContext middleware extracts org_id from URL and validates org exists
func (am *AuthMiddleware) BuildOrgContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			orgIDStr := c.Param("org_id")
			if orgIDStr == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "Organization ID required")
			}

			orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "Invalid organization ID")
			}

			// Check if organization exists
			org := &models.Org{}
			err = am.db.QueryRow(`
				SELECT id, name, description, created_at, updated_at
				FROM orgs WHERE id = $1
			`, orgID).Scan(&org.ID, &org.Name, &org.Description, &org.CreatedAt, &org.UpdatedAt)

			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusNotFound, "Organization not found")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch organization")
			}

			// Store org in context
			c.Set(string(OrgContextKey), org)
			c.Set("org_id", org.ID)
			return next(c)
		}
	}
}

// ValidateOrgAccess middleware checks if user has access to the organization
func (am *AuthMiddleware) ValidateOrgAccess() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context
			userInterface := c.Get(string(UserContextKey))
			if userInterface == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
			}
			user := userInterface.(*models.User)

			// Get org from context
			orgInterface := c.Get(string(OrgContextKey))
			if orgInterface == nil {
				return echo.NewHTTPError(http.StatusBadRequest, "Organization not found in context")
			}
			org := orgInterface.(*models.Org)

			// Check if user is a super admin first - they have access to all orgs
			var isSuperAdmin bool
			err := am.db.QueryRow(`
				SELECT EXISTS(
					SELECT 1 FROM user_roles ur
					JOIN roles r ON ur.role_id = r.id
					WHERE ur.user_id = $1 AND r.name = $2
				)
			`, user.ID, RoleSuperAdmin).Scan(&isSuperAdmin)

			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check super admin status")
			}

			// If super admin, grant access with super_admin role
			if isSuperAdmin {
				c.Set("user_role", RoleSuperAdmin)
				return next(c)
			}

			// Check if user has access to this organization
			var userRole string
			err = am.db.QueryRow(`
				SELECT r.name
				FROM user_roles ur
				JOIN roles r ON ur.role_id = r.id
				WHERE ur.user_id = $1 AND ur.org_id = $2
			`, user.ID, org.ID).Scan(&userRole)

			if err != nil {
				if err == sql.ErrNoRows {
					return echo.NewHTTPError(http.StatusForbidden, "Access denied to this organization")
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check organization access")
			}

			// Store role information in context for later use
			c.Set("user_role", userRole)
			return next(c)
		}
	}
}

// BuildPermissionContext middleware builds the complete permission context
func (am *AuthMiddleware) BuildPermissionContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context
			userInterface := c.Get(string(UserContextKey))
			if userInterface == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
			}
			user := userInterface.(*models.User)

			// Get org from context
			orgInterface := c.Get(string(OrgContextKey))
			if orgInterface == nil {
				return echo.NewHTTPError(http.StatusBadRequest, "Organization not found in context")
			}
			org := orgInterface.(*models.Org)

			// Get user role from context
			userRole := c.Get("user_role").(string)

			// Build permission context
			permissionContext := &PermissionContext{
				User:         user,
				CurrentOrg:   org,
				Role:         userRole,
				IsSuperAdmin: userRole == RoleSuperAdmin,
				IsOwner:      userRole == RoleOwner,
				IsMember:     userRole == RoleMember,
				OrgID:        org.ID,
			}

			// Store permission context
			c.Set(string(PermissionContextKey), permissionContext)
			return next(c)
		}
	}
}

// BuildGlobalPermissionContext middleware builds a permission context for global endpoints
// that don't require organization context (like listing user's organizations)
func (am *AuthMiddleware) BuildGlobalPermissionContext() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context
			userInterface := c.Get(string(UserContextKey))
			if userInterface == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
			}
			user := userInterface.(*models.User)

			// Check if user is super admin in any organization
			var isSuperAdmin bool
			err := am.db.QueryRow(`
				SELECT EXISTS(
					SELECT 1 FROM user_roles ur
					JOIN roles r ON ur.role_id = r.id
					WHERE ur.user_id = $1 AND r.name = $2
				)
			`, user.ID, RoleSuperAdmin).Scan(&isSuperAdmin)

			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check super admin status")
			}

			// Build minimal permission context with just super admin flag
			permissionContext := &PermissionContext{
				User:         user,
				IsSuperAdmin: isSuperAdmin,
			}

			// Store permission context
			c.Set(string(PermissionContextKey), permissionContext)
			return next(c)
		}
	}
}

// RequireSuperAdmin middleware checks if user is super admin
func (am *AuthMiddleware) RequireSuperAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get user from context
			userInterface := c.Get(string(UserContextKey))
			if userInterface == nil {
				return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
			}
			user := userInterface.(*models.User)

			// Check if user is super admin in any organization
			var isSuperAdmin bool
			err := am.db.QueryRow(`
				SELECT EXISTS(
					SELECT 1 FROM user_roles ur
					JOIN roles r ON ur.role_id = r.id
					WHERE ur.user_id = $1 AND r.name = $2
				)
			`, user.ID, RoleSuperAdmin).Scan(&isSuperAdmin)

			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to check super admin status")
			}

			if !isSuperAdmin {
				return echo.NewHTTPError(http.StatusForbidden, "Super admin access required")
			}

			return next(c)
		}
	}
}

// RequirePermission creates middleware that checks for a specific permission
func (am *AuthMiddleware) RequirePermission(permission Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get permission context
			permCtxInterface := c.Get(string(PermissionContextKey))
			if permCtxInterface == nil {
				return echo.NewHTTPError(http.StatusForbidden, "Permission context not found")
			}
			permCtx := permCtxInterface.(*PermissionContext)

			// Check permission
			if !permCtx.HasPermission(permission) {
				return echo.NewHTTPError(http.StatusForbidden, fmt.Sprintf("Permission denied: %s", permission))
			}

			return next(c)
		}
	}
}

// Helper functions to extract context values

// GetUser extracts user from echo context
func GetUser(c echo.Context) *models.User {
	userInterface := c.Get(string(UserContextKey))
	if userInterface == nil {
		return nil
	}
	return userInterface.(*models.User)
}

// GetOrganization extracts organization from echo context
func GetOrganization(c echo.Context) *models.Org {
	orgInterface := c.Get(string(OrgContextKey))
	if orgInterface == nil {
		return nil
	}
	return orgInterface.(*models.Org)
}

// GetPermissionContext extracts permission context from echo context
func GetPermissionContext(c echo.Context) *PermissionContext {
	permCtxInterface := c.Get(string(PermissionContextKey))
	if permCtxInterface == nil {
		return nil
	}
	return permCtxInterface.(*PermissionContext)
}

// MustGetPermissionContext extracts permission context and panics if not found
// Use this only in handlers where permission context is guaranteed by middleware
func MustGetPermissionContext(c echo.Context) *PermissionContext {
	permCtx := GetPermissionContext(c)
	if permCtx == nil {
		panic("Permission context not found - ensure middleware is properly configured")
	}
	return permCtx
}

// GetConnectorIDFromContext extracts connector_id from echo context
// Returns the connector_id and true if found, 0 and false otherwise
func GetConnectorIDFromContext(c echo.Context) (int64, bool) {
	connectorIDInterface := c.Get("connector_id")
	if connectorIDInterface == nil {
		return 0, false
	}
	connectorID, ok := connectorIDInterface.(int64)
	return connectorID, ok
}

// GetOrgIDFromContext extracts org_id from echo context
// Returns the org_id and true if found, 0 and false otherwise
func GetOrgIDFromContext(c echo.Context) (int64, bool) {
	orgIDInterface := c.Get("org_id")
	if orgIDInterface == nil {
		return 0, false
	}
	orgID, ok := orgIDInterface.(int64)
	return orgID, ok
}
