package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// TestCIRulesetsEvaluateAgainstRealDB exercises buildCanonicalReviewDoc and
// the CRUD queries against a real Postgres instance (DATABASE_URL), since
// runJQBool's unit test above doesn't touch the reviews/ci_rulesets SQL at
// all. Skips if DATABASE_URL isn't set (e.g. CI without a DB configured).
func TestCIRulesetsEvaluateAgainstRealDB(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB-backed integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("db not reachable: %v", err)
	}

	ctx := context.Background()
	const orgID = int64(1)

	metadata := map[string]interface{}{
		"review_result": map[string]interface{}{
			"comments": []map[string]interface{}{
				{"severity": "critical", "confidence": "high", "category": "security", "subcategory": "sql-injection", "type": "bug", "file_path": "a.go", "line": 10},
				{"severity": "low", "confidence": "medium", "category": "style", "subcategory": "naming", "type": "suggestion", "file_path": "b.go", "line": 20},
			},
		},
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	var reviewID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO reviews (repository, status, trigger_type, org_id, metadata)
		VALUES ($1, 'completed', 'manual', $2, $3::jsonb) RETURNING id`,
		"ci-ruleset-test/repo", orgID, string(metaJSON)).Scan(&reviewID)
	if err != nil {
		t.Fatalf("insert test review: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM reviews WHERE id = $1`, reviewID)
	})

	h := NewCIRulesetsHandler(db)

	doc, err := h.buildCanonicalReviewDoc(ctx, orgID, reviewID)
	if err != nil {
		t.Fatalf("buildCanonicalReviewDoc: %v", err)
	}
	if doc.Counts.Total != 2 {
		t.Fatalf("expected 2 findings, got %d (findings=%+v)", doc.Counts.Total, doc.Findings)
	}
	if doc.Counts.BySeverity["critical"] != 1 {
		t.Fatalf("expected 1 critical finding, got %d", doc.Counts.BySeverity["critical"])
	}
	if doc.Counts.ByCategory["security"] != 1 {
		t.Fatalf("expected 1 security finding, got %d", doc.Counts.ByCategory["security"])
	}
	if doc.Status != "completed" {
		t.Fatalf("expected status completed, got %q", doc.Status)
	}

	blocked, _, err := runJQBool(".counts.by_severity.critical > 0", doc)
	if err != nil {
		t.Fatalf("runJQBool: %v", err)
	}
	if !blocked {
		t.Fatalf("expected block=true for a review with a critical finding")
	}

	// Wrong org must not resolve the review (tenant isolation).
	if _, err := h.buildCanonicalReviewDoc(ctx, orgID+999, reviewID); err == nil {
		t.Fatalf("expected error resolving review under a different org, got none")
	}

	// ---- ruleset CRUD round-trip ----
	var rulesetID int64
	err = db.QueryRowContext(ctx, `
		INSERT INTO ci_rulesets (org_id, name, description, jq_expr)
		VALUES ($1, 'test ruleset', 'created by integration test', '.counts.by_severity.critical > 0')
		RETURNING id`, orgID).Scan(&rulesetID)
	if err != nil {
		t.Fatalf("insert ruleset: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM ci_rulesets WHERE id = $1`, rulesetID)
	})

	rs, err := h.getRuleset(ctx, orgID, rulesetID)
	if err != nil {
		t.Fatalf("getRuleset: %v", err)
	}
	if rs.Name != "test ruleset" {
		t.Fatalf("expected name 'test ruleset', got %q", rs.Name)
	}

	if _, err := h.getRuleset(ctx, orgID+999, rulesetID); err == nil {
		t.Fatalf("expected error resolving ruleset under a different org, got none")
	}
}
