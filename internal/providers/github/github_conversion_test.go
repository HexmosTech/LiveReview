package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

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

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read fixture %s", path)
	return data
}
