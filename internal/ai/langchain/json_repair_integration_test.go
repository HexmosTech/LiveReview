package langchain

import (
	"context"
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

func TestParseResponseWithRepair_AppliesSanitizationAfterRepair(t *testing.T) {
	provider := &LangchainProvider{}
	diffs := []models.CodeDiff{
		{
			FilePath: "a.go",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 1,
					OldLineCount: 1,
					NewStartLine: 1,
					NewLineCount: 1,
					Content:      "@@ -1 +1 @@\n+line",
				},
			},
		},
	}

	// Trailing comma forces original parse to fail and triggers repair path.
	response := `{"fileSummaries":[{"filePath":"a.go","summary":"Contact alice@example.com"}],"comments":[{"filePath":"a.go","lineNumber":1,"content":"Use sk-12345678901234567890","severity":"warning","suggestions":["mail bob@example.com"],"isInternal":false}],}`

	result, err := provider.parseResponseWithRepair(context.Background(), response, diffs, 0, 0, "batch-test", nil)
	if err != nil {
		t.Fatalf("expected repaired parse to succeed, got error: %v", err)
	}

	if len(result.TechnicalSummaries) == 0 {
		t.Fatalf("expected technical summaries in parsed result")
	}
	if strings.Contains(result.TechnicalSummaries[0].Summary, "alice@example.com") {
		t.Fatalf("expected summary email to be redacted, got: %s", result.TechnicalSummaries[0].Summary)
	}

	if len(result.Comments) == 0 {
		t.Fatalf("expected comments in parsed result")
	}
	if strings.Contains(result.Comments[0].Content, "sk-12345678901234567890") {
		t.Fatalf("expected secret to be redacted in comment, got: %s", result.Comments[0].Content)
	}
	if len(result.Comments[0].Suggestions) == 0 {
		t.Fatalf("expected suggestions in parsed result")
	}
	if strings.Contains(result.Comments[0].Suggestions[0], "bob@example.com") {
		t.Fatalf("expected suggestion email to be redacted, got: %s", result.Comments[0].Suggestions[0])
	}
}
