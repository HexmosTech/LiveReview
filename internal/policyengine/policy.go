// Package policyengine implements the risk-based auto-approve policy funnel:
// eligibility -> static jq gates -> trained risk score -> LLM classification
// -> final decision. See docs/risk-policy-engine.md. It reuses the CI/CD gate's
// canonical-document + jq evaluation (internal/canonicaldoc) for the static
// stage and the trained model (internal/riskscore) for the risk stage.
package policyengine

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
)

// Policy modes.
const (
	ModeInherit       = "inherit"
	ModeCustom        = "custom"
	ModeAlwaysApprove = "always_approve"
	ModeNeverApprove  = "never_approve"
)

// Stage verdicts.
const (
	VerdictPass = "pass"
	VerdictFail = "fail"
	VerdictSkip = "skip"
)

// Funnel stages.
const (
	StageEligibility = "eligibility"
	StageStatic      = "static"
	StageRisk        = "risk"
	StageLLM         = "llm"
	StageFinal       = "final"
)

// Final verdicts.
const (
	FinalApproved = "approved"
	FinalBlocked  = "blocked"
)

// Policy is one auto_approve_policies row (org-default or repo-scoped).
type Policy struct {
	ID                               int64
	OrgID                            int64
	RepositoryID                     *int64
	Mode                             string
	MinAuthorLevel                   int
	EligibilityWhitelistUserIDs      []int64
	RiskThresholdPercentile          int
	RequireLLMConfidence             int
	BlockingRulesetIDs               []int64
	LandingDelaySeconds              int
	DailyApproveCap                  *int
	RestrictOutcomeReportingToAdmins bool
	ForceApproveMinLevel             int
}

// ResolvePolicy returns the effective policy for a review in orgID/repositoryID:
// the repo-scoped row if present and not 'inherit', else the org-default row,
// else (found=false) the feature is off. repositoryID 0 means "no repo scope".
func ResolvePolicy(ctx context.Context, db *sql.DB, orgID, repositoryID int64) (*Policy, bool, error) {
	if repositoryID > 0 {
		p, err := loadPolicy(ctx, db, orgID, &repositoryID)
		if err != nil {
			return nil, false, err
		}
		if p != nil && p.Mode != ModeInherit {
			return p, true, nil
		}
	}

	p, err := loadPolicy(ctx, db, orgID, nil)
	if err != nil {
		return nil, false, err
	}
	if p == nil {
		return nil, false, nil
	}
	return p, true, nil
}

// loadPolicy loads one row; repositoryID nil selects the org-default row.
func loadPolicy(ctx context.Context, db *sql.DB, orgID int64, repositoryID *int64) (*Policy, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, org_id, repository_id, mode, min_author_level,
		       eligibility_whitelist_user_ids, risk_threshold_percentile,
		       require_llm_confidence, blocking_ruleset_ids, landing_delay_seconds,
		       daily_approve_cap, restrict_outcome_reporting_to_admins, force_approve_min_level
		FROM auto_approve_policies
		WHERE org_id = $1 AND repository_id IS NOT DISTINCT FROM $2`, orgID, repositoryID)

	var p Policy
	var repoID sql.NullInt64
	err := row.Scan(
		&p.ID, &p.OrgID, &repoID, &p.Mode, &p.MinAuthorLevel,
		pq.Array(&p.EligibilityWhitelistUserIDs), &p.RiskThresholdPercentile,
		&p.RequireLLMConfidence, pq.Array(&p.BlockingRulesetIDs), &p.LandingDelaySeconds,
		&p.DailyApproveCap, &p.RestrictOutcomeReportingToAdmins, &p.ForceApproveMinLevel,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if repoID.Valid {
		p.RepositoryID = &repoID.Int64
	}
	return &p, nil
}
