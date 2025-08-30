package gitlab

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// AlternativeCreateMRLineComment is an alternative implementation of line comment creation
func (c *GitLabHTTPClient) AlternativeCreateMRLineComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	fmt.Printf("Creating line comment for file %s at line %d\n", filePath, lineNum)

	// Normalize the file path
	// Remove any leading slash that might cause issues
	filePath = strings.TrimPrefix(filePath, "/")

	// DEBUG info to help diagnose issues
	fmt.Printf("DEBUG: Creating line comment\n")
	fmt.Printf("DEBUG: - Project ID: %s\n", projectID)
	fmt.Printf("DEBUG: - MR IID: %d\n", mrIID)
	fmt.Printf("DEBUG: - File Path: %s (normalized)\n", filePath)
	fmt.Printf("DEBUG: - Line Number: %d\n", lineNum)
	fmt.Printf("DEBUG: - Comment begins: %s\n", getCommentBeginningAlt(comment, 50))

	// Try the simple direct approach first - this works on newer GitLab versions
	// Method 1: Direct API call with minimum parameters
	err := c.createSimpleDiffComment(projectID, mrIID, filePath, lineNum, comment)
	if err == nil {
		// This worked! No need to try other methods
		fmt.Println("Successfully posted comment using simple method")
		return nil
	}

	fmt.Printf("Simple method failed: %v\nTrying discussion method...\n", err)

	// Method 2: Try the more complex discussion approach with position info
	// First, get the merge request versions to get the SHAs we need
	versions, err := c.GetMergeRequestVersions(projectID, mrIID)
	if err != nil {
		fmt.Printf("Error getting MR versions: %v\nFalling back to direct line comment method\n", err)
		return c.createDirectLineComment(projectID, mrIID, filePath, lineNum, comment)
	}

	if len(versions) == 0 {
		fmt.Println("No versions found for this MR, falling back to direct line comment method")
		return c.createDirectLineComment(projectID, mrIID, filePath, lineNum, comment)
	}

	// Use the latest version (first in the list)
	latestVersion := versions[0]
	fmt.Printf("Using MR version with base_sha=%s, start_sha=%s, head_sha=%s\n",
		latestVersion.BaseCommitSHA, latestVersion.StartCommitSHA, latestVersion.HeadCommitSHA)

	// Try the discussions endpoint with text position type
	err = c.createPositionComment(projectID, mrIID, filePath, lineNum, comment, latestVersion, "text")
	if err == nil {
		fmt.Println("Successfully posted comment using text position type")
		return nil
	}

	fmt.Printf("Text position type failed: %v\nTrying code position type...\n", err)

	// Try with code position type
	err = c.createPositionComment(projectID, mrIID, filePath, lineNum, comment, latestVersion, "code")
	if err == nil {
		fmt.Println("Successfully posted comment using code position type")
		return nil
	}

	fmt.Printf("Code position type failed: %v\nFalling back to direct line comment method\n", err)

	// Try the original direct line comment method
	err = c.createDirectLineComment(projectID, mrIID, filePath, lineNum, comment)
	if err == nil {
		fmt.Println("Successfully posted comment using direct line comment method")
		return nil
	}

	fmt.Printf("Direct line comment method failed: %v\nFalling back to fallback method\n", err)

	// As a last resort, try the fallback method (general comment with file/line info)
	return c.createFallbackLineComment(projectID, mrIID, filePath, lineNum, comment)
}

// createSimpleDiffComment tries to create a diff comment using the simplest possible parameters
func (c *GitLabHTTPClient) createSimpleDiffComment(projectID string, mrIID int, filePath string, lineNum int, comment string) error {
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/notes",
		c.baseURL, url.PathEscape(projectID), mrIID)

	fmt.Printf("DEBUG: Simple diff comment URL: %s\n", requestURL)

	// Create the query parameters with just the minimal set needed
	values := url.Values{}
	values.Add("body", comment)
	values.Add("path", filePath)
	values.Add("line", fmt.Sprintf("%d", lineNum))
	values.Add("line_type", "new") // Comment on the new version

	fmt.Println("DEBUG: Simple diff comment parameters:")
	fmt.Printf("DEBUG: - path: %s\n", values.Get("path"))
	fmt.Printf("DEBUG: - line: %s\n", values.Get("line"))
	fmt.Printf("DEBUG: - line_type: %s\n", values.Get("line_type"))

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

	// Check the response
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("DEBUG: Simple diff comment succeeded with status %d\n", resp.StatusCode)
	return nil
}

// createPositionComment creates a comment with the discussions endpoint and a specific position type
func (c *GitLabHTTPClient) createPositionComment(
	projectID string,
	mrIID int,
	filePath string,
	lineNum int,
	comment string,
	version struct {
		ID             int    `json:"id"`
		HeadCommitSHA  string `json:"head_commit_sha"`
		BaseCommitSHA  string `json:"base_commit_sha"`
		StartCommitSHA string `json:"start_commit_sha"`
		CreatedAt      string `json:"created_at"`
		MergeRequestID int    `json:"merge_request_id"`
		State          string `json:"state"`
		RealSize       string `json:"real_size"`
	},
	positionType string,
) error {
	// Create the correct URL for discussions (which supports line comments)
	requestURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d/discussions",
		c.baseURL, url.PathEscape(projectID), mrIID)

	fmt.Printf("DEBUG: Position comment URL: %s\n", requestURL)
	fmt.Printf("DEBUG: Position type: %s\n", positionType)

	// Create form data for the request
	form := url.Values{}
	form.Add("body", comment)
	form.Add("position[base_sha]", version.BaseCommitSHA)
	form.Add("position[start_sha]", version.StartCommitSHA)
	form.Add("position[head_sha]", version.HeadCommitSHA)
	form.Add("position[position_type]", positionType)
	form.Add("position[new_path]", filePath)
	form.Add("position[new_line]", fmt.Sprintf("%d", lineNum))

	// Generate a line_code based on SHAs and path - this is required by GitLab
	lineCode := fmt.Sprintf("%s_%s_%s_%s_%d",
		version.BaseCommitSHA[:8],
		version.HeadCommitSHA[:8],
		strings.ReplaceAll(filePath, "/", "_"),
		"right", // We're always commenting on the new version
		lineNum)
	form.Add("position[line_code]", lineCode)

	// Add old_path and old_line only for modified files, not for new files
	// For now, let's assume the file is modified and set both
	form.Add("position[old_path]", filePath)
	form.Add("position[old_line]", fmt.Sprintf("%d", lineNum))

	fmt.Println("DEBUG: Position comment parameters:")
	fmt.Printf("DEBUG: - position[position_type]: %s\n", form.Get("position[position_type]"))
	fmt.Printf("DEBUG: - position[new_path]: %s\n", form.Get("position[new_path]"))
	fmt.Printf("DEBUG: - position[new_line]: %s\n", form.Get("position[new_line]"))
	fmt.Printf("DEBUG: - position[line_code]: %s\n", form.Get("position[line_code]"))

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

	fmt.Printf("DEBUG: Position comment succeeded with status %d\n", resp.StatusCode)
	return nil
}

// getCommentBeginningAlt is an alternative helper function to get the beginning of a comment for logging purposes
func getCommentBeginningAlt(comment string, maxLen int) string {
	if len(comment) <= maxLen {
		return comment
	}
	return comment[:maxLen] + "..."
}
