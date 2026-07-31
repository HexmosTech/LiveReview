package github

import "testing"

func pullRequestPayload(action, state string, merged bool) []byte {
	mergedStr := "false"
	if merged {
		mergedStr = "true"
	}
	return []byte(`{
		"action": "` + action + `",
		"number": 42,
		"pull_request": {
			"id": 999,
			"number": 42,
			"title": "Add feature",
			"body": "description here",
			"state": "` + state + `",
			"merged": ` + mergedStr + `,
			"draft": false,
			"html_url": "https://github.com/acme/repo/pull/42",
			"created_at": "2026-01-01T00:00:00Z",
			"updated_at": "2026-01-02T00:00:00Z",
			"head": {"ref": "feature-branch"},
			"base": {"ref": "main"},
			"user": {"id": 7, "login": "alice", "name": "Alice", "avatar_url": "https://example.com/a.png"}
		},
		"repository": {
			"id": 555,
			"name": "repo",
			"full_name": "acme/repo",
			"html_url": "https://github.com/acme/repo"
		}
	}`)
}

func TestConvertPullRequestStateEvent_Opened(t *testing.T) {
	p := &GitHubV2Provider{}
	event, matched, err := p.ConvertPullRequestStateEvent(
		map[string]string{"X-GitHub-Event": "pull_request"},
		pullRequestPayload("opened", "open", false),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected matched=true for pull_request/opened")
	}
	if event.State != "open" {
		t.Errorf("expected state open, got %s", event.State)
	}
	if event.RepositoryProviderID != "555" || event.RepositoryFullName != "acme/repo" {
		t.Errorf("unexpected repository identity: %+v", event)
	}
	if event.Number != 42 || event.ProviderPRID != "999" {
		t.Errorf("unexpected PR identity: %+v", event)
	}
	if event.AuthorUsername != "alice" {
		t.Errorf("expected author alice, got %s", event.AuthorUsername)
	}
	if event.SourceBranch != "feature-branch" || event.TargetBranch != "main" {
		t.Errorf("unexpected branches: %+v", event)
	}
}

func TestConvertPullRequestStateEvent_Merged(t *testing.T) {
	p := &GitHubV2Provider{}
	event, matched, err := p.ConvertPullRequestStateEvent(
		map[string]string{"X-GitHub-Event": "pull_request"},
		pullRequestPayload("closed", "closed", true),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected matched=true")
	}
	if event.State != "merged" {
		t.Errorf("expected state merged, got %s", event.State)
	}
}

// TestConvertPullRequestStateEvent_ReviewRequestedFallsThrough is the critical
// regression guard: review_requested/review_request_removed actions must NOT
// be intercepted here, so they keep flowing through the existing (working)
// ConvertReviewerEvent path unchanged.
func TestConvertPullRequestStateEvent_ReviewRequestedFallsThrough(t *testing.T) {
	p := &GitHubV2Provider{}
	for _, action := range []string{"review_requested", "review_request_removed"} {
		body := pullRequestPayload(action, "open", false)
		event, matched, err := p.ConvertPullRequestStateEvent(
			map[string]string{"X-GitHub-Event": "pull_request"}, body,
		)
		if err != nil {
			t.Fatalf("action=%s: unexpected error: %v", action, err)
		}
		if matched {
			t.Fatalf("action=%s: expected matched=false so ConvertReviewerEvent handles it, got matched=true event=%+v", action, event)
		}
	}
}

func TestConvertPullRequestStateEvent_IgnoresOtherEventTypes(t *testing.T) {
	p := &GitHubV2Provider{}
	_, matched, err := p.ConvertPullRequestStateEvent(
		map[string]string{"X-GitHub-Event": "issue_comment"},
		[]byte(`{"action":"created"}`),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched {
		t.Fatal("expected matched=false for a non pull_request event type")
	}
}
