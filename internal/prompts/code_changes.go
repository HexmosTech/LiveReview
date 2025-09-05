package prompts

import (
	"fmt"
	"strings"

	"github.com/livereview/pkg/models"
)

// BuildCodeChangesSection returns the markdown section for code changes
// identical to what the legacy PromptBuilder used to append.
func BuildCodeChangesSection(diffs []*models.CodeDiff) string {
	var b strings.Builder
	b.WriteString("# Code Changes\n\n")
	for _, diff := range diffs {
		b.WriteString(fmt.Sprintf("%s%s\n", FilePrefix, diff.FilePath))

		if diff.IsNew {
			b.WriteString(NewFileMarker + "\n")
		} else if diff.IsDeleted {
			b.WriteString(DeletedFileMarker + "\n")
		} else if diff.IsRenamed {
			b.WriteString(fmt.Sprintf("%s%s%s\n", RenamedFilePrefix, diff.OldFilePath, RenamedFileSuffix))
		}
		b.WriteString("\n")

		for _, hunk := range diff.Hunks {
			b.WriteString("```diff\n")
			b.WriteString(hunk.Content)
			b.WriteString("\n```\n\n")
		}
	}
	return b.String()
}
