package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	reviewpkg "github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

const defaultUpgradeURL = "/settings-subscriptions-overview"

// ReviewService encapsulates the review orchestration logic
type ReviewService struct {
	reviewService   *reviewpkg.Service
	configService   *reviewpkg.ConfigurationService
	resultCallbacks map[string]func(*reviewpkg.ReviewResult)
}

// reviewSetupContext holds the state for setting up a review
type reviewSetupContext struct {
	orgID       int64
	planCode    license.PlanType
	actorUserID *int64
	actorEmail  string
	review      *Review
	reviewID    string
	logger      *logging.ReviewLogger
	token       *IntegrationToken
	accessToken string
	request     *reviewpkg.ReviewRequest
	requestURL  string
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

	// LOC Quota preflight check — block before creating any DB records
	// Only run LOC quota preflight in Cloud Mode
	if apimiddleware.IsCloudMode() {
		orgID, orgOK := c.Get("org_id").(int64)
		planCode := license.PlanFree30K
		if planCtx, ok := c.Get(apimiddleware.PlanContextKey).(apimiddleware.PlanContext); ok && planCtx.PlanType != "" {
			planCode = planCtx.PlanType
		}
		if orgOK && orgID > 0 {
			accountingService := license.NewLOCAccountingService(s.db)
			preflightResult, pfErr := accountingService.CheckPreflight(context.Background(), license.LOCPreflightInput{
				OrgID:       orgID,
				RequiredLOC: 0, // unknown at this point, just check current state
				PlanCode:    planCode,
			})
			if pfErr != nil {
				log.Printf("[WARN] LOC preflight check failed for org=%d: %v", orgID, pfErr)
			} else {
				applyPreflightToEnvelopeContext(c, preflightResult)
				if preflightResult.Blocked {
					errorCode := "quota_exceeded"
					errorMessage := "monthly LOC quota exceeded for this organization"
					if preflightResult.BlockReason == "trial_readonly" {
						errorCode = "trial_readonly"
						errorMessage = "trial period ended; review operations are read-only until plan update"
					}
					log.Printf("[INFO] TriggerReviewV2: LOC quota blocked for org=%d, used=%d, limit=%d",
						orgID, preflightResult.LOCUsedMonth, preflightResult.LOCLimitMonth)
					return JSONWithEnvelope(c, http.StatusForbidden, map[string]interface{}{
						"error":         errorMessage,
						"error_code":    errorCode,
						"loc_remaining": preflightResult.LOCRemainingMonth,
						"usage_percent": preflightResult.UsagePercent,
						"upgrade_url":   defaultUpgradeURL,
					})
				}
			}
		}
	}

	// Phase 1: Setup review context (org_id, parse request, create DB record, init logger)
	var req TriggerReviewRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body: " + err.Error()})
	}
	ctx, err := s.setupReviewContext(c, req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
	}

	if apimiddleware.IsCloudMode() {
		quotaModule := license.NewQuotaModule(s.db)
		quotaPreflight, err := quotaModule.PreflightCheck(c.Request().Context(), license.QuotaPreflightInput{
			OrgID:       ctx.orgID,
			RequiredLOC: 1,
			PlanCode:    ctx.planCode,
		})
		if err != nil {
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed quota preflight: %v", err)})
		}
		if quotaPreflight.Blocked {
			errorCode := "quota_exceeded"
			errorMessage := "monthly LOC quota exceeded for this operation"
			if quotaPreflight.BlockReason == "trial_readonly" {
				errorCode = "trial_readonly"
				errorMessage = "trial period ended; review operations are read-only until plan update"
			}
			return JSONWithEnvelope(c, http.StatusForbidden, map[string]interface{}{
				"error":         errorMessage,
				"error_code":    errorCode,
				"required_loc":  1,
				"loc_remaining": quotaPreflight.LOCRemainingMonth,
				"usage_percent": quotaPreflight.UsagePercent,
				"upgrade_url":   defaultUpgradeURL,
			})
		}
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

	// Phase 6: Launch background processing via River job queue
	if err := s.launchBackgroundProcessing(ctx); err != nil {
		log.Printf("[ERROR] TriggerReviewV2: Failed to queue manual review: %v", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("failed to queue review: %v", err)})
	}

	// Return success response immediately
	if ctx.logger != nil {
		ctx.logger.LogSection("RESPONSE")
		ctx.logger.Log("Returning success response to frontend")
		ctx.logger.Log("  Review ID: %s", ctx.reviewID)
		ctx.logger.Log("  Response: 200 OK")
		ctx.logger.Log("=== Frontend request handling completed ===")
	}
	log.Printf("[DEBUG] TriggerReviewV2: Returning success response with reviewID: %s (DB ID: %d)", ctx.reviewID, ctx.review.ID)
	c.Set(EnvelopeOperationTypeContextKey, "manual_review")
	c.Set(EnvelopeTriggerSourceContextKey, "manual")
	operationID := fmt.Sprintf("manual-review:%d", ctx.review.ID)
	c.Set(EnvelopeOperationIDContextKey, operationID)
	c.Set(EnvelopeIdempotencyKeyContextKey, operationID)

	aiExecutionMode := ""
	aiExecutionSource := ""
	if ctx.request != nil {
		if mode, ok := ctx.request.AI.Config["ai_execution_mode"].(string); ok {
			aiExecutionMode = strings.TrimSpace(mode)
		}
		if source, ok := ctx.request.AI.Config["ai_execution_source"].(string); ok {
			aiExecutionSource = strings.TrimSpace(source)
		}
	}
	response := map[string]interface{}{
		"message":  "Review triggered successfully using comprehensive logging architecture. Check review_logs/ for detailed progress.",
		"url":      ctx.requestURL,
		"reviewId": ctx.reviewID,
	}
	if aiExecutionMode != "" {
		response["ai_execution_mode"] = aiExecutionMode
	}
	if aiExecutionSource != "" {
		response["ai_execution_source"] = aiExecutionSource
	}

	return JSONWithEnvelope(c, http.StatusOK, response)
}

func optionalString(value string) *string {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	return &v
}

// setupReviewContext extracts org_id, parses request, creates DB record, initializes logger
func (s *Server) setupReviewContext(c echo.Context, req TriggerReviewRequest) (*reviewSetupContext, error) {
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
	ctx.planCode = license.PlanFree30K
	if planCtx, ok := c.Get(apimiddleware.PlanContextKey).(apimiddleware.PlanContext); ok && planCtx.PlanType != "" {
		ctx.planCode = planCtx.PlanType
	}
	if user, ok := c.Get("user").(*models.User); ok && user != nil {
		userID := user.ID
		ctx.actorUserID = &userID
		ctx.actorEmail = strings.TrimSpace(user.Email)
	}
	log.Printf("[DEBUG] ✓ Organization ID: %d", orgID)

	// Use passed parsed request
	ctx.requestURL = req.URL
	log.Printf("[DEBUG] ✓ Request parsed successfully - MR/PR URL: %s", req.URL)

	// Create database record first to get proper numeric ID
	log.Printf("[DEBUG] DATABASE RECORD CREATION: Creating review record...")
	reviewManager := NewReviewManager(s.db)
	review, err := reviewManager.CreateReviewWithOrg(
		req.URL,  // repository (using URL as repository for now)
		"",       // branch (will be populated during processing)
		"",       // commit_hash (will be populated during processing)
		req.URL,  // pr_mr_url
		"manual", // trigger_type
		ctx.actorEmail,
		"unknown", // provider (will be determined during processing)
		nil,       // connector_id
		map[string]interface{}{
			"triggered_from": "frontend",
		},
		orgID,
		"", // friendlyName (only for CLI reviews)
		"", // authorName (only for CLI reviews)
		"", // authorUsername (only for CLI reviews)
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
	token, err := s.findIntegrationToken(baseURL, ctx.orgID)
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

// createReviewRequest builds the review request object for the River job payload.
func (s *Server) createReviewRequest(ctx *reviewSetupContext) error {
	log.Printf("[DEBUG] TriggerReviewV2: Building review request")

	reviewRequest, err := s.buildReviewRequest(ctx.token, ctx.requestURL, ctx.reviewID, ctx.accessToken, ctx.orgID, ctx.planCode)
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

// launchBackgroundProcessing enqueues the review request into the River job queue.
// The ManualReviewWorker picks it up, runs the AI review, and then queues a
// ToolReviewOrchestratorJob if any tools are enabled for the org.
func (s *Server) launchBackgroundProcessing(ctx *reviewSetupContext) error {
	if ctx.logger != nil {
		ctx.logger.LogSection("BACKGROUND QUEUEING")
		ctx.logger.Log("Enqueuing review into River job queue...")
	}

	requestJSONBytes, err := json.Marshal(ctx.request)
	if err != nil {
		return fmt.Errorf("marshal review request: %w", err)
	}

	err = s.jobQueue.QueueManualReviewJob(
		context.Background(),
		ctx.orgID,
		string(ctx.planCode),
		ctx.actorUserID,
		ctx.actorEmail,
		ctx.review.ID,
		string(requestJSONBytes),
	)
	if err != nil {
		return fmt.Errorf("queue manual review job: %w", err)
	}

	if ctx.logger != nil {
		ctx.logger.Log("✓ Successfully enqueued manual review job (River)")
		ctx.logger.Close()
	}
	return nil
}
