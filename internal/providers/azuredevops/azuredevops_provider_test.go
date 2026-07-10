package azuredevops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/livereview/pkg/models"
)

func TestParsePullRequestURL(t *testing.T) {
	org, project, repo, id, err := parsePullRequestURL("https://dev.azure.com/myorg/My%20Project/_git/myrepo/pullrequest/42")
	require.NoError(t, err)
	require.Equal(t, "myorg", org)
	require.Equal(t, "My Project", project)
	require.Equal(t, "myrepo", repo)
	require.Equal(t, 42, id)
}

func TestParsePullRequestURL_InvalidShape(t *testing.T) {
	_, _, _, _, err := parsePullRequestURL("https://dev.azure.com/myorg/myproject/myrepo/pullrequest/42")
	require.Error(t, err)
}

func TestMergeRequestIDRoundTrip(t *testing.T) {
	mrID := "myorg/My Project/myrepo/42"
	org, project, repo, id, err := splitMergeRequestID(mrID)
	require.NoError(t, err)
	require.Equal(t, "myorg", org)
	require.Equal(t, "My Project", project)
	require.Equal(t, "myrepo", repo)
	require.Equal(t, 42, id)
}

func TestSplitMergeRequestID_InvalidFormat(t *testing.T) {
	_, _, _, _, err := splitMergeRequestID("owner/repo/42")
	require.Error(t, err)
}

func TestGetMergeRequestDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /myproject/_apis/git/repositories/myrepo/pullRequests/7":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(pullRequest{
				PullRequestID:         7,
				Status:                "active",
				Title:                 "My PR",
				Description:           "desc",
				SourceRefName:         "refs/heads/feature",
				TargetRefName:         "refs/heads/main",
				CreatedBy:             identity{DisplayName: "Jane Doe", UniqueName: "jane@example.com"},
				LastMergeSourceCommit: commitRef{CommitID: "headsha"},
				LastMergeTargetCommit: commitRef{CommitID: "basesha"},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "pat"})
	require.NoError(t, err)

	mrURL := server.URL + "/myorg/myproject/_git/myrepo/pullrequest/7"
	details, err := p.GetMergeRequestDetails(context.Background(), mrURL)
	require.NoError(t, err)
	require.Equal(t, "myorg/myproject/myrepo/7", details.ID)
	require.Equal(t, "My PR", details.Title)
	require.Equal(t, "feature", details.SourceBranch)
	require.Equal(t, "main", details.TargetBranch)
	require.Equal(t, "headsha", details.DiffRefs.HeadSHA)
	require.Equal(t, "basesha", details.DiffRefs.BaseSHA)
	require.Equal(t, "azuredevops", details.ProviderType)
}

func TestPostCommentGeneral(t *testing.T) {
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /myproject/_apis/git/repositories/myrepo/pullRequests/7/threads":
			require.Contains(t, r.Header.Get("Authorization"), "Basic ")
			_ = json.NewDecoder(r.Body).Decode(&capturedBody)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: ""})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "myorg/myproject/myrepo/7", &models.ReviewComment{Content: "hello"})
	require.NoError(t, err)

	require.Contains(t, capturedBody, "comments")
	require.NotContains(t, capturedBody, "threadContext")
}

func TestPostCommentInline(t *testing.T) {
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "pat"})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "myorg/myproject/myrepo/7", &models.ReviewComment{
		FilePath: "src/foo.go",
		Line:     12,
		Content:  "inline comment",
	})
	require.NoError(t, err)

	threadContext, ok := capturedBody["threadContext"].(map[string]interface{})
	require.True(t, ok, "expected threadContext for inline comment")
	require.Equal(t, "/src/foo.go", threadContext["filePath"])
	require.Contains(t, threadContext, "rightFileStart")
	require.NotContains(t, threadContext, "leftFileStart")
}

func TestPostCommentInlineDeletedLineUsesLeftSide(t *testing.T) {
	var capturedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&capturedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "pat"})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "myorg/myproject/myrepo/7", &models.ReviewComment{
		FilePath:      "src/foo.go",
		Line:          5,
		Content:       "deleted line comment",
		IsDeletedLine: true,
	})
	require.NoError(t, err)

	threadContext := capturedBody["threadContext"].(map[string]interface{})
	require.Contains(t, threadContext, "leftFileStart")
	require.NotContains(t, threadContext, "rightFileStart")
}

func TestBuildCodeDiff(t *testing.T) {
	entry := changeEntry{
		Item:       changeItem{Path: "/src/foo.go"},
		ChangeType: "edit",
	}
	oldContent := "line1\nline2\nline3\n"
	newContent := "line1\nline2 changed\nline3\n"

	diff := buildCodeDiff(entry, oldContent, newContent)
	require.Equal(t, "src/foo.go", diff.FilePath)
	require.False(t, diff.IsNew)
	require.False(t, diff.IsDeleted)
	require.Len(t, diff.Hunks, 1)
	require.Contains(t, diff.Hunks[0].Content, "-line2")
	require.Contains(t, diff.Hunks[0].Content, "+line2 changed")
}

func TestBuildCodeDiff_AddedFile(t *testing.T) {
	entry := changeEntry{
		Item:       changeItem{Path: "/src/new.go"},
		ChangeType: "add",
	}
	diff := buildCodeDiff(entry, "", "package main\n")
	require.True(t, diff.IsNew)
	require.Len(t, diff.Hunks, 1)
	require.Contains(t, diff.Hunks[0].Content, "+package main")
}

func TestIsEmptyObjectID(t *testing.T) {
	require.True(t, isEmptyObjectID(""))
	require.True(t, isEmptyObjectID("0000000000000000000000000000000000000000"))
	require.False(t, isEmptyObjectID("abc123"))
}

// TestFetchBlobRequestsOctetStreamFormat locks in the fix for a bug where
// blob fetches returned Azure DevOps's GitBlobRef JSON metadata
// ({objectId, size, url, _links}) instead of the file's raw text content,
// because $format was never set and the Accept header was clobbered by
// applyAuth (which always sets Accept: application/json). Without
// $format=octetstream in the query string, Azure DevOps ignores the (dead)
// Accept header and returns JSON regardless.
func TestFetchBlobRequestsOctetStreamFormat(t *testing.T) {
	const rawContent = "package main\n\nfunc main() {}\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "octetstream", r.URL.Query().Get("$format"))
		_, _ = w.Write([]byte(rawContent))
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "pat"})
	require.NoError(t, err)

	content, err := p.fetchBlob(context.Background(), server.URL, "myproject", "myrepo", "abc123")
	require.NoError(t, err)
	require.Equal(t, rawContent, content)
}
