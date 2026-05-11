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
        "url": {
          "type": "string"
        }
      },
      "required": [
        "url"
      ],
      "type": "object"
    }
  },
  "type": "object"
}`

// Response Template for the TriggerReviewV2 tool (Status: 400, Content-Type: application/json)
const TriggerReviewV2ResponseTemplate_A = `# API Response Information

Below is the response template for this API endpoint.

The template shows a possible response, including its status code and content type, to help you understand and generate correct outputs.

**Status Code:** 400

**Content-Type:** application/json

> Bad Request

## Response Structure

- Structure (Type: object):
  - **error** (Type: string):
`

// Response Template for the TriggerReviewV2 tool (Status: 500, Content-Type: application/json)
const TriggerReviewV2ResponseTemplate_B = `# API Response Information

Below is the response template for this API endpoint.

The template shows a possible response, including its status code and content type, to help you understand and generate correct outputs.

**Status Code:** 500

**Content-Type:** application/json

> Internal Server Error

## Response Structure

- Structure (Type: object):
  - **error** (Type: string):
`

// NewTriggerReviewV2MCPTool creates the MCP Tool instance for TriggerReviewV2
func NewTriggerReviewV2MCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"TriggerReviewV2",
		"TriggerReviewV2 handles the request to trigger a code review using the new decoupled architecture",
		[]byte(TriggerReviewV2InputSchema),
	)
}

// TriggerReviewV2Handler is the handler function for the TriggerReviewV2 tool.
// This function is automatically generated. Users should implement the actual
// logic within this function body to integrate with backend APIs.
// You can generate types, http client and helpers for parsing request params to facilitate the implementation.
func TriggerReviewV2Handler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "POST", "/api/v1/connectors/trigger-review", request.Params.Arguments)
}
