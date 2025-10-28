package main

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
