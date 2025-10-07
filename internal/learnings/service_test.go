package learnings

import (
	"context"
	"testing"
)

func TestUpsertAndDedupe(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()
	orgID := int64(1)

	// first insert
	id1, sid1, act1, err := svc.UpsertFromMetadata(ctx, orgID, Draft{Title: "Prefer snake_case", Body: "We prefer snake_case for db columns.", Scope: ScopeOrg}, &MRContext{Provider: "github", Repository: "org/repo"})
	if err != nil || act1 != "added" || id1 == "" || sid1 == "" {
		t.Fatalf("add failed: %v %s", err, act1)
	}

	// near-duplicate should update
	id2, sid2, act2, err := svc.UpsertFromMetadata(ctx, orgID, Draft{Title: "snake_case preference", Body: "snake_case for db columns pls", Scope: ScopeOrg}, &MRContext{Provider: "github", Repository: "org/repo"})
	if err != nil || act2 != "updated" {
		t.Fatalf("dedupe update failed: %v %s", err, act2)
	}
	if id1 != id2 || sid1 != sid2 {
		t.Fatalf("should update same record")
	}
}

func TestUpdateDeleteAndFetch(t *testing.T) {
	store := NewInMemoryStore()
	svc := NewService(store)
	ctx := context.Background()
	orgID := int64(1)

	_, sid, act, err := svc.UpsertFromMetadata(ctx, orgID, Draft{Title: "Prefer UTC time", Body: "All timestamps in UTC", Scope: ScopeRepo, RepoID: "repo1"}, &MRContext{Provider: "gitlab", Repository: "g/r"})
	if err != nil || act != "added" {
		t.Fatalf("add failed: %v", err)
	}

	// update title via deltas
	newTitle := "Always use UTC"
	_, actU, err := svc.UpdateFromMetadata(ctx, orgID, sid, Deltas{Title: &newTitle})
	if err != nil || actU != "updated" {
		t.Fatalf("update failed: %v", err)
	}

	// fetch relevant for same repo
	list, err := svc.FetchRelevant(ctx, orgID, "repo1", []string{"db/models/user.go"}, "user times", "UTC handling", 5)
	if err != nil || len(list) == 0 {
		t.Fatalf("fetch relevant failed: %v", err)
	}

	// delete
	if err := svc.DeleteByShortID(ctx, orgID, sid, &MRContext{Provider: "gitlab"}); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// should not appear after archive
	list2, err := svc.FetchRelevant(ctx, orgID, "repo1", nil, "user times", "UTC handling", 5)
	if err != nil {
		t.Fatalf("fetch after delete failed: %v", err)
	}
	if len(list2) != 0 {
		t.Fatalf("archived learning should not be returned")
	}
}
