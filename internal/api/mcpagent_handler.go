package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/aiconnectors"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/mcpagent"
	"github.com/rs/zerolog/log"
)

// MCPAgentChatRequest is the request body for the MCP agent chat endpoint.
type MCPAgentChatRequest struct {
	ConnectorID  int64                  `json:"connector_id"`
	MCPServerURL string                 `json:"mcp_server_url"`
	MCPHeaders   map[string]string      `json:"mcp_headers,omitempty"`
	Message      string                 `json:"message"`
	History      []mcpagent.HistoryEntry `json:"history,omitempty"`
}

// MCPAgentChatResponse is the response from the MCP agent chat endpoint.
type MCPAgentChatResponse struct {
	Response string                `json:"response"`
	History  []mcpagent.HistoryEntry `json:"history"`
	Tools    []mcpagent.MCPToolDef   `json:"tools,omitempty"`
}

// HandleMCPAgentChat processes a chat message through the agent loop.
func (s *Server) HandleMCPAgentChat(c echo.Context) error {
	var req MCPAgentChatRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if req.Message == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "message is required"})
	}
	if req.ConnectorID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "connector_id is required"})
	}
	if req.MCPServerURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "mcp_server_url is required"})
	}

	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication required"})
	}
	orgID := pc.OrgID

	ctx := c.Request().Context()

	// 1. Resolve the AI connector
	connector, err := s.resolveAIConnector(ctx, orgID, req.ConnectorID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	// 2. Connect to the MCP server
	mcpSession, err := mcpagent.ConnectMCP(ctx, req.MCPServerURL, req.MCPHeaders)
	if err != nil {
		log.Error().Err(err).Str("url", req.MCPServerURL).Msg("Failed to connect to MCP server")
		return c.JSON(http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("Failed to connect to MCP server: %s", err.Error())})
	}

	// 3. Create provider and agent
	provider := mcpagent.NewProvider(connector)
	agent := mcpagent.NewAgent(provider, mcpSession, 0)

	// 4. Run the agent loop
	responseText, updatedHistory, err := agent.RunTurn(ctx, req.History, req.Message)
	if err != nil {
		log.Error().Err(err).Msg("Agent loop failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Agent loop failed: %s", err.Error())})
	}

	return c.JSON(http.StatusOK, MCPAgentChatResponse{
		Response: responseText,
		History:  updatedHistory,
		Tools:    mcpSession.Tools,
	})
}

// resolveAIConnector fetches an AI connector by ID and org, and creates a
// connector instance from it.
func (s *Server) resolveAIConnector(ctx context.Context, orgID, connectorID int64) (*aiconnectors.Connector, error) {
	storage := aiconnectors.NewStorage(s.db)
	record, err := storage.GetConnectorByID(ctx, orgID, connectorID)
	if err != nil {
		return nil, fmt.Errorf("connector not found: %s", err.Error())
	}

	options := storage.GetConnectorOptions(ctx, record)
	connector, err := aiconnectors.NewConnector(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to create connector: %s", err.Error())
	}
	return connector, nil
}
