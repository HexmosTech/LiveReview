package mcptools

import (
	"context"

	"github.com/livereview/internal/api/mcp/shared"
	"github.com/mark3labs/mcp-go/mcp"
)

// Input Schema for the GetReviewByID tool
const getReviewByIDInputSchema = `{
  "properties": {
    "id": {
      "type": "string"
    }
  },
  "required": [
    "id"
  ],
  "type": "object"
}`

// NewGetReviewByIDMCPTool creates the MCP Tool instance for GetReviewByID
func NewGetReviewByIDMCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"GetReviewByID",
		"getReviewByID handles GET /api/v1/reviews/{id}",
		[]byte(getReviewByIDInputSchema),
	)
}

// GetReviewByIDHandler is the handler function for the GetReviewByID tool.
func GetReviewByIDHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "GET", "/api/v1/reviews/{id}", request.Params.Arguments)
}
