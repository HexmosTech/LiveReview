package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

// fakeJobQueue captures QueueRepoPRSyncJob calls without needing a real River
// queue, so syncRepositoriesAndQueueBackfill can be tested in isolation.
type fakeJobQueue struct {
	backfillRepositoryIDs []int64
}

func (f *fakeJobQueue) QueueWebhookInstallJob(ctx context.Context, connectorID int, projectPath, provider, baseURL, pat string) error {
	return nil
}

func (f *fakeJobQueue) QueueRepoPRSyncJob(ctx context.Context, repositoryID int64, initialBackfill bool) error {
	if initialBackfill {
		f.backfillRepositoryIDs = append(f.backfillRepositoryIDs, repositoryID)
	}
	return nil
}

func TestSyncRepositoriesAndQueueBackfill_GitHub(t *testing.T) {
	server := setupBotUserTestServer(t)
	tokenID := insertIntegrationToken(t, server.db, "github", "https://github.com", "pat-gh", nil)

	stubHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.github.com" || req.URL.Path != "/user/repos" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		body := `[{"id":501,"name":"repo-a","full_name":"acme/repo-a","html_url":"https://github.com/acme/repo-a","default_branch":"main","private":true}]`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	var connector ConnectorDetails
	if err := server.db.QueryRow(
		`SELECT id, org_id, provider, provider_url, pat_token FROM integration_tokens WHERE id = $1`, tokenID,
	).Scan(&connector.ID, &connector.OrgID, &connector.Provider, &connector.ProviderURL, &connector.PATToken); err != nil {
		t.Fatalf("failed to load connector: %v", err)
	}

	fq := &fakeJobQueue{}
	awi := &AutoWebhookInstaller{db: server.db, jobQueue: fq}

	if err := awi.syncRepositoriesAndQueueBackfill(context.Background(), int(tokenID), &connector); err != nil {
		t.Fatalf("syncRepositoriesAndQueueBackfill: %v", err)
	}

	var count int
	if err := server.db.QueryRow(`SELECT count(*) FROM repositories WHERE connector_id = $1`, tokenID).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 repository row, got %d", count)
	}
	t.Cleanup(func() {
		_, _ = server.db.Exec(`DELETE FROM repositories WHERE connector_id = $1`, tokenID)
	})

	if len(fq.backfillRepositoryIDs) != 1 {
		t.Fatalf("expected exactly 1 initial backfill job queued, got %d", len(fq.backfillRepositoryIDs))
	}

	var fullName string
	var isPrivate bool
	if err := server.db.QueryRow(`SELECT full_name, is_private FROM repositories WHERE id = $1`, fq.backfillRepositoryIDs[0]).
		Scan(&fullName, &isPrivate); err != nil {
		t.Fatalf("select repository: %v", err)
	}
	if fullName != "acme/repo-a" {
		t.Errorf("expected full_name acme/repo-a, got %s", fullName)
	}
	if !isPrivate {
		t.Errorf("expected repository to be private")
	}
}
