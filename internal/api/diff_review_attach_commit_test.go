package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAttachDiffReviewCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}

	db, err := sql.Open("postgres", "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	orgID := int64(1)
	otherOrgID := int64(999998)
	_, _ = db.Exec(`INSERT INTO orgs (id, name) VALUES ($1, 'attach-commit-test-other-org') ON CONFLICT (id) DO NOTHING`, otherOrgID)

	insertReview := func(t *testing.T, org int64, triggerType, status string) int64 {
		t.Helper()
		var reviewID int64
		err := db.QueryRow(`
			INSERT INTO public.reviews (repository, branch, commit_hash, pr_mr_url, status, trigger_type, user_email, provider, org_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, "test/attach-commit-repo", "main", "", "", status, triggerType, "attach@example.com", "test", org).Scan(&reviewID)
		require.NoError(t, err)
		return reviewID
	}

	cliReviewID := insertReview(t, orgID, "cli_diff", "completed")
	mrReviewID := insertReview(t, orgID, "manual", "completed")
	otherOrgReviewID := insertReview(t, otherOrgID, "cli_diff", "completed")

	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM review_commits WHERE review_id IN ($1, $2, $3)", cliReviewID, mrReviewID, otherOrgReviewID)
		_, _ = db.Exec("DELETE FROM public.reviews WHERE id IN ($1, $2, $3)", cliReviewID, mrReviewID, otherOrgReviewID)
		_, _ = db.Exec("DELETE FROM orgs WHERE id = $1", otherOrgID)
	})

	s := &Server{db: db}
	e := echo.New()

	doRequest := func(t *testing.T, org, reviewID int64, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/diff-review/x/commit", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("review_id")
		c.SetParamValues(strconv.FormatInt(reviewID, 10))
		c.Set("org_id", org)

		err := s.AttachDiffReviewCommit(c)
		require.NoError(t, err)
		return rec
	}

	t.Run("attaches a commit to a cli_diff review", func(t *testing.T) {
		rec := doRequest(t, orgID, cliReviewID, `{"commit_sha":"deadbeefcafedeadbeefcafedeadbeefcafedead"}`)
		assert.Equal(t, http.StatusOK, rec.Code)

		var count int
		require.NoError(t, db.QueryRow(
			"SELECT COUNT(*) FROM review_commits WHERE review_id = $1 AND ref = $2",
			cliReviewID, "deadbeefcafedeadbeefcafedeadbeefcafedead",
		).Scan(&count))
		assert.Equal(t, 1, count)
	})

	t.Run("idempotent on repeat", func(t *testing.T) {
		rec := doRequest(t, orgID, cliReviewID, `{"commit_sha":"deadbeefcafedeadbeefcafedeadbeefcafedead"}`)
		assert.Equal(t, http.StatusOK, rec.Code)

		var count int
		require.NoError(t, db.QueryRow(
			"SELECT COUNT(*) FROM review_commits WHERE review_id = $1 AND ref = $2",
			cliReviewID, "deadbeefcafedeadbeefcafedeadbeefcafedead",
		).Scan(&count))
		assert.Equal(t, 1, count, "repeating the same attach must not duplicate the row")
	})

	t.Run("rejects non-cli_diff trigger types", func(t *testing.T) {
		rec := doRequest(t, orgID, mrReviewID, `{"commit_sha":"abc123"}`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var count int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM review_commits WHERE review_id = $1", mrReviewID).Scan(&count))
		assert.Equal(t, 0, count)
	})

	t.Run("404s on org mismatch, does not leak existence", func(t *testing.T) {
		rec := doRequest(t, orgID, otherOrgReviewID, `{"commit_sha":"shouldnotattach"}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var count int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM review_commits WHERE review_id = $1", otherOrgReviewID).Scan(&count))
		assert.Equal(t, 0, count, "wrong-org caller must not be able to attach a commit")
	})

	t.Run("rejects empty commit_sha", func(t *testing.T) {
		rec := doRequest(t, orgID, cliReviewID, `{"commit_sha":""}`)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("404s on nonexistent review", func(t *testing.T) {
		rec := doRequest(t, orgID, 99999999, `{"commit_sha":"abc123"}`)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}
