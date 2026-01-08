package gitea

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// GiteaRepositoryBasic represents basic repository information from Gitea API.
type GiteaRepositoryBasic struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Private bool `json:"private"`
}

// DiscoverProjectsGitea fetches repositories accessible with the given PAT from a Gitea instance.
// It iterates over /api/v1/user/repos with pagination and returns repo full names.
func DiscoverProjectsGitea(baseURL, pat string) ([]string, error) {
	var allRepos []string
	page := 1
	perPage := 50 // Gitea default max is typically 50

	// Unpack PAT if it's stored in packed format (JSON with pat/username/password)
	pat = UnpackGiteaPAT(pat)

	client := &http.Client{}
	apiBase := NormalizeGiteaBaseURL(baseURL)
	if apiBase == "" {
		return nil, fmt.Errorf("base_url is required for Gitea")
	}

	for {
		apiURL := fmt.Sprintf("%s/api/v1/user/repos", apiBase)
		params := url.Values{}
		params.Add("page", strconv.Itoa(page))
		params.Add("limit", strconv.Itoa(perPage))
		apiURL += "?" + params.Encode()

		fmt.Printf("[DEBUG] Gitea discovery - baseURL: %s, apiBase after normalize: %s, final apiURL: %s\n", baseURL, apiBase, apiURL)

		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("token %s", pat))
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
		}

		var repos []GiteaRepositoryBasic
		if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		if len(repos) == 0 {
			break
		}

		for _, repo := range repos {
			if repo.FullName != "" {
				allRepos = append(allRepos, repo.FullName)
			} else if repo.Owner.Login != "" {
				allRepos = append(allRepos, fmt.Sprintf("%s/%s", repo.Owner.Login, repo.Name))
			} else {
				allRepos = append(allRepos, repo.Name)
			}
		}

		if len(repos) < perPage {
			break
		}

		page++
	}

	return allRepos, nil
}
