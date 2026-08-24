// Package chatexport builds a renderer-agnostic representation of one or
// more Livi chat conversations (see internal/chat) and renders it to PDF or
// self-contained HTML. Both renderers consume the same CompiledDoc - built
// once per export by BuildDoc/BuildCompiledDoc, including every chart
// already rendered to PNG - so the two formats can never show different
// data for the same export.
package chatexport

import "time"

// CompiledDoc is the complete input to both RenderPDF and RenderHTML - the
// storagechat.Store is not touched again once this is built. A
// single-conversation export (see internal/api.ExportConversation) is just
// a CompiledDoc with one Conversation and no Subtitle; a multi-conversation
// "compile" export (internal/api.CompileExport) is the general case.
type CompiledDoc struct {
	Title         string
	Subtitle      string
	Conversations []ExportDoc
}

// ExportDoc is one conversation's contribution to a CompiledDoc.
type ExportDoc struct {
	Conversation ExportConversation
	Turns        []ExportTurn
}

// ExportConversation is the front-matter shown once, at the top of the
// export.
type ExportConversation struct {
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ExportTurn is one persisted message - a user prompt or an assistant
// reply - plus whatever it produced.
type ExportTurn struct {
	Seq            int
	Role           string // "user" | "assistant"
	CreatedAt      time.Time
	Text           string
	Charts         []ExportChart
	Files          []ExportFile
	DebugArtifacts []byte // raw JSON; nil unless BuildOptions.IncludeDebugArtifacts
}

// ExportChart is one chart artifact, pre-rendered to PNG once during
// BuildDoc so both renderers embed the identical image.
type ExportChart struct {
	Title       string
	Description string
	PNG         []byte
	// Stats is the chart's precomputed KPI chips (see
	// internal/chatstats.ComputeAllStats), resolved once during BuildDoc to
	// the day-granularity view for a trend-shaped chart (matching what a
	// reader sees opening the chat fresh) - nil when the chart's shape has
	// no precomputed KPIs.
	Stats []StatLine
}

// StatLine is one label/value KPI chip, the flattened form every renderer
// (HTML/PDF/Markdown) consumes so none of them need to understand the
// chatstats.AllStats "kind" discriminator themselves.
type StatLine struct {
	Label string
	Value string
}

// ExportFile is one file attachment, summarized - not the raw payload,
// which stays a separate in-app download so exports don't balloon with
// full CSV dumps.
type ExportFile struct {
	Filename string
	Kind     string
	Rows     int
}
