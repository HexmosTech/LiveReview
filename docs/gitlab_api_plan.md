# GitLab API Integration Plan

This document outlines the plan for properly implementing the GitLab API integration to replace the current mock implementation.

## Current Issues

The GitLab client library v0.3.0 has issues with API endpoint paths:
- It uses `/merge_request/` (singular) instead of `/merge_requests/` (plural)
- This causes 404 errors when trying to fetch MR details or changes

## Implementation Options

### Option 1: Upgrade GitLab Client Version

The preferred approach is to upgrade to a newer version of the GitLab client that has fixed the endpoint issues.

1. Run the `scripts/test_gitlab_versions.sh` script to test different client versions
2. Identify a version that correctly uses `/merge_requests/` plural endpoints
3. Update the go.mod file to use the working version
4. Refactor the GitLab provider implementation to use the updated client

### Option 2: Direct HTTP Requests

If upgrading is not possible, implement direct HTTP requests to the GitLab API:

1. Create a custom HTTP client in `internal/providers/gitlab/http_client.go`
2. Implement functions to directly call GitLab API endpoints with the correct paths
3. Update the GitLab provider to use this custom client instead of the official one

Example implementation:

```go
// http_client.go
package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type GitLabHTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewHTTPClient(baseURL, token string) *GitLabHTTPClient {
	return &GitLabHTTPClient{
		baseURL: fmt.Sprintf("%s/api/v4", baseURL),
		token:   token,
		client:  &http.Client{},
	}
}

func (c *GitLabHTTPClient) GetMergeRequest(projectID string, mrIID int) (*GitLabMergeRequest, error) {
	// Create the correct URL with plural 'merge_requests'
	url := fmt.Sprintf("%s/projects/%s/merge_requests/%d", 
		c.baseURL, url.PathEscape(projectID), mrIID)
	
	// Make the request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	// Add authentication
	req.Header.Add("PRIVATE-TOKEN", c.token)
	
	// Execute the request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	// Check for errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}
	
	// Parse the response
	var mr GitLabMergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, err
	}
	
	return &mr, nil
}

// Add other methods for GetMergeRequestChanges, etc.
```

### Option 3: Create a Custom Fork

If none of the above options work:

1. Fork the GitLab client repository
2. Fix the endpoint path issues
3. Update the go.mod file to use your fork
4. Submit a pull request to the original repository

## Implementation Timeline

1. **Phase 1: Research and Testing (1-2 days)**
   - Test different client versions
   - Document findings
   - Select the best approach

2. **Phase 2: Implementation (2-3 days)**
   - Implement the chosen solution
   - Update the GitLab provider
   - Add tests

3. **Phase 3: Testing and Validation (1-2 days)**
   - Test with real GitLab instances
   - Fix any issues
   - Document the implementation

## Future Enhancements

Once the basic GitLab API integration is working:

1. **Improve Error Handling**
   - Add retries for failed API requests
   - Add better error messages

2. **Add Pagination Support**
   - Handle large merge requests with many changes
   - Add support for loading changes in chunks

3. **Optimize API Requests**
   - Cache API responses where appropriate
   - Minimize the number of API calls
