package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/jobqueue"
	"github.com/livereview/internal/policyengine"
	"github.com/livereview/internal/riskscore"
)

// requireSession rejects API-key-authenticated requests (sensitive account
// actions require a live JWT session per the scoping guardrails).
func requireSession(c echo.Context) error {
	if c.Get(authMethodContextKey) != authMethodSession {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "this action requires an active user session")
	}
	return nil
}

// getReviewPolicyLog handles GET /api/v1/reviews/:id/policy-log.
func (s *Server) getReviewPolicyLog(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid review id")
	}

	rows, err := s.db.QueryContext(c.Request().Context(), `
		SELECT stage, verdict, detail FROM review_policy_log
		WHERE review_id = $1 AND org_id = $2 ORDER BY id`, reviewID, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "query failed: "+err.Error())
	}
	defer rows.Close()

	type stageRow struct {
		Stage   string          `json:"stage"`
		Verdict string          `json:"verdict"`
		Detail  json.RawMessage `json:"detail"`
	}
	out := []stageRow{}
	for rows.Next() {
		var r stageRow
		if err := rows.Scan(&r.Stage, &r.Verdict, &r.Detail); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "scan failed: "+err.Error())
		}
		out = append(out, r)
	}

	// The feature snapshot for the risk stage's explainability view.
	var snapshot struct {
		ModelVersion int     `json:"model_version"`
		Features     []byte  `json:"-"`
		Score        float64 `json:"score"`
	}
	features := []float64{}
	var featuresRaw []byte
	var snapshotScore float64
	var modelVersion int
	if err := s.db.QueryRowContext(c.Request().Context(), `
		SELECT model_version, features, score FROM risk_feature_snapshots
		WHERE review_id = $1 AND org_id = $2`, reviewID, orgID).
		Scan(&modelVersion, &featuresRaw, &snapshotScore); err == nil {
		_ = json.Unmarshal(featuresRaw, &features)
		snapshot.ModelVersion = modelVersion
		snapshot.Score = snapshotScore
	}

	// Force-approve eligibility context: is the review blocked, and what's the floor?
	blocked := false
	for _, r := range out {
		if r.Stage == policyengine.StageFinal && r.Verdict == policyengine.FinalBlocked {
			blocked = true
		}
	}
	forceApproveMinLevel := 3
	if p, found, _ := policyengine.ResolvePolicy(c.Request().Context(), s.db, orgID, 0); found {
		forceApproveMinLevel = p.ForceApproveMinLevel
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"stages":                  out,
		"blocked":                 blocked,
		"force_approve_min_level": forceApproveMinLevel,
		"features":                features,
		"feature_names":           riskscore.FeatureSpecs,
		"model_version":           modelVersion,
		"score":                   snapshotScore,
	})
}

// forceApproveReview handles POST /api/v1/reviews/:id/force-approve (session-only,
// gated by review_level OR owner/admin).
func (s *Server) forceApproveReview(c echo.Context) error {
	if err := requireSession(c); err != nil {
		return err
	}
	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid review id")
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Reason == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "reason is required")
	}

	// Resolve the policy floor for force-approve.
	forceApproveMinLevel := 3
	if p, found, _ := policyengine.ResolvePolicy(c.Request().Context(), s.db, orgID, 0); found {
		forceApproveMinLevel = p.ForceApproveMinLevel
	}

	// Authorize: review_level >= floor OR owner/admin.
	var actorLevel int
	if err := s.db.QueryRowContext(c.Request().Context(), `SELECT review_level FROM users WHERE id = $1`, pc.GetUserID()).Scan(&actorLevel); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load user level")
	}
	if actorLevel < forceApproveMinLevel && !pc.IsOwner && !pc.IsSuperAdmin {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "insufficient review level or role to force-approve")
	}

	// Only blocked -> approved; there is no symmetric force-request-changes.
	var finalVerdict string
	if err := s.db.QueryRowContext(c.Request().Context(), `
		SELECT verdict FROM review_policy_log
		WHERE review_id = $1 AND org_id = $2 AND stage = 'final' ORDER BY id DESC LIMIT 1`,
		reviewID, orgID).Scan(&finalVerdict); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "no policy evaluation found for this review")
	}
	if finalVerdict != policyengine.FinalBlocked {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "review is not blocked")
	}

	_, err = s.db.ExecContext(c.Request().Context(), `
		INSERT INTO review_policy_overrides (review_id, org_id, actor_user_id, original_verdict, forced_verdict, reason)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		reviewID, orgID, pc.GetUserID(), finalVerdict, policyengine.FinalApproved, body.Reason)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to record override: "+err.Error())
	}

	if err := s.jobQueue.QueuePRPolicyForceApprove(c.Request().Context(), jobqueue.PRPolicyForceApproveJobArgs{
		OrgID:       orgID,
		ReviewID:    reviewID,
		ActorUserID: pc.GetUserID(),
		Reason:      body.Reason,
	}); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to queue force-approve: "+err.Error())
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "force-approved"})
}

// markReviewOutcome handles POST /api/v1/reviews/:id/outcomes (manual in-app
// "Mark as reverted/incident" action).
func (s *Server) markReviewOutcome(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()
	reviewID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid review id")
	}

	var body struct {
		OutcomeType string `json:"outcome_type"`
		Reason      string `json:"reason"`
	}
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Reason == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "reason is required")
	}
	if body.OutcomeType != "revert" && body.OutcomeType != "incident" && body.OutcomeType != "confirmed_safe" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid outcome_type")
	}

	// Confirm the review belongs to this org before writing (never trust a
	// caller-supplied review id without checking ownership first).
	var exists bool
	if err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT EXISTS(SELECT 1 FROM reviews WHERE id = $1 AND org_id = $2)`, reviewID, orgID).Scan(&exists); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "lookup failed: "+err.Error())
	}
	if !exists {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "review not found in this organization")
	}

	if err := s.insertOutcome(c, orgID, reviewID, pc.GetUserID(), body.OutcomeType, "manual", body.Reason, nil); err != nil {
		return err
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "recorded", "outcome_type": body.OutcomeType})
}

// insertOutcome inserts a review_outcomes row and (for non-test sources) fires
// the real-time online model nudge.
func (s *Server) insertOutcome(c echo.Context, orgID, reviewID, userID int64, outcomeType, source, reason string, detail map[string]interface{}) error {
	// Permission lockdown switch (default open): when set, only owner/admin may
	// report an outcome in-app. Webhooks are always authenticated by API key.
	if source == "manual" && s.restrictOutcomeReporting(orgID) {
		pc := auth.MustGetPermissionContext(c)
		if !pc.IsOwner && !pc.IsSuperAdmin {
			return JSONErrorWithEnvelope(c, http.StatusForbidden, "outcome reporting is restricted to admins")
		}
	}

	detailJSON := "{}"
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON = string(b)
		}
	}
	var reportedBy interface{}
	if userID > 0 {
		reportedBy = userID
	}
	if _, err := s.db.ExecContext(c.Request().Context(), `
		INSERT INTO review_outcomes (review_id, org_id, reported_by_user_id, outcome_type, source, reason, detail)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		reviewID, orgID, reportedBy, outcomeType, source, reason, detailJSON); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to record outcome: "+err.Error())
	}

	// Real-time online nudge (path 1), skipped for test events.
	if source != "test" {
		store := riskscore.NewStore(s.db)
		if err := store.ApplyOutcome(c.Request().Context(), orgID, reviewID, outcomeType); err != nil {
			// Never fail the request over a learning update.
			_ = err
		}
	}
	return nil
}

// restrictOutcomeReporting returns the org's lockdown switch.
func (s *Server) restrictOutcomeReporting(orgID int64) bool {
	var restrict bool
	if err := s.db.QueryRow(`SELECT COALESCE(restrict_outcome_reporting_to_admins, false)
		FROM auto_approve_policies WHERE org_id = $1 AND repository_id IS NULL`, orgID).Scan(&restrict); err != nil {
		return false
	}
	return restrict
}
