package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	storagelicense "github.com/livereview/storage/license"
)

// ReviewEventsHandler handles review events API endpoints
type ReviewEventsHandler struct {
	service         *PollingEventService
	accountingStore *storagelicense.ReviewAccountingStore
}

// NewReviewEventsHandler creates a new review events handler
func NewReviewEventsHandler(db *sql.DB) *ReviewEventsHandler {
	return &ReviewEventsHandler{
		service:         NewPollingEventService(db),
		accountingStore: storagelicense.NewReviewAccountingStore(db),
	}
}

type ReviewAccountingOperationResponse struct {
	OperationType  string   `json:"operationType"`
	TriggerSource  string   `json:"triggerSource"`
	OperationID    string   `json:"operationId"`
	IdempotencyKey string   `json:"idempotencyKey"`
	BillableLOC    int64    `json:"billableLoc"`
	AccountedAt    string   `json:"accountedAt"`
	Provider       string   `json:"provider,omitempty"`
	Model          string   `json:"model,omitempty"`
	PricingVersion string   `json:"pricingVersion,omitempty"`
	InputTokens    *int64   `json:"inputTokens,omitempty"`
	OutputTokens   *int64   `json:"outputTokens,omitempty"`
	CostUSD        *float64 `json:"costUsd,omitempty"`
	Metadata       string   `json:"metadata,omitempty"`
}

type ReviewAccountingResponse struct {
	ReviewID            int64                              `json:"reviewId"`
	TotalBillableLOC    int64                              `json:"totalBillableLoc"`
	AccountedOperations int64                              `json:"accountedOperations"`
	TokenTrackedOps     int64                              `json:"tokenTrackedOperations"`
	LastAccountedAt     string                             `json:"lastAccountedAt,omitempty"`
	TotalInputTokens    *int64                             `json:"totalInputTokens,omitempty"`
	TotalOutputTokens   *int64                             `json:"totalOutputTokens,omitempty"`
	TotalCostUSD        *float64                           `json:"totalCostUsd,omitempty"`
	LatestOperation     *ReviewAccountingOperationResponse `json:"latestOperation,omitempty"`
}

// GetReviewEvents handles GET /api/v1/reviews/{id}/events (polling endpoint)
func (h *ReviewEventsHandler) GetReviewEvents(c echo.Context) error {
	// Extract review ID from path
	reviewIDStr := c.Param("id")
	reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid review ID")
	}

	// Extract org context from middleware
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	// Parse query parameters for cursor-based pagination
	var since *time.Time
	if sinceStr := c.QueryParam("since"); sinceStr != "" {
		if parsedTime, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			since = &parsedTime
		}
	}

	limit := 50 // default
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	// Get events
	events, err := h.service.GetRecentEvents(c.Request().Context(), reviewID, orgID, since, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to retrieve events")
	}

	var reviewStatus *string
	var statusValue string
	statusErr := h.service.repo.db.QueryRowContext(
		c.Request().Context(),
		`SELECT status FROM reviews WHERE id = $1 AND org_id = $2`,
		reviewID,
		orgID,
	).Scan(&statusValue)
	if statusErr == nil {
		reviewStatus = &statusValue
	}

	// Ensure events is a non-nil slice so JSON encodes to []
	if events == nil {
		events = make([]*ReviewEvent, 0)
	}

	// Return events in the standard envelope format
	response := map[string]interface{}{
		"events": events,
		"meta": map[string]interface{}{
			"reviewId": reviewID,
			"count":    len(events),
			"limit":    limit,
		},
	}

	if reviewStatus != nil {
		response["meta"].(map[string]interface{})["status"] = *reviewStatus
	}

	if since != nil {
		response["meta"].(map[string]interface{})["since"] = since.Format(time.RFC3339)
	}

	return c.JSON(http.StatusOK, response)
}

// GetReviewEventsByType handles GET /api/v1/reviews/{id}/events/{type}
func (h *ReviewEventsHandler) GetReviewEventsByType(c echo.Context) error {
	// Extract review ID from path
	reviewIDStr := c.Param("id")
	reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid review ID")
	}

	eventType := c.Param("type")
	if eventType == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "Event type is required")
	}

	// Extract org context from middleware
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	limit := 20 // default for filtered queries
	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	// Get events by type
	events, err := h.service.GetEventsByType(c.Request().Context(), reviewID, orgID, eventType, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to retrieve events")
	}

	if events == nil {
		events = make([]*ReviewEvent, 0)
	}

	response := map[string]interface{}{
		"events": events,
		"meta": map[string]interface{}{
			"reviewId":  reviewID,
			"eventType": eventType,
			"count":     len(events),
			"limit":     limit,
		},
	}

	return c.JSON(http.StatusOK, response)
}

// GetReviewSummary handles GET /api/v1/reviews/{id}/summary
func (h *ReviewEventsHandler) GetReviewSummary(c echo.Context) error {
	// Extract review ID from path
	reviewIDStr := c.Param("id")
	reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid review ID")
	}

	// Extract org context from middleware
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	// Get review summary
	summary, err := h.service.GetReviewSummary(c.Request().Context(), reviewID, orgID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to retrieve review summary")
	}

	return c.JSON(http.StatusOK, summary)
}

// GetReviewAccounting handles GET /api/v1/reviews/{id}/accounting
func (h *ReviewEventsHandler) GetReviewAccounting(c echo.Context) error {
	reviewIDStr := c.Param("id")
	reviewID, err := strconv.ParseInt(reviewIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid review ID")
	}

	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "Missing organization context")
	}

	totals, err := h.accountingStore.GetReviewAccountingTotals(c.Request().Context(), orgID, reviewID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to retrieve review accounting totals")
	}

	latestOperation, err := h.accountingStore.GetLatestReviewAccountingOperation(c.Request().Context(), orgID, reviewID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to retrieve latest review accounting operation")
	}

	response := ReviewAccountingResponse{
		ReviewID:            reviewID,
		TotalBillableLOC:    totals.TotalBillableLOC,
		AccountedOperations: totals.AccountedOperations,
		TokenTrackedOps:     totals.TokenTrackedOps,
		TotalInputTokens:    totals.TotalInputTokens,
		TotalOutputTokens:   totals.TotalOutputTokens,
		TotalCostUSD:        totals.TotalCostUSD,
	}

	if totals.LastAccountedAt != nil {
		response.LastAccountedAt = totals.LastAccountedAt.UTC().Format(time.RFC3339)
	}

	if latestOperation != nil {
		response.LatestOperation = &ReviewAccountingOperationResponse{
			OperationType:  latestOperation.OperationType,
			TriggerSource:  latestOperation.TriggerSource,
			OperationID:    latestOperation.OperationID,
			IdempotencyKey: latestOperation.IdempotencyKey,
			BillableLOC:    latestOperation.BillableLOC,
			AccountedAt:    latestOperation.AccountedAt.UTC().Format(time.RFC3339),
			Provider:       latestOperation.Provider,
			Model:          latestOperation.Model,
			PricingVersion: latestOperation.PricingVersion,
			InputTokens:    latestOperation.InputTokens,
			OutputTokens:   latestOperation.OutputTokens,
			CostUSD:        latestOperation.CostUSD,
			Metadata:       latestOperation.Metadata,
		}
	}

	return c.JSON(http.StatusOK, response)
}
