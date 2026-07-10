package reviewprocessor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	reviewpkg "github.com/livereview/internal/review"
)

// ProcessManualReview runs a manual code review task
func ProcessManualReview(
	ctx context.Context,
	db *sql.DB,
	orgID int64,
	planCode string,
	actorUserID *int64,
	actorEmail string,
	reviewID int64,
	requestJSON string,
	onSuccess func(ctx context.Context, model string, batch license.QuotaBatchInput, extraMeta map[string]interface{}) error,
) error {
	var request reviewpkg.ReviewRequest
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		log.Printf("[ERROR] ProcessManualReview: Failed to unmarshal review request: %v", err)
		return fmt.Errorf("failed to unmarshal review request: %w", err)
	}

	// Recreate reviewService instance
	providerFactory := reviewpkg.NewStandardProviderFactory()
	aiProviderFactory := reviewpkg.NewStandardAIProviderFactory()
	reviewConfig := reviewpkg.DefaultReviewConfig()
	reviewService := reviewpkg.NewService(providerFactory, aiProviderFactory, reviewConfig)

	reviewIDStr := fmt.Sprintf("%d", reviewID)
	logger, err := logging.StartReviewLoggingWithIDs(reviewIDStr, reviewID, orgID)
	if err != nil {
		log.Printf("[WARN] ProcessManualReview: Failed to start logging: %v", err)
	}

	if logger != nil {
		eventSink := NewDatabaseEventSink(db)
		logger.SetEventSink(eventSink)
		logger.LogSection("REVIEW PROCESSING STARTED VIA QUEUE")
		logger.Log("Review ID: %d", reviewID)
		logger.Log("Organization ID: %d", orgID)
	}

	// Update review status to in_progress
	rm := NewReviewManager(db)
	_ = rm.UpdateReviewStatus(reviewID, "in_progress")

	// Call ProcessReview
	result := reviewService.ProcessReview(ctx, request)

	if logger != nil {
		logger.LogSection("REVIEW COMPLETION CALLBACK")
		logger.Log("Review processing completed")
	}

	if result != nil && result.Success {
		_ = rm.UpdateReviewStatus(reviewID, "completed")
		if err := rm.MergeReviewMetadata(reviewID, buildQueuedReviewAIMetadata(&request, result)); err != nil {
			log.Printf("[WARN] failed to persist AI stage metadata for review %d: %v", reviewID, err)
		}

		if result.BillableLOC > 0 && onSuccess != nil {
			extraMeta := buildQueuedReviewAIMetadata(&request, result)
			batchInput := license.QuotaBatchInput{
				PlanCode:                 license.PlanType(planCode),
				Provider:                 result.Provider,
				RawLOCBatch:              result.BillableLOC,
				ProviderTotalInputTokens: result.InputTokens,
				OutputTokensBatch:        result.OutputTokens,
			}
			if err := onSuccess(ctx, result.Model, batchInput, extraMeta); err != nil {
				log.Printf("[WARN] Manual review accounting callback failed: %v", err)
			}
		}
	} else {
		_ = rm.UpdateReviewStatus(reviewID, "failed")
	}

	if logger != nil {
		logger.Log("=== Background processing completed ===")
		logger.Close()
	}

	return nil
}

func buildQueuedReviewAIMetadata(request *reviewpkg.ReviewRequest, result *reviewpkg.ReviewResult) map[string]interface{} {
	if request == nil || result == nil {
		return map[string]interface{}{}
	}

	meta := map[string]interface{}{
		"helper_enabled": request.HelperEnabled,
		"helper_mode":    strings.TrimSpace(request.HelperMode),
	}

	stages := make([]map[string]interface{}, 0, 2)
	if result.LeaderUsage != nil {
		stages = append(stages, queuedStageUsageToMetadata(result.LeaderUsage))
	}
	if result.HelperUsage != nil {
		stages = append(stages, queuedStageUsageToMetadata(result.HelperUsage))
	}
	if len(stages) > 0 {
		meta["stage_breakdown"] = stages
	}

	for k, v := range queuedAIExecutionMetadataForRole("leader", request.AI.Config) {
		meta[k] = v
	}
	if request.HelperAI != nil {
		for k, v := range queuedAIExecutionMetadataForRole("helper", request.HelperAI.Config) {
			meta[k] = v
		}
	}

	return meta
}

func queuedStageUsageToMetadata(usage *reviewpkg.AIStageUsage) map[string]interface{} {
	meta := map[string]interface{}{
		"stage":           usage.Stage,
		"provider":        usage.Provider,
		"model":           usage.Model,
		"pricing_version": usage.PricingVersion,
	}
	if usage.InputTokens != nil {
		meta["input_tokens"] = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		meta["output_tokens"] = *usage.OutputTokens
	}
	if usage.CostUSD != nil {
		meta["cost_usd"] = *usage.CostUSD
	}
	return meta
}

func queuedAIExecutionMetadataForRole(role string, config map[string]interface{}) map[string]interface{} {
	meta := map[string]interface{}{}
	if len(config) == 0 {
		return meta
	}
	prefix := strings.TrimSpace(role)
	if prefix == "" {
		prefix = "ai"
	} else {
		prefix = prefix + "_ai"
	}
	if mode, ok := config["ai_execution_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		meta[prefix+"_execution_mode"] = strings.TrimSpace(mode)
	}
	if source, ok := config["ai_execution_source"].(string); ok && strings.TrimSpace(source) != "" {
		meta[prefix+"_execution_source"] = strings.TrimSpace(source)
	}
	if provider, ok := config["provider_name"].(string); ok && strings.TrimSpace(provider) != "" {
		meta[prefix+"_provider_name"] = strings.TrimSpace(provider)
	}
	if connectorName, ok := config["connector_name"].(string); ok && strings.TrimSpace(connectorName) != "" {
		meta[prefix+"_connector_name"] = strings.TrimSpace(connectorName)
	}
	return meta
}
