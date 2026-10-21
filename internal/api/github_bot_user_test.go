package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	coreprocessor "github.com/livereview/internal/core_processor"
)

func TestGitHubProvider_GetBotUserInfo(t *testing.T) {
	server := setupBotUserTestServer(t)
	if server.githubProviderV2 == nil {
		t.Fatalf("github provider not initialized")
	}

	connectorID := insertIntegrationToken(t, server.db, "github", "https://api.github.com", "pat-gh", map[string]interface{}{"installation_id": 42})

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.github.com" || req.URL.Path != "/user" {
			t.Fatalf("unexpected GitHub request: %s", req.URL.String())
		}
		if auth := req.Header.Get("Authorization"); auth != "token pat-gh" && auth != "Bearer pat-gh" {
			t.Fatalf("unexpected Authorization header: %s", auth)
		}
		body := `{"id":1001,"login":"livereview-bot","name":"LiveReview Bot","type":"Bot","avatar_url":"https://example.com/avatar.png","html_url":"https://github.com/livereview-bot"}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	// The webhook orchestrator stamps the receiving connector on the event; the token is resolved from it.
	repo := coreprocessor.UnifiedRepositoryV2{FullName: "owner/repo", Metadata: map[string]interface{}{"connector_id": connectorID}}
	info, err := server.githubProviderV2.GetBotUserInfo(repo)
	if err != nil {
		t.Fatalf("expected bot info, got error: %v", err)
	}

	if info.Username != "livereview-bot" {
		t.Fatalf("unexpected username: %s", info.Username)
	}
	if !info.IsBot {
		t.Fatalf("expected IsBot true")
	}
	if url, ok := info.Metadata["html_url"].(string); !ok || url != "https://github.com/livereview-bot" {
		t.Fatalf("unexpected metadata html_url: %v", info.Metadata["html_url"])
	}
}

func TestGitHubProvider_GetBotUserInfo_HTTPError(t *testing.T) {
	server := setupBotUserTestServer(t)
	if server.githubProviderV2 == nil {
		t.Fatalf("github provider not initialized")
	}

	connectorID := insertIntegrationToken(t, server.db, "github", "https://api.github.com", "pat-gh-error", nil)

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("boom")),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	repo := coreprocessor.UnifiedRepositoryV2{FullName: "owner/repo", Metadata: map[string]interface{}{"connector_id": connectorID}}
	if _, err := server.githubProviderV2.GetBotUserInfo(repo); err == nil {
		t.Fatalf("expected error when GitHub API responds with failure")
	}
}

// Without a connector on the event there is no token to use: never fall back to some other tenant's.
func TestGitHubProvider_GetBotUserInfo_RequiresConnector(t *testing.T) {
	server := setupBotUserTestServer(t)
	insertIntegrationToken(t, server.db, "github", "https://api.github.com", "pat-gh-other", nil)

	if _, err := server.githubProviderV2.GetBotUserInfo(coreprocessor.UnifiedRepositoryV2{FullName: "owner/repo"}); err == nil {
		t.Fatalf("expected an error when the event carries no connector")
	}
}
