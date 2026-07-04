package review

import (
	"strings"
	"testing"

	"github.com/livereview/pkg/models"
)

// TestBuildHelperPromptExcludesInternalComments guards against sending
// internal-only synthesis comments to the (billed) helper model. Only
// leaderResult.Comments (the external, user-visible set — internal comments
// live in the separate leaderResult.InternalComments slice and must never be
// passed in here) should ever reach the helper prompt.
func TestBuildHelperPromptExcludesInternalComments(t *testing.T) {
	leaderResult := &models.ReviewResult{
		Summary: "adds Java inheritance extraction; refactors repodag chain building for determinism",
		Comments: []*models.ReviewComment{
			{Content: "example shows `interfaces` keyword; ensure LLM handles both `class` and `interface` inheritance correctly"},
			{Content: "debug log commented out; might hide useful information for debugging missing file content"},
		},
		InternalComments: []*models.ReviewComment{
			{Content: "this internal-only synthesis note must never be billed to the helper model"},
			{Content: "another internal-only note that should be excluded from the helper payload"},
		},
	}

	prompt := buildHelperPrompt("concise_then_expand", leaderResult)

	for _, c := range leaderResult.Comments {
		if !strings.Contains(prompt, c.Content) {
			t.Errorf("expected prompt to include external comment %q, got: %s", c.Content, prompt)
		}
	}
	for _, c := range leaderResult.InternalComments {
		if strings.Contains(prompt, c.Content) {
			t.Errorf("expected prompt to exclude internal-only comment %q (it must not be billed to the helper), but it was present: %s", c.Content, prompt)
		}
	}
}
