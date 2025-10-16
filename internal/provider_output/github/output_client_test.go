package github

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	coreprocessor "github.com/livereview/internal/core_processor"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func newBaseEvent() *coreprocessor.UnifiedWebhookEventV2 {
	return &coreprocessor.UnifiedWebhookEventV2{
		Provider: "github",
		Repository: coreprocessor.UnifiedRepositoryV2{
			FullName: "owner/repo",
		},
		MergeRequest: &coreprocessor.UnifiedMergeRequestV2{
			Number: 42,
		},
		Comment: &coreprocessor.UnifiedCommentV2{
			ID:   "123",
			Body: "@livereviewbot please help",
		},
	}
}

func makeSuccessResponse() *http.Response {
	return &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
		Header:     make(http.Header),
	}
}

func TestPostCommentReply_IssueCommentUsesIssueEndpoint(t *testing.T) {
	client := NewAPIClient()
	event := newBaseEvent()

	var capturedURL string
	var capturedBody map[string]interface{}

	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		capturedURL = req.URL.String()
		payload, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(payload, &capturedBody)
		return makeSuccessResponse()
	})}

	require.NoError(t, client.PostCommentReply(event, "token", "hello"))
	require.Equal(t, "https://api.github.com/repos/owner/repo/issues/42/comments", capturedURL)
	require.Equal(t, "hello", capturedBody["body"])
	require.NotContains(t, capturedBody, "in_reply_to")
}

func TestPostCommentReply_ReviewCommentTopLevelUsesRepliesEndpoint(t *testing.T) {
	client := NewAPIClient()
	event := newBaseEvent()
	event.Comment.Position = &coreprocessor.UnifiedPositionV2{FilePath: "foo.go", LineNumber: 10}

	var capturedURL string
	var capturedBody map[string]interface{}

	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		capturedURL = req.URL.String()
		payload, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(payload, &capturedBody)
		return makeSuccessResponse()
	})}

	require.NoError(t, client.PostCommentReply(event, "token", "hello"))
	require.Equal(t, "https://api.github.com/repos/owner/repo/pulls/42/comments", capturedURL)
	require.Equal(t, float64(123), capturedBody["in_reply_to"])
	require.Equal(t, "hello", capturedBody["body"])
}

func TestPostCommentReply_ReviewThreadReplyUsesInReplyTo(t *testing.T) {
	client := NewAPIClient()
	event := newBaseEvent()
	event.Comment.Position = &coreprocessor.UnifiedPositionV2{FilePath: "foo.go", LineNumber: 10}
	replyTo := "456"
	event.Comment.InReplyToID = &replyTo

	var capturedURL string
	var capturedBody map[string]interface{}

	client.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) *http.Response {
		capturedURL = req.URL.String()
		payload, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(payload, &capturedBody)
		return makeSuccessResponse()
	})}

	require.NoError(t, client.PostCommentReply(event, "token", "hello"))
	require.Equal(t, "https://api.github.com/repos/owner/repo/pulls/42/comments", capturedURL)
	require.Equal(t, float64(456), capturedBody["in_reply_to"])
	require.Equal(t, "hello", capturedBody["body"])
}
