package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/riskscore"
)

// getRiskModel handles GET /api/v1/risk-model — the Risk Model sub-tab's data:
// the active version/sample-count (model-confidence indicator), the category
// overrides, and the recent score distribution for the calibration chart.
func (s *Server) getRiskModel(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}

	scorer := riskscore.NewScorer(s.db)
	info, err := scorer.Info(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load model: "+err.Error())
	}

	store := riskscore.NewStore(s.db)
	overrides, err := store.LoadOverrides(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load overrides: "+err.Error())
	}
	// Fill in categories the org hasn't explicitly overridden with their
	// starting default so the UI's sliders render at the real effective
	// value (e.g. Security pre-set to 1.5x) rather than always showing 1x
	// until someone saves a row.
	for _, cat := range riskscore.OverrideCategories() {
		if _, ok := overrides[cat]; !ok {
			overrides[cat] = riskscore.DefaultCategoryMultiplier(cat)
		}
	}
	scores, err := riskscore.LoadRecentScores(c.Request().Context(), s.db, orgID, 2000)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to load calibration: "+err.Error())
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"model":     info,
		"overrides": overrides,
		"scores":    scores,
	})
}

// putRiskModelOverrides handles PUT /api/v1/risk-model/overrides — save category
// multipliers (instant effect, decoupled from the retrain cadence).
func (s *Server) putRiskModelOverrides(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()

	var body struct {
		Overrides map[string]float64 `json:"overrides"`
	}
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}

	store := riskscore.NewStore(s.db)
	for category, multiplier := range body.Overrides {
		if err := store.UpsertOverride(c.Request().Context(), orgID, category, multiplier); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid override for "+category+": "+err.Error())
		}
	}
	overrides, _ := store.LoadOverrides(c.Request().Context(), orgID)
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"overrides": overrides})
}

// retrainRiskModel handles POST /api/v1/risk-model/retrain — the on-demand batch
// refit (same as the daily job).
func (s *Server) retrainRiskModel(c echo.Context) error {
	pc := auth.MustGetPermissionContext(c)
	if err := pc.RequireOrgOwner(); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, err.Error())
	}
	orgID := pc.GetOrgID()

	res, err := riskscore.NewStore(s.db).Refit(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "retrain failed: "+err.Error())
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"result": res})
}
