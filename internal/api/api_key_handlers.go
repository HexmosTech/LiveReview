package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/auditlog"
)

// CreateAPIKeyRequest represents the request body for creating an API key
type CreateAPIKeyRequest struct {
	Label     string   `json:"label"`
	Kind      string   `json:"kind,omitempty"` // standard | audit_reader
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

	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "standard"
	}
	if !ValidAPIKeyKind(kind) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "kind must be standard or audit_reader"})
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

	if kind == "audit_reader" {
		if !pc.IsOwner && !pc.IsSuperAdmin {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "only owners can create audit-reader keys"})
		}
		if method, _ := c.Get(authMethodContextKey).(string); method == authMethodAPIKey {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "audit keys must be created from a user session"})
		}
		scopes = appendUniqueString(scopes, "audit:read")
	}

	manager := NewAPIKeyManager(s.db)
	tx, err := s.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create API key"})
	}
	defer func() { _ = tx.Rollback() }()

	apiKey, plainKey, err := manager.CreateAPIKeyTx(tx, userID, orgID, req.Label, scopes, expiresAt, kind)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create API key"})
	}

	if s.auditRecorder != nil {
		expires := ""
		if expiresAt != nil {
			expires = expiresAt.UTC().Format(time.RFC3339)
		}
		if err := s.auditRecorder.RecordTxEcho(c, tx, auditlog.Event{
			OrgID:        orgID,
			Action:       auditlog.ActionAPIKeyCreate,
			Category:     auditlog.CategoryAPIKey,
			ResourceType: "api_key",
			ResourceID:   strconv.FormatInt(apiKey.ID, 10),
			Outcome:      auditlog.OutcomeSuccess,
			Severity:     auditlog.SeverityWarning,
			Details:      auditlog.Details("label", req.Label, "kind", kind, "scopes", scopes, "expires_at", expires),
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record audit event"})
		}
	}

	if err := tx.Commit(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create API key"})
	}

	return c.JSON(http.StatusCreated, CreateAPIKeyResponse{
		APIKey:   apiKey,
		PlainKey: plainKey,
	})
}

func appendUniqueString(in []string, value string) []string {
	for _, v := range in {
		if v == value {
			return in
		}
	}
	return append(in, value)
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
	tx, err := s.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke API key"})
	}
	defer func() { _ = tx.Rollback() }()
	if err := manager.RevokeAPIKeyTx(tx, keyID, userID, orgID); err != nil {
		if err.Error() == "API key not found or already revoked" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke API key"})
	}
	if s.auditRecorder != nil {
		if err := s.auditRecorder.RecordTxEcho(c, tx, auditlog.Event{
			OrgID:        orgID,
			Action:       auditlog.ActionAPIKeyRevoke,
			Category:     auditlog.CategoryAPIKey,
			ResourceType: "api_key",
			ResourceID:   strconv.FormatInt(keyID, 10),
			Outcome:      auditlog.OutcomeSuccess,
			Severity:     auditlog.SeverityWarning,
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record audit event"})
		}
	}
	if err := tx.Commit(); err != nil {
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
	tx, err := s.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete API key"})
	}
	defer func() { _ = tx.Rollback() }()
	if err := manager.DeleteAPIKeyTx(tx, keyID, userID, orgID); err != nil {
		if err.Error() == "API key not found" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": err.Error()})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete API key"})
	}
	if s.auditRecorder != nil {
		if err := s.auditRecorder.RecordTxEcho(c, tx, auditlog.Event{
			OrgID:        orgID,
			Action:       auditlog.ActionAPIKeyDelete,
			Category:     auditlog.CategoryAPIKey,
			ResourceType: "api_key",
			ResourceID:   strconv.FormatInt(keyID, 10),
			Outcome:      auditlog.OutcomeSuccess,
			Severity:     auditlog.SeverityWarning,
		}); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to record audit event"})
		}
	}
	if err := tx.Commit(); err != nil {
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
			keyRecord, _, err := manager.ValidateAPIKey(apiKey)
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

// Context keys/values recording which auth path a request took — read by triggerTypeForRequest (review_service.go) to classify how a review was triggered.
const (
	authMethodContextKey = "auth_method"
	authMethodAPIKey     = "api_key"
	authMethodSession    = "session"
)

// RequireAuthOrAPIKey creates authentication middleware that supports both Bearer tokens and API keys
// This allows endpoints to accept either authentication method without breaking existing Bearer auth
func RequireAuthOrAPIKey(tokenService *auth.TokenService, db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {

			// First, try API key authentication
			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey != "" {
				manager := NewAPIKeyManager(db)
				keyRecord, user, err := manager.ValidateAPIKey(apiKey)
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

				// Update last used timestamp (async to not slow down request)
				go manager.UpdateLastUsed(keyRecord.ID)

				// Audit-only keys may call audit read/export endpoints and
				// nothing else. This is the first genuinely enforced key kind.
				if keyRecord.Kind == "audit_reader" && !isAuditReaderAllowed(c) {
					return c.JSON(http.StatusForbidden, map[string]string{
						"error":      "this API key can only read audit logs",
						"error_code": "AUDIT_KEY_SCOPE_DENIED",
					})
				}

				// Set user and org context (same as APIKeyAuthMiddleware)
				c.Set(string(auth.UserContextKey), user)
				c.Set("user_id", keyRecord.UserID)
				c.Request().Header.Set("X-Org-Context", strconv.FormatInt(keyRecord.OrgID, 10))
				c.Set("org_id", keyRecord.OrgID)
				c.Set("api_key_id", keyRecord.ID)
				c.Set("api_key_kind", keyRecord.Kind)
				c.Set("api_key_scopes", keyRecord.Scopes)
				c.Set(authMethodContextKey, authMethodAPIKey)

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
				fallbackUser, ferr := auth.ValidateWithCloudSecret(tokenString, db)
				if ferr != nil {
					return c.JSON(http.StatusUnauthorized, map[string]string{
						"error": "Invalid or expired token",
					})
				}
				// Add user to context and continue
				c.Set(string(auth.UserContextKey), fallbackUser)
				c.Set(authMethodContextKey, authMethodSession)
				return next(c)
			}

			// Add user to context
			c.Set(string(auth.UserContextKey), user)
			c.Set(authMethodContextKey, authMethodSession)

			return next(c)
		}
	}
}

// isAuditReaderAllowed is the route allowlist for audit_reader keys. It is
// deliberately path-based (not c.Path()) so it works before the router's
// template is available on every Echo version. Only GET/HEAD requests to an
// audit-events endpoint pass.
func isAuditReaderAllowed(c echo.Context) bool {
	method := c.Request().Method
	if method != echo.GET && method != echo.HEAD {
		return false
	}
	return strings.Contains(c.Request().URL.Path, "/audit-events")
}
