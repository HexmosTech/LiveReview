package bitbucket

import (
	"fmt"
	"testing"
)

// TestDiscoverProjectsBitbucket tests the discover_projects_bitbucket function
func TestDiscoverProjectsBitbucket(t *testing.T) {
	// Test values - you should replace with valid credentials for testing
	baseURL := "https://bitbucket.org" // Bitbucket Cloud
	email := "your-email@example.com"  // Replace with your Atlassian account email
	apiToken := "your-api-token-here"  // Replace with a valid Atlassian API token

	// Skip test if no credentials provided
	if email == "your-email@example.com" || apiToken == "your-api-token-here" {
		t.Skip("Skipping test - no Bitbucket credentials provided")
	}

	// Call the function
	repositories, err := DiscoverProjectsBitbucket(baseURL, email, apiToken)
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
