package lib

import "github.com/livereview/internal/reviewmodel"

// UnifiedArtifact combines all processed data for a given merge/pull request
// from a specific provider. This is the canonical structure for downstream
// processing, such as prompt generation.
type UnifiedArtifact struct {
	Provider     string                     `json:"provider"`
	Timeline     []reviewmodel.TimelineItem `json:"timeline"`
	CommentTree  reviewmodel.CommentTree    `json:"comment_tree"`
	Diffs        []*LocalCodeDiff           `json:"diffs"`
	Participants []reviewmodel.AuthorInfo   `json:"participants"`
	// RawDataPaths holds the paths to the raw API data files used to generate
	// this artifact, useful for debugging and testing.
	RawDataPaths map[string]string `json:"raw_data_paths,omitempty"`
}

// LocalDiffLine represents a single line in a diff hunk.
type LocalDiffLine struct {
	Content   string `json:"content"`
	LineType  string `json:"line_type"` // 'added', 'deleted', 'context'
	OldLineNo int    `json:"old_line_no"`
	NewLineNo int    `json:"new_line_no"`
}

// LocalDiffHunk represents a hunk of changes in a diff.
type LocalDiffHunk struct {
	OldStartLine int             `json:"old_start_line"`
	OldLineCount int             `json:"old_line_count"`
	NewStartLine int             `json:"new_start_line"`
	NewLineCount int             `json:"new_line_count"`
	HeaderText   string          `json:"header_text"`
	Lines        []LocalDiffLine `json:"lines"`
}

// LocalCodeDiff represents the parsed diff for a single file.
type LocalCodeDiff struct {
	OldPath string          `json:"old_path"`
	NewPath string          `json:"new_path"`
	Hunks   []LocalDiffHunk `json:"hunks"`
}
