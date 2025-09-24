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

	// Generate unique review ID for comprehensive logging (we'll update this with DB ID later)
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

	// JWT authentication is already handled by the RequireAuth() middleware
	// No additional authentication checks needed
	if logger != nil {
		logger.LogSection("AUTHENTICATION")
		logger.Log("✓ JWT authentication handled by middleware")
	}

	// Get org_id from context (set by BuildOrgContextFromHeader middleware)
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		if logger != nil {
			logger.LogError("Organization context not found", fmt.Errorf("no org_id in context"))
		}
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Organization context required - missing X-Org-Context header",
		})
	}

	if logger != nil {
		logger.Log("✓ Organization ID: %d", orgID)
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

	// Create database record first to get proper numeric ID
	if logger != nil {
		logger.LogSection("DATABASE RECORD CREATION")
		logger.Log("Creating review record in database...")
	}
	reviewManager := NewReviewManager(s.db)
	review, err := reviewManager.CreateReviewWithOrg(
		req.URL,   // repository (using URL as repository for now)
		"",        // branch (will be populated during processing)
		"",        // commit_hash (will be populated during processing)
		req.URL,   // pr_mr_url
		"manual",  // trigger_type
		"",        // user_email (will be populated from JWT if available)
		"unknown", // provider (will be determined during processing)
		nil,       // connector_id
		map[string]interface{}{
			"triggered_from": "frontend",
		},
		orgID,
	)
	if err != nil {
		if logger != nil {
			logger.LogError("Failed to create database record", err)
		}
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("Failed to create review record: %v", err),
		})
	}

	// Update review ID to use database ID for consistency and initialize event sink
	reviewID = fmt.Sprintf("%d", review.ID)
	if logger != nil {
		logger.Log("✓ Database record created")
		logger.Log("  Database ID: %d", review.ID)
		logger.Log("  Review ID: %s", reviewID)

		// Reinitialize logger with numeric IDs for event emission and attach DB event sink
		// Close the previous basic logger and start one with explicit IDs
		logger.Close()
		newLogger, err := logging.StartReviewLoggingWithIDs(reviewID, review.ID, orgID)
		if err == nil && newLogger != nil {
			eventSink := NewDatabaseEventSink(s.db)
			newLogger.SetEventSink(eventSink)
			logger = newLogger
			logger.Log("✓ Event sink attached; events will be persisted to review_events")
		} else {
			// Fall back gracefully without event sink
			logger = newLogger
		}
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

	// Track the review trigger activity (reference existing review ID)
	go func() {
		repository := extractRepositoryFromURL(req.URL)
		branch := extractBranchFromURL(req.URL)
		commitHash := extractCommitFromURL(req.URL)
		tracker := NewActivityTracker(s.db)
		eventData := map[string]interface{}{
			"repository":   repository,
			"branch":       branch,
			"commit_hash":  commitHash,
			"trigger_type": "manual",
			"provider":     token.Provider,
			"user_email":   "admin",
			"original_url": req.URL,
			"review_id":    review.ID,
		}
		if err := tracker.TrackActivityWithReview("review_triggered", eventData, &review.ID); err != nil {
			fmt.Printf("Failed to track review triggered: %v\n", err)
		}
	}()

	// Set up completion callback (simplified to avoid type issues for now)
	// TODO: Fix ReviewResult type resolution issue and restore full callback functionality
	completionCallback := func(result interface{}) {
		if logger != nil {
			logger.LogSection("REVIEW COMPLETION CALLBACK")
			logger.Log("Review processing completed")
		}
		log.Printf("[INFO] TriggerReviewV2: Review processing completed for %s", reviewID)
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
	log.Printf("[DEBUG] TriggerReviewV2: Returning success response with reviewID: %s (DB ID: %d)", finalReviewID, review.ID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully using comprehensive logging architecture. Check review_logs/ for detailed progress.",
		URL:      req.URL,
		ReviewID: fmt.Sprintf("%d", review.ID), // Return the database ID as string for frontend compatibility
	})
}
