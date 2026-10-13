package policyengine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/livereview/internal/canonicaldoc"
	"github.com/livereview/internal/riskscore"
)

// ErrPolicyDisabled is returned when no auto-approve policy is configured for
// the review's org/repo (the feature is off).
var ErrPolicyDisabled = errors.New("auto-approve policy not configured")

// StageResult is one funnel stage's outcome, as written to review_policy_log.
type StageResult struct {
	Stage   string                 `json:"stage"`
	Verdict string                 `json:"verdict"`
	Detail  map[string]interface{} `json:"detail"`
}

// FunnelResult is the full outcome of one policy evaluation.
type FunnelResult struct {
	Approved        bool          `json:"approved"`
	Mode            string        `json:"mode"`
	ModeOverride    bool          `json:"mode_override"`
	Stages          []StageResult `json:"stages"`
	Score           float64       `json:"score,omitempty"`
	Percentile      int           `json:"percentile,omitempty"`
	ThresholdCutoff float64       `json:"threshold_cutoff,omitempty"`
	Author          *AuthorInfo   `json:"author,omitempty"`
	// LandingDelaySeconds is the post-approve human-rejection window, copied
	// from the resolved policy so the worker can schedule the delayed publish.
	LandingDelaySeconds int `json:"landing_delay_seconds,omitempty"`
}

// Engine runs the policy funnel against a database.
type Engine struct {
	db     *sql.DB
	scorer *riskscore.Scorer
	store  *riskscore.Store
}

// NewEngine returns an Engine backed by db.
func NewEngine(db *sql.DB) *Engine {
	return &Engine{db: db, scorer: riskscore.NewScorer(db), store: riskscore.NewStore(db)}
}

func (e *Engine) reason(st StageResult) string {
	if r, ok := st.Detail["reason"].(string); ok {
		return r
	}
	return ""
}

func skipStage(stage string) StageResult {
	return StageResult{Stage: stage, Verdict: VerdictSkip, Detail: map[string]interface{}{"reason": "skipped (earlier stage blocked)"}}
}

func skipModeStage(stage string) StageResult {
	return StageResult{Stage: stage, Verdict: VerdictSkip, Detail: map[string]interface{}{"reason": "skipped: mode override"}}
}

// reviewRepositoryID resolves the repository_id for a review (via its linked
// pull request), or 0 when there is none.
func (e *Engine) reviewRepositoryID(ctx context.Context, orgID, reviewID int64) int64 {
	var repoID int64
	err := e.db.QueryRowContext(ctx, `
		SELECT COALESCE(pr.repository_id, 0)
		FROM reviews r LEFT JOIN pull_requests pr ON pr.id = r.pull_request_id
		WHERE r.id = $1 AND r.org_id = $2`, reviewID, orgID).Scan(&repoID)
	if err != nil {
		return 0
	}
	return repoID
}

// Run evaluates the policy funnel for a review. dryRun skips all writes
// (review_policy_log rows and the feature snapshot) for the simulate endpoint.
func (e *Engine) Run(ctx context.Context, orgID, reviewID int64, dryRun bool) (*FunnelResult, error) {
	repoID := e.reviewRepositoryID(ctx, orgID, reviewID)
	p, found, err := ResolvePolicy(ctx, e.db, orgID, repoID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrPolicyDisabled
	}
	return e.runWithPolicy(ctx, orgID, reviewID, p, dryRun)
}

// RunWithPolicy evaluates the funnel against an explicit policy (the simulate
// endpoint's draft policy), bypassing DB resolution. dryRun skips all writes.
func (e *Engine) RunWithPolicy(ctx context.Context, orgID, reviewID int64, p *Policy, dryRun bool) (*FunnelResult, error) {
	return e.runWithPolicy(ctx, orgID, reviewID, p, dryRun)
}

func (e *Engine) runWithPolicy(ctx context.Context, orgID, reviewID int64, p *Policy, dryRun bool) (*FunnelResult, error) {
	res := &FunnelResult{Mode: p.Mode, Stages: []StageResult{}, LandingDelaySeconds: p.LandingDelaySeconds}

	doc, docErr := canonicaldoc.Build(ctx, e.db, orgID, reviewID)

	switch p.Mode {
	case ModeNeverApprove:
		res.ModeOverride = true
		res.Stages = append(res.Stages, StageResult{Stage: StageFinal, Verdict: FinalBlocked,
			Detail: map[string]interface{}{"reason": "mode override: never_approve"}})
		e.persist(ctx, orgID, reviewID, res, dryRun)
		return res, nil

	case ModeAlwaysApprove:
		res.ModeOverride = true
		// The static jq gate still runs even under always_approve: a repo-wide
		// "trust this repo" setting must never silence a secrets-leak detector.
		var staticStage StageResult
		if docErr == nil {
			staticStage = e.staticStage(ctx, orgID, reviewID, p, doc)
		} else {
			staticStage = StageResult{Stage: StageStatic, Verdict: VerdictSkip,
				Detail: map[string]interface{}{"reason": "no findings document"}}
		}
		res.Stages = append(res.Stages, staticStage)
		res.Stages = append(res.Stages, skipModeStage(StageEligibility), skipModeStage(StageRisk), skipModeStage(StageLLM))

		if staticStage.Verdict == VerdictFail {
			res.Approved = false
			res.Stages = append(res.Stages, StageResult{Stage: StageFinal, Verdict: FinalBlocked,
				Detail: map[string]interface{}{"reason": "blocked by static ruleset despite always_approve: " + e.reason(staticStage)}})
		} else {
			res.Approved = true
			res.Stages = append(res.Stages, StageResult{Stage: StageFinal, Verdict: FinalApproved,
				Detail: map[string]interface{}{"reason": "mode override: always_approve"}})
		}
		e.persist(ctx, orgID, reviewID, res, dryRun)
		return res, nil
	}

	// custom (the full funnel)
	author, authStage := e.eligibilityStage(ctx, orgID, reviewID, p)
	res.Author = author
	res.Stages = append(res.Stages, authStage)
	blocked := authStage.Verdict == VerdictFail

	// static
	if !blocked {
		if docErr != nil {
			st := StageResult{Stage: StageStatic, Verdict: VerdictFail,
				Detail: map[string]interface{}{"reason": "failed to build findings document: " + docErr.Error()}}
			res.Stages = append(res.Stages, st)
			blocked = true
		} else {
			st := e.staticStage(ctx, orgID, reviewID, p, doc)
			res.Stages = append(res.Stages, st)
			blocked = st.Verdict == VerdictFail
		}
	} else {
		res.Stages = append(res.Stages, skipStage(StageStatic))
	}

	// risk
	if !blocked && docErr == nil {
		rt := e.riskStage(ctx, orgID, reviewID, p, doc, dryRun)
		res.Stages = append(res.Stages, rt.StageResult)
		res.Score = rt.Score
		res.Percentile = rt.Percentile
		res.ThresholdCutoff = rt.Cutoff
		blocked = rt.Verdict == VerdictFail
	} else {
		res.Stages = append(res.Stages, skipStage(StageRisk))
	}

	// llm
	if !blocked && docErr == nil {
		lt := e.llmStage(p, doc)
		res.Stages = append(res.Stages, lt)
		blocked = lt.Verdict == VerdictFail
	} else {
		res.Stages = append(res.Stages, skipStage(StageLLM))
	}

	// daily approve cap (only affects approval, recorded on the final stage)
	finalReason := "approved"
	if !blocked {
		if capped, reason := e.dailyCapExceeded(ctx, orgID, p); capped {
			blocked = true
			finalReason = reason
		}
	} else {
		finalReason = e.blockReason(res.Stages)
	}

	res.Approved = !blocked
	verdict := FinalBlocked
	if res.Approved {
		verdict = FinalApproved
	}
	res.Stages = append(res.Stages, StageResult{Stage: StageFinal, Verdict: verdict,
		Detail: map[string]interface{}{"reason": finalReason}})

	e.persist(ctx, orgID, reviewID, res, dryRun)
	return res, nil
}

// blockReason walks the stages for the first failing stage's reason.
func (e *Engine) blockReason(stages []StageResult) string {
	for _, st := range stages {
		if st.Verdict == VerdictFail {
			return fmt.Sprintf("%s: %s", st.Stage, e.reason(st))
		}
	}
	return "blocked"
}

// riskStageResult is the risk stage's outcome plus the score fields surfaced on
// the funnel result.
type riskStageResult struct {
	StageResult
	Score      float64
	Percentile int
	Cutoff     float64
}

// eligibilityStage resolves the author and applies the whitelist + min-level rule.
func (e *Engine) eligibilityStage(ctx context.Context, orgID, reviewID int64, p *Policy) (*AuthorInfo, StageResult) {
	author, err := e.resolveAuthor(ctx, orgID, reviewID)
	if err != nil {
		return nil, StageResult{Stage: StageEligibility, Verdict: VerdictFail,
			Detail: map[string]interface{}{"reason": "failed to resolve author: " + err.Error()}}
	}

	// Whitelist bypasses the level gate.
	if author.Matched && containsInt64(p.EligibilityWhitelistUserIDs, author.UserID) {
		return author, StageResult{Stage: StageEligibility, Verdict: VerdictPass,
			Detail: map[string]interface{}{"reason": "author whitelisted", "level": author.Level, "matched": author.Matched}}
	}

	level := author.Level
	if author.IsBot && !author.Matched {
		level = 0
	}

	if level >= p.MinAuthorLevel {
		return author, StageResult{Stage: StageEligibility, Verdict: VerdictPass,
			Detail: map[string]interface{}{"reason": fmt.Sprintf("author level %d >= required %d", level, p.MinAuthorLevel), "level": level, "matched": author.Matched}}
	}
	return author, StageResult{Stage: StageEligibility, Verdict: VerdictFail,
		Detail: map[string]interface{}{
			"reason":      fmt.Sprintf("author level %d < required %d", level, p.MinAuthorLevel),
			"level":       level,
			"matched":     author.Matched,
			"unmapped":    !author.Matched,
			"author_name": author.Username,
		}}
}

// staticStage runs the configured blocking jq rulesets (reusing the CI/CD gate).
func (e *Engine) staticStage(ctx context.Context, orgID, reviewID int64, p *Policy, doc *canonicaldoc.ReviewDoc) StageResult {
	if len(p.BlockingRulesetIDs) == 0 {
		return StageResult{Stage: StageStatic, Verdict: VerdictPass,
			Detail: map[string]interface{}{"reason": "no blocking rulesets configured"}}
	}
	for _, rsID := range p.BlockingRulesetIDs {
		expr, name, err := e.loadRuleset(ctx, orgID, rsID)
		if err != nil {
			return StageResult{Stage: StageStatic, Verdict: VerdictFail,
				Detail: map[string]interface{}{"reason": fmt.Sprintf("ruleset %d unavailable: %v", rsID, err)}}
		}
		blocked, _, evalErr := canonicaldoc.RunJQBool(expr, doc)
		if evalErr != nil {
			return StageResult{Stage: StageStatic, Verdict: VerdictFail,
				Detail: map[string]interface{}{"reason": fmt.Sprintf("ruleset %q failed to evaluate: %v", name, evalErr)}}
		}
		if blocked {
			return StageResult{Stage: StageStatic, Verdict: VerdictFail,
				Detail: map[string]interface{}{"reason": fmt.Sprintf("blocked by ruleset %q", name), "ruleset_id": rsID, "ruleset_name": name}}
		}
	}
	return StageResult{Stage: StageStatic, Verdict: VerdictPass,
		Detail: map[string]interface{}{"reason": fmt.Sprintf("passed %d blocking ruleset(s)", len(p.BlockingRulesetIDs))}}
}

// loadRuleset returns a ruleset's jq expression + name, org-scoped.
func (e *Engine) loadRuleset(ctx context.Context, orgID, rulesetID int64) (expr, name string, err error) {
	err = e.db.QueryRowContext(ctx,
		`SELECT jq_expr, name FROM ci_rulesets WHERE id = $1 AND org_id = $2`, rulesetID, orgID).Scan(&expr, &name)
	return expr, name, err
}

// riskStage scores the review against the trained model and compares it to the
// percentile threshold.
func (e *Engine) riskStage(ctx context.Context, orgID, reviewID int64, p *Policy, doc *canonicaldoc.ReviewDoc, dryRun bool) riskStageResult {
	br, err := riskscore.LoadBlastStats(ctx, e.db, orgID, reviewID)
	if err != nil {
		br = riskscore.BlastStats{}
	}
	features := riskscore.ExtractFeatures(br, doc)
	score, err := e.scorer.ScoreFor(ctx, orgID, features)
	if err != nil {
		return riskStageResult{StageResult: StageResult{Stage: StageRisk, Verdict: VerdictFail,
			Detail: map[string]interface{}{"reason": "risk model unavailable: " + err.Error()}}}
	}

	scores, _ := riskscore.LoadRecentScores(ctx, e.db, orgID, 1000)
	cutoff := riskscore.PercentileCutoff(scores, p.RiskThresholdPercentile)
	pct := riskscore.PercentileOf(score, scores)

	if !dryRun {
		info, infoErr := e.scorer.Info(ctx, orgID)
		if infoErr == nil {
			_ = e.store.SaveFeatureSnapshot(ctx, reviewID, orgID, info.Version, features, score)
		}
	}

	detail := map[string]interface{}{
		"score":       score,
		"percentile":  pct,
		"cutoff":      cutoff,
		"threshold":   p.RiskThresholdPercentile,
		"features":    featureTable(features),
	}
	if score >= cutoff {
		detail["reason"] = fmt.Sprintf("risk score %.2f (P%d) >= threshold P%d (cutoff %.2f)", score, pct, p.RiskThresholdPercentile, cutoff)
		return riskStageResult{StageResult: StageResult{Stage: StageRisk, Verdict: VerdictFail, Detail: detail}, Score: score, Percentile: pct, Cutoff: cutoff}
	}
	detail["reason"] = fmt.Sprintf("risk score %.2f (P%d) < threshold P%d (cutoff %.2f)", score, pct, p.RiskThresholdPercentile, cutoff)
	return riskStageResult{StageResult: StageResult{Stage: StageRisk, Verdict: VerdictPass, Detail: detail}, Score: score, Percentile: pct, Cutoff: cutoff}
}

// featureTable renders the feature vector as a human-readable list for the UI's
// explainability view.
func featureTable(features []float64) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(features))
	for i, v := range features {
		if i >= len(riskscore.FeatureSpecs) {
			break
		}
		out = append(out, map[string]interface{}{
			"feature": riskscore.FeatureSpecs[i].Name,
			"value":   v,
		})
	}
	return out
}

// llmStage gates on the diff-level semantic classification (confidence +
// disqualifying class).
func (e *Engine) llmStage(p *Policy, doc *canonicaldoc.ReviewDoc) StageResult {
	dc := doc.DiffClassification
	if dc == nil {
		return StageResult{Stage: StageLLM, Verdict: VerdictFail,
			Detail: map[string]interface{}{"reason": "no diff classification available"}}
	}
	if dc.Confidence < p.RequireLLMConfidence {
		return StageResult{Stage: StageLLM, Verdict: VerdictFail,
			Detail: map[string]interface{}{
				"reason":     fmt.Sprintf("classification confidence %d < required %d", dc.Confidence, p.RequireLLMConfidence),
				"class":      dc.Class,
				"confidence": dc.Confidence,
			}}
	}
	if isDisqualifyingClass(dc.Class) {
		return StageResult{Stage: StageLLM, Verdict: VerdictFail,
			Detail: map[string]interface{}{"reason": "disqualifying class: " + dc.Class, "class": dc.Class, "confidence": dc.Confidence}}
	}
	return StageResult{Stage: StageLLM, Verdict: VerdictPass,
		Detail: map[string]interface{}{"reason": fmt.Sprintf("class %q, confidence %d", dc.Class, dc.Confidence), "class": dc.Class, "confidence": dc.Confidence}}
}

// isDisqualifyingClass implements RADAR's "any risk signal disqualifies": a
// security-sensitive diff never auto-approves regardless of score.
func isDisqualifyingClass(class string) bool {
	switch class {
	case "security-sensitive":
		return true
	default:
		return false
	}
}

// dailyCapExceeded reports whether the org's daily auto-approve cap has been
// reached (nil cap = unlimited).
func (e *Engine) dailyCapExceeded(ctx context.Context, orgID int64, p *Policy) (bool, string) {
	if p.DailyApproveCap == nil {
		return false, ""
	}
	var count int
	err := e.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM review_policy_log
		WHERE org_id = $1 AND stage = 'final' AND verdict = 'approved'
		  AND created_at >= date_trunc('day', now())`, orgID).Scan(&count)
	if err != nil {
		return false, ""
	}
	if count >= *p.DailyApproveCap {
		return true, fmt.Sprintf("daily approve cap reached (%d/%d)", count, *p.DailyApproveCap)
	}
	return false, ""
}

// persist writes each stage to review_policy_log (skipped in dryRun). It
// replaces any prior evaluation of this review so a worker retry doesn't leave
// duplicate funnel rows.
func (e *Engine) persist(ctx context.Context, orgID, reviewID int64, res *FunnelResult, dryRun bool) {
	if dryRun {
		return
	}
	_, _ = e.db.ExecContext(ctx, `DELETE FROM review_policy_log WHERE review_id = $1 AND org_id = $2`, reviewID, orgID)
	for _, st := range res.Stages {
		detail, _ := json.Marshal(st.Detail)
		_, _ = e.db.ExecContext(ctx, `
			INSERT INTO review_policy_log (review_id, org_id, stage, verdict, detail)
			VALUES ($1, $2, $3, $4, $5)`, reviewID, orgID, st.Stage, st.Verdict, detail)
	}
}

func containsInt64(slice []int64, v int64) bool {
	for _, x := range slice {
		if x == v {
			return true
		}
	}
	return false
}
