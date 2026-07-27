package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/storage/providers/pullrequests"
)

func newTestEchoContext(method, target string, orgID int64, paramNames, paramValues []string) (echo.Context, *httptest.ResponseRecorder) {
	req := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)
	if len(paramNames) > 0 {
		c.SetParamNames(paramNames...)
		c.SetParamValues(paramValues...)
	}
	return c, rec
}

func seedRepoAndPRsForAPITest(t *testing.T, server *Server, orgID, connectorID int64) (repoID, openPRID, closedPRID int64) {
	t.Helper()
	store := pullrequests.NewStore(server.db)

	repoID, err := store.UpsertRepository(pullrequests.RepositoryUpsert{
		OrgID: orgID, ConnectorID: connectorID, Provider: "github", ProviderRepoID: "api-test-repo",
		FullName: "acme/api-test-repo", Name: "api-test-repo", WebURL: "https://github.com/acme/api-test-repo",
		DefaultBranch: "main", IsPrivate: true, Description: "test repo",
	})
	if err != nil {
		t.Fatalf("seed repository: %v", err)
	}
	t.Cleanup(func() { _, _ = server.db.Exec(`DELETE FROM repositories WHERE id = $1`, repoID) })

	openPRID, err = store.UpsertPullRequest(pullrequests.PullRequestUpsert{
		RepositoryID: repoID, OrgID: orgID, Provider: "github", ProviderPRID: "pr-open", Number: 1,
		Title: "Open PR", State: "open", AuthorUsername: "alice",
		WebURL:            "https://github.com/acme/api-test-repo/pull/1",
		ProviderUpdatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		LastSyncedSource:  "poll",
	})
	if err != nil {
		t.Fatalf("seed open PR: %v", err)
	}

	closedPRID, err = store.UpsertPullRequest(pullrequests.PullRequestUpsert{
		RepositoryID: repoID, OrgID: orgID, Provider: "github", ProviderPRID: "pr-closed", Number: 2,
		Title: "Closed PR", State: "closed", AuthorUsername: "bob",
		WebURL:            "https://github.com/acme/api-test-repo/pull/2",
		ProviderUpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSyncedSource:  "poll",
	})
	if err != nil {
		t.Fatalf("seed closed PR: %v", err)
	}

	return repoID, openPRID, closedPRID
}

func TestListRepositories_ScopesByOrg(t *testing.T) {
	server := setupBotUserTestServer(t)
	orgID := getAnyOrgID(t, server.db)
	connectorID := insertIntegrationToken(t, server.db, "github", "https://github.com", "pat", nil)
	repoID, _, _ := seedRepoAndPRsForAPITest(t, server, orgID, connectorID)

	c, rec := newTestEchoContext(http.MethodGet, "/api/v1/repositories", orgID, nil, nil)
	if err := server.ListRepositories(c); err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp RepositoriesListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, r := range resp.Repositories {
		if r.ID == repoID {
			found = true
			if r.FullName != "acme/api-test-repo" {
				t.Errorf("unexpected full_name: %s", r.FullName)
			}
		}
	}
	if !found {
		t.Fatalf("expected seeded repository %d in listing", repoID)
	}

	// A different (non-existent) org must not see this org's repository.
	otherOrgC, otherRec := newTestEchoContext(http.MethodGet, "/api/v1/repositories", orgID+999999, nil, nil)
	if err := server.ListRepositories(otherOrgC); err != nil {
		t.Fatalf("ListRepositories (other org): %v", err)
	}
	var otherResp RepositoriesListResponse
	if err := json.Unmarshal(otherRec.Body.Bytes(), &otherResp); err != nil {
		t.Fatalf("unmarshal (other org): %v", err)
	}
	for _, r := range otherResp.Repositories {
		if r.ID == repoID {
			t.Fatalf("repository %d leaked into a different org's listing", repoID)
		}
	}
}

func TestGetRepository_NotFoundForWrongOrg(t *testing.T) {
	server := setupBotUserTestServer(t)
	orgID := getAnyOrgID(t, server.db)
	connectorID := insertIntegrationToken(t, server.db, "github", "https://github.com", "pat", nil)
	repoID, _, _ := seedRepoAndPRsForAPITest(t, server, orgID, connectorID)

	c, rec := newTestEchoContext(http.MethodGet, "/api/v1/repositories/:repoId", orgID,
		[]string{"repoId"}, []string{strconv.FormatInt(repoID, 10)})
	if err := server.GetRepository(c); err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	wrongOrgC, _ := newTestEchoContext(http.MethodGet, "/api/v1/repositories/:repoId", orgID+999999,
		[]string{"repoId"}, []string{strconv.FormatInt(repoID, 10)})
	err := server.GetRepository(wrongOrgC)
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong org, got err=%v", err)
	}
}

func TestListPullRequestsForRepo_FiltersByState(t *testing.T) {
	server := setupBotUserTestServer(t)
	orgID := getAnyOrgID(t, server.db)
	connectorID := insertIntegrationToken(t, server.db, "github", "https://github.com", "pat", nil)
	repoID, openPRID, closedPRID := seedRepoAndPRsForAPITest(t, server, orgID, connectorID)

	target := "/api/v1/repositories/" + strconv.FormatInt(repoID, 10) + "/pull-requests?state=open"
	c, rec := newTestEchoContext(http.MethodGet, target, orgID, []string{"repoId"}, []string{strconv.FormatInt(repoID, 10)})
	if err := server.ListPullRequestsForRepo(c); err != nil {
		t.Fatalf("ListPullRequestsForRepo: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp PullRequestsListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.PullRequests) != 1 || resp.PullRequests[0].ID != openPRID {
		t.Fatalf("expected only the open PR (%d), got %+v", openPRID, resp.PullRequests)
	}
	_ = closedPRID
}

func TestGetPullRequest_IncludesReviewHistory(t *testing.T) {
	server := setupBotUserTestServer(t)
	orgID := getAnyOrgID(t, server.db)
	connectorID := insertIntegrationToken(t, server.db, "github", "https://github.com", "pat", nil)
	repoID, openPRID, _ := seedRepoAndPRsForAPITest(t, server, orgID, connectorID)

	var reviewID int64
	err := server.db.QueryRow(`
		INSERT INTO reviews (repository, pr_mr_url, status, trigger_type, provider, org_id, pull_request_id, created_at)
		VALUES ($1, $2, 'completed', 'manual', 'github', $3, $4, now())
		RETURNING id
	`, "acme/api-test-repo", "https://github.com/acme/api-test-repo/pull/1", orgID, openPRID).Scan(&reviewID)
	if err != nil {
		t.Fatalf("seed review: %v", err)
	}
	t.Cleanup(func() { _, _ = server.db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })

	target := "/api/v1/repositories/" + strconv.FormatInt(repoID, 10) + "/pull-requests/" + strconv.FormatInt(openPRID, 10)
	c, rec := newTestEchoContext(http.MethodGet, target, orgID,
		[]string{"repoId", "prId"}, []string{strconv.FormatInt(repoID, 10), strconv.FormatInt(openPRID, 10)})
	if err := server.GetPullRequest(c); err != nil {
		t.Fatalf("GetPullRequest: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp PullRequestDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Reviews) != 1 || resp.Reviews[0].ID != reviewID {
		t.Fatalf("expected review %d in history, got %+v", reviewID, resp.Reviews)
	}
	if resp.Reviews[0].Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Reviews[0].Status)
	}
}
