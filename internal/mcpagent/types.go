package mcpagent

import (
	"github.com/livereview/internal/vlrender"
	"github.com/tmc/langchaingo/llms"
)

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
	TimeRange   string
	Granularity string
	Context     vlrender.ChartContext
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

// Interpretation is one possible SQL+chart reading of a user's analytics
// question, produced by the multi-interpret pipeline (runMultiInterpret).
// The model returns up to 5 of these in a single call, each with its own
// SQL, chart type, and encoding — like prelivi's interpretation.py.
type Interpretation struct {
	SQL          string         `json:"sql"`
	ChartType    string         `json:"chart_type"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	VegaLiteSpec map[string]any `json:"vega_lite_spec,omitempty"`
	Encoding     map[string]any `json:"encoding,omitempty"`
	Name         string         `json:"name,omitempty"`
	QuerySummary string         `json:"query_summary,omitempty"`
	TimeWindow   string         `json:"time_range,omitempty"`
	Granularity  string         `json:"granularity,omitempty"`
	Context      vlrender.ChartContext `json:"context"`
}

// InterpretationResult pairs an interpretation with its executed outcome.
type InterpretationResult struct {
	Interpretation Interpretation
	RowCount       int
	Stats          []string
	Chart          *vlrender.VegaLiteReport // nil if CSV/no-data
	Artifact       *Artifact               // nil if chart
	Status         string                  // "rendered", "skipped", "failed"
	SkipReason     string
	Rows           []map[string]any // actual row data for debug preview
	RetryCount     int
	Retries        []RetryInfo
}

// RetryInfo captures one retry attempt during SQL execution.
type RetryInfo struct {
	Attempt     int    `json:"attempt"`
	Error       string `json:"error"`
	RepairedSQL string `json:"repaired_sql,omitempty"`
}

// DebugArtifacts captures every intermediate representation of a multi-
// interpret analytics turn. Persisted as JSONB on chat_messages for the
// /chat-debug page. Temporary — will be dropped when the chatbot works.
type DebugArtifacts struct {
	Query           string              `json:"query"`
	SchemaContext   string              `json:"schema_context"`
	SystemPrompt    string              `json:"system_prompt"`
	LLMRawResponse  string              `json:"llm_raw_response"`
	FullRequest     string              `json:"full_request"` // system_prompt + schema_context sent to LLM
	Interpretations []Interpretation    `json:"interpretations"`
	Results         []DebugResultEntry  `json:"results"`
}

// DebugResultEntry is one interpretation's outcome for the debug view.
type DebugResultEntry struct {
	Index      int         `json:"index"`
	Title      string      `json:"title"`
	ChartType  string      `json:"chart_type"`
	SQL        string      `json:"sql"`
	Status     string      `json:"status"`
	SkipReason string      `json:"skip_reason,omitempty"`
	RowCount   int         `json:"row_count"`
	Stats      []string    `json:"stats,omitempty"`
	CSVData    string      `json:"csv_data,omitempty"`   // first N rows for preview
	VegaSpec   string      `json:"vega_spec,omitempty"`  // rendered chart spec JSON
	RetryCount int         `json:"retry_count,omitempty"`
	Retries    []RetryInfo `json:"retries,omitempty"`
}
