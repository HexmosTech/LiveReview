package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/prsync"
)

// gitLabProjectListItem mirrors the fields needed from GET /api/v4/projects to
// populate a repositories row, richer than the name-only GitLabProjectBasic
// used by DiscoverProjectsGitlab (project_discovery.go).
type gitLabProjectListItem struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	WebURL            string `json:"web_url"`
	HTTPURLToRepo     string `json:"http_url_to_repo"`
	SSHURLToRepo      string `json:"ssh_url_to_repo"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
	Description       string `json:"description"`
}

// ListRepositories fetches one page of projects visible to the given PAT
// (GET /api/v4/projects?membership=true), following GitLab's X-Next-Page
// response header for pagination. Pass the previous call's NextCursor back in
// (a page number) to fetch the next page; an empty cursor fetches page 1.
func ListRepositories(ctx context.Context, baseURL, pat, cursor string) (*providers.RepositoryPage, error) {
	page := 1
	if cursor != "" {
		if p, err := strconv.Atoi(cursor); err == nil {
			page = p
		}
	}

	params := url.Values{}
	params.Add("membership", "true")
	params.Add("page", strconv.Itoa(page))
	params.Add("per_page", "100")
	requestURL := fmt.Sprintf("%s/api/v4/projects?%s", strings.TrimSuffix(baseURL, "/"), params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("PRIVATE-TOKEN", pat)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if rlErr := checkGitLabRateLimit(resp); rlErr != nil {
		return nil, rlErr
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitLab API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var items []gitLabProjectListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := &providers.RepositoryPage{
		NextCursor: resp.Header.Get("X-Next-Page"),
	}
	for _, item := range items {
		result.Repositories = append(result.Repositories, prsync.RepositorySummary{
			ProviderRepoID: strconv.Itoa(item.ID),
			FullName:       item.PathWithNamespace,
			Name:           item.Name,
			WebURL:         item.WebURL,
			CloneURL:       item.HTTPURLToRepo,
			SSHURL:         item.SSHURLToRepo,
			DefaultBranch:  item.DefaultBranch,
			IsPrivate:      item.Visibility != "public",
			Description:    item.Description,
		})
	}

	return result, nil
}

// checkGitLabRateLimit returns a *providers.RateLimitedError if the response
// indicates the GitLab API rate limit has been exhausted.
func checkGitLabRateLimit(resp *http.Response) error {
	if resp.StatusCode != http.StatusTooManyRequests && resp.Header.Get("RateLimit-Remaining") != "0" {
		return nil
	}
	retryAfter := 60 * time.Second
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil && secs > 0 {
			retryAfter = time.Duration(secs) * time.Second
		}
	}
	return &providers.RateLimitedError{Provider: "gitlab", RetryAfter: retryAfter}
}
