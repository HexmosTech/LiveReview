package prompts

import (
	"fmt"
	"strings"

	"github.com/livereview/pkg/models"
)

// BuildSummarySection formats the file summaries and line comments section
// to append after the base summary prompt rendered by Manager.Render("summary").
func BuildSummarySection(fileSummaries []string, comments []*models.ReviewComment) string {
	var b strings.Builder

	// File summaries
	b.WriteString("File-level summaries:\n")
	for _, fs := range fileSummaries {
		if strings.TrimSpace(fs) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(fs)
		b.WriteString("\n")
	}

	// Line comments
	b.WriteString("\nLine comments:\n")
	for _, c := range comments {
		if c == nil {
			continue
		}
		// Keep close to legacy formatting used by tests/consumers
		b.WriteString(fmt.Sprintf("- [%s:%d] %s\n", c.FilePath, c.Line, c.Content))
	}

	return b.String()
}
