package prompts

import (
	"fmt"
	"strings"

	"github.com/livereview/pkg/models"
)

// PromptBuilder provides methods for building different types of AI prompts
type PromptBuilder struct{}

// NewPromptBuilder creates a new prompt builder instance
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// BuildCodeReviewPrompt generates a comprehensive code review prompt
// This is the main prompt used by the trigger-review endpoint
func (pb *PromptBuilder) BuildCodeReviewPrompt(diffs []*models.CodeDiff) string {
	var prompt strings.Builder

	// System role and instructions
	prompt.WriteString("# Code Review Request\n\n")
	prompt.WriteString(CodeReviewInstructions)
	prompt.WriteString("\n\n")

	// Review guidelines
	prompt.WriteString(ReviewGuidelines)
	prompt.WriteString("\n\n")

	// Output format requirements
	prompt.WriteString(CommentRequirements)
	prompt.WriteString("\n\n")

	// JSON structure specification
	prompt.WriteString(JSONStructureExample)
	prompt.WriteString("\n\n")

	// Comment classification guidelines
	prompt.WriteString(CommentClassification)
	prompt.WriteString("\n\n")

	// Line number interpretation instructions
	prompt.WriteString(LineNumberInstructions)
	prompt.WriteString("\n\n")

	// Code changes section
	prompt.WriteString(CodeChangesHeader)
	prompt.WriteString("\n\n")
	pb.addCodeDiffs(&prompt, diffs)

	return prompt.String()
}

// BuildSummaryPrompt generates a prompt for synthesizing high-level summaries
func (pb *PromptBuilder) BuildSummaryPrompt(fileSummaries []string, comments []*models.ReviewComment) string {
	var prompt strings.Builder

	prompt.WriteString(SummaryWriterRole)
	prompt.WriteString(" using proper markdown formatting.\n\n")

	// Requirements
	prompt.WriteString(SummaryRequirements)
	prompt.WriteString("\n\n")

	// Add file summaries
	prompt.WriteString("File-level summaries:\n")
	for _, fs := range fileSummaries {
		prompt.WriteString("- " + fs + "\n")
	}

	// Add line comments
	prompt.WriteString("\nLine comments:\n")
	for _, c := range comments {
		prompt.WriteString(fmt.Sprintf("- [%s:%d] %s\n", c.FilePath, c.Line, c.Content))
	}

	// Expected output structure
	prompt.WriteString("\n")
	prompt.WriteString(SummaryStructure)

	return prompt.String()
}

// addCodeDiffs adds the actual code changes to the prompt
func (pb *PromptBuilder) addCodeDiffs(prompt *strings.Builder, diffs []*models.CodeDiff) {
	for _, diff := range diffs {
		prompt.WriteString(fmt.Sprintf("%s%s\n", FilePrefix, diff.FilePath))

		if diff.IsNew {
			prompt.WriteString(NewFileMarker + "\n")
		} else if diff.IsDeleted {
			prompt.WriteString(DeletedFileMarker + "\n")
		} else if diff.IsRenamed {
			prompt.WriteString(fmt.Sprintf("%s%s%s\n", RenamedFilePrefix, diff.OldFilePath, RenamedFileSuffix))
		}
		prompt.WriteString("\n")

		for _, hunk := range diff.Hunks {
			prompt.WriteString("```diff\n")
			prompt.WriteString(hunk.Content)
			prompt.WriteString("\n```\n\n")
		}
	}
}
