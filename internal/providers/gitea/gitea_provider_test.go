package gitea

import (
	"context"
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
		case "GET /user/login":
			io.WriteString(w, `<input name="_csrf" value="csrf123">`)
		case "POST /user/login":
			w.Header().Add("Set-Cookie", "gitea_incredible=abc; Path=/")
			w.Header().Add("Set-Cookie", "_csrf=csrfCookie; Path=/")
			w.WriteHeader(http.StatusFound)
		case "GET /api/v1/repos/owner/repo/pulls/42":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"head":{"sha":"headsha"},"base":{"sha":"basesha"}}`)
		case "POST /owner/repo/pulls/42/files/reviews/comments":
			_ = r.ParseForm()
			capturedInlinePayload = map[string]interface{}{
				"path":      r.FormValue("path"),
				"line":      r.FormValue("line"),
				"side":      r.FormValue("side"),
				"commit_id": r.FormValue("latest_commit_id"),
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: `{"pat":"t","username":"u","password":"pw"}`})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "owner/repo/42", &models.ReviewComment{
		FilePath: "file.go",
		Line:     10,
		Content:  "c",
	})
	require.NoError(t, err)

	require.Equal(t, "file.go", capturedInlinePayload["path"])
	require.Equal(t, "10", capturedInlinePayload["line"])
	require.Equal(t, "proposed", capturedInlinePayload["side"])
	require.Equal(t, "headsha", capturedInlinePayload["commit_id"])
}

func TestPostCommentInlineDeletedLineUsesLeftSide(t *testing.T) {
	var capturedInlinePayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /user/login":
			io.WriteString(w, `<input name="_csrf" value="csrf123">`)
		case "POST /user/login":
			w.Header().Add("Set-Cookie", "gitea_incredible=abc; Path=/")
			w.Header().Add("Set-Cookie", "_csrf=csrfCookie; Path=/")
			w.WriteHeader(http.StatusFound)
		case "GET /api/v1/repos/owner/repo/pulls/42":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"head":{"sha":"headsha"},"base":{"sha":"basesha"}}`)
		case "POST /owner/repo/pulls/42/files/reviews/comments":
			_ = r.ParseForm()
			capturedInlinePayload = map[string]interface{}{
				"path":      r.FormValue("path"),
				"line":      r.FormValue("line"),
				"side":      r.FormValue("side"),
				"commit_id": r.FormValue("latest_commit_id"),
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: `{"pat":"t","username":"u","password":"pw"}`})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "owner/repo/42", &models.ReviewComment{
		FilePath:      "file.go",
		Line:          5,
		Content:       "c",
		IsDeletedLine: true,
	})
	require.NoError(t, err)

	require.Equal(t, "previous", capturedInlinePayload["side"])
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

// Fallback path: API returns 404, provider uses session login to post inline.
func TestPostCommentInlineSessionFallback(t *testing.T) {
	loginServed := false
	var capturedSide string
	var capturedCSRF string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /user/login":
			loginServed = true
			io.WriteString(w, `<html><input name="_csrf" value="csrf123">`)
		case "POST /user/login":
			w.Header().Add("Set-Cookie", "gitea_incredible=abc; Path=/")
			w.Header().Add("Set-Cookie", "_csrf=csrfCookie; Path=/")
			w.WriteHeader(http.StatusFound)
		case "GET /api/v1/repos/owner/repo/pulls/42":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"head":{"sha":"headsha"}}`)
		case "POST /api/v1/repos/owner/repo/pulls/42/comments":
			w.WriteHeader(http.StatusNotFound) // force fallback
		case "POST /owner/repo/pulls/42/files/reviews/comments":
			_ = r.ParseForm()
			capturedSide = r.FormValue("side")
			capturedCSRF = r.FormValue("_csrf")
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	p, err := NewProvider(Config{BaseURL: server.URL, Token: `{"pat":"p","username":"u","password":"pw"}`})
	require.NoError(t, err)

	err = p.PostComment(context.Background(), "owner/repo/42", &models.ReviewComment{
		FilePath:      "file.go",
		Line:          10,
		Content:       "c",
		IsDeletedLine: true,
	})
	require.NoError(t, err)
	require.True(t, loginServed)
	require.Equal(t, "previous", capturedSide) // deleted -> previous side
	require.Equal(t, "csrfCookie", capturedCSRF)
}
