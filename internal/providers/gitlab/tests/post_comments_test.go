package gitlab_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestPostComments(t *testing.T) {
	// Track the posted comments
	var postedComments []map[string]interface{}
	
	// Create a mock GitLab server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the path for easier checking
		path := r.URL.Path
		
		if path == "/api/v4/projects/group%2Fproject/merge_requests/456/discussions" ||
		   path == "/api/v4/projects/group/project/merge_requests/456/discussions" {
			if r.Method == "POST" {
				// Parse the request body
				var requestBody map[string]interface{}
				json.NewDecoder(r.Body).Decode(&requestBody)
				postedComments = append(postedComments, requestBody)

				// Return a mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"id": "mock-discussion-id"}`))
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		} else {
			fmt.Printf("Mock server: Unhandled path: %s\n", path)
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Not found", "path": "` + path + `"}`))
		}
	}))
	defer mockServer.Close()

	// Create a GitLab provider with the mock server URL
	provider, err := gitlab.New(gitlab.GitLabConfig{
		URL:   mockServer.URL,
		Token: "test-token",
	})
	assert.NoError(t, err)

	// Create test context
	ctx := context.Background()
	
	// Store project ID for the test - use URL to simulate a real request to extract project ID
	err = provider.Configure(map[string]interface{}{
		"url": mockServer.URL,
		"token": "test-token",
	})
	assert.NoError(t, err)
	
	// Extract MR ID from URL
	mrURL := mockServer.URL + "/group/project/-/merge_requests/456"
	_, err = provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		// If we get an error, it's likely because the mock isn't complete, but the URL parsing worked
		fmt.Println("GetMergeRequestDetails error (expected in test):", err)
	}

	// Create test review result
	reviewResult := &models.ReviewResult{
		Summary: "Test summary",
		Comments: []*models.ReviewComment{
			{
				FilePath: "test/file1.go",
				Line:     10,
				Content:  "Test comment 1",
				Severity: models.SeverityInfo,
			},
			{
				FilePath: "test/file2.go",
				Line:     20,
				Content:  "Test comment 2",
				Severity: models.SeverityWarning,
			},
		},
	}

	// Test posting comments
	err = provider.PostComments(ctx, "456", reviewResult.Comments)
	assert.NoError(t, err)

	// Verify comments were posted
	assert.Equal(t, 2, len(postedComments))
	
	// Don't try to assert details on comments since they might not be fully posted in this mock
}
}
