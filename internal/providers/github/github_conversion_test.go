package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/livereview/pkg/models"
)

// This regression test ensures the JSON diff conversion logic turns a real GitHub patch
// capture into the exact `models.CodeDiff` structure we previously stored. If the converter
// ever diverges (e.g., hunk parsing or file flags change), the comparison against the
// golden diff fixture fails and alerts us that the provider->unified mapping regressed.

// captureFixtures mirrors the structure we persist from the live GitHub API.
type captureFixtures struct {
	Files []struct {
		Filename  string `json:"filename"`
		Status    string `json:"status"`
		Additions int    `json:"additions"`
		Deletions int    `json:"deletions"`
		Changes   int    `json:"changes"`
		Patch     string `json:"patch"`
		SHA       string `json:"sha"`
	} `json:"files"`
}

type expectedDiffs struct {
	Diffs []*models.CodeDiff `json:"diffs"`
}

type unifiedEventsFixture struct {
	Events []coreprocessor.UnifiedWebhookEventV2 `json:"events"`
}

func TestGitHubUnifiedTimelineReplayMatchesGolden(t *testing.T) {
	t.Parallel()

	events := readUnifiedEventsFixture(t, "github-webhook-unified-events-0001.json")
	expected := readExpectedTimeline(t, "github-webhook-expected-timeline-0001.json")

	var comments []coreprocessor.UnifiedCommentV2
	for idx, event := range events.Events {
		require.NotNilf(t, event.Comment, "event %d missing comment", idx)
		comments = append(comments, *event.Comment)
	}

	builder := coreprocessor.UnifiedContextBuilderV2{}
	timeline := builder.BuildTimelineFromData(nil, comments)

	require.NotNil(t, timeline)
	require.Len(t, timeline.Items, len(expected.Items))
	require.Nil(t, timeline.Items[0].Comment.Position, "issue comment should not record a file position")
	require.Equal(t, expected, *timeline)
}

func TestGitHubPatchConversionMatchesFixture(t *testing.T) {
	t.Parallel()

	raw := readRawFixture(t, "github-pr-files-0003.json")
	want := readExpectedDiffs(t, "github-pr-diffs-0004.json")

	provider := &GitHubProvider{}

	var got []*models.CodeDiff
	for _, f := range raw.Files {
		hunks := provider.parsePatchIntoHunks(f.Patch)
		diff := &models.CodeDiff{
			FilePath:   f.Filename,
			OldContent: "",
			NewContent: "",
			Hunks:      hunks,
			CommitID:   f.SHA,
			FileType:   provider.getFileType(f.Filename),
			IsDeleted:  f.Status == "removed",
			IsNew:      f.Status == "added",
			IsRenamed:  f.Status == "renamed",
		}

		got = append(got, diff)
	}

	require.Equal(t, len(want.Diffs), len(got), "unexpected diff count")
	require.Equal(t, want.Diffs, got)
}

func readRawFixture(t *testing.T, name string) captureFixtures {
	t.Helper()

	bytes := readFixture(t, name)

	var fixture captureFixtures
	require.NoError(t, json.Unmarshal(bytes, &fixture))
	return fixture
}

func readExpectedDiffs(t *testing.T, name string) expectedDiffs {
	t.Helper()

	bytes := readFixture(t, name)

	var diff expectedDiffs
	require.NoError(t, json.Unmarshal(bytes, &diff))
	return diff
}

func readUnifiedEventsFixture(t *testing.T, name string) unifiedEventsFixture {
	t.Helper()

	bytes := readFixture(t, name)

	var fixture unifiedEventsFixture
	require.NoError(t, json.Unmarshal(bytes, &fixture))
	return fixture
}

func readExpectedTimeline(t *testing.T, name string) coreprocessor.UnifiedTimelineV2 {
	t.Helper()

	bytes := readFixture(t, name)

	var timeline coreprocessor.UnifiedTimelineV2
	require.NoError(t, json.Unmarshal(bytes, &timeline))
	return timeline
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read fixture %s", path)
	return data
}
