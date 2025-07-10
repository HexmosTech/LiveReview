package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

// GitLabProvider implements the Provider interface for GitLab
type GitLabProvider struct {
	client *gitlab.Client
	config GitLabConfig
}

// GitLabConfig contains configuration for the GitLab provider
type GitLabConfig struct {
	URL   string `koanf:"url"`
	Token string `koanf:"token"`
}

// New creates a new GitLabProvider
func New(config GitLabConfig) (*GitLabProvider, error) {
	// Create a new client with nil HTTP client (defaults to http.DefaultClient)
	// and the provided token
	client := gitlab.NewClient(nil, config.Token)

	// We don't need to handle error for setting base URL in this implementation

	return &GitLabProvider{
		client: client,
		config: config,
	}, nil
}

// GetMergeRequestDetails retrieves the details of a merge request
func (p *GitLabProvider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	// Extract project ID and MR IID from URL
	projectID, mrIID, err := p.extractMRInfo(mrURL)
	if err != nil {
		return nil, err
	}

	// TODO: Implement using GitLab API client
	// This is a stub implementation
	return &providers.MergeRequestDetails{
		ID:           strconv.Itoa(mrIID),
		ProjectID:    projectID,
		Title:        "Sample MR",
		Description:  "Sample description",
		SourceBranch: "feature-branch",
		TargetBranch: "main",
		State:        "opened",
		URL:          mrURL,
		ProviderType: "gitlab",
	}, nil
}

// GetMergeRequestChanges retrieves the code changes in a merge request
func (p *GitLabProvider) GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error) {
	// TODO: Implement using GitLab API client
	// This is a stub implementation
	return []*models.CodeDiff{
		{
			FilePath:   "example.go",
			OldContent: "package main\n\nfunc main() {\n\tprint(\"Hello World\")\n}\n",
			NewContent: "package main\n\nfunc main() {\n\tfmt.Println(\"Hello World\")\n}\n",
			Hunks: []models.DiffHunk{
				{
					OldStartLine: 3,
					OldLineCount: 1,
					NewStartLine: 3,
					NewLineCount: 1,
					Content:      "-\tprint(\"Hello World\")\n+\tfmt.Println(\"Hello World\")\n",
				},
			},
			CommitID: "abc123",
			FileType: "go",
		},
	}, nil
}

// PostComment posts a comment on a merge request
func (p *GitLabProvider) PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error {
	// TODO: Implement using GitLab API client
	return nil
}

// PostComments posts multiple comments on a merge request
func (p *GitLabProvider) PostComments(ctx context.Context, mrID string, comments []*models.ReviewComment) error {
	for _, comment := range comments {
		if err := p.PostComment(ctx, mrID, comment); err != nil {
			return err
		}
	}
	return nil
}

// Name returns the name of the provider
func (p *GitLabProvider) Name() string {
	return "gitlab"
}

// Configure configures the provider with the given configuration
func (p *GitLabProvider) Configure(config map[string]interface{}) error {
	// TODO: Implement configuration
	return nil
}

// extractMRInfo extracts project ID and MR IID from a GitLab MR URL
func (p *GitLabProvider) extractMRInfo(mrURL string) (string, int, error) {
	// Parse URL to extract project and MR IID
	// Example URL: https://gitlab.example.com/group/project/-/merge_requests/123
	parsedURL, err := url.Parse(mrURL)
	if err != nil {
		return "", 0, fmt.Errorf("invalid URL: %w", err)
	}

	re := regexp.MustCompile(`/(.+)/-/merge_requests/(\d+)$`)
	matches := re.FindStringSubmatch(parsedURL.Path)
	if len(matches) != 3 {
		return "", 0, fmt.Errorf("could not extract project and MR ID from URL: %s", mrURL)
	}

	projectPath := matches[1]
	mrIID, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, fmt.Errorf("invalid MR ID: %w", err)
	}

	// TODO: Convert project path to project ID using API

	return projectPath, mrIID, nil
}
