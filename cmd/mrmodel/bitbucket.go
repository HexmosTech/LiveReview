package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/livereview/internal/database"
	"github.com/livereview/internal/providers/bitbucket"
	"github.com/livereview/pkg/shared"
)

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
	outDir := fs.String("out", "artifacts", "Output directory for generated artifacts")
	enableArtifacts := fs.Bool("enable-artifacts", false, "Enable writing artifacts to disk")
	urlFlag := stringFlag{value: defaultBitbucketPRURL}
	fs.Var(&urlFlag, "url", "Bitbucket pull request URL")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("failed to parse flags: %w", err)
	}

	mrModel := &MrModelImpl{
		EnableArtifactWriting: *enableArtifacts,
	}

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

	unifiedArtifact, err := mrModel.buildBitbucketArtifact(provider, prID, prURL, *outDir)
	if err != nil {
		return err
	}

	fmt.Printf("Target PR: %s\n", prURL)
	fmt.Printf("Summary: timeline_items=%d participants=%d diff_files=%d\n", len(unifiedArtifact.Timeline), len(unifiedArtifact.Participants), len(unifiedArtifact.Diffs))
	return nil
}
