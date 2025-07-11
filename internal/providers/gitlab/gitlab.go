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
	client      *gitlab.Client
	httpClient  *GitLabHTTPClient
	config      GitLabConfig
	projectID   string // Store the current project ID
	currentMRID int    // Store the current MR IID
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

	// Set the base URL for the GitLab API
	if config.URL != "" {
		err := client.SetBaseURL(fmt.Sprintf("%s/api/v4", config.URL))
		if err != nil {
			return nil, fmt.Errorf("failed to set GitLab API base URL: %w", err)
		}
	}

	// Initialize our custom HTTP client that bypasses the endpoint issues
	httpClient := NewHTTPClient(config.URL, config.Token)

	fmt.Printf("Initialized GitLab client with URL: %s\n", config.URL)

	return &GitLabProvider{
		client:     client,
		httpClient: httpClient,
		config:     config,
	}, nil
}

// GetMergeRequestDetails retrieves the details of a merge request
func (p *GitLabProvider) GetMergeRequestDetails(ctx context.Context, mrURL string) (*providers.MergeRequestDetails, error) {
	// Extract project ID and MR IID from URL
	projectID, mrIID, err := p.extractMRInfo(mrURL)
	if err != nil {
		return nil, err
	}

	// Store the project ID and MR IID for later use
	p.projectID = projectID
	p.currentMRID = mrIID

	// Make an API call to get the merge request details using our custom HTTP client
	fmt.Printf("Fetching GitLab MR details for project=%s, mrIID=%d using custom HTTP client\n", projectID, mrIID)

	mr, err := p.httpClient.GetMergeRequest(projectID, mrIID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch merge request: %w", err)
	}

	// Convert GitLab MR to our internal model
	return ConvertToMergeRequestDetails(mr, projectID), nil
}

// GetMergeRequestChanges retrieves the code changes in a merge request
func (p *GitLabProvider) GetMergeRequestChanges(ctx context.Context, mrID string) ([]*models.CodeDiff, error) {
	// Convert mrID to integer
	mrIID, err := strconv.Atoi(mrID)
	if err != nil {
		return nil, fmt.Errorf("invalid MR ID: %w", err)
	}

	// Use the stored project ID if available, otherwise try to extract from URL
	projectID := p.projectID
	if projectID == "" {
		// Fallback to trying to extract from a URL (this might not work)
		var extractErr error
		projectID, _, extractErr = p.extractMRInfo(fmt.Sprintf("%s/-/merge_requests/%d", p.config.URL, mrIID))
		if extractErr != nil {
			return nil, fmt.Errorf("failed to get project ID: %w", extractErr)
		}
	}

	// Get merge request changes using our custom HTTP client
	fmt.Printf("Fetching GitLab MR changes for project=%s, mrIID=%d using custom HTTP client\n", projectID, mrIID)

	changes, err := p.httpClient.GetMergeRequestChanges(projectID, mrIID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch merge request changes: %w", err)
	}

	// Convert the changes to our internal model
	return ConvertToCodeDiffs(changes), nil
}

// PostComment posts a comment on a merge request
func (p *GitLabProvider) PostComment(ctx context.Context, mrID string, comment *models.ReviewComment) error {
	// Convert mrID to integer
	mrIID, err := strconv.Atoi(mrID)
	if err != nil {
		return fmt.Errorf("invalid MR ID: %w", err)
	}

	// Use the stored project ID if available, otherwise try to extract from URL
	projectID := p.projectID
	if projectID == "" {
		// Fallback to trying to extract from a URL (this might not work)
		var extractErr error
		projectID, _, extractErr = p.extractMRInfo(fmt.Sprintf("%s/-/merge_requests/%d", p.config.URL, mrIID))
		if extractErr != nil {
			return fmt.Errorf("failed to get project ID: %w", extractErr)
		}
	}

	// Prepare the comment text
	commentText := comment.Content

	// Add severity information
	if comment.Severity != "" {
		commentText = fmt.Sprintf("**Severity: %s**\n\n%s", comment.Severity, commentText)
	}

	// Add suggestions
	if len(comment.Suggestions) > 0 {
		commentText += "\n\n**Suggestions:**\n"
		for _, suggestion := range comment.Suggestions {
			commentText += fmt.Sprintf("- %s\n", suggestion)
		}
	}

	fmt.Printf("Posting comment on MR #%d for project %s\n", mrIID, projectID)

	// If we have file path and line number, post a line-specific comment
	if comment.FilePath != "" && comment.Line > 0 {
		fmt.Printf("  (as line comment on %s:%d)\n", comment.FilePath, comment.Line)
		return p.httpClient.CreateMRLineComment(projectID, mrIID, comment.FilePath, comment.Line, commentText)
	} else {
		// Otherwise post a general MR comment
		return p.httpClient.CreateMRComment(projectID, mrIID, commentText)
	}
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
	// Extract URL
	if url, ok := config["url"].(string); ok && url != "" {
		p.config.URL = url
	} else {
		return fmt.Errorf("GitLab URL is required")
	}

	// Extract token
	if token, ok := config["token"].(string); ok && token != "" {
		p.config.Token = token
	} else {
		return fmt.Errorf("GitLab token is required")
	}

	// Create a new client with the provided token
	client := gitlab.NewClient(nil, p.config.Token)

	// Set the base URL for the GitLab API
	err := client.SetBaseURL(fmt.Sprintf("%s/api/v4", p.config.URL))
	if err != nil {
		return fmt.Errorf("failed to set GitLab API base URL: %w", err)
	}

	// Initialize our custom HTTP client
	httpClient := NewHTTPClient(p.config.URL, p.config.Token)

	// Update the provider
	p.client = client
	p.httpClient = httpClient

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

	// Remove the leading slash from the path
	path := parsedURL.Path
	if path[0] == '/' {
		path = path[1:]
	}

	// Look for the merge_requests part
	re := regexp.MustCompile(`(.+)/-/merge_requests/(\d+)$`)
	matches := re.FindStringSubmatch(path)
	if len(matches) != 3 {
		return "", 0, fmt.Errorf("could not extract project and MR ID from URL: %s", mrURL)
	}

	projectPath := matches[1]
	mrIID, err := strconv.Atoi(matches[2])
	if err != nil {
		return "", 0, fmt.Errorf("invalid MR ID: %w", err)
	}

	fmt.Printf("Extracted project=%s, mrIID=%d from URL=%s\n", projectPath, mrIID, mrURL)
	return projectPath, mrIID, nil
}
