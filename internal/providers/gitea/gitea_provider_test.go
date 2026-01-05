package gitea

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/livereview/pkg/models"
)

func TestPostCommentInline(t *testing.T) {
	var capturedInlinePayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/repos/owner/repo/pulls/42":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"head":{"sha":"headsha"},"base":{"sha":"basesha"}}`)
		case "POST /api/v1/repos/owner/repo/pulls/42/comments":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &capturedInlinePayload)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "t"})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "owner/repo/42", &models.ReviewComment{
		FilePath: "file.go",
		Line:     10,
		Content:  "c",
	})
	require.NoError(t, err)

	require.Equal(t, "file.go", capturedInlinePayload["path"])
	require.Equal(t, float64(10), capturedInlinePayload["line"])
	require.Equal(t, "RIGHT", capturedInlinePayload["side"])
	require.Equal(t, "headsha", capturedInlinePayload["commit_id"])
}

func TestPostCommentInlineDeletedLineUsesLeftSide(t *testing.T) {
	var capturedInlinePayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/repos/owner/repo/pulls/42":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"head":{"sha":"headsha"},"base":{"sha":"basesha"}}`)
		case "POST /api/v1/repos/owner/repo/pulls/42/comments":
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &capturedInlinePayload)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "t"})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "owner/repo/42", &models.ReviewComment{
		FilePath:      "file.go",
		Line:          5,
		Content:       "c",
		IsDeletedLine: true,
	})
	require.NoError(t, err)

	require.Equal(t, "LEFT", capturedInlinePayload["side"])
}

func TestPostCommentGeneral(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/repos/owner/repo/issues/42/comments":
			called = true
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: "t"})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "owner/repo/42", &models.ReviewComment{Content: "hi"})
	require.NoError(t, err)
	require.True(t, called)
}
