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
	"github.com/livereview/pkg/models"
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
		if err := rm.MergeReviewMetadata(reviewID, buildQueuedReviewAIMetadata(&request, result)); err != nil {
			log.Printf("[WARN] failed to persist AI stage metadata for review %d: %v", reviewID, err)
		}

		// Same review_result shape the CLI path writes (summary + comments, no diff content) — this is what the dashboard and taxonomy reports both read.
		// Written BEFORE the status flips to "completed" below: a caller polling
		// (e.g. the CI/CD gate's /evaluate endpoint) treats status=="completed" as
		// the signal that findings are ready, so writing review_result first closes
		// the race where a poll could see "completed" with no findings yet and
		// wrongly allow a build that actually has critical findings.
		if result.CommentsNotPosted != "" {
			// The findings are stored below and shown in LiveReview; this tells the UI why nothing is on the PR.
			if err := rm.MergeReviewMetadata(reviewID, map[string]interface{}{
				"comments_not_posted": map[string]interface{}{"reason": result.CommentsNotPosted},
			}); err != nil {
				log.Printf("[WARN] failed to persist comments_not_posted for review %d: %v", reviewID, err)
			}
		}
		// Lock review and merge any existing tool comments in a transaction to prevent race conditions
		tx, txErr := db.BeginTx(ctx, nil)
		if txErr != nil {
			log.Printf("[WARN] ProcessManualReview: failed to begin tx for review %d: %v", reviewID, txErr)
		} else {
			defer func() {
				if rErr := tx.Rollback(); rErr != nil && rErr != sql.ErrTxDone {
					log.Printf("[WARN] ProcessManualReview: failed to rollback tx for review %d: %v", reviewID, rErr)
				}
			}()

			var existingCommentsJSON []byte
			var toolsDispatched int
			scanErr := tx.QueryRowContext(ctx, `
				SELECT 
					COALESCE(metadata->'review_result'->'comments', '[]'::jsonb),
					COALESCE((metadata->>'tools_dispatched')::int, 0)
				FROM public.reviews WHERE id = $1 FOR UPDATE
			`, reviewID).Scan(&existingCommentsJSON, &toolsDispatched)

			if scanErr == nil && len(existingCommentsJSON) > 0 {
				var existingToolComments []*models.ReviewComment
				if unmarshalErr := json.Unmarshal(existingCommentsJSON, &existingToolComments); unmarshalErr == nil {
					seenKeys := make(map[string]bool)
					for _, c := range result.Comments {
						if c != nil && c.Source == "tool" {
							seenKeys[fmt.Sprintf("%s:%s:%d:%s", c.ToolName, c.FilePath, c.Line, strings.TrimSpace(c.Content))] = true
						}
					}
					for _, tc := range existingToolComments {
						if tc != nil && tc.Source == "tool" {
							key := fmt.Sprintf("%s:%s:%d:%s", tc.ToolName, tc.FilePath, tc.Line, strings.TrimSpace(tc.Content))
							if !seenKeys[key] {
								seenKeys[key] = true
								result.Comments = append(result.Comments, tc)
							}
						}
					}
				}
			}

			reviewResultPayload := map[string]interface{}{
				"summary":            result.Summary,
				"comments":           result.Comments,
				"quiz":               result.Quiz,
				"diffClassification": result.DiffClassification,
			}
			resultBytes, mErr := json.Marshal(reviewResultPayload)
			if mErr != nil {
				log.Printf("[WARN] ProcessManualReview: failed to marshal review %d result: %v", reviewID, mErr)
			} else {
				_, updErr := tx.ExecContext(ctx, `
					UPDATE public.reviews
					SET metadata = jsonb_set(
						jsonb_set(
							COALESCE(metadata, '{}'::jsonb),
							'{review_result}',
							$1::jsonb
						),
						'{ai_review_done}',
						'true'::jsonb
					)
					WHERE id = $2
				`, resultBytes, reviewID)
				if updErr != nil {
					log.Printf("[WARN] ProcessManualReview: failed to update review %d metadata: %v", reviewID, updErr)
				} else if commitErr := tx.Commit(); commitErr != nil {
					log.Printf("[WARN] ProcessManualReview: failed to commit review %d metadata tx: %v", reviewID, commitErr)
				}
			}
		}

		// Check pending tools safely without race conditions
		var toolsDispatchedTotal int
		_ = db.QueryRowContext(ctx, `
			SELECT COALESCE((metadata->>'tools_dispatched')::int, 0)
			FROM public.reviews WHERE id = $1
		`, reviewID).Scan(&toolsDispatchedTotal)

		var completedTools int
		_ = db.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT data->>'tool_name')
			FROM public.review_events
			WHERE review_id = $1 AND event_type = 'tool_result'
		`, reviewID).Scan(&completedTools)

		var pendingTools int
		if toolsDispatchedTotal > 0 {
			pendingTools = toolsDispatchedTotal - completedTools
			if pendingTools < 0 {
				pendingTools = 0
			}
		} else {
			_ = db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM public.review_events
				WHERE review_id = $1 AND event_type = 'tool_dispatch' AND data->>'tool_name' NOT IN (
					SELECT data->>'tool_name' FROM public.review_events WHERE review_id = $1 AND event_type = 'tool_result'
				)`, reviewID).Scan(&pendingTools)
		}

		if pendingTools == 0 {
			res, updErr := db.ExecContext(ctx, `
				UPDATE public.reviews
				SET status = 'completed', completed_at = NOW()
				WHERE id = $1 AND status != 'completed'`, reviewID)
			if updErr == nil {
				if rows, _ := res.RowsAffected(); rows > 0 && logger != nil {
					eventSink := NewDatabaseEventSink(db)
					_ = eventSink.EmitCompletionEvent(ctx, reviewID, orgID, result.Summary, len(result.Comments), "")
				}
			}
		} else {
			if logger != nil {
				logger.Log("AI review portion completed (%d comments). Waiting for %d pending static analysis tool(s)...", len(result.Comments), pendingTools)
			}
			log.Printf("[INFO] ProcessManualReview: review=%d AI review completed (%d comments), waiting for %d pending tool(s)", reviewID, len(result.Comments), pendingTools)
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
		_ = rm.MergeReviewMetadata(reviewID, map[string]interface{}{
			"ai_review_done": true,
			"failure_reason": "review processing encountered errors",
		})
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
