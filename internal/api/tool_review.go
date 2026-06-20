package api

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	storagetools "github.com/livereview/storage/tools"
)

// CreateToolReview handles POST /api/v1/reviews/tool-reviews
func (s *Server) CreateToolReview(c echo.Context) error {
	// Gated to cloud-mode only
	if !s.deploymentConfig.IsCloud {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Tool reviews endpoint is only available in cloud mode",
		})
	}

	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()
	userEmail := ""
	if pc.User != nil {
		userEmail = pc.User.Email
	}

	type RequestBody struct {
		PRURL string `json:"pr_url"`
	}

	var req RequestBody
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}

	if req.PRURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "pr_url is required"})
	}

	// Auto-detect connector from PR URL (same as AI review TriggerReviewV2)
	_, baseURL, err := validateAndParseURL(req.PRURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("invalid PR URL: %v", err)})
	}

	token, err := s.findIntegrationToken(baseURL, orgID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}

	provider := token.Provider
	connectorID := token.ID

	// Fetch enabled tools to calculate total multiplier
	toolsStore := storagetools.NewToolsStore(s.db)
	enabledTools, err := toolsStore.GetEnabledToolsForOrg(c.Request().Context(), orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to check tools configuration"})
	}

	if len(enabledTools) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "no tools are enabled for this organization"})
	}

	var totalMultiplier float64
	for _, t := range enabledTools {
		totalMultiplier += t.Multiplier
	}

	// Pre-flight credit check
	creditStore := storagetools.NewCreditStore(s.db)
	if err := creditStore.CheckCreditPreflight(c.Request().Context(), orgID, totalMultiplier); err != nil {
		return c.JSON(http.StatusPaymentRequired, map[string]string{"error": err.Error()})
	}

	// Create review row with trigger_type = 'tool_review'
	reviewManager := NewReviewManager(s.db)
	review, err := reviewManager.CreateReviewWithOrg(
		req.PRURL,     // repository
		"",            // branch
		"",            // commit_hash
		req.PRURL,     // pr_mr_url
		"tool_review", // trigger_type
		userEmail,
		provider,
		&connectorID,
		map[string]interface{}{
			"triggered_from": "tool_review",
			"multiplier_used": totalMultiplier,
		},
		orgID,
		"", "", "", // friendlyName, authorName, authorUsername
	)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to create review: %v", err)})
	}

	// Mark status as in progress immediately
	_ = reviewManager.UpdateReviewStatus(review.ID, "in_progress")

	// Queue the orchestrator job to River queue
	err = s.jobQueue.QueueToolReviewOrchestratorJob(
		c.Request().Context(),
		review.ID,
		orgID,
		req.PRURL,
		connectorID,
		provider,
		totalMultiplier,
	)
	if err != nil {
		_ = reviewManager.UpdateReviewStatus(review.ID, "failed")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to enqueue tool review job"})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"reviewId": review.ID,
		"message":  "Tool review triggered successfully",
	})
}

