package mcptools

import (
	"context"

	"github.com/livereview/internal/api/mcp/shared"
	"github.com/mark3labs/mcp-go/mcp"
)

// Input Schema for the TriggerReviewV2 tool
const TriggerReviewV2InputSchema = `{
  "properties": {
    "body": {
      "properties": {
        "connector_id": {
          "format": "int64",
          "type": "integer"
        },
        "pr_mr_url": {
          "type": "string"
        }
      },
      "required": [
        "pr_mr_url",
        "connector_id"
      ],
      "type": "object"
    }
  },
  "type": "object"
}`

// NewTriggerReviewV2MCPTool creates the MCP Tool instance for TriggerReviewV2
func NewTriggerReviewV2MCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"TriggerReviewV2",
		"TriggerReviewV2 initiates a review process for a given Pull/Merge Request URL.",
		[]byte(TriggerReviewV2InputSchema),
	)
}

// TriggerReviewV2Handler is the handler function for the TriggerReviewV2 tool.
func TriggerReviewV2Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "POST", "/api/v1/connectors/trigger-review", request.Params.Arguments)
}
