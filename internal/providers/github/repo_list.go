package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/prsync"
)

// gitHubRepositoryListItem mirrors the fields needed from GET /user/repos to
// populate a repositories row (id, url, default branch, private/description),
// which the older name-only GitHubRepositoryBasic (project_discovery.go) does
// not carry.
type gitHubRepositoryListItem struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	CloneURL      string `json:"clone_url"`
	SSHURL        string `json:"ssh_url"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
}

// ListRepositories fetches one page of repositories visible to the given PAT
// (GET /user/repos), following GitHub's Link-header pagination. Pass the
// previous call's NextCursor back in to fetch the next page; an empty cursor
// fetches the first page.
func ListRepositories(ctx context.Context, baseURL, pat, cursor string) (*providers.RepositoryPage, error) {
	requestURL := cursor
	if requestURL == "" {
		params := url.Values{}
		params.Add("page", "1")
		params.Add("per_page", "100")
		params.Add("affiliation", "owner,collaborator,organization_member")
		params.Add("sort", "updated")
		requestURL = fmt.Sprintf("%s/user/repos?%s", apiBaseURL(baseURL), params.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Authorization", "token "+pat)
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("User-Agent", "LiveReview/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if rlErr := checkGitHubRateLimit(resp); rlErr != nil {
		return nil, rlErr
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var items []gitHubRepositoryListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	page := &providers.RepositoryPage{
		NextCursor: nextPageFromLinkHeader(resp),
	}
	for _, item := range items {
		page.Repositories = append(page.Repositories, prsync.RepositorySummary{
			ProviderRepoID: strconv.FormatInt(item.ID, 10),
			FullName:       item.FullName,
			Name:           item.Name,
			WebURL:         item.HTMLURL,
			CloneURL:       item.CloneURL,
			SSHURL:         item.SSHURL,
			DefaultBranch:  item.DefaultBranch,
			IsPrivate:      item.Private,
			Description:    item.Description,
		})
	}

	return page, nil
}

// checkGitHubRateLimit returns a *providers.RateLimitedError if the response
// indicates the GitHub API rate limit has been exhausted, so callers can
// reschedule the work instead of treating it as a hard failure.
func checkGitHubRateLimit(resp *http.Response) error {
	if resp.Header.Get("X-RateLimit-Remaining") != "0" {
		return nil
	}
	retryAfter := 60 * time.Second
	if resetStr := resp.Header.Get("X-RateLimit-Reset"); resetStr != "" {
		if resetUnix, err := strconv.ParseInt(resetStr, 10, 64); err == nil {
			if d := time.Until(time.Unix(resetUnix, 0)); d > 0 {
				retryAfter = d
			}
		}
	}
	return &providers.RateLimitedError{Provider: "github", RetryAfter: retryAfter}
}
