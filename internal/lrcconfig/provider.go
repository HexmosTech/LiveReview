package lrcconfig

import "context"

// RepoConfigProvider is an optional capability for fetching a repository's
// .lrc/ directory at a given ref, for non-CLI (PR/MR) reviews.
//
// It lives in this package (rather than internal/providers) because
// internal/providers is imported by cmd/mrmodel/lib, which this package
// itself depends on (for lib.LocalCodeDiff) — defining it here avoids an
// import cycle.
//
// TODO(repo-rules): implement for github/gitlab/bitbucket/gitea by
// fetching the .lrc/ tree via each provider's contents/tree API at `ref`
// (the MR's head ref) and returning it as a Bundle. See
// BuildRulesBundle/LoadIgnorePatterns/FilterDiffs in this package, which
// this path should reuse unchanged (the same pipeline used for CLI
// diff-review).
type RepoConfigProvider interface {
	GetRepoConfigBundle(ctx context.Context, ref string) (bundle Bundle, ok bool, err error)
}
