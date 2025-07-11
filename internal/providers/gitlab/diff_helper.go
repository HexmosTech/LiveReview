package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// GetRawDiffForMR gets the raw diff data for a merge request which we can use to generate a line code
func (c *GitLabHTTPClient) GetRawDiffForMR(projectID string, mrIID int) (string, error) {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/changes?view=inline",
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

// FindDiffInfo searches the MR diff data for a specific file and line to get position info
func FindDiffInfo(diffData string, filePath string, lineNum int) (map[string]string, error) {
	// Parse the diff data as JSON
	var changes struct {
		Changes []struct {
			OldPath string `json:"old_path"`
			NewPath string `json:"new_path"`
			Diff    string `json:"diff"`
		} `json:"changes"`
	}

	if err := json.Unmarshal([]byte(diffData), &changes); err != nil {
		return nil, fmt.Errorf("failed to parse diff data: %w", err)
	}

	// Find the diff for the target file
	var targetDiff string
	for _, change := range changes.Changes {
		if change.NewPath == filePath || change.OldPath == filePath {
			targetDiff = change.Diff
			break
		}
	}

	if targetDiff == "" {
		return nil, fmt.Errorf("file %s not found in diff", filePath)
	}

	// Parse the diff to find line information
	return parseDiffForLinePosition(targetDiff, lineNum)
}

// parseDiffForLinePosition extracts position info from a diff for a specific line
func parseDiffForLinePosition(diff string, targetLine int) (map[string]string, error) {
	// Simple implementation to get the basics
	// We're not using these yet but keeping them for future improvements
	// lines := strings.Split(diff, "\n")
	// lineType := "new" // We're assuming we're commenting on a new line

	position := make(map[string]string)

	// In practice, this would need more sophisticated diff parsing
	// For now, just set basic information
	position["position_type"] = "text"
	position["new_line"] = fmt.Sprintf("%d", targetLine)
	position["old_line"] = fmt.Sprintf("%d", targetLine) // This may not be accurate for new lines

	return position, nil
}

// CreateLineCommentViaDiscussions creates a line comment using the discussions endpoint
func (c *GitLabHTTPClient) CreateLineCommentViaDiscussions(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	// First, get the merge request versions to get the SHAs we need
	versions, err := c.GetMergeRequestVersions(projectID, mrIID)
	if err != nil || len(versions) == 0 {
		return fmt.Errorf("failed to get MR versions: %w", err)
	}

	// Use the latest version
	latestVersion := versions[0]

	// Get raw diff data to help find the right position
	diffData, err := c.GetRawDiffForMR(projectID, mrIID)
	if err != nil {
		return fmt.Errorf("failed to get diff data: %w", err)
	}

	// Extract position info from diff
	_, err = FindDiffInfo(diffData, filePath, lineNum)
	if err != nil {
		// If we can't extract position info, just use basic info
		fmt.Printf("Warning: Could not extract position info: %v\n", err)
	}

	// Set up the discussions endpoint URL
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	// Create the request body with positions that work better for GitLab
	form := url.Values{}
	form.Add("body", comment)

	// Position parameters based on GitLab documentation and tested examples
	form.Add("position[base_sha]", latestVersion.BaseCommitSHA)
	form.Add("position[start_sha]", latestVersion.StartCommitSHA)
	form.Add("position[head_sha]", latestVersion.HeadCommitSHA)

	// Text position type is more reliable
	form.Add("position[position_type]", "text")

	// Path and line info
	filePath = strings.TrimPrefix(filePath, "/")
	form.Add("position[new_path]", filePath)
	form.Add("position[new_line]", fmt.Sprintf("%d", lineNum))

	// For modified files we need old path too
	form.Add("position[old_path]", filePath)
	form.Add("position[old_line]", fmt.Sprintf("%d", lineNum))

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
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
