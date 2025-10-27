package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	gl "github.com/livereview/internal/providers/gitlab"
	rm "github.com/livereview/internal/reviewmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitLabMRModelGeneration(t *testing.T) {
	// This test uses pre-saved raw API data to build a unified artifact.
	// It ensures that the data processing and structuring logic is correct
	// without making live API calls.

	testDataDir := filepath.Join("testdata", "gitlab")

	// 1. Read raw data from test files
	var commits []gl.GitLabCommit
	rawCommits, err := os.ReadFile(filepath.Join(testDataDir, "commits.json"))
	require.NoError(t, err)
	err = json.Unmarshal(rawCommits, &commits)
	require.NoError(t, err)

	var discussions []gl.GitLabDiscussion
	rawDiscussions, err := os.ReadFile(filepath.Join(testDataDir, "discussions.json"))
	require.NoError(t, err)
	err = json.Unmarshal(rawDiscussions, &discussions)
	require.NoError(t, err)

	var notes []gl.GitLabNote
	rawNotes, err := os.ReadFile(filepath.Join(testDataDir, "notes.json"))
	require.NoError(t, err)
	err = json.Unmarshal(rawNotes, &notes)
	require.NoError(t, err)

	rawDiff, err := os.ReadFile(filepath.Join(testDataDir, "diff.txt"))
	require.NoError(t, err)

	// 2. Process data and build unified artifact
	timeline := rm.BuildTimeline(commits, discussions, notes)
	commentTree := rm.BuildCommentTree(discussions, notes)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(string(rawDiff))
	require.NoError(t, err)

	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := UnifiedArtifact{
		Provider:    "gitlab",
		Timeline:    timeline,
		CommentTree: commentTree,
		Diffs:       diffsPtrs,
	}

	// 3. Assertions to validate the unified artifact
	assert.Equal(t, "gitlab", unifiedArtifact.Provider)
	assert.NotEmpty(t, unifiedArtifact.Timeline)
	assert.NotEmpty(t, unifiedArtifact.CommentTree.Roots)
	assert.NotEmpty(t, unifiedArtifact.Diffs)

	// Example assertions on the content
	assert.Equal(t, 20, len(commits))
	assert.Equal(t, 20, len(discussions))
	assert.Equal(t, 20, len(notes))

	// Check timeline items
	// The timeline should contain all commits, all notes from discussions, and all standalone notes.
	totalNotesInDiscussions := 0
	for _, d := range discussions {
		totalNotesInDiscussions += len(d.Notes)
	}
	assert.Equal(t, len(commits)+totalNotesInDiscussions+len(notes), len(unifiedArtifact.Timeline))

	// Check a specific timeline item (e.g., a commit)
	var foundCommit bool
	for _, item := range unifiedArtifact.Timeline {
		if item.Kind == "commit" && item.Commit != nil && item.Commit.SHA == "ac3323855ece2064f9d312a2b487ce80dfe94273" {
			assert.Contains(t, item.Commit.Title, "reverted-changes-adding-system-prompt")
			foundCommit = true
		}
	}
	assert.True(t, foundCommit, "Expected commit not found in timeline")

	// Check comment tree
	// The comment tree roots should be the discussions and standalone notes.
	assert.Equal(t, len(discussions)+len(notes), len(unifiedArtifact.CommentTree.Roots))

	// Check diffs
	assert.True(t, len(unifiedArtifact.Diffs) > 0)
	firstDiff := unifiedArtifact.Diffs[0]
	assert.Equal(t, "liveapi-backend/prompt/prompt.go", firstDiff.NewPath)
}
