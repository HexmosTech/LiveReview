package bitbucket

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// BitbucketRepositoryBasic represents basic repository information from Bitbucket API
type BitbucketRepositoryBasic struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Username string `json:"username"`
	} `json:"owner"`
	IsPrivate bool `json:"is_private"`
}

// BitbucketWorkspaceBasic represents basic workspace information from Bitbucket API
type BitbucketWorkspaceBasic struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// BitbucketAPIResponse represents the paginated response structure from Bitbucket API
type BitbucketAPIResponse struct {
	Values []BitbucketRepositoryBasic `json:"values"`
	Next   string                     `json:"next"`
}

// BitbucketWorkspaceAPIResponse represents the paginated response structure for workspaces
type BitbucketWorkspaceAPIResponse struct {
	Values []BitbucketWorkspaceBasic `json:"values"`
	Next   string                    `json:"next"`
}

// DiscoverProjectsBitbucket fetches all repositories accessible with the given credentials from Bitbucket
func DiscoverProjectsBitbucket(baseURL, email, apiToken string) ([]string, error) {
	var allRepositories []string

	// Create HTTP client
	client := &http.Client{}

	// Bitbucket API base URL - always use the cloud API
	apiBaseURL := "https://api.bitbucket.org/2.0"
	if baseURL != "" && baseURL != "https://bitbucket.org" {
		// For Bitbucket Server (on-premise), the API is typically at /rest/api/1.0
		// Note: This implementation focuses on Bitbucket Cloud
		return nil, fmt.Errorf("bitbucket Server is not currently supported, only Bitbucket Cloud")
	}

	// Try to get repositories directly from the user's accessible repositories
	// This approach works without requiring workspace enumeration permissions
	userRepos, err := getUserAccessibleRepositories(client, apiBaseURL, email, apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get user accessible repositories: %w", err)
	}

	allRepositories = append(allRepositories, userRepos...)

	// If we have workspace permissions, try to get additional workspace repositories
	// This is optional and will fail silently if permissions are missing
	workspaces, err := getAccessibleWorkspaces(client, apiBaseURL, email, apiToken)
	if err != nil {
		// Log the warning but don't fail - workspace enumeration requires additional permissions
		fmt.Printf("Warning: Could not enumerate workspaces (may require read:workspace:bitbucket scope): %v\n", err)
	} else {
		// For each workspace, get all repositories
		for _, workspace := range workspaces {
			repos, err := getWorkspaceRepositories(client, apiBaseURL, email, apiToken, workspace.Slug)
			if err != nil {
				// Log the error but continue with other workspaces
				fmt.Printf("Warning: failed to get repositories for workspace %s: %v\n", workspace.Slug, err)
				continue
			}

			// Add only repositories that aren't already in our list
			for _, repo := range repos {
				found := false
				for _, existingRepo := range allRepositories {
					if existingRepo == repo {
						found = true
						break
					}
				}
				if !found {
					allRepositories = append(allRepositories, repo)
				}
			}
		}
	}

	return allRepositories, nil
}

// getAccessibleWorkspaces fetches all workspaces the user has access to
func getAccessibleWorkspaces(client *http.Client, apiBaseURL, email, apiToken string) ([]BitbucketWorkspaceBasic, error) {
	var allWorkspaces []BitbucketWorkspaceBasic
	nextURL := fmt.Sprintf("%s/workspaces", apiBaseURL)

	for nextURL != "" {
		// Create request
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add authentication and headers
		req.SetBasicAuth(email, apiToken)
		req.Header.Add("Accept", "application/json")
		req.Header.Add("User-Agent", "LiveReview/1.0")

		// Execute request
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}
		defer resp.Body.Close()

		// Check for errors
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var response BitbucketWorkspaceAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Add workspaces to result
		allWorkspaces = append(allWorkspaces, response.Values...)

		// Set next URL for pagination
		nextURL = response.Next
	}

	return allWorkspaces, nil
}

// getWorkspaceRepositories fetches all repositories from a specific workspace
func getWorkspaceRepositories(client *http.Client, apiBaseURL, email, apiToken, workspace string) ([]string, error) {
	var repositories []string
	nextURL := fmt.Sprintf("%s/repositories/%s", apiBaseURL, url.PathEscape(workspace))

	// Add query parameters for pagination and filtering
	params := url.Values{}
	params.Add("pagelen", "100") // Maximum allowed by Bitbucket API
	params.Add("role", "member") // Only repositories where user is a member
	nextURL += "?" + params.Encode()

	for nextURL != "" {
		// Create request
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add authentication and headers
		req.SetBasicAuth(email, apiToken)
		req.Header.Add("Accept", "application/json")
		req.Header.Add("User-Agent", "LiveReview/1.0")

		// Execute request
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}
		defer resp.Body.Close()

		// Check for errors
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var response BitbucketAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Add repositories to result
		for _, repo := range response.Values {
			repositories = append(repositories, repo.FullName)
		}

		// Set next URL for pagination
		nextURL = response.Next
	}

	return repositories, nil
}

// getUserAccessibleRepositories fetches all repositories the user has access to
// This uses a more comprehensive approach that works without workspace enumeration
func getUserAccessibleRepositories(client *http.Client, apiBaseURL, email, apiToken string) ([]string, error) {
	var repositories []string
	nextURL := fmt.Sprintf("%s/repositories", apiBaseURL)

	// Add query parameters for pagination and filtering
	params := url.Values{}
	params.Add("pagelen", "100") // Maximum allowed by Bitbucket API
	params.Add("role", "member") // Only repositories where user is a member
	nextURL += "?" + params.Encode()

	for nextURL != "" {
		// Create request
		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add authentication and headers
		req.SetBasicAuth(email, apiToken)
		req.Header.Add("Accept", "application/json")
		req.Header.Add("User-Agent", "LiveReview/1.0")

		// Execute request
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}
		defer resp.Body.Close()

		// Check for errors
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		// Parse response
		var response BitbucketAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// Add repositories to result
		for _, repo := range response.Values {
			repositories = append(repositories, repo.FullName)
		}

		// Set next URL for pagination (Bitbucket uses URL-based pagination)
		nextURL = response.Next
	}

	return repositories, nil
}
