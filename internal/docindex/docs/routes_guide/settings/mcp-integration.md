# Settings → MCP Integration

**Route:** `/settings#mcp`
**Component:** `ui/src/pages/Settings/MCPIntegrationTab.tsx` (rendered both
here and standalone at [Create Review via MCP](../reviews/create-review-mcp.md))
**Who sees it:** any org member

## Purpose

Setup instructions for connecting AI coding assistants (Claude Code,
Cursor, Codex, Antigravity, Claude Desktop, Windsurf, VS Code) to
LiveReview over MCP, so reviews and product Q&A can be triggered from
within those tools.

## Key actions

- Copy MCP server connection config for the assistant of choice.
- Follow links to full API documentation and MCP use-case examples.

## Prerequisites

- **Production URL must be configured** (Settings → Instance). A warning
  banner is shown if it is missing — the MCP server URL in the setup
  snippet depends on the correct production URL being set.

## Related pages

[Settings overview](settings-overview.md), [Create Review via MCP](../reviews/create-review-mcp.md)
