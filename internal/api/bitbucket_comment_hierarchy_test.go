package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBitbucketCommentHierarchyFromCapture(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(cwd, "..", ".."))
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("failed to change directory to repo root %s: %v", repoRoot, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	server, err := NewServer(8888, &VersionInfo{Version: "test"})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	t.Cleanup(func() {
		if server.db != nil {
			server.db.Close()
		}
	})

	provider := server.bitbucketProviderV2

	capturePath := filepath.Join("captures", "bitbucket", "20251014-194407", "bitbucket-webhook-pullrequest_comment_created-body-0025.json")
	body, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("failed to read capture file %s: %v", capturePath, err)
	}

	headers := map[string]string{
		"X-Event-Key": "pullrequest:comment_created",
	}

	event, err := provider.ConvertCommentEvent(headers, body)
	if err != nil {
		t.Fatalf("failed to convert comment event: %v", err)
	}

	if event.Comment == nil {
		t.Fatalf("converted event has nil comment")
	}

	commentJSON, err := json.MarshalIndent(event.Comment, "", "  ")
	if err != nil {
		t.Fatalf("failed to format comment: %v", err)
	}
	fmt.Println("Unified comment:\n" + string(commentJSON))
	t.Logf("Unified comment: %s", string(commentJSON))

	actorJSON, err := json.MarshalIndent(event.Actor, "", "  ")
	if err != nil {
		t.Fatalf("failed to format actor: %v", err)
	}
	fmt.Println("Actor:\n" + string(actorJSON))
	t.Logf("Actor: %s", string(actorJSON))

	if event.Comment.InReplyToID == nil {
		t.Fatalf("expected InReplyToID to be populated")
	}
	if *event.Comment.InReplyToID != "699722325" {
		t.Fatalf("unexpected InReplyToID: got %s", *event.Comment.InReplyToID)
	}

	if !strings.Contains(event.Comment.Body, "difference between") {
		t.Fatalf("comment body did not contain expected prompt; got: %s", event.Comment.Body)
	}

	metadata := event.Comment.Metadata
	if metadata == nil {
		t.Fatalf("metadata map is nil")
	}

	requiredKeys := []string{"workspace", "repository", "pr_number"}
	for _, key := range requiredKeys {
		if _, ok := metadata[key]; !ok {
			t.Fatalf("metadata missing key %s", key)
		}
	}

	workspace, _ := metadata["workspace"].(string)
	repository, _ := metadata["repository"].(string)
	prNumber := metadata["pr_number"]
	fmt.Printf("Metadata summary:\n  workspace=%s\n  repository=%s\n  pr_number=%v\n", workspace, repository, prNumber)
	t.Logf("Metadata summary: workspace=%s repository=%s pr_number=%v", workspace, repository, prNumber)

	if authorID := event.Comment.Author.ID; authorID == "" {
		t.Fatalf("author ID is empty")
	} else {
		fmt.Printf("Author ID: %s\n", authorID)
		t.Logf("Author ID: %s", authorID)
	}
}
