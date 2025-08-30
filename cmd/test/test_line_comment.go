package main

import (
	"fmt"
	"os"
	"time"

	"github.com/livereview/internal/providers/gitlab"
)

// Test script to verify line-specific comments on GitLab
// This tests the fixed implementation of line comments
func main_test_line_comment() {
	fmt.Println("Testing fixed GitLab line comment functionality")

	// Configuration
	config := gitlab.GitLabConfig{
		URL:   "https://git.apps.hexmos.com",
		Token: "REDACTED_GITLAB_PAT_6",
	}

	// Create a GitLab client
	httpClient := gitlab.NewHTTPClient(config.URL, config.Token)
	fmt.Println("Created GitLab HTTP client")

	// Target information for MR 335 and metric_analysis.go line 29
	projectID := "hexmos/liveapi"
	mrIID := 335
	filePath := "liveapi-backend/exam/metric_analysis.go"
	lineNum := 29 // Target line 29 (was 19 before)

	// Create a test comment with timestamp
	timestamp := time.Now().Format(time.RFC3339)
	comment := fmt.Sprintf("FIXED IMPLEMENTATION TEST: This comment should appear on line %d of metric_analysis.go in the Changes tab. Created at %s", lineNum, timestamp)

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
