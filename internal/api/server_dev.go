//go:build !production

package api

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/mcpagent"
	storageanalytics "github.com/livereview/storage/analytics"
)

// registerDevRoutes registers development-only endpoints (excluded in production builds)
func (s *Server) registerDevRoutes(v1 *echo.Group) {
	v1.POST("/test-chat", s.HandleTestChat)
}

// HandleTestChat is a TEMPORARY endpoint that bypasses auth for local testing.
func (s *Server) HandleTestChat(c echo.Context) error {
	var req WebChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}
	if req.Message == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "message is required"})
	}

	ctx := c.Request().Context()

	// Use org 151 (hexmos-internal) for testing.
	orgID := int64(151)

	connector, err := s.resolveOrgConnector(ctx, orgID)
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": err.Error()})
	}

	mcpURL := resolveMCPBaseURL(s.db)
	maxSteps := 20

	mcpSession, err := mcpagent.ConnectMCP(ctx, mcpURL, map[string]string{})
	if err != nil {
		return c.JSON(http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("MCP connect: %s", err)})
	}

	mcpSession.OrgID = orgID
	mcpSession.OrgName = "hexmos-internal"
	mcpSession.UserRole = "owner"

	provider := mcpagent.NewProvider(connector)
	agent := mcpagent.NewAgent(provider, mcpSession, maxSteps).
		WithAnalytics(storageanalytics.NewAdHocStore(s.db))

	responseText, _, _, _, err := agent.RunTurnWithArtifacts(ctx, nil, req.Message, "test-session", "livi")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"response": responseText})
}
