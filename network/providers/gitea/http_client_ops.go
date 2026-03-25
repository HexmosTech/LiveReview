package gitea

import (
	"context"
	"fmt"
	"io"
	"net/http"
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
