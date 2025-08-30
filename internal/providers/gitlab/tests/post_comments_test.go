package gitlab_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestPostComments(t *testing.T) {
	// Track the posted comments
	var postedComments []map[string]interface{}
	var mrVersionsRequested bool

	// Create a mock GitLab server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract the path for easier checking
		path := r.URL.Path

		// Log all requests for debugging
		fmt.Printf("Mock server received request: %s %s\n", r.Method, path)

		// Handle MR versions endpoint
		if path == "/api/v4/projects/group%2Fproject/merge_requests/456/versions" ||
			path == "/api/v4/projects/group/project/merge_requests/456/versions" {
			if r.Method == "GET" {
				mrVersionsRequested = true
				// Return mock MR version data
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Return sample data with the SHAs needed for line comments
				w.Write([]byte(`[
					{
						"id": 1,
						"head_commit_sha": "abcdef1234567890",
						"base_commit_sha": "1234567890abcdef",
						"start_commit_sha": "9876543210abcdef",
						"created_at": "2025-07-11T00:00:00Z",
						"merge_request_id": 456,
						"state": "collected",
						"real_size": "2"
					}
				]`))
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		} else if path == "/api/v4/projects/group%2Fproject/merge_requests/456/discussions" ||
			path == "/api/v4/projects/group/project/merge_requests/456/discussions" {
			if r.Method == "POST" {
				// Read the request body
				body, _ := io.ReadAll(r.Body)

				// Parse the request body
				var requestBody map[string]interface{}
				json.Unmarshal(body, &requestBody)

				// Log the request body for debugging
				fmt.Printf("Received comment data: %+v\n", requestBody)

				// Store the comment data for assertions
				postedComments = append(postedComments, requestBody)

				// Return a mock response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"id": "mock-discussion-id"}`))
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		} else if path == "/api/v4/projects/group%2Fproject/merge_requests/456/changes" ||
			path == "/api/v4/projects/group/project/merge_requests/456/changes" {
			if r.Method == "GET" {
				// Return mock changes data
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"id": 456,
					"iid": 456,
					"project_id": 123,
					"title": "Test MR",
					"changes": [
						{
							"old_path": "test/file1.go",
							"new_path": "test/file1.go",
							"diff": "@@ -1,10 +1,20 @@\npackage test\n\nfunc TestFunc() {\n\t// Test code\n}\n",
							"new_file": false,
							"renamed_file": false,
							"deleted_file": false
						},
						{
							"old_path": "test/file2.go",
							"new_path": "test/file2.go",
							"diff": "@@ -1,5 +1,25 @@\npackage test\n\nfunc AnotherTestFunc() {\n\t// More test code\n}\n",
							"new_file": false,
							"renamed_file": false,
							"deleted_file": false
						}
					]
				}`))
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		} else if path == "/api/v4/projects/group%2Fproject/merge_requests/456" ||
			path == "/api/v4/projects/group/project/merge_requests/456" {
			if r.Method == "GET" {
				// Return mock MR data
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{
					"id": 456,
					"iid": 456,
					"project_id": 123,
					"title": "Test MR",
					"description": "Test MR description",
					"state": "opened",
					"source_branch": "feature-branch",
					"target_branch": "main",
					"web_url": "http://example.com/group/project/-/merge_requests/456",
					"author": {
						"id": 1,
						"username": "testuser",
						"name": "Test User"
					}
				}`))
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
		"url":   mockServer.URL,
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

	// Create test review result with different comment types
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
				Suggestions: []string{
					"Consider adding error handling",
					"Use a more descriptive variable name",
				},
			},
			// Add a general comment without file path or line
			{
				Content:  "General comment on the entire MR",
				Severity: models.SeverityCritical,
			},
		},
	}

	// Test posting comments
	err = provider.PostComments(ctx, "456", reviewResult.Comments)
	assert.NoError(t, err)

	// Verify comments were posted
	assert.Equal(t, 3, len(postedComments), "Expected 3 comments to be posted")

	// Verify that MR versions endpoint was called
	assert.True(t, mrVersionsRequested, "Expected MR versions endpoint to be called")

	// Check that at least one comment has position data (for line comments)
	hasPositionData := false
	for _, comment := range postedComments {
		if position, ok := comment["position"].(map[string]interface{}); ok {
			hasPositionData = true
			// Check that position has required fields
			assert.Contains(t, position, "position_type", "Position should contain 'position_type'")
			assert.Contains(t, position, "new_path", "Position should contain 'new_path'")
			assert.Contains(t, position, "new_line", "Position should contain 'new_line'")
		}
	}
	assert.True(t, hasPositionData, "At least one comment should have position data")
}

// TestLineCommentSpecifics tests the specific functionality of creating line comments
func TestLineCommentSpecifics(t *testing.T) {
	// Track if MR versions endpoint was called
	mrVersionsCalled := false

	// Create a mock server that verifies correct parameters for line comments
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Handle MR versions endpoint
		if strings.HasSuffix(path, "/versions") {
			mrVersionsCalled = true
			// Return mock version data
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{
					"id": 1,
					"head_commit_sha": "headsha123456",
					"base_commit_sha": "basesha123456",
					"start_commit_sha": "startsha123456",
					"created_at": "2025-07-11T00:00:00Z",
					"merge_request_id": 123,
					"state": "collected",
					"real_size": "2"
				}
			]`))
		} else if strings.HasSuffix(path, "/discussions") {
			// Parse the request body to verify correct parameters
			var requestBody map[string]interface{}
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &requestBody)

			// Check if this is a line comment request
			if position, ok := requestBody["position"].(map[string]interface{}); ok {
				// Verify required position fields
				if position["position_type"] == "text" &&
					position["base_sha"] == "basesha123456" &&
					position["head_sha"] == "headsha123456" &&
					position["start_sha"] == "startsha123456" &&
					position["new_path"] != nil &&
					position["new_line"] != nil {
					// Position parameters look good
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					w.Write([]byte(`{"id": "new-discussion-id"}`))
					return
				}
			}

			// If we get here, position parameters weren't correct
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": "Invalid position parameters"}`))
		} else {
			// Return 404 for unhandled paths
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Not found"}`))
		}
	}))
	defer mockServer.Close()

	// Create a GitLab HTTP client
	client := gitlab.NewHTTPClient(mockServer.URL, "test-token")

	// Test creating a line comment
	err := client.CreateMRLineCommentWithPosition("testproject", 123, "src/main.go", 42, "This line needs improvement")

	// Verify that no error occurred
	assert.NoError(t, err, "Line comment creation should succeed")

	// Verify that MR versions endpoint was called
	assert.True(t, mrVersionsCalled, "MR versions endpoint should be called")
}
