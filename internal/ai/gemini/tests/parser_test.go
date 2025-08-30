package gemini_test

import (
	"testing"

	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestParseResponse(t *testing.T) {
	// Setup a test provider
	provider, err := gemini.New(gemini.GeminiConfig{
		APIKey: "dummy-key",
	})
	assert.NoError(t, err)

	// Sample code diffs for testing
	testDiffs := []*models.CodeDiff{
		{
			FilePath: "test/file1.go",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 3,
					NewStartLine: 1,
					NewLineCount: 4,
					Content:      " func OldFunction() {\n+    // New comment\n     fmt.Println(\"Hello\")\n }",
				},
			},
		},
		{
			FilePath: "test/file2.go",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 10,
					OldLineCount: 2,
					NewStartLine: 10,
					NewLineCount: 3,
					Content:      " func AnotherFunction() {\n+    // Another new line\n     fmt.Println(\"World\")\n }",
				},
			},
		},
	}

	tests := []struct {
		name           string
		response       string
		expectedResult *models.ReviewResult
	}{
		{
			name: "Valid JSON Response",
			response: `{
				"summary": "This is a test summary",
				"filesChanged": ["test/file1.go", "test/file2.go"],
				"comments": [
					{
						"filePath": "test/file1.go",
						"lineNumber": 2,
						"content": "Good comment addition",
						"severity": "info",
						"suggestions": ["Maybe add more detail"]
					}
				]
			}`,
			expectedResult: &models.ReviewResult{
				Summary: "This is a test summary",
				Comments: []*models.ReviewComment{
					{
						FilePath:    "test/file1.go",
						Line:        2,
						Content:     "Good comment addition",
						Severity:    models.SeverityInfo,
						Suggestions: []string{"Maybe add more detail"},
						Category:    "review",
					},
					// A generic comment will be added for file2.go because it has no specific comments
				},
			},
		},
		{
			name: "Partial JSON Response",
			response: `I'll analyze the code changes.

			{
				"summary": "This is a partial JSON response",
				"comments": [
					{
						"filePath": "test/file1.go",
						"lineNumber": 2,
						"content": "Comment with incomplete JSON",
						"severity": "warning"
					}
				]
			}`,
			expectedResult: &models.ReviewResult{
				Summary: "This is a partial JSON response",
				Comments: []*models.ReviewComment{
					{
						FilePath:    "test/file1.go",
						Line:        2,
						Content:     "Comment with incomplete JSON",
						Severity:    models.SeverityWarning,
						Suggestions: []string{},
						Category:    "review",
					},
					// A generic comment will be added for file2.go because it has no specific comments
				},
			},
		},
		{
			name: "Text Response",
			response: `# AI Review Summary

This is a text-formatted summary.

## Specific Comments

FILE: test/file1.go, Line: 2
This is a comment about file1.
Severity: warning

FILE: test/file2.go, Line: 11
This is a comment about file2.
Severity: info`,
			expectedResult: &models.ReviewResult{
				Summary: "# AI Review Summary\n\nThis is a text-formatted summary.",
				Comments: []*models.ReviewComment{
					{
						FilePath: "test/file1.go",
						Line:     2,
						Content:  "This is a comment about file1.",
						Severity: models.SeverityWarning,
						Category: "review",
					},
					{
						FilePath: "test/file2.go",
						Line:     11,
						Content:  "This is a comment about file2.",
						Severity: models.SeverityInfo,
						Category: "review",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a method to expose parseResponse for testing
			result, err := provider.TestParseResponse(tt.response, testDiffs)
			assert.NoError(t, err)

			// Verify the summary
			assert.Equal(t, tt.expectedResult.Summary, result.Summary)

			// Verify we have the right number of comments (accounting for auto-generated ones)
			assert.GreaterOrEqual(t, len(result.Comments), len(tt.expectedResult.Comments))

			// For each expected comment, make sure it exists in the result
			for _, expectedComment := range tt.expectedResult.Comments {
				found := false
				for _, actualComment := range result.Comments {
					if actualComment.FilePath == expectedComment.FilePath &&
						actualComment.Line == expectedComment.Line &&
						actualComment.Severity == expectedComment.Severity &&
						actualComment.Content == expectedComment.Content {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected comment not found: %+v", expectedComment)
			}
		})
	}
}
