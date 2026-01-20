package middleware

import (
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/license"
)

// seatCountCache caches the active user count to avoid excessive DB queries
type seatCountCache struct {
	count      int
	lastUpdate time.Time
	mu         sync.RWMutex
	ttl        time.Duration
}

var (
	// Global cache for active user seat count (5 minute TTL)
	activeSeatCache = &seatCountCache{
		ttl: 5 * time.Minute,
	}
)

// getActiveUserCount returns the count of active users with caching
func (sc *seatCountCache) getActiveUserCount(db *sql.DB) (int, error) {
	sc.mu.RLock()
	if time.Since(sc.lastUpdate) < sc.ttl {
		count := sc.count
		sc.mu.RUnlock()
		return count, nil
	}
	sc.mu.RUnlock()

	// Cache expired, fetch fresh count
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Double-check after acquiring write lock
	if time.Since(sc.lastUpdate) < sc.ttl {
		return sc.count, nil
	}

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = true").Scan(&count)
	if err != nil {
		return 0, err
	}

	sc.count = count
	sc.lastUpdate = time.Now()
	return count, nil
}

// isAdminOrOwner checks if the user has admin or owner role in any organization
func isAdminOrOwner(db *sql.DB, userID int64) (bool, error) {
	var hasAdminRole bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles r ON ur.role_id = r.id
			WHERE ur.user_id = $1 
			AND r.name IN ('admin', 'owner')
		)
	`, userID).Scan(&hasAdminRole)

	return hasAdminRole, err
}

// EnforceSelfHostedLicense checks license validity and seat limits for self-hosted deployments
// Only runs when LIVEREVIEW_IS_CLOUD=false
func EnforceSelfHostedLicense(db *sql.DB, licenseService *license.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// CRITICAL: Only enforce in self-hosted mode
			// In cloud mode, use subscription-based enforcement instead
			if isCloudMode() {
				return next(c)
			}

			// Get JWT claims from context (set by auth middleware)
			claims, ok := c.Get("claims").(*auth.JWTClaims)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or missing authentication")
			}

			// Get current license state from service
			state, err := licenseService.LoadOrInit(c.Request().Context())
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "Failed to validate license")
			}

			// Block if license is missing, expired, or invalid
			if state.Status == "missing" {
				return echo.NewHTTPError(http.StatusPaymentRequired,
					"No license found. Please enter a valid license to continue.")
			}

			if state.Status == "expired" {
				return echo.NewHTTPError(http.StatusPaymentRequired,
					"Your license has expired. Please renew to continue using LiveReview.")
			}

			if state.Status == "invalid" {
				return echo.NewHTTPError(http.StatusPaymentRequired,
					"Your license is invalid. Please enter a valid license.")
			}

			// Check seat count limit (only if license has seat limit)
			if !state.Unlimited && state.SeatCount != nil && *state.SeatCount > 0 {
				// Check if user is admin/owner - they bypass seat limits
				isAdmin, err := isAdminOrOwner(db, claims.UserID)
				if err != nil {
					// Log error but don't block - fail open for admin check
					c.Logger().Errorf("Failed to check admin status: %v", err)
				}

				if !isAdmin {
					// Get active user count (cached)
					activeUsers, err := activeSeatCache.getActiveUserCount(db)
					if err != nil {
						c.Logger().Errorf("Failed to get active user count: %v", err)
						return echo.NewHTTPError(http.StatusInternalServerError,
							"Failed to validate license seat count")
					}

					// Block if seat limit exceeded
					if activeUsers > *state.SeatCount {
						return echo.NewHTTPError(http.StatusForbidden,
							"License seat limit exceeded. Please upgrade your license or deactivate unused users.")
					}
				}
			}

			return next(c)
		}
	}
}
