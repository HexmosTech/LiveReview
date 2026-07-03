package prompts

import (
	"context"
	"testing"
)

func TestConciseModeDisabledByDefault(t *testing.T) {
	if got := BuildConciseModeSection(context.Background()); got != "" {
		t.Fatalf("BuildConciseModeSection on empty context = %q, want \"\"", got)
	}
}

func TestConciseModeEnabled(t *testing.T) {
	ctx := WithConciseMode(context.Background(), true)
	if !ConciseModeFromContext(ctx) {
		t.Fatal("ConciseModeFromContext = false, want true")
	}
	if got := BuildConciseModeSection(ctx); got == "" {
		t.Fatal("BuildConciseModeSection with concise mode enabled = \"\", want non-empty instructions")
	}
}

func TestConciseModeExplicitlyDisabled(t *testing.T) {
	ctx := WithConciseMode(context.Background(), false)
	if got := BuildConciseModeSection(ctx); got != "" {
		t.Fatalf("BuildConciseModeSection with concise mode disabled = %q, want \"\"", got)
	}
}
