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

	server := &Server{}
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

	updated := server.appendLearningsToPrompt(prompt, learningsItems)

	require.Contains(t, updated, "=== Org learnings ===")
	require.Contains(t, updated, "LR-7")
	require.Contains(t, updated, "Prefer early returns")
	require.Contains(t, updated, "LR-8")
	require.Contains(t, updated, "Guard nil receivers")
	assert.True(t, strings.HasPrefix(updated, prompt))
}

func TestAppendLearningsToPromptNoItemsPreservesPrompt(t *testing.T) {
	t.Helper()

	server := &Server{}
	original := "No extra data"

	updated := server.appendLearningsToPrompt(original, nil)

	assert.Equal(t, original, updated)
}

func TestTruncateLearningBody(t *testing.T) {
	t.Helper()

	longBody := strings.Repeat("detail ", 120)

	truncated := truncateLearningBody(longBody, 80)

	require.True(t, len(truncated) <= 83)
	assert.True(t, strings.HasSuffix(truncated, "..."))

	empty := truncateLearningBody("   \n\t  ", 80)
	assert.Equal(t, "", empty)
}
