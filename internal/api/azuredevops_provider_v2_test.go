package api

import (
	"testing"

	azuredevopsprovider "github.com/livereview/internal/provider_input/azuredevops"
	giteaprovider "github.com/livereview/internal/provider_input/gitea"
	githubprovider "github.com/livereview/internal/provider_input/github"
	"github.com/stretchr/testify/assert"
)

type stubAzureDevOpsOutput struct{}

func (stubAzureDevOpsOutput) PostCommentReply(_ *azuredevopsprovider.UnifiedWebhookEventV2, _, _ string) error {
	return nil
}

func (stubAzureDevOpsOutput) PostEmojiReaction(_ *azuredevopsprovider.UnifiedWebhookEventV2, _, _ string) error {
	return nil
}

func (stubAzureDevOpsOutput) PostReviewComments(_ azuredevopsprovider.UnifiedMergeRequestV2, _ string, _ []azuredevopsprovider.UnifiedReviewCommentV2) error {
	return nil
}

type stubGiteaOutput struct{}

func (stubGiteaOutput) PostCommentReply(_ *giteaprovider.UnifiedWebhookEventV2, _, _ string) error {
	return nil
}

func (stubGiteaOutput) PostEmojiReaction(_ *giteaprovider.UnifiedWebhookEventV2, _, _ string) error {
	return nil
}

func (stubGiteaOutput) PostReviewComments(_ giteaprovider.UnifiedMergeRequestV2, _ string, _ []giteaprovider.UnifiedReviewCommentV2) error {
	return nil
}

var azureCreatedFixture = []byte(`{
	"id": "guid-1",
	"eventType": "git.pullrequest.created",
	"publisherId": "tfs",
	"resource": {
		"repository": {"id": "r1", "name": "repo", "project": {"id": "p1", "name": "proj"}},
		"pullRequestId": 1,
		"status": "active",
		"createdBy": {"id": "u1", "displayName": "Alice", "uniqueName": "alice@example.com"},
		"sourceRefName": "refs/heads/feature",
		"targetRefName": "refs/heads/main",
		"url": "https://dev.azure.com/org/proj/_apis/git/repositories/repo/pullRequests/1"
	},
	"resourceContainers": {
		"collection": {"id": "c1"},
		"account": {"id": "a1"},
		"project": {"id": "p1"}
	},
	"createdDate": "2026-01-01T00:00:00Z"
}`)

// azureCommentFixture only needs to be valid enough for CanHandleWebhook's
// envelope-level detection (publisherId/eventType/resourceContainers) - it
// doesn't parse "resource" at all, so this fixture is only used for that.
// For the real (flat, doc-defying) resource shape and full conversion
// correctness, see the live-captured fixture in
// internal/provider_input/azuredevops/azuredevops_conversion_test.go.
var azureCommentFixture = []byte(`{
	"id": "guid-2",
	"eventType": "ms.vss-code.git-pullrequest-comment-event",
	"publisherId": "tfs",
	"resource": {
		"id": 5,
		"parentCommentId": 0,
		"content": "@livereview please review",
		"author": {"id": "u2", "displayName": "Bob", "uniqueName": "bob@example.com"},
		"publishedDate": "2026-01-01T00:00:00Z",
		"_links": {
			"repository": {"href": "https://dev.azure.com/org/proj/_apis/git/repositories/repo-guid"},
			"threads": {"href": "https://dev.azure.com/org/proj/_apis/git/repositories/repo-guid/pullRequests/1/threads/9"},
			"pullRequests": {"href": "https://dev.azure.com/org/_apis/git/pullRequests/1"}
		}
	},
	"resourceContainers": {
		"collection": {"id": "c1"},
		"account": {"id": "a1"},
		"project": {"id": "p1"}
	},
	"createdDate": "2026-01-01T00:00:00Z"
}`)

func TestAzureDevOpsV2Provider_CanHandleWebhook(t *testing.T) {
	provider := azuredevopsprovider.NewAzureDevOpsV2Provider(nil, stubAzureDevOpsOutput{})
	assert.Equal(t, "azuredevops", provider.ProviderName())

	tests := []struct {
		name      string
		headers   map[string]string
		body      []byte
		canHandle bool
	}{
		{
			name:      "Azure DevOps pull request created",
			headers:   map[string]string{"Content-Type": "application/json"},
			body:      azureCreatedFixture,
			canHandle: true,
		},
		{
			name:      "Azure DevOps comment updated",
			headers:   map[string]string{"Content-Type": "application/json"},
			body:      azureCommentFixture,
			canHandle: true,
		},
		{
			name: "GitHub webhook does not cross-match",
			headers: map[string]string{
				"X-GitHub-Event":    "issue_comment",
				"X-GitHub-Delivery": "test-id",
			},
			body:      []byte(`{"action": "created"}`),
			canHandle: false,
		},
		{
			name:    "GitLab webhook does not cross-match",
			headers: map[string]string{"X-Gitlab-Event": "Note Hook"},
			body:    []byte(`{"object_kind": "note"}`),
			canHandle: false,
		},
		{
			name:      "Bitbucket webhook does not cross-match",
			headers:   map[string]string{"X-Event-Key": "pullrequest:comment_created"},
			body:      []byte(`{"eventKey": "pullrequest:comment_created"}`),
			canHandle: false,
		},
		{
			name:      "Gitea webhook does not cross-match",
			headers:   map[string]string{"X-Gitea-Event": "issue_comment"},
			body:      []byte(`{"action": "created"}`),
			canHandle: false,
		},
		{
			name:      "non-JSON body",
			headers:   map[string]string{},
			body:      []byte(`not json`),
			canHandle: false,
		},
		{
			name:      "JSON missing publisherId/resourceContainers",
			headers:   map[string]string{},
			body:      []byte(`{"eventType": "git.pullrequest.created"}`),
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

// TestAzureDevOpsFixture_DoesNotCrossMatchOtherProviders confirms other
// providers' CanHandleWebhook implementations reject Azure DevOps payloads,
// verifying zero cross-detection in both directions.
func TestAzureDevOpsFixture_DoesNotCrossMatchOtherProviders(t *testing.T) {
	giteaProv := giteaprovider.NewGiteaV2Provider(nil, stubGiteaOutput{})
	githubProv := githubprovider.NewGitHubV2Provider(nil, stubGitHubOutput{})

	assert.False(t, githubProv.CanHandleWebhook(map[string]string{}, azureCreatedFixture))
	assert.False(t, giteaProv.CanHandleWebhook(map[string]string{}, azureCreatedFixture))
	assert.False(t, giteaProv.CanHandleWebhook(map[string]string{}, azureCommentFixture))
}
