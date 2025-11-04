package prompts

import (
	"fmt"
	"strings"
)

// BuildSummarySection formats structured technical summaries into a textual
// block that feeds the final summary LLM prompt.
func BuildSummarySection(entries []TechnicalSummary) string {
	var b strings.Builder

	b.WriteString("File-level technical summaries:\n")

	written := false
	for _, entry := range entries {
		summary := strings.TrimSpace(entry.Summary)
		if summary == "" && len(entry.KeyChanges) == 0 {
			continue
		}

		label := strings.TrimSpace(entry.FilePath)
		if label == "" {
			label = "Repository-wide"
		}

		b.WriteString(fmt.Sprintf("- %s\n", label))
		if summary != "" {
			b.WriteString(fmt.Sprintf("  Summary: %s\n", summary))
		}
		for _, change := range entry.KeyChanges {
			changeText := strings.TrimSpace(change)
			if changeText == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("  * %s\n", changeText))
		}
		written = true
	}

	if !written {
		b.WriteString("- Repository-wide\n")
		b.WriteString("  Summary: No substantive technical summaries were captured.\n")
	}

	return b.String()
}
