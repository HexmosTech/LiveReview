package prompts

import (
	"context"
	"testing"
)

func TestRepoRulesRoundTrip(t *testing.T) {
	ctx := WithRepoRules(context.Background(), "## rules/security.md\n\nNo secrets in logs.\n")

	if got := RepoRulesFromContext(ctx); got != "## rules/security.md\n\nNo secrets in logs.\n" {
		t.Fatalf("RepoRulesFromContext = %q, want round-tripped rules", got)
	}

	want := "# Repository Rules\n\n## rules/security.md\n\nNo secrets in logs.\n\n"
	if got := BuildRepoRulesSection(ctx); got != want {
		t.Fatalf("BuildRepoRulesSection = %q, want %q", got, want)
	}
}

func TestRepoRulesFromContextEmpty(t *testing.T) {
	if got := RepoRulesFromContext(context.Background()); got != "" {
		t.Fatalf("RepoRulesFromContext on empty context = %q, want \"\"", got)
	}
}

func TestBuildRepoRulesSectionEmpty(t *testing.T) {
	if got := BuildRepoRulesSection(context.Background()); got != "" {
		t.Fatalf("BuildRepoRulesSection on empty context = %q, want \"\"", got)
	}

	ctx := WithRepoRules(context.Background(), "   \n\t")
	if got := BuildRepoRulesSection(ctx); got != "" {
		t.Fatalf("BuildRepoRulesSection with whitespace-only rules = %q, want \"\"", got)
	}
}
