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

	// Track the review trigger activity and create review record
	go func() {
		repository := extractRepositoryFromURL(req.URL)
		branch := extractBranchFromURL(req.URL)
		commitHash := extractCommitFromURL(req.URL)
		reviewID, err := TrackReviewTriggered(s.db, repository, branch, commitHash, "manual", token.Provider, &token.ID, "admin", req.URL)
		if err != nil {
			fmt.Printf("Failed to track review triggered: %v\n", err)
		} else {
			fmt.Printf("Created review record with ID: %d\n", reviewID)
		}
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

			// Track AI comments in the database
			go func() {
				// Track summary comment
				summaryContent := map[string]interface{}{
					"content": result.Summary,
					"type":    "summary",
				}
				err := TrackAICommentFromURL(s.db, req.URL, "summary", summaryContent, nil, nil, orgID)
				if err != nil {
					fmt.Printf("Failed to track summary AI comment: %v\n", err)
				}

				// Track individual comments with actual details
				if len(result.Comments) > 0 {
					for i, comment := range result.Comments {
						commentContent := map[string]interface{}{
							"content":         comment.Content,
							"file_path":       comment.FilePath,
							"line":            comment.Line,
							"severity":        string(comment.Severity),
							"confidence":      comment.Confidence,
							"category":        comment.Category,
							"suggestions":     comment.Suggestions,
							"is_deleted_line": comment.IsDeletedLine,
							"is_internal":     comment.IsInternal,
							"review_id":       result.ReviewID,
							"comment_index":   i + 1,
						}

						// Use file path and line number for proper tracking
						linePtr := &comment.Line
						filePtr := &comment.FilePath
						err := TrackAICommentFromURL(s.db, req.URL, "line_comment", commentContent, filePtr, linePtr, orgID)
						if err != nil {
							fmt.Printf("Failed to track AI line comment %d: %v\n", i+1, err)
						}
					}
				}

				// Update review status to completed
				reviewManager := NewReviewManager(s.db)
				query := `SELECT id FROM reviews WHERE pr_mr_url = $1 ORDER BY created_at DESC LIMIT 1`
				var dbReviewID int64
				err = s.db.QueryRow(query, req.URL).Scan(&dbReviewID)
				if err == nil {
					reviewManager.UpdateReviewStatus(dbReviewID, "completed")
				}
			}()
		} else {
			if logger != nil {
				logger.LogError("Review failed", result.Error)
				logger.Log("✗ FAILURE: Review %s failed (took %v)",
					result.ReviewID, result.Duration)
			}
			log.Printf("[ERROR] TriggerReviewV2: Review %s failed: %v (took %v)",
				result.ReviewID, result.Error, result.Duration)

			// Update review status to failed
			go func() {
				reviewManager := NewReviewManager(s.db)
				query := `SELECT id FROM reviews WHERE pr_mr_url = $1 ORDER BY created_at DESC LIMIT 1`
				var dbReviewID int64
				err := s.db.QueryRow(query, req.URL).Scan(&dbReviewID)
				if err == nil {
					reviewManager.UpdateReviewStatus(dbReviewID, "failed")
				}
			}()
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
