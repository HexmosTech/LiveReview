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

	// Create or get review service
	var reviewService *ReviewService
	if s.reviewService == nil {
		cfg, err := config.LoadConfig("")
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error: "Failed to load configuration: " + err.Error(),
			})
		}
		reviewService = NewReviewService(cfg)
		s.reviewService = reviewService
	} else {
		reviewService = s.reviewService
	}

	// Build review request using configuration service
	reviewRequest, err := reviewService.configService.BuildReviewRequest(
		context.Background(),
		req.URL,
		reviewID,
		token.Provider,
		token.ProviderURL,
		accessToken,
	)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Failed to build review request: " + err.Error(),
		})
	}

	// Set up a callback to handle review completion
	reviewService.resultCallbacks[reviewID] = func(result *review.ReviewResult) {
		if result.Success {
			log.Printf("[INFO] Review %s completed successfully: %s (%d comments, took %v)",
				result.ReviewID, result.Summary[:min(50, len(result.Summary))],
				result.CommentsCount, result.Duration)
		} else {
			log.Printf("[ERROR] Review %s failed: %v (took %v)",
				result.ReviewID, result.Error, result.Duration)
		}

		// Clean up the callback
		delete(reviewService.resultCallbacks, result.ReviewID)
	}

	// Trigger the review process asynchronously
	log.Printf("[DEBUG] TriggerReviewV2: Starting review process in background")
	reviewService.reviewService.ProcessReviewAsync(
		context.Background(),
		*reviewRequest,
		reviewService.resultCallbacks[reviewID],
	)

	// Return success response immediately
	log.Printf("[DEBUG] TriggerReviewV2: Returning success response with reviewID: %s", reviewID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully using new decoupled architecture. You will receive a notification when it's complete.",
		URL:      req.URL,
		ReviewID: reviewID,
	})
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
