package prompts

import (
	"context"
	"fmt"
	"strings"

	vendorpack "github.com/livereview/internal/prompts/vendor"
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
	// Delegate to Manager.Render("code_review") to avoid legacy assembly
	base, err := NewManager(nil, vendorpack.New()).Render(context.Background(), Context{OrgID: 0}, "code_review", nil)
	if err != nil {
		return ""
	}
	return base + "\n\n" + BuildCodeChangesSection(diffs)
}

// BuildSummaryPrompt generates a prompt for synthesizing high-level summaries
func (pb *PromptBuilder) BuildSummaryPrompt(fileSummaries []string, comments []*models.ReviewComment) string {
	base, err := NewManager(nil, vendorpack.New()).Render(context.Background(), Context{OrgID: 0}, "summary", nil)
	if err != nil {
		return ""
	}
	return base + "\n\n" + BuildSummarySection(fileSummaries, comments) + "\n\n" + SummaryStructure
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
