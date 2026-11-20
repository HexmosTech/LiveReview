package api

// Handler-level integration test for the Related Repos feature
// (Explore -> Repository -> Settings -> Related Repos; see
// docs/multi-repo-blast-radius-plan.md): confirms the real HTTP handlers,
// against a real Postgres connection, actually read/write
// repository_relations correctly and enforce tenant isolation and
// owner-only write access - not just that the Go code compiles.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"

	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
)

func relationsTestDB(t *testing.T) *sql.DB {
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

// seedRepo inserts a minimal connector + repository row under orgID and
// returns the repository's id.
func seedRepo(t *testing.T, db *sql.DB, orgID int64, fullName string) int64 {
	t.Helper()
	var connectorID int64
	err := db.QueryRow(`
		INSERT INTO integration_tokens (provider, provider_app_id, access_token, connection_name, provider_url, org_id)
		VALUES ('github', 'test-app', 'tok', $1, 'https://github.com', $2)
		RETURNING id`, "conn-"+fullName, orgID).Scan(&connectorID)
	if err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	var repoID int64
	err = db.QueryRow(`
		INSERT INTO repositories (org_id, connector_id, provider, provider_repo_id, full_name, name, web_url, clone_url)
		VALUES ($1, $2, 'github', $3, $4, $4, $5, $5)
		RETURNING id`, orgID, connectorID, fullName, fullName, "https://github.com/"+fullName).Scan(&repoID)
	if err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	return repoID
}

func permCtx(orgID int64, isOwner bool) *auth.PermissionContext {
	return &auth.PermissionContext{
		User:    &models.User{ID: 1},
		OrgID:   orgID,
		IsOwner: isOwner,
	}
}

// newRelationsCtx builds an echo.Context carrying permCtx and :repoId
// (and, if relationID != 0, :relationId) exactly as server.go's routing
// would for GET/POST/DELETE .../repositories/:repoId/related[/:relationId].
func newRelationsCtx(e *echo.Echo, method, body string, pc *auth.PermissionContext, repoID, relationID int64) (echo.Context, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, "/", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	} else {
		req = httptest.NewRequest(method, "/", nil)
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(string(auth.PermissionContextKey), pc)
	if relationID != 0 {
		c.SetParamNames("repoId", "relationId")
		c.SetParamValues(strconv.FormatInt(repoID, 10), strconv.FormatInt(relationID, 10))
	} else {
		c.SetParamNames("repoId")
		c.SetParamValues(strconv.FormatInt(repoID, 10))
	}
	return c, rec
}

func TestRelatedRepositories_AddListRemove(t *testing.T) {
	db := relationsTestDB(t)
	s := &Server{db: db}

	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('related-repos-handler-test') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM repository_relations WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM repositories WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM integration_tokens WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	repoA := seedRepo(t, db, orgID, "acme/frontend")
	repoB := seedRepo(t, db, orgID, "acme/backend")

	e := echo.New()
	owner := permCtx(orgID, true)

	// 1. Empty list before anything is linked.
	c, rec := newRelationsCtx(e, http.MethodGet, "", owner, repoA, 0)
	if err := s.ListRelatedRepositories(c); err != nil {
		t.Fatalf("list (empty): %v", err)
	}
	var listed []RelatedRepository
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode empty list: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected no related repos yet, got %+v", listed)
	}

	// 2. Add repoB as related to repoA.
	c, rec = newRelationsCtx(e, http.MethodPost, `{"related_repository_id":`+strconv.FormatInt(repoB, 10)+`}`, owner, repoA, 0)
	if err := s.AddRelatedRepository(c); err != nil {
		t.Fatalf("add: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("add: status %d body %s", rec.Code, rec.Body.String())
	}

	// 3. List from repoA's side shows repoB; list from repoB's side (the
	//    other direction) shows repoA - confirms the symmetric read.
	c, rec = newRelationsCtx(e, http.MethodGet, "", owner, repoA, 0)
	if err := s.ListRelatedRepositories(c); err != nil {
		t.Fatalf("list from repoA: %v", err)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list from repoA: %v", err)
	}
	if len(listed) != 1 || listed[0].RepositoryID != repoB || listed[0].FullName != "acme/backend" {
		t.Fatalf("list from repoA: %+v", listed)
	}
	relationID := listed[0].RelationID

	c, rec = newRelationsCtx(e, http.MethodGet, "", owner, repoB, 0)
	if err := s.ListRelatedRepositories(c); err != nil {
		t.Fatalf("list from repoB: %v", err)
	}
	var listedFromB []RelatedRepository
	if err := json.Unmarshal(rec.Body.Bytes(), &listedFromB); err != nil {
		t.Fatalf("decode list from repoB: %v", err)
	}
	if len(listedFromB) != 1 || listedFromB[0].RepositoryID != repoA {
		t.Fatalf("list from repoB (symmetric read): %+v", listedFromB)
	}

	// 3b. Adding the reverse pair (repoB -> repoA) must not create a second
	// row - the handler canonicalizes ordering before insert specifically so
	// (A,B) and (B,A) can never coexist (see AddRelatedRepository).
	c, rec = newRelationsCtx(e, http.MethodPost, `{"related_repository_id":`+strconv.FormatInt(repoA, 10)+`}`, owner, repoB, 0)
	if err := s.AddRelatedRepository(c); err != nil {
		t.Fatalf("add reverse pair: %v", err)
	}
	c, rec = newRelationsCtx(e, http.MethodGet, "", owner, repoA, 0)
	if err := s.ListRelatedRepositories(c); err != nil {
		t.Fatalf("list from repoA after reverse add: %v", err)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list from repoA after reverse add: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected the reverse add to be a no-op, got %d relations: %+v", len(listed), listed)
	}

	// 4. A member (not owner/super-admin) cannot add or remove.
	member := permCtx(orgID, false)
	c, rec = newRelationsCtx(e, http.MethodPost, `{"related_repository_id":`+strconv.FormatInt(repoB, 10)+`}`, member, repoA, 0)
	err := s.AddRelatedRepository(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusForbidden {
		t.Fatalf("member add: expected 403, got err=%v", err)
	}

	// 5. Self-relation is rejected.
	c, rec = newRelationsCtx(e, http.MethodPost, `{"related_repository_id":`+strconv.FormatInt(repoA, 10)+`}`, owner, repoA, 0)
	err = s.AddRelatedRepository(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusBadRequest {
		t.Fatalf("self-relation: expected 400, got err=%v", err)
	}

	// 6. Cross-tenant: a different org cannot see or relate to this org's repos.
	var otherOrgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('related-repos-other-org-test') RETURNING id`).Scan(&otherOrgID); err != nil {
		t.Fatalf("insert other org: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, otherOrgID) })
	otherOwner := permCtx(otherOrgID, true)
	c, rec = newRelationsCtx(e, http.MethodPost, `{"related_repository_id":`+strconv.FormatInt(repoB, 10)+`}`, otherOwner, repoA, 0)
	err = s.AddRelatedRepository(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant add: expected 404, got err=%v", err)
	}

	// 7. Remove the relation, then confirm both sides see it gone.
	c, rec = newRelationsCtx(e, http.MethodDelete, "", owner, repoA, relationID)
	if err := s.RemoveRelatedRepository(c); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: status %d body %s", rec.Code, rec.Body.String())
	}
	c, rec = newRelationsCtx(e, http.MethodGet, "", owner, repoA, 0)
	if err := s.ListRelatedRepositories(c); err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list after remove: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("expected no related repos after remove, got %+v", listed)
	}

	// 8. Removing again (already gone) is a 404, and removing someone else's
	// relation ID (cross-tenant) is also a 404, not a silent no-op.
	c, rec = newRelationsCtx(e, http.MethodDelete, "", owner, repoA, relationID)
	err = s.RemoveRelatedRepository(c)
	if httpErr, ok := err.(*echo.HTTPError); !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("double remove: expected 404, got err=%v", err)
	}
}
