package review

import (
	"context"
	"fmt"
	"log"
	"time"

	neturl "net/url"
	"strings"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

// Helper function for minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Service represents the review orchestration service
type Service struct {
	providers   ProviderFactory
	aiProviders AIProviderFactory
	config      Config
	logger      *logging.ReviewLogger // Logger for this review instance
}

// Config holds the review service configuration
type Config struct {
	ReviewTimeout time.Duration
	DefaultAI     string
	DefaultModel  string
	Temperature   float64
}

// ReviewRequest contains all the information needed to perform a review
type ReviewRequest struct {
	URL      string
	ReviewID string
	Provider ProviderConfig
	AI       AIConfig
}

// ProviderConfig contains provider-specific configuration
type ProviderConfig struct {
	Type   string
	URL    string
	Token  string
	Config map[string]interface{}
}

// AIConfig contains AI provider-specific configuration
type AIConfig struct {
	Type        string
	APIKey      string
	Model       string
	Temperature float64
	Config      map[string]interface{}
}

// ReviewResult contains the results of a review process
type ReviewResult struct {
	ReviewID      string
	Success       bool
	Error         error
	Summary       string
	CommentsCount int
	Comments      []*models.ReviewComment // Added to track actual comment details
	Duration      time.Duration
}

// ReviewWorkflowResult contains the full workflow result including MR details
type ReviewWorkflowResult struct {
	MRDetails *providers.MergeRequestDetails
	Result    *models.ReviewResult
}

// ProviderFactory creates provider instances
type ProviderFactory interface {
	CreateProvider(ctx context.Context, config ProviderConfig) (providers.Provider, error)
	SupportsProvider(providerType string) bool
}

// AIProviderFactory creates AI provider instances
type AIProviderFactory interface {
	CreateAIProvider(ctx context.Context, config AIConfig, logger *logging.ReviewLogger) (ai.Provider, error)
	SupportsAIProvider(aiType string) bool
}

// NewService creates a new review service
func NewService(providers ProviderFactory, aiProviders AIProviderFactory, config Config) *Service {
	return &Service{
		providers:   providers,
		aiProviders: aiProviders,
		config:      config,
		logger:      nil, // Will be set when ProcessReview is called
	}
}

// ProcessReview orchestrates the entire review process
func (s *Service) ProcessReview(ctx context.Context, request ReviewRequest) *ReviewResult {
	start := time.Now()
	result := &ReviewResult{
		ReviewID: request.ReviewID,
	}

	// Get the logger for this review (should be set by caller)
	s.logger = logging.GetLoggerByReviewID(request.ReviewID)
	if s.logger != nil {
		s.logger.LogSection("REVIEW PROCESS ORCHESTRATION")
		s.logger.EmitStageStarted("Preparation")
		s.logger.Log("Starting review process for URL: %s", request.URL)
		s.logger.Log("Review ID: %s", request.ReviewID)
		s.logger.Log("Provider: %s", request.Provider.Type)

		// Log the actual AI connector being used, not just "langchain"
		aiConnectorName := "unknown"
		if providerName, ok := request.AI.Config["provider_name"].(string); ok && providerName != "" {
			aiConnectorName = providerName
		} else {
			aiConnectorName = request.AI.Type
		}
		s.logger.Log("AI Provider: %s (model: %s)", aiConnectorName, request.AI.Model)
		s.logger.Log("Start time: %s", start.Format("2006-01-02 15:04:05.000"))
	}

	log.Printf("[INFO] Starting review process for %s (ReviewID: %s)", request.URL, request.ReviewID)

	// Create timeout context
	reviewCtx, cancel := context.WithTimeout(ctx, s.config.ReviewTimeout)
	defer cancel()

	// Step 1: Create provider
	if s.logger != nil {
		s.logger.LogSection("PROVIDER CREATION")
		s.logger.Log("Creating %s provider...", request.Provider.Type)
	}
	provider, err := s.providers.CreateProvider(reviewCtx, request.Provider)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("Provider creation failed", err)
			s.logger.EmitStageError("Preparation", err)
		}
		result.Error = fmt.Errorf("failed to create provider: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	if s.logger != nil {
		s.logger.Log("✓ %s provider created successfully", request.Provider.Type)
	}

	// Step 2: Create AI provider
	if s.logger != nil {
		s.logger.LogSection("AI PROVIDER CREATION")
		s.logger.Log("Creating %s AI provider...", request.AI.Type)
	}
	aiProvider, err := s.aiProviders.CreateAIProvider(reviewCtx, request.AI, s.logger)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("AI provider creation failed", err)
			s.logger.EmitStageError("Preparation", err)
		}
		result.Error = fmt.Errorf("failed to create AI provider: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	if s.logger != nil {
		s.logger.Log("✓ %s AI provider created successfully", request.AI.Type)
		s.logger.EmitStageCompleted("Preparation", "Providers initialized and configured")
	}

	// Step 3: Execute review workflow
	if s.logger != nil {
		s.logger.LogSection("REVIEW WORKFLOW EXECUTION")
		s.logger.EmitStageStarted("Analysis")
		s.logger.Log("Executing review workflow...")
	}
	reviewData, err := s.executeReviewWorkflow(reviewCtx, provider, aiProvider, request.URL)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("Review workflow execution failed", err)
			s.logger.EmitStageError("Analysis", err)
		}
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}
	if s.logger != nil {
		s.logger.Log("✓ Review workflow executed successfully")
		s.logger.Log("  Generated %d comments", len(reviewData.Result.Comments))
		s.logger.Log("  Summary length: %d characters", len(reviewData.Result.Summary))
	}

	// Step 4: Post results (Artifact Generation stage)
	if s.logger != nil {
		s.logger.LogSection("RESULTS POSTING")
		s.logger.EmitStageStarted("Artifact Generation")
		s.logger.Log("Posting review results...")
		s.logger.Log("  MR ID: %s", reviewData.MRDetails.ID)
		s.logger.Log("  MR Title: %s", reviewData.MRDetails.Title)
		s.logger.Log("  Provider type: %s", reviewData.MRDetails.ProviderType)
	}

	// For GitHub, we need to convert the MR ID to owner/repo/number format for posting comments
	postingID := reviewData.MRDetails.ID
	if reviewData.MRDetails.ProviderType == "github" {
		if s.logger != nil {
			s.logger.Log("Converting GitHub MR ID for comment posting...")
		}
		u, err := neturl.Parse(reviewData.MRDetails.URL)
		if err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull" {
				owner := parts[1]
				repo := parts[2]
				number := parts[4]
				postingID = owner + "/" + repo + "/" + number
				if s.logger != nil {
					s.logger.Log("✓ GitHub posting ID: %s", postingID)
				}
				log.Printf("[DEBUG] GitHub: Using posting ID '%s' for comments", postingID)
			}
		}
	} else if reviewData.MRDetails.ProviderType == "bitbucket" {
		if s.logger != nil {
			s.logger.Log("Converting Bitbucket MR ID for comment posting...")
		}
		u, err := neturl.Parse(reviewData.MRDetails.URL)
		if err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull-requests" {
				workspace := parts[1]
				repo := parts[2]
				number := parts[4]
				postingID = workspace + "/" + repo + "/" + number
				if s.logger != nil {
					s.logger.Log("✓ Bitbucket posting ID: %s", postingID)
				}
				log.Printf("[DEBUG] Bitbucket: Using posting ID '%s' for comments", postingID)
			}
		}
	}
	err = s.postReviewResults(reviewCtx, provider, postingID, reviewData.Result)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("Failed to post results", err)
			s.logger.EmitStageError("Artifact Generation", err)
		}
		result.Error = fmt.Errorf("failed to post results: %w", err)
		result.Duration = time.Since(start)
		return result
	}
	if s.logger != nil {
		s.logger.Log("✓ Results posted successfully")
		s.logger.EmitStageCompleted("Artifact Generation", fmt.Sprintf("Posted %d comments to merge request", len(reviewData.Result.Comments)))
	}

	// Step 5: Finalization
	if s.logger != nil {
		s.logger.LogSection("REVIEW FINALIZATION")
		s.logger.EmitStageStarted("Finalization")
		s.logger.Log("Finalizing review process...")
	}

	// Success
	result.Success = true
	result.Summary = reviewData.Result.Summary
	result.CommentsCount = len(reviewData.Result.Comments)
	result.Comments = reviewData.Result.Comments // Include actual comment details
	result.Duration = time.Since(start)

	if s.logger != nil {
		s.logger.Log("✓ Review finalization complete")
		s.logger.Log("  Total duration: %v", result.Duration)
		s.logger.Log("  Comments posted: %d", result.CommentsCount)
		s.logger.Log("  Summary: %s", result.Summary[:minInt(100, len(result.Summary))]+"...")
		s.logger.EmitStageCompleted("Finalization", "Review process completed successfully")
		s.logger.EmitReviewCompletion(result.CommentsCount, fmt.Sprintf("Review finalized successfully • Posted %d comments", result.CommentsCount))
	}

	log.Printf("[INFO] Review completed successfully for %s (ReviewID: %s) in %v",
		request.URL, request.ReviewID, result.Duration)

	return result
}

// executeReviewWorkflow handles the core review logic
func (s *Service) executeReviewWorkflow(
	ctx context.Context,
	provider providers.Provider,
	aiProvider ai.Provider,
	url string,
) (*ReviewWorkflowResult, error) {

	// Get MR details
	if s.logger != nil {
		s.logger.LogSection("MERGE REQUEST DETAILS")
		s.logger.Log("Fetching merge request details for URL: %s", url)
	}
	log.Printf("[DEBUG] Fetching merge request details for URL: %s", url)
	mrDetails, err := provider.GetMergeRequestDetails(ctx, url)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("Failed to get merge request details", err)
		}
		return nil, fmt.Errorf("failed to get merge request details: %w", err)
	}
	if s.logger != nil {
		s.logger.Log("✓ MR details retrieved successfully")
		s.logger.Log("  ID: %s", mrDetails.ID)
		s.logger.Log("  Title: %s", mrDetails.Title)
		s.logger.Log("  Provider: %s", mrDetails.ProviderType)
		s.logger.Log("  URL: %s", mrDetails.URL)
	}
	log.Printf("[DEBUG] Retrieved MR details successfully. ID: %s, Title: %s", mrDetails.ID, mrDetails.Title)

	// Get MR changes
	if s.logger != nil {
		s.logger.LogSection("CODE CHANGES RETRIEVAL")
		s.logger.Log("Fetching merge request changes for MR ID: %s", mrDetails.ID)
	}
	log.Printf("[DEBUG] Fetching merge request changes for MR ID: %s", mrDetails.ID)
	// For GitHub, pass owner/repo/number as PR ID
	prID := mrDetails.ID
	if mrDetails.ProviderType == "github" {
		if s.logger != nil {
			s.logger.Log("Converting GitHub URL for changes API...")
		}
		// Robustly parse owner, repo, number from MR URL
		// Example: https://github.com/owner/repo/pull/123
		u, err := neturl.Parse(mrDetails.URL)
		if err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull" {
				owner := parts[1]
				repo := parts[2]
				number := parts[4]
				prID = owner + "/" + repo + "/" + number
				if s.logger != nil {
					s.logger.Log("✓ GitHub PR ID: %s", prID)
				}
				log.Printf("[DEBUG] GitHub: Converted MR ID from '%s' to '%s'", mrDetails.ID, prID)
			} else {
				if s.logger != nil {
					s.logger.Log("⚠ Failed to parse GitHub URL parts (len=%d)", len(parts))
				}
				log.Printf("[DEBUG] GitHub: Failed to parse URL parts, len=%d, parts=%v", len(parts), parts)
			}
		} else {
			if s.logger != nil {
				s.logger.LogError("Failed to parse GitHub URL", err)
			}
			log.Printf("[DEBUG] GitHub: Failed to parse URL: %v", err)
		}
	} else if mrDetails.ProviderType == "bitbucket" {
		if s.logger != nil {
			s.logger.Log("Converting Bitbucket URL for changes API...")
		}
		// Robustly parse workspace, repo, number from MR URL
		// Example: https://bitbucket.org/workspace/repository/pull-requests/123
		u, err := neturl.Parse(mrDetails.URL)
		if err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull-requests" {
				workspace := parts[1]
				repo := parts[2]
				number := parts[4]
				prID = workspace + "/" + repo + "/" + number
				if s.logger != nil {
					s.logger.Log("✓ Bitbucket PR ID: %s", prID)
				}
				log.Printf("[DEBUG] Bitbucket: Converted MR ID from '%s' to '%s'", mrDetails.ID, prID)
			} else {
				if s.logger != nil {
					s.logger.Log("⚠ Failed to parse Bitbucket URL parts (len=%d)", len(parts))
				}
				log.Printf("[DEBUG] Bitbucket: Failed to parse URL parts, len=%d, parts=%v", len(parts), parts)
			}
		} else {
			if s.logger != nil {
				s.logger.LogError("Failed to parse Bitbucket URL", err)
			}
			log.Printf("[DEBUG] Bitbucket: Failed to parse URL: %v", err)
		}
	}
	if s.logger != nil {
		s.logger.Log("Using PR ID for changes API: %s", prID)
	}
	log.Printf("[DEBUG] Using PR ID for GetMergeRequestChanges: %s", prID)
	changes, err := provider.GetMergeRequestChanges(ctx, prID)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("Failed to get code changes", err)
			s.logger.EmitStageError("Analysis", err)
		}
		return nil, fmt.Errorf("failed to get code changes: %w", err)
	}
	if s.logger != nil {
		s.logger.Log("✓ Retrieved %d changed files", len(changes))
		for i, change := range changes {
			hunkCount := len(change.Hunks)
			totalLines := 0
			for _, hunk := range change.Hunks {
				totalLines += len(strings.Split(hunk.Content, "\n"))
			}
			s.logger.Log("  File %d: %s (%d hunks, ~%d lines)", i+1, change.FilePath, hunkCount, totalLines)
		}
	}
	log.Printf("[DEBUG] Retrieved %d changed files", len(changes))
	if s.logger != nil {
		s.logger.EmitStageCompleted("Analysis", fmt.Sprintf("Retrieved %d changed files from merge request", len(changes)))
	}

	// Check if there are no changes to review
	if len(changes) == 0 {
		if s.logger != nil {
			s.logger.Log("⚠ No changes found - skipping AI review")
		}
		log.Printf("[DEBUG] No changes found, returning early")

		// Return a simple success result for empty changes
		return &ReviewWorkflowResult{
			MRDetails: mrDetails,
			Result: &models.ReviewResult{
				Summary:          "# No Changes Detected (LiveReview)\n\nNo changes were found in this pull request.",
				Comments:         []*models.ReviewComment{},
				InternalComments: []*models.ReviewComment{},
			},
		}, nil
	}

	// Review code using batching, structured output, and retry
	if s.logger != nil {
		s.logger.LogSection("AI CODE REVIEW")
		s.logger.EmitStageStarted("Review")
		s.logger.Log("Sending code to AI for review (batching enabled)")
		s.logger.Log("  Total files: %d", len(changes))
	}
	log.Printf("[DEBUG] Sending code to AI for review (batching enabled), total files: %d", len(changes))
	batchProcessor := s.createBatchProcessor()
	result, err := aiProvider.ReviewCodeWithBatching(ctx, changes, batchProcessor)
	if err != nil {
		if s.logger != nil {
			s.logger.LogError("AI review failed", err)
			s.logger.EmitStageError("Review", err)
		}
		return nil, fmt.Errorf("failed to review code (batching): %w", err)
	}
	if s.logger != nil {
		s.logger.Log("✓ AI Review completed successfully")
		s.logger.Log("  Generated %d comments", len(result.Comments))
		s.logger.Log("  Internal comments: %d", len(result.InternalComments))
		s.logger.Log("  Summary length: %d characters", len(result.Summary))
		s.logger.EmitStageCompleted("Review", fmt.Sprintf("Generated %d comments and summary", len(result.Comments)))
	}
	log.Printf("[DEBUG] AI Review (batching) completed successfully with %d comments", len(result.Comments))

	return &ReviewWorkflowResult{
		MRDetails: mrDetails,
		Result:    result,
	}, nil
}

// createBatchProcessor returns a batch processor with recommended settings for batching and retry
func (s *Service) createBatchProcessor() *batch.BatchProcessor {
	processor := batch.DefaultBatchProcessor()
	// Set max tokens per batch to 10,000 (or ~40,000 chars)
	processor.MaxBatchTokens = 10000
	// Configure retry logic (3 retries, 2s delay)
	processor.TaskQueueConfig.MaxRetries = 3
	processor.TaskQueueConfig.RetryDelay = 2 * time.Second
	return processor
}

// postReviewResults posts the review results back to the provider
func (s *Service) postReviewResults(
	ctx context.Context,
	provider providers.Provider,
	mrID string,
	result *models.ReviewResult,
) error {

	if s.logger != nil {
		s.logger.Log("Posting review results to MR ID: %s", mrID)
		s.logger.Log("  Summary length: %d characters", len(result.Summary))
		s.logger.Log("  Individual comments: %d", len(result.Comments))
	}

	// Post the summary as a general comment
	if s.logger != nil {
		s.logger.Log("Creating and posting summary comment...")
	}
	log.Printf("[DEBUG] Creating summary comment")
	summaryComment := &models.ReviewComment{
		FilePath: "",
		Line:     0,
		Content:  result.Summary,
		Severity: models.SeverityInfo,
		Category: "summary",
	}

	log.Printf("[DEBUG] Posting summary comment to MR ID: %s", mrID)
	if err := provider.PostComment(ctx, mrID, summaryComment); err != nil {
		if s.logger != nil {
			s.logger.LogError("Failed to post summary comment", err)
		}
		return fmt.Errorf("failed to post summary comment: %w", err)
	}
	if s.logger != nil {
		s.logger.Log("✓ Summary comment posted successfully")
	}

	// Post specific comments
	if len(result.Comments) > 0 {
		if s.logger != nil {
			s.logger.Log("Posting %d individual comments...", len(result.Comments))
			// Log details of each comment being posted
			for i, comment := range result.Comments {
				s.logger.Log("  Comment %d: %s:%d (%s) - %s",
					i+1, comment.FilePath, comment.Line, comment.Severity,
					comment.Content[:minInt(50, len(comment.Content))]+"...")
			}
		}
		log.Printf("[DEBUG] Posting %d individual comments to merge request...", len(result.Comments))
		err := provider.PostComments(ctx, mrID, result.Comments)
		if err != nil {
			if s.logger != nil {
				s.logger.LogError("Failed to post individual comments", err)
				// This is likely where the 422 error occurs - log detailed info
				s.logger.Log("DETAILED ERROR CONTEXT:")
				s.logger.Log("  MR ID: %s", mrID)
				s.logger.Log("  Comments being posted: %d", len(result.Comments))
				for i, comment := range result.Comments {
					s.logger.Log("  FAILED Comment %d:", i+1)
					s.logger.Log("    File: %s", comment.FilePath)
					s.logger.Log("    Line: %d", comment.Line)
					s.logger.Log("    IsDeletedLine: %v", comment.IsDeletedLine)
					s.logger.Log("    Content: %s", comment.Content)
				}
			}
			return fmt.Errorf("failed to post comments: %w", err)
		}
		if s.logger != nil {
			s.logger.Log("✓ All %d individual comments posted successfully", len(result.Comments))
		}
		log.Printf("[DEBUG] Successfully posted all %d comments", len(result.Comments))
	}

	return nil
}

// ProcessReviewAsync processes a review asynchronously in a goroutine
func (s *Service) ProcessReviewAsync(ctx context.Context, request ReviewRequest, callback func(*ReviewResult)) {
	go func() {
		result := s.ProcessReview(ctx, request)
		if callback != nil {
			callback(result)
		}
	}()
}
