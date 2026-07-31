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
