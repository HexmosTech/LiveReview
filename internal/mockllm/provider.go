//go:build !production

package mockllm

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/livereview/internal/batch"
	"github.com/livereview/internal/logging"
	"github.com/livereview/pkg/models"
)

// MockAIProvider implements ai.Provider and behaves like a realistic, flaky LLM
type MockAIProvider struct {
	logger *logging.ReviewLogger
}

func (m *MockAIProvider) ReviewCode(ctx context.Context, diffs []*models.CodeDiff) (*models.ReviewResult, error) {
	comments := []*models.ReviewComment{}

	// Seed randomizer
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Only attach comments if we have diffs and hunks
	validDiffs := []*models.CodeDiff{}
	for _, d := range diffs {
		if len(d.Hunks) > 0 {
			validDiffs = append(validDiffs, d)
		}
	}

	// Determine random comment count
	commentCount := MockAIMinCommentCount
	if MockAIMaxCommentCount > MockAIMinCommentCount {
		commentCount += r.Intn(MockAIMaxCommentCount - MockAIMinCommentCount + 1)
	}

	severities := []models.CommentSeverity{
		models.SeverityInfo,
		models.SeverityWarning,
		models.SeverityCritical,
	}

	categories := []string{
		"security",
		"performance",
		"correctness",
		"style",
		"refactoring",
		"concurrency",
	}

	if len(validDiffs) > 0 && commentCount > 0 {
		for i := 0; i < commentCount; i++ {
			diff := validDiffs[r.Intn(len(validDiffs))]
			hunk := diff.Hunks[r.Intn(len(diff.Hunks))]

			lineNum := hunk.NewStartLine
			if hunk.NewLineCount > 0 {
				lineNum = hunk.NewStartLine + r.Intn(hunk.NewLineCount)
			}

			// Construct a random technical comment
			term := technicalTerms[r.Intn(len(technicalTerms))]
			template := reviewtemplates[r.Intn(len(reviewtemplates))]
			commentContent := fmt.Sprintf(template, term)

			// Randomize severity and category
			severity := severities[r.Intn(len(severities))]
			category := categories[r.Intn(len(categories))]

			comments = append(comments, &models.ReviewComment{
				FilePath: diff.FilePath,
				Line:     lineNum,
				Content:  commentContent,
				Severity: severity,
				Category: category,
			})
		}
	}

	return &models.ReviewResult{
		Summary:          "### Mock LLM Review Summary\nSimulated feedback completed successfully.",
		Comments:         comments,
		InternalComments: nil,
	}, nil
}

func (m *MockAIProvider) ReviewCodeBatch(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 1. Simulate Flakiness (503 / 429)
	if MockAIFailureRate > 0.0 && r.Float64() < MockAIFailureRate {
		errors := []error{
			fmt.Errorf("LLM API Error (503): Service Unavailable - overloaded"),
			fmt.Errorf("LLM API Error (429): Rate Limit Exceeded - quota exhausted"),
		}
		chosenErr := errors[r.Intn(len(errors))]
		return nil, chosenErr
	}

	// 2. Simulate Random Processing Delay
	delayRange := int64(MockAIMaxDelay - MockAIMinDelay)
	var actualDelay time.Duration
	if delayRange > 0 {
		actualDelay = MockAIMinDelay + time.Duration(r.Int63n(delayRange))
	} else {
		actualDelay = MockAIMinDelay
	}

	if actualDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(actualDelay):
		}
	}

	// 3. Process Batch
	ptrDiffs := make([]*models.CodeDiff, len(diffs))
	for i := range diffs {
		ptrDiffs[i] = &diffs[i]
	}
	res, err := m.ReviewCode(ctx, ptrDiffs)
	if err != nil {
		return nil, err
	}

	return &batch.BatchResult{
		Summary:            res.Summary,
		FileSummary:        res.Summary,
		TechnicalSummaries: nil,
		Comments:           res.Comments,
	}, nil
}

func (m *MockAIProvider) ReviewCodeWithBatching(ctx context.Context, diffs []*models.CodeDiff, batchProcessor *batch.BatchProcessor) (*models.ReviewResult, error) {
	if len(diffs) == 0 {
		return &models.ReviewResult{
			Summary:  "# No Changes Detected (LiveReview)\n\nNo changes were found in this merge request.",
			Comments: []*models.ReviewComment{},
		}, nil
	}

	// 1. Prepare full input
	input := batchProcessor.PrepareFullInput(diffs)

	// 2. Assess batch requirements (mimicking Gemini/Langchain context window splits)
	needsBatching, batchCount, totalTokens := batchProcessor.AssessBatchRequirements(input)

	if batchProcessor.Logger != nil {
		batchProcessor.Logger.Info("MOCK LLM BATCH PROCESSING ASSESSMENT")
		batchProcessor.Logger.Info("Total changes: %d files, %d total tokens", len(diffs), totalTokens)
		batchProcessor.Logger.Info("Max tokens per batch: %d", batchProcessor.MaxBatchTokens)
		batchProcessor.Logger.Info("Batch processing required: %v", needsBatching)
		batchProcessor.Logger.Info("Number of batches needed: %d", batchCount)
	}

	// 3. Batch inputs
	batchInput := batchProcessor.BatchInputs(input)

	// 4. Process batches using task queue
	taskQueue := batch.NewTaskQueue(4)
	if batchProcessor.TaskQueueConfig.MaxWorkers > 0 {
		taskQueue = batch.ConfigureTaskQueue(batchProcessor.TaskQueueConfig)
	}

	for i, batchDiffs := range batchInput.Batches {
		batchID := fmt.Sprintf("batch-%d", i+1)
		processor := func(ctx context.Context, diffs []models.CodeDiff) (*batch.BatchResult, error) {
			if m.logger != nil {
				m.logger.EmitBatchStart(batchID, len(diffs))
			}
			res, err := m.ReviewCodeBatch(ctx, diffs)
			if err != nil {
				if m.logger != nil {
					m.logger.Log("⚠️ Batch %s failed: %v. Retrying...", batchID, err)
				}
			} else if m.logger != nil && res != nil {
				m.logger.EmitBatchComplete(batchID, len(res.Comments), res.Comments)
			}
			return res, err
		}
		task := batch.NewBatchTask(batchID, batchDiffs, processor)
		task.SetBatchNumber(i + 1)
		task.SetLogger(batchProcessor.Logger)
		taskQueue.AddTask(task)
	}

	// Execute tasks (TaskQueue handles retries for any simulated failures)
	results := taskQueue.ProcessAll(ctx)

	// Collect batch results
	batchResults := make([]*batch.BatchResult, len(batchInput.Batches))
	totalComments := 0

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
		totalComments += len(batchResult.Comments)
		batchResults[i] = batchResult
	}

	// 5. Aggregate results
	allComments := []*models.ReviewComment{}
	for _, br := range batchResults {
		allComments = append(allComments, br.Comments...)
	}

	// Generate structured summary with UI slides format
	aggSummary := GenerateMockSummary(diffs)

	return &models.ReviewResult{
		Summary:          aggSummary,
		Comments:         allComments,
		InternalComments: nil,
	}, nil
}

// GenerateMockSummary creates a slide-compatible markdown summary using actual diff filepaths
func GenerateMockSummary(diffs []*models.CodeDiff) string {
	var highlights []string
	for i, d := range diffs {
		if i >= 3 {
			break // limit to 3 highlight items
		}
		highlights = append(highlights, fmt.Sprintf("- **%s**: Refactor structure and improve safety constraints.", d.FilePath))
	}
	if len(highlights) == 0 {
		highlights = append(highlights, "- **general**: No major changes found in this execution path.")
	}

	return fmt.Sprintf(`# Implement Mock LLM Simulation and Latency Verification

## Overview
This review covers recent updates to the codebase. It simulates potential runtime improvements, refactoring targets, and potential concurrency/resource optimizations across active directories.

## Technical Highlights
%s

## Impact
- **Functionality**: Standardize interface patterns and strengthen safety error handling paths.
- **Risk**: Verifying flaky concurrent pathways requires standard retry and fallback configurations in the task queue.`, strings.Join(highlights, "\n"))
}

func (m *MockAIProvider) Configure(config map[string]interface{}) error {
	return nil
}

func (m *MockAIProvider) Name() string {
	return "mock"
}

func (m *MockAIProvider) MaxTokensPerBatch() int {
	return MockAIMaxTokensPerBatch
}
