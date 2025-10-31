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
	t.Logf("Index has %d entries (files + general)", len(index))

	totalComments := 0
	generalComments := 0
	for filePath, comments := range index {
		totalComments += len(comments)
		if filePath == "" {
			generalComments = len(comments)
			t.Logf("  [General comments]: %d comments", len(comments))
		} else {
			t.Logf("  %s: %d comments", filePath, len(comments))
		}
	}
	t.Logf("Total comments in index: %d (general: %d, file-specific: %d)",
		totalComments, generalComments, totalComments-generalComments)

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

	t.Logf("Built index with %d entries (files + general)", len(index))

	// Count and categorize
	totalComments := 0
	generalCount := 0

	// Show index contents
	for filePath, comments := range index {
		totalComments += len(comments)
		if filePath == "" {
			generalCount = len(comments)
			t.Logf("General comments: %d", len(comments))
		} else {
			t.Logf("File: %s -> %d comments", filePath, len(comments))
		}
	}

	t.Logf("Total: %d comments (%d general, %d file-specific)",
		totalComments, generalCount, totalComments-generalCount)
}

// TestBuildFileCommentTree tests building the tree structure per file
func TestBuildFileCommentTree(t *testing.T) {
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

	// Build the tree
	tree := BuildFileCommentTree(&artifact)

	t.Logf("Built tree with %d entries (files + general)", len(tree))

	// Validate the tree
	validation := ValidateFileCommentTree(tree, &artifact)

	t.Logf("Validation results:")
	t.Logf("  Total comments in tree: %d", validation["total_tree_comments"])
	t.Logf("  Total comments in artifact: %d", validation["total_artifact_comments"])
	t.Logf("  All comments preserved: %v", validation["all_comments_preserved"])
	t.Logf("  File count: %d", validation["file_count"])

	threadsPerFile := validation["threads_per_file"].(map[string]int)
	t.Logf("  Threads per file:")
	for file, count := range threadsPerFile {
		t.Logf("    %s: %d thread(s)", file, count)
	}

	// Verify all comments are preserved
	if !validation["all_comments_preserved"].(bool) {
		t.Errorf("Not all comments preserved! Tree has %d but artifact has %d",
			validation["total_tree_comments"], validation["total_artifact_comments"])
	}
}

// TestShowCommentsPerFileTree tests showing the tree structure
func TestShowCommentsPerFileTree(t *testing.T) {
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

	// Show comments per file with tree structure
	ShowCommentsPerFileTree(&artifact)
}

// TestShowCommentsPerFileTreeBitbucket tests with Bitbucket data that has threaded comments
func TestShowCommentsPerFileTreeBitbucket(t *testing.T) {
	artifactPath := "../../../artifacts/bb_unified.json"

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

	t.Logf("Loaded Bitbucket UnifiedArtifact with %d diffs, %d comment roots",
		len(artifact.Diffs), len(artifact.CommentTree.Roots))

	// Build and validate tree
	tree := BuildFileCommentTree(&artifact)
	validation := ValidateFileCommentTree(tree, &artifact)

	t.Logf("Validation: %d comments in tree, %d in artifact, preserved: %v",
		validation["total_tree_comments"], validation["total_artifact_comments"],
		validation["all_comments_preserved"])

	// Verify we have threaded comments
	foundThreadedComment := false
	for _, roots := range tree {
		for _, root := range roots {
			if len(root.Children) > 0 {
				foundThreadedComment = true
				t.Logf("Found threaded comment: root ID=%s has %d children", root.ID, len(root.Children))
				for i, child := range root.Children {
					t.Logf("  Child %d: ID=%s, ParentID=%s", i+1, child.ID, child.ParentID)
				}
			}
		}
	}

	if !foundThreadedComment {
		t.Error("Expected to find threaded comments but found none")
	}

	// Show comments per file with tree structure
	ShowCommentsPerFileTree(&artifact)
}

// TestShowCommentsPerFileTreeGitlab tests with GitLab data
func TestShowCommentsPerFileTreeGitlab(t *testing.T) {
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

	t.Logf("Loaded GitLab UnifiedArtifact with %d diffs, %d comment roots",
		len(artifact.Diffs), len(artifact.CommentTree.Roots))

	// Build and validate tree
	tree := BuildFileCommentTree(&artifact)
	validation := ValidateFileCommentTree(tree, &artifact)

	t.Logf("Validation: %d comments in tree, %d in artifact, preserved: %v",
		validation["total_tree_comments"], validation["total_artifact_comments"],
		validation["all_comments_preserved"])

	// Check for threaded comments
	foundThreadedComment := false
	threadCount := 0
	for _, roots := range tree {
		for _, root := range roots {
			if len(root.Children) > 0 {
				foundThreadedComment = true
				threadCount++
				t.Logf("Found threaded comment: root ID=%s has %d children", root.ID, len(root.Children))
			}
		}
	}

	if foundThreadedComment {
		t.Logf("Total threaded comments found: %d", threadCount)
	} else {
		t.Logf("No threaded comments found (all comments are top-level)")
	}

	// Show comments per file with tree structure
	ShowCommentsPerFileTree(&artifact)
}

// TestShowCommentsPerFileTreeGithub tests with GitHub data
func TestShowCommentsPerFileTreeGithub(t *testing.T) {
	artifactPath := "../../../artifacts/gh_unified.json"

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

	t.Logf("Loaded GitHub UnifiedArtifact with %d diffs, %d comment roots",
		len(artifact.Diffs), len(artifact.CommentTree.Roots))

	// Build and validate tree
	tree := BuildFileCommentTree(&artifact)
	validation := ValidateFileCommentTree(tree, &artifact)

	t.Logf("Validation: %d comments in tree, %d in artifact, preserved: %v",
		validation["total_tree_comments"], validation["total_artifact_comments"],
		validation["all_comments_preserved"])

	// Check for threaded comments
	foundThreadedComment := false
	threadCount := 0
	for _, roots := range tree {
		for _, root := range roots {
			if len(root.Children) > 0 {
				foundThreadedComment = true
				threadCount++
				t.Logf("Found threaded comment: root ID=%s has %d children", root.ID, len(root.Children))
			}
		}
	}

	if foundThreadedComment {
		t.Logf("Total threaded comments found: %d", threadCount)
	} else {
		t.Logf("No threaded comments found (all comments are top-level)")
	}

	// Show comments per file with tree structure
	ShowCommentsPerFileTree(&artifact)
}
