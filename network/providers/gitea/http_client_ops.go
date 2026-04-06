package gitea

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		return &http.Client{}
	}
	return &http.Client{Timeout: timeout}
}

func NewHTTPClientWithJar(timeout time.Duration, jar http.CookieJar) *http.Client {
	if timeout <= 0 {
		return &http.Client{Jar: jar}
	}
	return &http.Client{Timeout: timeout, Jar: jar}
}

func NewRequest(method, requestURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return req, nil
}

func NewRequestWithContext(ctx context.Context, method, requestURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return req, nil
}

func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	if req == nil {
		return nil, fmt.Errorf("http request is nil")
	}
	return client.Do(req)
}

func FetchPatchContent(ctx context.Context, client *http.Client, patchURL string, token string) (string, error) {
	patchURL = strings.TrimSpace(patchURL)
	if patchURL == "" {
		return "", fmt.Errorf("patch URL is empty")
	}

	req, err := NewRequestWithContext(ctx, http.MethodGet, patchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/plain")
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s", strings.TrimSpace(token)))
	}

	resp, err := Do(client, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read patch response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("patch request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
