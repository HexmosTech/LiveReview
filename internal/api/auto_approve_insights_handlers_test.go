package api

// Handler-level test for GET .../auto-approve-insights -- the "Auto Approve"
// Reports tab's data source. Confirms (a) the aggregation math is correct
// against hand-seeded review_policy_log/risk_feature_snapshots/
// review_outcomes/review_policy_overrides rows, and (b) tenant isolation: an
// org can never see another org's counts, per CLAUDE.md's scoping rules.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func seedInsightsOrg(t *testing.T, db *sql.DB, name string) int64 {
	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ($1) RETURNING id`, name).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM review_outcomes WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM review_policy_overrides WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM review_policy_log WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM risk_feature_snapshots WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM reviews WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})
	return orgID
}

func seedInsightsReview(t *testing.T, db *sql.DB, orgID int64) int64 {
	var reviewID int64
	if err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, metadata) VALUES ('insights/test-repo', 'completed', $1, '{}') RETURNING id`,
		orgID).Scan(&reviewID); err != nil {
		t.Fatalf("insert review: %v", err)
	}
	return reviewID
}

func TestAutoApproveInsightsAggregatesAndIsolatesByOrg(t *testing.T) {
	db := simulateTestDB(t)

	orgA := seedInsightsOrg(t, db, "insights-handler-test-a")
	orgB := seedInsightsOrg(t, db, "insights-handler-test-b")

	// Org A: 2 approved, 1 blocked at risk (also overridden), 1 blocked at
	// eligibility. Plus one reported outcome on an approved review that
	// turned out to be an incident -- the false-approve signal.
	approved1 := seedInsightsReview(t, db, orgA)
	approved2 := seedInsightsReview(t, db, orgA)
	blockedAtRisk := seedInsightsReview(t, db, orgA)
	blockedAtEligibility := seedInsightsReview(t, db, orgA)

	for _, r := range []struct {
		reviewID int64
		stage    string
		verdict  string
	}{
		{approved1, "eligibility", "pass"}, {approved1, "static", "pass"}, {approved1, "risk", "pass"}, {approved1, "llm", "pass"}, {approved1, "final", "approved"},
		{approved2, "eligibility", "pass"}, {approved2, "static", "pass"}, {approved2, "risk", "pass"}, {approved2, "llm", "pass"}, {approved2, "final", "approved"},
		{blockedAtRisk, "eligibility", "pass"}, {blockedAtRisk, "static", "pass"}, {blockedAtRisk, "risk", "fail"}, {blockedAtRisk, "llm", "pass"}, {blockedAtRisk, "final", "blocked"},
		{blockedAtEligibility, "eligibility", "fail"}, {blockedAtEligibility, "static", "skip"}, {blockedAtEligibility, "risk", "skip"}, {blockedAtEligibility, "llm", "skip"}, {blockedAtEligibility, "final", "blocked"},
	} {
		if _, err := db.Exec(`
			INSERT INTO review_policy_log (review_id, org_id, stage, verdict, detail) VALUES ($1, $2, $3, $4, '{}')`,
			r.reviewID, orgA, r.stage, r.verdict); err != nil {
			t.Fatalf("seed policy log: %v", err)
		}
	}

	var actorUserID int64
	if err := db.QueryRow(`
		INSERT INTO users (email, password_hash) VALUES ('insights-handler-test@example.com', 'x') RETURNING id`).Scan(&actorUserID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM review_policy_overrides WHERE actor_user_id = $1`, actorUserID)
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, actorUserID)
	})

	if _, err := db.Exec(`
		INSERT INTO review_policy_overrides (review_id, org_id, actor_user_id, original_verdict, forced_verdict, reason)
		VALUES ($1, $2, $3, 'blocked', 'approved', 'test override')`, blockedAtRisk, orgA, actorUserID); err != nil {
		t.Fatalf("seed override: %v", err)
	}

	if _, err := db.Exec(`
		INSERT INTO review_outcomes (review_id, org_id, outcome_type, source, reason) VALUES ($1, $2, 'incident', 'manual', 'test outcome')`,
		approved1, orgA); err != nil {
		t.Fatalf("seed outcome: %v", err)
	}

	for _, s := range []float64{0.1, 0.2, 0.9} {
		if _, err := db.Exec(`
			INSERT INTO risk_feature_snapshots (review_id, org_id, model_version, features, score) VALUES ($1, $2, 1, '{}', $3)`,
			approved1, orgA, s); err != nil {
			t.Fatalf("seed feature snapshot: %v", err)
		}
	}

	// Org B: one isolated approved review -- must never show up in org A's
	// results, and org A's data must never show up when org B is queried.
	orgBReview := seedInsightsReview(t, db, orgB)
	if _, err := db.Exec(`
		INSERT INTO review_policy_log (review_id, org_id, stage, verdict, detail) VALUES ($1, $2, 'final', 'approved', '{}')`,
		orgBReview, orgB); err != nil {
		t.Fatalf("seed org B policy log: %v", err)
	}

	handler := NewAutoApproveInsightsHandler(db)
	e := echo.New()

	getInsights := func(orgID int64) autoApproveInsightsDTO {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auto-approve-insights?window=30", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set("org_id", orgID)
		if err := handler.Insights(c); err != nil {
			t.Fatalf("Insights: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Insights autoApproveInsightsDTO `json:"insights"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp.Insights
	}

	a := getInsights(orgA)

	if a.Decisions.Approved != 2 {
		t.Errorf("org A approved: got %d, want 2", a.Decisions.Approved)
	}
	if a.Decisions.TotalDecided != 4 {
		t.Errorf("org A total decided: got %d, want 4", a.Decisions.TotalDecided)
	}
	if a.Decisions.BlockedAtRisk != 1 {
		t.Errorf("org A blocked at risk: got %d, want 1", a.Decisions.BlockedAtRisk)
	}
	if a.Decisions.BlockedAtEligibility != 1 {
		t.Errorf("org A blocked at eligibility: got %d, want 1", a.Decisions.BlockedAtEligibility)
	}
	if a.Decisions.Overridden != 1 {
		t.Errorf("org A overridden: got %d, want 1", a.Decisions.Overridden)
	}
	if a.Decisions.OverrideRate != 0.25 {
		t.Errorf("org A override rate: got %v, want 0.25", a.Decisions.OverrideRate)
	}
	if a.ScoreDistribution.TotalScored != 3 {
		t.Errorf("org A total scored: got %d, want 3", a.ScoreDistribution.TotalScored)
	}
	if a.Calibration.TotalOutcomes != 1 {
		t.Errorf("org A total outcomes: got %d, want 1", a.Calibration.TotalOutcomes)
	}
	if a.Calibration.FalseApproveRate == nil || *a.Calibration.FalseApproveRate != 1.0 {
		t.Errorf("org A false approve rate: got %v, want 1.0", a.Calibration.FalseApproveRate)
	}
	if !a.Calibration.LowVolumeWarning {
		t.Errorf("org A calibration should warn on low volume (1 outcome < %d)", lowVolumeThreshold)
	}

	b := getInsights(orgB)
	if b.Decisions.Approved != 1 {
		t.Errorf("org B approved: got %d, want 1 (tenant isolation leak?)", b.Decisions.Approved)
	}
	if b.Decisions.TotalDecided != 1 {
		t.Errorf("org B total decided: got %d, want 1 (tenant isolation leak?)", b.Decisions.TotalDecided)
	}
	if b.Decisions.Overridden != 0 {
		t.Errorf("org B overridden: got %d, want 0 -- org A's override leaked across tenants", b.Decisions.Overridden)
	}
	if b.ScoreDistribution.TotalScored != 0 {
		t.Errorf("org B total scored: got %d, want 0 -- org A's snapshots leaked across tenants", b.ScoreDistribution.TotalScored)
	}
	if b.Calibration.TotalOutcomes != 0 {
		t.Errorf("org B total outcomes: got %d, want 0 -- org A's outcomes leaked across tenants", b.Calibration.TotalOutcomes)
	}
}
