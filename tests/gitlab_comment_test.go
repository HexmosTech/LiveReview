package livereview

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

// TestGitLabLineCommentMethods tests different methods of posting line-specific comments to GitLab
// to help debug why comments aren't being properly attached to specific lines in files.
//
// This test specifically targets the issue with comments not being attached to
// liveapi-backend/exam/metric_analysis.go line 19 in MR 335.
func TestGitLabLineCommentMethods(t *testing.T) {
	// Skip the test by default since it requires credentials and a real GitLab instance
	// Run with -test.run=TestGitLabLineCommentMethods -test.v to execute
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Replace these with your actual values from configuration
	// Test token for git.apps.hexmos.com - hardcoded for testing only
	token := "REDACTED_GITLAB_PAT_6"
	if token == "" {
		t.Skip("GITLAB_TOKEN environment variable not set, skipping test")
	}
	config := gitlab.GitLabConfig{
		URL:   "https://git.apps.hexmos.com", // Replace with your GitLab URL
		Token: token,
	}

	// Create a GitLab provider with your config
	provider, err := gitlab.New(config)
	assert.NoError(t, err)

	// Create test context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// The MR ID and URL
	mrID := "335"                                                              // MR ID 335
	mrURL := "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/335" // Full MR URL with project path
	filePath := "liveapi-backend/exam/metric_analysis.go"                      // File path
	lineNum := 19                                                              // Line number

	// Create a test comment that clearly shows which method was used
	reviewComment := &models.ReviewComment{
		FilePath: filePath,
		Line:     lineNum,
		Content:  "TEST COMMENT: This is a test comment posted via LiveReview debugging - " + time.Now().Format(time.RFC3339),
		Severity: models.SeverityInfo,
		Category: "test",
	}

	// Method 1: Use the standard PostComment method
	t.Run("StandardPostComment", func(t *testing.T) {
		// Reset the provider's project ID cache before testing with a new URL
		provider, err := gitlab.New(config)
		assert.NoError(t, err)

		// First get the MR details to properly set up the provider
		_, err = provider.GetMergeRequestDetails(ctx, mrURL)
		if err != nil {
			t.Logf("GetMergeRequestDetails failed: %v", err)
			t.Fail()
			return
		}

		err = provider.PostComment(ctx, mrID, reviewComment)
		if err != nil {
			t.Logf("PostComment failed: %v", err)
			t.Fail()
		} else {
			t.Logf("PostComment succeeded, check GitLab to see if it's attached correctly")
		}
	})

	// Let's also test the HTTP client methods directly for comparison
	httpClient := provider.GetHTTPClient()
	if httpClient == nil {
		t.Fatal("Failed to get HTTP client from provider")
	}

	// Extract project ID
	projectID := "hexmos/liveapi" // Replace with the actual project ID
	mrIID := 335                  // MR IID as an integer

	// Method 2: Directly use CreateMRLineComment
	t.Run("DirectMRLineComment", func(t *testing.T) {
		err := httpClient.CreateMRLineComment(projectID, mrIID, filePath, lineNum,
			"TEST COMMENT (Direct): This is a test comment posted via direct line comment - "+time.Now().Format(time.RFC3339))
		if err != nil {
			t.Logf("CreateMRLineComment failed: %v", err)
			t.Fail()
		} else {
			t.Logf("CreateMRLineComment succeeded, check GitLab to see if it's attached correctly")
		}
	})

	// Skip methods that require the internal functions for now

	// Try with different path prefix handling
	if !strings.HasPrefix(filePath, "/") {
		// Try with leading slash
		err = httpClient.CreateMRLineComment(projectID, mrIID, "/"+filePath, lineNum,
			"TEST COMMENT (with leading slash): Testing with leading slash in path - "+time.Now().Format(time.RFC3339))
		if err != nil {
			t.Logf("Leading slash path failed: %v", err)
		}
	}

	// Method 6: Try with the simplest approach possible - using 'path' and 'line' parameters directly
	t.Run("SimplestApproach", func(t *testing.T) {
		requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes",
			httpClient.GetBaseURL(), url.PathEscape(projectID), mrIID)

		// Create a direct request with minimal parameters
		values := url.Values{}
		values.Add("body", "TEST COMMENT (Simplest): Using minimal parameters - "+time.Now().Format(time.RFC3339))
		values.Add("path", filePath)
		values.Add("line", fmt.Sprintf("%d", lineNum))

		// Make the request directly
		req, err := http.NewRequest("POST", requestURL, strings.NewReader(values.Encode()))
		if err != nil {
			t.Logf("Failed to create request: %v", err)
			t.Fail()
			return
		}

		// Add headers
		req.Header.Add("PRIVATE-TOKEN", httpClient.GetToken())
		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

		// Execute the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Logf("Failed to execute request: %v", err)
			t.Fail()
			return
		}
		defer resp.Body.Close()

		// Check the response
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusCreated {
			t.Logf("API request failed with status %d: %s", resp.StatusCode, string(body))
			t.Fail()
		} else {
			t.Logf("Simplest approach succeeded with status %d", resp.StatusCode)
		}
	})
}
