package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livereview/internal/database"
	storagegithub "github.com/livereview/storage/providers/github"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func testOrg(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'githubapp test') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM integration_tokens WHERE org_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, id)
	})
	return id
}

// ghes returns a Config whose GitHub host is a test server (non-github.com hosts use <base>/api/v3).
func ghes(t *testing.T, handler http.HandlerFunc) (*Config, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &Config{AppID: 99, Slug: "lr-test", BaseURL: srv.URL, PrivateKey: key}, srv
}

func uniqueInstallation() int64 { return time.Now().UnixNano() % 1_000_000_000_000 }

func TestUpsertAppInstallationConnector_OneOrgPerInstallation(t *testing.T) {
	db := testDB(t)
	store := storagegithub.NewTokenStore(db)
	suffix := time.Now().UnixNano()
	orgA := testOrg(t, db, fmt.Sprintf("ghapp-a-%d", suffix))
	orgB := testOrg(t, db, fmt.Sprintf("ghapp-b-%d", suffix))
	inst := uniqueInstallation()
	in := storagegithub.AppConnectorUpsert{
		OrgID: orgA, BaseURL: "https://ghes.test", InstallationID: inst, AccountLogin: "acme", AccountType: "Organization",
		RepositorySelection: "selected", AppSlug: "lr-test", Token: "ghs_one", ExpiresAt: time.Now().Add(time.Hour),
	}

	id, created, err := store.UpsertAppInstallationConnector(in)
	if err != nil || !created {
		t.Fatalf("first upsert = (%d, %v, %v), want created", id, created, err)
	}
	in.Token = "ghs_two"
	id2, created, err := store.UpsertAppInstallationConnector(in)
	if err != nil || created || id2 != id {
		t.Fatalf("second upsert = (%d, %v, %v), want same row refreshed", id2, created, err)
	}
	rec, err := store.GetGitHubTokenByConnectorID(id)
	if err != nil || rec.PatToken != "ghs_two" || rec.TokenType != storagegithub.TokenTypeGitHubApp || rec.OrgID != orgA {
		t.Fatalf("stored row wrong: %+v err=%v", rec, err)
	}

	in.OrgID = orgB
	if _, _, err := store.UpsertAppInstallationConnector(in); err != storagegithub.ErrInstallationBoundToOtherOrg {
		t.Fatalf("foreign org upsert err = %v, want ErrInstallationBoundToOtherOrg", err)
	}
	if rec, _ = store.GetGitHubTokenByConnectorID(id); rec.OrgID != orgA {
		t.Fatalf("installation moved to org %d", rec.OrgID)
	}
}

func TestConnectorToken_CachesThenRefreshes(t *testing.T) {
	db := testDB(t)
	store := storagegithub.NewTokenStore(db)
	org := testOrg(t, db, fmt.Sprintf("ghapp-tok-%d", time.Now().UnixNano()))

	var mints int32
	cfg, srv := ghes(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&mints, 1)
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"token":"ghs_minted_%d","expires_at":%q}`, atomic.LoadInt32(&mints), time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	})
	inst := uniqueInstallation()
	id, _, err := store.UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: org, BaseURL: srv.URL, InstallationID: inst, AccountLogin: "acme", AccountType: "User",
		Token: "ghs_cached", ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := store.GetGitHubTokenByConnectorID(id)

	got, err := connectorToken(context.Background(), cfg, store, rec, false)
	if err != nil || got != "ghs_cached" || atomic.LoadInt32(&mints) != 0 {
		t.Fatalf("valid cached token: got (%q, %v) mints=%d, want cached with no mint", got, err, mints)
	}

	// Expiring within the margin => replaced and persisted.
	rec.ExpiresAt = sql.NullTime{Time: time.Now().Add(2 * time.Minute), Valid: true}
	got, err = connectorToken(context.Background(), cfg, store, rec, false)
	if err != nil || got != "ghs_minted_1" {
		t.Fatalf("expiring token: got (%q, %v), want ghs_minted_1", got, err)
	}
	if stored, _ := store.GetGitHubTokenByConnectorID(id); stored.PatToken != "ghs_minted_1" {
		t.Fatalf("minted token not persisted: %q", stored.PatToken)
	}
}

func TestConnectorToken_RemovedInstallationAndGone404(t *testing.T) {
	db := testDB(t)
	store := storagegithub.NewTokenStore(db)
	org := testOrg(t, db, fmt.Sprintf("ghapp-gone-%d", time.Now().UnixNano()))
	cfg, srv := ghes(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound) })

	id, _, err := store.UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: org, BaseURL: srv.URL, InstallationID: uniqueInstallation(), AccountLogin: "acme", AccountType: "User",
		Token: "old", ExpiresAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := store.GetGitHubTokenByConnectorID(id)
	if _, err := connectorToken(context.Background(), cfg, store, rec, false); err != ErrInstallationRemoved {
		t.Fatalf("err = %v, want ErrInstallationRemoved", err)
	}
	stored, _ := store.GetGitHubTokenByConnectorID(id)
	if stored.PatToken != "" {
		t.Errorf("token for a removed installation was not cleared: %q", stored.PatToken)
	}
	if _, err := connectorToken(context.Background(), cfg, store, stored, false); err != ErrInstallationRemoved {
		t.Fatalf("second call err = %v, want ErrInstallationRemoved without hitting GitHub", err)
	}
}

func TestConnectorToken_PATPassesThroughAndNilConfigFails(t *testing.T) {
	if got, err := ConnectorToken(context.Background(), nil, &storagegithub.IntegrationTokenRecord{TokenType: "PAT", PatToken: "pat"}); err != nil || got != "pat" {
		t.Fatalf("PAT: got (%q, %v)", got, err)
	}
	if _, err := connectorToken(context.Background(), nil, nil, &storagegithub.IntegrationTokenRecord{TokenType: storagegithub.TokenTypeGitHubApp}, false); err != ErrNotConfigured {
		t.Fatalf("err = %v, want ErrNotConfigured", err)
	}
}

// Code that already read a connector row (workers, diff review) hands over the stored token: non-App rows must
// get it back untouched, App rows must get a valid installation token even if the stored one is stale.
func TestResolveIfApp_PassesThroughNonAppAndResolvesApp(t *testing.T) {
	db := testDB(t)

	if got, err := ResolveIfApp(context.Background(), db, 0, "PAT", "ghp_stored"); err != nil || got != "ghp_stored" {
		t.Fatalf("PAT row: got %q, %v; want the stored token untouched", got, err)
	}
	if got, err := ResolveIfApp(context.Background(), db, 0, "", "oauth_stored"); err != nil || got != "oauth_stored" {
		t.Fatalf("untyped row: got %q, %v; want the stored token untouched", got, err)
	}

	org := testOrg(t, db, fmt.Sprintf("ghapp-resolve-%d", time.Now().UnixNano()))
	store := storagegithub.NewTokenStore(db)
	connectorID, _, err := store.UpsertAppInstallationConnector(storagegithub.AppConnectorUpsert{
		OrgID: org, BaseURL: "https://github.com", InstallationID: uniqueInstallation(), AccountLogin: "acme", AccountType: "Organization",
		RepositorySelection: "selected", AppSlug: "lr-test", Token: "ghs_cached", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	// A still-valid cached token is returned as is, with no App credentials needed.
	if got, err := ResolveIfApp(context.Background(), db, connectorID, "GITHUB_APP", "ignored-stored-value"); err != nil || got != "ghs_cached" {
		t.Fatalf("App row: got %q, %v; want the cached installation token", got, err)
	}
}
