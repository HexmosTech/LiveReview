package githubapp

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
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

func TestUpsertAppInstallationConnector_OneConnectorPerOrg(t *testing.T) {
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

	// Another org gets its own connector for the same installation; the first one is untouched.
	in.OrgID, in.Token = orgB, "ghs_b"
	idB, created, err := store.UpsertAppInstallationConnector(in)
	if err != nil || !created || idB == id {
		t.Fatalf("other org upsert = (%d, %v, %v), want a new connector", idB, created, err)
	}
	if rec, _ = store.GetGitHubTokenByConnectorID(id); rec.OrgID != orgA || rec.PatToken != "ghs_two" {
		t.Fatalf("first org's connector changed: %+v", rec)
	}
	recs, err := store.ListAppConnectorsByInstallation("https://ghes.test", inst)
	if err != nil || len(recs) != 2 || recs[0].ID != id || recs[1].ID != idB {
		t.Fatalf("ListAppConnectorsByInstallation = %v, %v; want [%d %d]", recs, err, id, idB)
	}
	if n, err := store.CountOtherOrgsUsingInstallation("https://ghes.test", inst, orgA); err != nil || n != 1 {
		t.Fatalf("CountOtherOrgsUsingInstallation = %d, %v; want 1", n, err)
	}
}

func scopeOf(t *testing.T, store *storagegithub.TokenStore, id int64) (scope string, allowed []int64, removed bool) {
	t.Helper()
	rec, err := store.GetGitHubTokenByConnectorID(id)
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		RepoScope      string  `json:"repo_scope"`
		AllowedRepoIDs []int64 `json:"allowed_repo_ids"`
		Removed        bool    `json:"removed"`
	}
	if err := json.Unmarshal(rec.Metadata, &meta); err != nil {
		t.Fatal(err)
	}
	return meta.RepoScope, meta.AllowedRepoIDs, meta.Removed
}

func TestUpsertAppInstallationConnector_RepoScope(t *testing.T) {
	db := testDB(t)
	store := storagegithub.NewTokenStore(db)
	org := testOrg(t, db, fmt.Sprintf("ghapp-scope-%d", time.Now().UnixNano()))
	in := storagegithub.AppConnectorUpsert{
		OrgID: org, BaseURL: "https://ghes.test", InstallationID: uniqueInstallation(), AccountLogin: "acme",
		AccountType: "Organization", Token: "t", ExpiresAt: time.Now().Add(time.Hour),
		RepoScope: storagegithub.RepoScopeUser, AllowedRepoIDs: []int64{11, 22}, KeepDisconnected: true,
	}
	id, _, err := store.UpsertAppInstallationConnector(in)
	if err != nil {
		t.Fatal(err)
	}
	if scope, allowed, _ := scopeOf(t, store, id); scope != storagegithub.RepoScopeUser || len(allowed) != 2 {
		t.Fatalf("scope = %q %v, want user [11 22]", scope, allowed)
	}

	// A later sign-in refreshes the list.
	in.AllowedRepoIDs = []int64{22}
	if _, _, err := store.UpsertAppInstallationConnector(in); err != nil {
		t.Fatal(err)
	}
	if _, allowed, _ := scopeOf(t, store, id); len(allowed) != 1 || allowed[0] != 22 {
		t.Fatalf("allowed = %v, want [22]", allowed)
	}

	// Installing/configuring widens it to installer; a plain sign-in afterwards doesn't narrow it again.
	installer := in
	installer.RepoScope, installer.AllowedRepoIDs, installer.KeepDisconnected = storagegithub.RepoScopeInstaller, nil, false
	if _, _, err := store.UpsertAppInstallationConnector(installer); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertAppInstallationConnector(in); err != nil {
		t.Fatal(err)
	}
	if scope, allowed, _ := scopeOf(t, store, id); scope != storagegithub.RepoScopeInstaller || allowed != nil {
		t.Fatalf("scope = %q %v, want installer with no allow list", scope, allowed)
	}

	// Disconnected by the org: plain sign-ins leave it alone, installing again reconnects it.
	if err := store.DisconnectAppConnector(id); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.UpsertAppInstallationConnector(in); err != storagegithub.ErrAppConnectorDisconnected {
		t.Fatalf("plain sign-in after disconnect: err = %v, want ErrAppConnectorDisconnected", err)
	}
	if _, _, removed := scopeOf(t, store, id); !removed {
		t.Fatal("a plain sign-in reconnected a disconnected installation")
	}
	if _, _, err := store.UpsertAppInstallationConnector(installer); err != nil {
		t.Fatal(err)
	}
	if _, _, removed := scopeOf(t, store, id); removed {
		t.Fatal("installing again did not reconnect it")
	}
}

func TestUpsertAppInstallationConnector_OfferIfShared(t *testing.T) {
	db := testDB(t)
	store := storagegithub.NewTokenStore(db)
	suffix := time.Now().UnixNano()
	orgA := testOrg(t, db, fmt.Sprintf("ghapp-offer-a-%d", suffix))
	orgB := testOrg(t, db, fmt.Sprintf("ghapp-offer-b-%d", suffix))
	inst := uniqueInstallation()
	plain := storagegithub.AppConnectorUpsert{
		OrgID: orgA, BaseURL: "https://ghes.test", InstallationID: inst, AccountLogin: "acme", AccountType: "Organization",
		Token: "ghs_a", ExpiresAt: time.Now().Add(time.Hour), KeepDisconnected: true, OfferIfShared: true,
	}

	// The first org to see the installation gets it connected, even on a plain sign-in.
	idA, _, err := store.UpsertAppInstallationConnector(plain)
	if err != nil {
		t.Fatalf("first org: err = %v, want connected", err)
	}

	// Another org's plain sign-in is only offered it: no token, nothing live.
	plain.OrgID, plain.Token, plain.RepoScope, plain.AllowedRepoIDs = orgB, "ghs_b", storagegithub.RepoScopeUser, []int64{5}
	idB, created, err := store.UpsertAppInstallationConnector(plain)
	if !errors.Is(err, storagegithub.ErrAppConnectorOffered) || !created {
		t.Fatalf("second org: (created %v, err %v), want a new offered connector", created, err)
	}
	if rec, _ := store.GetGitHubTokenByConnectorID(idB); rec.PatToken != "" || rec.ExpiresAt.Valid {
		t.Fatalf("offered connector stored a token: %+v", rec)
	}
	if live, _ := store.ListAppConnectorsForOrg(orgB); len(live) != 0 {
		t.Fatalf("offered connector is live: %v", live)
	}
	if avail, err := store.ListNotConnectedAppConnectorsForOrg(orgB); err != nil || len(avail) != 1 || avail[0].ID != idB {
		t.Fatalf("ListNotConnectedAppConnectorsForOrg = %v, %v; want [%d]", avail, err, idB)
	}
	if n, _ := store.CountOtherOrgsUsingInstallation("https://ghes.test", inst, orgA); n != 0 {
		t.Fatalf("an offered connector counts as using the installation: %d", n)
	}

	// Later sign-ins keep it offered but refresh its scope.
	plain.AllowedRepoIDs = []int64{5, 6}
	if _, _, err := store.UpsertAppInstallationConnector(plain); !errors.Is(err, storagegithub.ErrAppConnectorOffered) {
		t.Fatalf("second sign-in: err = %v, want ErrAppConnectorOffered", err)
	}
	if scope, allowed, removed := scopeOf(t, store, idB); scope != storagegithub.RepoScopeUser || len(allowed) != 2 || !removed {
		t.Fatalf("after refresh: %q %v removed=%v, want user [5 6], still not connected", scope, allowed, removed)
	}

	// Connect makes it live with the recorded scope; connecting twice is refused.
	if err := store.ConnectAppConnector(idB, "ghs_b2", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if scope, allowed, removed := scopeOf(t, store, idB); scope != storagegithub.RepoScopeUser || len(allowed) != 2 || removed {
		t.Fatalf("after connect: %q %v removed=%v, want live user [5 6]", scope, allowed, removed)
	}
	if rec, _ := store.GetGitHubTokenByConnectorID(idB); rec.PatToken != "ghs_b2" {
		t.Fatal("connect did not store the token")
	}
	if err := store.ConnectAppConnector(idB, "x", time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("connecting a live connector: err = %v, want sql.ErrNoRows", err)
	}
	// Connected now, so a plain sign-in just refreshes it.
	if _, _, err := store.UpsertAppInstallationConnector(plain); err != nil {
		t.Fatalf("sign-in after connect: err = %v", err)
	}

	// Disconnected, then uninstalled: nothing is left to connect.
	if err := store.DisconnectAppConnector(idB); err != nil {
		t.Fatal(err)
	}
	if avail, _ := store.ListNotConnectedAppConnectorsForOrg(orgB); len(avail) != 1 {
		t.Fatalf("a disconnected connector should be available: %v", avail)
	}
	yes := true
	for _, id := range []int64{idA, idB} {
		if err := store.SetAppConnectorFlags(id, &yes, nil); err != nil {
			t.Fatal(err)
		}
	}
	if avail, _ := store.ListNotConnectedAppConnectorsForOrg(orgB); len(avail) != 0 {
		t.Fatalf("an uninstalled connector is still offered: %v", avail)
	}
	if err := store.ConnectAppConnector(idB, "x", time.Now()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("connecting an uninstalled connector: err = %v, want sql.ErrNoRows", err)
	}
	// Installed again with nobody else using it: a plain sign-in connects it.
	if _, _, err := store.UpsertAppInstallationConnector(plain); err != nil {
		t.Fatalf("sign-in after reinstall: err = %v, want connected", err)
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
