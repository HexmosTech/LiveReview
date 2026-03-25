package jobqueue

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// WebhookHTTPClient centralizes outbound HTTP request execution for jobqueue workers.
type WebhookHTTPClient struct {
	client *http.Client
}

func NewWebhookHTTPClient(timeout time.Duration) *WebhookHTTPClient {
	return &WebhookHTTPClient{client: &http.Client{Timeout: timeout}}
}

func (c *WebhookHTTPClient) ensureClient() error {
	if c == nil {
		return fmt.Errorf("webhook HTTP client is nil")
	}
	if c.client == nil {
		return fmt.Errorf("underlying HTTP client is nil")
	}
	return nil
}

func (c *WebhookHTTPClient) NewRequest(method, url string, body io.Reader) (*http.Request, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return http.NewRequest(method, url, body)
}

func (c *WebhookHTTPClient) NewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*http.Request, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, url, body)
}

// Do returns the response and leaves resp.Body ownership to the caller.
func (c *WebhookHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if err := c.ensureClient(); err != nil {
		return nil, err
	}
	return c.client.Do(req)
}
