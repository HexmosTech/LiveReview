package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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

// GetMergeRequestDiffs gets the raw diff for a merge request
func (c *GitLabHTTPClient) GetMergeRequestDiffs(projectID string, mrIID int) (string, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/diffs?view=inline",
		c.baseURL, url.PathEscape(projectID), mrIID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("PRIVATE-TOKEN", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// We're only interested in the raw diff data
	var diffs []struct {
		Diff string `json:"diff"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&diffs); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(diffs) == 0 {
		return "", fmt.Errorf("no diffs found")
	}

	return diffs[0].Diff, nil
}

// GetMergeRequestVersions gets the versions of a merge request
func (c *GitLabHTTPClient) GetMergeRequestVersions(projectID string, mrIID int) ([]struct {
	ID             int    `json:"id"`
	HeadCommitSHA  string `json:"head_commit_sha"`
	BaseCommitSHA  string `json:"base_commit_sha"`
	StartCommitSHA string `json:"start_commit_sha"`
	CreatedAt      string `json:"created_at"`
	MergeRequestID int    `json:"merge_request_id"`
	State          string `json:"state"`
	RealSize       string `json:"real_size"`
}, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/versions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("PRIVATE-TOKEN", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var versions []struct {
		ID             int    `json:"id"`
		HeadCommitSHA  string `json:"head_commit_sha"`
		BaseCommitSHA  string `json:"base_commit_sha"`
		StartCommitSHA string `json:"start_commit_sha"`
		CreatedAt      string `json:"created_at"`
		MergeRequestID int    `json:"merge_request_id"`
		State          string `json:"state"`
		RealSize       string `json:"real_size"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return versions, nil
}

// CreateMRLineComment creates a comment on a specific line in a file in a merge request
func (c *GitLabHTTPClient) CreateMRLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	// Normalize file path - remove any leading slash
	filePath = strings.TrimPrefix(filePath, "/")

	fmt.Printf("DEBUG: Creating line comment\n")
	fmt.Printf("DEBUG: - Project ID: %s\n", projectID)
	fmt.Printf("DEBUG: - MR IID: %d\n", mrIID)
	fmt.Printf("DEBUG: - File Path: %s (normalized)\n", filePath)
	fmt.Printf("DEBUG: - Line Number: %d\n", lineNum)
	fmt.Printf("DEBUG: - Comment begins: %s\n", getCommentBeginning(comment, 50))

	// First try the discussions-based approach which should work for GitLab's Changes tab
	err := c.CreateLineCommentViaDiscussions(projectID, mrIID, filePath, lineNum, comment)
	if err == nil {
		fmt.Println("DEBUG: Successfully posted comment using discussions approach")
		return nil
	}

	fmt.Printf("DEBUG: Discussions approach failed: %v\n", err)
	fmt.Println("DEBUG: Trying notes API approach...")

	// Try the notes API approach next
	// Create the correct URL
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Create the form data
	form := url.Values{}
	form.Add("body", comment)
	form.Add("path", filePath)
	form.Add("line", fmt.Sprintf("%d", lineNum))
	form.Add("line_type", "new")

	// Make the request
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response
	if resp.StatusCode == http.StatusCreated {
		fmt.Println("DEBUG: Successfully posted comment using notes API")
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("DEBUG: Notes API failed with status %d: %s\n", resp.StatusCode, string(body))

	// As a last resort, fall back to a regular comment with file and line info
	fmt.Println("DEBUG: All line comment methods failed, falling back to regular comment with file/line info")
	return c.createFallbackLineComment(projectID, mrIID, filePath, lineNum, comment)
}

// createDirectLineComment tries to create a line comment using a simpler method
// This method uses note_position_type which is used in newer GitLab versions
func (c *GitLabHTTPClient) createDirectLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	fmt.Println("DEBUG: Using direct method for line comment")

	// First get the versions to get the SHAs needed for proper positioning
	versions, err := c.GetMergeRequestVersions(projectID, mrIID)
	if err != nil || len(versions) == 0 {
		fmt.Printf("DEBUG: Error getting MR versions: %v\n", err)
		return fmt.Errorf("failed to get MR versions: %v", err)
	}

	// Use the latest version (first in the list)
	latestVersion := versions[0]
	fmt.Printf("DEBUG: Using MR version with base_sha=%s, start_sha=%s, head_sha=%s\n",
		latestVersion.BaseCommitSHA, latestVersion.StartCommitSHA, latestVersion.HeadCommitSHA)

	// Use the discussions endpoint instead of notes for line comments
	// This is crucial for properly attaching comments to specific lines in the Changes tab
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	fmt.Printf("DEBUG: Request URL: %s\n", requestURL)

	// Normalize file path - ensure no leading slash
	filePath = strings.TrimPrefix(filePath, "/")

	// Create the form data with all required parameters
	form := url.Values{}

	// The actual comment text
	form.Add("body", comment)

	// These position parameters are critical for line comments to appear in the Changes tab
	form.Add("position[base_sha]", latestVersion.BaseCommitSHA)
	form.Add("position[start_sha]", latestVersion.StartCommitSHA)
	form.Add("position[head_sha]", latestVersion.HeadCommitSHA)

	// Position type must be "text" for it to work reliably
	form.Add("position[position_type]", "text")

	// Set path and line info for the new version of the file
	form.Add("position[new_path]", filePath)
	form.Add("position[new_line]", fmt.Sprintf("%d", lineNum))

	// Generate a line_code which is crucial for proper positioning
	// Format: SHA1_SHA2_path_line
	lineCode := fmt.Sprintf("%s_%s_%s_%s_%d",
		latestVersion.StartCommitSHA[:8],
		latestVersion.HeadCommitSHA[:8],
		strings.ReplaceAll(filePath, "/", "_"),
		"right", // right side of the diff
		lineNum)
	form.Add("position[line_code]", lineCode)

	// Include old path and line for context (helps GitLab position the comment correctly)
	form.Add("position[old_path]", filePath)
	form.Add("position[old_line]", fmt.Sprintf("%d", lineNum))

	fmt.Printf("DEBUG: Form values for direct line comment:\n")
	for k, v := range form {
		fmt.Printf("DEBUG: - %s: %s\n", k, v[0])
	}

	// Make the request
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		fmt.Printf("DEBUG: Direct line comment failed with status %d\n", resp.StatusCode)
		fmt.Printf("DEBUG: Response body: %s\n", string(body))
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("DEBUG: Direct line comment succeeded with status %d\n", resp.StatusCode)
	fmt.Printf("DEBUG: Response body: %s\n", getCommentBeginning(string(body), 100))

	return nil
}

// createFallbackLineComment creates a regular comment with file and line information
func (c *GitLabHTTPClient) createFallbackLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	fmt.Println("Using fallback method for line comment")
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Format the comment to clearly indicate the file and line
	formattedComment := fmt.Sprintf("**Comment for %s, line %d:**\n\n%s",
		filePath, lineNum, comment)

	// Create the query parameters for a regular comment
	values := url.Values{}
	values.Add("body", formattedComment)

	// Make the request
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
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

// createDiscussionLineComment creates a line comment using the discussions endpoint
// which may have better support for attaching comments to specific lines
// Note: This method is currently unused but kept for reference and possible future use
func (c *GitLabHTTPClient) createDiscussionLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	fmt.Println("DEBUG: Using discussions endpoint for line comment")

	// Get the merge request versions to get the SHAs
	versions, err := c.GetMergeRequestVersions(projectID, mrIID)
	if err != nil {
		fmt.Printf("DEBUG: Error getting MR versions: %v\n", err)
		return err
	}

	if len(versions) == 0 {
		return fmt.Errorf("no versions found for this MR")
	}

	// Use the latest version (first in the list)
	latestVersion := versions[0]
	fmt.Printf("DEBUG: Using MR version with base_sha=%s, start_sha=%s, head_sha=%s\n",
		latestVersion.BaseCommitSHA, latestVersion.StartCommitSHA, latestVersion.HeadCommitSHA)

	// Create the correct URL for discussions endpoint
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	fmt.Printf("DEBUG: Discussions endpoint URL: %s\n", requestURL)

	// Ensure file path has no leading slash
	filePath = strings.TrimPrefix(filePath, "/")

	// Create form data for the request
	form := url.Values{}
	form.Add("body", comment)

	// Position data is critical for line comments
	form.Add("position[base_sha]", latestVersion.BaseCommitSHA)
	form.Add("position[start_sha]", latestVersion.StartCommitSHA)
	form.Add("position[head_sha]", latestVersion.HeadCommitSHA)

	// Try with "text" position_type which sometimes works better
	form.Add("position[position_type]", "text")

	// File path and line information
	form.Add("position[new_path]", filePath)
	form.Add("position[new_line]", fmt.Sprintf("%d", lineNum))

	// Line code is important for GitLab to locate the correct line
	// Format: start_sha_head_sha_file_path_right_line
	lineCode := fmt.Sprintf("%s_%s_%s_%s_%d",
		latestVersion.StartCommitSHA[:8],
		latestVersion.HeadCommitSHA[:8],
		strings.ReplaceAll(filePath, "/", "_"),
		"right", // right side of the diff
		lineNum)
	form.Add("position[line_code]", lineCode)

	// For completeness, add old_path as well
	form.Add("position[old_path]", filePath)
	form.Add("position[old_line]", fmt.Sprintf("%d", lineNum))

	// Print debug info
	fmt.Println("DEBUG: Discussion position parameters:")
	for k, v := range form {
		if strings.HasPrefix(k, "position") {
			fmt.Printf("DEBUG: - %s: %s\n", k, v[0])
		}
	}

	// Make the request
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		fmt.Printf("DEBUG: Discussion endpoint failed with status %d\n", resp.StatusCode)
		fmt.Printf("DEBUG: Response body: %s\n", string(body))
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("DEBUG: Discussion endpoint succeeded with status %d\n", resp.StatusCode)
	fmt.Printf("DEBUG: Response body: %s\n", getCommentBeginning(string(body), 100))
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

// Helper function to get the beginning of a comment for logging purposes
func getCommentBeginning(comment string, maxLen int) string {
	if len(comment) <= maxLen {
		return comment
	}
	return comment[:maxLen] + "..."
}

// TEST METHODS FOR DEBUGGING GITLAB COMMENT ISSUES

// TestCreateDirectLineComment exposes the createDirectLineComment method for testing
func (c *GitLabHTTPClient) TestCreateDirectLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	return c.createDirectLineComment(projectID, mrIID, filePath, lineNum, comment)
}

// TestCreateFallbackLineComment exposes the createFallbackLineComment method for testing
func (c *GitLabHTTPClient) TestCreateFallbackLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	return c.createFallbackLineComment(projectID, mrIID, filePath, lineNum, comment)
}

// GetBaseURL returns the base URL for testing
func (c *GitLabHTTPClient) GetBaseURL() string {
	return c.baseURL
}

// GetToken returns the token for testing
func (c *GitLabHTTPClient) GetToken() string {
	return c.token
}

// TestCreateCommentWithPositionType creates a comment with a specific position_type value
func (c *GitLabHTTPClient) TestCreateCommentWithPositionType(projectID string, mrIID int, filePath string, lineNum int, comment string, positionType string) error {
	// First, get the merge request versions to get the SHAs we need
	versions, err := c.GetMergeRequestVersions(projectID, mrIID)
	if err != nil {
		fmt.Printf("DEBUG: Error getting MR versions: %v\n", err)
		return err
	}

	if len(versions) == 0 {
		return fmt.Errorf("no versions found for this MR")
	}

	// Use the latest version (first in the list)
	latestVersion := versions[0]
	fmt.Printf("Using MR version with base_sha=%s, start_sha=%s, head_sha=%s\n",
		latestVersion.BaseCommitSHA, latestVersion.StartCommitSHA, latestVersion.HeadCommitSHA)

	// Create the correct URL for discussions (which supports line comments)
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Create form data for the request
	form := url.Values{}
	form.Add("body", comment)
	form.Add("position[base_sha]", latestVersion.BaseCommitSHA)
	form.Add("position[start_sha]", latestVersion.StartCommitSHA)
	form.Add("position[head_sha]", latestVersion.HeadCommitSHA)

	// Use the specified position type
	form.Add("position[position_type]", positionType)

	// Add file path and line information
	form.Add("position[new_path]", filePath)
	form.Add("position[new_line]", fmt.Sprintf("%d", lineNum))

	// Generate a line_code
	lineCode := fmt.Sprintf("%s_%s_%s_%s_%d",
		latestVersion.BaseCommitSHA[:8],
		latestVersion.HeadCommitSHA[:8],
		strings.ReplaceAll(filePath, "/", "_"),
		"right",
		lineNum)
	form.Add("position[line_code]", lineCode)

	// Add old_path and old_line (these may be needed for modified files)
	form.Add("position[old_path]", filePath)
	form.Add("position[old_line]", fmt.Sprintf("%d", lineNum))

	// Print debug info
	fmt.Printf("DEBUG: Testing comment with position_type=%s\n", positionType)
	fmt.Printf("DEBUG: Request URL: %s\n", requestURL)
	fmt.Printf("DEBUG: Form data:\n")
	for k, v := range form {
		fmt.Printf("DEBUG:   %s: %s\n", k, v)
	}

	// Make the request
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		fmt.Printf("DEBUG: API request failed with status %d\n", resp.StatusCode)
		fmt.Printf("DEBUG: Response body: %s\n", string(body))
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("DEBUG: API request succeeded with status %d\n", resp.StatusCode)
	fmt.Printf("DEBUG: Response body: %s\n", getCommentBeginning(string(body), 100))
	return nil
}
