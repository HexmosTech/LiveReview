package policyengine

// Scenario-corpus verification: see docs/risk-policy-explainability.md and the
// plan at docs/risk-policy-engine.md's "Active plan" section. Each case in
// testdata/scenarios.json is a plain-data recipe for one review's inputs plus
// the expected funnel outcome; this file's job is only to seed those inputs
// into real tables and run the actual production call, engine.RunWithPolicy
// -- never a parallel reimplementation of funnel logic. Every row this file
// writes is cleaned up via t.Cleanup (LIFO, registered right after each
// insert) regardless of pass/fail, so repeated runs -- including against a
// shared/live database -- leave zero residue.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/livereview/internal/blastradius"
	"github.com/livereview/internal/riskscore"
	"github.com/livereview/internal/riskscore/referencemix"
)

// --- scenario data shape (mirrors testdata/scenarios.json) ---

type scenarioHunk struct {
	Combined float64 `json:"combined"`
	NewLines int     `json:"new_lines"`
	FilePath string  `json:"file_path"`
}

type scenarioFinding struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Type     string `json:"type"`
	Count    int    `json:"count"`
}

type scenarioLLMClass struct {
	Class      string `json:"class"`
	Confidence int    `json:"confidence"`
}

type scenarioDiff struct {
	FileCount  int `json:"file_count"`
	LinesAdded int `json:"lines_added"`
}

type scenarioPolicyOverrides struct {
	Mode                    string   `json:"mode"`
	MinAuthorLevel          *int     `json:"min_author_level"`
	RiskThresholdPercentile *int     `json:"risk_threshold_percentile"`
	RequireLLMConfidence    *int     `json:"require_llm_confidence"`
	DailyApproveCap         *int     `json:"daily_approve_cap"`
	BlockingRulesets        []string `json:"blocking_rulesets"` // names, resolved against Scenario.Rulesets
}

type scenarioRuleset struct {
	Name   string `json:"name"`
	JQExpr string `json:"jq_expr"`
}

type scenarioWant struct {
	Approve        bool   `json:"approve"`
	BlockedAt      string `json:"blocked_at"`
	ReasonContains string `json:"reason_contains"`
}

// Scenario is one "common sense" case: a recipe for seeding a single review's
// inputs, plus the expected funnel outcome.
type Scenario struct {
	Name      string `json:"name"`
	Rationale string `json:"rationale"`
	// Bucket groups scenarios for the verification report (e.g. "eligibility",
	// "llm-gate") -- an explicit field in the data, not a name-prefix guess a
	// report-generation script would otherwise have to re-derive.
	Bucket             string                  `json:"bucket"`
	BlastHunks         []scenarioHunk          `json:"blast_hunks"`
	Findings           []scenarioFinding       `json:"findings"`
	LLMClass           *scenarioLLMClass       `json:"llm_class"`
	Diff               scenarioDiff            `json:"diff"`
	AuthorLevel        int                     `json:"author_level"`
	AuthorIsBot        bool                    `json:"author_is_bot"`
	BotUsername        string                  `json:"bot_username"`
	AuthorUnmapped     bool                    `json:"author_unmapped"`
	Whitelisted        bool                    `json:"whitelisted"`
	Policy             scenarioPolicyOverrides `json:"policy"`
	Rulesets           []scenarioRuleset       `json:"rulesets"`
	SeedRecentScores   []float64               `json:"seed_recent_scores"`
	SeedApprovalsToday int                     `json:"seed_approvals_today"`
	Want               scenarioWant            `json:"want"`
	// Skip, when set, is why this scenario is skipped for now.
	Skip string `json:"skip"`
}

func loadScenarios(t *testing.T) []Scenario {
	t.Helper()
	raw, err := os.ReadFile("testdata/scenarios.json")
	if err != nil {
		t.Fatalf("reading scenarios.json: %v", err)
	}
	var out []Scenario
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parsing scenarios.json: %v", err)
	}
	return out
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func sanitize(name string) string {
	return strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// --- Policy construction (defaults mirror auto_approve_policies' column defaults) ---

func (sc Scenario) buildPolicy(whitelistUserID int64, rulesetIDs []int64) *Policy {
	p := &Policy{
		Mode:                    ModeCustom,
		MinAuthorLevel:          1,
		RiskThresholdPercentile: 20,
		RequireLLMConfidence:    8,
		ForceApproveMinLevel:    3,
	}
	if sc.Policy.Mode != "" {
		p.Mode = sc.Policy.Mode
	}
	if sc.Policy.MinAuthorLevel != nil {
		p.MinAuthorLevel = *sc.Policy.MinAuthorLevel
	}
	if sc.Policy.RiskThresholdPercentile != nil {
		p.RiskThresholdPercentile = *sc.Policy.RiskThresholdPercentile
	}
	if sc.Policy.RequireLLMConfidence != nil {
		p.RequireLLMConfidence = *sc.Policy.RequireLLMConfidence
	}
	if sc.Policy.DailyApproveCap != nil {
		p.DailyApproveCap = sc.Policy.DailyApproveCap
	}
	if sc.Whitelisted && whitelistUserID > 0 {
		p.EligibilityWhitelistUserIDs = []int64{whitelistUserID}
	}
	p.BlockingRulesetIDs = rulesetIDs
	return p
}

// insertRuleset creates a ci_rulesets row (the same table/rows CI/CD gates
// use) and returns its id.
func insertRuleset(t *testing.T, db *sql.DB, orgID int64, name, jqExpr string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO ci_rulesets (org_id, name, jq_expr) VALUES ($1, $2, $3) RETURNING id`,
		orgID, name, jqExpr).Scan(&id)
	if err != nil {
		t.Fatalf("insert ci_ruleset: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM ci_rulesets WHERE id = $1`, id) })
	return id
}

// resolveRulesets creates every ruleset a scenario declares and resolves its
// policy.blocking_rulesets names to the ids the funnel actually reads.
func resolveRulesets(t *testing.T, db *sql.DB, orgID int64, sc Scenario) []int64 {
	t.Helper()
	byName := map[string]int64{}
	for _, rs := range sc.Rulesets {
		byName[rs.Name] = insertRuleset(t, db, orgID, rs.Name, rs.JQExpr)
	}
	var ids []int64
	for _, name := range sc.Policy.BlockingRulesets {
		id, ok := byName[name]
		if !ok {
			t.Fatalf("policy.blocking_rulesets references undeclared ruleset %q", name)
		}
		ids = append(ids, id)
	}
	return ids
}

// --- DB seeding helpers (own cleanup, LIFO-safe per the convention in policy_test.go) ---

func insertUser(t *testing.T, db *sql.DB, email string, reviewLevel int) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO users (email, password_hash, review_level) VALUES ($1, 'x', $2) RETURNING id`,
		email, reviewLevel).Scan(&id)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, id) })
	return id
}

func insertUserIdentity(t *testing.T, db *sql.DB, orgID, userID int64, kind, value string) {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO user_identities (org_id, user_id, kind, value) VALUES ($1, $2, $3, $4) RETURNING id`,
		orgID, userID, kind, value).Scan(&id)
	if err != nil {
		t.Fatalf("insert user_identity: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM user_identities WHERE id = $1`, id) })
}

// buildReviewMetadata renders a scenario's findings + LLM classification as the
// review_result JSONB blob canonicaldoc.Build reads from.
func buildReviewMetadata(sc Scenario) []byte {
	comments := []map[string]interface{}{}
	fileIdx := 0
	for _, f := range sc.Findings {
		findingType := f.Type
		if findingType == "" {
			findingType = "issue"
		}
		for i := 0; i < f.Count; i++ {
			comments = append(comments, map[string]interface{}{
				"severity":   f.Severity,
				"confidence": "high",
				"category":   f.Category,
				"type":       findingType,
				"file_path":  fmt.Sprintf("finding-%d.go", fileIdx),
				"line":       1,
			})
			fileIdx++
		}
	}
	reviewResult := map[string]interface{}{"comments": comments}
	if sc.LLMClass != nil {
		reviewResult["diffClassification"] = map[string]interface{}{
			"class":      sc.LLMClass.Class,
			"confidence": sc.LLMClass.Confidence,
		}
	}
	raw, _ := json.Marshal(map[string]interface{}{"review_result": reviewResult})
	return raw
}

func insertReview(t *testing.T, db *sql.DB, orgID int64, authorUsername, userEmail, provider string, metadata []byte) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		INSERT INTO reviews (repository, status, org_id, user_email, provider, author_username, metadata)
		VALUES ('scenario/test-repo', 'completed', $1, $2, $3, $4, $5) RETURNING id`,
		orgID, userEmail, provider, authorUsername, metadata).Scan(&id)
	if err != nil {
		t.Fatalf("insert review: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM reviews WHERE id = $1`, id) })
	return id
}

// expandHunks pads a scenario's explicit "risk" hunks with zero-combined filler
// hunks so the review's total file count / lines added matches sc.Diff exactly
// (LoadBlastStats' FileCount/LinesAdded come from every hunk row, not just the
// risk-relevant ones) without diluting sum/max/weighted_avg/high_count, since
// filler hunks carry 0 new_lines whenever risk hunks already account for every
// line in sc.Diff.LinesAdded.
func expandHunks(sc Scenario) []scenarioHunk {
	hunks := append([]scenarioHunk{}, sc.BlastHunks...)
	for i := range hunks {
		if hunks[i].FilePath == "" {
			hunks[i].FilePath = fmt.Sprintf("risk-%d.go", i)
		}
	}
	remainingFiles := sc.Diff.FileCount - len(hunks)
	if remainingFiles <= 0 {
		return hunks
	}
	// All filler lines go on the first filler file when there are no risk
	// hunks at all (lines_added is otherwise unaccounted for); filler files
	// carry 0 lines whenever risk hunks already total sc.Diff.LinesAdded.
	fillerLines := 0
	if len(sc.BlastHunks) == 0 {
		fillerLines = sc.Diff.LinesAdded
	}
	for i := 0; i < remainingFiles; i++ {
		lines := 0
		if i == 0 {
			lines = fillerLines
		}
		hunks = append(hunks, scenarioHunk{Combined: 0, NewLines: lines, FilePath: fmt.Sprintf("filler-%d.go", i)})
	}
	return hunks
}

func insertBlastHunks(t *testing.T, db *sql.DB, orgID, reviewID int64, hunks []scenarioHunk) {
	t.Helper()
	for _, h := range hunks {
		tier := blastradius.Tier(h.Combined)
		_, err := db.Exec(`
			INSERT INTO blast_radius_hunks (review_id, org_id, file_path, new_start, new_lines, combined, tier, math_mode)
			VALUES ($1, $2, $3, 1, $4, $5, $6, '{}'::jsonb)`,
			reviewID, orgID, h.FilePath, h.NewLines, h.Combined, tier)
		if err != nil {
			t.Fatalf("insert blast_radius_hunks: %v", err)
		}
	}
	// blast_radius_hunks cascades on review delete (ON DELETE CASCADE), so no
	// separate cleanup is required here.
}

// seedRecentScores pre-populates risk_feature_snapshots with a synthetic score
// distribution so PercentileCutoff/PercentileOf have real data to calibrate
// against (a dry-run evaluation never writes its own snapshot, so without this
// an org's risk_threshold_percentile would always resolve to the cold-start
// default cutoff regardless of the configured percentile).
func seedRecentScores(t *testing.T, db *sql.DB, orgID, reviewID int64, scores []float64) {
	t.Helper()
	for _, s := range scores {
		var id int64
		err := db.QueryRow(`
			INSERT INTO risk_feature_snapshots (review_id, org_id, model_version, features, score)
			VALUES ($1, $2, 1, '[]'::jsonb, $3) RETURNING id`, reviewID, orgID, s).Scan(&id)
		if err != nil {
			t.Fatalf("seed risk_feature_snapshots: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM risk_feature_snapshots WHERE id = $1`, id) })
	}
}

// seedApprovalsToday pre-populates review_policy_log with n already-approved
// final verdicts today, so dailyCapExceeded sees real history to count against
// (dry-run evaluations never persist their own log rows).
func seedApprovalsToday(t *testing.T, db *sql.DB, orgID, reviewID int64, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		var id int64
		err := db.QueryRow(`
			INSERT INTO review_policy_log (review_id, org_id, stage, verdict, detail)
			VALUES ($1, $2, 'final', 'approved', '{}'::jsonb) RETURNING id`, reviewID, orgID).Scan(&id)
		if err != nil {
			t.Fatalf("seed review_policy_log: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM review_policy_log WHERE id = $1`, id) })
	}
}

// blockedAtStage extracts which funnel stage actually blocked a review, for
// assertions: the first non-final stage with a "fail" verdict, or the
// "mode_override" sentinel when the funnel never evaluated per-stage at all
// (never_approve, or a cap/mode block recorded only on the final stage).
func blockedAtStage(res *FunnelResult) string {
	if res.Approved {
		return ""
	}
	for _, st := range res.Stages {
		if st.Stage != StageFinal && st.Verdict == VerdictFail {
			return st.Stage
		}
	}
	return "mode_override"
}

func finalReason(res *FunnelResult) string {
	for _, st := range res.Stages {
		if st.Stage == StageFinal {
			if r, ok := st.Detail["reason"].(string); ok {
				return r
			}
		}
	}
	return ""
}

// runScenario seeds one scenario's review and runs it through the real
// production funnel call, exactly as the /simulate endpoint does.
func runScenario(t *testing.T, db *sql.DB, orgID int64, sc Scenario) *FunnelResult {
	t.Helper()
	ctx := context.Background()

	var authorUsername, authorEmail, provider string
	var whitelistUserID int64

	suffix := fmt.Sprintf("%s-%d", sanitize(sc.Name), time.Now().UnixNano())

	switch {
	case sc.AuthorIsBot:
		authorUsername = sc.BotUsername
		provider = "github"
	case sc.AuthorUnmapped:
		// No users/user_identities row at all: resolveAuthor's lookup finds
		// nothing and falls through to the fail-closed default (level 1,
		// matched=false) -- deliberately not a bot-looking username.
		authorUsername = "new-contributor-" + suffix
		authorEmail = fmt.Sprintf("unmapped-%s@scenario.test", suffix)
		provider = "github"
	default:
		email := fmt.Sprintf("author-%s@scenario.test", suffix)
		userID := insertUser(t, db, email, sc.AuthorLevel)
		insertUserIdentity(t, db, orgID, userID, "email", email)
		authorUsername = "author-" + suffix
		authorEmail = email
		provider = "github"
		if sc.Whitelisted {
			whitelistUserID = userID
		}
	}

	metadata := buildReviewMetadata(sc)
	reviewID := insertReview(t, db, orgID, authorUsername, authorEmail, provider, metadata)
	insertBlastHunks(t, db, orgID, reviewID, expandHunks(sc))

	if len(sc.SeedRecentScores) > 0 {
		seedRecentScores(t, db, orgID, reviewID, sc.SeedRecentScores)
	}
	if sc.SeedApprovalsToday > 0 {
		seedApprovalsToday(t, db, orgID, reviewID, sc.SeedApprovalsToday)
	}

	rulesetIDs := resolveRulesets(t, db, orgID, sc)
	policy := sc.buildPolicy(whitelistUserID, rulesetIDs)
	engine := NewEngine(db)
	res, err := engine.RunWithPolicy(ctx, orgID, reviewID, policy, true)
	if err != nil {
		t.Fatalf("[%s] RunWithPolicy: %v", sc.Name, err)
	}
	return res
}

// TestScenarios runs the full testdata/scenarios.json corpus against the real
// policy funnel (engine.RunWithPolicy, dry-run) and asserts each case's
// expected outcome. Run with -v to see the printed table, which doubles as the
// explainability doc's worked-example evidence.
// scenarioFeatureRow is one feature's full arithmetic for the report's
// expandable "show the math" row: raw/transformed value, its weight, its
// category multiplier, and what it contributed to z.
type scenarioFeatureRow struct {
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	Value        float64 `json:"value"`
	Weight       float64 `json:"weight"`
	Multiplier   float64 `json:"multiplier"`
	Contribution float64 `json:"contribution"`
}

// scenarioStageRow is one funnel stage's verdict + reason, for the report's
// per-scenario funnel trace.
type scenarioStageRow struct {
	Stage   string `json:"stage"`
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
}

// scenarioDump is the full machine-readable record TestScenarios logs per
// case -- the report is built directly from these lines (grep the
// "SCDATA:" prefix out of `go test -v` output), never from hand-transcribed
// numbers. Uses the real Prior()/DefaultCategoryMultiplier (the same seed
// weights every throwaway-org scenario is actually scored with, since none
// of them configure risk_factor_overrides) against the real feature values
// the engine itself computed and logged on the risk stage -- not a
// recomputation.
// scenarioTierRow is this scenario's approve/block decision under one
// percentile setting, measured against the shared referencemix distribution
// -- the same 60-review mix internal/riskscore's threshold experiment uses,
// so every scenario's reported percentile and per-tier decision lines up
// with the numbers the Thresholds tab already shows.
type scenarioTierRow struct {
	Percentile int     `json:"percentile"`
	Cutoff     float64 `json:"cutoff"`
	Approved   bool    `json:"approved"`
}

type scenarioDump struct {
	Name           string               `json:"name"`
	Bucket         string               `json:"bucket"`
	Rationale      string               `json:"rationale"`
	Approved       bool                 `json:"approved"`
	BlockedAt      string               `json:"blocked_at"`
	Score          float64              `json:"score"`
	PercentileRank int                  `json:"percentile_rank"`
	Bias           float64              `json:"bias"`
	Z              float64              `json:"z"`
	Features       []scenarioFeatureRow `json:"features"`
	Stages         []scenarioStageRow   `json:"stages"`
	Tiers          []scenarioTierRow    `json:"tiers"`
}

// referenceMixScores is computed once per test run (it's a fixed,
// reproducible 60-review mix -- see internal/riskscore/referencemix) and
// reused to rank every scenario's score against the same distribution.
var referenceMixScoresOnce []float64

func referenceMixScores() []float64 {
	if referenceMixScoresOnce != nil {
		return referenceMixScoresOnce
	}
	prior := riskscore.Prior()
	for _, ex := range referencemix.Build(riskscore.FeatureCount()) {
		referenceMixScoresOnce = append(referenceMixScoresOnce, riskscore.Score(prior, ex.Features, nil))
	}
	return referenceMixScoresOnce
}

// reportTiers is the fixed set of percentile settings the report compares,
// including both edges (P0 blocks everything, P100 approves everything but
// the single riskiest review in the reference mix -- see
// internal/riskscore/threshold_experiment_test.go for the proof).
var reportTiers = []int{0, 10, 20, 50, 80, 100}

func buildScenarioDump(sc Scenario, res *FunnelResult) scenarioDump {
	prior := riskscore.Prior()
	dump := scenarioDump{
		Name:      sc.Name,
		Bucket:    sc.Bucket,
		Rationale: sc.Rationale,
		Approved:  res.Approved,
		BlockedAt: blockedAtStage(res),
		Score:     res.Score,
		Bias:      prior[0],
		Z:         prior[0],
	}

	var rawFeatures []interface{}
	riskRan := false
	for _, st := range res.Stages {
		dump.Stages = append(dump.Stages, scenarioStageRow{Stage: st.Stage, Verdict: st.Verdict, Reason: fmt.Sprintf("%v", st.Detail["reason"])})
		if st.Stage == StageRisk {
			if st.Verdict != VerdictSkip {
				riskRan = true
			}
			if f, ok := st.Detail["features"].([]map[string]interface{}); ok {
				for _, m := range f {
					rawFeatures = append(rawFeatures, m)
				}
			}
		}
	}

	for i, spec := range riskscore.FeatureSpecs {
		var value float64
		if i < len(rawFeatures) {
			if m, ok := rawFeatures[i].(map[string]interface{}); ok {
				if v, ok := m["value"].(float64); ok {
					value = v
				}
			}
		}
		weight := prior[i+1]
		multiplier := riskscore.DefaultCategoryMultiplier(spec.Category)
		if spec.Category == "" {
			multiplier = 1
		}
		contribution := weight * multiplier * value
		dump.Z += contribution
		dump.Features = append(dump.Features, scenarioFeatureRow{
			Name: spec.Name, Category: spec.Category, Value: value,
			Weight: weight, Multiplier: multiplier, Contribution: contribution,
		})
	}

	// Percentile rank and per-tier decisions only mean something when the
	// risk stage actually produced a real score; a scenario blocked earlier
	// (eligibility/static) has res.Score == 0 (Go's zero value, never
	// computed), and ranking that zero against the reference mix would
	// falsely read as "extremely safe" rather than "never scored."
	if riskRan {
		mix := referenceMixScores()
		dump.PercentileRank = riskscore.PercentileOf(dump.Score, mix)
		for _, p := range reportTiers {
			cutoff := riskscore.PercentileCutoff(mix, p)
			dump.Tiers = append(dump.Tiers, scenarioTierRow{
				Percentile: p, Cutoff: cutoff, Approved: dump.Score < cutoff,
			})
		}
	}
	return dump
}

func TestScenarios(t *testing.T) {
	db := testDB(t)
	orgID := seedOrg(t, db, "scenario-corpus")

	for _, sc := range loadScenarios(t) {
		sc := sc
		t.Run(sc.Name, func(t *testing.T) {
			if sc.Skip != "" {
				t.Skip(sc.Skip)
			}
			res := runScenario(t, db, orgID, sc)

			gotApprove := res.Approved
			gotBlockedAt := blockedAtStage(res)

			status := "PASS"
			if gotApprove != sc.Want.Approve {
				status = "FAIL"
			}
			if !sc.Want.Approve && sc.Want.BlockedAt != "" && gotBlockedAt != sc.Want.BlockedAt {
				status = "FAIL"
			}
			reason := finalReason(res)
			if sc.Want.ReasonContains != "" && !strings.Contains(reason, sc.Want.ReasonContains) {
				status = "FAIL"
			}
			t.Logf("%-4s %-55s want_approve=%-5v got_approve=%-5v blocked_at=%-12s score=%.3f  // %s",
				status, sc.Name, sc.Want.Approve, gotApprove, gotBlockedAt, res.Score, sc.Rationale)

			dumpJSON, err := json.Marshal(buildScenarioDump(sc, res))
			if err != nil {
				t.Fatalf("[%s] marshal scenario dump: %v", sc.Name, err)
			}
			t.Logf("SCDATA:%s", dumpJSON)

			if gotApprove != sc.Want.Approve {
				t.Errorf("[%s] approve = %v, want %v (reason: %s)", sc.Name, gotApprove, sc.Want.Approve, reason)
			}
			if !sc.Want.Approve && sc.Want.BlockedAt != "" && gotBlockedAt != sc.Want.BlockedAt {
				t.Errorf("[%s] blocked_at = %q, want %q (reason: %s)", sc.Name, gotBlockedAt, sc.Want.BlockedAt, reason)
			}
			if sc.Want.ReasonContains != "" && !strings.Contains(reason, sc.Want.ReasonContains) {
				t.Errorf("[%s] final reason %q does not contain %q", sc.Name, reason, sc.Want.ReasonContains)
			}
		})
	}
}
