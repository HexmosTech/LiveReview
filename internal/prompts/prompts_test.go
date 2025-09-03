package prompts

import (
	"fmt"
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptBuilder_BuildCodeReviewPrompt(t *testing.T) {
	builder := NewPromptBuilder()

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

	// Generate prompt
	prompt := builder.BuildCodeReviewPrompt(testDiffs)

	// Verify the prompt contains important sections
	assert.Contains(t, prompt, "Code Review Request")
	assert.Contains(t, prompt, "Review the following code changes thoroughly")
	assert.Contains(t, prompt, "IMPORTANT REVIEW GUIDELINES")
	assert.Contains(t, prompt, "Format your response as JSON")
	assert.Contains(t, prompt, "COMMENT CLASSIFICATION")
	assert.Contains(t, prompt, "CRITICAL: LINE NUMBER REFERENCES")
	assert.Contains(t, prompt, "# Code Changes")

	// Verify it contains file information
	assert.Contains(t, prompt, "test/file1.go")
	assert.Contains(t, prompt, "test/file2.go")
	assert.Contains(t, prompt, "(New file)")

	// Verify it contains diff content
	assert.Contains(t, prompt, "// New comment")
	assert.Contains(t, prompt, "func NewFunction()")
	assert.Contains(t, prompt, "```diff")

	// Verify structure
	lines := strings.Split(prompt, "\n")
	assert.True(t, len(lines) > 50, "Prompt should be comprehensive")

	// Check that JSON structure is properly included
	assert.Contains(t, prompt, "fileSummary")
	assert.Contains(t, prompt, "lineNumber")
	assert.Contains(t, prompt, "isInternal")
}

func TestPromptBuilder_BuildSummaryPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	fileSummaries := []string{
		"Added new authentication module",
		"Updated database schema",
		"Fixed security vulnerability in login",
	}

	comments := []*models.ReviewComment{
		{
			FilePath: "auth.go",
			Line:     42,
			Content:  "Consider using bcrypt for password hashing",
		},
		{
			FilePath: "db.go",
			Line:     15,
			Content:  "Add migration for this schema change",
		},
	}

	// Generate summary prompt
	prompt := builder.BuildSummaryPrompt(fileSummaries, comments)

	// Verify prompt contains key elements
	assert.Contains(t, prompt, "expert code reviewer")
	assert.Contains(t, prompt, "synthesize a single, high-level summary")
	assert.Contains(t, prompt, "REQUIREMENTS:")
	assert.Contains(t, prompt, "markdown formatting")

	// Verify file summaries are included
	assert.Contains(t, prompt, "Added new authentication module")
	assert.Contains(t, prompt, "Updated database schema")
	assert.Contains(t, prompt, "Fixed security vulnerability")

	// Verify comments are included
	assert.Contains(t, prompt, "[auth.go:42]")
	assert.Contains(t, prompt, "bcrypt for password hashing")
	assert.Contains(t, prompt, "[db.go:15]")
	assert.Contains(t, prompt, "migration for this schema")

	// Verify structure template is included
	assert.Contains(t, prompt, "## Overview")
	assert.Contains(t, prompt, "## Key Changes")
	assert.Contains(t, prompt, "## Impact")
}

func TestPromptBuilder_EmptyDiffs(t *testing.T) {
	builder := NewPromptBuilder()

	// Test with empty diffs
	prompt := builder.BuildCodeReviewPrompt([]*models.CodeDiff{})

	// Should still contain all the instructions
	assert.Contains(t, prompt, "Code Review Request")
	assert.Contains(t, prompt, "# Code Changes")

	// But no actual file content
	assert.NotContains(t, prompt, "## File:")
}

func TestPromptBuilder_AddCodeDiffs(t *testing.T) {
	builder := NewPromptBuilder()

	// Test different file scenarios
	testDiffs := []*models.CodeDiff{
		{
			FilePath: "new_file.go",
			IsNew:    true,
			Hunks:    []models.DiffHunk{{Content: "+new content"}},
		},
		{
			FilePath:  "deleted_file.go",
			IsDeleted: true,
			Hunks:     []models.DiffHunk{{Content: "-deleted content"}},
		},
		{
			FilePath:    "renamed_file.go",
			OldFilePath: "old_name.go",
			IsRenamed:   true,
			Hunks:       []models.DiffHunk{{Content: " unchanged"}},
		},
	}

	var prompt strings.Builder
	builder.addCodeDiffs(&prompt, testDiffs)
	result := prompt.String()

	// Verify file markers
	assert.Contains(t, result, "## File: new_file.go")
	assert.Contains(t, result, "(New file)")

	assert.Contains(t, result, "## File: deleted_file.go")
	assert.Contains(t, result, "(Deleted file)")

	assert.Contains(t, result, "## File: renamed_file.go")
	assert.Contains(t, result, "(Renamed from: old_name.go)")

	// Verify diff blocks
	assert.Contains(t, result, "```diff")
	assert.Contains(t, result, "+new content")
	assert.Contains(t, result, "-deleted content")
}

func TestPromptBuilder_SingletonBuilder(t *testing.T) {
	// Test that we can create multiple builders
	builder1 := NewPromptBuilder()
	builder2 := NewPromptBuilder()

	require.NotNil(t, builder1)
	require.NotNil(t, builder2)

	// They should be separate instances
	assert.NotSame(t, builder1, builder2)
}

func TestTemplateConstants(t *testing.T) {
	// Verify all template constants are non-empty
	assert.NotEmpty(t, CodeReviewerRole)
	assert.NotEmpty(t, SummaryWriterRole)
	assert.NotEmpty(t, CodeReviewInstructions)
	assert.NotEmpty(t, ReviewGuidelines)
	assert.NotEmpty(t, CommentRequirements)
	assert.NotEmpty(t, JSONStructureExample)
	assert.NotEmpty(t, CommentClassification)
	assert.NotEmpty(t, LineNumberInstructions)
	assert.NotEmpty(t, SummaryRequirements)
	assert.NotEmpty(t, SummaryStructure)

	// Verify specific content
	assert.Contains(t, CodeReviewerRole, "expert code reviewer")
	assert.Contains(t, JSONStructureExample, "filePath")
	assert.Contains(t, JSONStructureExample, "lineNumber")
	assert.Contains(t, JSONStructureExample, "isInternal")
}

// Benchmark test for large diffs
func BenchmarkBuildCodeReviewPrompt(b *testing.B) {
	builder := NewPromptBuilder()

	// Create a large diff set
	var diffs []*models.CodeDiff
	for i := 0; i < 100; i++ {
		diffs = append(diffs, &models.CodeDiff{
			FilePath: fmt.Sprintf("file%d.go", i),
			Hunks: []models.DiffHunk{
				{Content: strings.Repeat("+added line\n", 50)},
			},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = builder.BuildCodeReviewPrompt(diffs)
	}
}
