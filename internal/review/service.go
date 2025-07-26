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
	"github.com/livereview/internal/providers"
	"github.com/livereview/pkg/models"
)

// Service represents the review orchestration service
type Service struct {
	providers   ProviderFactory
	aiProviders AIProviderFactory
	config      Config
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
	CreateAIProvider(ctx context.Context, config AIConfig) (ai.Provider, error)
	SupportsAIProvider(aiType string) bool
}

// NewService creates a new review service
func NewService(providers ProviderFactory, aiProviders AIProviderFactory, config Config) *Service {
	return &Service{
		providers:   providers,
		aiProviders: aiProviders,
		config:      config,
	}
}

// ProcessReview orchestrates the entire review process
func (s *Service) ProcessReview(ctx context.Context, request ReviewRequest) *ReviewResult {
	start := time.Now()
	result := &ReviewResult{
		ReviewID: request.ReviewID,
	}

	log.Printf("[INFO] Starting review process for %s (ReviewID: %s)", request.URL, request.ReviewID)

	// Create timeout context
	reviewCtx, cancel := context.WithTimeout(ctx, s.config.ReviewTimeout)
	defer cancel()

	// Step 1: Create provider
	provider, err := s.providers.CreateProvider(reviewCtx, request.Provider)
	if err != nil {
		result.Error = fmt.Errorf("failed to create provider: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Step 2: Create AI provider
	aiProvider, err := s.aiProviders.CreateAIProvider(reviewCtx, request.AI)
	if err != nil {
		result.Error = fmt.Errorf("failed to create AI provider: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Step 3: Execute review workflow
	reviewData, err := s.executeReviewWorkflow(reviewCtx, provider, aiProvider, request.URL)
	if err != nil {
		result.Error = err
		result.Duration = time.Since(start)
		return result
	}

	// Step 4: Post results
	// For GitHub, we need to convert the MR ID to owner/repo/number format for posting comments
	postingID := reviewData.MRDetails.ID
	if reviewData.MRDetails.ProviderType == "github" {
		u, err := neturl.Parse(reviewData.MRDetails.URL)
		if err == nil {
			parts := strings.Split(u.Path, "/")
			if len(parts) >= 5 && parts[3] == "pull" {
				owner := parts[1]
				repo := parts[2]
				number := parts[4]
				postingID = owner + "/" + repo + "/" + number
				log.Printf("[DEBUG] GitHub: Using posting ID '%s' for comments", postingID)
			}
		}
	}
	err = s.postReviewResults(reviewCtx, provider, postingID, reviewData.Result)
	if err != nil {
		result.Error = fmt.Errorf("failed to post results: %w", err)
		result.Duration = time.Since(start)
		return result
	}

	// Success
	result.Success = true
	result.Summary = reviewData.Result.Summary
	result.CommentsCount = len(reviewData.Result.Comments)
	result.Duration = time.Since(start)

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
	log.Printf("[DEBUG] Fetching merge request details for URL: %s", url)
	mrDetails, err := provider.GetMergeRequestDetails(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request details: %w", err)
	}
	log.Printf("[DEBUG] Retrieved MR details successfully. ID: %s, Title: %s", mrDetails.ID, mrDetails.Title)

	// Get MR changes
	log.Printf("[DEBUG] Fetching merge request changes for MR ID: %s", mrDetails.ID)
	// For GitHub, pass owner/repo/number as PR ID
	prID := mrDetails.ID
	if mrDetails.ProviderType == "github" {
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
				log.Printf("[DEBUG] GitHub: Converted MR ID from '%s' to '%s'", mrDetails.ID, prID)
			} else {
				log.Printf("[DEBUG] GitHub: Failed to parse URL parts, len=%d, parts=%v", len(parts), parts)
			}
		} else {
			log.Printf("[DEBUG] GitHub: Failed to parse URL: %v", err)
		}
	}
	log.Printf("[DEBUG] Using PR ID for GetMergeRequestChanges: %s", prID)
	changes, err := provider.GetMergeRequestChanges(ctx, prID)
	if err != nil {
		return nil, fmt.Errorf("failed to get code changes: %w", err)
	}
	log.Printf("[DEBUG] Retrieved %d changed files", len(changes))

	// Review code using batching, structured output, and retry
	log.Printf("[DEBUG] Sending code to AI for review (batching enabled), total files: %d", len(changes))
	batchProcessor := s.createBatchProcessor()
	result, err := aiProvider.ReviewCodeWithBatching(ctx, changes, batchProcessor)
	if err != nil {
		return nil, fmt.Errorf("failed to review code (batching): %w", err)
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

	// Post the summary as a general comment
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
		return fmt.Errorf("failed to post summary comment: %w", err)
	}

	// Post specific comments
	if len(result.Comments) > 0 {
		log.Printf("[DEBUG] Posting %d individual comments to merge request...", len(result.Comments))
		err := provider.PostComments(ctx, mrID, result.Comments)
		if err != nil {
			return fmt.Errorf("failed to post comments: %w", err)
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
