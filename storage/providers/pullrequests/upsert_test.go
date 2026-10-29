package pullrequests

import (
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/livereview/internal/database"
)

// testDB opens a connection to the real dev Postgres (via DATABASE_URL / .env,
// same convention as internal/api's bot_user_test_helpers_test.go), matching
// this codebase's established no-testcontainers, real-DB testing convention.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.NewDB()
	if err != nil {
		t.Skipf("skipping: no database available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// insertTestConnector inserts a minimal integration_tokens row and returns its id.
func insertTestConnector(t *testing.T, db *sql.DB, provider string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id)
		VALUES ($1, 'test-app', 'na', 'test-connector', 'https://example.test', 1)
		RETURNING id
	`, provider).Scan(&id)
	if err != nil {
		t.Fatalf("failed to insert test connector: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, id)
	})
	return id
}

func TestUpsertRepository_Idempotent(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	connectorID := insertTestConnector(t, db, "github")

	repo := RepositoryUpsert{
		OrgID:          1,
		ConnectorID:    connectorID,
		Provider:       "github",
		ProviderRepoID: "12345",
		FullName:       "acme/repo",
		Name:           "repo",
		WebURL:         "https://github.com/acme/repo",
		DefaultBranch:  "main",
		IsPrivate:      true,
	}

	id1, err := store.UpsertRepository(repo)
	if err != nil {
		t.Fatalf("first UpsertRepository: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, id1) })

	repo.Description = "updated description"
	id2, err := store.UpsertRepository(repo)
	if err != nil {
		t.Fatalf("second UpsertRepository: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id on repeated upsert, got %d then %d", id1, id2)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM repositories WHERE connector_id = $1 AND provider_repo_id = $2`,
		connectorID, repo.ProviderRepoID).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after repeated upsert, got %d", count)
	}

	var desc string
	if err := db.QueryRow(`SELECT description FROM repositories WHERE id = $1`, id1).Scan(&desc); err != nil {
		t.Fatalf("select description: %v", err)
	}
	if desc != "updated description" {
		t.Fatalf("expected description to be updated, got %q", desc)
	}
}

func TestEnsureRepositoryStub_DoesNotOverwrite(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	connectorID := insertTestConnector(t, db, "github")

	full := RepositoryUpsert{
		OrgID: 1, ConnectorID: connectorID, Provider: "github", ProviderRepoID: "999",
		FullName: "acme/full", Name: "full", WebURL: "https://github.com/acme/full",
		DefaultBranch: "main", Description: "full description",
	}
	id1, err := store.UpsertRepository(full)
	if err != nil {
		t.Fatalf("UpsertRepository: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, id1) })

	stub := RepositoryUpsert{
		OrgID: 1, ConnectorID: connectorID, Provider: "github", ProviderRepoID: "999",
		FullName: "acme/full", Name: "full", WebURL: "https://github.com/acme/full",
	}
	id2, err := store.EnsureRepositoryStub(stub)
	if err != nil {
		t.Fatalf("EnsureRepositoryStub: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected stub to resolve to existing row id %d, got %d", id1, id2)
	}

	var desc string
	if err := db.QueryRow(`SELECT description FROM repositories WHERE id = $1`, id1).Scan(&desc); err != nil {
		t.Fatalf("select description: %v", err)
	}
	if desc != "full description" {
		t.Fatalf("EnsureRepositoryStub must not overwrite existing fields, got description %q", desc)
	}
}

func insertTestRepository(t *testing.T, db *sql.DB, connectorID int64, providerRepoID string) int64 {
	t.Helper()
	store := NewStore(db)
	id, err := store.UpsertRepository(RepositoryUpsert{
		OrgID: 1, ConnectorID: connectorID, Provider: "github", ProviderRepoID: providerRepoID,
		FullName: "acme/repo-" + providerRepoID, Name: "repo-" + providerRepoID,
		WebURL: "https://github.com/acme/repo-" + providerRepoID,
	})
	if err != nil {
		t.Fatalf("insertTestRepository: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, id) })
	return id
}

func TestUpsertPullRequest_Idempotent(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	connectorID := insertTestConnector(t, db, "github")
	repoID := insertTestRepository(t, db, connectorID, "r1")

	base := PullRequestUpsert{
		RepositoryID: repoID, OrgID: 1, Provider: "github", ProviderPRID: "pr-1", Number: 1,
		Title: "first title", State: "open", WebURL: "https://github.com/acme/repo/pull/1",
		ProviderUpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSyncedSource:  "poll",
	}

	id1, err := store.UpsertPullRequest(base)
	if err != nil {
		t.Fatalf("first UpsertPullRequest: %v", err)
	}

	id2, err := store.UpsertPullRequest(base)
	if err != nil {
		t.Fatalf("repeated UpsertPullRequest: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same id on repeated upsert, got %d then %d", id1, id2)
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pull_requests WHERE repository_id = $1 AND provider_pr_id = $2`,
		repoID, base.ProviderPRID).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after repeated upsert, got %d", count)
	}
}

func TestUpsertPullRequest_StaleWriteIsRejected(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	connectorID := insertTestConnector(t, db, "github")
	repoID := insertTestRepository(t, db, connectorID, "r2")

	newer := PullRequestUpsert{
		RepositoryID: repoID, OrgID: 1, Provider: "github", ProviderPRID: "pr-2", Number: 2,
		Title: "newer title", State: "closed", WebURL: "https://github.com/acme/repo/pull/2",
		ProviderUpdatedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		LastSyncedSource:  "webhook",
	}
	id1, err := store.UpsertPullRequest(newer)
	if err != nil {
		t.Fatalf("insert newer: %v", err)
	}

	stale := newer
	stale.Title = "stale title (should be ignored)"
	stale.State = "open"
	stale.ProviderUpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // older than what's stored

	id2, err := store.UpsertPullRequest(stale)
	if err != nil {
		t.Fatalf("upsert with stale provider_updated_at: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected same row id even when stale write is rejected, got %d then %d", id1, id2)
	}

	var title, state string
	if err := db.QueryRow(`SELECT title, state FROM pull_requests WHERE id = $1`, id1).Scan(&title, &state); err != nil {
		t.Fatalf("select: %v", err)
	}
	if title != "newer title" || state != "closed" {
		t.Fatalf("stale write must not overwrite newer state, got title=%q state=%q", title, state)
	}
}

func TestUpsertPullRequest_ConcurrentUpsertsDoNotDuplicate(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	connectorID := insertTestConnector(t, db, "github")
	repoID := insertTestRepository(t, db, connectorID, "r3")

	pr := PullRequestUpsert{
		RepositoryID: repoID, OrgID: 1, Provider: "github", ProviderPRID: "pr-3", Number: 3,
		Title: "concurrent", State: "open", WebURL: "https://github.com/acme/repo/pull/3",
		ProviderUpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSyncedSource:  "poll",
	}

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := store.UpsertPullRequest(pr)
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM pull_requests WHERE repository_id = $1 AND provider_pr_id = $2`,
		repoID, pr.ProviderPRID).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 row after concurrent upserts, got %d", count)
	}
}

func TestSiblingPullRequestForUser_OnlyInTheUsersOrgs(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)

	mkOrg := func(name string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'sibling test') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, id) })
		return id
	}
	mkConnector := func(orgID int64) int64 {
		var id int64
		if err := db.QueryRow(`
			INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id)
			VALUES ('github', 'sibling-test', 'na', 'c', 'https://github.test', $1) RETURNING id`, orgID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, id) })
		return id
	}
	mkPR := func(orgID, connectorID int64) int64 {
		repoID, err := store.UpsertRepository(RepositoryUpsert{
			OrgID: orgID, ConnectorID: connectorID, Provider: "github", ProviderRepoID: "repo-1",
			FullName: "acme/api", Name: "api", WebURL: "https://github.test/acme/api",
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, repoID) })
		prID, err := store.UpsertPullRequest(PullRequestUpsert{
			RepositoryID: repoID, OrgID: orgID, Provider: "github", ProviderPRID: "gh-pr-777", Number: 7,
			Title: "t", State: "open", WebURL: "https://github.test/acme/api/pull/7", LastSyncedSource: "webhook",
			ProviderUpdatedAt: time.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return prID
	}
	orgA, orgB := mkOrg("sibling-a-"+time.Now().Format("150405.000000")), mkOrg("sibling-b-"+time.Now().Format("150405.000000"))
	prA, prB := mkPR(orgA, mkConnector(orgA)), mkPR(orgB, mkConnector(orgB))

	var userID int64
	email := "sibling-" + time.Now().Format("150405.000000") + "@example.test"
	if err := db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = $1`, userID)
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})

	if _, _, err := store.SiblingPullRequestForUser(prA, userID); err != sql.ErrNoRows {
		t.Fatalf("a user in neither org: err = %v, want sql.ErrNoRows", err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, org_id, role_id) VALUES ($1, $2, (SELECT id FROM roles WHERE name = 'member'))`, userID, orgB); err != nil {
		t.Fatal(err)
	}
	gotOrg, gotPR, err := store.SiblingPullRequestForUser(prA, userID)
	if err != nil || gotOrg != orgB || gotPR != prB {
		t.Fatalf("got (%d, %d, %v), want org %d's copy %d", gotOrg, gotPR, err, orgB, prB)
	}
}

func TestAvailableConnectorForPullRequest(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	stamp := time.Now().Format("150405.000000")
	inst := "avail-" + stamp

	mkOrg := func(name string) int64 {
		var id int64
		if err := db.QueryRow(`INSERT INTO orgs (name, description) VALUES ($1, 'available test') RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, id) })
		return id
	}
	mkConnector := func(orgID int64, meta string) int64 {
		var id int64
		if err := db.QueryRow(`
			INSERT INTO integration_tokens (provider, provider_app_id, access_token, token_type, connection_name, provider_url, org_id, metadata)
			VALUES ('github', $1, '', 'GITHUB_APP', 'c', 'https://github.test', $2, $3::jsonb) RETURNING id`, inst, orgID, meta).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM integration_tokens WHERE id = $1`, id) })
		return id
	}
	orgA, orgB := mkOrg("avail-a-"+stamp), mkOrg("avail-b-"+stamp)
	connA := mkConnector(orgA, `{"account_login":"acme"}`)
	connB := mkConnector(orgB, `{"account_login":"acme","removed":true,"disconnected":true,"offered":true,"repo_scope":"user","allowed_repo_ids":[42]}`)
	repoID, err := store.UpsertRepository(RepositoryUpsert{
		OrgID: orgA, ConnectorID: connA, Provider: "github", ProviderRepoID: "42", FullName: "acme/api", Name: "api",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM repositories WHERE id = $1`, repoID) })
	prA, err := store.UpsertPullRequest(PullRequestUpsert{
		RepositoryID: repoID, OrgID: orgA, Provider: "github", ProviderPRID: "gh-pr-" + stamp, Number: 1, Title: "t", State: "open",
		ProviderUpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	var userID int64
	if err := db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`, "avail-"+stamp+"@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = $1`, userID)
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := store.AvailableConnectorForPullRequest(prA, userID); err != sql.ErrNoRows {
		t.Fatalf("a user in neither org: err = %v, want sql.ErrNoRows", err)
	}
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, org_id, role_id) VALUES ($1, $2, (SELECT id FROM roles WHERE name = 'owner'))`, userID, orgB); err != nil {
		t.Fatal(err)
	}
	got, err := store.AvailableConnectorForPullRequest(prA, userID)
	if err != nil || got.OrgID != orgB || got.ConnectorID != connB || got.AccountLogin != "acme" || !got.CanConnect {
		t.Fatalf("got %+v, %v; want org %d's connector %d, connectable", got, err, orgB, connB)
	}
	// Not on the user-scoped connector's allowed list: connecting wouldn't reach this repository.
	if _, err := db.Exec(`UPDATE integration_tokens SET metadata = jsonb_set(metadata, '{allowed_repo_ids}', '[7]') WHERE id = $1`, connB); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AvailableConnectorForPullRequest(prA, userID); err != sql.ErrNoRows {
		t.Fatalf("repository not covered: err = %v, want sql.ErrNoRows", err)
	}
}

func TestDeleteRepositoriesByConnectorExcept(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	connectorID := insertTestConnector(t, db, "github")
	keep := insertTestRepository(t, db, connectorID, "keep-1")
	drop := insertTestRepository(t, db, connectorID, "drop-1")

	if err := store.DeleteRepositoriesByConnectorExcept(connectorID, []string{"keep-1"}); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM repositories WHERE id = $1`, keep).Scan(&n)
	if n != 1 {
		t.Error("the allowed repository was deleted")
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM repositories WHERE id = $1`, drop).Scan(&n)
	if n != 0 {
		t.Error("the repository that is no longer allowed was kept")
	}
}

func TestUpsertRepository_RejectsIncompatibleProvider(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	gitlabConnectorID := insertTestConnector(t, db, "gitlab-self-hosted")

	// Attempt to upsert a GitHub repo with a GitLab connector
	_, err := store.UpsertRepository(RepositoryUpsert{
		OrgID:          1,
		ConnectorID:    gitlabConnectorID,
		Provider:       "github",
		ProviderRepoID: "12345",
		FullName:       "acme/github-repo",
		Name:           "github-repo",
	})
	if err == nil {
		t.Fatal("expected error when upserting github repo with gitlab connector, got nil")
	}
}

func TestEnsureRepositoryStub_RejectsIncompatibleProvider(t *testing.T) {
	db := testDB(t)
	store := NewStore(db)
	gitlabConnectorID := insertTestConnector(t, db, "gitlab-self-hosted")

	// Attempt to create a stub for a GitHub repo with a GitLab connector
	_, err := store.EnsureRepositoryStub(RepositoryUpsert{
		OrgID:          1,
		ConnectorID:    gitlabConnectorID,
		Provider:       "github",
		ProviderRepoID: "12345",
		FullName:       "acme/github-repo",
		Name:           "github-repo",
	})
	if err == nil {
		t.Fatal("expected error when creating stub for github repo with gitlab connector, got nil")
	}
}
