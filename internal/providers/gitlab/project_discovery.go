package gitlab

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// GitLabProjectBasic represents basic project information from GitLab API
type GitLabProjectBasic struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
}

// DiscoverProjectsGitlab fetches all projects accessible with the given PAT from GitLab
func DiscoverProjectsGitlab(baseURL, pat string) ([]string, error) {
	var allProjects []string
	page := 1
	perPage := 100 // Maximum allowed by GitLab API

	// Create HTTP client
	client := &http.Client{}

	for {
		// Build API URL
		apiURL := fmt.Sprintf("%s/api/v4/projects", strings.TrimSuffix(baseURL, "/"))

		// Add query parameters
		params := url.Values{}
		params.Add("page", strconv.Itoa(page))
		params.Add("per_page", strconv.Itoa(perPage))
		params.Add("membership", "true") // Only projects user is a member of
		apiURL += "?" + params.Encode()

		// Create request
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Add authentication header
		req.Header.Add("PRIVATE-TOKEN", pat)

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
		var projects []GitLabProjectBasic
		if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		// If no projects returned, we've reached the end
		if len(projects) == 0 {
			break
		}

		// Add projects to result
		for _, project := range projects {
			allProjects = append(allProjects, project.PathWithNamespace)
		}

		// If we got fewer projects than requested, we've reached the end
		if len(projects) < perPage {
			break
		}

		page++
	}

	return allProjects, nil
}
