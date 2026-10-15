package bitbucket

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
// Bitbucket Cloud API tokens use basic auth with the account email.

const bitbucketAPIBase = "https://api.bitbucket.org/2.0"

type bitbucketPR struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GetPRBody returns the PR description.
func GetPRBody(ctx context.Context, email, token, fullName string, number int) (string, error) {
	var pr bitbucketPR
	err := doBitbucketJSON(ctx, http.MethodGet, email, token, fmt.Sprintf("/repositories/%s/pullrequests/%d", fullName, number), nil, &pr)
	return pr.Description, err
}

// SetPRBody replaces the PR description. Bitbucket's update needs the title too, so it is re-read first.
func SetPRBody(ctx context.Context, email, token, fullName string, number int, description string) error {
	path := fmt.Sprintf("/repositories/%s/pullrequests/%d", fullName, number)
	var pr bitbucketPR
	if err := doBitbucketJSON(ctx, http.MethodGet, email, token, path, nil, &pr); err != nil {
		return err
	}
	return doBitbucketJSON(ctx, http.MethodPut, email, token, path, bitbucketPR{Title: pr.Title, Description: description}, nil)
}

// PostPRComment posts a general comment on the PR.
func PostPRComment(ctx context.Context, email, token, fullName string, number int, body string) error {
	payload := map[string]interface{}{"content": map[string]string{"raw": body}}
	return doBitbucketJSON(ctx, http.MethodPost, email, token, fmt.Sprintf("/repositories/%s/pullrequests/%d/comments", fullName, number), payload, nil)
}

// SetPRVerdict approves the PR, or marks it "changes requested" when approve is false.
func SetPRVerdict(ctx context.Context, email, token, fullName string, number int, approve bool) error {
	action := "request-changes"
	if approve {
		action = "approve"
	}
	return doBitbucketJSON(ctx, http.MethodPost, email, token, fmt.Sprintf("/repositories/%s/pullrequests/%d/%s", fullName, number, action), nil, nil)
}

func doBitbucketJSON(ctx context.Context, method, email, token, path string, payload, out interface{}) error {
	var reqBody io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, bitbucketAPIBase+path, reqBody)
	if err != nil {
		return err
	}
	req.SetBasicAuth(email, token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "LiveReview-Bot")

	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bitbucket %s %s failed with status %d: %s", method, path, resp.StatusCode, string(data))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
