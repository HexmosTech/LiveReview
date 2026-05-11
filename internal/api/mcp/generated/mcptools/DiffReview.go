package mcptools

import (
	"context"
	"github.com/livereview/internal/api/mcp/shared"
	"github.com/mark3labs/mcp-go/mcp"
)

// Input Schema for the DiffReview tool
const DiffReviewInputSchema = `{
  "properties": {
    "body": {
      "properties": {
        "diff_zip_base64": {
          "type": "string"
        },
        "repo_name": {
          "type": "string"
        }
      },
      "required": [
        "diff_zip_base64",
        "repo_name"
      ],
      "type": "object"
    }
  },
  "type": "object"
}`

// NewDiffReviewMCPTool creates the MCP Tool instance for DiffReview
func NewDiffReviewMCPTool() mcp.Tool {
	return mcp.NewToolWithRawSchema(
		"DiffReview",
		"DiffReview accepts a base64-encoded ZIP containing a unified diff and triggers a review.",
		[]byte(DiffReviewInputSchema),
	)
}

// DiffReviewHandler is the handler function for the DiffReview tool.
// This function is automatically generated. Users should implement the actual
// logic within this function body to integrate with backend APIs.
// You can generate types, http client and helpers for parsing request params to facilitate the implementation.
func DiffReviewHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return shared.GlobalProxy.CallAPI(ctx, "POST", "/api/v1/diff-review", request.Params.Arguments)
}
