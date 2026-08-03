// Package prsync holds provider-agnostic types and state-normalization logic shared
// between the bulk repository/PR listing code (internal/providers/{github,gitlab})
// and the webhook-driven PR/MR state sync code (internal/api, internal/jobqueue),
// so those two packages don't need to depend on each other.
package prsync

import "time"

// RepositorySummary is the normalized shape returned by a provider's bulk
// repository-listing call, sufficient to populate a repositories row.
type RepositorySummary struct {
	ProviderRepoID string
	FullName       string
	Name           string
	WebURL         string
	CloneURL       string
	SSHURL         string
	DefaultBranch  string
	IsPrivate      bool
	Description    string
}

// PullRequestSummary is the normalized shape returned by a provider's bulk
// PR/MR-listing call. State is already normalized via NormalizeGitHubState /
// NormalizeGitLabState.
type PullRequestSummary struct {
	ProviderPRID      string
	Number            int
	Title             string
	Description       string
	State             string // "open" | "closed" | "merged"
	AuthorID          string
	AuthorUsername    string
	AuthorName        string
	AuthorAvatarURL   string
	SourceBranch      string
	TargetBranch      string
	WebURL            string
	ProviderCreatedAt time.Time
	ProviderUpdatedAt time.Time
	Metadata          map[string]interface{}
}

// PullRequestStateEvent is the normalized shape produced when a webhook delivers
// a PR/MR lifecycle event (GitHub "pull_request", GitLab "merge_request").
// It carries enough repository identity for the sync worker to resolve (or
// create) the owning repositories row before upserting the pull_requests row.
type PullRequestStateEvent struct {
	RepositoryProviderID string
	RepositoryFullName   string
	RepositoryWebURL     string

	Number            int
	ProviderPRID      string
	Title             string
	Description       string
	State             string // "open" | "closed" | "merged"
	AuthorID          string
	AuthorUsername    string
	AuthorName        string
	AuthorAvatarURL   string
	SourceBranch      string
	TargetBranch      string
	WebURL            string
	ProviderCreatedAt time.Time
	ProviderUpdatedAt time.Time
	Metadata          map[string]interface{}
}
