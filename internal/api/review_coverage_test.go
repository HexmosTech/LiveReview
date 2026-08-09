package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReviewCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	// Registered via t.Cleanup (not `defer`) so it runs *after* the delete
	// cleanup below: function-local defers unwind before t.Cleanup callbacks
	// fire, so a plain `defer db.Close()` here would close the connection
	// before the delete cleanup got to use it, leaving rows behind.
	t.Cleanup(func() { db.Close() })

	orgID := int64(1)
	otherOrgID := int64(999999)
	_, _ = db.Exec(`INSERT INTO orgs (id, name) VALUES ($1, 'coverage-test-other-org') ON CONFLICT (id) DO NOTHING`, otherOrgID)

	insertReview := func(t *testing.T, org int64, triggerType, status string) int64 {
		t.Helper()
		var reviewID int64
		err := db.QueryRow(`
			INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, "test/coverage-repo", "main", "", "https://test.com/pr/coverage", status, triggerType, "coverage@example.com", "test", org).Scan(&reviewID)
		require.NoError(t, err)
		return reviewID
	}

	cliReviewID := insertReview(t, orgID, "cli_diff", "completed")
	mcpReviewID := insertReview(t, orgID, "mcp", "completed")
	otherOrgReviewID := insertReview(t, otherOrgID, "manual", "completed")

	const sharedSHA = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	const rangeRef = "aaaa111..bbbb222"
	const untouchedSHA = "0000000000000000000000000000000000000000untouched"

	require.NoError(t, insertReviewCommitsTx(context.Background(), db, cliReviewID, orgID, nil, []CommitRef{{Ref: sharedSHA, Type: "commit"}, {Ref: rangeRef, Type: "range"}}))
	require.NoError(t, insertReviewCommitsTx(context.Background(), db, mcpReviewID, orgID, nil, []CommitRef{{Ref: sharedSHA, Type: "commit"}}))
	require.NoError(t, insertReviewCommitsTx(context.Background(), db, otherOrgReviewID, otherOrgID, nil, []CommitRef{{Ref: sharedSHA, Type: "commit"}}))

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM review_commits WHERE review_id IN ($1, $2, $3)", cliReviewID, mcpReviewID, otherOrgReviewID)
		_, _ = db.Exec("DELETE FROM public.reviews WHERE id IN ($1, $2, $3)", cliReviewID, mcpReviewID, otherOrgReviewID)
		_, _ = db.Exec("DELETE FROM orgs WHERE id = $1", otherOrgID)
	})

	s := &Server{db: db}
	e := echo.New()

	doRequest := func(t *testing.T, org int64, commits []string) (*httptest.ResponseRecorder, map[string]interface{}) {
		t.Helper()
		body, err := json.Marshal(ReviewCoverageRequest{Commits: commits})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/review-coverage", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("org_id", org)

		err = s.ReviewCoverage(c)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &parsed))
		return rec, parsed
	}

	t.Run("returns multiple reports for a commit reviewed twice", func(t *testing.T) {
		rec, parsed := doRequest(t, orgID, []string{sharedSHA})
		assert.Equal(t, http.StatusOK, rec.Code)

		reports, ok := parsed["reports"].([]interface{})
		require.True(t, ok)
		assert.Len(t, reports, 2, "commit was reviewed once via cli_diff and once via mcp")

		triggerTypes := map[string]bool{}
		for _, r := range reports {
			report := r.(map[string]interface{})
			assert.Equal(t, "database", report["source"])
			assert.Equal(t, sharedSHA, report["ref"])
			triggerTypes[report["trigger_type"].(string)] = true
		}
		assert.True(t, triggerTypes["cli_diff"])
		assert.True(t, triggerTypes["mcp"])
	})

	t.Run("matches a literal range ref", func(t *testing.T) {
		_, parsed := doRequest(t, orgID, []string{rangeRef})
		reports := parsed["reports"].([]interface{})
		require.Len(t, reports, 1)
		assert.Equal(t, rangeRef, reports[0].(map[string]interface{})["ref"])
	})

	t.Run("no reports for an unknown ref", func(t *testing.T) {
		_, parsed := doRequest(t, orgID, []string{untouchedSHA})
		reports := parsed["reports"].([]interface{})
		assert.Len(t, reports, 0)
	})

	t.Run("scoped by org - other org's review is invisible from orgID", func(t *testing.T) {
		_, parsed := doRequest(t, otherOrgID, []string{sharedSHA})
		reports := parsed["reports"].([]interface{})
		require.Len(t, reports, 1, "otherOrgID should only see its own review, not orgID's")
		assert.Equal(t, "manual", reports[0].(map[string]interface{})["trigger_type"])
	})

	t.Run("rejects empty commits", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/review-coverage", strings.NewReader(`{"commits":[]}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("org_id", orgID)

		err := s.ReviewCoverage(c)
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("rejects oversized commit lists", func(t *testing.T) {
		refs := make([]string, maxReviewCoverageCommits+1)
		for i := range refs {
			refs[i] = fmt.Sprintf("sha-%d", i)
		}
		rec, _ := doRequest(t, orgID, refs)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestInsertReviewCommits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	// Registered via t.Cleanup (not `defer`) so it runs *after* the delete
	// cleanup below: function-local defers unwind before t.Cleanup callbacks
	// fire, so a plain `defer db.Close()` here would close the connection
	// before the delete cleanup got to use it, leaving rows behind.
	t.Cleanup(func() { db.Close() })

	orgID := int64(1)
	var reviewID int64
	err = db.QueryRow(`
		INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, "test/insert-repo", "main", "", "", "completed", "cli_diff", "insert@example.com", "test", orgID).Scan(&reviewID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM review_commits WHERE review_id = $1", reviewID)
		_, _ = db.Exec("DELETE FROM public.reviews WHERE id = $1", reviewID)
	})

	s := &Server{db: db}
	err = s.insertReviewCommits(context.Background(), reviewID, orgID, []CommitRef{
		{Ref: "sha1", Type: "commit"},
		{Ref: "a..b", Type: "range"},
		{Ref: "", Type: "commit"}, // blank refs are skipped
	})
	require.NoError(t, err)

	rows, err := db.Query("SELECT ref, ref_type FROM review_commits WHERE review_id = $1 ORDER BY ref", reviewID)
	require.NoError(t, err)
	defer rows.Close()

	got := map[string]string{}
	for rows.Next() {
		var ref, refType string
		require.NoError(t, rows.Scan(&ref, &refType))
		got[ref] = refType
	}
	assert.Equal(t, map[string]string{"sha1": "commit", "a..b": "range"}, got)

	// Calling again is idempotent (ON CONFLICT DO NOTHING on review_id+ref).
	err = s.insertReviewCommits(context.Background(), reviewID, orgID, []CommitRef{{Ref: "sha1", Type: "commit"}})
	require.NoError(t, err)
}
