package azuredevops

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"time"
)

// NewHTTPClient creates an http.Client with the given timeout.
func NewHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		return &http.Client{}
	}
	return &http.Client{Timeout: timeout}
}

// NewRequestWithContext builds an *http.Request bound to ctx.
func NewRequestWithContext(ctx context.Context, method, requestURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return req, nil
}

// Do executes req using client.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	if req == nil {
		return nil, fmt.Errorf("http request is nil")
	}
	return client.Do(req)
}

// BasicAuthHeader builds the Azure DevOps PAT auth header value: Basic base64(":"+pat).
func BasicAuthHeader(pat string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(":" + pat))
	return "Basic " + encoded
}

// ApplyPATAuth sets the Authorization and Accept headers for an Azure DevOps PAT request.
func ApplyPATAuth(req *http.Request, pat string) {
	req.Header.Set("Authorization", BasicAuthHeader(pat))
	req.Header.Set("Accept", "application/json")
}
