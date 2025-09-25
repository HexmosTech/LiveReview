package providers

import (
	"context"

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
}

// DiffRefs contains references to the diff endpoints
type DiffRefs struct {
	BaseSHA  string
	HeadSHA  string
	StartSHA string
}
