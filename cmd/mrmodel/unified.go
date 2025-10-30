package main

import "github.com/livereview/cmd/mrmodel/lib"

// UnifiedArtifact combines all processed data for a given merge/pull request
// from a specific provider. This is the canonical structure for downstream
// processing, such as prompt generation.
type UnifiedArtifact = lib.UnifiedArtifact

// LocalCodeDiff represents the parsed diff for a single file.
type LocalCodeDiff = lib.LocalCodeDiff
