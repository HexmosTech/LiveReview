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

// Response Template for the GetReviewByID tool (Status: 200, Content-Type: application/json)
const GetReviewByIDResponseTemplate_A = `# API Response Information

Below is the response template for this API endpoint.

The template shows a possible response, including its status code and content type, to help you understand and generate correct outputs.

**Status Code:** 200

**Content-Type:** application/json

> OK

## Response Structure

- Structure (Type: object):
  - **status** (Type: string):
  - **mrTitle** (Type: string, nullable):
      - Nullable: true
  - **prMrUrl** (Type: string, nullable):
      - Nullable: true
  - **aiSummaryTitle** (Type: string, nullable):
      - Nullable: true
  - **startedAt** (Type: string, date-time, nullable):
      - Nullable: true
  - **id** (Type: integer, int64):
  - **completedAt** (Type: string, date-time, nullable):
      - Nullable: true
  - **connectorId** (Type: integer, int64, nullable):
      - Nullable: true
  - **branch** (Type: string, nullable):
      - Nullable: true
  - **repository** (Type: string):
  - **authorName** (Type: string, nullable):
      - Nullable: true
  - **friendlyName** (Type: string, nullable):
      - Nullable: true
  - **orgId** (Type: integer, int64):
  - **createdAt** (Type: string, date-time):
  - **authorUsername** (Type: string, nullable):
      - Nullable: true
  - **commitHash** (Type: string, nullable):
      - Nullable: true
  - **provider** (Type: string, nullable):
      - Nullable: true
  - **triggerType** (Type: string):
  - **userEmail** (Type: string, nullable):
      - Nullable: true
  - **metadata** (Type: object):
    - **Additional Properties**:
      - **property value** (Type: unknown):
`

// NewGetReviewByIDMCPTool creates the MCP Tool instance for GetReviewByID
func NewGetReviewByIDMCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"GetReviewByID",
		"getReviewByID handles GET /api/v1/reviews/:id",
		[]byte(getReviewByIDInputSchema),
	)
}

// GetReviewByIDHandler is the handler function for the GetReviewByID tool.
// This function is automatically generated. Users should implement the actual
// logic within this function body to integrate with backend APIs.
// You can generate types, http client and helpers for parsing request params to facilitate the implementation.
func GetReviewByIDHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "GET", "/api/v1/reviews/{id}", request.Params.Arguments)
}
