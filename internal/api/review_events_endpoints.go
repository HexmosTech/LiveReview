package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

// ReviewEventsHandler handles review events API endpoints
type ReviewEventsHandler struct {
	service *PollingEventService
}

// NewReviewEventsHandler creates a new review events handler
func NewReviewEventsHandler(db *sql.DB) *ReviewEventsHandler {
	return &ReviewEventsHandler{
		service: NewPollingEventService(db),
	}
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
