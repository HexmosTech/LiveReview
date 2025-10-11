package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/livereview/internal/capture"
)

// FetchGitHubPRCommitsV2 fetches commits for a GitHub PR.
func FetchGitHubPRCommitsV2(owner, repo, prNumber, token string) ([]GitHubV2CommitInfo, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%s/commits", owner, repo, prNumber)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
	}

	var commits []GitHubV2CommitInfo
	if err = json.NewDecoder(resp.Body).Decode(&commits); err != nil {
		return nil, err
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
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%s/comments", owner, repo, prNumber)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API request failed with status %d", resp.StatusCode)
	}

	var comments []GitHubV2CommentInfo
	if err = json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, err
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
