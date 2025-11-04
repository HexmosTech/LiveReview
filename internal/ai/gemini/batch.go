package gemini

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/livereview/internal/batch"
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
	taskQueue := batch.NewTaskQueue(runtime.NumCPU())

	// Use the batch processor's configuration for the task queue if available
	if batchProcessor.TaskQueueConfig.MaxWorkers > 0 {
		taskQueue = batch.ConfigureTaskQueue(batchProcessor.TaskQueueConfig)
		fmt.Printf("🔄 Using task queue with %d workers, %d max retries\n",
			batchProcessor.TaskQueueConfig.MaxWorkers,
			batchProcessor.TaskQueueConfig.MaxRetries)
	}

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

	// Collect batch results
	batchResults := make([]*batch.BatchResult, len(batchInput.Batches))
	totalComments := 0

	fmt.Printf("===== 📋 BATCH RESULTS SUMMARY =====\n")
	for i := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)
		taskResult, ok := results[batchID]

		if !ok || taskResult.Error != nil {
			if !ok {
				return nil, fmt.Errorf("batch %s not found in results", batchID)
			}
			return nil, fmt.Errorf("error processing batch %s: %v", batchID, taskResult.Error)
		}

		batchResult, ok := taskResult.Result.(*batch.BatchResult)
		if !ok {
			return nil, fmt.Errorf("invalid result type for batch %s", batchID)
		}

		// Display batch result details
		fmt.Printf("📦 Batch %d: %d comments", i+1, len(batchResult.Comments))
		if taskResult.Retries > 0 {
			fmt.Printf(" (after %d retries)", taskResult.Retries)
		}
		fmt.Println()

		totalComments += len(batchResult.Comments)
		batchResults[i] = batchResult
	}
	fmt.Printf("✨ Total comments generated: %d\n", totalComments)
	fmt.Printf("===================================\n\n")

	// Step 5: Aggregate and combine outputs
	fmt.Printf("===== 🔄 AGGREGATING RESULTS =====\n")
	result, err := batchProcessor.AggregateAndCombineOutputs(ctx, p.llm, batchResults)
	if err != nil {
		fmt.Printf("❌ Error aggregating results: %v\n", err)
		return nil, err
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
		Error:              nil,
	}, nil
}

// MaxTokensPerBatch returns the maximum number of tokens allowed per batch
func (p *GeminiProvider) MaxTokensPerBatch() int {
	// Return the configured maximum tokens per batch
	return p.TestableFields.MaxTokensPerBatch
}
