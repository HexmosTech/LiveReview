package providers

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/livereview/internal/prsync"
	"github.com/livereview/pkg/models"
)

// Provider represents a code hosting provider (GitLab, GitHub, BitBucket)
type Provider interface {
	GetMergeRequestDetails(ctx context.Context, mrURL string) (*MergeRequestDetails, error)
	GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error)
	PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error
	PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error
	Name() string
	Configure(config map[string]interface{}) error
}

// MergeRequestDetails contains information about a merge request
type MergeRequestDetails struct {
	ID             string
	ProjectID      string
	Title          string
	Description    string
	SourceBranch   string
	TargetBranch   string
	Author         string
	AuthorName     string
	AuthorUsername string
	AuthorEmail    string
	AuthorAvatar   string
	CreatedAt      string
	URL            string
	State          string
	MergeStatus    string
	DiffRefs       DiffRefs
	WebURL         string
	ProviderType   string
	RepositoryURL  string
	// Commits is every individual commit SHA on this PR/MR, oldest first - populated by providers that support fetching it (GitHub, GitLab so far); empty for the rest, who fall back to DiffRefs' head/base pair.
	Commits []string
}

// DiffRefs contains references to the diff endpoints
type DiffRefs struct {
	BaseSHA  string
	HeadSHA  string
	StartSHA string
}

// ResolveMRID converts a fetched MergeRequestDetails into whatever ID format that same provider's GetMergeRequestChanges expects - GitHub/Bitbucket need owner/repo/number parsed from the URL (their changes API is stateless), everything else (GitLab, and so on) just reuses details.ID as-is.
func ResolveMRID(details *MergeRequestDetails) string {
	switch details.ProviderType {
	case "github":
		if u, err := url.Parse(details.URL); err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull" {
				return parts[1] + "/" + parts[2] + "/" + parts[4]
			}
		}
	case "bitbucket":
		if u, err := url.Parse(details.URL); err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull-requests" {
				return parts[1] + "/" + parts[2] + "/" + parts[4]
			}
		}
	}
	return details.ID
}

// PullRequestListState is the state filter passed to a bulk PR/MR listing call.
type PullRequestListState string

const (
	PRListStateAll    PullRequestListState = "all"
	PRListStateOpen   PullRequestListState = "open"
	PRListStateClosed PullRequestListState = "closed"
)

// RepositoryPage is one page of a bulk repository-listing call, with an opaque
// cursor for the next page (empty string means no more pages).
type RepositoryPage struct {
	Repositories []prsync.RepositorySummary
	NextCursor   string
}

// PullRequestPage is one page of a bulk PR/MR-listing call, with an opaque
// cursor for the next page (empty string means no more pages).
type PullRequestPage struct {
	PullRequests []prsync.PullRequestSummary
	NextCursor   string
}

// RateLimitedError signals that a provider API call was rejected or should be
// deferred due to rate limiting. Callers (e.g. the repo/PR sync worker) can
// check for this via errors.As and reschedule the work instead of failing it.
type RateLimitedError struct {
	Provider   string
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("%s rate limit exceeded, retry after %s", e.Provider, e.RetryAfter)
}
