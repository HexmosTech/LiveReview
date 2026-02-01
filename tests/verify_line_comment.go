package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/livereview/internal/providers/gitlab"
)

// Test script to verify line-specific comments on GitLab
func main() {
	fmt.Println("Testing GitLab line comment functionality")

	// Get GitLab URL and token from environment variables
	gitlabURL := os.Getenv("GITLAB_URL")
	// Test token for git.apps.hexmos.com - hardcoded for testing only
	gitlabToken := "REDACTED_GITLAB_PAT_6"

	if gitlabURL == "" || gitlabToken == "" {
		fmt.Println("Error: GITLAB_URL and GITLAB_TOKEN environment variables must be set")
		fmt.Println("Example:")
		fmt.Println("  export GITLAB_URL=https://git.example.com")
		fmt.Println("  export GITLAB_TOKEN=your_access_token")
		os.Exit(1)
	}

	// Check command line arguments
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run verify_line_comment.go <project_id> <mr_iid> <file_path> [line_number]")
		fmt.Println("Example: go run verify_line_comment.go hexmos/liveapi 335 liveapi-backend/exam/metric_analysis.go 19")
		os.Exit(1)
	}

	// Get parameters from command line
	projectID := os.Args[1]
	mrIID := 0
	fmt.Sscanf(os.Args[2], "%d", &mrIID)
	filePath := os.Args[3]
	lineNum := 1 // Default to line 1
	if len(os.Args) > 4 {
		fmt.Sscanf(os.Args[4], "%d", &lineNum)
	}

	// Configuration
	config := gitlab.GitLabConfig{
		URL:   gitlabURL,
		Token: gitlabToken,
	}

	// Create a GitLab client
	httpClient := gitlab.NewHTTPClient(config.URL, config.Token)
	fmt.Println("Created GitLab HTTP client")

	// Create a test comment with timestamp
	timestamp := time.Now().Format(time.RFC3339)
	comment := fmt.Sprintf("TEST COMMENT: This is a test comment at %s using improved line_code generation", timestamp)

	// Try to post the comment
	fmt.Println("Posting line comment to:")
	fmt.Printf("- Project: %s\n", projectID)
	fmt.Printf("- MR: %d\n", mrIID)
	fmt.Printf("- File: %s\n", filePath)
	fmt.Printf("- Line: %d\n", lineNum)

	// Get the MR versions to verify they're accessible
	versions, err := httpClient.GetMergeRequestVersions(projectID, mrIID)
	if err != nil {
		fmt.Printf("Error getting MR versions: %v\n", err)
		os.Exit(1)
	}

	if len(versions) == 0 {
		fmt.Println("Error: No versions found for this merge request")
		os.Exit(1)
	}

	// Show version information
	latestVersion := versions[0]
	fmt.Println("MR version information:")
	fmt.Printf("- Base SHA: %s\n", latestVersion.BaseCommitSHA)
	fmt.Printf("- Head SHA: %s\n", latestVersion.HeadCommitSHA)
	fmt.Printf("- Start SHA: %s\n", latestVersion.StartCommitSHA)

	// Generate the line_code that will be used
	lineCode := gitlab.GenerateLineCode(latestVersion.StartCommitSHA, latestVersion.HeadCommitSHA, filePath, lineNum)
	parts := strings.Split(lineCode, ":")
	fmt.Println("Line code formats:")
	fmt.Printf("- Combined format: %s\n", lineCode)
	fmt.Printf("- New style: %s\n", parts[0])
	if len(parts) > 1 {
		fmt.Printf("- Old style: %s\n", parts[1])
	}

	err = httpClient.CreateMRLineComment(projectID, mrIID, filePath, lineNum, comment)
	if err != nil {
		fmt.Printf("Error posting comment: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Comment posted successfully! Check GitLab to verify it's attached to the correct line.")
}
