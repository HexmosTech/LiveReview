package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// TestHandlers provides test endpoints for manual testing of middleware
type TestHandlers struct{}

// NewTestHandlers creates a new instance of TestHandlers
func NewTestHandlers() *TestHandlers {
	return &TestHandlers{}
}

// PublicTest - No authentication required
func (h *TestHandlers) PublicTest(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "✅ Public endpoint works",
		"endpoint":   "GET /api/v1/test/public",
		"protection": "none",
		"user":       nil,
	})
}

// ProtectedTest - Requires valid JWT token
func (h *TestHandlers) ProtectedTest(c echo.Context) error {
	// Get user from context (set by RequireAuth middleware)
	user := c.Get("user")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "✅ Protected endpoint works",
		"endpoint":   "GET /api/v1/test/protected",
		"protection": "RequireAuth",
		"user":       user,
	})
}

// OrgScopedTest - Requires valid JWT + org membership
func (h *TestHandlers) OrgScopedTest(c echo.Context) error {
	// Get user and org context from middleware
	user := c.Get("user")
	orgID := c.Param("org_id")
	orgContext := c.Get("org_context")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":     "✅ Org-scoped endpoint works",
		"endpoint":    "GET /api/v1/orgs/:org_id/test",
		"protection":  "RequireAuth + OrgAccess",
		"user":        user,
		"org_id":      orgID,
		"org_context": orgContext,
	})
}

// SuperAdminTest - Requires super admin role
func (h *TestHandlers) SuperAdminTest(c echo.Context) error {
	// Get user from context
	user := c.Get("user")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "✅ Super admin endpoint works",
		"endpoint":   "GET /api/v1/admin/test",
		"protection": "RequireAuth + SuperAdmin",
		"user":       user,
	})
}

// TokenInfoTest - Shows token information for debugging
func (h *TestHandlers) TokenInfoTest(c echo.Context) error {
	user := c.Get("user")
	tokenHash := c.Get("token_hash")

	return c.JSON(http.StatusOK, map[string]interface{}{
		"message":    "✅ Token info endpoint works",
		"endpoint":   "GET /api/v1/test/token-info",
		"protection": "RequireAuth",
		"user":       user,
		"token_hash": tokenHash,
		"headers": map[string]string{
			"authorization": c.Request().Header.Get("Authorization"),
			"user_agent":    c.Request().Header.Get("User-Agent"),
		},
	})
}
