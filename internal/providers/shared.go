package providers

import (
	"context"
	"errors"
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
	// Commits is every individual commit on this PR/MR, oldest first - populated by providers that support fetching it (GitHub, GitLab so far); empty for the rest, who fall back to DiffRefs' head/base pair.
	Commits []CommitSummary
}

// CommitSummary is a single PR/MR commit, as returned by a provider's commits endpoint.
type CommitSummary struct {
	SHA     string
	Message string // best-effort; may be empty if the provider's response omitted it
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

// AuthError signals that a provider API call failed due to authentication
// or authorization issues (e.g. 401 Unauthorized, 403 Forbidden with bad/expired credentials).
// Callers can check this via IsAuthError to immediately cancel jobs and avoid retry loops.
type AuthError struct {
	Provider   string
	StatusCode int
	Message    string
}

func (e *AuthError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s authentication failed (status %d): %s", e.Provider, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s authentication failed: %s", e.Provider, e.Message)
}

// IsAuthError returns true if err or any error in its chain is an *AuthError,
// or contains authentication error indicators for the specified provider (or across all
// known providers if no provider is passed).
func IsAuthError(err error, providerOpt ...string) bool {
	if err == nil {
		return false
	}
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return true
	}
	msg := strings.ToLower(err.Error())

	// Exclude rate limits first (e.g. GitHub/GitLab 403 secondary rate limits)
	if strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") {
		return false
	}

	provider := ""
	if len(providerOpt) > 0 && providerOpt[0] != "" {
		provider = strings.ToLower(providerOpt[0])
	}

	// Standard HTTP 401 Unauthorized across any provider:
	// - Go HTTP status text: "401 unauthorized"
	// - Generic status format: "status 401", "status: 401", "status code 401", "http 401"
	// - Bracketed status format (Azure DevOps, Gitea): "(401)"
	has401 := strings.Contains(msg, "401") && (
		strings.Contains(msg, "unauthorized") ||
			strings.Contains(msg, "status") ||
			strings.Contains(msg, "http") ||
			strings.Contains(msg, "(401)"))

	// Standard HTTP 403 Forbidden across any provider (rate limits already excluded):
	// - "403 forbidden", "status 403", "http 403", "(403)"
	has403 := strings.Contains(msg, "403") && (
		strings.Contains(msg, "forbidden") ||
			strings.Contains(msg, "status") ||
			strings.Contains(msg, "http") ||
			strings.Contains(msg, "(403)"))

	if has401 || has403 {
		return true
	}

	// Common credential errors across all providers:
	if strings.Contains(msg, "authentication failed") || strings.Contains(msg, "oauth token is invalid") {
		return true
	}

	// Provider-specific checks:
	switch {
	case strings.HasPrefix(provider, "github"):
		return strings.Contains(msg, "bad credentials") ||
			strings.Contains(msg, "failed to resolve github app token") ||
			strings.Contains(msg, "invalid token")
	case strings.HasPrefix(provider, "gitlab"):
		return strings.Contains(msg, "invalid_token") ||
			strings.Contains(msg, "invalid token") ||
			strings.Contains(msg, "token expired")
	case strings.HasPrefix(provider, "azure"):
		return strings.Contains(msg, "invalid personal access token") ||
			strings.Contains(msg, "tf400813") ||
			strings.Contains(msg, "invalid token")
	case strings.HasPrefix(provider, "bitbucket"):
		return strings.Contains(msg, "token is invalid") ||
			strings.Contains(msg, "access token expired") ||
			strings.Contains(msg, "token expired")
	case strings.HasPrefix(provider, "gitea"):
		return strings.Contains(msg, "invalid token") ||
			strings.Contains(msg, "token expired")
	default:
		// When no specific provider is passed, check across all known provider signatures:
		return strings.Contains(msg, "bad credentials") ||
			strings.Contains(msg, "failed to resolve github app token") ||
			strings.Contains(msg, "invalid_token") ||
			strings.Contains(msg, "invalid token") ||
			strings.Contains(msg, "invalid personal access token") ||
			strings.Contains(msg, "token expired") ||
			strings.Contains(msg, "token has expired") ||
			strings.Contains(msg, "access token expired") ||
			strings.Contains(msg, "token is invalid") ||
			strings.Contains(msg, "tf400813")
	}
}
