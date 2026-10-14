package api

// Handler-level test for POST .../auto-approve-policies/simulate (see
// docs/risk-policy-engine.md §8): the "what would've happened" panel the
// Settings UI's Risk Model sub-tab calls. Confirms (a) it scores real
// reviews through the real funnel and returns the expected per-review
// verdicts, and (b) its dry-run guarantee actually holds -- no
// review_policy_log or risk_feature_snapshots rows are written, since the UI
// depends on simulate being side-effect-free to safely preview an unsaved
// draft policy against production data.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

func simulateTestDB(t *testing.T) *sql.DB {
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

func TestSimulateScoresRealReviewsWithNoSideEffects(t *testing.T) {
	db := simulateTestDB(t)

	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ('simulate-handler-test') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM risk_feature_snapshots WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM review_policy_log WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM reviews WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})

	safeMetadata := []byte(`{"review_result": {"comments": [], "diffClassification": {"class": "refactor-only", "confidence": 9}}}`)
	// file_path/line are required: TaxonomyReportStore.ListFindings only
	// counts rows that "look like persisted line findings" (see its
	// buildWhereClause), so a comment without them is silently excluded.
	riskyMetadata := []byte(`{"review_result": {"comments": [{"severity":"critical","confidence":"high","category":"security","type":"issue","file_path":"a.go","line":1}], "diffClassification": {"class": "logic-change", "confidence": 9}}}`)

	var safeID, riskyID int64
	if err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, metadata) VALUES ('simulate/test-repo', 'completed', $1, $2) RETURNING id`,
		orgID, safeMetadata).Scan(&safeID); err != nil {
		t.Fatalf("insert safe review: %v", err)
	}
	if err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, metadata) VALUES ('simulate/test-repo', 'completed', $1, $2) RETURNING id`,
		orgID, riskyMetadata).Scan(&riskyID); err != nil {
		t.Fatalf("insert risky review: %v", err)
	}

	handler := NewAutoApprovePoliciesHandler(db)

	body, _ := json.Marshal(map[string]interface{}{
		"review_ids": []int64{safeID, riskyID},
	})
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auto-approve-policies/simulate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("org_id", orgID)

	if err := handler.Simulate(c); err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Rows []struct {
			ReviewID     int64   `json:"review_id"`
			WouldApprove bool    `json:"would_approve"`
			BlockedAt    string  `json:"blocked_at,omitempty"`
			Score        float64 `json:"score,omitempty"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %s: %v", rec.Body.String(), err)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("got %d rows, want 2 (response: %s)", len(resp.Rows), rec.Body.String())
	}

	byID := map[int64]bool{}
	blockedAt := map[int64]string{}
	for _, r := range resp.Rows {
		byID[r.ReviewID] = r.WouldApprove
		blockedAt[r.ReviewID] = r.BlockedAt
	}
	if !byID[safeID] {
		t.Fatalf("safe review %d: would_approve = false, want true", safeID)
	}
	if byID[riskyID] {
		t.Fatalf("risky review %d: would_approve = true, want false", riskyID)
	}
	if blockedAt[riskyID] != "risk" {
		t.Fatalf("risky review %d: blocked_at = %q, want %q", riskyID, blockedAt[riskyID], "risk")
	}

	// The dry-run guarantee: simulate must never write review_policy_log or
	// risk_feature_snapshots rows, since the UI relies on being able to
	// preview an unsaved draft policy against real production reviews
	// without side effects.
	var logCount, snapshotCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM review_policy_log WHERE org_id = $1`, orgID).Scan(&logCount); err != nil {
		t.Fatalf("count review_policy_log: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM risk_feature_snapshots WHERE org_id = $1`, orgID).Scan(&snapshotCount); err != nil {
		t.Fatalf("count risk_feature_snapshots: %v", err)
	}
	if logCount != 0 {
		t.Fatalf("simulate wrote %d review_policy_log rows, want 0 (dry-run must have no side effects)", logCount)
	}
	if snapshotCount != 0 {
		t.Fatalf("simulate wrote %d risk_feature_snapshots rows, want 0 (dry-run must have no side effects)", snapshotCount)
	}
}
