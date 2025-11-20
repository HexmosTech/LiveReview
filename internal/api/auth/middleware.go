package auth

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/livereview/pkg/models"
)

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
}

// NewAuthMiddleware creates a new auth middleware
func NewAuthMiddleware(tokenService *TokenService, db *sql.DB) *AuthMiddleware {
	return &AuthMiddleware{
		tokenService: tokenService,
		db:           db,
	}
}

// RequireAuth middleware validates that a valid JWT token is present
func (am *AuthMiddleware) RequireAuth() echo.MiddlewareFunc {
	return RequireAuth(am.tokenService, am.db)
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

			// Check if user has access to this organization
			var userRole string
			err := am.db.QueryRow(`
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
