package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/livereview/internal/ai/gemini"
	"github.com/livereview/internal/config"
	"github.com/livereview/internal/review"
)

// Example demonstrating how to use the new decoupled review architecture
func main() {
	fmt.Println("🚀 LiveReview - New Decoupled Architecture Demo")
	fmt.Println("================================================")

	// Load configuration (replaces hard-coded values)
	cfg, err := config.LoadConfig("")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create factories for dependency injection
	providerFactory := review.NewStandardProviderFactory()
	aiProviderFactory := review.NewStandardAIProviderFactory()

	// Create configuration service
	configService := review.NewConfigurationService(cfg)

	// Create review service with configuration
	reviewConfig := review.Config{
		ReviewTimeout: 10 * time.Minute,
		DefaultAI:     "gemini",
		DefaultModel:  gemini.DefaultModel,
		Temperature:   0.4,
	}

	service := review.NewService(providerFactory, aiProviderFactory, reviewConfig)

	// Example: Review a GitLab merge request
	fmt.Println("\n📋 Creating review request...")

	reviewRequest, err := configService.BuildReviewRequestWithConfig(
		context.Background(),
		"https://git.example.com/group/project/-/merge_requests/123",
		"demo-review-001",
		"gitlab",
		"https://git.example.com",
		"glpat-xxxxxxxxxxxxxxxxxxxx",
		nil, // No extra config
	)
	if err != nil {
		log.Fatalf("Failed to build review request: %v", err)
	}

	// Process review asynchronously with callback
	fmt.Println("⚙️  Starting review process...")

	result := make(chan *review.ReviewResult, 1)

	service.ProcessReviewAsync(context.Background(), *reviewRequest, func(r *review.ReviewResult) {
		result <- r
	})

	// Wait for result (in real usage, this would be handled by the callback)
	fmt.Println("⏳ Waiting for review completion...")

	select {
	case r := <-result:
		if r.Success {
			fmt.Printf("✅ Review completed successfully!\n")
			fmt.Printf("   📝 Summary: %s\n", r.Summary[:min(80, len(r.Summary))])
			fmt.Printf("   💬 Comments: %d\n", r.CommentsCount)
			fmt.Printf("   ⏱️  Duration: %v\n", r.Duration)
		} else {
			fmt.Printf("❌ Review failed: %v\n", r.Error)
		}
	case <-time.After(30 * time.Second):
		fmt.Println("⏰ Timeout waiting for review (this is just a demo)")
	}

	fmt.Println("\n🎉 Demo completed!")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
