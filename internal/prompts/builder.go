package prompts

import (
	"context"
	"encoding/json"
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
func (pb *PromptBuilder) BuildSummaryPrompt(entries []TechnicalSummary) string {
	base, err := NewManager(nil, vendorpack.New()).Render(context.Background(), Context{OrgID: 0}, "summary", nil)
	if err != nil {
		return ""
	}
	return base + "\n\n" + BuildSummarySection(entries) + "\n\n" + SummaryStructure
}

// ToolFindingInput represents raw linter finding details passed into the classifier
type ToolFindingInput struct {
	ToolName    string `json:"tool_name"`
	RuleID      string `json:"rule_id"`
	FilePath    string `json:"file_path"`
	LineNumber  int    `json:"line_number"`
	Message     string `json:"message"`
	CodeSnippet string `json:"code_snippet"`
}

// BuildToolFindingClassificationPrompt composes a minimal classification prompt
// reusing the authoritative TaxonomyClassificationRules, CommentClassification,
// and CommentRequirements constants.
func (pb *PromptBuilder) BuildToolFindingClassificationPrompt(finding ToolFindingInput) string {
	var sb strings.Builder
	sb.WriteString("You are an expert code analysis classifier. Classify the following static analysis tool finding into the LiveReview Taxonomy.\n\n")
	sb.WriteString(TaxonomyClassificationRules)
	sb.WriteString("\n\n")
	sb.WriteString(CommentClassification)
	sb.WriteString("\n\n")
	sb.WriteString(CommentRequirements)
	sb.WriteString("\n\nRAW TOOL FINDING (JSON):\n```json\n")
	findingBytes, err := json.MarshalIndent(finding, "", "  ")
	if err != nil {
		fallbackObj := map[string]interface{}{
			"tool_name":   finding.ToolName,
			"rule_id":     finding.RuleID,
			"file_path":   finding.FilePath,
			"line_number": finding.LineNumber,
			"message":     finding.Message,
		}
		fallbackBytes, _ := json.Marshal(fallbackObj)
		sb.Write(fallbackBytes)
	} else {
		sb.Write(findingBytes)
	}
	sb.WriteString("\n```\n")
	sb.WriteString("\nFormat your response strictly as JSON with keys: category, subcategory, severity, type, confidence, isInternal.\n")
	sb.WriteString("- 'category' MUST be one of the 10 top-level categories in the taxonomy (e.g., Security, Reliability, Correctness).\n")
	sb.WriteString("- 'subcategory' MUST be one of the valid subcategories under that category (e.g., Secrets Management).\n")
	sb.WriteString("- 'severity' MUST be one of: critical, warning, info.\n")
	sb.WriteString("- 'type' MUST be one of: Bug, Risk, Optimization, Code Smell, Best Practice, Technical Debt.\n")
	sb.WriteString("- 'confidence' MUST be one of: High, Medium, Low.\n")
	return sb.String()
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
