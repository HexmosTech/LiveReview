package gitlab_test

import (
	"context"
	"encoding/json"
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
		if r.URL.Path == "/api/v4/projects/123/merge_requests/456/discussions" && r.Method == "POST" {
			// Parse the request body
			var requestBody map[string]interface{}
			json.NewDecoder(r.Body).Decode(&requestBody)
			postedComments = append(postedComments, requestBody)

			// Return a mock response
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": "mock-discussion-id"}`))
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
	ctx := context.Background()
	err = provider.PostComments(ctx, "123", "456", reviewResult)
	assert.NoError(t, err)

	// Verify comments were posted
	assert.Equal(t, 2, len(postedComments))

	// Verify first comment
	assert.Contains(t, postedComments[0]["body"], "Test comment 1")
	assert.Equal(t, "test/file1.go", postedComments[0]["position"].(map[string]interface{})["new_path"])
	assert.Equal(t, float64(10), postedComments[0]["position"].(map[string]interface{})["new_line"])

	// Verify second comment
	assert.Contains(t, postedComments[1]["body"], "Test comment 2")
	assert.Equal(t, "test/file2.go", postedComments[1]["position"].(map[string]interface{})["new_path"])
	assert.Equal(t, float64(20), postedComments[1]["position"].(map[string]interface{})["new_line"])
}
