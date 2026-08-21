// Package chat holds the domain model for persisted Livi chat conversations:
// a linear sequence of user/assistant messages plus any chart artifacts an
// assistant turn produced. See storage/chat for the Postgres-backed store.
package chat

import (
	"encoding/json"
	"time"

	"github.com/livereview/internal/mcpagent"
)

// Conversation is one persisted Livi chat thread, owned by a single user
// within an org.
type Conversation struct {
	ID        int64
	OrgID     int64
	UserID    int64
	Title     string
	SessionID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Message is one turn boundary (a user prompt or an assistant reply) within a
// conversation. RawHistoryEntries is the exact slice of mcpagent.HistoryEntry
// this turn contributed to the agent's history - stored verbatim so a
// conversation can be replayed into RunTurnWithArtifacts without loss.
// DebugArtifacts is a temporary JSONB field for the /chat-debug page.
type Message struct {
	ID                int64
	ConversationID    int64
	Role              string // "user" | "assistant"
	Content           string
	RawHistoryEntries []mcpagent.HistoryEntry
	TurnSeq           int
	CreatedAt         time.Time
	Charts            []Chart
	Files             []File
	DebugArtifacts    json.RawMessage // nil when not a count_query turn
}

// File is a downloadable export (CSV today) captured from an assistant turn.
// The payload is stored in full so the download survives reloads and restarts
// rather than being tied to a transient in-memory entry.
type File struct {
	ID          int64
	MessageID   int64
	Kind        string
	Filename    string
	Title       string
	Description string
	Query       string
	TimeRange   string
	Granularity string
	Rows        int
	Data        []byte
	CreatedAt   time.Time
}

// Chart is a Vega-Lite chart artifact captured from an assistant turn: the
// spec actually rendered, the inputs that produced it, and the raw LLM text
// it was extracted from, so it can be reproduced or reformatted later
// without re-running the LLM.
type Chart struct {
	ID                    int64
	MessageID             int64
	Title                 string
	Description           string
	Query                 string
	TimeRange             string
	Granularity           string
	TriggeringUserMessage string
	VegaSpec              []byte // raw JSON
	RawLLMOutput          string
	CreatedAt             time.Time
}

// ConversationSearchResult is a conversation matched by SearchConversations,
// with a snippet of the matching text for display.
type ConversationSearchResult struct {
	Conversation
	Snippet string
}
