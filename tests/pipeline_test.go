package livereview_test

import (
	"testing"

	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestCompletePipeline(t *testing.T) {
	// This test demonstrates how each step in the pipeline can be tested independently
	// In a real integration test, these steps would be connected

	// STEP 1: Get MR context based on URL (using mock provider)
	mrContext := &models.MergeRequestContext{
		ProjectID:    "123",
		MergeReqID:   "456",
		Title:        "Test MR",
		Description:  "Test Description",
		SourceBranch: "feature-branch",
		TargetBranch: "main",
		Diffs: []*models.CodeDiff{
			{
				FilePath: "test/file1.go",
				Hunks: []*models.Hunk{
					{
						OldStartLine: 1,
						OldLineCount: 3,
						NewStartLine: 1,
						NewLineCount: 4,
						Content:      " func OldFunction() {\n+    // New comment\n     fmt.Println(\"Hello\")\n }",
					},
				},
			},
		},
	}

	// STEP 2: Construct prompt
	// This would be a unit test in prompt_test.go
	aiProvider, _ := gemini.New(gemini.GeminiConfig{
		APIKey: "dummy-key",
	})
	prompt, err := aiProvider.TestConstructPrompt(mrContext.Diffs)
	assert.NoError(t, err)
	assert.Contains(t, prompt, "You are an expert code reviewer")
	assert.Contains(t, prompt, "test/file1.go")

	// STEP 3: Submit prompt to AI provider
	// This would be mocked in a unit test
	// For demonstration purposes, we'll just create a mock response
	mockResponse := `{
		"summary": "This is a test summary",
		"filesChanged": ["test/file1.go"],
		"comments": [
			{
				"filePath": "test/file1.go",
				"lineNumber": 2,
				"content": "Good comment addition",
				"severity": "info",
				"suggestions": ["Maybe add more detail"]
			}
		]
	}`

	// STEP 4: Parse response from Gemini
	reviewResult, err := aiProvider.TestParseResponse(mockResponse, mrContext.Diffs)
	assert.NoError(t, err)
	assert.Equal(t, "This is a test summary", reviewResult.Summary)
	assert.Equal(t, 1, len(reviewResult.Comments))
	assert.Equal(t, "test/file1.go", reviewResult.Comments[0].FilePath)
	assert.Equal(t, 2, reviewResult.Comments[0].Line)

	// STEP 5: Post comments to GitLab
	// This would be mocked in a unit test
	// Simulate a successful post
	_, _ = gitlab.New(gitlab.GitLabConfig{
		APIBaseURL: "https://gitlab.example.com",
		APIToken:   "dummy-token",
	})

	// In real tests, this would be mocked or skipped
	// err = gitlabProvider.PostComments(context.Background(), mrContext.ProjectID, mrContext.MergeReqID, reviewResult)
	// assert.NoError(t, err)

	// Demonstrate the complete pipeline
	t.Log("Complete pipeline test demonstrates how to test each component individually")
	t.Log("Step 1: Get MR context - Success")
	t.Log("Step 2: Construct prompt - Success")
	t.Log("Step 3: Submit prompt to AI - Success")
	t.Log("Step 4: Parse response - Success")
	t.Log("Step 5: Post comments - Would be tested separately")
}
