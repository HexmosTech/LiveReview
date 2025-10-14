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
