package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/license/payment"
)

// SubscriptionsHandler handles subscription-related API endpoints
type SubscriptionsHandler struct {
	service *payment.SubscriptionService
}

// NewSubscriptionsHandler creates a new subscriptions handler
func NewSubscriptionsHandler(db *sql.DB) *SubscriptionsHandler {
	return &SubscriptionsHandler{
		service: payment.NewSubscriptionService(db),
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
	orgID, ok := c.Get("org_id").(int)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required",
		})
	}

	// Get user ID from auth context
	userID, ok := c.Get("user_id").(int)
	if !ok {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "authentication required",
		})
	}

	// Parse request
	var req CreateSubscriptionRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	// Validate plan type
	if req.PlanType != "monthly" && req.PlanType != "yearly" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "plan_type must be 'monthly' or 'yearly'",
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
	sub, err := h.service.CreateTeamSubscription(userID, orgID, req.PlanType, req.Quantity, mode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusCreated, sub)
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

	// Determine mode
	mode := "test" // TODO: Get from config or environment

	// Get subscription details
	details, err := h.service.GetSubscriptionDetails(subscriptionID, mode)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, details)
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

	// Get org context
	orgID, ok := c.Get("org_id").(int)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required",
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
	if err := h.service.AssignLicense(subscriptionID, req.UserID, orgID); err != nil {
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

	// Get org context
	orgID, ok := c.Get("org_id").(int)
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "organization context required",
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
	if err := h.service.RevokeLicense(subscriptionID, userID, orgID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "license revoked successfully",
	})
}
