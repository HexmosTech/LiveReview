package plancode

import (
	"strings"

	"github.com/livereview/internal/license"
)

// NormalizePlanTypeCode maps legacy and unknown values to canonical plan codes.
func NormalizePlanTypeCode(raw string) string {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return string(license.PlanFree)
	}

	candidate := license.PlanType(trimmed)
	if candidate.IsValid() {
		return candidate.String()
	}

	switch trimmed {
	case "free":
		return string(license.PlanFree)
	case "team", "team_monthly", "team_annual", "team_yearly", "monthly", "yearly":
		return string(license.PlanTeam)
	default:
		return string(license.PlanFree)
	}
}
