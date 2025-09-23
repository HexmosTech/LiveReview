package reviewmodel

import "time"

// TimelineItem represents a single event in the MR timeline
// It merges commits and comments in a single ascending-by-time sequence.
type TimelineItem struct {
	// One of: commit, comment, system
	Kind string `json:"kind"`

	// Common fields
	ID        string    `json:"id"`         // provider-scoped identifier (e.g., commit SHA, note ID)
	CreatedAt time.Time `json:"created_at"` // event timestamp
	Author    string    `json:"author"`     // display name or username

	// Commit-specific
	Commit *TimelineCommit `json:"commit,omitempty"`

	// Comment-specific
	Comment *TimelineComment `json:"comment,omitempty"`
}

// TimelineCommit captures minimal commit facts for timeline display.
type TimelineCommit struct {
	SHA     string `json:"sha"`
	Title   string `json:"title"`
	Message string `json:"message"`
	WebURL  string `json:"web_url"`
}

// TimelineComment captures minimal note/discussion message facts for timeline display.
type TimelineComment struct {
	NoteID       string `json:"note_id"`
	Discussion   string `json:"discussion_id,omitempty"`
	Body         string `json:"body"`
	IsSystem     bool   `json:"is_system"`
	IsResolvable bool   `json:"is_resolvable"`
	Resolved     bool   `json:"resolved"`
	// Optional file context
	FilePath string `json:"file_path,omitempty"`
	LineOld  int    `json:"line_old,omitempty"`
	LineNew  int    `json:"line_new,omitempty"`
}

// CommentNode represents a node in the nested comment hierarchy (discussion thread tree).
type CommentNode struct {
	ID           string    `json:"id"`
	DiscussionID string    `json:"discussion_id,omitempty"`
	ParentID     string    `json:"parent_id,omitempty"`
	Author       string    `json:"author"`
	Body         string    `json:"body"`
	CreatedAt    time.Time `json:"created_at"`
	// Context
	FilePath string `json:"file_path,omitempty"`
	LineOld  int    `json:"line_old,omitempty"`
	LineNew  int    `json:"line_new,omitempty"`

	Children []*CommentNode `json:"children,omitempty"`
}

// CommentTree is a collection of top-level threads (roots) with nested replies.
type CommentTree struct {
	Roots []*CommentNode `json:"roots"`
}
