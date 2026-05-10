package generated

import (
	"github.com/livereview/internal/api/mcp/generated/mcptools"
	"github.com/mark3labs/mcp-go/server"
)

// NewMCPServer creates and returns an MCP server with all tools registered
func NewMCPServer() *server.MCPServer {
	// Create a new MCP server
	s := server.NewMCPServer(
		"MCP Server",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	// Register all tools
	s.AddTool(mcptools.NewDiffReviewMCPTool(), mcptools.DiffReviewHandler)
	s.AddTool(mcptools.NewTriggerReviewV2MCPTool(), mcptools.TriggerReviewV2Handler)
	s.AddTool(mcptools.NewGetReviewByIDMCPTool(), mcptools.GetReviewByIDHandler)
	s.AddTool(mcptools.NewGetReviewsMCPTool(), mcptools.GetReviewsHandler)

	return s
}
