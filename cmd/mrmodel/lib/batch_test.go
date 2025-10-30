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
	t.Logf("Listing all diff paths:")

	// List all paths
	ListPaths(&artifact)
}
