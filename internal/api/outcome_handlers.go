package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

// orgReviewOutcome handles POST /api/v1/orgs/:org_id/review-outcomes — the
// documented webhook endpoint CI/CD or incident tooling POSTs to report a
// rollback/incident against a known review id. Authenticated by API key or JWT
// via the standard org middleware chain.
func (s *Server) orgReviewOutcome(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}

	var body struct {
		ReviewID    int64                  `json:"review_id"`
		OutcomeType string                 `json:"outcome_type"`
		Reason      string                 `json:"reason"`
		Source      string                 `json:"source"`
		Detail      map[string]interface{} `json:"detail"`
	}
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	if body.ReviewID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "review_id is required")
	}
	if body.OutcomeType != "revert" && body.OutcomeType != "incident" && body.OutcomeType != "confirmed_safe" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid outcome_type")
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Reason == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "reason is required")
	}
	source := body.Source
	if source == "" {
		source = "webhook"
	}
	if source != "webhook" && source != "ci" && source != "test" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid source")
	}

	// Confirm the review belongs to this org before writing.
	var exists bool
	if err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT EXISTS(SELECT 1 FROM reviews WHERE id = $1 AND org_id = $2)`, body.ReviewID, orgID).Scan(&exists); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "lookup failed: "+err.Error())
	}
	if !exists {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "review not found in this organization")
	}

	if err := s.insertOutcome(c, orgID, body.ReviewID, 0, body.OutcomeType, source, body.Reason, body.Detail); err != nil {
		return err
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "recorded", "outcome_type": body.OutcomeType})
}

// listReviewOutcomes handles GET /api/v1/orgs/:org_id/review-outcomes — the
// "last 30 days" counter on the setup panel.
func (s *Server) listReviewOutcomes(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}

	rows, err := s.db.QueryContext(c.Request().Context(), `
		SELECT review_id, reported_by_user_id, outcome_type, source, reason, detail, created_at
		FROM review_outcomes
		WHERE org_id = $1 AND created_at >= now() - interval '30 days'
		ORDER BY created_at DESC LIMIT 500`, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "query failed: "+err.Error())
	}
	defer rows.Close()

	type outcomeRow struct {
		ReviewID       int64           `json:"review_id"`
		ReportedBy     *int64          `json:"reported_by_user_id,omitempty"`
		OutcomeType    string          `json:"outcome_type"`
		Source         string          `json:"source"`
		Reason         string          `json:"reason"`
		Detail         json.RawMessage `json:"detail"`
		CreatedAt      time.Time       `json:"created_at"`
	}
	out := []outcomeRow{}
	manualCount, webhookCount := 0, 0
	for rows.Next() {
		var r outcomeRow
		var detail []byte
		if err := rows.Scan(&r.ReviewID, &r.ReportedBy, &r.OutcomeType, &r.Source, &r.Reason, &detail, &r.CreatedAt); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "scan failed: "+err.Error())
		}
		r.Detail = detail
		if r.Source == "manual" {
			manualCount++
		} else if r.Source != "test" {
			webhookCount++
		}
		out = append(out, r)
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"rows":          out,
		"total":         len(out),
		"manual_count":  manualCount,
		"webhook_count": webhookCount,
	})
}
