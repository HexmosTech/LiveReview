package reviewprocessor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/livereview/internal/license"
	"github.com/livereview/internal/logging"
	reviewpkg "github.com/livereview/internal/review"
)

// ProcessManualReview runs a manual code review task
func ProcessManualReview(ctx context.Context, db *sql.DB, orgID int64, planCode string, actorUserID *int64, actorEmail string, reviewID int64, requestJSON string) error {
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

		operationID := fmt.Sprintf("manual-review:%d", reviewID)
		idempotencyKey := operationID
		if result.BillableLOC > 0 {
			quotaModule := license.NewQuotaModule(db)
			_, err := quotaModule.RecordBatch(ctx, license.QuotaRecordBatchInput{
				OrgID:          orgID,
				ReviewID:       &reviewID,
				OperationType:  "manual_review",
				TriggerSource:  "manual",
				OperationID:    operationID,
				IdempotencyKey: idempotencyKey,
				BatchIndex:     1,
				Batch: license.QuotaBatchInput{
					PlanCode:                 license.PlanType(planCode),
					Provider:                 result.Provider,
					RawLOCBatch:              result.BillableLOC,
					ProviderTotalInputTokens: result.InputTokens,
					OutputTokensBatch:        result.OutputTokens,
				},
			})
			if err != nil {
				log.Printf("[WARN] Manual review batch accounting failed for review %d: %v", reviewID, err)
			} else {
				finalized, err := quotaModule.FinalizeOperation(ctx, license.QuotaFinalizeInput{
					OrgID:          orgID,
					ReviewID:       &reviewID,
					ActorUserID:    actorUserID,
					ActorEmail:     actorEmail,
					OperationType:  "manual_review",
					TriggerSource:  "manual",
					OperationID:    operationID,
					IdempotencyKey: idempotencyKey,
					Provider:       result.Provider,
					Model:          result.Model,
					BatchFallback:  nil,
				})
				if err != nil {
					log.Printf("[WARN] Manual review accounting finalization failed for review %d: %v", reviewID, err)
				} else {
					meta := map[string]interface{}{
						"operation_raw_loc":      finalized.RawLOCTotal,
						"operation_billable_loc": finalized.EffectiveLOCTotal,
						"operation_extra_loc":    finalized.ExtraEffectiveLOCTotal,
						"context_tokens":         finalized.ContextTokensTotal,
						"allowed_context_tokens": finalized.AllowedContextTokensTotal,
						"extra_context_tokens":   finalized.ExtraContextTokensTotal,
						"input_cost_usd":         finalized.InputCostUSDTotal,
						"output_cost_usd":        finalized.OutputCostUSDTotal,
						"total_cost_usd":         finalized.TotalCostUSDTotal,
						"pricing_version":        finalized.PricingVersion,
						"operation_id":           operationID,
						"idempotency_key":        idempotencyKey,
						"accounted_at":           time.Now().UTC().Format(time.RFC3339),
					}
					for k, v := range aiExecutionMetadataFromConfig(request.AI.Config) {
						meta[k] = v
					}
					_ = rm.MergeReviewMetadata(reviewID, meta)
				}
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

func aiExecutionMetadataFromConfig(config map[string]interface{}) map[string]interface{} {
	meta := map[string]interface{}{}
	if len(config) == 0 {
		return meta
	}
	if mode, ok := config["ai_execution_mode"].(string); ok && strings.TrimSpace(mode) != "" {
		meta["ai_execution_mode"] = strings.TrimSpace(mode)
	}
	if source, ok := config["ai_execution_source"].(string); ok && strings.TrimSpace(source) != "" {
		meta["ai_execution_source"] = strings.TrimSpace(source)
	}
	if provider, ok := config["provider_name"].(string); ok && strings.TrimSpace(provider) != "" {
		meta["ai_provider_name"] = strings.TrimSpace(provider)
	}
	if connectorName, ok := config["connector_name"].(string); ok && strings.TrimSpace(connectorName) != "" {
		meta["ai_connector_name"] = strings.TrimSpace(connectorName)
	}
	return meta
}
