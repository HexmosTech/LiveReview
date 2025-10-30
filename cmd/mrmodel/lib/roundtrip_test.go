package lib

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// TestRoundTripUnifiedArtifact tests loading a saved UnifiedArtifact JSON back into the struct
func TestRoundTripUnifiedArtifact(t *testing.T) {
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

	// Verify basic structure
	if artifact.Provider == "" {
		t.Error("Provider should not be empty")
	}

	t.Logf("Successfully loaded UnifiedArtifact:")
	t.Logf("  Provider: %s", artifact.Provider)
	t.Logf("  Timeline items: %d", len(artifact.Timeline))
	t.Logf("  Comment tree roots: %d", len(artifact.CommentTree.Roots))
	t.Logf("  Diffs: %d", len(artifact.Diffs))
	t.Logf("  Participants: %d", len(artifact.Participants))

	// Marshal it back to JSON
	output, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal back to JSON: %v", err)
	}

	t.Logf("Successfully marshaled back to JSON (%d bytes)", len(output))

	// Verify round-trip: unmarshal the output and compare
	var roundtrip UnifiedArtifact
	if err := json.Unmarshal(output, &roundtrip); err != nil {
		t.Fatalf("Failed to unmarshal round-trip JSON: %v", err)
	}

	// Deep comparison of the two structures
	if diff := cmp.Diff(artifact, roundtrip); diff != "" {
		t.Errorf("Round-trip mismatch (-original +roundtrip):\n%s", diff)
	}

	t.Logf("Round-trip verification successful - all data preserved")
}
