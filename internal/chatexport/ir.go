// Package chatexport builds a renderer-agnostic representation of a Livi
// chat conversation (see internal/chat) and renders it to PDF or
// self-contained HTML. Both renderers consume the same ExportDoc - built
// once per export by BuildDoc, including every chart already rendered to
// PNG - so the two formats can never show different data for the same
// conversation.
package chatexport

import "time"

// ExportDoc is the complete input to both RenderPDF and RenderHTML - the
// storagechat.Store is not touched again once this is built.
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
}

// ExportFile is one file attachment, summarized - not the raw payload,
// which stays a separate in-app download so exports don't balloon with
// full CSV dumps.
type ExportFile struct {
	Filename string
	Kind     string
	Rows     int
}
