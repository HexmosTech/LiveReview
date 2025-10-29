package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gl "github.com/livereview/internal/providers/gitlab"
	rm "github.com/livereview/internal/reviewmodel"
)

// NOTE: This sample hardcodes connection details for Phase 0 connectivity.
// Requested by user: do not use env vars.
const (
	gitlabEnableArtifactWriting = false
	hardcodedBaseURL            = "https://git.apps.hexmos.com"
	hardcodedMRURL              = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426"
	hardcodedPAT                = "REDACTED_GITLAB_PAT_4N286MQp1OjJiCA.01.0y0a9upua"
)

// Helpers
func atoi(s string) int {
	var n int
	fmt.Sscan(s, &n)
	return n
}

func runGitLab(args []string) error {
	fs := flag.NewFlagSet("gitlab", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print prompt and result, do not post")
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	fs.Parse(args)

	baseURL := hardcodedBaseURL
	token := hardcodedPAT
	mrURL := hardcodedMRURL

	cfg := gl.GitLabConfig{URL: baseURL, Token: token}
	provider, err := gl.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to init gitlab provider: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	details, err := provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		return fmt.Errorf("GetMergeRequestDetails failed: %w", err)
	}

	diffs, err := provider.GetMergeRequestChangesAsText(ctx, details.ID)
	if err != nil {
		return fmt.Errorf("failed to get MR changes: %w", err)
	}

	httpClient := provider.GetHTTPClient()
	commits, err := httpClient.GetMergeRequestCommits(details.ProjectID, atoi(details.ID))
	if err != nil {
		return fmt.Errorf("GetMergeRequestCommits failed: %w", err)
	}
	discussions, err := httpClient.GetMergeRequestDiscussions(details.ProjectID, atoi(details.ID))
	if err != nil {
		return fmt.Errorf("GetMergeRequestDiscussions failed: %w", err)
	}
	standaloneNotes, err := httpClient.GetMergeRequestNotes(details.ProjectID, atoi(details.ID))
	if err != nil {
		return fmt.Errorf("GetMergeRequestNotes failed: %w", err)
	}

	if gitlabEnableArtifactWriting {
		// 1. Create output directories
		if err := os.MkdirAll(*outDir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
		testDataDir := filepath.Join("cmd", "mrmodel", "testdata", "gitlab")
		if err := os.MkdirAll(testDataDir, 0o755); err != nil {
			return fmt.Errorf("create testdata dir: %w", err)
		}

		// 2. Write raw API responses to testdata directory
		rawDataPaths := make(map[string]string)

		rawCommitsPath := filepath.Join(testDataDir, "commits.json")
		if err := writeJSONPretty(rawCommitsPath, commits); err != nil {
			return fmt.Errorf("write raw commits: %w", err)
		}
		rawDataPaths["commits"] = rawCommitsPath

		rawDiscussionsPath := filepath.Join(testDataDir, "discussions.json")
		if err := writeJSONPretty(rawDiscussionsPath, discussions); err != nil {
			return fmt.Errorf("write raw discussions: %w", err)
		}
		rawDataPaths["discussions"] = rawDiscussionsPath

		rawNotesPath := filepath.Join(testDataDir, "notes.json")
		if err := writeJSONPretty(rawNotesPath, standaloneNotes); err != nil {
			return fmt.Errorf("write raw notes: %w", err)
		}
		rawDataPaths["notes"] = rawNotesPath

		rawDiffPath := filepath.Join(testDataDir, "diff.txt")
		if err := os.WriteFile(rawDiffPath, []byte(diffs), 0644); err != nil {
			return fmt.Errorf("write raw diff: %w", err)
		}
		rawDataPaths["diff"] = rawDiffPath
	}

	// 3. Process data and build unified artifact
	timelineItems := rm.BuildTimeline(commits, discussions, standaloneNotes)
	commentTree := rm.BuildCommentTree(discussions, standaloneNotes)

	diffParser := NewLocalParser()
	parsedDiffs, err := diffParser.Parse(diffs)
	if err != nil {
		return fmt.Errorf("parse diff: %w", err)
	}

	diffsPtrs := make([]*LocalCodeDiff, len(parsedDiffs))
	for i := range parsedDiffs {
		diffsPtrs[i] = &parsedDiffs[i]
	}

	unifiedArtifact := UnifiedArtifact{
		Provider:     "gitlab",
		Timeline:     timelineItems,
		CommentTree:  commentTree,
		Diffs:        diffsPtrs,
		Participants: extractParticipants(timelineItems),
		// RawDataPaths: rawDataPaths,
	}

	if gitlabEnableArtifactWriting {
		// 4. Write unified artifact to a single file
		unifiedPath := filepath.Join(*outDir, "gl_unified.json")
		if err := writeJSONPretty(unifiedPath, unifiedArtifact); err != nil {
			return fmt.Errorf("write unified artifact: %w", err)
		}
		fmt.Printf("GitLab unified artifact written to %s\n", unifiedPath)
	}
	fmt.Printf("Summary: commits=%d discussions=%d notes=%d\n", len(commits), len(discussions), len(standaloneNotes))

	if *dryRun {
		fmt.Println("\n[dry-run] Skipping comment processing and posting.")
		return nil
	}

	// The rest of the logic for finding and replying to comments can remain
	// as it was, since it's for the interactive part of the tool.
	// ... (rest of the function)
	return nil
}
