package gitea

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Helpers for the PR review links section and the auto-approve policy result.
// baseURL is the connector's Gitea URL; rawToken is the stored (possibly packed) PAT.

// GetPRBody returns the PR description.
func GetPRBody(ctx context.Context, baseURL, rawToken, ownerRepo string, number int) (string, error) {
	var pr struct {
		Body string `json:"body"`
	}
	err := doGiteaJSON(ctx, http.MethodGet, baseURL, rawToken, fmt.Sprintf("/repos/%s/pulls/%d", ownerRepo, number), nil, &pr)
	return pr.Body, err
}

// SetPRBody replaces the PR description.
func SetPRBody(ctx context.Context, baseURL, rawToken, ownerRepo string, number int, body string) error {
	return doGiteaJSON(ctx, http.MethodPatch, baseURL, rawToken, fmt.Sprintf("/repos/%s/pulls/%d", ownerRepo, number), map[string]string{"body": body}, nil)
}

// PostIssueComment posts a plain comment on the PR conversation.
func PostIssueComment(ctx context.Context, baseURL, rawToken, ownerRepo string, number int, body string) error {
	return doGiteaJSON(ctx, http.MethodPost, baseURL, rawToken, fmt.Sprintf("/repos/%s/issues/%d/comments", ownerRepo, number), map[string]string{"body": body}, nil)
}

// SubmitPullRequestReview submits a review; event is "APPROVED" or "REQUEST_CHANGES".
func SubmitPullRequestReview(ctx context.Context, baseURL, rawToken, ownerRepo string, number int, event, body string) error {
	return doGiteaJSON(ctx, http.MethodPost, baseURL, rawToken, fmt.Sprintf("/repos/%s/pulls/%d/reviews", ownerRepo, number), map[string]string{"event": event, "body": body}, nil)
}

func doGiteaJSON(ctx context.Context, method, baseURL, rawToken, path string, payload, out interface{}) error {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, NormalizeGiteaBaseURL(baseURL)+"/api/v1"+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+UnpackGiteaPAT(rawToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("gitea %s %s failed with status %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
