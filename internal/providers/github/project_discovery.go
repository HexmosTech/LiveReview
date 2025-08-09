package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// GitHubRepositoryBasic represents basic repository information from GitHub API
type GitHubRepositoryBasic struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private bool `json:"private"`
}

// DiscoverProjectsGitHub fetches all repositories accessible with the given PAT from GitHub
func DiscoverProjectsGitHub(baseURL, pat string) ([]string, error) {
	var allRepositories []string
	page := 1
	perPage := 100 // Maximum allowed by GitHub API

	// Create HTTP client
	client := &http.Client{}

	// GitHub API base URL - use provided baseURL if it's a GitHub Enterprise instance
	apiBaseURL := "https://api.github.com"
	if baseURL != "" && baseURL != "https://github.com" {
		// For GitHub Enterprise, the API is typically at /api/v3
		apiBaseURL = strings.TrimSuffix(baseURL, "/") + "/api/v3"
	}

	for {
		// Build API URL for user repositories
		apiURL := fmt.Sprintf("%s/user/repos", apiBaseURL)

		// Add query parameters
		params := url.Values{}
		params.Add("page", strconv.Itoa(page))
		params.Add("per_page", strconv.Itoa(perPage))
		params.Add("affiliation", "owner,collaborator,organization_member") // Repositories user has access to
		params.Add("sort", "updated")                                       // Sort by last updated
		apiURL += "?" + params.Encode()

		// Create request
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add authentication header
		req.Header.Add("Authorization", "token "+pat)
		req.Header.Add("Accept", "application/vnd.github.v3+json")
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
		var repositories []GitHubRepositoryBasic
		if err := json.NewDecoder(resp.Body).Decode(&repositories); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// If no repositories returned, we've reached the end
		if len(repositories) == 0 {
			break
		}

		// Add repositories to result
		for _, repo := range repositories {
			allRepositories = append(allRepositories, repo.FullName)
		}

		// If we got fewer repositories than requested, we've reached the end
		if len(repositories) < perPage {
			break
		}

		page++
	}

	return allRepositories, nil
}
