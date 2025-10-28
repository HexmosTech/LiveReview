package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/livereview/internal/providers/bitbucket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitbucketMRModelGeneration(t *testing.T) {
	const testRepoURL = "https://bitbucket.org/goprasanth/test-go-repo"

	// Read test data from files
	commitsData, err := os.ReadFile(filepath.Join("testdata", "bitbucket", "commits.json"))
	require.NoError(t, err)
	var bbCommits []bitbucket.BitbucketCommit
	require.NoError(t, json.Unmarshal(commitsData, &bbCommits))

	commentsData, err := os.ReadFile(filepath.Join("testdata", "bitbucket", "comments.json"))
	require.NoError(t, err)
	var bbComments []bitbucket.BitbucketComment
	require.NoError(t, json.Unmarshal(commentsData, &bbComments))

	timelineItems := buildBitbucketTimeline(testRepoURL, bbCommits, bbComments)
	assert.NotEmpty(t, timelineItems)

	commentTree := buildBitbucketCommentTree(bbComments)
	assert.NotEmpty(t, commentTree.Roots)
}
