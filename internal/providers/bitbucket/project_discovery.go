package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	networkbitbucket "github.com/livereview/network/providers/bitbucket"
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
	// The new /user/workspaces API might return a nested workspace object depending on token type
	Workspace *struct {
		Slug string `json:"slug"`
		Name string `json:"name"`
	} `json:"workspace"`
}

// BitbucketAPIResponse represents the paginated response for repositories
type BitbucketAPIResponse struct {
	Values []BitbucketRepositoryBasic `json:"values"`
	Next   string                     `json:"next"`
}

// BitbucketWorkspaceAPIResponse represents the paginated response for workspaces
type BitbucketWorkspaceAPIResponse struct {
	Values []BitbucketWorkspaceBasic `json:"values"`
	Next   string                    `json:"next"`
}

// DiscoverProjectsBitbucket fetches all repositories accessible with the given credentials.
//
// Migration from deprecated APIs (CHANGE-2770):
//   - Removed: GET /2.0/repositories         → replaced by /user/workspaces + /repositories/{workspace}
//   - Removed: GET /2.0/workspaces            → replaced by GET /2.0/user/workspaces
//   - Removed: GET /2.0/user/permissions/workspaces → replaced by GET /2.0/user/workspaces
func DiscoverProjectsBitbucket(baseURL, email, apiToken string) ([]string, error) {
	client := &http.Client{}

	apiBaseURL := "https://api.bitbucket.org/2.0"
	if baseURL != "" && baseURL != "https://bitbucket.org" {
		return nil, fmt.Errorf("only Bitbucket Cloud is supported (not Bitbucket Server)")
	}

	// Step 1: list all accessible workspaces using the new /user/workspaces endpoint.
	workspaces, err := getUserWorkspaces(client, apiBaseURL, email, apiToken)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces: %w", err)
	}

	// Step 2: for each workspace, list repositories using /repositories/{workspace}.
	seen := make(map[string]struct{})
	var all []string
	for _, ws := range workspaces {
		slug := ws.Slug
		if slug == "" && ws.Workspace != nil {
			slug = ws.Workspace.Slug
		}

		fmt.Printf("[DEBUG] Extracted workspace slug: %q from object: %+v\n", slug, ws)
		if slug == "" {
			continue // Skip if we couldn't parse the slug
		}

		repos, err := getWorkspaceRepositories(client, apiBaseURL, email, apiToken, slug)
		if err != nil {
			fmt.Printf("Warning: failed to get repositories for workspace %s: %v\n", slug, err)
			continue
		}
		for _, r := range repos {
			if _, ok := seen[r]; !ok {
				seen[r] = struct{}{}
				all = append(all, r)
			}
		}
	}
	return all, nil
}

// DiscoverProjectsBitbucketForWorkspaces lists repositories for a set of known workspace slugs,
// bypassing workspace enumeration entirely. Useful as a fallback when the workspace slug is
// already known (e.g. derived from a past review URL) but enumeration fails.
func DiscoverProjectsBitbucketForWorkspaces(baseURL, email, apiToken string, workspaces []string) ([]string, error) {
	client := &http.Client{}
	apiBaseURL := "https://api.bitbucket.org/2.0"
	if baseURL != "" && baseURL != "https://bitbucket.org" {
		return nil, fmt.Errorf("only Bitbucket Cloud is supported (not Bitbucket Server)")
	}

	seen := make(map[string]struct{})
	var all []string
	for _, ws := range workspaces {
		repos, err := getWorkspaceRepositories(client, apiBaseURL, email, apiToken, ws)
		if err != nil {
			fmt.Printf("Warning: failed to get repositories for workspace %s: %v\n", ws, err)
			continue
		}
		for _, r := range repos {
			if _, ok := seen[r]; !ok {
				seen[r] = struct{}{}
				all = append(all, r)
			}
		}
	}
	return all, nil
}

// getUserWorkspaces calls GET /2.0/user/workspaces — the new public API announced alongside
// CHANGE-2770, replacing both the deprecated GET /2.0/workspaces and
// GET /2.0/user/permissions/workspaces endpoints.
func getUserWorkspaces(client *http.Client, apiBaseURL, email, apiToken string) ([]BitbucketWorkspaceBasic, error) {
	var all []BitbucketWorkspaceBasic
	nextURL := fmt.Sprintf("%s/user/workspaces", apiBaseURL)
	ctx := context.Background()

	for nextURL != "" {
		resp, err := networkbitbucket.FetchUserWorkspacesPage(ctx, client, nextURL, email, apiToken)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("GET /user/workspaces failed (status %d): %s", resp.StatusCode, string(body))
		}

		var response BitbucketWorkspaceAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode workspace response: %w", err)
		}
		resp.Body.Close()

		all = append(all, response.Values...)
		nextURL = response.Next
	}

	return all, nil
}

// getWorkspaceRepositories fetches all repositories from a specific workspace using
// GET /2.0/repositories/{workspace} — the current non-deprecated, workspace-scoped endpoint.
func getWorkspaceRepositories(client *http.Client, apiBaseURL, email, apiToken, workspace string) ([]string, error) {
	var repositories []string

	params := url.Values{}
	params.Set("pagelen", "100")
	params.Set("role", "member")
	nextURL := fmt.Sprintf("%s/repositories/%s?%s", apiBaseURL, url.PathEscape(workspace), params.Encode())
	ctx := context.Background()

	for nextURL != "" {
		resp, err := networkbitbucket.FetchWorkspaceRepositoriesPage(ctx, client, nextURL, email, apiToken)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("GET /repositories/%s failed (status %d): %s", workspace, resp.StatusCode, string(body))
		}

		var response BitbucketAPIResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode repository response: %w", err)
		}
		resp.Body.Close()

		for _, repo := range response.Values {
			repositories = append(repositories, repo.FullName)
		}
		nextURL = response.Next
	}

	return repositories, nil
}
