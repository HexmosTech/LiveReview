package gemini

import (
	"context"
	"fmt"
	"strings"

	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/llm/adaptivelimiter"
	"github.com/livereview/internal/prompts"
	"github.com/livereview/pkg/models"
)

// ReviewCodeWithBatching processes code diffs in batches for large merge requests
func (p *GeminiProvider) ReviewCodeWithBatching(
	ctx context.Context,
	diffs []*models.CodeDiff,
	batchProcessor *batch.BatchProcessor,
) (*models.ReviewResult, error) {
	// If no diffs, return an empty review
	if len(diffs) == 0 {
		return &models.ReviewResult{
			Summary:  "# No Changes Detected (LiveReview)\n\nNo changes were found in this merge request.",
			Comments: []*models.ReviewComment{},
		}, nil
	}

	// Step 1: Prepare full input
	input := batchProcessor.PrepareFullInput(diffs)

	// Step 2: Assess batch requirements
	needsBatching, batchCount, totalTokens := batchProcessor.AssessBatchRequirements(input)

	fmt.Printf("\n\n===== 📊 BATCH PROCESSING ASSESSMENT =====\n")
	fmt.Printf("📝 Total changes: %d files, %d total tokens\n", len(diffs), totalTokens)
	fmt.Printf("⚙️ Max tokens per batch: %d\n", batchProcessor.MaxBatchTokens)
	fmt.Printf("🔀 Batch processing required: %v\n", needsBatching)
	fmt.Printf("📦 Number of batches needed: %d\n", batchCount)
	fmt.Printf("==========================================\n\n")

	// Step 3: Batch inputs
	batchInput := batchProcessor.BatchInputs(input)

	// Step 4: Process batches with LLM using task queue
	taskQueue := batch.NewTaskQueue(batch.DefaultMaxConcurrentBatches)

	// Use the batch processor's configuration for the task queue if available
	if batchProcessor.TaskQueueConfig.MaxWorkers > 0 {
		taskQueue = batch.ConfigureTaskQueue(batchProcessor.TaskQueueConfig)
		fmt.Printf("🔄 Using task queue with %d workers, %d max retries\n",
			batchProcessor.TaskQueueConfig.MaxWorkers,
			batchProcessor.TaskQueueConfig.MaxRetries)
	}

	// Tier 2: gate every batch's LLM call through the process-wide, self-tuning
	// limiter for this connector (see internal/llm/adaptivelimiter) - shared
	// across every concurrently-running review using this same Gemini connector.
	connectorLimiter := adaptivelimiter.ForConnector(
		"gemini",
		adaptivelimiter.Config{Floor: 2, Ceiling: 64, Initial: batch.DefaultMaxConcurrentBatches},
	)

	// Create tasks for each batch
	for i, batchDiffs := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)

		// Create a processor function for this batch
		processor := func(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
			return p.ReviewCodeBatch(ctx, diffs)
		}

		// Create and add the task
		task := batch.NewBatchTask(batchID, batchDiffs, processor)
		task.SetBatchNumber(i + 1)
		task.SetLogger(batchProcessor.Logger)
		task.SetLimiter(connectorLimiter)
		task.SetProviderType("gemini")
		taskQueue.AddTask(task)

		// Display batch details
		if i == 0 || i == len(batchInput.Batches)-1 || len(batchInput.Batches) <= 5 {
			fmt.Printf("📦 Batch %d: %d files\n", i+1, len(batchDiffs))
		} else if i == 1 && len(batchInput.Batches) > 5 {
			fmt.Printf("... %d more batches ...\n", len(batchInput.Batches)-2)
		}
	}

	fmt.Printf("\n===== 🚀 STARTING BATCH PROCESSING =====\n")
	// Process all tasks
	results := taskQueue.ProcessAll(ctx)
	fmt.Printf("===== ✅ BATCH PROCESSING COMPLETE =====\n\n")

	// Collect batch results - TOLERATE PARTIAL FAILURES. One bad batch (a
	// transient LLM error, a parse failure, ...) must not discard every other
	// batch's already-good comments and fail the whole review; only abort if
	// every batch failed. Mirrors internal/ai/langchain/provider.go's
	// ReviewCodeWithBatching, which already does this correctly.
	var batchResults []*batch.BatchResult
	var failedBatches []string
	var partialFailure bool
	totalComments := 0

	fmt.Printf("===== 📋 BATCH RESULTS SUMMARY =====\n")
	for i := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)
		taskResult, ok := results[batchID]

		if !ok || taskResult.Error != nil {
			failedBatches = append(failedBatches, batchID)
			partialFailure = true
			if !ok {
				fmt.Printf("[PARTIAL FAILURE] Batch %s not found in results, skipping\n", batchID)
			} else {
				fmt.Printf("[PARTIAL FAILURE] Batch %s failed: %v, skipping\n", batchID, taskResult.Error)
			}
			continue
		}

		batchResult, ok := taskResult.Result.(*batch.BatchResult)
		if !ok {
			failedBatches = append(failedBatches, batchID)
			partialFailure = true
			fmt.Printf("[PARTIAL FAILURE] Batch %s: invalid result type, skipping\n", batchID)
			continue
		}

		// Display batch result details
		fmt.Printf("📦 Batch %d: %d comments", i+1, len(batchResult.Comments))
		if taskResult.Retries > 0 {
			fmt.Printf(" (after %d retries)", taskResult.Retries)
		}
		fmt.Println()

		totalComments += len(batchResult.Comments)
		batchResults = append(batchResults, batchResult)
	}
	fmt.Printf("✨ Total comments generated: %d\n", totalComments)
	fmt.Printf("===================================\n\n")

	// If NO batches succeeded, fail the entire review - there's nothing to aggregate.
	if len(batchResults) == 0 {
		return nil, fmt.Errorf("all %d batches failed: %v", len(batchInput.Batches), failedBatches)
	}

	// Step 5: Aggregate and combine outputs
	fmt.Printf("===== 🔄 AGGREGATING RESULTS =====\n")
	result, err := batchProcessor.AggregateAndCombineOutputs(ctx, p.llm, batchResults)
	if err != nil {
		fmt.Printf("❌ Error aggregating results: %v\n", err)
		return nil, err
	}

	if partialFailure {
		warningMsg := fmt.Sprintf("\n\n---\n\n⚠️ **Partial Review Results**\n\nThis review completed with partial results: **%d out of %d batches** processed successfully.\n\n**Failed batches:** %v\n\nThe comments and analysis shown are based only on the files that were successfully reviewed. Some files may have been skipped due to batch processing errors.",
			len(batchResults), len(batchInput.Batches), failedBatches)
		result.Summary = result.Summary + warningMsg
		fmt.Printf("\n[PARTIAL SUCCESS] Review completed with %d/%d successful batches (failed: %v)\n",
			len(batchResults), len(batchInput.Batches), failedBatches)
	}

	// Filter out any useless comments and enhance the meaningful ones
	result.Comments = filterAndEnhanceComments(result.Comments)

	fmt.Printf("✅ Final result: %d meaningful, high-quality comments\n", len(result.Comments))
	fmt.Printf("================================\n\n")

	return result, nil
}

// ReviewCodeBatch processes a single batch of code diffs
func (p *GeminiProvider) ReviewCodeBatch(
	ctx context.Context,
	diffs []models.CodeDiff,
) (*batch.BatchResult, error) {
	// Convert []models.CodeDiff to []*models.CodeDiff for legacy method
	ptrDiffs := make([]*models.CodeDiff, len(diffs))
	for i := range diffs {
		ptrDiffs[i] = &diffs[i]
	}

	// Use the existing ReviewCode method
	result, err := p.ReviewCode(ctx, ptrDiffs)
	if err != nil {
		return nil, err
	}

	// Convert to BatchResult
	trimmedSummary := strings.TrimSpace(result.Summary)
	var summaries []prompts.TechnicalSummary
	if trimmedSummary != "" {
		summaries = append(summaries, prompts.TechnicalSummary{Summary: trimmedSummary})
	}

	return &batch.BatchResult{
		Summary:            trimmedSummary,
		FileSummary:        trimmedSummary,
		TechnicalSummaries: summaries,
		Comments:           result.Comments,
		DiffClassification: result.DiffClassification,
		Error:              nil,
	}, nil
}

// MaxTokensPerBatch returns the maximum number of tokens allowed per batch
func (p *GeminiProvider) MaxTokensPerBatch() int {
	// Return the configured maximum tokens per batch
	return p.TestableFields.MaxTokensPerBatch
}
