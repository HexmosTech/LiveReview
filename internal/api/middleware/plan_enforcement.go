package middleware

import (
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/license"
)

// isCloudMode checks if running in cloud deployment mode
func isCloudMode() bool {
	valueStr := os.Getenv("LIVEREVIEW_IS_CLOUD")
	if valueStr == "" {
		return false
	}
	valueStr = strings.ToLower(strings.TrimSpace(valueStr))
	return valueStr == "true" || valueStr == "1"
}

// IsCloudMode is the exported version of isCloudMode for use by other packages
func IsCloudMode() bool {
	return isCloudMode()
}

// EnforcePlan checks if user's plan allows access to a specific feature
func EnforcePlan(requiredFeature string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get JWT claims from context (set by auth middleware)
			claims, ok := c.Get("claims").(*auth.JWTClaims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or missing authentication")
			}

			// Check license expiration first
			if claims.LicenseExpiresAt != nil {
				expiryTime := time.Unix(*claims.LicenseExpiresAt, 0)
				if time.Now().After(expiryTime) {
					return echo.NewHTTPError(http.StatusPaymentRequired,
						"Your license has expired. Please renew to continue.")
				}
			}

			// Get plan type and check feature access
			planType := license.PlanType(claims.PlanType)
			if !planType.HasFeature(requiredFeature) {
				return echo.NewHTTPError(http.StatusForbidden,
					"This feature requires an upgrade to Team plan")
			}

			return next(c)
		}
	}
}

// CheckReviewLimit enforces daily review limits based on plan
func CheckReviewLimit(db *sql.DB) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// CRITICAL: Only enforce limits in cloud mode
			// In self-hosted mode, skip all subscription/plan checks
			if !isCloudMode() {
				return next(c)
			}

			// Get JWT claims from context
			claims, ok := c.Get("claims").(*auth.JWTClaims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or missing authentication")
			}

			// Check license expiration first
			if claims.LicenseExpiresAt != nil {
				expiryTime := time.Unix(*claims.LicenseExpiresAt, 0)
				if time.Now().After(expiryTime) {
					return echo.NewHTTPError(http.StatusPaymentRequired,
						"Your license has expired. Please renew to continue.")
				}
			}

			// Get plan limits
			planType := license.PlanType(claims.PlanType)
			limits := planType.GetLimits()

			// If unlimited reviews, skip the check
			if limits.MaxReviewsPerDay == -1 {
				return next(c)
			}

			// Count today's reviews for this user in this org
			var reviewCount int
			err := db.QueryRow(`
				SELECT COUNT(*) 
				FROM reviews 
				WHERE created_by_user_id = $1 
				AND org_id = $2
				AND created_at >= CURRENT_DATE
			`, claims.UserID, claims.CurrentOrgID).Scan(&reviewCount)

			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError,
					"Failed to check review limit")
			}

			// Check if limit exceeded
			if reviewCount >= limits.MaxReviewsPerDay {
				return echo.NewHTTPError(http.StatusTooManyRequests,
					"Daily review limit reached. Upgrade to Team plan for unlimited reviews.")
			}

			return next(c)
		}
	}
}

// RequirePlan ensures user has at least the specified plan level
func RequirePlan(minPlan license.PlanType) echo.MiddlewareFunc {
	// Plan hierarchy: free < team < enterprise
	planHierarchy := map[license.PlanType]int{
		license.PlanFree:       0,
		license.PlanTeam:       1,
		license.PlanEnterprise: 2,
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			claims, ok := c.Get("claims").(*auth.JWTClaims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or missing authentication")
			}

			// Check license expiration
			if claims.LicenseExpiresAt != nil {
				expiryTime := time.Unix(*claims.LicenseExpiresAt, 0)
				if time.Now().After(expiryTime) {
					return echo.NewHTTPError(http.StatusPaymentRequired,
						"Your license has expired. Please renew to continue.")
				}
			}

			// Compare plan levels
			userPlanLevel := planHierarchy[license.PlanType(claims.PlanType)]
			requiredPlanLevel := planHierarchy[minPlan]

			if userPlanLevel < requiredPlanLevel {
				return echo.NewHTTPError(http.StatusForbidden,
					"This feature requires "+string(minPlan)+" plan or higher")
			}

			return next(c)
		}
	}
}
