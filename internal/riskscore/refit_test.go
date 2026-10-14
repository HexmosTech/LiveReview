package riskscore

// DB-backed tests for the real-time online nudge (ApplyOutcome) and the
// batch refit's promotion gate (Refit) -- the two paths riskscore_test.go's
// pure-math tests can't reach, and the ones that actually answer "does the
// ML feedback loop do anything real." See docs/risk-policy-explainability.md.

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
	t.Helper()
	var orgID int64
	if err := db.QueryRow(`INSERT INTO orgs (name) VALUES ($1) RETURNING id`, name).Scan(&orgID); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM dashboard_cache WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM risk_factor_overrides WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM risk_model_versions WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM review_outcomes WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM risk_feature_snapshots WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM reviews WHERE org_id = $1`, orgID)
		_, _ = db.Exec(`DELETE FROM orgs WHERE id = $1`, orgID)
	})
	return orgID
}

func seedReview(t *testing.T, db *sql.DB, orgID int64) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id) VALUES ('riskscore/test-repo', 'completed', $1) RETURNING id`,
		orgID).Scan(&id)
	if err != nil {
		t.Fatalf("insert review: %v", err)
	}
	// Cascades risk_feature_snapshots/review_outcomes cleanup isn't automatic
	// (no ON DELETE CASCADE on those FKs), so seedOrg's cleanup above deletes
	// them by org_id before deleting reviews; no separate cleanup needed here.
	return id
}

// seedLabeledExample writes one risk_feature_snapshots + review_outcomes pair
// -- the join LoadTrainingSet reads.
func seedLabeledExample(t *testing.T, db *sql.DB, orgID int64, features []float64, outcomeType string) {
	t.Helper()
	reviewID := seedReview(t, db, orgID)
	raw := mustMarshal(t, features)
	if _, err := db.Exec(`
		INSERT INTO risk_feature_snapshots (review_id, org_id, model_version, features, score)
		VALUES ($1, $2, 1, $3, 0.5)`, reviewID, orgID, raw); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO review_outcomes (review_id, org_id, outcome_type, source, reason)
		VALUES ($1, $2, $3, 'test', 'seeded for refit test')`, reviewID, orgID, outcomeType); err != nil {
		t.Fatalf("insert outcome: %v", err)
	}
}

func mustMarshal(t *testing.T, v []float64) []byte {
	t.Helper()
	raw, err := MarshalWeights(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// --- Refit ---

func TestRefitNoOutcomesYetReturnsReason(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "refit-no-outcomes")
	store := NewStore(db)

	res, err := store.Refit(context.Background(), orgID)
	if err != nil {
		t.Fatalf("Refit: %v", err)
	}
	if res.Promoted {
		t.Fatalf("Refit with zero outcomes should never promote")
	}
	if res.TrainingCount != 0 {
		t.Fatalf("TrainingCount = %d, want 0", res.TrainingCount)
	}
	if res.Reason == "" {
		t.Fatalf("expected a non-empty reason explaining why nothing was promoted")
	}
}

// TestRefitLearnsAndPromotesOnSeparableData is the Refit-level proof that
// training actually moves the model: seed outcomes with a clear, deliberate
// pattern (high critical_count -> revert, low critical_count -> confirmed
// safe), refit, and confirm (a) it promotes, (b) the holdout AUC clears a real
// discrimination bar (not 0.5, i.e. not random), and (c) the *persisted,
// active* model -- reloaded from the DB exactly as production scoring would
// -- actually ranks a fresh high-critical review above a fresh low-critical
// one it was never trained on. LoadTrainingSet excludes outcomes with
// source='test', so these are seeded with source='ci' (a real, non-test
// source) to participate in training.
func TestRefitLearnsAndPromotesOnSeparableData(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "refit-separable")
	store := NewStore(db)

	seedSeparableOutcomes(t, db, orgID, 30)

	res, err := store.Refit(context.Background(), orgID)
	if err != nil {
		t.Fatalf("Refit: %v", err)
	}
	if !res.Promoted {
		t.Fatalf("expected promotion on cleanly separable training data, got reason: %s", res.Reason)
	}
	if res.HoldoutAUC < 0.75 {
		t.Fatalf("holdout AUC %.3f is too close to random (0.5) for cleanly separable data -- the fit doesn't appear to be learning anything", res.HoldoutAUC)
	}
	if res.TrainingCount != 30 {
		t.Fatalf("TrainingCount = %d, want 30", res.TrainingCount)
	}

	// Reload exactly as production scoring does (ActiveWeights, not the
	// in-memory `res`) and confirm the persisted model itself discriminates a
	// fresh pair of reviews it was never trained on.
	_, weights, sampleCount, err := store.ActiveWeights(context.Background(), orgID)
	if err != nil {
		t.Fatalf("ActiveWeights: %v", err)
	}
	if sampleCount != 30 {
		t.Fatalf("persisted training_sample_count = %d, want 30", sampleCount)
	}
	lowRisk := make([]float64, FeatureCount())
	highRisk := make([]float64, FeatureCount())
	highRisk[4] = 5 // critical_count -- the signal the seeded outcomes were keyed on
	lowScore := Score(weights, lowRisk, nil)
	highScore := Score(weights, highRisk, nil)
	if highScore <= lowScore {
		t.Fatalf("persisted model doesn't rank a high-critical review above a low-critical one (low=%.3f, high=%.3f) -- the learned pattern wasn't actually saved/activated", lowScore, highScore)
	}
}

// seedSeparableOutcomes writes n labeled examples with a deliberate pattern:
// even indices get a low critical_count and a confirmed_safe outcome, odd
// indices get a high critical_count and a revert outcome.
func seedSeparableOutcomes(t *testing.T, db *sql.DB, orgID int64, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		features := make([]float64, FeatureCount())
		outcome := "confirmed_safe"
		if i%2 == 1 {
			features[4] = float64(3 + i%3) // critical_count 3-5
			outcome = "revert"
		}
		seedLabeledExampleFromSource(t, db, orgID, features, outcome, "ci")
	}
}

func seedLabeledExampleFromSource(t *testing.T, db *sql.DB, orgID int64, features []float64, outcomeType, source string) {
	t.Helper()
	reviewID := seedReview(t, db, orgID)
	raw := mustMarshal(t, features)
	if _, err := db.Exec(`
		INSERT INTO risk_feature_snapshots (review_id, org_id, model_version, features, score)
		VALUES ($1, $2, 1, $3, 0.5)`, reviewID, orgID, raw); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO review_outcomes (review_id, org_id, outcome_type, source, reason)
		VALUES ($1, $2, $3, $4, 'seeded for refit test')`, reviewID, orgID, outcomeType, source); err != nil {
		t.Fatalf("insert outcome: %v", err)
	}
}

// TestRefitExcludesTestSourceOutcomes confirms the setup panel's "send test
// event" round trip (source='test') can never influence training, per
// docs/risk-policy-engine.md §10.
func TestRefitExcludesTestSourceOutcomes(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "refit-excludes-test-source")
	store := NewStore(db)

	seedLabeledExample(t, db, orgID, make([]float64, FeatureCount()), "revert") // source='test'

	examples, err := store.LoadTrainingSet(context.Background(), orgID)
	if err != nil {
		t.Fatalf("LoadTrainingSet: %v", err)
	}
	if len(examples) != 0 {
		t.Fatalf("LoadTrainingSet returned %d examples; source='test' rows must never be included", len(examples))
	}
}

// --- ApplyOutcome ---

// TestApplyOutcomeNudgesWeightsTowardTheReportedLabel confirms the real-time
// path: one reported outcome against a review's snapshot should move the
// corresponding feature's weight in the direction that outcome implies
// (revert -> nudge the risky features up), and persist+activate a new
// version immediately -- no batch job involved.
func TestApplyOutcomeNudgesWeightsTowardTheReportedLabel(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "apply-outcome-nudge")
	store := NewStore(db)
	ctx := context.Background()

	if err := store.EnsureSeedModel(ctx); err != nil {
		t.Fatalf("EnsureSeedModel: %v", err)
	}
	_, before, _, err := store.ActiveWeights(ctx, orgID)
	if err != nil {
		t.Fatalf("ActiveWeights (before): %v", err)
	}

	reviewID := seedReview(t, db, orgID)
	features := make([]float64, FeatureCount())
	features[4] = 5 // critical_count: a strong, specific signal to nudge on
	raw := mustMarshal(t, features)
	if _, err := db.Exec(`
		INSERT INTO risk_feature_snapshots (review_id, org_id, model_version, features, score)
		VALUES ($1, $2, 1, $3, 0.5)`, reviewID, orgID, raw); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	if err := store.ApplyOutcome(ctx, orgID, reviewID, "revert"); err != nil {
		t.Fatalf("ApplyOutcome: %v", err)
	}

	version, after, sampleCount, err := store.ActiveWeights(ctx, orgID)
	if err != nil {
		t.Fatalf("ActiveWeights (after): %v", err)
	}
	if sampleCount != 1 {
		t.Fatalf("training_sample_count after one outcome = %d, want 1", sampleCount)
	}
	if version == 0 {
		t.Fatalf("expected a new active version, got version 0")
	}
	// A positive (risky) label should push the critical_count weight up, not
	// down -- the single-step gradient direction the online nudge takes.
	if after[5] <= before[5] {
		t.Fatalf("one 'revert' outcome on a high-critical_count review should increase its weight: before=%.6f after=%.6f", before[5], after[5])
	}

	var updateKind string
	if err := db.QueryRow(`SELECT update_kind FROM risk_model_versions WHERE org_id = $1 AND is_active`, orgID).Scan(&updateKind); err != nil {
		t.Fatalf("query active version: %v", err)
	}
	if updateKind != "online_update" {
		t.Fatalf("active version's update_kind = %q, want %q", updateKind, "online_update")
	}
}

// TestApplyOutcomeWithNoSnapshotIsANoop confirms a review that was never
// scored (no risk_feature_snapshots row -- e.g. the policy was disabled when
// it ran) is silently skipped rather than erroring the caller's outcome
// insert.
func TestApplyOutcomeWithNoSnapshotIsANoop(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "apply-outcome-no-snapshot")
	store := NewStore(db)
	ctx := context.Background()

	reviewID := seedReview(t, db, orgID)
	if err := store.ApplyOutcome(ctx, orgID, reviewID, "revert"); err != nil {
		t.Fatalf("ApplyOutcome with no snapshot should be a no-op, got error: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM risk_model_versions WHERE org_id = $1`, orgID).Scan(&count); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no model version to be written when there's no snapshot to learn from, got %d", count)
	}
}
