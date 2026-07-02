package api

import (
	"context"
	"encoding/json"
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
	ConnectorID  int64             `json:"connector_id"`
	MCPServerURL string            `json:"mcp_server_url"`
	MCPHeaders   map[string]string `json:"mcp_headers,omitempty"`
	Message      string            `json:"message"`
	History      []json.RawMessage `json:"history,omitempty"`
}

// MCPAgentChatResponse is the response from the MCP agent chat endpoint.
type MCPAgentChatResponse struct {
	Response string              `json:"response"`
	History  []json.RawMessage   `json:"history"`
	Tools    []mcpagent.MCPToolDef `json:"tools,omitempty"`
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

	pc := auth.MustGetPermissionContext(c)
	orgID := pc.OrgID
	if orgID <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "organization context required"})
	}

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

	// 4. Convert history from raw JSON
	history := make([]mcpagent.HistoryEntry, len(req.History))
	for i, raw := range req.History {
		var entry mcpagent.HistoryEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid history entry %d: %s", i, err.Error())})
		}
		history[i] = entry
	}

	// 5. Run the agent loop
	responseText, updatedHistory, err := agent.RunTurn(ctx, history, req.Message)
	if err != nil {
		log.Error().Err(err).Msg("Agent loop failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Agent loop failed: %s", err.Error())})
	}

	// 6. Convert updated history back to raw JSON
	historyJSON := make([]json.RawMessage, len(updatedHistory))
	for i, entry := range updatedHistory {
		raw, err := json.Marshal(entry)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to serialize history"})
		}
		historyJSON[i] = raw
	}

	return c.JSON(http.StatusOK, MCPAgentChatResponse{
		Response: responseText,
		History:  historyJSON,
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
