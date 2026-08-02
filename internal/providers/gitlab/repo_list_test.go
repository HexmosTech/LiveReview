package gitlab

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/livereview/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// stubHTTPTransport swaps http.DefaultTransport for the duration of the test.
// GitLabHTTPClient (via network/providers/gitlab.NewHTTPClient) and the new
// standalone list functions both construct http.Client{Timeout: ...} with a
// nil Transport field, which falls back to http.DefaultTransport.
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
	page1 := `[{"id":1,"name":"repo-a","path_with_namespace":"group/repo-a","web_url":"https://gitlab.com/group/repo-a","http_url_to_repo":"https://gitlab.com/group/repo-a.git","default_branch":"main","visibility":"private","description":"first"}]`
	page2 := `[{"id":2,"name":"repo-b","path_with_namespace":"group/repo-b","web_url":"https://gitlab.com/group/repo-b","default_branch":"main","visibility":"public","description":"second"}]`

	callCount := 0
	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		callCount++
		if req.URL.Path != "/api/v4/projects" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if callCount == 1 {
			if got := req.URL.Query().Get("page"); got != "1" {
				t.Fatalf("expected page=1 on first call, got %q", got)
			}
			return jsonResponse(200, page1, map[string]string{"X-Next-Page": "2"}), nil
		}
		if got := req.URL.Query().Get("page"); got != "2" {
			t.Fatalf("expected page=2 on second call, got %q", got)
		}
		return jsonResponse(200, page2, map[string]string{"X-Next-Page": ""}), nil
	})

	page, err := ListRepositories(context.Background(), "https://gitlab.com", "pat", "")
	if err != nil {
		t.Fatalf("ListRepositories page 1: %v", err)
	}
	if len(page.Repositories) != 1 || page.Repositories[0].FullName != "group/repo-a" {
		t.Fatalf("unexpected page 1 result: %+v", page.Repositories)
	}
	if !page.Repositories[0].IsPrivate {
		t.Errorf("expected repo-a to be private")
	}
	if page.NextCursor != "2" {
		t.Fatalf("expected next cursor '2', got %q", page.NextCursor)
	}

	page2Result, err := ListRepositories(context.Background(), "https://gitlab.com", "pat", page.NextCursor)
	if err != nil {
		t.Fatalf("ListRepositories page 2: %v", err)
	}
	if len(page2Result.Repositories) != 1 || page2Result.Repositories[0].FullName != "group/repo-b" {
		t.Fatalf("unexpected page 2 result: %+v", page2Result.Repositories)
	}
	if page2Result.Repositories[0].IsPrivate {
		t.Errorf("expected repo-b to be public")
	}
	if page2Result.NextCursor != "" {
		t.Fatalf("expected no next cursor after final page, got %q", page2Result.NextCursor)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 requests, got %d", callCount)
	}
}

func TestListRepositories_RateLimited(t *testing.T) {
	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		return jsonResponse(429, `{"message":"429 Too Many Requests"}`, map[string]string{
			"Retry-After": "30",
		}), nil
	})

	_, err := ListRepositories(context.Background(), "https://gitlab.com", "pat", "")
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
