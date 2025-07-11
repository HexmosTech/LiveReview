package gitlab_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livereview/internal/providers/gitlab"
	"github.com/stretchr/testify/assert"
)

func TestGetMergeRequestContext(t *testing.T) {
	// Create a mock GitLab server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the path for easier checking
		path := r.URL.Path

		// Check if the path contains the project and merge request ID
		if path == "/api/v4/projects/group%2Fproject/merge_requests/456" ||
			path == "/api/v4/projects/group/project/merge_requests/456" {
			// Return a mock MR response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"id": 456,
				"iid": 456,
				"project_id": 123,
				"title": "Test Merge Request",
				"description": "This is a test MR",
				"source_branch": "feature-branch",
				"target_branch": "main",
				"web_url": "https://gitlab.example.com/group/project/-/merge_requests/456"
			}`))
		} else if path == "/api/v4/projects/group%2Fproject/merge_requests/456/changes" ||
			path == "/api/v4/projects/group/project/merge_requests/456/changes" {
			// Return mock changes
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"changes": [
					{
						"old_path": "file1.go",
						"new_path": "file1.go",
						"diff": "--- a/file1.go\n+++ b/file1.go\n@@ -1,3 +1,4 @@\n func OldFunction() {\n+    // New comment\n     fmt.Println(\"Hello\")\n }"
					}
				]
			}`))
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

	// Test fetching MR context
	ctx := context.Background()
	mrDetails, err := provider.GetMergeRequestDetails(ctx, mockServer.URL+"/group/project/-/merge_requests/456")
	assert.NoError(t, err)

	// Verify the MR details
	assert.Equal(t, "Test Merge Request", mrDetails.Title)
	assert.Equal(t, "This is a test MR", mrDetails.Description)

	// Now test getting changes
	diffs, err := provider.GetMergeRequestChanges(ctx, "456")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(diffs))
	assert.Equal(t, "file1.go", diffs[0].FilePath)
}
