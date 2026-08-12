package mcpagent

import "github.com/tmc/langchaingo/llms"

// ToolCall represents a tool call detected in the LLM's text response.
type ToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// MCPToolDef describes a tool exposed by the MCP server.
type MCPToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

// MCPSession holds the connection state to a remote MCP server.
type MCPSession struct {
	ServerURL string            `json:"server_url"`
	Headers   map[string]string `json:"headers,omitempty"`
	Tools     []MCPToolDef      `json:"tools"`
	// OrgName and UserName carry the requesting user's identity so the agent can
	// scope its answers (e.g. name the organization in chart descriptions).
	OrgName  string `json:"org_name,omitempty"`
	UserName string `json:"user_name,omitempty"`
	// OrgID scopes generated SQL. Zero disables the analytics path entirely -
	// there is no safe default org, so an unset id must never fall back to one.
	OrgID int64 `json:"org_id,omitempty"`
	// UserRole decides which tables generated SQL may read. Raw SQL bypasses the
	// per-endpoint authorization the REST tools applied, so it has to be
	// re-derived here. An empty or unrecognized value is treated as the least
	// privileged role, which matters for the bots: they authenticate an
	// organization rather than a user.
	UserRole string `json:"user_role,omitempty"`
}

// Artifact is a file produced by a turn that cannot travel inside the response
// string - today only CSV exports. It is returned from RunTurnWithArtifacts
// rather than stored on the Agent, because bots share one Agent across
// concurrent sessions and per-agent state would cross-deliver files between
// users.
type Artifact struct {
	Kind        string // "csv"
	Filename    string
	Title       string
	Description string
	Query       string
	Data        []byte
	Rows        int
}

// Config holds the runtime configuration for the agent.
type Config struct {
	MaxAgentSteps int
}

// ProviderTools are the langchaingo tool definitions for the provider.
type ProviderTools []llms.Tool

// HistoryEntry is a provider-agnostic conversation message.
type HistoryEntry map[string]any
