package review

import (
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

func TestBuildHelperPromptOmitsRedundantFields(t *testing.T) {
	leaderResult := &models.ReviewResult{
		Summary: "auth: token refresh race",
		Comments: []*models.ReviewComment{
			{
				FilePath:    "internal/auth/token.go",
				Line:        42,
				Content:     "refresh() no lock; concurrent calls double-fire",
				Severity:    models.SeverityCritical,
				Category:    "concurrency",
				Subcategory: "race-condition",
			},
		},
	}

	prompt := buildHelperPrompt("concise_then_expand", leaderResult)

	payloadStart := strings.Index(prompt, "Input review payload:")
	if payloadStart == -1 {
		t.Fatalf("expected prompt to contain the review payload section, got: %s", prompt)
	}
	payload := prompt[payloadStart:]

	if !strings.Contains(payload, leaderResult.Comments[0].Content) {
		t.Fatalf("expected payload to include the concise comment content, got: %s", payload)
	}
	// Only index and content are needed to expand wording; anything else is
	// billed tokens the helper model doesn't need.
	for _, field := range []string{"filePath", "internal/auth/token.go", "\"line\"", "\"severity\"", "critical", "\"category\"", "concurrency", "\"subcategory\"", "race-condition"} {
		if strings.Contains(payload, field) {
			t.Errorf("expected payload to omit %q (not needed for wording expansion), but it was present: %s", field, payload)
		}
	}
}
