package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/logging"
	reviewpkg "github.com/livereview/internal/review"
)

// ReviewService encapsulates the review orchestration logic
type ReviewService struct {
	reviewService   *reviewpkg.Service
	configService   *reviewpkg.ConfigurationService
	resultCallbacks map[string]func(*reviewpkg.ReviewResult)
}

// NewReviewService creates a new review service
func NewReviewService(cfg *config.Config) *ReviewService {
	// Create factories
	providerFactory := reviewpkg.NewStandardProviderFactory()
	aiProviderFactory := reviewpkg.NewStandardAIProviderFactory()

	// Create configuration service
	configService := reviewpkg.NewConfigurationService(cfg)

	// Create review service with default config
	reviewConfig := reviewpkg.DefaultReviewConfig()
	reviewSvc := reviewpkg.NewService(providerFactory, aiProviderFactory, reviewConfig)

	return &ReviewService{
		reviewService:   reviewSvc,
		configService:   configService,
		resultCallbacks: make(map[string]func(*reviewpkg.ReviewResult)),
	}
}

// TriggerReviewV2 handles the request to trigger a code review using the new decoupled architecture
func (s *Server) TriggerReviewV2(c echo.Context) error {
	log.Printf("[DEBUG] TriggerReviewV2: Starting review request handling")

	// Early validation logging - just use standard log.Printf, no special logger yet
	log.Printf("[DEBUG] FRONTEND TRIGGER-REVIEW STARTED")
	log.Printf("[DEBUG] Request received at: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("[DEBUG] Remote Address: %s", c.RealIP())
	log.Printf("[DEBUG] User Agent: %s", c.Request().UserAgent())

	// JWT authentication is already handled by the RequireAuth() middleware
	log.Printf("[DEBUG] AUTHENTICATION: JWT handled by middleware")

	// Get org_id from context (set by BuildOrgContextFromHeader middleware)
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		log.Printf("[ERROR] Organization context not found")
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Organization context required - missing X-Org-Context header",
		})
	}
	log.Printf("[DEBUG] ✓ Organization ID: %d", orgID)

	// Parse request body
	log.Printf("[DEBUG] REQUEST PARSING: Parsing request body...")
	req, err := parseTriggerReviewRequest(c)
	if err != nil {
		log.Printf("[ERROR] Failed to parse request: %v", err)
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "Invalid request format: " + err.Error(),
		})
	}
	log.Printf("[DEBUG] ✓ Request parsed successfully - MR/PR URL: %s", req.URL)

	// Create database record first to get proper numeric ID
	log.Printf("[DEBUG] DATABASE RECORD CREATION: Creating review record...")
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
		log.Printf("[ERROR] Failed to create database record: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: fmt.Sprintf("Failed to create review record: %v", err),
		})
	}

	// NOW initialize logger with real DB ID and event sink for customer-visible events
	reviewID := fmt.Sprintf("%d", review.ID)
	log.Printf("[DEBUG] ✓ Database record created - Review ID: %d", review.ID)

	logger, err := logging.StartReviewLoggingWithIDs(reviewID, review.ID, orgID)
	if err != nil {
		log.Printf("[ERROR] Failed to start comprehensive logging: %v", err)
		// Continue without logger rather than fail the request
	}

	// DON'T close logger in defer - let the background goroutine manage it

	if logger != nil {
		// Attach event sink so logs go to review_events table for UI
		eventSink := NewDatabaseEventSink(s.db)
		logger.SetEventSink(eventSink)
		logger.LogSection("REVIEW PROCESSING STARTED")
		logger.Log("Review ID: %d", review.ID)
		logger.Log("Organization ID: %d", orgID)
		logger.Log("MR/PR URL: %s", req.URL)
	}

	// Immediately mark review as in progress (sets started_at)
	go func() {
		rm := NewReviewManager(s.db)
		_ = rm.UpdateReviewStatus(review.ID, "in_progress")
	}()

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
		logger.Log("✓ URL validation passed - Base URL: %s", baseURL)
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
		logger.Log("✓ Integration token found - Provider: %s", token.Provider)
	}

	// Validate that the provider is supported
	if logger != nil {
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
		logger.Log("Checking if token needs refresh...")
	}
	forceRefresh := false // Can be configured
	if err := s.refreshTokenIfNeeded(token, forceRefresh); err != nil {
		if logger != nil {
			logger.Log("⚠ Token refresh failed, continuing with existing token: %v", err)
		}
	} else {
		if logger != nil {
			logger.Log("✓ Token refresh check completed")
		}
	}

	// Ensure we have a valid token (with config file fallback if needed)
	accessToken := ensureValidToken(token)
	if logger != nil {
		logger.Log("✓ Valid token obtained (length: %d characters)", len(accessToken))
	}

	// Update final review ID (keeping the same one from start)
	finalReviewID := reviewID
	log.Printf("[DEBUG] TriggerReviewV2: Generated review ID: %s", finalReviewID)

	// Create review service instance for this specific request
	if logger != nil {
		logger.LogSection("REVIEW SERVICE CREATION")
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
		logger.Log("✓ Review request built - Provider=%s, URL=%s", token.Provider, req.URL)
	}

	// Prefetch merge request metadata so the review list has rich fields immediately
	providerUpdated := false
	if logger != nil {
		logger.LogSection("METADATA ENRICHMENT")
		logger.Log("Fetching merge request metadata for listing...")
	}

	metadataCtx, cancelMetadata := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelMetadata()

	providerFactory := reviewpkg.NewStandardProviderFactory()
	providerInstance, providerErr := providerFactory.CreateProvider(metadataCtx, reviewRequest.Provider)
	if providerErr != nil {
		log.Printf("[WARN] TriggerReviewV2: Failed to create provider for metadata prefetch: %v", providerErr)
		if logger != nil {
			logger.Log("⚠ Unable to create provider for metadata prefetch: %v", providerErr)
		}
	} else {
		if details, err := providerInstance.GetMergeRequestDetails(metadataCtx, req.URL); err != nil {
			log.Printf("[WARN] TriggerReviewV2: Failed to fetch MR details for metadata: %v", err)
			if logger != nil {
				logger.Log("⚠ Unable to fetch MR details for metadata enrichment: %v", err)
			}
		} else if details != nil {
			if logger != nil {
				logger.Log("✓ Merge request metadata fetched")
				logger.Log("  MR Title: %s", details.Title)
				if details.AuthorName != "" {
					logger.Log("  Author: %s", details.AuthorName)
				} else if details.Author != "" {
					logger.Log("  Author: %s", details.Author)
				}
			}

			repo := details.RepositoryURL
			if strings.TrimSpace(repo) == "" {
				repo = extractRepositoryFromURL(req.URL)
			}

			providerValue := details.ProviderType
			if providerValue == "" {
				providerValue = normalizeProviderValue(token.Provider)
			}

			update := ReviewMetadataUpdate{}
			if ptr := optionalString(repo); ptr != nil {
				update.Repository = ptr
			}
			if ptr := optionalString(details.SourceBranch); ptr != nil {
				update.Branch = ptr
			}
			if ptr := optionalString(providerValue); ptr != nil {
				update.Provider = ptr
				providerUpdated = true
			}
			if ptr := optionalString(details.Title); ptr != nil {
				update.MRTitle = ptr
			}
			authorName := details.AuthorName
			if strings.TrimSpace(authorName) == "" {
				authorName = details.Author
			}
			if ptr := optionalString(authorName); ptr != nil {
				update.AuthorName = ptr
			}
			authorUsername := details.AuthorUsername
			if strings.TrimSpace(authorUsername) == "" {
				authorUsername = details.Author
			}
			if ptr := optionalString(authorUsername); ptr != nil {
				update.AuthorUsername = ptr
			}

			rm := NewReviewManager(s.db)
			if err := rm.UpdateReviewMetadata(review.ID, update); err != nil {
				log.Printf("[WARN] TriggerReviewV2: Failed to update review metadata: %v", err)
				if logger != nil {
					logger.Log("⚠ Failed to enrich review metadata: %v", err)
				}
			}
		}
	}

	if !providerUpdated {
		if ptr := optionalString(normalizeProviderValue(token.Provider)); ptr != nil {
			rm := NewReviewManager(s.db)
			if err := rm.UpdateReviewMetadata(review.ID, ReviewMetadataUpdate{Provider: ptr}); err != nil {
				log.Printf("[WARN] TriggerReviewV2: Failed to persist provider metadata: %v", err)
				if logger != nil {
					logger.Log("⚠ Failed to persist provider metadata: %v", err)
				}
			}
		}
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
		// Update review status based on result
		rm := NewReviewManager(s.db)
		if result != nil && result.Success {
			_ = rm.UpdateReviewStatus(review.ID, "completed")
		} else {
			_ = rm.UpdateReviewStatus(review.ID, "failed")
		}
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

func optionalString(value string) *string {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return &v
}
