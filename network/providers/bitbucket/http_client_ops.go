package bitbucket

import (
	"bytes"
	"context"
	"encoding/base64"
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

func NewRequestWithContext(ctx context.Context, method, requestURL string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return req, nil
}

// Do executes the request. Callers must close resp.Body when err is nil.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	if req == nil {
		return nil, fmt.Errorf("http request is nil")
	}
	return client.Do(req)
}

// PostCommentAPI handles the exact HTTP execution and authorization for posting Bitbucket comments.
func PostCommentAPI(ctx context.Context, client *http.Client, apiURL, email, token string, payload []byte) (*http.Response, error) {
	importBytes := bytes.NewReader(payload)
	req, err := NewRequestWithContext(ctx, "POST", apiURL, importBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Bitbucket Basic Auth encoding
	authBytes := []byte(email + ":" + token)
	authStr := "Basic " + base64.StdEncoding.EncodeToString(authBytes)
	req.Header.Set("Authorization", authStr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return Do(client, req)
}
