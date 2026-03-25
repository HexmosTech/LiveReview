package gitlab

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const defaultHTTPTimeout = 60 * time.Second

func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		return &http.Client{Timeout: defaultHTTPTimeout}
	}
	return &http.Client{Timeout: timeout}
}

func NewRequest(method, requestURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request %s %s: %w", method, requestURL, err)
	}
	return req, nil
}

func NewRequestWithContext(ctx context.Context, method, requestURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request %s %s with context: %w", method, requestURL, err)
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

func ParseURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL %q: %w", rawURL, err)
	}
	if !parsed.IsAbs() {
		return nil, fmt.Errorf("URL must be absolute: %q", rawURL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("URL scheme must be http or https: %q", rawURL)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("URL must include host: %q", rawURL)
	}
	return parsed, nil
}
