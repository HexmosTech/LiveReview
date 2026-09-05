package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"

	"github.com/livereview/internal/api/auth"
)

// TestCIGateGeneratedScriptE2E boots the REAL review-coverage + ci-rulesets
// HTTP routes (full auth middleware chain included) against a real Postgres
// DB, then runs the ACTUAL bash script text the UI hands to users (built the
// same way ui/src/pages/CiRulesets/shared.tsx's buildCurlSnippet does) via a
// real subprocess, asserting the real process exit codes a CI runner would
// see. This is the most realistic validation achievable in this environment
// without a public endpoint: no tunnel tool (ngrok/cloudflared) or `act` is
// available here, and GitHub-hosted Actions runners cannot reach localhost,
// so an actual GitHub Actions run against this server was not possible.
//
// Also verifies the `X-Org-Context` header the generated script sends:
// RequireAuthOrAPIKey actually overwrites this header itself from the API
// key's own org before BuildOrgContextFromHeader ever reads it, so it isn't
// strictly load-bearing for a single-org API key today — but every other
// LiveReview-authenticated endpoint requires the header to be present at
// all, so the script sends it explicitly rather than relying on that
// overwrite behavior, which is an internal implementation detail that could
// change.
func TestCIGateGeneratedScriptE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping database integration test")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://livereview:livereview_password_123@localhost:5432/livereview?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available (the generated script requires it, same as a real CI runner)")
	}

	// ---- seed a real org + owner user + API key ----
	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('ci-gate-e2e-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID) })

	var userID int64
	if err := db.QueryRow(`INSERT INTO users (email, password_hash) VALUES ('ci-gate-e2e@example.com', 'x') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID) })

	const ownerRoleID = 2 // seeded fixed row, see roles table
	if _, err := db.Exec(`INSERT INTO user_roles (user_id, org_id, role_id) VALUES ($1, $2, $3)`, userID, orgID, ownerRoleID); err != nil {
		t.Fatalf("insert user_roles: %v", err)
	}

	apiKeyMgr := NewAPIKeyManager(db)
	_, plainKey, err := apiKeyMgr.CreateAPIKey(userID, orgID, "ci-gate-e2e", []string{}, nil)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// ---- seed a completed review with one critical finding, reachable by commit SHA ----
	const commitSHA = "cigatee2eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	metadata := map[string]interface{}{
		"review_result": map[string]interface{}{
			"comments": []map[string]interface{}{
				{"severity": "critical", "confidence": "high", "category": "security", "subcategory": "sql-injection", "type": "bug", "file_path": "a.go", "line": 10},
			},
		},
	}
	metaJSON, _ := json.Marshal(metadata)
	var reviewID int64
	if err := db.QueryRow(`
		INSERT INTO reviews (repository, commit_hash, status, trigger_type, org_id, metadata)
		VALUES ($1, $2, 'completed', 'manual', $3, $4::jsonb) RETURNING id`,
		"ci-gate-e2e/repo", commitSHA, orgID, string(metaJSON)).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, reviewID) })

	if err := insertReviewCommitsTx(context.Background(), db, reviewID, orgID, nil, []CommitRef{{Ref: commitSHA, Type: "commit"}}); err != nil {
		t.Fatalf("insert review_commits: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM review_commits WHERE review_id = $1`, reviewID) })

	// ---- seed two rulesets: one that blocks, one that allows ----
	var blockRulesetID, allowRulesetID int64
	if err := db.QueryRow(`INSERT INTO ci_rulesets (org_id, name, jq_expr) VALUES ($1, 'blocks', '.counts.by_severity.critical > 0') RETURNING id`, orgID).Scan(&blockRulesetID); err != nil {
		t.Fatalf("insert block ruleset: %v", err)
	}
	if err := db.QueryRow(`INSERT INTO ci_rulesets (org_id, name, jq_expr) VALUES ($1, 'allows', 'false') RETURNING id`, orgID).Scan(&allowRulesetID); err != nil {
		t.Fatalf("insert allow ruleset: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM ci_rulesets WHERE id IN ($1, $2)`, blockRulesetID, allowRulesetID) })

	// ---- boot the real server with the real middleware chain ----
	s := &Server{db: db, echo: echo.New()}
	authMiddleware := auth.NewAuthMiddleware(nil, db)

	reviewCoverageGroup := s.echo.Group("/api/v1/review-coverage")
	reviewCoverageGroup.Use(RequireAuthOrAPIKey(nil, db))
	reviewCoverageGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	reviewCoverageGroup.Use(authMiddleware.ValidateOrgAccess())
	reviewCoverageGroup.Use(authMiddleware.BuildPermissionContext())
	reviewCoverageGroup.POST("", s.ReviewCoverage)

	ciRulesetsHandler := NewCIRulesetsHandler(db)
	ciRulesetsGroup := s.echo.Group("/api/v1/ci-rulesets")
	ciRulesetsGroup.Use(RequireAuthOrAPIKey(nil, db))
	ciRulesetsGroup.Use(authMiddleware.BuildOrgContextFromHeader())
	ciRulesetsGroup.Use(authMiddleware.ValidateOrgAccess())
	ciRulesetsGroup.Use(authMiddleware.BuildPermissionContext())
	ciRulesetsGroup.GET("/:id/evaluate", ciRulesetsHandler.Evaluate)

	srv := httptest.NewServer(s.echo)
	defer srv.Close()

	// ---- write the EXACT script text the UI generates (kept in sync by hand
	// with ui/src/pages/CiRulesets/shared.tsx's buildCurlSnippet) and run it
	// as a real subprocess against the real running server.
	buildScript := func(rulesetID int64, sha string) string {
		return "#!/usr/bin/env bash\n" +
			"set -euo pipefail\n\n" +
			"ORG_ID=" + strconv.FormatInt(orgID, 10) + "\n\n" +
			`REVIEW_ID=$(curl -sf -H "X-API-Key: $LIVEREVIEW_API_KEY" \` + "\n" +
			`  -H "X-Org-Context: $ORG_ID" \` + "\n" +
			`  -H "Content-Type: application/json" \` + "\n" +
			`  -d "{\"commits\":[\"` + sha + `\"]}" \` + "\n" +
			`  "` + srv.URL + `/api/v1/review-coverage" | jq -r '.reports[0].review_id // empty')` + "\n\n" +
			`if [ -z "$REVIEW_ID" ]; then` + "\n" +
			`  echo "No LiveReview review found for this commit yet; skipping gate."` + "\n" +
			`  exit 0` + "\n" +
			`fi` + "\n\n" +
			`curl -sf -H "X-API-Key: $LIVEREVIEW_API_KEY" -H "X-Org-Context: $ORG_ID" \` + "\n" +
			`  "` + srv.URL + `/api/v1/ci-rulesets/` + strconv.FormatInt(rulesetID, 10) + `/evaluate?review_id=$REVIEW_ID" \` + "\n" +
			`  || { echo "LiveReview gate blocked this build"; exit 1; }` + "\n"
	}

	runScript := func(t *testing.T, script string) int {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, "gate.sh")
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write script: %v", err)
		}
		cmd := exec.Command("bash", path)
		cmd.Env = append(os.Environ(), "LIVEREVIEW_API_KEY="+plainKey)
		out, err := cmd.CombinedOutput()
		t.Logf("script output:\n%s", out)
		if err == nil {
			return 0
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		t.Fatalf("run script: %v", err)
		return -1
	}

	t.Run("blocks on a critical finding", func(t *testing.T) {
		code := runScript(t, buildScript(blockRulesetID, commitSHA))
		if code != 1 {
			t.Fatalf("expected exit 1 (blocked), got %d", code)
		}
	})

	t.Run("allows when the rule is false", func(t *testing.T) {
		code := runScript(t, buildScript(allowRulesetID, commitSHA))
		if code != 0 {
			t.Fatalf("expected exit 0 (allowed), got %d", code)
		}
	})

	t.Run("skips gracefully when no review covers the commit", func(t *testing.T) {
		code := runScript(t, buildScript(blockRulesetID, "0000000000000000000000000000000000000000"))
		if code != 0 {
			t.Fatalf("expected exit 0 (skip, no review found), got %d", code)
		}
	})
}
