package api

import (
	"strings"
	"testing"

	"github.com/livereview/internal/learnings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendLearningsToPromptAddsSection(t *testing.T) {
	t.Helper()

	processor := &UnifiedProcessorV2Impl{}
	prompt := "Core prompt context"
	learningsItems := []*learnings.Learning{
		{
			ShortID: "LR-7",
			Title:   "Prefer early returns",
			Body:    "Keep conditionals shallow by returning early when possible.",
			Tags:    []string{"go", "style"},
		},
		{
			ShortID: "LR-8",
			Title:   "Guard nil receivers",
			Body:    "Always guard against nil receiver usage before accessing methods to avoid panics.",
		},
	}

	updated := processor.appendLearningsToPrompt(prompt, learningsItems)

	require.Contains(t, updated, "=== Org learnings ===")
	require.Contains(t, updated, "LR-7")
	require.Contains(t, updated, "Prefer early returns")
	require.Contains(t, updated, "LR-8")
	require.Contains(t, updated, "Guard nil receivers")
	assert.True(t, strings.HasPrefix(updated, prompt))
}

func TestAppendLearningsToPromptNoItemsPreservesPrompt(t *testing.T) {
	t.Helper()

	processor := &UnifiedProcessorV2Impl{}
	original := "No extra data"

	updated := processor.appendLearningsToPrompt(original, nil)

	assert.Equal(t, original, updated)
}

func TestTruncateLearningBody(t *testing.T) {
	t.Helper()

	longBody := strings.Repeat("detail ", 120)

	truncated := truncateLearningBodyV2(longBody, 80)

	require.True(t, len(truncated) <= 83)
	assert.True(t, strings.HasSuffix(truncated, "..."))

	empty := truncateLearningBodyV2("   \n\t  ", 80)
	assert.Equal(t, "", empty)
}
