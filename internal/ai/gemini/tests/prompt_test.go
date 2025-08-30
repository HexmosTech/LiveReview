package gemini_test

import (
	"testing"

	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestConstructPrompt(t *testing.T) {
	// Create a test provider
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
			IsNew:    true,
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 0,
					OldLineCount: 0,
					NewStartLine: 1,
					NewLineCount: 5,
					Content:      "+package test\n+\n+func NewFunction() {\n+    fmt.Println(\"New file\")\n+}",
				},
			},
		},
	}

	// Test prompt construction
	prompt, err := provider.TestConstructPrompt(testDiffs)
	assert.NoError(t, err)

	// Verify the prompt contains important sections
	assert.Contains(t, prompt, "You are an expert code reviewer")
	assert.Contains(t, prompt, "Format your response as a valid JSON object")
	assert.Contains(t, prompt, "\"summary\"")
	assert.Contains(t, prompt, "\"filesChanged\"")
	assert.Contains(t, prompt, "\"comments\"")

	// Verify it contains file information
	assert.Contains(t, prompt, "test/file1.go")
	assert.Contains(t, prompt, "test/file2.go")
	assert.Contains(t, prompt, "[NEW FILE]")

	// Verify it contains diff hunks
	assert.Contains(t, prompt, "@@ -L1,3 +L1,4 @@")
	assert.Contains(t, prompt, "// New comment")
	assert.Contains(t, prompt, "func NewFunction()")
}
