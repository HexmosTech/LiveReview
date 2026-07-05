package azuredevops

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"

	networkazuredevops "github.com/livereview/network/providers/azuredevops"
)

// DiscoverProjectsAzureDevOps enumerates projects and their repositories
// accessible with the given PAT, returning "{project}/{repo}" strings.
func DiscoverProjectsAzureDevOps(orgURL, pat string) ([]string, error) {
	if pt := decodePackedToken(pat); pt.pat != "" {
		pat = pt.pat
	}
	apiBase := NormalizeOrgURL(orgURL)
	if apiBase == "" {
		return nil, fmt.Errorf("organization URL is required for Azure DevOps")
	}

	client := &http.Client{}

	projects, err := listProjects(client, apiBase, pat)
	if err != nil {
		return nil, err
	}

	var result []string
	for _, project := range projects {
		repos, err := listRepositories(client, apiBase, pat, project.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories for project %s: %w", project.Name, err)
		}
		for _, repo := range repos {
			result = append(result, fmt.Sprintf("%s/%s", project.Name, repo.Name))
		}
	}

	return result, nil
}

func listProjects(client *http.Client, apiBase, pat string) ([]projectSummary, error) {
	var all []projectSummary
	continuationToken := ""

	for {
		apiURL := fmt.Sprintf("%s/_apis/projects?api-version=%s&$top=100", apiBase, apiVersion)
		if continuationToken != "" {
			apiURL += "&continuationToken=" + neturl.QueryEscape(continuationToken)
		}

		req, err := http.NewRequest(http.MethodGet, apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		networkazuredevops.ApplyPATAuth(req, pat)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("azure devops projects request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var out projectsResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&out)
		nextToken := resp.Header.Get("x-ms-continuationtoken")
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("failed to decode response: %w", decodeErr)
		}

		all = append(all, out.Value...)

		if nextToken == "" {
			break
		}
		continuationToken = nextToken
	}

	return all, nil
}

func listRepositories(client *http.Client, apiBase, pat, project string) ([]repositorySummary, error) {
	apiURL := fmt.Sprintf("%s/%s/_apis/git/repositories?api-version=%s", apiBase, neturl.PathEscape(project), apiVersion)

	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	networkazuredevops.ApplyPATAuth(req, pat)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("azure devops repositories request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var out repositoriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return out.Value, nil
}
