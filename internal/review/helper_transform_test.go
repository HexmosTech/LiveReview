package review

import (
	"context"
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

func TestApplyHelperStageSkipsLLMCallForEmptyComments(t *testing.T) {
	leaderResult := &models.ReviewResult{Summary: "no issues found"}

	result, usage, err := (&Service{}).applyHelperStage(context.Background(), AIConfig{}, "concise_then_expand", leaderResult)
	if err != nil {
		t.Fatalf("expected helper stage to succeed without an AI provider when there are no comments, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected a non-nil transformed result")
	}
	if len(result.Comments) != 0 {
		t.Fatalf("expected 0 comments, got %d", len(result.Comments))
	}
	if result.Summary != leaderResult.Summary {
		t.Fatalf("expected summary %q to be preserved, got %q", leaderResult.Summary, result.Summary)
	}
	if result == leaderResult {
		t.Fatal("expected a distinct clone of the leader result")
	}
	result.Summary = "mutated"
	if leaderResult.Summary != "no issues found" {
		t.Fatal("mutating the returned result must not alter the leader result")
	}
	if usage == nil {
		t.Fatal("expected non-nil helper usage")
	}
	if usage.Stage != "helper" {
		t.Errorf("expected stage helper, got %q", usage.Stage)
	}
	if usage.PricingVersion != "v1_estimated" {
		t.Errorf("expected pricing version v1_estimated, got %q", usage.PricingVersion)
	}
	if usage.InputTokens == nil || *usage.InputTokens != 0 || usage.OutputTokens == nil || *usage.OutputTokens != 0 || usage.CostUSD == nil || *usage.CostUSD != 0 {
		t.Errorf("expected zero token and cost usage, got %+v", usage)
	}
}
