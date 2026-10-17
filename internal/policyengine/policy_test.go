package policyengine

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

func testDB(t *testing.T) *sql.DB {
	if testing.Short() {
		t.Skip("skipping database integration test")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	return db
}

func seedOrg(t *testing.T, db *sql.DB, name string) int64 {
	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ($1) RETURNING id`, name).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		// The live dev server's background dashboard-cache refresh job may have
		// already inserted a row for this org by the time the test ends; its FK
		// has no cascade, so it must be cleared first or the org delete leaks.
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM auto_approve_policies WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})
	return orgID
}

func insertPolicy(t *testing.T, db *sql.DB, orgID int64, repoID *int64, mode string, minLevel int) int64 {
	var id int64
	if repoID == nil {
		// Every new org already gets a default org-level row from a trigger; reuse it.
		if err := db.QueryRow(`
			UPDATE auto_approve_policies SET mode = $2, min_author_level = $3
			WHERE org_id = $1 AND repository_id IS NULL RETURNING id`, orgID, mode, minLevel).Scan(&id); err != nil {
			t.Fatalf("update default policy: %v", err)
		}
		return id
	}
	if err := db.QueryRow(`
		INSERT INTO auto_approve_policies (org_id, repository_id, mode, min_author_level)
		VALUES ($1, $2, $3, $4) RETURNING id`, orgID, repoID, mode, minLevel).Scan(&id); err != nil {
		t.Fatalf("insert policy: %v", err)
	}
	// auto_approve_policies.org_id has no ON DELETE CASCADE, so this must be
	// cleaned up before seedOrg's org delete (t.Cleanup runs LIFO, and this is
	// registered after seedOrg's, so it runs first) or the org delete fails
	// silently and leaks on every run.
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM auto_approve_policies WHERE id = $1`, id) })
	return id
}

// No policy configured at all: ResolvePolicy reports the feature as off.
func TestResolvePolicyNoneConfigured(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "policy-resolve-none")
	// Drop the default row the org-insert trigger created.
	if _, err := db.Exec(`DELETE FROM auto_approve_policies WHERE org_id = $1`, orgID); err != nil {
		t.Fatalf("delete default policy: %v", err)
	}

	_, found, err := ResolvePolicy(context.Background(), db, orgID, 0)
	if err != nil {
		t.Fatalf("ResolvePolicy: %v", err)
	}
	if found {
		t.Fatalf("expected found=false when no policy row exists")
	}
}

// Only the org-default row exists: a repo-scoped lookup falls back to it.
func TestResolvePolicyFallsBackToOrgDefault(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "policy-resolve-fallback")
	insertPolicy(t, db, orgID, nil, ModeCustom, 2)

	p, found, err := ResolvePolicy(context.Background(), db, orgID, 999)
	if err != nil {
		t.Fatalf("ResolvePolicy: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true via org-default fallback")
	}
	if p.RepositoryID != nil {
		t.Fatalf("expected the org-default row (repository_id nil), got %+v", p.RepositoryID)
	}
	if p.MinAuthorLevel != 2 {
		t.Fatalf("expected org default's min_author_level=2, got %d", p.MinAuthorLevel)
	}
}

// A repo row with mode='inherit' defers wholesale to the org default, even
// though a repo-scoped row exists.
func TestResolvePolicyInheritDefersToOrgDefault(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "policy-resolve-inherit")
	insertPolicy(t, db, orgID, nil, ModeCustom, 3)
	repoID := int64(42)
	insertPolicy(t, db, orgID, &repoID, ModeInherit, 1)

	p, found, err := ResolvePolicy(context.Background(), db, orgID, repoID)
	if err != nil {
		t.Fatalf("ResolvePolicy: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true")
	}
	if p.RepositoryID != nil {
		t.Fatalf("expected the org-default row to win over an 'inherit' repo row, got repository_id=%v", p.RepositoryID)
	}
	if p.MinAuthorLevel != 3 {
		t.Fatalf("expected org default's min_author_level=3, got %d", p.MinAuthorLevel)
	}
}

// A repo row with mode='custom' takes precedence over the org default.
func TestResolvePolicyCustomRepoRowWins(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "policy-resolve-custom-repo")
	insertPolicy(t, db, orgID, nil, ModeCustom, 1)
	repoID := int64(43)
	insertPolicy(t, db, orgID, &repoID, ModeCustom, 5)

	p, found, err := ResolvePolicy(context.Background(), db, orgID, repoID)
	if err != nil {
		t.Fatalf("ResolvePolicy: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true")
	}
	if p.RepositoryID == nil || *p.RepositoryID != repoID {
		t.Fatalf("expected the repo-scoped row to win, got repository_id=%v", p.RepositoryID)
	}
	if p.MinAuthorLevel != 5 {
		t.Fatalf("expected repo row's min_author_level=5, got %d", p.MinAuthorLevel)
	}
}
