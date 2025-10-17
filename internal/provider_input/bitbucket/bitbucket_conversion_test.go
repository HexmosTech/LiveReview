package bitbucket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	coreprocessor "github.com/livereview/internal/core_processor"
)

type unifiedEventsFixture struct {
	Events []coreprocessor.UnifiedWebhookEventV2 `json:"events"`
}

type expectedTimelineFixture struct {
	Items []coreprocessor.UnifiedTimelineItemV2 `json:"Items"`
}

func TestBitbucketUnifiedTimelineReplayMatchesGolden(t *testing.T) {
	t.Parallel()

	events := readUnifiedEventsFixture(t, "bitbucket-webhook-unified-events-0001.json")
	require.NotEmpty(t, events.Events)

	var comments []coreprocessor.UnifiedCommentV2
	for idx, event := range events.Events {
		require.NotNilf(t, event.Comment, "event %d missing comment", idx)
		comments = append(comments, *event.Comment)
	}

	builder := coreprocessor.UnifiedContextBuilderV2{}
	timeline := builder.BuildTimelineFromData(nil, comments)
	require.NotNil(t, timeline)

	expected := readExpectedTimelineFixture(t, "bitbucket-webhook-expected-timeline-0001.json")
	require.Equal(t, expected.Items, timeline.Items)
}

func TestBitbucketUnifiedTimelinePreservesReplyThread(t *testing.T) {
	t.Parallel()

	events := readUnifiedEventsFixture(t, "bitbucket-webhook-unified-events-thread-0001.json")
	require.Len(t, events.Events, 2)

	var comments []coreprocessor.UnifiedCommentV2
	for idx, event := range events.Events {
		require.NotNilf(t, event.Comment, "event %d missing comment", idx)
		require.NotNilf(t, event.Comment.InReplyToID, "event %d missing InReplyTo pointer", idx)
		comments = append(comments, *event.Comment)
	}

	builder := coreprocessor.UnifiedContextBuilderV2{}
	timeline := builder.BuildTimelineFromData(nil, comments)
	require.NotNil(t, timeline)
	require.Len(t, timeline.Items, len(comments))

	expected := readExpectedTimelineFixture(t, "bitbucket-webhook-expected-timeline-thread-0001.json")
	require.Equal(t, expected.Items, timeline.Items)

	for idx, item := range timeline.Items {
		require.NotNilf(t, item.Comment, "timeline item %d missing comment", idx)
		require.NotNilf(t, item.Comment.InReplyToID, "timeline item %d missing InReplyTo pointer", idx)
		require.Equal(t, "699722325", *item.Comment.InReplyToID)
	}
}

func TestBitbucketConvertCommentThreadMetadata(t *testing.T) {
	t.Parallel()

	provider := &BitbucketV2Provider{}

	tests := []struct {
		name             string
		comment          BitbucketV2Comment
		expectedReplyID  *string
		expectedThreadID string
	}{
		{
			name: "top-level comment seeds its own thread",
			comment: BitbucketV2Comment{
				ID:        101,
				Content:   BitbucketV2CommentContent{Raw: "Looks good"},
				User:      BitbucketV2User{Username: "reviewer", AccountID: "acct-1", UUID: "{uuid-1}"},
				CreatedOn: "2025-10-17T10:00:00Z",
				UpdatedOn: "2025-10-17T10:00:00Z",
				Links: BitbucketV2CommentLinks{HTML: struct {
					Href string `json:"href"`
				}{Href: "https://bitbucket.org/ws/repo/pull-requests/1#comment-101"}},
				Type: "pullrequest_comment",
			},
			expectedReplyID:  nil,
			expectedThreadID: "101",
		},
		{
			name: "reply comment binds to parent thread",
			comment: BitbucketV2Comment{
				ID:        202,
				Content:   BitbucketV2CommentContent{Raw: "Please clarify"},
				User:      BitbucketV2User{Username: "author", AccountID: "acct-2", UUID: "{uuid-2}"},
				CreatedOn: "2025-10-17T10:05:00Z",
				UpdatedOn: "2025-10-17T10:05:00Z",
				Parent: &BitbucketV2CommentRef{ID: 101, Links: BitbucketV2CommentLinks{HTML: struct {
					Href string `json:"href"`
				}{Href: "https://bitbucket.org/ws/repo/pull-requests/1#comment-101"}}},
				Links: BitbucketV2CommentLinks{HTML: struct {
					Href string `json:"href"`
				}{Href: "https://bitbucket.org/ws/repo/pull-requests/1#comment-202"}},
				Type: "pullrequest_comment",
			},
			expectedReplyID:  func() *string { id := "101"; return &id }(),
			expectedThreadID: "101",
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			payload := BitbucketV2WebhookPayload{
				Comment: tc.comment,
				PullRequest: BitbucketV2PullRequest{ID: 1, Title: "Add feature", Links: BitbucketV2Links{HTML: struct {
					Href string `json:"href"`
				}{Href: "https://bitbucket.org/ws/repo/pull-requests/1"}}},
				Repository: BitbucketV2Repository{Name: "repo", Owner: BitbucketV2User{Username: "ws"}},
			}

			comment := provider.convertBitbucketToUnifiedCommentV2(payload)

			if tc.expectedReplyID == nil {
				require.Nil(t, comment.InReplyToID)
			} else {
				require.NotNil(t, comment.InReplyToID)
				require.Equal(t, *tc.expectedReplyID, *comment.InReplyToID)
			}

			require.NotNil(t, comment.DiscussionID)
			require.Equal(t, tc.expectedThreadID, *comment.DiscussionID)
			require.NotNil(t, comment.Metadata)
			require.Equal(t, tc.expectedThreadID, comment.Metadata["thread_id"])
		})
	}
}

func readUnifiedEventsFixture(t *testing.T, name string) unifiedEventsFixture {
	t.Helper()

	bytes := readFixture(t, name)

	var fixture unifiedEventsFixture
	require.NoError(t, json.Unmarshal(bytes, &fixture))
	return fixture
}

func readExpectedTimelineFixture(t *testing.T, name string) expectedTimelineFixture {
	t.Helper()

	bytes := readFixture(t, name)

	var fixture expectedTimelineFixture
	require.NoError(t, json.Unmarshal(bytes, &fixture))
	return fixture
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read fixture %s", path)
	return data
}
