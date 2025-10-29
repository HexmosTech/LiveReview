package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/livereview/internal/providers"
	gl "github.com/livereview/internal/providers/gitlab"
	rm "github.com/livereview/internal/reviewmodel"
)

func (m *MrModelImpl) fetchGitLabData(provider *gl.GitLabProvider, mrURL string) (
	*providers.MergeRequestDetails,
	string,
	[]gl.GitLabCommit,
	[]gl.GitLabDiscussion,
	[]gl.GitLabNote,
	error,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("GetMergeRequestDetails failed: %w", err)
	}

	diffs, err := provider.GetMergeRequestChangesAsText(ctx, details.ID)
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("failed to get MR changes: %w", err)
	}

	httpClient := provider.GetHTTPClient()
	commits, err := httpClient.GetMergeRequestCommits(details.ProjectID, atoi(details.ID))
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("GetMergeRequestCommits failed: %w", err)
	}
	discussions, err := httpClient.GetMergeRequestDiscussions(details.ProjectID, atoi(details.ID))
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("GetMergeRequestDiscussions failed: %w", err)
	}
	standaloneNotes, err := httpClient.GetMergeRequestNotes(details.ProjectID, atoi(details.ID))
	if err != nil {
		return nil, "", nil, nil, nil, fmt.Errorf("GetMergeRequestNotes failed: %w", err)
	}

	return details, diffs, commits, discussions, standaloneNotes, nil
}

func (m *MrModelImpl) writeArtifacts(outDir string, commits []gl.GitLabCommit, discussions []gl.GitLabDiscussion, standaloneNotes []gl.GitLabNote, diffs string, unifiedArtifact *UnifiedArtifact) error {
	if !m.EnableArtifactWriting {
		return nil
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	testDataDir := filepath.Join("cmd", "mrmodel", "testdata", "gitlab")
	if err := os.MkdirAll(testDataDir, 0o755); err != nil {
		return fmt.Errorf("create testdata dir: %w", err)
	}

	rawCommitsPath := filepath.Join(testDataDir, "commits.json")
	if err := writeJSONPretty(rawCommitsPath, commits); err != nil {
		return fmt.Errorf("write raw commits: %w", err)
	}

	rawDiscussionsPath := filepath.Join(testDataDir, "discussions.json")
	if err := writeJSONPretty(rawDiscussionsPath, discussions); err != nil {
		return fmt.Errorf("write raw discussions: %w", err)
	}

	rawNotesPath := filepath.Join(testDataDir, "notes.json")
	if err := writeJSONPretty(rawNotesPath, standaloneNotes); err != nil {
		return fmt.Errorf("write raw notes: %w", err)
	}

	rawDiffPath := filepath.Join(testDataDir, "diff.txt")
	if err := os.WriteFile(rawDiffPath, []byte(diffs), 0644); err != nil {
		return fmt.Errorf("write raw diff: %w", err)
	}

	unifiedPath := filepath.Join(outDir, "gl_unified.json")
	if err := writeJSONPretty(unifiedPath, unifiedArtifact); err != nil {
		return fmt.Errorf("write unified artifact: %w", err)
	}
	fmt.Printf("GitLab unified artifact written to %s\n", unifiedPath)

	return nil
}

func (m *MrModelImpl) buildUnifiedArtifact(commits []gl.GitLabCommit, discussions []gl.GitLabDiscussion, standaloneNotes []gl.GitLabNote, diffs string, outDir string) (*UnifiedArtifact, error) {
	timelineItems := rm.BuildTimeline(commits, discussions, standaloneNotes)
	commentTree := rm.BuildCommentTree(discussions, standaloneNotes)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(diffs)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := &UnifiedArtifact{
		Provider:     "gitlab",
		Timeline:     timelineItems,
		CommentTree:  commentTree,
		Diffs:        diffsPtrs,
		Participants: extractParticipants(timelineItems),
	}

	if err := m.writeArtifacts(outDir, commits, discussions, standaloneNotes, diffs, unifiedArtifact); err != nil {
		return nil, err
	}

	return unifiedArtifact, nil
}
