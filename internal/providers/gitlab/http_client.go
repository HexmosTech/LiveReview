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

// Helper function to mask tokens for safe logging
func maskToken(token string) string {
	if len(token) <= 10 {
		return "[HIDDEN]"
	}
	return token[:5] + "..." + token[len(token)-5:]
}

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

// GitLabCommit represents a commit returned by the MR commits API
type GitLabCommit struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id"`
	Title          string   `json:"title"`
	Message        string   `json:"message"`
	AuthorName     string   `json:"author_name"`
	AuthorEmail    string   `json:"author_email"`
	AuthoredDate   string   `json:"authored_date"`
	CommitterName  string   `json:"committer_name"`
	CommitterEmail string   `json:"committer_email"`
	CommittedDate  string   `json:"committed_date"`
	WebURL         string   `json:"web_url"`
	ParentIDs      []string `json:"parent_ids"`
}

// GitLabDiscussion represents a discussion thread in an MR
type GitLabDiscussion struct {
	ID             string       `json:"id"`
	IndividualNote bool         `json:"individual_note"`
	Notes          []GitLabNote `json:"notes"`
}

// GitLabNote represents an individual note (comment) within a discussion
type GitLabNote struct {
	ID         int             `json:"id"`
	Type       string          `json:"type"`
	Body       string          `json:"body"`
	Author     GitLabUser      `json:"author"`
	CreatedAt  string          `json:"created_at"`
	UpdatedAt  string          `json:"updated_at"`
	System     bool            `json:"system"`
	Resolvable bool            `json:"resolvable"`
	Resolved   bool            `json:"resolved"`
	ResolvedBy *GitLabUser     `json:"resolved_by"`
	ResolvedAt string          `json:"resolved_at"`
	Position   *GitLabPosition `json:"position"`
}

// GitLabUser is a minimal user representation
type GitLabUser struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// GitLabPosition captures diff position for a diff note
type GitLabPosition struct {
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	OldPath      string `json:"old_path"`
	NewPath      string `json:"new_path"`
	PositionType string `json:"position_type"`
	OldLine      int    `json:"old_line"`
	NewLine      int    `json:"new_line"`
}

// GetMergeRequest gets a merge request by project ID and MR IID
func (c *GitLabHTTPClient) GetMergeRequest(projectID string, mrIID int) (*GitLabMergeRequest, error) {
	// Create the correct URL with plural 'merge_requests'
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d",
		c.baseURL, url.PathEscape(projectID), mrIID)

	fmt.Printf("GetMergeRequest: Making request to %s\n", requestURL)

	// Determine token type from prefix
	tokenType := "unknown"
	if strings.HasPrefix(c.token, "glpat-") {
		tokenType = "personal access token"
	} else if strings.HasPrefix(c.token, "glrt-") {
		tokenType = "refresh token"
	} else if strings.HasPrefix(c.token, "gloa-") {
		tokenType = "oauth access token"
	}

	fmt.Printf("GetMergeRequest: Using token: %s (type: %s, length: %d)\n",
		maskToken(c.token), tokenType, len(c.token))

	// Make the request
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	// Check if token looks like a personal access token (PAT) with glpat- prefix
	// If not, try to use Bearer authentication which is used for OAuth tokens
	if strings.HasPrefix(c.token, "glpat-") {
		fmt.Println("GetMergeRequest: Using PRIVATE-TOKEN authentication")
		req.Header.Add("PRIVATE-TOKEN", c.token)
	} else {
		fmt.Println("GetMergeRequest: Token doesn't have glpat- prefix, trying Bearer authentication")
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

	// Print all request headers for debugging
	fmt.Println("GetMergeRequest: Request headers:")
	for key, values := range req.Header {
		fmt.Printf("  %s: %s\n", key, values)
	}

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("GetMergeRequest: API request failed with status %d: %s\n", resp.StatusCode, string(body))
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
	// Check if token looks like a personal access token (PAT) with glpat- prefix
	// If not, try to use Bearer authentication which is used for OAuth tokens
	if strings.HasPrefix(c.token, "glpat-") {
		fmt.Println("GetMergeRequestChanges: Using PRIVATE-TOKEN authentication")
		req.Header.Add("PRIVATE-TOKEN", c.token)
	} else {
		fmt.Println("GetMergeRequestChanges: Token doesn't have glpat- prefix, trying Bearer authentication")
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

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

// GetMergeRequestCommits lists commits for a merge request
func (c *GitLabHTTPClient) GetMergeRequestCommits(projectID string, mrIID int) ([]GitLabCommit, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/commits",
		c.baseURL, url.PathEscape(projectID), mrIID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if strings.HasPrefix(c.token, "glpat-") {
		req.Header.Add("PRIVATE-TOKEN", c.token)
	} else {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var commits []GitLabCommit
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return commits, nil
}

// GetMergeRequestDiscussions lists discussions and their notes for a merge request
func (c *GitLabHTTPClient) GetMergeRequestDiscussions(projectID string, mrIID int) ([]GitLabDiscussion, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if strings.HasPrefix(c.token, "glpat-") {
		req.Header.Add("PRIVATE-TOKEN", c.token)
	} else {
		req.Header.Add("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var discussions []GitLabDiscussion
	if err := json.NewDecoder(resp.Body).Decode(&discussions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return discussions, nil
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
// This is now implemented using CreateMRGeneralComment for consistency
func (c *GitLabHTTPClient) CreateMRComment(projectID string, mrIID int, comment string) error {
	return c.CreateMRGeneralComment(projectID, mrIID, comment)
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

// CreateMRLineComment function is now implemented in gitlab_comment.go
// All references to this function will use the enhanced implementation there.

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

	// Intentionally omit position[line_code] for single-line comments

	// Include old path for completeness; old_line only when targeting deleted lines (handled upstream)
	form.Add("position[old_path]", filePath)

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

// GetFileRawAtRef fetches the raw content of a file at a specific ref (commit SHA or branch).
// API: GET /projects/:id/repository/files/:file_path/raw?ref=<ref>
func (c *GitLabHTTPClient) GetFileRawAtRef(projectID string, filePath string, ref string) (string, error) {
	if filePath == "" || ref == "" {
		return "", fmt.Errorf("file path and ref are required")
	}
	// GitLab expects URL-encoded file path in the URL segment
	requestURL := fmt.Sprintf("%s/projects/%s/repository/files/%s/raw?ref=%s",
		c.baseURL, url.PathEscape(projectID), url.PathEscape(filePath), url.QueryEscape(ref))

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
		return "", fmt.Errorf("get file raw failed with status %d: %s", resp.StatusCode, string(body))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	return string(data), nil
}

// CompareCommitsRaw fetches a raw compare diff between base and head SHAs for a project.
// API: GET /projects/:id/repository/compare?from=base&to=head&straight=true
// Returns a list of CodeDiffs with a single hunk containing the raw diff text per file.
func (c *GitLabHTTPClient) CompareCommitsRaw(projectID string, fromSHA, toSHA string) ([]*models.CodeDiff, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/repository/compare?from=%s&to=%s&straight=true",
		c.baseURL, url.PathEscape(projectID), url.QueryEscape(fromSHA), url.QueryEscape(toSHA))

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
		return nil, fmt.Errorf("compare API failed with status %d: %s", resp.StatusCode, string(body))
	}

	// The compare API returns a structured JSON with diffs including new_path/old_path/diff
	var payload struct {
		Diffs []struct {
			OldPath     string `json:"old_path"`
			NewPath     string `json:"new_path"`
			Diff        string `json:"diff"`
			NewFile     bool   `json:"new_file"`
			RenamedFile bool   `json:"renamed_file"`
			DeletedFile bool   `json:"deleted_file"`
		} `json:"diffs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode compare response: %w", err)
	}

	diffs := make([]*models.CodeDiff, 0, len(payload.Diffs))
	for _, d := range payload.Diffs {
		diffs = append(diffs, &models.CodeDiff{
			FilePath:    d.NewPath,
			OldFilePath: d.OldPath,
			IsNew:       d.NewFile,
			IsDeleted:   d.DeletedFile,
			IsRenamed:   d.RenamedFile,
			Hunks: []models.DiffHunk{{
				Content: d.Diff,
			}},
		})
	}
	return diffs, nil
}

// AwardEmojiOnMRNote awards an emoji (e.g., "eyes") on a specific MR note
// API: POST /projects/:id/merge_requests/:merge_request_iid/notes/:note_id/award_emoji
func (c *GitLabHTTPClient) AwardEmojiOnMRNote(projectID string, mrIID int, noteID int, emojiName string) error {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes/%d/award_emoji",
		c.baseURL, url.PathEscape(projectID), mrIID, noteID)

	form := url.Values{}
	form.Add("name", emojiName)

	req, err := http.NewRequest("POST", requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("award emoji failed with status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ReplyToDiscussionNote posts a reply to a discussion thread in an MR
// API: POST /projects/:id/merge_requests/:merge_request_iid/discussions/:discussion_id/notes
func (c *GitLabHTTPClient) ReplyToDiscussionNote(projectID string, mrIID int, discussionID string, body string) error {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions/%s/notes",
		c.baseURL, url.PathEscape(projectID), mrIID, url.PathEscape(discussionID))

	form := url.Values{}
	form.Add("body", body)

	req, err := http.NewRequest("POST", requestURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyB, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reply to discussion failed with status %d: %s", resp.StatusCode, string(bodyB))
	}
	return nil
}
