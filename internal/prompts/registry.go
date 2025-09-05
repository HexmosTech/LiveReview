package prompts

// PlaintextTemplate is the shape returned for tooling to encrypt vendor templates.
type PlaintextTemplate struct {
	PromptKey string
	Provider  string // optional; empty means default
	Body      string
}

// PlaintextTemplates returns a small registry of vendor templates in plaintext.
// NOTE: In source-only; release builds embed encrypted versions instead.
func PlaintextTemplates() []PlaintextTemplate {
	return []PlaintextTemplate{
		{PromptKey: "code_review", Provider: "", Body: CodeReviewerRole + "\n\n" + CodeReviewInstructions + "\n\n{{VAR:style_guide}}\n\n{{VAR:security_guidelines}}\n\n" + JSONStructureExample},
		{PromptKey: "summary", Provider: "", Body: SummaryWriterRole + "\n\n" + SummaryRequirements + "\n\n" + SummaryStructure},
	}
}
