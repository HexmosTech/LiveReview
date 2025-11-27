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

// reviewSetupContext holds the state for setting up a review
type reviewSetupContext struct {
	orgID         int64
	review        *Review
	reviewID      string
	logger        *logging.ReviewLogger
	token         *IntegrationToken
	accessToken   string
	reviewService *reviewpkg.Service
	request       *reviewpkg.ReviewRequest
	requestURL    string
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

	// Phase 1: Setup review context (org_id, parse request, create DB record, init logger)
	ctx, err := s.setupReviewContext(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Phase 2: Prepare authentication (URL validation, token lookup, OAuth refresh)
	if err := s.prepareAuthentication(ctx); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	// Phase 3: Create review request (build review service & request objects)
	if err := s.createReviewRequest(ctx); err != nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Phase 4: Enrich metadata (fetch MR details, update DB)
	s.enrichMetadata(ctx)

	// Phase 5: Track activity (log the trigger event)
	s.trackActivity(ctx)

	// Phase 6: Launch background processing (goroutine with completion callback)
	s.launchBackgroundProcessing(ctx)

	// Return success response immediately
	if ctx.logger != nil {
		ctx.logger.LogSection("RESPONSE")
		ctx.logger.Log("Returning success response to frontend")
		ctx.logger.Log("  Review ID: %s", ctx.reviewID)
		ctx.logger.Log("  Response: 200 OK")
		ctx.logger.Log("=== Frontend request handling completed ===")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Returning success response with reviewID: %s (DB ID: %d)", ctx.reviewID, ctx.review.ID)
	return c.JSON(http.StatusOK, TriggerReviewResponse{
		Message:  "Review triggered successfully using comprehensive logging architecture. Check review_logs/ for detailed progress.",
		URL:      ctx.requestURL,
		ReviewID: ctx.reviewID,
	})
}

func optionalString(value string) *string {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return &v
}

// setupReviewContext extracts org_id, parses request, creates DB record, initializes logger
func (s *Server) setupReviewContext(c echo.Context) (*reviewSetupContext, error) {
	ctx := &reviewSetupContext{}

	log.Printf("[DEBUG] FRONTEND TRIGGER-REVIEW STARTED")
	log.Printf("[DEBUG] Request received at: %s", time.Now().Format("2006-01-02 15:04:05"))
	log.Printf("[DEBUG] Remote Address: %s", c.RealIP())
	log.Printf("[DEBUG] User Agent: %s", c.Request().UserAgent())
	log.Printf("[DEBUG] AUTHENTICATION: JWT handled by middleware")

	// Get org_id from context (set by BuildOrgContextFromHeader middleware)
	orgID, ok := c.Get("org_id").(int64)
	if !ok {
		log.Printf("[ERROR] Organization context not found")
		return nil, fmt.Errorf("organization context required - missing X-Org-Context header")
	}
	ctx.orgID = orgID
	log.Printf("[DEBUG] ✓ Organization ID: %d", orgID)

	// Parse request body
	log.Printf("[DEBUG] REQUEST PARSING: Parsing request body...")
	req, err := parseTriggerReviewRequest(c)
	if err != nil {
		log.Printf("[ERROR] Failed to parse request: %v", err)
		return nil, fmt.Errorf("invalid request format: %w", err)
	}
	ctx.requestURL = req.URL
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
		return nil, fmt.Errorf("failed to create review record: %w", err)
	}
	ctx.review = review
	ctx.reviewID = fmt.Sprintf("%d", review.ID)
	log.Printf("[DEBUG] ✓ Database record created - Review ID: %d", review.ID)

	// Initialize logger with real DB ID and event sink for customer-visible events
	logger, err := logging.StartReviewLoggingWithIDs(ctx.reviewID, review.ID, orgID)
	if err != nil {
		log.Printf("[ERROR] Failed to start comprehensive logging: %v", err)
		// Continue without logger rather than fail the request
	}
	ctx.logger = logger

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

	return ctx, nil
}

// prepareAuthentication validates URL, looks up token, validates provider, refreshes OAuth token
func (s *Server) prepareAuthentication(ctx *reviewSetupContext) error {
	// Validate and parse URL
	if ctx.logger != nil {
		ctx.logger.LogSection("URL VALIDATION")
		ctx.logger.Log("Validating URL: %s", ctx.requestURL)
	}
	_, baseURL, err := validateAndParseURL(ctx.requestURL)
	if err != nil {
		if ctx.logger != nil {
			ctx.logger.LogError("Invalid URL", err)
		}
		return err
	}
	if ctx.logger != nil {
		ctx.logger.Log("✓ URL validation passed - Base URL: %s", baseURL)
	}

	// Find and retrieve integration token from database
	if ctx.logger != nil {
		ctx.logger.LogSection("INTEGRATION TOKEN")
		ctx.logger.Log("Finding integration token...")
	}
	token, err := s.findIntegrationToken(baseURL)
	if err != nil {
		if ctx.logger != nil {
			ctx.logger.LogError("Failed to find integration token", err)
		}
		return err
	}
	ctx.token = token
	if ctx.logger != nil {
		ctx.logger.Log("✓ Integration token found - Provider: %s", token.Provider)
	}

	// Validate that the provider is supported
	if ctx.logger != nil {
		ctx.logger.Log("Validating provider: %s", token.Provider)
	}
	if err := validateProvider(token.Provider); err != nil {
		if ctx.logger != nil {
			ctx.logger.LogError("Unsupported provider", err)
		}
		return err
	}
	if ctx.logger != nil {
		ctx.logger.Log("✓ Provider validation passed")
	}

	// Check if token needs refresh and refresh if necessary
	if ctx.logger != nil {
		ctx.logger.Log("Checking if token needs refresh...")
	}
	forceRefresh := false // Can be configured
	if err := s.refreshTokenIfNeeded(token, forceRefresh); err != nil {
		if ctx.logger != nil {
			ctx.logger.Log("⚠ Token refresh failed, continuing with existing token: %v", err)
		}
	} else {
		if ctx.logger != nil {
			ctx.logger.Log("✓ Token refresh check completed")
		}
	}

	// Ensure we have a valid token (with config file fallback if needed)
	ctx.accessToken = ensureValidToken(token)
	if ctx.logger != nil {
		ctx.logger.Log("✓ Valid token obtained (length: %d characters)", len(ctx.accessToken))
	}

	return nil
}

// createReviewRequest builds the review service and request objects
func (s *Server) createReviewRequest(ctx *reviewSetupContext) error {
	log.Printf("[DEBUG] TriggerReviewV2: Generated review ID: %s", ctx.reviewID)

	// Create review service instance for this specific request
	if ctx.logger != nil {
		ctx.logger.LogSection("REVIEW SERVICE CREATION")
		ctx.logger.Log("Creating review service...")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Creating review service for request")
	reviewService, err := s.createReviewService(ctx.token)
	if err != nil {
		if ctx.logger != nil {
			ctx.logger.LogError("Failed to create review service", err)
		}
		return fmt.Errorf("failed to create review service: %w", err)
	}
	ctx.reviewService = reviewService
	if ctx.logger != nil {
		ctx.logger.Log("✓ Review service created successfully")
	}

	// Build review request
	if ctx.logger != nil {
		ctx.logger.Log("Building review request...")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Building review request")
	reviewRequest, err := s.buildReviewRequest(ctx.token, ctx.requestURL, ctx.reviewID, ctx.accessToken, ctx.orgID)
	if err != nil {
		if ctx.logger != nil {
			ctx.logger.LogError("Failed to build review request", err)
		}
		return fmt.Errorf("failed to build review request: %w", err)
	}
	ctx.request = reviewRequest
	if ctx.logger != nil {
		ctx.logger.Log("✓ Review request built - Provider=%s, URL=%s", ctx.token.Provider, ctx.requestURL)
	}

	return nil
}

// enrichMetadata fetches MR details and updates DB with rich metadata
func (s *Server) enrichMetadata(ctx *reviewSetupContext) {
	providerUpdated := false
	if ctx.logger != nil {
		ctx.logger.LogSection("METADATA ENRICHMENT")
		ctx.logger.Log("Fetching merge request metadata for listing...")
	}

	metadataCtx, cancelMetadata := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelMetadata()

	providerFactory := reviewpkg.NewStandardProviderFactory()
	providerInstance, providerErr := providerFactory.CreateProvider(metadataCtx, ctx.request.Provider)
	if providerErr != nil {
		log.Printf("[WARN] TriggerReviewV2: Failed to create provider for metadata prefetch: %v", providerErr)
		if ctx.logger != nil {
			ctx.logger.Log("⚠ Unable to create provider for metadata prefetch: %v", providerErr)
		}
		return
	}

	details, err := providerInstance.GetMergeRequestDetails(metadataCtx, ctx.requestURL)
	if err != nil {
		log.Printf("[WARN] TriggerReviewV2: Failed to fetch MR details for metadata: %v", err)
		if ctx.logger != nil {
			ctx.logger.Log("⚠ Unable to fetch MR details for metadata enrichment: %v", err)
		}
		return
	}

	if details == nil {
		return
	}

	if ctx.logger != nil {
		ctx.logger.Log("✓ Merge request metadata fetched")
		ctx.logger.Log("  MR Title: %s", details.Title)
		if details.AuthorName != "" {
			ctx.logger.Log("  Author: %s", details.AuthorName)
		} else if details.Author != "" {
			ctx.logger.Log("  Author: %s", details.Author)
		}
	}

	repo := details.RepositoryURL
	if strings.TrimSpace(repo) == "" {
		repo = extractRepositoryFromURL(ctx.requestURL)
	}

	providerValue := details.ProviderType
	if providerValue == "" {
		providerValue = normalizeProviderValue(ctx.token.Provider)
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
	if err := rm.UpdateReviewMetadata(ctx.review.ID, update); err != nil {
		log.Printf("[WARN] TriggerReviewV2: Failed to update review metadata: %v", err)
		if ctx.logger != nil {
			ctx.logger.Log("⚠ Failed to enrich review metadata: %v", err)
		}
	}

	if !providerUpdated {
		if ptr := optionalString(normalizeProviderValue(ctx.token.Provider)); ptr != nil {
			if err := rm.UpdateReviewMetadata(ctx.review.ID, ReviewMetadataUpdate{Provider: ptr}); err != nil {
				log.Printf("[WARN] TriggerReviewV2: Failed to persist provider metadata: %v", err)
				if ctx.logger != nil {
					ctx.logger.Log("⚠ Failed to persist provider metadata: %v", err)
				}
			}
		}
	}
}

// trackActivity logs the review trigger event
func (s *Server) trackActivity(ctx *reviewSetupContext) {
	go func() {
		repository := extractRepositoryFromURL(ctx.requestURL)
		branch := extractBranchFromURL(ctx.requestURL)
		commitHash := extractCommitFromURL(ctx.requestURL)
		tracker := NewActivityTracker(s.db, ctx.orgID)
		eventData := map[string]interface{}{
			"repository":   repository,
			"branch":       branch,
			"commit_hash":  commitHash,
			"trigger_type": "manual",
			"provider":     ctx.token.Provider,
			"user_email":   "admin",
			"original_url": ctx.requestURL,
			"review_id":    ctx.review.ID,
		}
		if err := tracker.TrackActivityWithReview("review_triggered", eventData, &ctx.review.ID); err != nil {
			fmt.Printf("Failed to track review triggered: %v\n", err)
		}
	}()
}

// launchBackgroundProcessing starts the review goroutine with completion callback
func (s *Server) launchBackgroundProcessing(ctx *reviewSetupContext) {
	// Set up completion callback
	completionCallback := func(result interface{}) {
		if ctx.logger != nil {
			ctx.logger.LogSection("REVIEW COMPLETION CALLBACK")
			ctx.logger.Log("Review processing completed")
		}
		log.Printf("[INFO] TriggerReviewV2: Review processing completed for %s", ctx.reviewID)
	}

	// Process review asynchronously using a goroutine
	if ctx.logger != nil {
		ctx.logger.LogSection("BACKGROUND PROCESSING")
		ctx.logger.Log("Starting review process in background goroutine...")
		ctx.logger.Log("⚠ Note: Detailed review processing logs will continue in this file")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Starting review process in background")
	go func() {
		if ctx.logger != nil {
			ctx.logger.LogSection("GOROUTINE EXECUTION")
			ctx.logger.Log("=== Background processing started ===")
			ctx.logger.Log("Calling reviewService.ProcessReview...")
		}
		result := ctx.reviewService.ProcessReview(context.Background(), *ctx.request)
		if ctx.logger != nil {
			ctx.logger.Log("ProcessReview returned, calling completion callback...")
		}
		completionCallback(result)
		// Update review status based on result
		rm := NewReviewManager(s.db)
		if result != nil && result.Success {
			_ = rm.UpdateReviewStatus(ctx.review.ID, "completed")
		} else {
			_ = rm.UpdateReviewStatus(ctx.review.ID, "failed")
		}
		if ctx.logger != nil {
			ctx.logger.Log("=== Background processing completed ===")
			// Close the logger now that all processing is done
			ctx.logger.Close()
		}
	}()
}
