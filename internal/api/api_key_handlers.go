package api

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
)

// CreateAPIKeyRequest represents the request body for creating an API key
type CreateAPIKeyRequest struct {
	Label     string   `json:"label"`
	Scopes    []string `json:"scopes,omitempty"`
	ExpiresAt *string  `json:"expires_at,omitempty"` // ISO 8601 format
}

// CreateAPIKeyResponse represents the response for a newly created API key
type CreateAPIKeyResponse struct {
	APIKey   *APIKey `json:"api_key"`
	PlainKey string  `json:"plain_key"` // Only returned once
}

// CreateAPIKeyHandler handles POST /api/v1/api-keys
func (s *Server) CreateAPIKeyHandler(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	userID := pc.GetUserID()
	orgID := pc.GetOrgID()

	var req CreateAPIKeyRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.Label == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "label is required"})
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		parsed, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid expires_at format (use ISO 8601)"})
		}
		expiresAt = &parsed
	}

	scopes := req.Scopes
	if scopes == nil {
		scopes = []string{}
	}

	manager := NewAPIKeyManager(s.db)
	apiKey, plainKey, err := manager.CreateAPIKey(userID, orgID, req.Label, scopes, expiresAt)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create API key"})
	}

	return c.JSON(http.StatusCreated, CreateAPIKeyResponse{
		APIKey:   apiKey,
		PlainKey: plainKey,
	})
}

// ListAPIKeysHandler handles GET /api/v1/api-keys
func (s *Server) ListAPIKeysHandler(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	userID := pc.GetUserID()
	orgID := pc.GetOrgID()

	manager := NewAPIKeyManager(s.db)
	keys, err := manager.ListAPIKeys(userID, orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list API keys"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"keys": keys,
	})
}

// RevokeAPIKeyHandler handles POST /api/v1/api-keys/:id/revoke
func (s *Server) RevokeAPIKeyHandler(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	userID := pc.GetUserID()
	orgID := pc.GetOrgID()

	keyIDStr := c.Param("id")
	keyID, err := strconv.ParseInt(keyIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid key ID"})
	}

	manager := NewAPIKeyManager(s.db)
	if err := manager.RevokeAPIKey(keyID, userID, orgID); err != nil {
		if err.Error() == "API key not found or already revoked" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke API key"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "API key revoked"})
}

// DeleteAPIKeyHandler handles DELETE /api/v1/api-keys/:id
func (s *Server) DeleteAPIKeyHandler(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	userID := pc.GetUserID()
	orgID := pc.GetOrgID()

	keyIDStr := c.Param("id")
	keyID, err := strconv.ParseInt(keyIDStr, 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid key ID"})
	}

	manager := NewAPIKeyManager(s.db)
	if err := manager.DeleteAPIKey(keyID, userID, orgID); err != nil {
		if err.Error() == "API key not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete API key"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "API key deleted"})
}

// APIKeyAuthMiddleware validates API key authentication
func APIKeyAuthMiddleware(db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error":      "LiveReview API key required",
					"error_code": "LIVE_REVIEW_API_KEY_REQUIRED",
				})
			}

			manager := NewAPIKeyManager(db)
			keyRecord, err := manager.ValidateAPIKey(apiKey)
			if err != nil {
				switch {
				case errors.Is(err, ErrLiveReviewAPIKeyInvalid):
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error":      "LiveReview API key is invalid",
						"error_code": "LIVE_REVIEW_API_KEY_INVALID",
					})
				case errors.Is(err, ErrLiveReviewAPIKeyRevoked):
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error":      "LiveReview API key is revoked",
						"error_code": "LIVE_REVIEW_API_KEY_REVOKED",
					})
				case errors.Is(err, ErrLiveReviewAPIKeyExpired):
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error":      "LiveReview API key is expired",
						"error_code": "LIVE_REVIEW_API_KEY_EXPIRED",
					})
				default:
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error":      "LiveReview API key validation failed",
						"error_code": "LIVE_REVIEW_API_KEY_VALIDATION_FAILED",
					})
				}
			}

			// Update last used timestamp (async to not slow down request)
			go manager.UpdateLastUsed(keyRecord.ID)

			// Set user and org context
			c.Set("user_id", keyRecord.UserID)
			c.Set("org_id", keyRecord.OrgID)
			c.Set("api_key_id", keyRecord.ID)

			return next(c)
		}
	}
}

// RequireAuthOrAPIKey creates authentication middleware that supports both Bearer tokens and API keys
// This allows endpoints to accept either authentication method without breaking existing Bearer auth
func RequireAuthOrAPIKey(tokenService *auth.TokenService, db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// First, try API key authentication
			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey != "" {
				manager := NewAPIKeyManager(db)
				keyRecord, err := manager.ValidateAPIKey(apiKey)
				if err != nil {
					// API key present but invalid - return error
					switch {
					case errors.Is(err, ErrLiveReviewAPIKeyInvalid):
						return c.JSON(http.StatusUnauthorized, map[string]string{
							"error":      "API key is invalid",
							"error_code": "LIVE_REVIEW_API_KEY_INVALID",
						})
					case errors.Is(err, ErrLiveReviewAPIKeyRevoked):
						return c.JSON(http.StatusUnauthorized, map[string]string{
							"error":      "API key is revoked",
							"error_code": "LIVE_REVIEW_API_KEY_REVOKED",
						})
					case errors.Is(err, ErrLiveReviewAPIKeyExpired):
						return c.JSON(http.StatusUnauthorized, map[string]string{
							"error":      "API key is expired",
							"error_code": "LIVE_REVIEW_API_KEY_EXPIRED",
						})
					default:
						return c.JSON(http.StatusUnauthorized, map[string]string{
							"error":      "API key validation failed",
							"error_code": "LIVE_REVIEW_API_KEY_VALIDATION_FAILED",
						})
					}
				}

				// Fetch user from database
				user := &models.User{}
				err = db.QueryRow(`
					SELECT id, email, password_hash, created_at, updated_at
					FROM users WHERE id = $1
				`, keyRecord.UserID).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
				if err != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "User not found"})
				}

				// Update last used timestamp (async to not slow down request)
				go manager.UpdateLastUsed(keyRecord.ID)

				// Set user and org context (same as APIKeyAuthMiddleware)
				c.Set(string(auth.UserContextKey), user)
				c.Set("user_id", keyRecord.UserID)
				c.Set("org_id", keyRecord.OrgID)
				c.Set("api_key_id", keyRecord.ID)

				return next(c)
			}

			// Fall back to Bearer token authentication (existing logic)
			authHeader := c.Request().Header.Get("Authorization")
			if authHeader == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Authorization header or API key required",
				})
			}

			// Check Bearer token format
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"error": "Invalid authorization header format",
				})
			}

			tokenString := tokenParts[1]

			// Validate token using the existing RequireAuth logic
			user, err := tokenService.ValidateAccessToken(tokenString)
			if err != nil {
				// Fallback: validate with CLOUD_JWT_SECRET for verification-stage tokens
				fallbackUser, ferr := validateWithCloudSecret(tokenString, db)
				if ferr != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error": "Invalid or expired token",
					})
				}
				// Add user to context and continue
				c.Set(string(auth.UserContextKey), fallbackUser)
				return next(c)
			}

			// Add user to context
			c.Set(string(auth.UserContextKey), user)

			return next(c)
		}
	}
}

// validateWithCloudSecret attempts to validate a JWT using CLOUD_JWT_SECRET without DB token checks
// This is a copy of the function from auth/middleware.go to avoid circular dependencies
func validateWithCloudSecret(tokenString string, db *sql.DB) (*models.User, error) {
	secret := os.Getenv("CLOUD_JWT_SECRET")
	if strings.TrimSpace(secret) == "" {
		return nil, errors.New("CLOUD_JWT_SECRET not configured")
	}

	token, err := jwt.ParseWithClaims(tokenString, &auth.JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !token.Valid {
		if err == nil {
			err = errors.New("invalid token")
		}
		return nil, err
	}

	claims, ok := token.Claims.(*auth.JWTClaims)
	if !ok {
		return nil, errors.New("invalid token claims")
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

	return nil, errors.New("user not found for cloud jwt")
}
