package gitlab_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livereview/internal/providers/gitlab"
	"github.com/stretchr/testify/assert"
)

func TestGetMergeRequestContext(t *testing.T) {
	// Create a mock GitLab server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check the path to determine which API is being called
		if r.URL.Path == "/api/v4/projects/123/merge_requests/456" {
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
		} else if r.URL.Path == "/api/v4/projects/123/merge_requests/456/changes" {
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
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	// Create a GitLab provider with the mock server URL
	provider, err := gitlab.New(gitlab.GitLabConfig{
		APIBaseURL: mockServer.URL,
		APIToken:   "test-token",
	})
	assert.NoError(t, err)

	// Test fetching MR context
	ctx := context.Background()
	mrContext, err := provider.GetMergeRequestContext(ctx, "123", "456")
	assert.NoError(t, err)

	// Verify the MR context
	assert.Equal(t, "Test Merge Request", mrContext.Title)
	assert.Equal(t, "This is a test MR", mrContext.Description)
	assert.Equal(t, 1, len(mrContext.Diffs))
	assert.Equal(t, "file1.go", mrContext.Diffs[0].FilePath)
}
