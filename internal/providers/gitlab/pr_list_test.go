package gitlab

import (
	"context"
	"net/http"
	"testing"

	"github.com/livereview/internal/providers"
)

func TestListPullRequests_StateMapping(t *testing.T) {
	body := `[
		{"id":10,"iid":1,"project_id":5,"title":"open mr","state":"opened","source_branch":"feature","target_branch":"main",
		 "web_url":"https://gitlab.com/group/repo/-/merge_requests/1",
		 "created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z",
		 "author":{"id":1,"username":"alice","name":"Alice"}},
		{"id":11,"iid":2,"project_id":5,"title":"merged mr","state":"merged","source_branch":"feature2","target_branch":"main",
		 "web_url":"https://gitlab.com/group/repo/-/merge_requests/2",
		 "created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-03T00:00:00Z",
		 "author":{"id":2,"username":"bob","name":"Bob"}},
		{"id":12,"iid":3,"project_id":5,"title":"closed mr","state":"closed","source_branch":"feature3","target_branch":"main",
		 "web_url":"https://gitlab.com/group/repo/-/merge_requests/3",
		 "created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-04T00:00:00Z",
		 "author":{"id":3,"username":"carol","name":"Carol"}}
	]`

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v4/projects/5/merge_requests" {
			t.Fatalf("unexpected path: %s", req.URL.Path)
		}
		if got := req.URL.Query().Get("state"); got != "all" {
			t.Fatalf("expected state=all, got %q", got)
		}
		return jsonResponse(200, body, map[string]string{"X-Next-Page": ""}), nil
	})

	page, err := ListPullRequests(context.Background(), "https://gitlab.com", "pat", "5", providers.PRListStateAll, "")
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if len(page.PullRequests) != 3 {
		t.Fatalf("expected 3 merge requests, got %d", len(page.PullRequests))
	}
	if page.PullRequests[0].State != "open" {
		t.Errorf("mr1: expected state open, got %s", page.PullRequests[0].State)
	}
	if page.PullRequests[1].State != "merged" {
		t.Errorf("mr2: expected state merged, got %s", page.PullRequests[1].State)
	}
	if page.PullRequests[2].State != "closed" {
		t.Errorf("mr3: expected state closed, got %s", page.PullRequests[2].State)
	}
	if page.PullRequests[0].Number != 1 {
		t.Errorf("mr1: expected Number (iid) 1, got %d", page.PullRequests[0].Number)
	}
	if page.PullRequests[0].AuthorUsername != "alice" {
		t.Errorf("mr1: expected author alice, got %s", page.PullRequests[0].AuthorUsername)
	}
	if page.NextCursor != "" {
		t.Fatalf("expected no next cursor, got %q", page.NextCursor)
	}
}
