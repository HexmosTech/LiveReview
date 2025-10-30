package lib

import (
	"encoding/json"
	"os"
	"testing"
)

// TestListPaths tests loading a unified artifact and listing its diff paths
func TestListPaths(t *testing.T) {
	artifactPath := "../../../artifacts/gl_unified.json"

	// Read the JSON file
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Failed to read artifact file: %v", err)
	}

	// Unmarshal into UnifiedArtifact
	var artifact UnifiedArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	t.Logf("Loaded UnifiedArtifact with %d diffs", len(artifact.Diffs))

	// List all paths
	paths := ListPaths(&artifact)
	t.Logf("Files in diff:")
	for _, p := range paths {
		t.Logf("%d. OldPath: %s, NewPath: %s", p.Index, p.OldPath, p.NewPath)
	}
}

// TestShowCommentsPerFile tests showing comments organized by file
func TestShowCommentsPerFile(t *testing.T) {
	artifactPath := "../../../artifacts/gl_unified.json"

	// Read the JSON file
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Failed to read artifact file: %v", err)
	}

	// Unmarshal into UnifiedArtifact
	var artifact UnifiedArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	t.Logf("Loaded UnifiedArtifact with %d diffs, %d comment roots",
		len(artifact.Diffs), len(artifact.CommentTree.Roots))

	// Build index and verify counts
	index := BuildFileCommentIndex(&artifact)
	t.Logf("Index has %d files with comments", len(index))

	totalComments := 0
	for filePath, comments := range index {
		totalComments += len(comments)
		t.Logf("  %s: %d comments", filePath, len(comments))
	}
	t.Logf("Total comments in index: %d", totalComments)

	// Show comments per file
	ShowCommentsPerFile(&artifact)
}

// TestBuildFileCommentIndex tests building the efficient file->comments index
func TestBuildFileCommentIndex(t *testing.T) {
	artifactPath := "../../../artifacts/gl_unified.json"

	// Read the JSON file
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Failed to read artifact file: %v", err)
	}

	// Unmarshal into UnifiedArtifact
	var artifact UnifiedArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Build the index
	index := BuildFileCommentIndex(&artifact)

	t.Logf("Built index with %d files having comments", len(index))

	// Show index contents
	for filePath, comments := range index {
		t.Logf("File: %s -> %d comments", filePath, len(comments))
	}
}
