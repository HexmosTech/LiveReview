package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/logging"
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

	// Generate unique review ID for comprehensive logging
	reviewID := fmt.Sprintf("frontend-review-%d", time.Now().Unix())

	// Start comprehensive logging for frontend triggers
	logger, err := logging.StartReviewLogging(reviewID)
	if err != nil {
		log.Printf("[ERROR] Failed to start comprehensive logging: %v", err)
		// Continue without logging rather than fail the request
	}

	// DON'T close logger in defer - let the background goroutine manage it
	// The logger will be closed when the review processing completes

	if logger != nil {
		logger.LogSection("FRONTEND TRIGGER-REVIEW STARTED")
		logger.Log("Review ID: %s", reviewID)
		logger.Log("Request received at: %s", time.Now().Format("2006-01-02 15:04:05"))
		logger.Log("Remote Address: %s", c.RealIP())
		logger.Log("User Agent: %s", c.Request().UserAgent())
	}

	// Authenticate admin user
	if logger != nil {
		logger.LogSection("AUTHENTICATION")
		logger.Log("Authenticating admin user...")
	}
	if err := s.authenticateAdmin(c); err != nil {
		if logger != nil {
			logger.LogError("Authentication failed", err)
		}
		return err
	}
	if logger != nil {
		logger.Log("✓ Authentication successful")
	}

	// Parse request body
	if logger != nil {
		logger.LogSection("REQUEST PARSING")
		logger.Log("Parsing request body...")
	}
	req, err := parseTriggerReviewRequest(c)
	if err != nil {
		if logger != nil {
			logger.LogError("Failed to parse request", err)
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}
	if logger != nil {
		logger.Log("✓ Request parsed successfully")
		logger.Log("  MR/PR URL: %s", req.URL)
	}

	// Validate and parse URL
	if logger != nil {
		logger.LogSection("URL VALIDATION")
		logger.Log("Validating URL: %s", req.URL)
	}
	_, baseURL, err := validateAndParseURL(req.URL)
	if err != nil {
		if logger != nil {
			logger.LogError("Invalid URL", err)
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}
	if logger != nil {
		logger.Log("✓ URL validation passed")
		logger.Log("  Base URL: %s", baseURL)
	}

	// Find and retrieve integration token from database
	if logger != nil {
		logger.LogSection("INTEGRATION TOKEN")
		logger.Log("Finding integration token...")
	}
	token, err := s.findIntegrationToken(baseURL)
	if err != nil {
		if logger != nil {
			logger.LogError("Failed to find integration token", err)
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}
	if logger != nil {
		logger.Log("✓ Integration token found")
		logger.Log("  Provider: %s", token.Provider)
		logger.Log("  Token exists: %v", token.AccessToken != "")
	}

	// Validate that the provider is supported
	if logger != nil {
		logger.LogSection("PROVIDER VALIDATION")
		logger.Log("Validating provider: %s", token.Provider)
	}
	if err := validateProvider(token.Provider); err != nil {
		if logger != nil {
			logger.LogError("Unsupported provider", err)
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
		})
	}
	if logger != nil {
		logger.Log("✓ Provider validation passed")
	}

	// Check if token needs refresh and refresh if necessary
	if logger != nil {
		logger.LogSection("TOKEN REFRESH")
		logger.Log("Checking if token needs refresh...")
	}
	forceRefresh := false // Can be configured
	if err := s.refreshTokenIfNeeded(token, forceRefresh); err != nil {
		if logger != nil {
			logger.Log("⚠ Token refresh failed, continuing with existing token: %v", err)
		}
		log.Printf("[DEBUG] TriggerReviewV2: Token refresh failed, continuing with existing token: %v", err)
	} else {
		if logger != nil {
			logger.Log("✓ Token refresh check completed")
		}
	}

	// Ensure we have a valid token (with config file fallback if needed)
	if logger != nil {
		logger.LogSection("TOKEN VALIDATION")
		logger.Log("Ensuring valid token...")
	}
	accessToken := ensureValidToken(token)
	if logger != nil {
		logger.Log("✓ Valid token obtained")
		logger.Log("  Token length: %d characters", len(accessToken))
	}

	// Update final review ID (keeping the same one from start)
	finalReviewID := reviewID // Use the same ID from the start
	if logger != nil {
		logger.LogSection("REVIEW SERVICE CREATION")
		logger.Log("Final Review ID: %s", finalReviewID)
	}
	log.Printf("[DEBUG] TriggerReviewV2: Generated review ID: %s", finalReviewID)

	// Create review service instance for this specific request
	if logger != nil {
		logger.Log("Creating review service...")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Creating review service for request")
	reviewService, err := s.createReviewService(token)
	if err != nil {
		if logger != nil {
			logger.LogError("Failed to create review service", err)
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to create review service: " + err.Error(),
		})
	}
	if logger != nil {
		logger.Log("✓ Review service created successfully")
	}

	// Build review request
	if logger != nil {
		logger.LogSection("REVIEW REQUEST BUILDING")
		logger.Log("Building review request...")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Building review request")
	reviewRequest, err := s.buildReviewRequest(token, req.URL, finalReviewID, accessToken)
	if err != nil {
		if logger != nil {
			logger.LogError("Failed to build review request", err)
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Failed to build review request: " + err.Error(),
		})
	}
	if logger != nil {
		logger.Log("✓ Review request built successfully")
		logger.Log("  Request details: Provider=%s, URL=%s", token.Provider, req.URL)
	}

	// Track the review trigger activity
	go func() {
		repository := extractRepositoryFromURL(req.URL)
		branch := extractBranchFromURL(req.URL)
		commitHash := extractCommitFromURL(req.URL)
		TrackReviewTriggered(s.db, repository, branch, commitHash, "manual", token.Provider, &token.ID, "admin", req.URL)
	}()

	// Set up completion callback
	completionCallback := func(result *review.ReviewResult) {
		if logger != nil {
			logger.LogSection("REVIEW COMPLETION CALLBACK")
		}
		if result.Success {
			if logger != nil {
				logger.Log("✓ SUCCESS: Review %s completed: %s (%d comments, took %v)",
					result.ReviewID, truncateString(result.Summary, 50),
					result.CommentsCount, result.Duration)
			}
			log.Printf("[INFO] TriggerReviewV2: Review %s completed successfully: %s (%d comments, took %v)",
				result.ReviewID, truncateString(result.Summary, 50),
				result.CommentsCount, result.Duration)
		} else {
			if logger != nil {
				logger.LogError("Review failed", result.Error)
				logger.Log("✗ FAILURE: Review %s failed (took %v)",
					result.ReviewID, result.Duration)
			}
			log.Printf("[ERROR] TriggerReviewV2: Review %s failed: %v (took %v)",
				result.ReviewID, result.Error, result.Duration)
		}
	}

	// Process review asynchronously using a goroutine
	if logger != nil {
		logger.LogSection("BACKGROUND PROCESSING")
		logger.Log("Starting review process in background goroutine...")
		logger.Log("⚠ Note: Detailed review processing logs will continue in this file")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Starting review process in background")
	go func() {
		if logger != nil {
			logger.LogSection("GOROUTINE EXECUTION")
			logger.Log("=== Background processing started ===")
			logger.Log("Calling reviewService.ProcessReview...")
		}
		result := reviewService.ProcessReview(context.Background(), *reviewRequest)
		if logger != nil {
			logger.Log("ProcessReview returned, calling completion callback...")
		}
		completionCallback(result)
		if logger != nil {
			logger.Log("=== Background processing completed ===")
			// Close the logger now that all processing is done
			logger.Close()
		}
	}()

	// Return success response immediately
	if logger != nil {
		logger.LogSection("RESPONSE")
		logger.Log("Returning success response to frontend")
		logger.Log("  Review ID: %s", finalReviewID)
		logger.Log("  Response: 200 OK")
		logger.Log("=== Frontend request handling completed ===")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Returning success response with reviewID: %s", finalReviewID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully using comprehensive logging architecture. Check review_logs/ for detailed progress.",
		URL:      req.URL,
		ReviewID: finalReviewID,
	})
}
