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

// gitHubPullRequestListItem mirrors the fields needed from
// GET /repos/{owner}/{repo}/pulls to populate a pull_requests row.
type gitHubPullRequestListItem struct {
	ID     int64  `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	Merged bool   `json:"merged"`
	Draft  bool   `json:"draft"`
	User   struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func githubListStateParam(state providers.PullRequestListState) string {
	switch state {
	case providers.PRListStateOpen:
		return "open"
	case providers.PRListStateClosed:
		return "closed"
	default:
		return "all"
	}
}

// ListPullRequests fetches one page of pull requests for a repository
// (GET /repos/{owner}/{repo}/pulls?state=...&sort=updated&direction=desc),
// following GitHub's Link-header pagination the same way as ListRepositories.
// ownerRepo must be in "owner/repo" form.
func ListPullRequests(ctx context.Context, baseURL, pat, ownerRepo string, state providers.PullRequestListState, cursor string) (*providers.PullRequestPage, error) {
	requestURL := cursor
	if requestURL == "" {
		params := url.Values{}
		params.Add("state", githubListStateParam(state))
		params.Add("sort", "updated")
		params.Add("direction", "desc")
		params.Add("page", "1")
		params.Add("per_page", "100")
		requestURL = fmt.Sprintf("%s/repos/%s/pulls?%s", apiBaseURL(baseURL), ownerRepo, params.Encode())
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

	var items []gitHubPullRequestListItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	page := &providers.PullRequestPage{
		NextCursor: nextPageFromLinkHeader(resp),
	}
	for _, item := range items {
		createdAt, _ := time.Parse(time.RFC3339, item.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, item.UpdatedAt)
		page.PullRequests = append(page.PullRequests, prsync.PullRequestSummary{
			ProviderPRID:      strconv.FormatInt(item.ID, 10),
			Number:            item.Number,
			Title:             item.Title,
			Description:       item.Body,
			State:             prsync.NormalizeGitHubState(item.State, item.Merged),
			AuthorID:          strconv.FormatInt(item.User.ID, 10),
			AuthorUsername:    item.User.Login,
			AuthorName:        item.User.Login,
			AuthorAvatarURL:   item.User.AvatarURL,
			SourceBranch:      item.Head.Ref,
			TargetBranch:      item.Base.Ref,
			WebURL:            item.HTMLURL,
			ProviderCreatedAt: createdAt,
			ProviderUpdatedAt: updatedAt,
			Metadata: map[string]interface{}{
				"draft":  item.Draft,
				"merged": item.Merged,
			},
		})
	}

	return page, nil
}
