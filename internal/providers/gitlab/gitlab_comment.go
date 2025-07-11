package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// MRVersion represents a merge request version information
type MRVersion struct {
	ID             int    `json:"id"`
	HeadCommitSHA  string `json:"head_commit_sha"`
	BaseCommitSHA  string `json:"base_commit_sha"`
	StartCommitSHA string `json:"start_commit_sha"`
	CreatedAt      string `json:"created_at"`
	MergeRequestID int    `json:"merge_request_id"`
	State          string `json:"state"`
	RealSize       string `json:"real_size"`
}

// GetLatestMRVersion gets the latest version of a merge request
// This is needed for properly positioning comments in the diff
func (c *GitLabHTTPClient) GetLatestMRVersion(projectID string, mrIID int) (*MRVersion, error) {
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

	var versions []MRVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found for this merge request")
	}

	// Return the latest version (first in the list)
	return &versions[0], nil
}

// CreateMRDiscussion creates a new discussion thread in a merge request
// This is used for both general comments and line comments
func (c *GitLabHTTPClient) CreateMRDiscussion(projectID string, mrIID int, comment string, position map[string]interface{}) (map[string]interface{}, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Create request data
	requestData := map[string]interface{}{
		"body": comment,
	}

	// If position data is provided, add it to the request
	if position != nil {
		requestData["position"] = position
	}

	// Convert request data to JSON
	requestBody, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", requestURL, strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Add("PRIVATE-TOKEN", c.token)
	req.Header.Add("Content-Type", "application/json")

	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check for errors
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse and return the response
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response, nil
}

// CreateMRLineCommentWithPosition creates a comment on a specific line in a file in a merge request
// using the discussions endpoint and providing position data
func (c *GitLabHTTPClient) CreateMRLineCommentWithPosition(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	// Get the latest merge request version to obtain required SHAs
	version, err := c.GetLatestMRVersion(projectID, mrIID)
	if err != nil {
		return fmt.Errorf("failed to get MR version: %w", err)
	}

	// Normalize file path (remove leading slash)
	filePath = strings.TrimPrefix(filePath, "/")

	// Prepare position data according to GitLab API requirements
	position := map[string]interface{}{
		"position_type": "text",
		"base_sha":      version.BaseCommitSHA,
		"head_sha":      version.HeadCommitSHA,
		"start_sha":     version.StartCommitSHA,
		"new_path":      filePath,
		"old_path":      filePath, // Usually same as new_path unless file is renamed
		"new_line":      lineNum,  // Line in the new version
	}

	// Create the discussion with the position data
	_, err = c.CreateMRDiscussion(projectID, mrIID, comment, position)
	if err != nil {
		return fmt.Errorf("failed to create line comment: %w", err)
	}

	return nil
}

// CreateMRGeneralComment creates a general comment on a merge request
func (c *GitLabHTTPClient) CreateMRGeneralComment(projectID string, mrIID int, comment string) error {
	// Create a discussion without position data for a general comment
	_, err := c.CreateMRDiscussion(projectID, mrIID, comment, nil)
	if err != nil {
		return fmt.Errorf("failed to create general comment: %w", err)
	}

	return nil
}

// Enhanced implementation of CreateMRLineComment that uses the newer approach
// This overrides the implementation in http_client.go
func (c *GitLabHTTPClient) CreateMRLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	// Log what we're trying to do
	fmt.Printf("Creating line comment on %s line %d for MR %d in project %s\n",
		filePath, lineNum, mrIID, projectID)

	// Try the newer approach first
	err := c.CreateMRLineCommentWithPosition(projectID, mrIID, filePath, lineNum, comment)
	if err == nil {
		fmt.Println("Successfully created line comment using position data")
		return nil
	}

	// Log the error and fall back
	fmt.Printf("Failed to create line comment with position data: %v\n", err)
	fmt.Println("Trying fallback method...")

	// Fall back to the existing method
	err = c.CreateLineCommentViaDiscussions(projectID, mrIID, filePath, lineNum, comment)
	if err == nil {
		fmt.Println("Successfully created line comment via discussions approach")
		return nil
	}

	// Log this error too
	fmt.Printf("Failed to create line comment via discussions: %v\n", err)
	fmt.Println("Trying direct approach...")

	// As a last resort, try the direct approach
	err = c.createDirectLineComment(projectID, mrIID, filePath, lineNum, comment)
	if err == nil {
		fmt.Println("Successfully created line comment using direct approach")
		return nil
	}

	// If all else fails, use the fallback to create a regular comment with file/line info
	fmt.Printf("All line comment methods failed: %v\n", err)
	fmt.Println("Falling back to regular comment with file/line info")
	return c.createFallbackLineComment(projectID, mrIID, filePath, lineNum, comment)
}
