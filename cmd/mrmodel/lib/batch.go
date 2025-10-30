package lib

import "fmt"

// ListPaths prints a numbered list of all file paths from the diffs in a UnifiedArtifact
func ListPaths(artifact *UnifiedArtifact) {
	for i, diff := range artifact.Diffs {
		fmt.Printf("%d. OldPath: %s, NewPath: %s\n", i+1, diff.OldPath, diff.NewPath)
	}
}
