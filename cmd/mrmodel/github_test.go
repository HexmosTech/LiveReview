package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	githubapi "github.com/livereview/internal/provider_input/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubDataProcessing(t *testing.T) {
	// 1. Define paths to test data
	testDataDir := filepath.Join("testdata", "github")
	commitsPath := filepath.Join(testDataDir, "commits.json")
	issueCommentsPath := filepath.Join(testDataDir, "issue_comments.json")
	reviewCommentsPath := filepath.Join(testDataDir, "review_comments.json")
	reviewsPath := filepath.Join(testDataDir, "reviews.json")
	diffPath := filepath.Join(testDataDir, "diff.txt")

	// 2. Load raw data from files
	var commits []githubapi.GitHubV2CommitInfo
	loadJSON(t, commitsPath, &commits)

	var issueComments []githubapi.GitHubV2CommentInfo
	loadJSON(t, issueCommentsPath, &issueComments)

	var reviewComments []githubapi.GitHubV2ReviewComment
	loadJSON(t, reviewCommentsPath, &reviewComments)

	var reviews []githubapi.GitHubV2ReviewInfo
	loadJSON(t, reviewsPath, &reviews)

	diffText, err := os.ReadFile(diffPath)
	require.NoError(t, err, "Failed to read diff.txt")

	// 3. Run the same processing logic as in runGitHub
	owner := "test-owner"
	repo := "test-repo"

	mrmodel := &MrModelImpl{}
	timelineItems := mrmodel.buildGitHubTimeline(owner, repo, commits, issueComments, reviewComments, reviews)
	sort.Slice(timelineItems, func(i, j int) bool {
		return timelineItems[i].CreatedAt.Before(timelineItems[j].CreatedAt)
	})

	commentTree := mrmodel.buildGitHubCommentTree(issueComments, reviewComments, reviews)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(string(diffText))
	require.NoError(t, err, "Failed to parse diff")

	// Convert []LocalCodeDiff to []*LocalCodeDiff
	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	// 4. Construct the UnifiedArtifact
	unifiedArtifact := UnifiedArtifact{
		Provider:    "github",
		Timeline:    timelineItems,
		CommentTree: commentTree,
		Diffs:       diffsPtrs,
	}

	// 5. Assertions to validate the structure
	assert.Equal(t, "github", unifiedArtifact.Provider)
	assert.NotEmpty(t, unifiedArtifact.Timeline, "Timeline should not be empty")
	assert.NotEmpty(t, unifiedArtifact.CommentTree.Roots, "CommentTree should have roots")
	assert.NotEmpty(t, unifiedArtifact.Diffs, "Diffs should not be empty")

	// Example of a more specific assertion:
	// Check if the number of timeline items matches the sum of raw items that create them.
	// Note: This is an approximation, as not all raw items (e.g., empty review bodies) create timeline entries.
	expectedTimelineItems := len(commits) + len(issueComments) + countNonEmptyReviewBodies(reviews) + len(reviewComments)
	assert.GreaterOrEqual(t, expectedTimelineItems, len(unifiedArtifact.Timeline), "Timeline items count should be consistent with raw data")

	// Check diff parsing
	assert.True(t, len(unifiedArtifact.Diffs) > 0, "Should have at least one file diff")
	firstDiff := unifiedArtifact.Diffs[0]
	assert.NotEmpty(t, firstDiff.NewPath, "First diff should have a NewPath")
	assert.NotEmpty(t, firstDiff.Hunks, "First diff should have hunks")
}

// loadJSON is a helper to load JSON from a file into a given interface
func loadJSON(t *testing.T, path string, v interface{}) {
	data, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read file: %s", path)
	err = json.Unmarshal(data, v)
	require.NoError(t, err, "Failed to unmarshal JSON from file: %s", path)
}

// countNonEmptyReviewBodies counts reviews that would generate a timeline comment.
func countNonEmptyReviewBodies(reviews []githubapi.GitHubV2ReviewInfo) int {
	count := 0
	for _, r := range reviews {
		if r.Body != "" {
			count++
		}
	}
	return count
}
