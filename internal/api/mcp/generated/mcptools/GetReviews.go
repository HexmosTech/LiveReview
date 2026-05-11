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

// Response Template for the GetReviews tool (Status: 200, Content-Type: application/json)
const GetReviewsResponseTemplate_A = `# API Response Information

Below is the response template for this API endpoint.

The template shows a possible response, including its status code and content type, to help you understand and generate correct outputs.

**Status Code:** 200

**Content-Type:** application/json

> OK

## Response Structure

- Structure (Type: object):
  - **page** (Type: integer):
  - **perPage** (Type: integer):
  - **reviews** (Type: array):
    - **Items** (Type: object):
      - **prMrUrl** (Type: string, nullable):
          - Nullable: true
      - **completedAt** (Type: string, date-time, nullable):
          - Nullable: true
      - **startedAt** (Type: string, date-time, nullable):
          - Nullable: true
      - **id** (Type: integer, int64):
      - **provider** (Type: string, nullable):
          - Nullable: true
      - **authorUsername** (Type: string, nullable):
          - Nullable: true
      - **commitHash** (Type: string, nullable):
          - Nullable: true
      - **userEmail** (Type: string, nullable):
          - Nullable: true
      - **aiSummaryTitle** (Type: string, nullable):
          - Nullable: true
      - **connectorId** (Type: integer, int64, nullable):
          - Nullable: true
      - **triggerType** (Type: string):
      - **mrTitle** (Type: string, nullable):
          - Nullable: true
      - **friendlyName** (Type: string, nullable):
          - Nullable: true
      - **repository** (Type: string):
      - **createdAt** (Type: string, date-time):
      - **metadata** (Type: object):
        - **Additional Properties**:
          - **property value** (Type: unknown):
      - **orgId** (Type: integer, int64):
      - **branch** (Type: string, nullable):
          - Nullable: true
      - **status** (Type: string):
      - **authorName** (Type: string, nullable):
          - Nullable: true
  - **total** (Type: integer):
  - **totalPages** (Type: integer):
  - **hasNext** (Type: boolean):
  - **hasPrevious** (Type: boolean):
`

// NewGetReviewsMCPTool creates the MCP Tool instance for GetReviews
func NewGetReviewsMCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"GetReviews",
		"getReviews handles GET /api/v1/reviews with filtering and pagination",
		[]byte(getReviewsInputSchema),
	)
}

// GetReviewsHandler is the handler function for the GetReviews tool.
// This function is automatically generated. Users should implement the actual
// logic within this function body to integrate with backend APIs.
// You can generate types, http client and helpers for parsing request params to facilitate the implementation.
func GetReviewsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "GET", "/api/v1/reviews", request.Params.Arguments)
}
