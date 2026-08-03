package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/livereview/internal/providers"
)

func TestListPullRequests_StateMapping(t *testing.T) {
	body := `[
		{"id":100,"number":1,"title":"open pr","body":"","state":"open","merged":false,"draft":false,
		 "user":{"id":1,"login":"alice"},"head":{"ref":"feature"},"base":{"ref":"main"},
		 "html_url":"https://github.com/acme/repo/pull/1",
		 "created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"},
		{"id":101,"number":2,"title":"merged pr","body":"","state":"closed","merged":true,"draft":false,
		 "user":{"id":2,"login":"bob"},"head":{"ref":"feature2"},"base":{"ref":"main"},
		 "html_url":"https://github.com/acme/repo/pull/2",
		 "created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-03T00:00:00Z"},
		{"id":102,"number":3,"title":"closed pr","body":"","state":"closed","merged":false,"draft":false,
		 "user":{"id":3,"login":"carol"},"head":{"ref":"feature3"},"base":{"ref":"main"},
		 "html_url":"https://github.com/acme/repo/pull/3",
		 "created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-04T00:00:00Z"}
	]`

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/repos/acme/repo/pulls" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if got := req.URL.Query().Get("state"); got != "all" {
			t.Fatalf("expected state=all, got %q", got)
		}
		return jsonResponse(200, body, nil), nil
	})

	page, err := ListPullRequests(context.Background(), "https://github.com", "pat", "acme/repo", providers.PRListStateAll, "")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(page.PullRequests) != 3 {
		t.Fatalf("expected 3 pull requests, got %d", len(page.PullRequests))
	}
	if page.PullRequests[0].State != "open" {
		t.Errorf("pr1: expected state open, got %s", page.PullRequests[0].State)
	}
	if page.PullRequests[1].State != "merged" {
		t.Errorf("pr2: expected state merged (merged=true overrides state=closed), got %s", page.PullRequests[1].State)
	}
	if page.PullRequests[2].State != "closed" {
		t.Errorf("pr3: expected state closed, got %s", page.PullRequests[2].State)
	}
	if page.PullRequests[0].AuthorUsername != "alice" {
		t.Errorf("pr1: expected author alice, got %s", page.PullRequests[0].AuthorUsername)
	}
	if page.PullRequests[0].SourceBranch != "feature" || page.PullRequests[0].TargetBranch != "main" {
		t.Errorf("pr1: unexpected branches: %+v", page.PullRequests[0])
	}
}
