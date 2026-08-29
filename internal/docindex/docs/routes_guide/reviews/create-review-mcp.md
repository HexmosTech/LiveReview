# Create Review via MCP

**Route:** `/reviews/create-mcp`
**Component:** `ui/src/pages/Reviews/CreateReviewMCP.tsx`

## Purpose

Standalone onboarding page (mega menu's "Create via MCP" entry) explaining
how to trigger reviews from AI coding assistants over MCP (Model Context
Protocol) — e.g. Claude Code, Cursor, Codex, Antigravity, Claude Desktop,
Windsurf, VS Code. Reuses the same MCP setup instructions/tab shown in
Settings → MCP Integration, plus links to API docs and MCP use-case examples.

## Who can access it

Any authenticated org member.

## Key actions

- Follow MCP setup steps for their editor/assistant of choice.
- Open external docs: full API reference, MCP use-case examples.

## Related pages

- [Dashboard](../dashboard.md)
- [Create Review via CLI](create-review-cli.md)
- [Settings → MCP Integration](../settings/mcp-integration.md)
