package gitlab

import (
	"fmt"
	"testing"
)

// TestDiscoverProjectsGitlab tests the discover_projects_gitlab function
func TestDiscoverProjectsGitlab(t *testing.T) {
	// Hardcoded test values
	baseURL := "https://git.apps.hexmos.com"
	pat := "REDACTED_GITLAB_PAT_3"

	// Call the function
	projects, err := DiscoverProjectsGitlab(baseURL, pat)
	if err != nil {
		t.Fatalf("Failed to discover projects: %v", err)
	}

	// Verify we got some projects
	if len(projects) == 0 {
		t.Error("Expected at least one project, got none")
	}

	// Print results for manual verification
	fmt.Printf("Discovered %d projects:\n", len(projects))
	for i, project := range projects {
		fmt.Printf("%d. %s\n", i+1, project)
	}
}
