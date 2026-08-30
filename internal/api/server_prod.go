//go:build production

package api

import "github.com/labstack/echo/v4"

// registerDevRoutes is a no-op in production builds.
// The /test-chat endpoint is excluded for security.
func (s *Server) registerDevRoutes(v1 *echo.Group) {
	// No dev routes in production
}
