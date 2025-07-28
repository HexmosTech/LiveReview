package gitlab

import (
	"fmt"
	"testing"
)

// TestDiscoverProjectsGitlab tests the discover_projects_gitlab function
func TestDiscoverProjectsGitlab(t *testing.T) {
	// Call the function
	projects := DiscoverProjectsGitlab()

	// Verify the response
	expectedCount := 5
	if len(projects) != expectedCount {
		t.Errorf("Expected %d projects, got %d", expectedCount, len(projects))
	}

	// Verify first project
	expectedFirst := "gitlab-org/gitlab"
	if len(projects) > 0 && projects[0] != expectedFirst {
		t.Errorf("Expected first project to be %s, got %s", expectedFirst, projects[0])
	}

	// Print results for manual verification
	fmt.Println("Discovered projects:")
	for i, project := range projects {
		fmt.Printf("%d. %s\n", i+1, project)
	}
}
