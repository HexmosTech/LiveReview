package api

import (
	"io"
	"net/http"
	"strings"
	"testing"

	coreprocessor "github.com/livereview/internal/core_processor"
)

func TestBitbucketProvider_GetBotUserInfo(t *testing.T) {
	server := setupBotUserTestServer(t)
	if server.bitbucketProviderV2 == nil {
		t.Fatalf("bitbucket provider not initialized")
	}

	repoFullName := "workspace/repo"
	tokenID := insertIntegrationToken(t, server.db, "bitbucket-cloud", "https://api.bitbucket.org", "pat-xyz", map[string]interface{}{"email": "bot@example.com"})
	insertBitbucketWebhook(t, server.db, repoFullName, tokenID)

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.bitbucket.org" || req.URL.Path != "/2.0/user" {
			t.Fatalf("unexpected Bitbucket request: %s", req.URL.String())
		}
		user, pass, ok := req.BasicAuth()
		if !ok || user != "bot@example.com" || pass != "pat-xyz" {
			t.Fatalf("unexpected basic auth credentials")
		}
		body := `{"uuid":"{123}","username":"bot-user","display_name":"Bot User","account_id":"acc-1","type":"bot"}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}
		return resp, nil
	})

	repo := coreprocessor.UnifiedRepositoryV2{FullName: repoFullName}
	botInfo, err := server.bitbucketProviderV2.GetBotUserInfo(repo)
	if err != nil {
		t.Fatalf("expected bot info, got error: %v", err)
	}

	if botInfo == nil {
		t.Fatalf("bot info is nil")
	}
	if botInfo.Username != "bot-user" {
		t.Fatalf("unexpected username: %s", botInfo.Username)
	}
	if !botInfo.IsBot {
		t.Fatalf("expected IsBot true")
	}
	if uuid, ok := botInfo.Metadata["uuid"].(string); !ok || uuid != "{123}" {
		t.Fatalf("unexpected uuid metadata: %v", botInfo.Metadata["uuid"])
	}
	if account, ok := botInfo.Metadata["account_id"].(string); !ok || account != "acc-1" {
		t.Fatalf("unexpected account metadata: %v", botInfo.Metadata["account_id"])
	}
}

func TestBitbucketProvider_GetBotUserInfo_MissingEmail(t *testing.T) {
	server := setupBotUserTestServer(t)
	if server.bitbucketProviderV2 == nil {
		t.Fatalf("bitbucket provider not initialized")
	}

	repoFullName := "workspace/repo-no-email"
	tokenID := insertIntegrationToken(t, server.db, "bitbucket-cloud", "https://api.bitbucket.org", "pat-missing", map[string]interface{}{"other": "value"})
	insertBitbucketWebhook(t, server.db, repoFullName, tokenID)

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		t.Fatalf("HTTP request should not be issued when email metadata is missing")
		return nil, nil
	})

	repo := coreprocessor.UnifiedRepositoryV2{FullName: repoFullName}
	if _, err := server.bitbucketProviderV2.GetBotUserInfo(repo); err == nil {
		t.Fatalf("expected error when email metadata is missing")
	}
}
