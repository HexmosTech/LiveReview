package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	coreprocessor "github.com/livereview/internal/core_processor"
)

func TestGitLabProvider_GetBotUserInfo(t *testing.T) {
	server := setupBotUserTestServer(t)
	if server.gitlabProviderV2 == nil {
		t.Fatalf("gitlab provider not initialized")
	}

	insertIntegrationToken(t, server.db, "gitlab", "https://gitlab.test", "gitlab-token", nil)

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "gitlab.test" || req.URL.Path != "/api/v4/user" {
			t.Fatalf("unexpected GitLab request: %s", req.URL.String())
		}
		if auth := req.Header.Get("Authorization"); auth != "Bearer gitlab-token" {
			t.Fatalf("unexpected Authorization header: %s", auth)
		}
		body := `{"id":55,"username":"lr-bot","name":"Live Bot","state":"active","avatar_url":"https://gitlab.test/avatar.png","web_url":"https://gitlab.test/u/lr-bot"}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	repo := coreprocessor.UnifiedRepositoryV2{WebURL: "https://gitlab.test/my-group/my-project"}
	info, err := server.gitlabProviderV2.GetBotUserInfo(repo)
	if err != nil {
		t.Fatalf("expected bot info, got error: %v", err)
	}

	if info.UserID != "55" {
		t.Fatalf("unexpected user id: %s", info.UserID)
	}
	if avatar, ok := info.Metadata["avatar_url"].(string); !ok || avatar != "https://gitlab.test/avatar.png" {
		t.Fatalf("unexpected avatar metadata: %v", info.Metadata["avatar_url"])
	}
	if !info.IsBot {
		t.Fatalf("expected IsBot true")
	}
}

func TestGitLabProvider_GetBotUserInfo_HTTPError(t *testing.T) {
	server := setupBotUserTestServer(t)
	if server.gitlabProviderV2 == nil {
		t.Fatalf("gitlab provider not initialized")
	}

	insertIntegrationToken(t, server.db, "gitlab", "https://gitlab.test", "gitlab-token-error", nil)

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("boom")),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	repo := coreprocessor.UnifiedRepositoryV2{WebURL: "https://gitlab.test/my-group/my-project"}
	if _, err := server.gitlabProviderV2.GetBotUserInfo(repo); err == nil {
		t.Fatalf("expected error when GitLab API responds with failure")
	}
}
