package github

import (
	"fmt"
	"testing"
)

// TestDiscoverProjectsGitHub tests the discover_projects_github function
func TestDiscoverProjectsGitHub(t *testing.T) {
	// Test values - you should replace with valid credentials for testing
	baseURL := "https://github.com" // or your GitHub Enterprise URL
	pat := "your-github-pat-here"   // Replace with a valid GitHub PAT

	// Skip test if no PAT provided
	if pat == "your-github-pat-here" {
		t.Skip("Skipping test - no GitHub PAT provided")
	}

	// Call the function
	repositories, err := DiscoverProjectsGitHub(baseURL, pat)
	if err != nil {
		t.Fatalf("Failed to discover repositories: %v", err)
	}

	// Verify we got some repositories
	if len(repositories) == 0 {
		t.Error("Expected at least one repository, got none")
	}

	// Print results for manual verification
	fmt.Printf("Discovered %d repositories:\n", len(repositories))
	for i, repo := range repositories {
		fmt.Printf("%d. %s\n", i+1, repo)
	}
}
