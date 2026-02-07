package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/livereview/cmd/mrmodel/lib"
	"github.com/livereview/internal/database"
	"github.com/livereview/internal/providers/bitbucket"
	gl "github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/shared"
)

// =====================================================================================
// stringFlag helper for URL flags
// =====================================================================================

type stringFlag struct {
	value string
	set   bool
}

func (s *stringFlag) String() string { return s.value }

func (s *stringFlag) Set(v string) error {
	s.value = v
	s.set = true
	return nil
}

// =====================================================================================
// GitHub CLI
// =====================================================================================

// runGitHub collects GitHub PR context and writes timeline + comment tree exports.
func runGitHub(args []string) error {
	const defaultGitHubPRURL = "https://github.com/livereviewbot/glabmig/pull/2"
	fs := flag.NewFlagSet("github", flag.ContinueOnError)
	repo := fs.String("repo", "", "GitHub repository in owner/repo format")
	prNumber := fs.Int("pr", 0, "Pull request number")
	token := fs.String("token", "", "GitHub personal access token (optional if GITHUB_TOKEN or GITHUB_PAT set)")
	outDir := fs.String("out", "docs/raw/artifacts", "Output directory for generated artifacts")
	urlFlag := stringFlag{value: defaultGitHubPRURL}
	fs.Var(&urlFlag, "url", "GitHub pull request URL (overrides --repo/--pr)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	repoVal := strings.TrimSpace(*repo)
	prVal := *prNumber
	urlVal := strings.TrimSpace(urlFlag.value)

	var owner, name, prID, prURL string
	useURL := urlFlag.set || (repoVal == "" && prVal == 0)

	mrmodel := &lib.MrModelImpl{}

	if useURL {
		var err error
		owner, name, prID, err = mrmodel.ParseGitHubPRURL(urlVal)
		if err != nil {
			return err
		}
		prURL = urlVal
	} else if repoVal != "" && prVal > 0 {
		var err error
		owner, name, err = mrmodel.SplitRepo(repoVal)
		if err != nil {
			return err
		}
		prID = fmt.Sprintf("%d", prVal)
		prURL = fmt.Sprintf("https://github.com/%s/%s/pull/%s", owner, name, prID)
	} else {
		return errors.New("must provide both --repo and --pr or a valid --url")
	}

	pat := strings.TrimSpace(*token)
	if pat == "" {
		pat = strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	}
	if pat == "" {
		pat = strings.TrimSpace(os.Getenv("GITHUB_PAT"))
	}
	if pat == "" {
		var dbErr error
		pat, dbErr = lib.FindGitHubTokenFromDB()
		if dbErr != nil {
			return fmt.Errorf("GitHub token not provided via flags/env and lookup failed: %w", dbErr)
		}
	}

	unifiedArtifact, err := mrmodel.BuildGitHubArtifact(owner, name, prID, pat, *outDir)
	if err != nil {
		return err
	}

	fmt.Printf("Target PR: %s\n", prURL)
	fmt.Printf("Summary: timeline_items=%d participants=%d diff_files=%d\n", len(unifiedArtifact.Timeline), len(unifiedArtifact.Participants), len(unifiedArtifact.Diffs))
	return nil
}

// =====================================================================================
// GitLab CLI
// =====================================================================================

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
	outDir := fs.String("out", "docs/raw/artifacts", "Output directory for generated artifacts")
	enableArtifacts := fs.Bool("enable-artifacts", false, "Enable writing artifacts to disk")
	fs.Parse(args)

	mrModel := &lib.MrModelImpl{}
	mrModel.EnableArtifactWriting = *enableArtifacts

	cfg := gl.GitLabConfig{URL: hardcodedGitlabBaseURL, Token: hardcodedGitlabPAT}
	provider, err := gl.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to init gitlab provider: %w", err)
	}

	_, diffs, commits, discussions, standaloneNotes, err := mrModel.FetchGitLabData(provider, hardcodedGitlabMRURL)
	if err != nil {
		return err
	}

	unifiedArtifact, err := mrModel.BuildGitLabUnifiedArtifact(commits, discussions, standaloneNotes, diffs, *outDir)
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

// =====================================================================================
// Bitbucket CLI
// =====================================================================================

const (
	defaultBitbucketPRURL = "https://bitbucket.org/contorted/fb_backends/pull-requests/1"
)

func getBitbucketPRIDFromURL(repoURL string) (string, error) {
	_, _, prID, err := bitbucket.ParseBitbucketURL(repoURL)
	if err != nil {
		return "", err
	}
	if prID == "" {
		return "", fmt.Errorf("pull request ID not found in URL: %s", repoURL)
	}
	return prID, nil
}

func findBitbucketCredentialsFromDB() (*shared.VCSCredentials, error) {
	db, err := database.NewDB()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	var creds shared.VCSCredentials
	// Use pat_token column which stores the Personal Access Token
	// For Bitbucket, we'll need to extract email from metadata if needed
	err = db.QueryRow("SELECT pat_token FROM integration_tokens WHERE provider = 'bitbucket' LIMIT 1").Scan(&creds.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to query bitbucket credentials: %w", err)
	}

	// For now, set a default email - in production this should come from metadata or config
	creds.Email = "livereviewbot@gmail.com"
	creds.Provider = "bitbucket"

	return &creds, nil
}

func runBitbucket(args []string) error {
	fs := flag.NewFlagSet("bitbucket", flag.ContinueOnError)
	outDir := fs.String("out", "docs/raw/artifacts", "Output directory for generated artifacts")
	enableArtifacts := fs.Bool("enable-artifacts", false, "Enable writing artifacts to disk")
	urlFlag := stringFlag{value: defaultBitbucketPRURL}
	fs.Var(&urlFlag, "url", "Bitbucket pull request URL")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	mrModel := &lib.MrModelImpl{}
	mrModel.EnableArtifactWriting = *enableArtifacts

	prURL := strings.TrimSpace(urlFlag.value)
	if prURL == "" {
		return errors.New("must provide a valid --url")
	}

	prID, err := getBitbucketPRIDFromURL(prURL)
	if err != nil {
		return fmt.Errorf("failed to get PR ID from URL: %w", err)
	}

	var provider *bitbucket.BitbucketProvider
	var errProv error

	token := os.Getenv("BITBUCKET_TOKEN")
	email := os.Getenv("BITBUCKET_EMAIL")

	if token != "" && email != "" {
		log.Println("Using Bitbucket credentials from environment variables")
		provider, errProv = bitbucket.NewBitbucketProvider(token, email, prURL)
	} else {
		log.Println("Using Bitbucket credentials from integration_tokens (fallback)")
		creds, err := findBitbucketCredentialsFromDB()
		if err != nil {
			return fmt.Errorf("failed to find bitbucket credentials: %w", err)
		}
		provider, errProv = bitbucket.NewBitbucketProvider(creds.Token, creds.Email, prURL)
	}

	if errProv != nil {
		return fmt.Errorf("bitbucket provider creation failed: %w", errProv)
	}

	unifiedArtifact, err := mrModel.BuildBitbucketArtifact(provider, prID, prURL, *outDir)
	if err != nil {
		return err
	}

	fmt.Printf("Target PR: %s\n", prURL)
	fmt.Printf("Summary: timeline_items=%d participants=%d diff_files=%d\n", len(unifiedArtifact.Timeline), len(unifiedArtifact.Participants), len(unifiedArtifact.Diffs))
	return nil
}
