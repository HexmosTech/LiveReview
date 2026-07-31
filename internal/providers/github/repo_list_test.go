package github

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/livereview/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// stubHTTPTransport swaps http.DefaultTransport for the duration of the test.
// The provider list functions in this package construct http.Client{Timeout: ...}
// with a nil Transport field, which falls back to http.DefaultTransport at
// request time, so this intercepts their calls.
func stubHTTPTransport(t *testing.T, responder func(*http.Request) (*http.Response, error)) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(responder)
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}

func jsonResponse(status int, body string, headers map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Header:     make(http.Header),
	}
	for k, v := range headers {
		resp.Header.Set(k, v)
	}
	return resp
}

func TestListRepositories_MultiPage(t *testing.T) {
	page1 := `[{"id":1,"name":"repo-a","full_name":"acme/repo-a","html_url":"https://github.com/acme/repo-a","clone_url":"https://github.com/acme/repo-a.git","ssh_url":"git@github.com:acme/repo-a.git","default_branch":"main","private":true,"description":"first"}]`
	page2 := `[{"id":2,"name":"repo-b","full_name":"acme/repo-b","html_url":"https://github.com/acme/repo-b","default_branch":"main","private":false,"description":"second"}]`

	callCount := 0
	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		callCount++
		if req.URL.Host != "api.github.com" || req.URL.Path != "/user/repos" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		if callCount == 1 {
			return jsonResponse(200, page1, map[string]string{
				"Link": `<https://api.github.com/user/repos?page=2>; rel="next"`,
			}), nil
		}
		if !strings.Contains(req.URL.String(), "page=2") {
			t.Fatalf("expected second call to hit page=2 cursor, got %s", req.URL.String())
		}
		return jsonResponse(200, page2, nil), nil
	})

	page, err := ListRepositories(context.Background(), "https://github.com", "pat", "")
	if err != nil {
		t.Fatalf("ListRepositories page 1: %v", err)
	}
	if len(page.Repositories) != 1 || page.Repositories[0].FullName != "acme/repo-a" {
		t.Fatalf("unexpected page 1 result: %+v", page.Repositories)
	}
	if page.NextCursor == "" {
		t.Fatalf("expected a next cursor after page 1")
	}
	if !page.Repositories[0].IsPrivate {
		t.Errorf("expected repo-a to be private")
	}

	page2Result, err := ListRepositories(context.Background(), "https://github.com", "pat", page.NextCursor)
	if err != nil {
		t.Fatalf("ListRepositories page 2: %v", err)
	}
	if len(page2Result.Repositories) != 1 || page2Result.Repositories[0].FullName != "acme/repo-b" {
		t.Fatalf("unexpected page 2 result: %+v", page2Result.Repositories)
	}
	if page2Result.NextCursor != "" {
		t.Fatalf("expected no next cursor after final page, got %q", page2Result.NextCursor)
	}
	if page2Result.Repositories[0].IsPrivate {
		t.Errorf("expected repo-b to be public")
	}
	if callCount != 2 {
		t.Fatalf("expected 2 requests, got %d", callCount)
	}
}

func TestListRepositories_GitHubEnterpriseBaseURL(t *testing.T) {
	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "ghe.example.com" || req.URL.Path != "/api/v3/user/repos" {
			t.Fatalf("expected GHE API path, got %s", req.URL.String())
		}
		return jsonResponse(200, `[]`, nil), nil
	})

	if _, err := ListRepositories(context.Background(), "https://ghe.example.com", "pat", ""); err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
}

func TestListRepositories_RateLimited(t *testing.T) {
	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(403, `{"message":"API rate limit exceeded"}`, map[string]string{
			"X-RateLimit-Remaining": "0",
			"X-RateLimit-Reset":     "9999999999",
		}), nil
	})

	_, err := ListRepositories(context.Background(), "https://github.com", "pat", "")
	if err == nil {
		t.Fatal("expected an error")
	}
	var rlErr *providers.RateLimitedError
	if !errors.As(err, &rlErr) {
		t.Fatalf("expected a *providers.RateLimitedError, got: %v", err)
	}
	if rlErr.RetryAfter <= 0 {
		t.Errorf("expected a positive RetryAfter, got %v", rlErr.RetryAfter)
	}
}
