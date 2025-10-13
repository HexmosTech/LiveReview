package api

import (
	"database/sql"
	"testing"

	_ "github.com/lib/pq"
	bitbucketprovider "github.com/livereview/internal/provider_input/bitbucket"
	githubprovider "github.com/livereview/internal/provider_input/github"
	gitlabprovider "github.com/livereview/internal/provider_input/gitlab"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubGitHubOutput struct{}

func (stubGitHubOutput) PostCommentReply(_ *githubprovider.UnifiedWebhookEventV2, _, _ string) error {
	return nil
}

func (stubGitHubOutput) PostEmojiReaction(_ *githubprovider.UnifiedWebhookEventV2, _, _ string) error {
	return nil
}

func (stubGitHubOutput) PostReviewComments(_ githubprovider.UnifiedMergeRequestV2, _ string, _ []githubprovider.UnifiedReviewCommentV2) error {
	return nil
}

type stubGitLabOutput struct{}

func (stubGitLabOutput) PostCommentReply(_ *gitlabprovider.UnifiedWebhookEventV2, _, _, _ string) error {
	return nil
}

func (stubGitLabOutput) PostEmojiReaction(_ *gitlabprovider.UnifiedWebhookEventV2, _, _, _ string) error {
	return nil
}

func (stubGitLabOutput) PostFullReview(_ *gitlabprovider.UnifiedWebhookEventV2, _, _, _ string) error {
	return nil
}

type stubBitbucketOutput struct{}

func (stubBitbucketOutput) PostCommentReply(_, _, _ string, _ *string, _, _, _ string) error {
	return nil
}

func newTestGitLabProvider(t *testing.T) *gitlabprovider.GitLabV2Provider {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})
	return gitlabprovider.NewGitLabV2Provider(db, stubGitLabOutput{})
}

func newTestBitbucketProvider(t *testing.T) *bitbucketprovider.BitbucketV2Provider {
	t.Helper()
	db, err := sql.Open("postgres", "postgres://test:test@localhost:5432/test?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close()
	})
	return bitbucketprovider.NewBitbucketV2Provider(db, stubBitbucketOutput{})
}

// Phase 9: Provider V2 Integration Tests
// Tests the individual V2 providers and their integration with the unified system

func TestGitLabV2Provider(t *testing.T) {
	provider := newTestGitLabProvider(t)

	// Test provider identification
	assert.Equal(t, "gitlab", provider.ProviderName())

	// Test webhook detection capability
	tests := []struct {
		name      string
		headers   map[string]string
		body      []byte
		canHandle bool
	}{
		{
			name: "GitLab Note Hook",
			headers: map[string]string{
				"X-Gitlab-Event": "Note Hook",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"object_kind": "note"}`),
			canHandle: true,
		},
		{
			name: "GitLab MR Hook",
			headers: map[string]string{
				"X-Gitlab-Event": "Merge Request Hook",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"object_kind": "merge_request"}`),
			canHandle: true,
		},
		{
			name: "Non-GitLab webhook",
			headers: map[string]string{
				"X-GitHub-Event": "issue_comment",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"action": "created"}`),
			canHandle: false,
		},
		{
			name: "No headers",
			headers: map[string]string{
				"Content-Type": "application/json",
			},
			body:      []byte(`{"test": "data"}`),
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.CanHandleWebhook(tt.headers, tt.body)
			assert.Equal(t, tt.canHandle, result)
		})
	}
}

func TestGitHubV2Provider(t *testing.T) {
	provider := githubprovider.NewGitHubV2Provider(nil, stubGitHubOutput{})

	assert.Equal(t, "github", provider.ProviderName())

	tests := []struct {
		name      string
		headers   map[string]string
		body      []byte
		canHandle bool
	}{
		{
			name: "GitHub issue comment",
			headers: map[string]string{
				"X-GitHub-Event":    "issue_comment",
				"X-GitHub-Delivery": "test-id",
				"Content-Type":      "application/json",
			},
			body:      []byte(`{"action": "created"}`),
			canHandle: true,
		},
		{
			name: "GitHub PR review comment",
			headers: map[string]string{
				"X-GitHub-Event":    "pull_request_review_comment",
				"X-GitHub-Delivery": "test-id",
				"Content-Type":      "application/json",
			},
			body:      []byte(`{"action": "created"}`),
			canHandle: true,
		},
		{
			name: "Non-GitHub webhook",
			headers: map[string]string{
				"X-Gitlab-Event": "Note Hook",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"object_kind": "note"}`),
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.CanHandleWebhook(tt.headers, tt.body)
			assert.Equal(t, tt.canHandle, result)
		})
	}
}

func TestBitbucketV2Provider(t *testing.T) {
	provider := newTestBitbucketProvider(t)

	assert.Equal(t, "bitbucket", provider.ProviderName())

	tests := []struct {
		name      string
		headers   map[string]string
		body      []byte
		canHandle bool
	}{
		{
			name: "Bitbucket comment created",
			headers: map[string]string{
				"X-Event-Key":    "pullrequest:comment_created",
				"X-Request-UUID": "test-uuid",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"eventKey": "pullrequest:comment_created"}`),
			canHandle: true,
		},
		{
			name: "Bitbucket PR updated",
			headers: map[string]string{
				"X-Event-Key":    "pullrequest:updated",
				"X-Request-UUID": "test-uuid",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"eventKey": "pullrequest:updated"}`),
			canHandle: true,
		},
		{
			name: "Non-Bitbucket webhook",
			headers: map[string]string{
				"X-GitHub-Event": "issue_comment",
				"Content-Type":   "application/json",
			},
			body:      []byte(`{"action": "created"}`),
			canHandle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.CanHandleWebhook(tt.headers, tt.body)
			assert.Equal(t, tt.canHandle, result)
		})
	}
}

func TestUnifiedEventConversion(t *testing.T) {
	t.Run("GitLab Comment Event Conversion", func(t *testing.T) {
		provider := newTestGitLabProvider(t)

		headers := map[string]string{
			"X-Gitlab-Event": "Note Hook",
			"Content-Type":   "application/json",
		}

		body := []byte(`{
			"object_kind": "note",
			"event_type": "note",
			"user": {
				"id": 123,
				"username": "testuser",
				"name": "Test User",
				"email": "test@example.com"
			},
			"project": {
				"id": 1,
				"name": "test-project",
				"path_with_namespace": "group/test-project",
				"web_url": "https://gitlab.example.com/group/test-project"
			},
			"object_attributes": {
				"id": 456,
				"note": "@livereview please review this code",
				"noteable_type": "MergeRequest",
				"noteable_id": 789,
				"system": false,
				"created_at": "2025-10-08T10:00:00Z",
				"updated_at": "2025-10-08T10:00:00Z"
			},
			"merge_request": {
				"id": 789,
				"iid": 1,
				"title": "Test MR",
				"description": "Test merge request",
				"state": "opened",
				"source_branch": "feature-branch",
				"target_branch": "main",
				"author_id": 456
			}
		}`)

		event, err := provider.ConvertCommentEvent(headers, body)

		require.NoError(t, err)
		require.NotNil(t, event)

		// Verify unified event structure
		assert.Equal(t, "comment_created", event.EventType)
		assert.Equal(t, "gitlab", event.Provider)
		assert.NotEmpty(t, event.Timestamp)

		// Verify repository info
		assert.Equal(t, "test-project", event.Repository.Name)
		assert.Equal(t, "group/test-project", event.Repository.FullName)
		assert.Equal(t, "https://gitlab.example.com/group/test-project", event.Repository.WebURL)

		// Verify comment info
		require.NotNil(t, event.Comment)
		assert.Equal(t, "@livereview please review this code", event.Comment.Body)
		assert.Equal(t, "testuser", event.Comment.Author.Username)

		// Verify MR info
		require.NotNil(t, event.MergeRequest)
		assert.Equal(t, "789", event.MergeRequest.ID)
		assert.Equal(t, 1, event.MergeRequest.Number)
		assert.Equal(t, "Test MR", event.MergeRequest.Title)
		assert.Equal(t, "feature-branch", event.MergeRequest.SourceBranch)
		assert.Equal(t, "main", event.MergeRequest.TargetBranch)
	})

	t.Run("GitHub Comment Event Conversion", func(t *testing.T) {
		provider := githubprovider.NewGitHubV2Provider(nil, stubGitHubOutput{})

		headers := map[string]string{
			"X-GitHub-Event":    "issue_comment",
			"X-GitHub-Delivery": "test-delivery-id",
			"Content-Type":      "application/json",
		}

		body := []byte(`{
			"action": "created",
			"issue": {
				"number": 1,
				"title": "Test Issue",
				"body": "Test issue body",
				"state": "open",
				"pull_request": {
					"url": "https://api.github.com/repos/owner/repo/pulls/1"
				}
			},
			"comment": {
				"id": 123456,
				"body": "@livereview please review this PR",
				"created_at": "2025-10-08T10:00:00Z",
				"updated_at": "2025-10-08T10:00:00Z",
				"user": {
					"id": 789,
					"login": "testuser",
					"type": "User"
				}
			},
			"repository": {
				"id": 456,
				"name": "test-repo",
				"full_name": "owner/test-repo",
				"html_url": "https://github.com/owner/test-repo",
				"private": false
			},
			"sender": {
				"id": 789,
				"login": "testuser",
				"type": "User"
			}
		}`)

		event, err := provider.ConvertCommentEvent(headers, body)

		require.NoError(t, err)
		require.NotNil(t, event)

		assert.Equal(t, "comment_created", event.EventType)
		assert.Equal(t, "github", event.Provider)

		// Verify repository info
		assert.Equal(t, "test-repo", event.Repository.Name)
		assert.Equal(t, "owner/test-repo", event.Repository.FullName)
		assert.Equal(t, "https://github.com/owner/test-repo", event.Repository.WebURL)

		// Verify comment info
		require.NotNil(t, event.Comment)
		assert.Equal(t, "@livereview please review this PR", event.Comment.Body)
		assert.Equal(t, "testuser", event.Comment.Author.Username)
	})
}

func TestProviderRegistry(t *testing.T) {
	server := &Server{}
	registry := NewWebhookProviderRegistry(server)

	// Test registry initialization
	stats := registry.GetProviderStats()
	assert.NotNil(t, stats)

	providers := stats["providers"].([]string)
	assert.Contains(t, providers, "gitlab")
	assert.Contains(t, providers, "github")
	assert.Contains(t, providers, "bitbucket")
	assert.Equal(t, 3, stats["total_providers"])

	// Test provider registration
	mockProvider := &MockProviderV2{name: "test-provider"}
	registry.RegisterProvider("test", mockProvider)

	updatedStats := registry.GetProviderStats()
	updatedProviders := updatedStats["providers"].([]string)
	assert.Contains(t, updatedProviders, "test")
	assert.Equal(t, 4, updatedStats["total_providers"])
}

func TestUnifiedTypes(t *testing.T) {
	// Test that unified types can be created and used correctly

	// Test UnifiedWebhookEventV2
	event := &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  "gitlab",
		Timestamp: "2025-10-08T10:00:00Z",
		Repository: UnifiedRepositoryV2{
			Name:     "test-repo",
			FullName: "group/test-repo",
			WebURL:   "https://gitlab.example.com/group/test-repo",
		},
		Comment: &UnifiedCommentV2{
			ID:   "123",
			Body: "test comment",
			Author: UnifiedUserV2{
				Username: "testuser",
				Name:     "Test User",
			},
		},
		Actor: UnifiedUserV2{
			Username: "testuser",
			Name:     "Test User",
		},
	}

	assert.Equal(t, "comment_created", event.EventType)
	assert.Equal(t, "gitlab", event.Provider)
	assert.NotNil(t, event.Comment)
	assert.Equal(t, "test comment", event.Comment.Body)

	// Test UnifiedMergeRequestV2
	mr := UnifiedMergeRequestV2{
		ID:           "456",
		Number:       1,
		Title:        "Test MR",
		Description:  "Test description",
		State:        "opened",
		SourceBranch: "feature",
		TargetBranch: "main",
		Author: UnifiedUserV2{
			Username: "author",
			Name:     "MR Author",
		},
	}

	assert.Equal(t, "456", mr.ID)
	assert.Equal(t, 1, mr.Number)
	assert.Equal(t, "Test MR", mr.Title)

	// Test ResponseScenarioV2
	scenario := ResponseScenarioV2{
		Type:       "comment_reply",
		Reason:     "bot_mentioned",
		Confidence: 0.95,
		Metadata: map[string]interface{}{
			"trigger": "direct_mention",
		},
	}

	assert.Equal(t, "comment_reply", scenario.Type)
	assert.Equal(t, "bot_mentioned", scenario.Reason)
	assert.Equal(t, 0.95, scenario.Confidence)

	// Test LearningMetadataV2
	learning := LearningMetadataV2{
		Type:       "best_practice",
		Content:    "Always validate inputs",
		Context:    "Security discussion",
		Confidence: 0.8,
		Tags:       []string{"security", "validation"},
		OrgID:      1,
		Metadata: map[string]interface{}{
			"pattern": "security_review",
		},
	}

	assert.Equal(t, "best_practice", learning.Type)
	assert.Equal(t, "Always validate inputs", learning.Content)
	assert.Contains(t, learning.Tags, "security")
	assert.Equal(t, int64(1), learning.OrgID)
}

// MockProviderV2 for testing
type MockProviderV2 struct {
	name string
}

func (m *MockProviderV2) ProviderName() string {
	return m.name
}

func (m *MockProviderV2) CanHandleWebhook(headers map[string]string, body []byte) bool {
	return headers["X-Test-Provider"] == m.name
}

func (m *MockProviderV2) ConvertCommentEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	return &UnifiedWebhookEventV2{
		EventType: "comment_created",
		Provider:  m.name,
	}, nil
}

func (m *MockProviderV2) ConvertReviewerEvent(headers map[string]string, body []byte) (*UnifiedWebhookEventV2, error) {
	return &UnifiedWebhookEventV2{
		EventType: "reviewer_assigned",
		Provider:  m.name,
	}, nil
}

func (m *MockProviderV2) FetchMergeRequestData(event *UnifiedWebhookEventV2) error {
	return nil
}

func (m *MockProviderV2) PostCommentReply(event *UnifiedWebhookEventV2, content string) error {
	return nil
}

func (m *MockProviderV2) PostEmojiReaction(event *UnifiedWebhookEventV2, emoji string) error {
	return nil
}

func (m *MockProviderV2) PostFullReview(event *UnifiedWebhookEventV2, overallComment string) error {
	return nil
}
