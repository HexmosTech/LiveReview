package github

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/livereview/internal/database"
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

func testOrg(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	name := fmt.Sprintf("tokenstore-test-%d", time.Now().UnixNano())
	if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'token store test') RETURNING id`, name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM repositories WHERE org_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM integration_tokens WHERE org_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, id)
	})
	return id
}

func addRepo(t *testing.T, db *sql.DB, orgID, connectorID int64, fullName string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO repositories (org_id, connector_id, provider, provider_repo_id, full_name, name, web_url)
		VALUES ($1, $2, 'github', $3, $4, 'r', 'http://x')
	`, orgID, connectorID, fmt.Sprintf("%d-%d", connectorID, time.Now().UnixNano()), fullName)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFindPreferredConnectorForRepo_AppBeatsPATAndRemovedIsSkipped(t *testing.T) {
	db := testDB(t)
	store := NewTokenStore(db)
	org := testOrg(t, db)
	otherOrg := testOrg(t, db)

	var pat int64
	if err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id, token_type, pat_token)
		VALUES ('github', 'p', 'na', 'pat', 'https://github.com', $1, 'PAT', 'ghp_x') RETURNING id`, org).Scan(&pat); err != nil {
		t.Fatal(err)
	}
	appID, _, err := store.UpsertAppInstallationConnector(AppConnectorUpsert{
		OrgID: org, BaseURL: "https://github.com.test", InstallationID: time.Now().UnixNano() % 1_000_000_000_000,
		AccountLogin: "acme", AccountType: "User", Token: "ghs_x", ExpiresAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	addRepo(t, db, org, pat, "Acme/API")
	addRepo(t, db, org, appID, "acme/api")

	got, err := store.FindPreferredConnectorForRepo(org, "acme/api")
	if err != nil || got != appID {
		t.Fatalf("got (%d, %v), want the App connector %d (case-insensitive repo match)", got, err, appID)
	}
	if has, _ := store.HasAppConnectorForRepo(org, "ACME/api"); !has {
		t.Error("HasAppConnectorForRepo = false, want true")
	}
	if _, err := store.FindPreferredConnectorForRepo(otherOrg, "acme/api"); err != sql.ErrNoRows {
		t.Errorf("another org must not see the repository: %v", err)
	}
	if has, _ := store.HasAppConnectorForRepo(otherOrg, "acme/api"); has {
		t.Error("another org must not report an App connector")
	}

	yes := true
	if err := store.SetAppConnectorFlags(appID, &yes, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := store.FindPreferredConnectorForRepo(org, "acme/api"); err != nil || got != pat {
		t.Fatalf("after uninstall got (%d, %v), want the PAT connector %d", got, err, pat)
	}
	if has, _ := store.HasAppConnectorForRepo(org, "acme/api"); has {
		t.Error("a removed App connector must not supersede the PAT connector")
	}
}

func TestListAppConnectorsExpiringBefore_SkipsRemovedAndSuspended(t *testing.T) {
	db := testDB(t)
	store := NewTokenStore(db)
	org := testOrg(t, db)
	base := time.Now().UnixNano() % 1_000_000_000_000
	mk := func(i int64, exp time.Duration) int64 {
		id, _, err := store.UpsertAppInstallationConnector(AppConnectorUpsert{
			OrgID: org, BaseURL: "https://github.com.test", InstallationID: base + i, AccountLogin: "a", AccountType: "User",
			Token: "t", ExpiresAt: time.Now().Add(exp),
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	soon, later, removed, suspended := mk(1, 5*time.Minute), mk(2, 3*time.Hour), mk(3, time.Minute), mk(4, time.Minute)
	yes := true
	_ = store.SetAppConnectorFlags(removed, &yes, nil)
	_ = store.SetAppConnectorFlags(suspended, nil, &yes)

	recs, err := store.ListAppConnectorsExpiringBefore(time.Now().Add(20 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	found := map[int64]bool{}
	for _, r := range recs {
		found[r.ID] = true
	}
	if !found[soon] || found[later] || found[removed] || found[suspended] {
		t.Fatalf("expiring set wrong: soon=%v later=%v removed=%v suspended=%v", found[soon], found[later], found[removed], found[suspended])
	}
	forOrg, _ := store.ListAppConnectorsForOrg(org)
	for _, r := range forOrg {
		if r.ID == removed {
			t.Error("ListAppConnectorsForOrg returned a removed connector")
		}
	}
}
