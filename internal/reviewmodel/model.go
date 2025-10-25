package reviewmodel

import "time"

// TimelineItem represents a single event in the MR timeline
// It merges commits and comments in a single ascending-by-time sequence.
type TimelineItem struct {
	// One of: commit, comment, system
	Kind string `json:"kind"`

	// Common fields
	ID        string     `json:"id"`         // provider-scoped identifier (e.g., commit SHA, note ID)
	CreatedAt time.Time  `json:"created_at"` // event timestamp
	Author    AuthorInfo `json:"author"`     // structured author identity

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
	ID           string     `json:"id"`
	DiscussionID string     `json:"discussion_id,omitempty"`
	ParentID     string     `json:"parent_id,omitempty"`
	Author       AuthorInfo `json:"author"`
	Body         string     `json:"body"`
	CreatedAt    time.Time  `json:"created_at"`
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

// AuthorInfo captures identifying information for a user/author.
// Prefer stable identifiers for matching (id/username); name is for display only.
type AuthorInfo struct {
	Provider  string `json:"provider,omitempty"`
	ID        int    `json:"id,omitempty"`
	Username  string `json:"username,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// Export models to reduce repetition in JSON outputs

// Participant is a normalized unique user in the export payload.
type Participant struct {
	Ref       string `json:"ref"` // stable key used by references (e.g., "u:gitlab:48" or derived from username/email)
	Provider  string `json:"provider,omitempty"`
	ID        int    `json:"id,omitempty"`
	Username  string `json:"username,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// ExportTimelineItem refers to a participant via AuthorRef and embeds the core event payload.
type ExportTimelineItem struct {
	Kind          string           `json:"kind"`
	ID            string           `json:"id"`
	CreatedAt     time.Time        `json:"created_at"`
	AuthorRef     string           `json:"author_ref,omitempty"`
	PrevCommitSHA string           `json:"prev_commit_sha,omitempty"`
	Commit        *TimelineCommit  `json:"commit,omitempty"`
	Comment       *TimelineComment `json:"comment,omitempty"`
}

// ExportCommentNode mirrors CommentNode but replaces Author with AuthorRef
type ExportCommentNode struct {
	ID            string               `json:"id"`
	DiscussionID  string               `json:"discussion_id,omitempty"`
	ParentID      string               `json:"parent_id,omitempty"`
	AuthorRef     string               `json:"author_ref,omitempty"`
	PrevCommitSHA string               `json:"prev_commit_sha,omitempty"`
	Body          string               `json:"body"`
	CreatedAt     time.Time            `json:"created_at"`
	FilePath      string               `json:"file_path,omitempty"`
	LineOld       int                  `json:"line_old,omitempty"`
	LineNew       int                  `json:"line_new,omitempty"`
	Children      []*ExportCommentNode `json:"children,omitempty"`
}

// ExportCommentTree aggregates participants and root nodes.
type ExportCommentTree struct {
	Participants []Participant        `json:"participants"`
	Roots        []*ExportCommentNode `json:"roots"`
}

// ExportTimeline aggregates participants and items.
type ExportTimeline struct {
	Participants []Participant        `json:"participants"`
	Items        []ExportTimelineItem `json:"items"`
}
