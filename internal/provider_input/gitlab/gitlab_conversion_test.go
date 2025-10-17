package gitlab

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	coreprocessor "github.com/livereview/internal/core_processor"
	providergitlab "github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/pkg/models"
)

type unifiedEventsFixture struct {
	Events []coreprocessor.UnifiedWebhookEventV2 `json:"events"`
}

type expectedDiffsFixture struct {
	Diffs []*models.CodeDiff `json:"diffs"`
}

func TestGitLabUnifiedTimelineReplayMatchesGolden(t *testing.T) {
	t.Parallel()

	events := readUnifiedEventsFixture(t, "gitlab-webhook-unified-events-0001.json")
	expected := readExpectedTimeline(t, "gitlab-webhook-expected-timeline-0001.json")

	var comments []coreprocessor.UnifiedCommentV2
	for idx, event := range events.Events {
		require.NotNilf(t, event.Comment, "event %d missing comment", idx)
		comments = append(comments, *event.Comment)
	}

	builder := coreprocessor.UnifiedContextBuilderV2{}
	timeline := builder.BuildTimelineFromData(nil, comments)
	require.NotNil(t, timeline)
	require.Equal(t, expected, *timeline)
}

func TestGitLabUnifiedTimelinePreservesDiscussionSequences(t *testing.T) {
	t.Parallel()

	events := readUnifiedEventsFixture(t, "gitlab-webhook-unified-events-thread-0002.json")
	require.Len(t, events.Events, 3)

	var comments []coreprocessor.UnifiedCommentV2
	for idx, event := range events.Events {
		require.NotNilf(t, event.Comment, "event %d missing comment", idx)
		comments = append(comments, *event.Comment)
	}

	builder := coreprocessor.UnifiedContextBuilderV2{}
	timeline := builder.BuildTimelineFromData(nil, comments)
	require.NotNil(t, timeline)
	require.Len(t, timeline.Items, 3)

	expected := readExpectedTimeline(t, "gitlab-webhook-expected-timeline-thread-0002.json")
	require.Equal(t, expected, *timeline)

	require.NotNil(t, timeline.Items[0].Comment)
	discussionID := timeline.Items[0].Comment.DiscussionID
	require.NotNil(t, discussionID)

	for idx, item := range timeline.Items {
		require.NotNilf(t, item.Comment, "timeline item %d missing comment", idx)
		require.NotNilf(t, item.Comment.DiscussionID, "timeline item %d missing discussion id", idx)
		require.Equal(t, *discussionID, *item.Comment.DiscussionID)
	}
}

func TestGitLabConvertCommentEventPopulatesThreadMetadata(t *testing.T) {
	t.Parallel()

	provider := &GitLabV2Provider{}
	headers := map[string]string{"X-Gitlab-Event": "Note Hook"}
	body := []byte(`{
		"object_kind": "note",
		"user": {
			"id": 11,
			"username": "reviewer",
			"name": "Reviewer",
			"web_url": "https://gitlab.example.com/reviewer",
			"avatar_url": "https://gitlab.example.com/uploads/avatar.png"
		},
		"project": {
			"id": 22,
			"name": "sample",
			"path_with_namespace": "org/sample",
			"web_url": "https://gitlab.example.com/org/sample"
		},
		"repository": {
			"name": "sample",
			"url": "https://gitlab.example.com/org/sample.git"
		},
		"merge_request": {
			"id": 33,
			"iid": 7,
			"title": "Add feature",
			"description": "",
			"state": "opened",
			"created_at": "2025-10-17T09:00:00Z",
			"updated_at": "2025-10-17T09:05:00Z",
			"web_url": "https://gitlab.example.com/org/sample/-/merge_requests/7",
			"target_branch": "main",
			"source_branch": "feature",
			"author": {
				"id": 44,
				"username": "author",
				"name": "Author",
				"web_url": "https://gitlab.example.com/author",
				"avatar_url": "https://gitlab.example.com/uploads/author.png"
			}
		},
		"object_attributes": {
			"id": 55,
			"note": "Thanks @bot for the insight",
			"noteable_type": "MergeRequest",
			"author_id": 11,
			"created_at": "2025-10-17T09:01:00Z",
			"updated_at": "2025-10-17T09:02:00Z",
			"project_id": 22,
			"noteable_id": 33,
			"system": false,
			"url": "https://gitlab.example.com/org/sample/-/merge_requests/7#note_55",
			"discussion_id": "abc123",
			"type": "DiscussionNote",
			"in_reply_to_id": 54
		}
	}`)

	event, err := provider.ConvertCommentEvent(headers, body)
	require.NoError(t, err)
	require.NotNil(t, event)
	require.NotNil(t, event.Comment)

	require.NotNil(t, event.Comment.InReplyToID)
	require.Equal(t, "54", *event.Comment.InReplyToID)
	require.NotNil(t, event.Comment.DiscussionID)
	require.Equal(t, "abc123", *event.Comment.DiscussionID)
	require.NotNil(t, event.Comment.Metadata)
	require.Equal(t, "DiscussionNote", event.Comment.Metadata["comment_type"])
	require.Equal(t, "MergeRequest", event.Comment.Metadata["noteable_type"])
	require.Equal(t, "abc123", event.Comment.Metadata["discussion_id"])
}

func TestGitLabDiscussionConversionIncludesPositions(t *testing.T) {
	t.Parallel()

	discussions := readDiscussionsFixture(t, "gitlab-mr-discussions-0001.json")

	comments := convertGitLabCommentsToUnifiedV2(discussions, nil)
	require.Len(t, comments, 7)

	commentByID := map[string]coreprocessor.UnifiedCommentV2{}
	for _, c := range comments {
		commentByID[c.ID] = c
	}

	botThread, ok := commentByID["24386"]
	require.True(t, ok)
	require.NotNil(t, botThread.DiscussionID)
	require.Equal(t, "9a6fe655f27a6768e63b32799d0899f1a32a5cb9", *botThread.DiscussionID)
	require.NotNil(t, botThread.Position)
	require.Equal(t, "liveapi-backend/qmanager/repodag.go", botThread.Position.FilePath)

	humanReply, ok := commentByID["24388"]
	require.True(t, ok)
	require.Equal(t, "shrijith", humanReply.Author.Username)
	require.False(t, humanReply.System)
	require.NotNil(t, humanReply.DiscussionID)
	require.Equal(t, "9a6fe655f27a6768e63b32799d0899f1a32a5cb9", *humanReply.DiscussionID)
	require.NotNil(t, humanReply.Position)
	require.Equal(t, "liveapi-backend/qmanager/repodag.go", humanReply.Position.FilePath)
}

func TestGitLabDiffConversionMatchesFixture(t *testing.T) {
	t.Parallel()

	raw := readMergeRequestChangesFixture(t, "gitlab-mr-changes-0001.json")
	want := readExpectedDiffsFixture(t, "gitlab-mr-diffs-0001.json")

	got := providergitlab.ConvertToCodeDiffs(raw)
	require.Equal(t, len(want.Diffs), len(got), "unexpected diff count")
	require.Equal(t, want.Diffs, got)
}

func readUnifiedEventsFixture(t *testing.T, name string) unifiedEventsFixture {
	t.Helper()

	bytes := readFixture(t, name)

	var fixture unifiedEventsFixture
	require.NoError(t, json.Unmarshal(bytes, &fixture))
	return fixture
}

func readExpectedTimeline(t *testing.T, name string) coreprocessor.UnifiedTimelineV2 {
	t.Helper()

	bytes := readFixture(t, name)

	var timeline coreprocessor.UnifiedTimelineV2
	require.NoError(t, json.Unmarshal(bytes, &timeline))
	return timeline
}

func readDiscussionsFixture(t *testing.T, name string) []GitLabV2Discussion {
	t.Helper()

	bytes := readFixture(t, name)

	var discussions []GitLabV2Discussion
	require.NoError(t, json.Unmarshal(bytes, &discussions))
	return discussions
}

func readMergeRequestChangesFixture(t *testing.T, name string) *providergitlab.GitLabMergeRequestChanges {
	t.Helper()

	bytes := readFixture(t, name)

	var changes providergitlab.GitLabMergeRequestChanges
	require.NoError(t, json.Unmarshal(bytes, &changes))
	return &changes
}

func readExpectedDiffsFixture(t *testing.T, name string) expectedDiffsFixture {
	t.Helper()

	bytes := readFixture(t, name)

	var diffs expectedDiffsFixture
	require.NoError(t, json.Unmarshal(bytes, &diffs))
	return diffs
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	require.NoErrorf(t, err, "read fixture %s", path)
	return data
}
