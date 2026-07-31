package gitlab

import "testing"

func mergeRequestWebhookBody(action, state string) []byte {
	return []byte(`{
		"object_kind": "merge_request",
		"user": {"id": 3, "username": "actor", "name": "Actor"},
		"project": {"id": 77, "path_with_namespace": "group/repo", "web_url": "https://gitlab.com/group/repo"},
		"object_attributes": {
			"id": 888,
			"iid": 5,
			"title": "Add feature",
			"description": "description here",
			"state": "` + state + `",
			"action": "` + action + `",
			"source_branch": "feature-branch",
			"target_branch": "main",
			"author_id": 9,
			"url": "https://gitlab.com/group/repo/-/merge_requests/5",
			"created_at": "2026-01-01 00:00:00 UTC",
			"updated_at": "2026-01-02 00:00:00 UTC"
		}
	}`)
}

func noteWebhookBody() []byte {
	return []byte(`{
		"object_kind": "note",
		"user": {"id": 3, "username": "actor", "name": "Actor"},
		"project": {"id": 77, "path_with_namespace": "group/repo"},
		"object_attributes": {"id": 1, "note": "a comment", "noteable_type": "MergeRequest"}
	}`)
}

func TestConvertPullRequestStateEvent_Opened(t *testing.T) {
	p := &GitLabV2Provider{}
	event, matched, err := p.ConvertPullRequestStateEvent(
		map[string]string{"X-Gitlab-Event": "Merge Request Hook"},
		mergeRequestWebhookBody("open", "opened"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !matched {
		t.Fatal("expected matched=true for merge_request object_kind")
	}
	if event.State != "open" {
		t.Errorf("expected state open, got %s", event.State)
	}
	if event.RepositoryProviderID != "77" || event.RepositoryFullName != "group/repo" {
		t.Errorf("unexpected repository identity: %+v", event)
	}
	if event.Number != 5 || event.ProviderPRID != "888" {
		t.Errorf("unexpected MR identity: %+v", event)
	}
	if event.SourceBranch != "feature-branch" || event.TargetBranch != "main" {
		t.Errorf("unexpected branches: %+v", event)
	}
	if event.AuthorID != "9" {
		t.Errorf("expected author id 9, got %s", event.AuthorID)
	}
}

func TestConvertPullRequestStateEvent_Merged(t *testing.T) {
	p := &GitLabV2Provider{}
	event, matched, err := p.ConvertPullRequestStateEvent(
		map[string]string{"X-Gitlab-Event": "Merge Request Hook"},
		mergeRequestWebhookBody("merge", "merged"),
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

// TestConvertPullRequestStateEvent_IgnoresNoteEvents is the critical
// regression guard: comment ("note") events must NOT be intercepted here, so
// ConvertCommentEvent continues to handle them unchanged.
func TestConvertPullRequestStateEvent_IgnoresNoteEvents(t *testing.T) {
	p := &GitLabV2Provider{}
	event, matched, err := p.ConvertPullRequestStateEvent(
		map[string]string{"X-Gitlab-Event": "Note Hook"},
		noteWebhookBody(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched {
		t.Fatalf("expected matched=false for a note event, got event=%+v", event)
	}
}
