package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
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
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "API key required"})
			}

			manager := NewAPIKeyManager(db)
			keyRecord, err := manager.ValidateAPIKey(apiKey)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid or expired API key"})
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
