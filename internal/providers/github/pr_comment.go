package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PostIssueComment posts a plain top-level comment on a PR (POST /repos/{owner}/{repo}/issues/{number}/comments).
// Unlike GitHubProvider.PostComment it honors GitHub Enterprise base URLs and posts body as-is.
func PostIssueComment(ctx context.Context, baseURL, pat, ownerRepo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/issues/%d/comments", ownerRepo, number)
	return postGitHubJSON(ctx, baseURL, pat, path, map[string]string{"body": body})
}

// SubmitPullRequestReview submits a PR review with event "APPROVE" or "REQUEST_CHANGES" and body as its text.
// GitHub rejects both events when the token's user is the PR author.
func SubmitPullRequestReview(ctx context.Context, baseURL, pat, ownerRepo string, number int, event, body string) error {
	path := fmt.Sprintf("/repos/%s/pulls/%d/reviews", ownerRepo, number)
	return postGitHubJSON(ctx, baseURL, pat, path, map[string]string{"event": event, "body": body})
}

// GetPRBody returns the PR description.
func GetPRBody(ctx context.Context, baseURL, pat, ownerRepo string, number int) (string, error) {
	var pr struct {
		Body string `json:"body"`
	}
	err := doGitHubJSON(ctx, http.MethodGet, baseURL, pat, fmt.Sprintf("/repos/%s/pulls/%d", ownerRepo, number), nil, &pr)
	return pr.Body, err
}

// SetPRBody replaces the PR description.
func SetPRBody(ctx context.Context, baseURL, pat, ownerRepo string, number int, body string) error {
	path := fmt.Sprintf("/repos/%s/pulls/%d", ownerRepo, number)
	return doGitHubJSON(ctx, http.MethodPatch, baseURL, pat, path, map[string]string{"body": body}, nil)
}

func postGitHubJSON(ctx context.Context, baseURL, pat, path string, payload interface{}) error {
	return doGitHubJSON(ctx, http.MethodPost, baseURL, pat, path, payload, nil)
}

// doGitHubJSON sends payload (if any) and decodes the response into out (if non-nil).
func doGitHubJSON(ctx context.Context, method, baseURL, pat, path string, payload, out interface{}) error {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, apiBaseURL(baseURL)+path, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Add("Authorization", "token "+pat)
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", "LiveReview/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GitHub request %s failed with status %d: %s", path, resp.StatusCode, string(respBody))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
