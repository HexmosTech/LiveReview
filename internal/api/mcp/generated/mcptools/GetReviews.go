package mcptools

import (
	"context"

	"github.com/livereview/internal/api/mcp/shared"
	"github.com/mark3labs/mcp-go/mcp"
)

// Input Schema for the GetReviews tool
const getReviewsInputSchema = `{
  "properties": {
    "page": {
      "type": "string"
    },
    "per_page": {
      "type": "string"
    },
    "provider": {
      "type": "string"
    },
    "search": {
      "type": "string"
    },
    "status": {
      "type": "string"
    }
  },
  "required": [
    "page",
    "per_page",
    "status",
    "provider",
    "search"
  ],
  "type": "object"
}`

// NewGetReviewsMCPTool creates the MCP Tool instance for GetReviews
func NewGetReviewsMCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"GetReviews",
		"getReviews handles GET /api/v1/reviews with filtering and pagination",
		[]byte(getReviewsInputSchema),
	)
}

// GetReviewsHandler is the handler function for the GetReviews tool.
func GetReviewsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "GET", "/api/v1/reviews", request.Params.Arguments)
}
