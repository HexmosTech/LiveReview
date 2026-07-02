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
}

// Config holds the runtime configuration for the agent.
type Config struct {
	MaxAgentSteps int
}

// ProviderTools are the langchaingo tool definitions for the provider.
type ProviderTools []llms.Tool

// HistoryEntry is a provider-agnostic conversation message.
type HistoryEntry map[string]any
