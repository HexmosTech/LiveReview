package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/license/payment"
)

// SubscriptionsHandler handles subscription-related API endpoints
//
// Timestamp Handling:
// All timestamp fields (created_at, updated_at, current_period_start, current_period_end, license_expires_at)
// are returned in RFC3339 format with timezone information (e.g., "2024-12-10T14:30:00Z" or "2024-12-10T14:30:00+05:30").
// This allows frontend applications to:
//   - Display times in the user's local timezone
//   - Show human-friendly relative times (e.g., "2 hours ago")
//   - Sort subscriptions accurately by creation time
//   - Distinguish between multiple subscriptions created on the same day
//
// Frontend implementations should use JavaScript's Date object or libraries like date-fns/dayjs
// to parse and format these timestamps according to user preferences.
type SubscriptionsHandler struct {
	service *payment.SubscriptionService
	db      *sql.DB
}

// NewSubscriptionsHandler creates a new subscriptions handler
func NewSubscriptionsHandler(db *sql.DB) *SubscriptionsHandler {
	return &SubscriptionsHandler{
		service: payment.NewSubscriptionService(db),
		db:      db,
	}
}

// CreateSubscriptionRequest represents the request to create a subscription
type CreateSubscriptionRequest struct {
	PlanType string `json:"plan_type"` // "monthly" or "yearly"
	Quantity int    `json:"quantity"`  // Number of seats
}

// CreateSubscription creates a new team subscription
func (h *SubscriptionsHandler) CreateSubscription(c echo.Context) error {
	// Get org context (set by middleware)
	orgIDVal := c.Get("org_id")
	var orgID int64
	switch v := orgIDVal.(type) {
	case int64:
		orgID = v
	case int:
		orgID = int64(v)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required (create subscription handler)",
		})
	}

	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required (create subscription handler)",
		})
	}

	// Get authenticated user from context
	user := auth.GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}
	userID := int(user.ID)

	// Parse request
	var req CreateSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	// Normalize plan type to internal values
	switch req.PlanType {
	case "team_monthly":
		req.PlanType = "monthly"
	case "team_annual", "team_yearly", "annual":
		req.PlanType = "yearly"
	}

	// Validate plan type after normalization
	if req.PlanType != "monthly" && req.PlanType != "yearly" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "plan_type must be 'monthly', 'yearly', 'team_monthly', or 'team_annual'",
		})
	}

	// Validate quantity
	if req.Quantity < 1 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "quantity must be at least 1",
		})
	}

	// Determine mode (test vs live)
	mode := "test" // TODO: Get from config or environment

	// Create subscription
	sub, err := h.service.CreateTeamSubscription(userID, int(orgID), req.PlanType, req.Quantity, mode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	// Fetch Razorpay public key for checkout initialization
	keyID, _, err := payment.GetRazorpayKeys(mode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to resolve Razorpay configuration",
		})
	}

	response := map[string]interface{}{
		"razorpay_subscription_id": sub.ID,
		"razorpay_key_id":          keyID,
		"status":                   sub.Status,
		"quantity":                 req.Quantity,
		"plan_type":                req.PlanType,
		"short_url":                sub.ShortURL,
		"current_period_start":     sub.CurrentStart,
		"current_period_end":       sub.CurrentEnd,
	}

	return c.JSON(http.StatusCreated, response)
}

// UpdateQuantityRequest represents the request to update subscription quantity
type UpdateQuantityRequest struct {
	Quantity         int   `json:"quantity"`
	ScheduleChangeAt int64 `json:"schedule_change_at,omitempty"` // Unix timestamp, optional
}

// UpdateQuantity updates the quantity of a subscription
func (h *SubscriptionsHandler) UpdateQuantity(c echo.Context) error {
	// Get subscription ID from URL
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "subscription_id required",
		})
	}

	// Parse request
	var req UpdateQuantityRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	// Validate quantity
	if req.Quantity < 1 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "quantity must be at least 1",
		})
	}

	// Determine mode
	mode := "test" // TODO: Get from config or environment

	// Update quantity
	sub, err := h.service.UpdateQuantity(subscriptionID, req.Quantity, req.ScheduleChangeAt, mode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, sub)
}

// CancelSubscriptionRequest represents the request to cancel a subscription
type CancelSubscriptionRequest struct {
	Immediate bool `json:"immediate"` // true = cancel immediately, false = cancel at end of cycle
}

// CancelSubscription cancels a subscription
func (h *SubscriptionsHandler) CancelSubscription(c echo.Context) error {
	// Get subscription ID from URL
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "subscription_id required",
		})
	}

	// Parse request
	var req CancelSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	// Determine mode
	mode := "test" // TODO: Get from config or environment

	// Cancel subscription
	sub, err := h.service.CancelSubscription(subscriptionID, req.Immediate, mode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, sub)
}

// GetSubscription retrieves subscription details
func (h *SubscriptionsHandler) GetSubscription(c echo.Context) error {
	// Get subscription ID from URL
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "subscription_id required",
		})
	}

	// Query subscription from database with all fields
	type SubscriptionResponse struct {
		ID                     int64      `json:"id"`
		RazorpaySubscriptionID string     `json:"razorpay_subscription_id"`
		OwnerUserID            int        `json:"owner_user_id"`
		OrgID                  int        `json:"org_id"`
		PlanType               string     `json:"plan_type"`
		Quantity               int        `json:"quantity"`
		AssignedSeats          int        `json:"assigned_seats"`
		Status                 string     `json:"status"`
		RazorpayPlanID         string     `json:"razorpay_plan_id"`
		CurrentPeriodStart     *time.Time `json:"current_period_start,omitempty"`
		CurrentPeriodEnd       *time.Time `json:"current_period_end,omitempty"`
		CreatedAt              time.Time  `json:"created_at"`
		UpdatedAt              time.Time  `json:"updated_at"`
		PaymentVerified        bool       `json:"payment_verified"`
		LastPaymentID          *string    `json:"last_payment_id,omitempty"`
		LastPaymentStatus      *string    `json:"last_payment_status,omitempty"`
	}

	var sub SubscriptionResponse
	err := h.db.QueryRow(`
		SELECT 
			id, razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, 
			COALESCE((SELECT COUNT(*) FROM user_roles ur WHERE ur.active_subscription_id = s.id AND ur.plan_type = 'team'), 0) as assigned_seats,
			status, razorpay_plan_id,
			current_period_start, current_period_end,
			created_at, updated_at,
			payment_verified, last_payment_id, last_payment_status
		FROM subscriptions s
		WHERE razorpay_subscription_id = $1
	`, subscriptionID).Scan(
		&sub.ID, &sub.RazorpaySubscriptionID, &sub.OwnerUserID, &sub.OrgID, &sub.PlanType,
		&sub.Quantity, &sub.AssignedSeats, &sub.Status, &sub.RazorpayPlanID,
		&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.CreatedAt, &sub.UpdatedAt,
		&sub.PaymentVerified, &sub.LastPaymentID, &sub.LastPaymentStatus,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "subscription not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch subscription",
		})
	}

	return c.JSON(http.StatusOK, sub)
}

// AssignLicenseRequest represents the request to assign a license to a user
type AssignLicenseRequest struct {
	UserID int `json:"user_id"`
}

// AssignLicense assigns a license from a subscription to a user
func (h *SubscriptionsHandler) AssignLicense(c echo.Context) error {
	// Get subscription ID from URL
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "subscription_id required",
		})
	}

	// Get authenticated user
	user := auth.GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Get org context
	orgIDVal := c.Get("org_id")
	var orgID int64
	switch v := orgIDVal.(type) {
	case int64:
		orgID = v
	case int:
		orgID = int64(v)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required (assign license handler)",
		})
	}

	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required (assign license handler)",
		})
	}

	// Verify user owns this subscription
	var ownerUserID int
	err := h.db.QueryRow(`
		SELECT owner_user_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&ownerUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "subscription not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to verify subscription ownership",
		})
	}

	if ownerUserID != int(user.ID) {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "only subscription owner can assign licenses",
		})
	}

	// Parse request
	var req AssignLicenseRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	// Assign license
	if err := h.service.AssignLicense(subscriptionID, req.UserID, int(orgID)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "license assigned successfully",
	})
}

// RevokeLicense revokes a license from a user
func (h *SubscriptionsHandler) RevokeLicense(c echo.Context) error {
	// Get subscription ID from URL
	subscriptionID := c.Param("id")
	if subscriptionID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "subscription_id required",
		})
	}

	// Get authenticated user
	user := auth.GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Get org context
	orgIDVal := c.Get("org_id")
	var orgID int64
	switch v := orgIDVal.(type) {
	case int64:
		orgID = v
	case int:
		orgID = int64(v)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required (revoke license handler)",
		})
	}

	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required (revoke license handler)",
		})
	}

	// Verify user owns this subscription
	var ownerUserID int
	err := h.db.QueryRow(`
		SELECT owner_user_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&ownerUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, map[string]string{
				"error": "subscription not found",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to verify subscription ownership",
		})
	}

	if ownerUserID != int(user.ID) {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "only subscription owner can revoke licenses",
		})
	}

	// Get user_id from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid user_id",
		})
	}

	// Revoke license
	if err := h.service.RevokeLicense(subscriptionID, userID, int(orgID)); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "license revoked successfully",
	})
}

// ListUserSubscriptions lists all subscriptions owned by the authenticated user
func (h *SubscriptionsHandler) ListUserSubscriptions(c echo.Context) error {
	// Get authenticated user
	user := auth.GetUser(c)
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Query subscriptions owned by the user with calculated assigned_seats from user_roles
	// Only return subscriptions that are active or have assigned seats
	rows, err := h.db.Query(`
		SELECT 
			s.id, s.razorpay_subscription_id, s.owner_user_id, s.org_id, s.plan_type,
			s.quantity, 
			COALESCE((SELECT COUNT(*) FROM user_roles ur WHERE ur.active_subscription_id = s.id AND ur.plan_type = 'team'), 0) as assigned_seats,
			s.status, s.razorpay_plan_id,
			s.current_period_start, s.current_period_end, s.license_expires_at,
			s.created_at, s.updated_at
		FROM subscriptions s
		WHERE s.owner_user_id = $1
		  AND (s.status IN ('created', 'authenticated', 'active') OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.active_subscription_id = s.id))
		ORDER BY s.created_at DESC
	`, user.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch subscriptions",
		})
	}
	defer rows.Close()

	type SubscriptionResponse struct {
		ID                     int64      `json:"id"`
		RazorpaySubscriptionID string     `json:"razorpay_subscription_id"`
		OwnerUserID            int        `json:"owner_user_id"`
		OrgID                  int        `json:"org_id"`
		PlanType               string     `json:"plan_type"`
		Quantity               int        `json:"quantity"`
		AssignedSeats          int        `json:"assigned_seats"`
		Status                 string     `json:"status"`
		RazorpayPlanID         string     `json:"razorpay_plan_id"`
		CurrentPeriodStart     *time.Time `json:"current_period_start,omitempty"` // Nullable timestamp, RFC3339 with timezone
		CurrentPeriodEnd       *time.Time `json:"current_period_end,omitempty"`   // Nullable timestamp, RFC3339 with timezone
		LicenseExpiresAt       *time.Time `json:"license_expires_at,omitempty"`   // Nullable timestamp, RFC3339 with timezone
		CreatedAt              time.Time  `json:"created_at"`                     // RFC3339 with timezone for precise sorting and display
		UpdatedAt              time.Time  `json:"updated_at"`                     // RFC3339 with timezone
	}

	var subscriptions []SubscriptionResponse
	for rows.Next() {
		var sub SubscriptionResponse
		if err := rows.Scan(
			&sub.ID, &sub.RazorpaySubscriptionID, &sub.OwnerUserID, &sub.OrgID, &sub.PlanType,
			&sub.Quantity, &sub.AssignedSeats, &sub.Status, &sub.RazorpayPlanID,
			&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.LicenseExpiresAt,
			&sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to parse subscriptions",
			})
		}
		subscriptions = append(subscriptions, sub)
	}

	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "error iterating subscriptions",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"subscriptions": subscriptions,
		"count":         len(subscriptions),
	})
}

// ListOrgSubscriptions lists all subscriptions for an organization (deprecated, kept for compatibility)
func (h *SubscriptionsHandler) ListOrgSubscriptions(c echo.Context) error {
	// Get org context
	orgIDVal := c.Get("org_id")
	var orgID int64
	switch v := orgIDVal.(type) {
	case int64:
		orgID = v
	case int:
		orgID = int64(v)
	default:
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required",
		})
	}

	if orgID == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required",
		})
	}

	// Query subscriptions for the org
	rows, err := h.db.Query(`
		SELECT 
			id, razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			current_period_start, current_period_end, license_expires_at,
			created_at, updated_at
		FROM subscriptions
		WHERE org_id = $1
		ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "failed to fetch subscriptions",
		})
	}
	defer rows.Close()

	type SubscriptionResponse struct {
		ID                     int64      `json:"id"`
		RazorpaySubscriptionID string     `json:"razorpay_subscription_id"`
		OwnerUserID            int        `json:"owner_user_id"`
		OrgID                  int        `json:"org_id"`
		PlanType               string     `json:"plan_type"`
		Quantity               int        `json:"quantity"`
		AssignedSeats          int        `json:"assigned_seats"`
		Status                 string     `json:"status"`
		RazorpayPlanID         string     `json:"razorpay_plan_id"`
		CurrentPeriodStart     *time.Time `json:"current_period_start,omitempty"` // Nullable timestamp, RFC3339 with timezone
		CurrentPeriodEnd       *time.Time `json:"current_period_end,omitempty"`   // Nullable timestamp, RFC3339 with timezone
		LicenseExpiresAt       *time.Time `json:"license_expires_at,omitempty"`   // Nullable timestamp, RFC3339 with timezone
		CreatedAt              time.Time  `json:"created_at"`                     // RFC3339 with timezone for precise sorting and display
		UpdatedAt              time.Time  `json:"updated_at"`                     // RFC3339 with timezone
	}

	var subscriptions []SubscriptionResponse
	for rows.Next() {
		var sub SubscriptionResponse
		if err := rows.Scan(
			&sub.ID, &sub.RazorpaySubscriptionID, &sub.OwnerUserID, &sub.OrgID, &sub.PlanType,
			&sub.Quantity, &sub.AssignedSeats, &sub.Status, &sub.RazorpayPlanID,
			&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &sub.LicenseExpiresAt,
			&sub.CreatedAt, &sub.UpdatedAt,
		); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to parse subscriptions",
			})
		}
		subscriptions = append(subscriptions, sub)
	}

	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "error iterating subscriptions",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"subscriptions": subscriptions,
		"count":         len(subscriptions),
	})
}
