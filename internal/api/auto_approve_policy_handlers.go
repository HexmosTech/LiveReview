package api

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/policyengine"
)

// AutoApprovePoliciesHandler implements the auto-approve policy CRUD,
// resolution, and simulation API. See docs/risk-policy-engine.md §1/§8.
type AutoApprovePoliciesHandler struct {
	db     *sql.DB
	engine *policyengine.Engine
}

func NewAutoApprovePoliciesHandler(db *sql.DB) *AutoApprovePoliciesHandler {
	return &AutoApprovePoliciesHandler{db: db, engine: policyengine.NewEngine(db)}
}

// PolicyDTO is the persisted/returned shape of an auto_approve_policies row.
type PolicyDTO struct {
	ID                               int64   `json:"id"`
	OrgID                            int64   `json:"org_id"`
	RepositoryID                     *int64  `json:"repository_id,omitempty"`
	Mode                             string  `json:"mode"`
	MinAuthorLevel                   int     `json:"min_author_level"`
	EligibilityWhitelistUserIDs      []int64 `json:"eligibility_whitelist_user_ids"`
	RiskThresholdPercentile          int     `json:"risk_threshold_percentile"`
	RequireLLMConfidence             int     `json:"require_llm_confidence"`
	BlockingRulesetIDs               []int64 `json:"blocking_ruleset_ids"`
	LandingDelaySeconds              int     `json:"landing_delay_seconds"`
	DailyApproveCap                  *int    `json:"daily_approve_cap,omitempty"`
	RestrictOutcomeReportingToAdmins bool    `json:"restrict_outcome_reporting_to_admins"`
	ForceApproveMinLevel             int     `json:"force_approve_min_level"`
}

type policyUpsertRequest struct {
	RepositoryID                     *int64  `json:"repository_id"`
	Mode                             *string `json:"mode"`
	MinAuthorLevel                   *int    `json:"min_author_level"`
	EligibilityWhitelistUserIDs      []int64 `json:"eligibility_whitelist_user_ids"`
	RiskThresholdPercentile          *int    `json:"risk_threshold_percentile"`
	RequireLLMConfidence             *int    `json:"require_llm_confidence"`
	BlockingRulesetIDs               []int64 `json:"blocking_ruleset_ids"`
	LandingDelaySeconds              *int    `json:"landing_delay_seconds"`
	DailyApproveCap                  *int    `json:"daily_approve_cap"`
	RestrictOutcomeReportingToAdmins *bool   `json:"restrict_outcome_reporting_to_admins"`
	ForceApproveMinLevel             *int    `json:"force_approve_min_level"`
}

func scanPolicy(row interface{ Scan(...interface{}) error }) (*PolicyDTO, error) {
	var p PolicyDTO
	var repoID sql.NullInt64
	err := row.Scan(
		&p.ID, &p.OrgID, &repoID, &p.Mode, &p.MinAuthorLevel,
		pq.Array(&p.EligibilityWhitelistUserIDs), &p.RiskThresholdPercentile,
		&p.RequireLLMConfidence, pq.Array(&p.BlockingRulesetIDs), &p.LandingDelaySeconds,
		&p.DailyApproveCap, &p.RestrictOutcomeReportingToAdmins, &p.ForceApproveMinLevel,
	)
	if err != nil {
		return nil, err
	}
	if repoID.Valid {
		p.RepositoryID = &repoID.Int64
	}
	return &p, nil
}

const policyColumns = `id, org_id, repository_id, mode, min_author_level,
	eligibility_whitelist_user_ids, risk_threshold_percentile,
	require_llm_confidence, blocking_ruleset_ids, landing_delay_seconds,
	daily_approve_cap, restrict_outcome_reporting_to_admins, force_approve_min_level`

// List returns all policies for the org (org-default + repo-scoped).
func (h *AutoApprovePoliciesHandler) List(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	rows, err := h.db.QueryContext(c.Request().Context(),
		`SELECT `+policyColumns+` FROM auto_approve_policies WHERE org_id = $1 ORDER BY repository_id NULLS FIRST`, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "list failed: "+err.Error())
	}
	defer rows.Close()

	out := []*PolicyDTO{}
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "scan failed: "+err.Error())
		}
		out = append(out, p)
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": out})
}

// Get returns one policy by id.
func (h *AutoApprovePoliciesHandler) Get(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid policy id")
	}
	row := h.db.QueryRowContext(c.Request().Context(),
		`SELECT `+policyColumns+` FROM auto_approve_policies WHERE id = $1 AND org_id = $2`, id, orgID)
	p, err := scanPolicy(row)
	if err == sql.ErrNoRows {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "policy not found")
	}
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "get failed: "+err.Error())
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"policy": p})
}

// Create inserts a new policy (org-default or repo-scoped).
func (h *AutoApprovePoliciesHandler) Create(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()

	var body policyUpsertRequest
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	mode := "custom"
	if body.Mode != nil && *body.Mode != "" {
		mode = *body.Mode
	}

	// A repo-scoped row that omits a field should inherit the org default for
	// that field, not the bare schema default — otherwise picking "Customize"
	// on a repo can silently loosen a stricter org-wide setting (e.g. eligibility).
	defaults := policyUpsertRequest{
		MinAuthorLevel:          policyIntPtr(1),
		RiskThresholdPercentile: policyIntPtr(20),
		RequireLLMConfidence:    policyIntPtr(8),
		LandingDelaySeconds:     policyIntPtr(0),
		ForceApproveMinLevel:    policyIntPtr(3),
	}
	if body.RepositoryID != nil {
		orgDefault, err := loadOrgDefaultPolicy(c.Request().Context(), h.db, orgID)
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load org default policy")
		}
		if orgDefault != nil {
			defaults = policyUpsertRequest{
				MinAuthorLevel:                   &orgDefault.MinAuthorLevel,
				EligibilityWhitelistUserIDs:      orgDefault.EligibilityWhitelistUserIDs,
				RiskThresholdPercentile:          &orgDefault.RiskThresholdPercentile,
				RequireLLMConfidence:             &orgDefault.RequireLLMConfidence,
				BlockingRulesetIDs:               orgDefault.BlockingRulesetIDs,
				LandingDelaySeconds:              &orgDefault.LandingDelaySeconds,
				DailyApproveCap:                  orgDefault.DailyApproveCap,
				RestrictOutcomeReportingToAdmins: &orgDefault.RestrictOutcomeReportingToAdmins,
				ForceApproveMinLevel:             &orgDefault.ForceApproveMinLevel,
			}
		}
	}
	if body.MinAuthorLevel == nil {
		body.MinAuthorLevel = defaults.MinAuthorLevel
	}
	if body.EligibilityWhitelistUserIDs == nil {
		body.EligibilityWhitelistUserIDs = defaults.EligibilityWhitelistUserIDs
	}
	if body.RiskThresholdPercentile == nil {
		body.RiskThresholdPercentile = defaults.RiskThresholdPercentile
	}
	if body.RequireLLMConfidence == nil {
		body.RequireLLMConfidence = defaults.RequireLLMConfidence
	}
	if body.BlockingRulesetIDs == nil {
		body.BlockingRulesetIDs = defaults.BlockingRulesetIDs
	}
	if body.LandingDelaySeconds == nil {
		body.LandingDelaySeconds = defaults.LandingDelaySeconds
	}
	if body.DailyApproveCap == nil {
		body.DailyApproveCap = defaults.DailyApproveCap
	}
	if body.RestrictOutcomeReportingToAdmins == nil {
		body.RestrictOutcomeReportingToAdmins = defaults.RestrictOutcomeReportingToAdmins
	}
	if body.ForceApproveMinLevel == nil {
		body.ForceApproveMinLevel = defaults.ForceApproveMinLevel
	}

	var id int64
	err := h.db.QueryRowContext(c.Request().Context(), `
		INSERT INTO auto_approve_policies (org_id, repository_id, mode, min_author_level,
			eligibility_whitelist_user_ids, risk_threshold_percentile, require_llm_confidence,
			blocking_ruleset_ids, landing_delay_seconds, daily_approve_cap,
			restrict_outcome_reporting_to_admins, force_approve_min_level, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		orgID, body.RepositoryID, mode, derefInt(body.MinAuthorLevel, 1),
		pq.Array(body.EligibilityWhitelistUserIDs), derefInt(body.RiskThresholdPercentile, 20),
		derefInt(body.RequireLLMConfidence, 8), pq.Array(body.BlockingRulesetIDs),
		derefInt(body.LandingDelaySeconds, 0), body.DailyApproveCap,
		derefBool(body.RestrictOutcomeReportingToAdmins, false), derefInt(body.ForceApproveMinLevel, 3),
		pc.GetUserID(),
	).Scan(&id)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "create failed: "+err.Error())
	}

	row := h.db.QueryRowContext(c.Request().Context(),
		`SELECT `+policyColumns+` FROM auto_approve_policies WHERE id = $1 AND org_id = $2`, id, orgID)
	p, err := scanPolicy(row)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "created but failed to reload")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"policy": p})
}

// Update modifies a policy.
func (h *AutoApprovePoliciesHandler) Update(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid policy id")
	}

	var body policyUpsertRequest
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}

	_, err = h.db.ExecContext(c.Request().Context(), `
		UPDATE auto_approve_policies SET
			mode = COALESCE($1, mode),
			min_author_level = COALESCE($2, min_author_level),
			eligibility_whitelist_user_ids = COALESCE($3, eligibility_whitelist_user_ids),
			risk_threshold_percentile = COALESCE($4, risk_threshold_percentile),
			require_llm_confidence = COALESCE($5, require_llm_confidence),
			blocking_ruleset_ids = COALESCE($6, blocking_ruleset_ids),
			landing_delay_seconds = COALESCE($7, landing_delay_seconds),
			daily_approve_cap = $8,
			restrict_outcome_reporting_to_admins = COALESCE($9, restrict_outcome_reporting_to_admins),
			force_approve_min_level = COALESCE($10, force_approve_min_level),
			updated_at = now()
		WHERE id = $11 AND org_id = $12`,
		body.Mode, body.MinAuthorLevel,
		pq.Array(body.EligibilityWhitelistUserIDs), body.RiskThresholdPercentile,
		body.RequireLLMConfidence, pq.Array(body.BlockingRulesetIDs),
		body.LandingDelaySeconds, body.DailyApproveCap,
		body.RestrictOutcomeReportingToAdmins, body.ForceApproveMinLevel,
		id, orgID,
	)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "update failed: "+err.Error())
	}

	row := h.db.QueryRowContext(c.Request().Context(),
		`SELECT `+policyColumns+` FROM auto_approve_policies WHERE id = $1 AND org_id = $2`, id, orgID)
	p, err := scanPolicy(row)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "policy not found")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"policy": p})
}

// Delete removes a policy.
func (h *AutoApprovePoliciesHandler) Delete(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid policy id")
	}
	res, err := h.db.ExecContext(c.Request().Context(), `DELETE FROM auto_approve_policies WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "delete failed: "+err.Error())
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "policy not found")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "deleted"})
}

// Resolved returns the effective policy for a repository (repo-scoped or org
// default), used by the repository settings UI to show "org default" context.
func (h *AutoApprovePoliciesHandler) Resolved(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	var repoID int64
	if v := c.QueryParam("repository_id"); v != "" {
		repoID, _ = strconv.ParseInt(v, 10, 64)
	}
	p, found, err := policyengine.ResolvePolicy(c.Request().Context(), h.db, orgID, repoID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "resolve failed: "+err.Error())
	}
	if !found {
		return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"policy": nil, "configured": false})
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"policy": p, "configured": true})
}

// Simulate replays a draft policy against the org's recent completed reviews,
// discarding all results (dry-run). See docs/risk-policy-engine.md §8.
func (h *AutoApprovePoliciesHandler) Simulate(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	var body struct {
		RepositoryID *int64              `json:"repository_id"`
		Policy       policyUpsertRequest `json:"policy"`
		ReviewIDs    []int64             `json:"review_ids"`
		Limit        int                 `json:"limit"`
	}
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	if body.Limit <= 0 || body.Limit > 100 {
		body.Limit = 20
	}

	draft := &policyengine.Policy{
		OrgID:                            orgID,
		RepositoryID:                     body.RepositoryID,
		Mode:                             "custom",
		MinAuthorLevel:                   derefInt(body.Policy.MinAuthorLevel, 1),
		EligibilityWhitelistUserIDs:      body.Policy.EligibilityWhitelistUserIDs,
		RiskThresholdPercentile:          derefInt(body.Policy.RiskThresholdPercentile, 20),
		RequireLLMConfidence:             derefInt(body.Policy.RequireLLMConfidence, 8),
		BlockingRulesetIDs:               body.Policy.BlockingRulesetIDs,
		LandingDelaySeconds:              derefInt(body.Policy.LandingDelaySeconds, 0),
		DailyApproveCap:                  body.Policy.DailyApproveCap,
		RestrictOutcomeReportingToAdmins: derefBool(body.Policy.RestrictOutcomeReportingToAdmins, false),
		ForceApproveMinLevel:             derefInt(body.Policy.ForceApproveMinLevel, 3),
	}
	if body.Policy.Mode != nil {
		draft.Mode = *body.Policy.Mode
	}

	reviewIDs := body.ReviewIDs
	if len(reviewIDs) == 0 {
		rows, err := h.db.QueryContext(c.Request().Context(), `
			SELECT id FROM reviews
			WHERE org_id = $1 AND status = 'completed'
			ORDER BY created_at DESC LIMIT $2`, orgID, body.Limit)
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load reviews: "+err.Error())
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "scan failed: "+err.Error())
			}
			reviewIDs = append(reviewIDs, id)
		}
	}

	type simRow struct {
		ReviewID  int64                      `json:"review_id"`
		Approved  bool                       `json:"would_approve"`
		BlockedAt string                     `json:"blocked_at,omitempty"`
		Score     float64                    `json:"score,omitempty"`
		Stages    []policyengine.StageResult `json:"stages"`
	}
	// simSummary answers "is this org's current slider configuration actually
	// differentiating anything" at a glance, instead of requiring a hand-query
	// against the database (how the original br_sum saturation bug behind this
	// package's riskscore changes was actually found). saturated_high/_low
	// count scores pinned at either extreme, which a well-tuned model should
	// rarely produce for a typical history.
	type simSummary struct {
		ScoredCount   int     `json:"scored_count"`
		SkippedCount  int     `json:"skipped_count"`
		SaturatedHigh int     `json:"saturated_high"`
		SaturatedLow  int     `json:"saturated_low"`
		ScoreMin      float64 `json:"score_min,omitempty"`
		ScoreMax      float64 `json:"score_max,omitempty"`
		ScoreMean     float64 `json:"score_mean,omitempty"`
	}
	out := []simRow{}
	summary := simSummary{ScoreMin: 1}
	var scoreSum float64
	for _, rid := range reviewIDs {
		res, err := h.engine.RunWithPolicy(c.Request().Context(), orgID, rid, draft, true)
		if err != nil {
			log.Printf("auto-approve simulate: review %d skipped: %v", rid, err)
			summary.SkippedCount++
			continue
		}
		row := simRow{ReviewID: rid, Approved: res.Approved, Score: res.Score, Stages: res.Stages}
		riskRan := false
		for _, st := range res.Stages {
			if row.BlockedAt == "" && st.Verdict == policyengine.VerdictFail {
				row.BlockedAt = st.Stage
			}
			// Risk stage is skipped (VerdictSkip) whenever an earlier stage
			// (eligibility/static) already blocked the review -- res.Score
			// stays its zero value in that case, which isn't a real "low
			// score" and must not be counted in the summary stats below.
			if st.Stage == policyengine.StageRisk && st.Verdict != policyengine.VerdictSkip {
				riskRan = true
			}
		}
		out = append(out, row)

		if riskRan {
			summary.ScoredCount++
			scoreSum += res.Score
			if res.Score > summary.ScoreMax {
				summary.ScoreMax = res.Score
			}
			if res.Score < summary.ScoreMin {
				summary.ScoreMin = res.Score
			}
			if res.Score > 0.99 {
				summary.SaturatedHigh++
			}
			if res.Score < 0.01 {
				summary.SaturatedLow++
			}
		}
	}
	if summary.ScoredCount == 0 {
		summary.ScoreMin = 0
	} else {
		summary.ScoreMean = scoreSum / float64(summary.ScoredCount)
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": out, "summary": summary})
}

// loadOrgDefaultPolicy returns the org-default (repository_id IS NULL) policy
// row, or (nil, nil) if none is configured yet.
func loadOrgDefaultPolicy(ctx context.Context, db *sql.DB, orgID int64) (*PolicyDTO, error) {
	row := db.QueryRowContext(ctx,
		`SELECT `+policyColumns+` FROM auto_approve_policies WHERE org_id = $1 AND repository_id IS NULL`, orgID)
	p, err := scanPolicy(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func policyIntPtr(v int) *int { return &v }

func derefInt(p *int, d int) int {
	if p == nil {
		return d
	}
	return *p
}

func derefBool(p *bool, d bool) bool {
	if p == nil {
		return d
	}
	return *p
}
