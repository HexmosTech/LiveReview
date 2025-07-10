package providers

import (
	"context"

	"github.com/livereview/pkg/models"
)

// Provider represents a code hosting provider (GitLab, GitHub, BitBucket)
type Provider interface {
	// GetMergeRequestDetails retrieves the details of a merge request
	GetMergeRequestDetails(ctx context.Context, mrURL string) (*MergeRequestDetails, error)

	// GetMergeRequestChanges retrieves the code changes in a merge request
	GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error)

	// PostComment posts a comment on a merge request
	PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error

	// PostComments posts multiple comments on a merge request
	PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error

	// Name returns the name of the provider
	Name() string

	// Configure configures the provider with the given configuration
	Configure(config map[string]interface{}) error
}

// MergeRequestDetails contains information about a merge request
type MergeRequestDetails struct {
	ID            string
	ProjectID     string
	Title         string
	Description   string
	SourceBranch  string
	TargetBranch  string
	Author        string
	CreatedAt     string
	URL           string
	State         string
	MergeStatus   string
	DiffRefs      DiffRefs
	WebURL        string
	ProviderType  string
	RepositoryURL string
}

// DiffRefs contains references to the diff endpoints
type DiffRefs struct {
	BaseSHA  string
	HeadSHA  string
	StartSHA string
}
