//go:build !vendor_prompts

package prompts

// PlaintextTemplates returns plaintext vendor templates for dev/tooling only.
// This file is excluded from vendor builds by build tag.
func PlaintextTemplates() []PlaintextTemplate {
	return []PlaintextTemplate{
		{PromptKey: "code_review", Provider: "", Body: CodeReviewerRole + "\n\n" + CodeReviewInstructions + "\n\n{{VAR:style_guide}}\n\n{{VAR:security_guidelines}}\n\n" + JSONStructureExample},
		{PromptKey: "summary", Provider: "", Body: SummaryWriterRole + "\n\n" + SummaryRequirements + "\n\n" + SummaryStructure},
	}
}
