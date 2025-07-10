package models

// CodeDiff represents a code diff from a merge/pull request
type CodeDiff struct {
	FilePath    string
	OldContent  string
	NewContent  string
	Hunks       []DiffHunk
	CommitID    string
	FileType    string
	IsDeleted   bool
	IsNew       bool
	IsRenamed   bool
	OldFilePath string // Only set if IsRenamed is true
}

// DiffHunk represents a single chunk of changes in a diff
type DiffHunk struct {
	OldStartLine int
	OldLineCount int
	NewStartLine int
	NewLineCount int
	Content      string
}

// ReviewResult contains the overall review result including summary and specific comments
type ReviewResult struct {
	Summary  string           // High-level summary of what the diff is about
	Comments []*ReviewComment // Specific comments on the code
}

// ReviewComment represents a single comment from the AI review
type ReviewComment struct {
	FilePath    string
	Line        int
	Content     string
	Severity    CommentSeverity
	Confidence  float64
	Category    string
	Suggestions []string
}

// CommentSeverity represents the severity level of a review comment
type CommentSeverity string

const (
	SeverityInfo     CommentSeverity = "info"
	SeverityWarning  CommentSeverity = "warning"
	SeverityCritical CommentSeverity = "critical"
)
