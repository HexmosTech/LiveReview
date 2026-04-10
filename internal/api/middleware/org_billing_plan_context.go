package middleware

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/plancode"
)

// BuildOrgBillingPlanContext populates plan_type from org_billing_state.current_plan_code
// using org_id already attached by upstream middleware.
func BuildOrgBillingPlanContext(db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if db == nil {
				return next(c)
			}

			orgID, ok := readOrgIDFromContext(c)
			if !ok {
				return next(c)
			}

			var currentPlanCode sql.NullString
			err := db.QueryRowContext(c.Request().Context(), `
				SELECT current_plan_code
				FROM org_billing_state
				WHERE org_id = $1
			`, orgID).Scan(&currentPlanCode)
			if err != nil {
				if err == sql.ErrNoRows {
					return next(c)
				}
				return echo.NewHTTPError(http.StatusInternalServerError, "failed to resolve organization plan")
			}

			normalizedPlan := plancode.NormalizePlanTypeCode(currentPlanCode.String)
			if normalizedPlan != "" {
				c.Set("plan_type", normalizedPlan)
			}

			return next(c)
		}
	}
}

func readOrgIDFromContext(c echo.Context) (int64, bool) {
	v := c.Get("org_id")
	switch value := v.(type) {
	case int64:
		if value > 0 {
			return value, true
		}
	case int:
		if value > 0 {
			return int64(value), true
		}
	case float64:
		if value > 0 {
			return int64(value), true
		}
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}

	return 0, false
}
