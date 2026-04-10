package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/plancode"
	"github.com/livereview/internal/license"
	"github.com/livereview/pkg/models"
)

// QuotaStatusHandler handles quota/status endpoint
type QuotaStatusHandler struct {
	db *sql.DB
}

// NewQuotaStatusHandler creates a new quota status handler
func NewQuotaStatusHandler(db *sql.DB) *QuotaStatusHandler {
	return &QuotaStatusHandler{db: db}
}

// QuotaStatus represents the current quota status for an organization
type QuotaStatus struct {
	PlanType           string            `json:"plan_type"`
	DailyLimit         *int              `json:"daily_limit"`
	DailyUsed          int               `json:"daily_used"`
	CanActivateMembers bool              `json:"can_activate_members"`
	SeatsAvailable     *int              `json:"seats_available,omitempty"`
	SeatsTotal         *int              `json:"seats_total,omitempty"`
	SeatsAssigned      *int              `json:"seats_assigned,omitempty"`
	IsOrgCreator       bool              `json:"is_org_creator"`
	CanTriggerReviews  bool              `json:"can_trigger_reviews"`
	Envelope           PlanUsageEnvelope `json:"envelope"`
}

// GetQuotaStatus returns the current quota status for the user's organization
func (h *QuotaStatusHandler) GetQuotaStatus(c echo.Context) error {
	// Get org_id and user from context
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	user, ok := c.Get("user").(*models.User)
	if !ok || user == nil {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "user authentication required")
	}

	dailyLimitPtr, _ := c.Get("daily_review_limit").(*int)

	// Check if user is org creator
	var isOrgCreator bool
	err := h.db.QueryRow(`
		SELECT (o.created_by_user_id = $1) as is_creator
		FROM orgs o
		WHERE o.id = $2
	`, user.ID, orgID).Scan(&isOrgCreator)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "failed to check org creator status")
	}

	// Count reviews created today by this user
	var dailyUsed int
	err = h.db.QueryRow(`
		SELECT COUNT(*)
		FROM reviews
		WHERE org_id = $1
		  AND user_email = $2
		  AND created_at >= CURRENT_DATE
	`, orgID, user.Email).Scan(&dailyUsed)
	if err != nil {
		dailyUsed = 0 // Default to 0 if query fails
	}

	envelope := BuildEnvelopeFromContext(c)
	contextPlanType, _ := c.Get("plan_type").(string)
	planType := resolveQuotaPlanType(envelope.PlanCode, contextPlanType)

	isFreeTier := isQuotaFreePlan(planType)
	status := QuotaStatus{
		PlanType:     planType,
		DailyLimit:   dailyLimitPtr,
		DailyUsed:    dailyUsed,
		IsOrgCreator: isOrgCreator,
		Envelope:     envelope,
	}

	// Determine if user can trigger reviews
	if isFreeTier {
		// On free plan, only org creator can trigger reviews AND must be under daily limit
		status.CanTriggerReviews = isOrgCreator && (dailyLimitPtr == nil || dailyUsed < *dailyLimitPtr)
	} else {
		// On team plan, all members can trigger unlimited reviews
		status.CanTriggerReviews = true
	}

	// Determine if user can activate members
	status.CanActivateMembers = !isFreeTier

	// If on paid plans, get subscription seat information.
	if !isFreeTier {
		var seatsTotal, seatsAssigned int
		err := h.db.QueryRow(`
			SELECT s.quantity, 
			       COALESCE((
					SELECT COUNT(*)
					FROM user_roles ur
					WHERE ur.active_subscription_id = s.id
					  AND LOWER(TRIM(COALESCE(ur.plan_type, ''))) NOT IN ('free', 'free_30k')
				), 0) as assigned_seats
			FROM subscriptions s
			WHERE s.org_id = $1
			  AND s.status IN ('active', 'authenticated')
			ORDER BY s.updated_at DESC, s.created_at DESC
			LIMIT 1
		`, orgID).Scan(&seatsTotal, &seatsAssigned)

		if err == nil {
			status.SeatsTotal = &seatsTotal
			status.SeatsAssigned = &seatsAssigned
			seatsAvailable := seatsTotal - seatsAssigned
			status.SeatsAvailable = &seatsAvailable
		}
	}

	return c.JSON(http.StatusOK, status)
}

func resolveQuotaPlanType(envelopePlanCode string, contextPlanType string) string {
	if strings.TrimSpace(envelopePlanCode) != "" {
		return plancode.NormalizePlanTypeCode(envelopePlanCode)
	}

	if strings.TrimSpace(contextPlanType) != "" {
		return plancode.NormalizePlanTypeCode(contextPlanType)
	}

	return plancode.NormalizePlanTypeCode("")
}

func isQuotaFreePlan(planCode string) bool {
	normalized := plancode.NormalizePlanTypeCode(planCode)
	return normalized == string(license.PlanFree)
}
