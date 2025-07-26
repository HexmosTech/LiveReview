package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"

	"math"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/logging"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/gitlab"
	"github.com/livereview/internal/review"
	"github.com/livereview/pkg/models"
)

// ReviewCommand returns the review command
func ReviewCommand() *cli.Command {
	return &cli.Command{
		Name:  "review",
		Usage: "Review a merge/pull request",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "dry-run",
				Aliases: []string{"d"},
				Usage:   "Run review without posting comments",
			},
			&cli.StringFlag{
				Name:    "provider",
				Aliases: []string{"p"},
				Usage:   "Override the provider to use",
			},
			&cli.StringFlag{
				Name:    "ai",
				Aliases: []string{"a"},
				Usage:   "Override the AI provider to use",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Enable verbose output for this command",
			},
			&cli.IntFlag{
				Name:    "batch-size",
				Aliases: []string{"b"},
				Usage:   "Maximum number of tokens per batch (0 for provider default)",
				Value:   0,
			},
		},
		ArgsUsage: "MR_URL",
		Action:    runReview,
	}
}

func runReview(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing required argument: MR URL")
	}

	mrURL := c.Args().Get(0)
	dryRun := c.Bool("dry-run")
	verbose := c.Bool("verbose")     // Use the command-specific verbose flag
	batchSize := c.Int("batch-size") // Get batch size from command line

	fmt.Printf("Starting review of MR: %s (dry-run: %v, verbose: %v, batch-size: %d)\n",
		mrURL, dryRun, verbose, batchSize)

	// Load configuration
	cfg, err := config.LoadConfig(c.String("config"))
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	// Determine provider to use
	providerName := cfg.General.DefaultProvider
	if override := c.String("provider"); override != "" {
		providerName = override
	}

	// Determine AI provider to use
	aiName := cfg.General.DefaultAI
	if override := c.String("ai"); override != "" {
		aiName = override
	}

	// Create provider
	provider, err := createProvider(providerName, cfg.Providers[providerName])
	if err != nil {
		return fmt.Errorf("failed to create provider: %w", err)
	}

	// Create AI provider using factory
	aiFactory := review.NewStandardAIProviderFactory()
	aiConfig := review.AIConfig{
		Type:   aiName,
		APIKey: cfg.AI[aiName]["api_key"].(string),
		Model:  cfg.AI[aiName]["model"].(string),
	}
	aiProvider, err := aiFactory.CreateAIProvider(context.Background(), aiConfig)
	if err != nil {
		return fmt.Errorf("failed to create AI provider: %w", err)
	}

	// Get batch configuration
	var batchConfig batch.Config
	if cfg.Batch != nil {
		batchConfig = batch.ConfigFromMap(cfg.Batch)
	} else {
		batchConfig = batch.DefaultConfig()
	}

	// Override batch size from command line if specified
	if batchSize > 0 {
		batchConfig.MaxBatchSize = batchSize
	}

	// Run review
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute) // Increased timeout for batch processing
	defer cancel()

	return runReviewProcess(ctx, provider, aiProvider, mrURL, dryRun, verbose, batchConfig)
}

func createProvider(name string, config map[string]interface{}) (providers.Provider, error) {
	switch name {
	case "gitlab":
		// Extract GitLab config
		url, _ := config["url"].(string)
		token, _ := config["token"].(string)

		return gitlab.New(gitlab.GitLabConfig{
			URL:   url,
			Token: token,
		})
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}

func runReviewProcess(
	ctx context.Context,
	provider providers.Provider,
	aiProvider ai.Provider,
	mrURL string,
	dryRun bool,
	verbose bool,
	batchConfig batch.Config,
) error {
	// Generate unique review ID
	reviewID := fmt.Sprintf("review-%d", time.Now().Unix())

	// Start comprehensive logging
	logger, err := logging.StartReviewLogging(reviewID)
	if err != nil {
		return fmt.Errorf("failed to start review logging: %w", err)
	}
	defer logger.Close()

	logger.Log("Starting trigger-review process")
	logger.Log("Review ID: %s", reviewID)
	logger.Log("MR URL: %s", mrURL)
	logger.Log("Dry run: %v", dryRun)
	logger.Log("Provider: %s", provider.Name())
	logger.Log("AI Provider: %s", aiProvider.Name())

	fmt.Printf("Starting review process (ID: %s)...\n", reviewID)

	// Get MR details
	if verbose {
		fmt.Println("Fetching merge request details...")
	}
	logger.LogSection("FETCHING MR DETAILS")

	mrDetails, err := provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		logger.LogError("GetMergeRequestDetails", err)
		return fmt.Errorf("failed to get merge request details: %w", err)
	}

	logger.Log("MR Details: ID=%s, Title=%s", mrDetails.ID, mrDetails.Title)
	fmt.Printf("Got MR details: ID=%s, Title=%s\n", mrDetails.ID, mrDetails.Title)

	// Get MR changes
	if verbose {
		fmt.Println("Fetching code changes...")
	}
	logger.LogSection("FETCHING CODE CHANGES")

	changes, err := provider.GetMergeRequestChanges(ctx, mrDetails.ID)
	if err != nil {
		logger.LogError("GetMergeRequestChanges", err)
		return fmt.Errorf("failed to get code changes: %w", err)
	}

	logger.Log("Retrieved %d changed files", len(changes))
	for i, change := range changes {
		logger.LogDiff(change.FilePath, change.Hunks[0].Content) // Log first hunk as sample
		if i >= 2 {                                              // Limit initial diff logging to first 3 files
			logger.Log("... (and %d more files)", len(changes)-3)
			break
		}
	}
	fmt.Printf("Got %d changed files\n", len(changes))

	// Review code using batch processing
	if verbose {
		fmt.Println("Reviewing code changes (with batch processing)...")
	}
	logger.LogSection("STARTING AI REVIEW")

	// Create a batch processor with the config
	batchProcessor := batch.DefaultBatchProcessor()

	// Configure the batch processor with the batch config
	batchProcessor.MaxBatchTokens = batchConfig.MaxBatchSize
	batchProcessor.TaskQueueConfig = batchConfig

	// Set up logging based on verbose flag
	if verbose {
		batchProcessor.SetVerboseLogging(true)
	}

	// Override with AI provider max tokens if not specified in config
	if batchProcessor.MaxBatchTokens <= 0 {
		batchProcessor.MaxBatchTokens = aiProvider.MaxTokensPerBatch()
	}

	if verbose {
		fmt.Printf("🔄 Using batch processor with max tokens: %d, workers: %d\n",
			batchProcessor.MaxBatchTokens, batchProcessor.TaskQueueConfig.MaxWorkers)
	}

	// Perform the review with batch processing
	result, err := aiProvider.ReviewCodeWithBatching(ctx, changes, batchProcessor)
	if err != nil {
		return fmt.Errorf("failed to review code: %w", err)
	}

	fmt.Println("AI Review completed successfully")

	// Debug output of results when verbose
	if verbose {
		fmt.Println("\nDEBUG: REVIEW RESULT DETAILS")
		fmt.Println("=============================")
		fmt.Printf("Summary length: %d characters\n", len(result.Summary))
		fmt.Printf("Number of comments: %d\n", len(result.Comments))

		for i, comment := range result.Comments {
			fmt.Printf("\nDEBUG: Comment #%d\n", i+1)
			fmt.Printf("  File Path: '%s'\n", comment.FilePath)
			fmt.Printf("  Line Number: %d\n", comment.Line)
			fmt.Printf("  Severity: %s\n", comment.Severity)
			fmt.Printf("  Content begins: %s\n",
				comment.Content[:int(math.Min(50, float64(len(comment.Content))))])
			fmt.Printf("  Number of suggestions: %d\n", len(comment.Suggestions))
		}
		fmt.Println("=============================")
	}

	// Post comments
	if !dryRun {
		if verbose {
			fmt.Println("Posting review comments...")
		}

		// Post the summary as a general comment
		summaryComment := &models.ReviewComment{
			FilePath: "",             // Empty for MR-level comment
			Line:     0,              // 0 for MR-level comment
			Content:  result.Summary, // The summary already includes a title
			Severity: models.SeverityInfo,
			Category: "summary",
		}

		if err := provider.PostComment(ctx, mrDetails.ID, summaryComment); err != nil {
			return fmt.Errorf("failed to post summary comment: %w", err)
		}

		fmt.Printf("Posted summary comment to merge request\n")

		// Post specific comments
		if len(result.Comments) > 0 {
			fmt.Printf("Posting %d individual comments to merge request...\n", len(result.Comments))
			err = provider.PostComments(ctx, mrDetails.ID, result.Comments)
			if err != nil {
				return fmt.Errorf("failed to post comments: %w", err)
			}
			fmt.Printf("Successfully posted all comments\n")
		}

		if verbose {
			fmt.Printf("Posted summary and %d comments on merge request\n", len(result.Comments))
		}
	}

	// Always print the results
	fmt.Println("\n=== Code Review Results ===")
	fmt.Println(result.Summary)
	fmt.Println("\n=== Specific Comments ===")

	if len(result.Comments) == 0 {
		fmt.Println("\nNo specific issues were identified in the code review.")
	} else {
		for i, comment := range result.Comments {
			fmt.Printf("\n--- Comment %d ---\n", i+1)
			fmt.Printf("File: %s, Line: %d\n", comment.FilePath, comment.Line)
			fmt.Printf("Severity: %s\n", comment.Severity)

			// Print the content (which shouldn't have suggestions in it anymore)
			fmt.Printf("%s\n", comment.Content)

			// Print suggestions separately
			if len(comment.Suggestions) > 0 {
				fmt.Println("\nSuggestions:")
				for j, suggestion := range comment.Suggestions {
					fmt.Printf("%d. %s\n", j+1, suggestion)
				}
			}
		}
	}

	return nil
}
