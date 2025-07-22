package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/review"
)

// ReviewService encapsulates the review orchestration logic
type ReviewService struct {
	reviewService   *review.Service
	configService   *review.ConfigurationService
	resultCallbacks map[string]func(*review.ReviewResult)
}

// NewReviewService creates a new review service
func NewReviewService(cfg *config.Config) *ReviewService {
	// Create factories
	providerFactory := review.NewStandardProviderFactory()
	aiProviderFactory := review.NewStandardAIProviderFactory()

	// Create configuration service
	configService := review.NewConfigurationService(cfg)

	// Create review service with default config
	reviewConfig := review.DefaultReviewConfig()
	reviewSvc := review.NewService(providerFactory, aiProviderFactory, reviewConfig)

	return &ReviewService{
		reviewService:   reviewSvc,
		configService:   configService,
		resultCallbacks: make(map[string]func(*review.ReviewResult)),
	}
}

// TriggerReviewV2 handles the request to trigger a code review using the new decoupled architecture
func (s *Server) TriggerReviewV2(c echo.Context) error {
	log.Printf("[DEBUG] TriggerReviewV2: Starting review request handling")

	// Authenticate admin user
	if err := s.authenticateAdmin(c); err != nil {
		return err
	}

	// Parse request body
	req, err := parseTriggerReviewRequest(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}

	_, baseURL, err := validateAndParseURL(req.URL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Find and retrieve integration token from database
	token, err := s.findIntegrationToken(baseURL)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Validate that the provider is supported
	if err := validateProvider(token.Provider); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}

	// Check if token needs refresh and refresh if necessary
	forceRefresh := false // Can be configured
	if err := s.refreshTokenIfNeeded(token, forceRefresh); err != nil {
		log.Printf("[DEBUG] TriggerReviewV2: Token refresh failed, continuing with existing token: %v", err)
	}

	// Ensure we have a valid token (with config file fallback if needed)
	accessToken := ensureValidToken(token)

	// Create a reviewID
	reviewID := fmt.Sprintf("review-%d", time.Now().Unix())
	log.Printf("[DEBUG] TriggerReviewV2: Generated review ID: %s", reviewID)

	// Create review service instance for this specific request
	log.Printf("[DEBUG] TriggerReviewV2: Creating review service for request")
	reviewService, err := s.createReviewService(token)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to create review service: " + err.Error(),
		})
	}

	// Build review request
	log.Printf("[DEBUG] TriggerReviewV2: Building review request")
	reviewRequest, err := s.buildReviewRequest(token, req.URL, reviewID, accessToken)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Failed to build review request: " + err.Error(),
		})
	}

	// Set up completion callback
	completionCallback := func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] TriggerReviewV2: Review %s completed successfully: %s (%d comments, took %v)",
				result.ReviewID, truncateString(result.Summary, 50),
				result.CommentsCount, result.Duration)
		} else {
			log.Printf("[ERROR] TriggerReviewV2: Review %s failed: %v (took %v)",
				result.ReviewID, result.Error, result.Duration)
		}
	}

	// Process review asynchronously using a goroutine
	log.Printf("[DEBUG] TriggerReviewV2: Starting review process in background")
	go func() {
		result := reviewService.ProcessReview(context.Background(), *reviewRequest)
		completionCallback(result)
	}()

	// Return success response immediately
	log.Printf("[DEBUG] TriggerReviewV2: Returning success response with reviewID: %s", reviewID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully using new per-request decoupled architecture. You will receive a notification when it's complete.",
		URL:      req.URL,
		ReviewID: reviewID,
	})
}
