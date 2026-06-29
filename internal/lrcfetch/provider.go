// Package lrcfetch defines the interface for fetching a repository's .lrc/
// directory from a remote git host (GitHub, GitLab, Bitbucket, Gitea).
//
// This package is intentionally dependency-free so that provider_input
// packages can implement Provider without creating an import cycle via
// lrcconfig (which depends on cmd/mrmodel/lib, which in turn imports
// provider_input packages).
//
// Callers that need lrcconfig.Bundle wrap the returned map:
//
//	files, ok, err := p.GetRepoConfigFiles(ctx, repoFullName, ref)
//	bundle := lrcconfig.Bundle{Files: files}
package lrcfetch

import "context"

// Provider is an optional capability for fetching a repository's .lrc/
// directory at a given ref, for non-CLI (PR/MR) triggered reviews.
//
// repoFullName is "owner/repo" for GitHub/Gitea, "namespace/project" for
// GitLab, and "workspace/repo" for Bitbucket.
// ref is a branch name or commit SHA (e.g. the PR's target branch).
//
// Returns (files, true, nil) when .lrc/ is present and successfully fetched.
// Returns (nil, false, nil) when .lrc/ does not exist on the repo (404).
// Returns (nil, false, err) for unexpected API errors.
type Provider interface {
	GetRepoConfigFiles(ctx context.Context, repoFullName, ref string) (files map[string][]byte, ok bool, err error)
}
