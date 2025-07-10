package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

// GitLabHTTPClient is a custom HTTP client for GitLab API
// that doesn't rely on the official client which has endpoint issues
type GitLabHTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewHTTPClient creates a new GitLab HTTP client
func NewHTTPClient(baseURL, token string) *GitLabHTTPClient {
	// Make sure baseURL doesn't end with a slash
	if baseURL[len(baseURL)-1] == '/' {
		baseURL = baseURL[:len(baseURL)-1]
	}

	return &GitLabHTTPClient{
		baseURL: fmt.Sprintf("%s/api/v4", baseURL),
		token:   token,
		client:  &http.Client{},
	}
}

// GitLabMergeRequest represents a GitLab merge request
type GitLabMergeRequest struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	ProjectID    int    `json:"project_id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	WebURL       string `json:"web_url"`
	Author       struct {
		ID       int    `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"author"`
}

// GitLabMergeRequestChanges represents the changes in a GitLab merge request
type GitLabMergeRequestChanges struct {
	ID        int    `json:"id"`
	IID       int    `json:"iid"`
	ProjectID int    `json:"project_id"`
	Title     string `json:"title"`
	State     string `json:"state"`
	Changes   []struct {
		OldPath     string `json:"old_path"`
		NewPath     string `json:"new_path"`
		Diff        string `json:"diff"`
		NewFile     bool   `json:"new_file"`
		RenamedFile bool   `json:"renamed_file"`
		DeletedFile bool   `json:"deleted_file"`
	} `json:"changes"`
}

// GetMergeRequest gets a merge request by project ID and MR IID
func (c *GitLabHTTPClient) GetMergeRequest(projectID string, mrIID int) (*GitLabMergeRequest, error) {
	// Create the correct URL with plural 'merge_requests'
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Make the request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	req.Header.Add("PRIVATE-TOKEN", c.token)

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var mr GitLabMergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &mr, nil
}

// GetMergeRequestChanges gets the changes for a merge request
func (c *GitLabHTTPClient) GetMergeRequestChanges(projectID string, mrIID int) (*GitLabMergeRequestChanges, error) {
	// Create the correct URL with plural 'merge_requests'
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/changes",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Make the request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	req.Header.Add("PRIVATE-TOKEN", c.token)

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var changes GitLabMergeRequestChanges
	if err := json.NewDecoder(resp.Body).Decode(&changes); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &changes, nil
}

// ListMergeRequests lists merge requests for a project
func (c *GitLabHTTPClient) ListMergeRequests(projectID string) ([]GitLabMergeRequest, error) {
	// Create the correct URL with plural 'merge_requests'
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests?state=opened",
		c.baseURL, url.PathEscape(projectID))

	// Make the request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	req.Header.Add("PRIVATE-TOKEN", c.token)

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var mrs []GitLabMergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mrs); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return mrs, nil
}

// CreateMRComment creates a comment on a merge request
func (c *GitLabHTTPClient) CreateMRComment(projectID string, mrIID int, comment string) error {
	// Create the correct URL
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Create the query parameters
	values := url.Values{}
	values.Add("body", comment)

	// Make the request
	req, err := http.NewRequest("POST", requestURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add the query parameters
	req.URL.RawQuery = values.Encode()

	// Add authentication
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ConvertToMergeRequestDetails converts a GitLab MR to our internal model
func ConvertToMergeRequestDetails(mr *GitLabMergeRequest, projectID string) *providers.MergeRequestDetails {
	return &providers.MergeRequestDetails{
		ID:           fmt.Sprintf("%d", mr.IID),
		ProjectID:    projectID,
		Title:        mr.Title,
		Description:  mr.Description,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		Author:       mr.Author.Username,
		State:        mr.State,
		URL:          mr.WebURL,
		ProviderType: "gitlab",
		MergeStatus:  "unknown", // Not available in this API response
	}
}

// ConvertToCodeDiffs converts GitLab changes to our internal model
func ConvertToCodeDiffs(changes *GitLabMergeRequestChanges) []*models.CodeDiff {
	diffs := make([]*models.CodeDiff, 0, len(changes.Changes))

	for _, change := range changes.Changes {
		// Create a new CodeDiff
		diff := &models.CodeDiff{
			FilePath:    change.NewPath,
			OldFilePath: change.OldPath,
			IsNew:       change.NewFile,
			IsDeleted:   change.DeletedFile,
			IsRenamed:   change.RenamedFile,
			// We don't have the full content from the API, just the diff
			Hunks: []models.DiffHunk{
				{
					// We don't have line numbers from the API in this response
					Content: change.Diff,
				},
			},
		}

		diffs = append(diffs, diff)
	}

	return diffs
}
