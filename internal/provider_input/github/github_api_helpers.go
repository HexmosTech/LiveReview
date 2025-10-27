package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/livereview/internal/capture"
)

// FetchGitHubPRCommitsV2 fetches commits for a GitHub PR.
func FetchGitHubPRCommitsV2(owner, repo, prNumber, token string) ([]GitHubV2CommitInfo, error) {
	baseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/commits", owner, repo, prNumber)
	client := &http.Client{Timeout: 30 * time.Second}

	commits := make([]GitHubV2CommitInfo, 0, 64)
	page := 1
	const perPage = 100

	for {
		apiURL := buildPaginatedURL(baseURL, page, perPage)
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		setGitHubHeaders(req, token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
		}

		var pageItems []GitHubV2CommitInfo
		if err = json.NewDecoder(resp.Body).Decode(&pageItems); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		commits = append(commits, pageItems...)

		if !hasNextPage(resp) {
			break
		}

		page++
	}

	if capture.Enabled() {
		payload := map[string]interface{}{
			"owner":   owner,
			"repo":    repo,
			"number":  prNumber,
			"commits": commits,
		}
		capture.WriteJSON("github-pr-commits", payload)
	}

	return commits, nil
}

// FetchGitHubPRCommentsV2 fetches comments for a GitHub PR.
func FetchGitHubPRCommentsV2(owner, repo, prNumber, token string) ([]GitHubV2CommentInfo, error) {
	baseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", owner, repo, prNumber)
	client := &http.Client{Timeout: 30 * time.Second}

	comments := make([]GitHubV2CommentInfo, 0, 32)
	page := 1
	const perPage = 100

	for {
		apiURL := buildPaginatedURL(baseURL, page, perPage)
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		setGitHubHeaders(req, token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
		}

		var pageItems []GitHubV2CommentInfo
		if err = json.NewDecoder(resp.Body).Decode(&pageItems); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		comments = append(comments, pageItems...)

		if !hasNextPage(resp) {
			break
		}

		page++
	}

	if capture.Enabled() {
		payload := map[string]interface{}{
			"owner":    owner,
			"repo":     repo,
			"number":   prNumber,
			"comments": comments,
		}
		capture.WriteJSON("github-pr-comments", payload)
	}

	return comments, nil
}

// FetchGitHubPRReviewCommentsV2 fetches review (inline) comments for a GitHub PR.
func FetchGitHubPRReviewCommentsV2(owner, repo, prNumber, token string) ([]GitHubV2ReviewComment, error) {
	baseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/comments", owner, repo, prNumber)
	client := &http.Client{Timeout: 30 * time.Second}

	comments := make([]GitHubV2ReviewComment, 0, 64)
	page := 1
	const perPage = 100

	for {
		apiURL := buildPaginatedURL(baseURL, page, perPage)
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		setGitHubHeaders(req, token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
		}

		var pageItems []GitHubV2ReviewComment
		if err = json.NewDecoder(resp.Body).Decode(&pageItems); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		comments = append(comments, pageItems...)

		if !hasNextPage(resp) {
			break
		}

		page++
	}

	if capture.Enabled() {
		payload := map[string]interface{}{
			"owner":    owner,
			"repo":     repo,
			"number":   prNumber,
			"comments": comments,
		}
		capture.WriteJSON("github-pr-review-comments", payload)
	}

	return comments, nil
}

// FetchGitHubPRReviewsV2 fetches review submissions for a GitHub PR.
func FetchGitHubPRReviewsV2(owner, repo, prNumber, token string) ([]GitHubV2ReviewInfo, error) {
	baseURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/reviews", owner, repo, prNumber)
	client := &http.Client{Timeout: 30 * time.Second}

	reviews := make([]GitHubV2ReviewInfo, 0, 16)
	page := 1
	const perPage = 100

	for {
		apiURL := buildPaginatedURL(baseURL, page, perPage)
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return nil, err
		}

		setGitHubHeaders(req, token)

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
		}

		var pageItems []GitHubV2ReviewInfo
		if err = json.NewDecoder(resp.Body).Decode(&pageItems); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		reviews = append(reviews, pageItems...)

		if !hasNextPage(resp) {
			break
		}

		page++
	}

	if capture.Enabled() {
		payload := map[string]interface{}{
			"owner":   owner,
			"repo":    repo,
			"number":  prNumber,
			"reviews": reviews,
		}
		capture.WriteJSON("github-pr-reviews", payload)
	}

	return reviews, nil
}

// FetchGitHubPRDiff fetches the diff for a GitHub PR.
func FetchGitHubPRDiff(owner, repo, prNumber, token string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s", owner, repo, prNumber)
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}

	setGitHubHeaders(req, token)
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API request for diff failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func setGitHubHeaders(req *http.Request, token string) {
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")
}

func buildPaginatedURL(base string, page, perPage int) string {
	separator := "?"
	if strings.Contains(base, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%vper_page=%d&page=%d", base, separator, perPage, page)
}

func hasNextPage(resp *http.Response) bool {
	linkHeader := resp.Header.Get("Link")
	return strings.Contains(linkHeader, "rel=\"next\"")
}
