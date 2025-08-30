package main

import (
	"fmt"
	"os"
	"time"

	"github.com/livereview/internal/providers/gitlab"
)

// Test script to verify line-specific comments on GitLab
func main_verify_line_comment() {
	fmt.Println("Testing GitLab line comment functionality")

	// Configuration
	config := gitlab.GitLabConfig{
		URL:   "https://git.apps.hexmos.com",
		Token: "REDACTED_GITLAB_PAT_6",
	}

	// Create a GitLab client
	httpClient := gitlab.NewHTTPClient(config.URL, config.Token)
	fmt.Println("Created GitLab HTTP client")

	// Target information
	projectID := "hexmos/liveapi"
	mrIID := 335
	filePath := "liveapi-backend/exam/metric_analysis.go"
	lineNum := 19 // Target line 19

	// Create a test comment with timestamp
	timestamp := time.Now().Format(time.RFC3339)
	comment := fmt.Sprintf("FIXED TEST COMMENT: This is a test comment at %s targeting metric_analysis.go line 19", timestamp)

	// Try to post the comment
	fmt.Println("Posting line comment to:")
	fmt.Printf("- Project: %s\n", projectID)
	fmt.Printf("- MR: %d\n", mrIID)
	fmt.Printf("- File: %s\n", filePath)
	fmt.Printf("- Line: %d\n", lineNum)

	err := httpClient.CreateMRLineComment(projectID, mrIID, filePath, lineNum, comment)
	if err != nil {
		fmt.Printf("Error posting comment: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Comment posted successfully! Check GitLab to verify it's attached to the correct line.")
}
