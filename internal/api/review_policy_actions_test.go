package api

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"

	"github.com/livereview/internal/api/auth"
)

// Regression test: markReviewOutcome (POST /api/v1/reviews/:id/outcomes) used
// to write a review_outcomes row without ever checking that the review
// actually belongs to the caller's org — unlike its webhook twin
// (orgReviewOutcome), which does. A member of org B could report a
// revert/incident against any review id from org A just by guessing it. This
// boots the real route with the real middleware chain and a real API key, and
// confirms org B is now rejected (404) and no row is written.
func TestMarkReviewOutcomeRejectsCrossOrgReview(t *testing.T) {
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

	// The live dev server's background dashboard-cache refresh job may insert a
	// row for a new org before cleanup runs; that FK has no cascade, so it must
	// be cleared first or the org delete silently no-ops.
	cleanupOrg := func(orgID int64) {
		t.Cleanup(func() {
			_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
			_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
		})
	}

	// ---- org A owns the review ----
	var orgAID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('outcome-isolation-org-a') RETURNING id`).Scan(&orgAID); err != nil {
		t.Fatalf("insert org a: %v", err)
	}
	cleanupOrg(orgAID)

	var reviewID int64
	if err := db.QueryRow(`INSERT INTO reviews (repository, status, org_id, metadata) VALUES ('a/repo', 'completed', $1, '{}'::jsonb) RETURNING id`, orgAID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })

	// ---- org B is the attacker's own org, with its own owner + API key ----
	var orgBID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('outcome-isolation-org-b') RETURNING id`).Scan(&orgBID); err != nil {
		t.Fatalf("insert org b: %v", err)
	}
	cleanupOrg(orgBID)

	var userBID int64
	if err := db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ('outcome-isolation-b@example.com', 'x') RETURNING id`).Scan(&userBID); err != nil {
		t.Fatalf("insert user b: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userBID) })

	const ownerRoleID = 2 // seeded fixed row, see roles table
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, org_id, role_id) VALUES ($1, $2, $3)`, userBID, orgBID, ownerRoleID); err != nil {
		t.Fatalf("insert user_roles: %v", err)
	}
	// user_roles has no cascading FK from users/orgs, so it must be cleaned up
	// explicitly (and before the user/org deletes it's registered ahead of, since
	// t.Cleanup runs funcs in LIFO order) or a rerun collides on users_email_key.
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM user_roles WHERE user_id = $1 AND org_id = $2`, userBID, orgBID) })

	apiKeyMgr := NewAPIKeyManager(db)
	_, plainKey, err := apiKeyMgr.CreateAPIKey(userBID, orgBID, "outcome-isolation-b", []string{}, nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// ---- boot the real route with the real middleware chain ----
	s := &Server{db: db, echo: echo.New()}
	authMiddleware := auth.NewAuthMiddleware(nil, db)

	reviewsGroup := s.echo.Group("/api/v1/reviews")
	reviewsGroup.Use(RequireAuthOrAPIKey(nil, db))
	reviewsGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	reviewsGroup.Use(authMiddleware.ValidateOrgAccess())
	reviewsGroup.Use(authMiddleware.BuildPermissionContext())
	reviewsGroup.POST("/:id/outcomes", s.markReviewOutcome)

	srv := httptest.NewServer(s.echo)
	defer srv.Close()

	body := `{"outcome_type":"revert","reason":"cross-org isolation regression test"}`
	req, err := http.NewRequest(http.MethodPost,
		srv.URL+"/api/v1/reviews/"+strconv.FormatInt(reviewID, 10)+"/outcomes", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", plainKey)
	req.Header.Set("X-Org-Context", strconv.FormatInt(orgBID, 10))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 (review not found in this organization), got %d", resp.StatusCode)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_outcomes WHERE review_id = $1`, reviewID).Scan(&count); err != nil {
		t.Fatalf("query review_outcomes: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no review_outcomes row to be written across orgs, found %d", count)
	}
}
