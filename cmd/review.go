package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/livereview/internal/ai"
	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/providers"
	"github.com/livereview/internal/providers/gitlab"
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
	verbose := c.Bool("verbose") // Use the command-specific verbose flag

	fmt.Printf("Starting review of MR: %s (dry-run: %v, verbose: %v)\n", mrURL, dryRun, verbose)
	fmt.Printf("Debug: All flags: dry-run=%v, v=%v, d=%v\n", c.Bool("dry-run"), c.Bool("v"), c.Bool("d"))

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

	// Create AI provider
	aiProvider, err := createAIProvider(aiName, cfg.AI[aiName])
	if err != nil {
		return fmt.Errorf("failed to create AI provider: %w", err)
	}

	// Run review
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return runReviewProcess(ctx, provider, aiProvider, mrURL, dryRun, verbose)
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

func createAIProvider(name string, config map[string]interface{}) (ai.Provider, error) {
	switch name {
	case "gemini":
		// Extract Gemini config
		apiKey, _ := config["api_key"].(string)
		model, _ := config["model"].(string)
		temperature, _ := config["temperature"].(float64)

		return gemini.New(gemini.GeminiConfig{
			APIKey:      apiKey,
			Model:       model,
			Temperature: temperature,
		})
	default:
		return nil, fmt.Errorf("unsupported AI provider: %s", name)
	}
}

func runReviewProcess(
	ctx context.Context,
	provider providers.Provider,
	aiProvider ai.Provider,
	mrURL string,
	dryRun bool,
	verbose bool,
) error {
	fmt.Println("Starting review process...")

	// Get MR details
	if verbose {
		fmt.Println("Fetching merge request details...")
	}

	mrDetails, err := provider.GetMergeRequestDetails(ctx, mrURL)
	if err != nil {
		return fmt.Errorf("failed to get merge request details: %w", err)
	}

	fmt.Printf("Got MR details: ID=%s, Title=%s\n", mrDetails.ID, mrDetails.Title)

	// Get MR changes
	if verbose {
		fmt.Println("Fetching code changes...")
	}

	changes, err := provider.GetMergeRequestChanges(ctx, mrDetails.ID)
	if err != nil {
		return fmt.Errorf("failed to get code changes: %w", err)
	}

	fmt.Printf("Got %d changed files\n", len(changes))

	// Review code
	if verbose {
		fmt.Println("Reviewing code changes...")
	}

	result, err := aiProvider.ReviewCode(ctx, changes)
	if err != nil {
		return fmt.Errorf("failed to review code: %w", err)
	}

	fmt.Println("AI Review completed successfully")

	// Post comments
	if !dryRun {
		if verbose {
			fmt.Println("Posting review summary and comments...")
		}

		// Post the summary as a general comment
		summaryComment := &models.ReviewComment{
			FilePath: "", // Empty for MR-level comment
			Line:     0,  // 0 for MR-level comment
			Content:  fmt.Sprintf("# AI Review Summary\n\n%s", result.Summary),
			Severity: models.SeverityInfo,
			Category: "summary",
		}

		if err := provider.PostComment(ctx, mrDetails.ID, summaryComment); err != nil {
			return fmt.Errorf("failed to post summary comment: %w", err)
		}

		// Post specific comments
		if len(result.Comments) > 0 {
			err = provider.PostComments(ctx, mrDetails.ID, result.Comments)
			if err != nil {
				return fmt.Errorf("failed to post comments: %w", err)
			}
		}

		if verbose {
			fmt.Printf("Posted summary and %d comments on merge request\n", len(result.Comments))
		}
	}

	// Always print the results
	fmt.Println("\n=== AI Review Summary ===")
	fmt.Println(result.Summary)
	fmt.Println("\n=== Specific Comments ===")
	for i, comment := range result.Comments {
		fmt.Printf("\n--- Comment %d ---\n", i+1)
		fmt.Printf("File: %s, Line: %d\n", comment.FilePath, comment.Line)
		fmt.Printf("Severity: %s\n", comment.Severity)
		fmt.Printf("Content: %s\n", comment.Content)
		if len(comment.Suggestions) > 0 {
			fmt.Println("Suggestions:")
			for _, suggestion := range comment.Suggestions {
				fmt.Printf("  - %s\n", suggestion)
			}
		}
	}

	return nil
}
