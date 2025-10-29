package main

import (
	"flag"
	"fmt"

	gl "github.com/livereview/internal/providers/gitlab"
)

// NOTE: This sample hardcodes connection details for Phase 0 connectivity.
// Requested by user: do not use env vars.
const (
	hardcodedGitlabBaseURL = "https://git.apps.hexmos.com"
	hardcodedGitlabMRURL   = "https://git.apps.hexmos.com/hexmos/liveapi/-/merge_requests/426"
	hardcodedGitlabPAT     = "REDACTED_GITLAB_PAT_4N286MQp1OjJiCA.01.0y0a9upua"
)

func runGitLab(args []string) error {
	fs := flag.NewFlagSet("gitlab", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "print prompt and result, do not post")
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	enableArtifacts := fs.Bool("enable-artifacts", false, "Enable writing artifacts to disk")
	fs.Parse(args)

	mrModel := &MrModelImpl{
		EnableArtifactWriting: *enableArtifacts,
	}

	cfg := gl.GitLabConfig{URL: hardcodedGitlabBaseURL, Token: hardcodedGitlabPAT}
	provider, err := gl.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to init gitlab provider: %w", err)
	}

	_, diffs, commits, discussions, standaloneNotes, err := mrModel.fetchGitLabData(provider, hardcodedGitlabMRURL)
	if err != nil {
		return err
	}

	unifiedArtifact, err := mrModel.buildUnifiedArtifact(commits, discussions, standaloneNotes, diffs, *outDir)
	if err != nil {
		return err
	}

	fmt.Printf("Summary: commits=%d discussions=%d notes=%d\n", len(commits), len(discussions), len(standaloneNotes))

	if *dryRun {
		fmt.Println("\n[dry-run] Skipping comment processing and posting.")
		return nil
	}

	_ = unifiedArtifact
	return nil
}
