package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	coreprocessor "github.com/livereview/internal/core_processor"
	bitbucketprovider "github.com/livereview/internal/provider_input/bitbucket"
)

// capturingBitbucketOutput records the parameters used when posting a Bitbucket reply.
type capturingBitbucketOutput struct {
	callCount   int
	workspace   string
	repository  string
	prNumber    string
	inReplyToID *string
	replyText   string
}

func (c *capturingBitbucketOutput) PostCommentReply(workspace, repository, prNumber string, inReplyToID *string, replyText, _, _ string) error {
	c.callCount++
	c.workspace = workspace
	c.repository = repository
	c.prNumber = prNumber
	if inReplyToID != nil {
		copied := *inReplyToID
		c.inReplyToID = &copied
	} else {
		c.inReplyToID = nil
	}
	c.replyText = replyText
	return nil
}

// stubUnifiedProcessor forces a deterministic response for the reply flow.
type stubUnifiedProcessor struct {
	reply string
}

func (s *stubUnifiedProcessor) CheckResponseWarrant(coreprocessor.UnifiedWebhookEventV2, *coreprocessor.UnifiedBotUserInfoV2) (bool, coreprocessor.ResponseScenarioV2) {
	return true, coreprocessor.ResponseScenarioV2{Type: "comment_reply"}
}

func (s *stubUnifiedProcessor) ProcessCommentReply(context.Context, coreprocessor.UnifiedWebhookEventV2, *coreprocessor.UnifiedTimelineV2, int64) (string, *coreprocessor.LearningMetadataV2, error) {
	return s.reply, nil, nil
}

func (s *stubUnifiedProcessor) ProcessFullReview(context.Context, coreprocessor.UnifiedWebhookEventV2, *coreprocessor.UnifiedTimelineV2) ([]coreprocessor.UnifiedReviewCommentV2, *coreprocessor.LearningMetadataV2, error) {
	return nil, nil, fmt.Errorf("unexpected full review invocation")
}

func TestBitbucketCommentReplyPostingFromCapture(t *testing.T) {
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
		if db := server.DB(); db != nil {
			db.Close()
		}
	})

	captureDir := filepath.Join("captures", "bitbucket", "20251014-194407")
	requestCapturePath := filepath.Join(captureDir, "bitbucket-webhook-pullrequest_comment_created-body-0025.json")
	responseCapturePath := filepath.Join(captureDir, "bitbucket-webhook-pullrequest_comment_created-unified-0032.json")

	body, err := os.ReadFile(requestCapturePath)
	if err != nil {
		t.Fatalf("failed to read request capture %s: %v", requestCapturePath, err)
	}

	responsePayload, err := os.ReadFile(responseCapturePath)
	if err != nil {
		t.Fatalf("failed to read response capture %s: %v", responseCapturePath, err)
	}

	var responseEvent coreprocessor.UnifiedWebhookEventV2
	if err := json.Unmarshal(responsePayload, &responseEvent); err != nil {
		t.Fatalf("failed to unmarshal response capture: %v", err)
	}
	if responseEvent.Comment == nil {
		t.Fatalf("response capture missing comment")
	}
	expectedReply := responseEvent.Comment.Body

	stubOutput := &capturingBitbucketOutput{}
	provider := bitbucketprovider.NewBitbucketV2Provider(server.DB(), stubOutput)
	server.bitbucketProviderV2 = provider
	if server.webhookRegistryV2 != nil {
		server.webhookRegistryV2.providers["bitbucket"] = provider
	}
	if server.webhookOrchestratorV2 == nil {
		t.Fatalf("webhook orchestrator not initialized")
	}
	stubProcessor := &stubUnifiedProcessor{reply: expectedReply}
	server.webhookOrchestratorV2.unifiedProcessor = stubProcessor

	headers := map[string]string{"X-Event-Key": "pullrequest:comment_created"}
	event, err := provider.ConvertCommentEvent(headers, body)
	if err != nil {
		t.Fatalf("failed to convert comment event: %v", err)
	}
	if event.Comment == nil {
		t.Fatalf("converted event has nil comment")
	}
	if event.Comment.InReplyToID == nil {
		t.Fatalf("converted event missing InReplyToID")
	}

	timeline := &coreprocessor.UnifiedTimelineV2{}
	server.webhookOrchestratorV2.handleCommentReplyFlow(context.Background(), event, provider, timeline, 0)

	if stubOutput.callCount != 1 {
		t.Fatalf("expected PostCommentReply to be called once, got %d", stubOutput.callCount)
	}
	if stubOutput.workspace != "contorted" {
		t.Fatalf("unexpected workspace: %s", stubOutput.workspace)
	}
	if stubOutput.repository != "fb_backends" {
		t.Fatalf("unexpected repository: %s", stubOutput.repository)
	}
	if stubOutput.prNumber != "1" {
		t.Fatalf("unexpected PR number: %s", stubOutput.prNumber)
	}
	if stubOutput.inReplyToID == nil || *stubOutput.inReplyToID != *event.Comment.InReplyToID {
		t.Fatalf("unexpected InReplyToID: got %v, want %s", stubOutput.inReplyToID, *event.Comment.InReplyToID)
	}
	if stubOutput.replyText != expectedReply {
		t.Fatalf("reply text mismatch. got:\n%s\nwant:\n%s", stubOutput.replyText, expectedReply)
	}
}
