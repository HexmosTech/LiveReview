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
	"github.com/livereview/pkg/models"
)

// ManualToolsOptions coordinates inline static analysis tool execution with ProcessManualReview.
type ManualToolsOptions struct {
	WaitForTools func(ctx context.Context) []*models.ReviewComment
	PostConfig   ToolPostConfig
}

// ProcessManualReview runs a manual code review task without inline tool callbacks.
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
	return ProcessManualReviewWithTools(ctx, db, orgID, planCode, actorUserID, actorEmail, reviewID, requestJSON, nil, onSuccess)
}

// ProcessManualReviewWithTools runs a manual code review task, awaiting any inline
// static analysis tool goroutines before persisting the combined result and completing the review.
func ProcessManualReviewWithTools(
	ctx context.Context,
	db *sql.DB,
	orgID int64,
	planCode string,
	actorUserID *int64,
	actorEmail string,
	reviewID int64,
	requestJSON string,
	toolsOpts *ManualToolsOptions,
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

	eventSink := NewDatabaseEventSink(db)
	if logger != nil {
		logger.SetEventSink(eventSink)
		logger.LogSection("REVIEW PROCESSING STARTED VIA QUEUE")
		logger.Log("Review ID: %d", reviewID)
		logger.Log("Organization ID: %d", orgID)
	}

	// Update review status to in_progress
	rm := NewReviewManager(db)
	_ = rm.UpdateReviewStatus(reviewID, "in_progress")

	// Call ProcessReview (runs concurrently with any inline SAST tool goroutines).
	result := reviewService.ProcessReview(ctx, request)

	if logger != nil {
		logger.LogSection("REVIEW COMPLETION CALLBACK")
		logger.Log("Review processing completed")
	}

	var toolComments []*models.ReviewComment
	if toolsOpts != nil && toolsOpts.WaitForTools != nil {
		toolComments = toolsOpts.WaitForTools(ctx)
	}

	if result != nil && result.Success {
		// Dedup against AI comments BEFORE posting so the PR never receives a tool
		// comment on a line the AI already covered, keeping PR and UI identical.
		toolCommentsToPost, mergedComments := DedupeToolCommentsAgainstAI(result.Comments, toolComments)

		if toolsOpts != nil && len(toolCommentsToPost) > 0 {
			postCtx, postCancel := boundedPostContext(ctx, 2*time.Second)
			PostToolCommentsToProvider(postCtx, db, orgID, reviewID, toolCommentsToPost, toolsOpts.PostConfig, eventSink)
			postCancel()
		}

		// Preserve DB persistence budget even if PR posting consumed the job deadline.
		persistCtx, persistCancel := persistenceGraceContext(ctx, 10*time.Second)
		defer persistCancel()

		if err := rm.MergeReviewMetadata(reviewID, buildQueuedReviewAIMetadata(&request, result)); err != nil {
			log.Printf("[WARN] failed to persist AI stage metadata for review %d: %v", reviewID, err)
		}

		// Same review_result shape the CLI path writes (summary + comments, no diff content) — this is what the dashboard and taxonomy reports both read.
		// Written atomically WITH status='completed' when no tools remain so no
		// poller ever observes status=='completed' before review_result lands.
		if result.CommentsNotPosted != "" {
			// The findings are stored below and shown in LiveReview; this tells the UI why nothing is on the PR.
			if err := rm.MergeReviewMetadata(reviewID, map[string]interface{}{
				"comments_not_posted": map[string]interface{}{"reason": result.CommentsNotPosted},
			}); err != nil {
				log.Printf("[WARN] failed to persist comments_not_posted for review %d: %v", reviewID, err)
			}
		}

		pendingTools := 0
		if toolsOpts == nil {
			// Legacy compatibility path when ProcessManualReview is invoked without inline toolsOpts.
			// Acquire FOR UPDATE on the review row BEFORE computing pendingTools so a sibling
			// ToolReviewWorker cannot finish in between and leave the review stuck in_progress.
			tx, txErr := db.BeginTx(persistCtx, nil)
			if txErr != nil {
				log.Printf("[WARN] ProcessManualReview: failed to begin tx for review %d: %v", reviewID, txErr)
			} else {
				defer func() {
					if rErr := tx.Rollback(); rErr != nil && rErr != sql.ErrTxDone {
						log.Printf("[WARN] ProcessManualReview: failed to rollback tx for review %d: %v", reviewID, rErr)
					}
				}()

				var existingCommentsJSON []byte
				var toolsDispatchedTotal int
				scanErr := tx.QueryRowContext(persistCtx, `
					SELECT 
						COALESCE(metadata->'review_result'->'comments', '[]'::jsonb),
						COALESCE((metadata->>'tools_dispatched')::int, 0)
					FROM public.reviews WHERE id = $1 FOR UPDATE
				`, reviewID).Scan(&existingCommentsJSON, &toolsDispatchedTotal)

				if scanErr == nil && len(existingCommentsJSON) > 0 {
					var existingToolComments []*models.ReviewComment
					if unmarshalErr := json.Unmarshal(existingCommentsJSON, &existingToolComments); unmarshalErr == nil {
						var existingToolsOnly []*models.ReviewComment
						for _, tc := range existingToolComments {
							if tc != nil && (tc.Source == "tool" || tc.ToolName != "") {
								existingToolsOnly = append(existingToolsOnly, tc)
							}
						}
						mergedComments = MergeAndDedupeReviewComments(mergedComments, existingToolsOnly)
					}
				}

				var completedTools int
				_ = tx.QueryRowContext(persistCtx, `SELECT COUNT(DISTINCT data->>'tool_name') FROM public.review_events WHERE review_id = $1 AND event_type = 'tool_result'`, reviewID).Scan(&completedTools)
				if toolsDispatchedTotal > 0 {
					pendingTools = max(toolsDispatchedTotal-completedTools, 0)
				} else {
					_ = tx.QueryRowContext(persistCtx, `
						SELECT COUNT(*) FROM public.review_events
						WHERE review_id = $1 AND event_type = 'tool_dispatch' AND data->>'tool_name' NOT IN (
							SELECT data->>'tool_name' FROM public.review_events WHERE review_id = $1 AND event_type = 'tool_result'
						)`, reviewID).Scan(&pendingTools)
				}

				targetStatus := ""
				if pendingTools == 0 {
					targetStatus = "completed"
				}
				persistManualReviewResultTxWithStatus(persistCtx, tx, reviewID, result.Summary, mergedComments, result.Quiz, result.DiffClassification, "", targetStatus)
			}
		} else {
			if persistManualReviewResultDBWithStatus(persistCtx, db, reviewID, result.Summary, mergedComments, result.Quiz, result.DiffClassification, "", "completed") {
				_ = eventSink.EmitCompletionEvent(persistCtx, reviewID, orgID, result.Summary, len(mergedComments), "")
			}
		}

		if pendingTools == 0 {
			if toolsOpts == nil {
				_ = eventSink.EmitCompletionEvent(persistCtx, reviewID, orgID, result.Summary, len(mergedComments), "")
			}
		} else {
			if logger != nil {
				logger.Log("AI review portion completed (%d comments). Waiting for %d pending static analysis tool(s)...", len(mergedComments), pendingTools)
			}
			log.Printf("[INFO] ProcessManualReview: review=%d AI review completed (%d comments), waiting for %d pending tool(s)", reviewID, len(mergedComments), pendingTools)
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
			if err := onSuccess(persistCtx, result.Model, batchInput, extraMeta); err != nil {
				log.Printf("[WARN] Manual review accounting callback failed: %v", err)
			}
		}
	} else {
		// On AI failure, preserve merged tool findings in LiveReview's review_result
		// but do NOT post tool comments onto the pull request.
		failReason := "review processing encountered errors"
		if result != nil && result.Error != nil {
			failReason = result.Error.Error()
		}
		persistCtx, persistCancel := persistenceGraceContext(ctx, 10*time.Second)
		defer persistCancel()

		mergedToolComments := MergeAndDedupeReviewComments(nil, toolComments)
		if len(mergedToolComments) > 0 {
			summary := ""
			if result != nil {
				summary = result.Summary
			}
			persistManualReviewResultDBWithStatus(persistCtx, db, reviewID, summary, mergedToolComments, nil, nil, failReason, "failed")
		} else {
			_, _ = db.ExecContext(persistCtx, `
				UPDATE public.reviews
				SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb,
					status = 'failed',
					completed_at = NOW()
				WHERE id = $2
			`, mustMarshalFailureMeta(failReason), reviewID)
		}
		_ = eventSink.EmitCompletionEvent(persistCtx, reviewID, orgID, "", len(mergedToolComments), failReason)
	}

	if logger != nil {
		logger.Log("=== Background processing completed ===")
		logger.Close()
	}

	return nil
}

func persistManualReviewResultTx(
	ctx context.Context,
	tx *sql.Tx,
	reviewID int64,
	summary string,
	comments []*models.ReviewComment,
	quiz []models.QuizQuestion,
	diffClassification *models.DiffClassification,
	failureReason string,
) {
	persistManualReviewResultTxWithStatus(ctx, tx, reviewID, summary, comments, quiz, diffClassification, failureReason, "")
}

func persistManualReviewResultTxWithStatus(
	ctx context.Context,
	tx *sql.Tx,
	reviewID int64,
	summary string,
	comments []*models.ReviewComment,
	quiz []models.QuizQuestion,
	diffClassification *models.DiffClassification,
	failureReason string,
	targetStatus string,
) {
	metaBytes, mErr := marshalManualReviewResultMeta(summary, comments, quiz, diffClassification, failureReason)
	if mErr != nil {
		log.Printf("[WARN] ProcessManualReview: failed to marshal review %d result: %v", reviewID, mErr)
		_ = tx.Rollback()
		return
	}
	var updErr error
	if targetStatus != "" {
		_, updErr = tx.ExecContext(ctx, `
			UPDATE public.reviews
			SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb,
				status = CASE WHEN status NOT IN ('completed', 'failed') THEN $3 ELSE status END,
				completed_at = CASE WHEN status NOT IN ('completed', 'failed') THEN NOW() ELSE completed_at END
			WHERE id = $2
		`, metaBytes, reviewID, targetStatus)
	} else {
		_, updErr = tx.ExecContext(ctx, `
			UPDATE public.reviews
			SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb
			WHERE id = $2
		`, metaBytes, reviewID)
	}
	if updErr != nil {
		log.Printf("[WARN] ProcessManualReview: failed to update review %d metadata: %v", reviewID, updErr)
		_ = tx.Rollback()
		return
	}
	if commitErr := tx.Commit(); commitErr != nil {
		log.Printf("[WARN] ProcessManualReview: failed to commit review %d metadata tx: %v", reviewID, commitErr)
		_ = tx.Rollback()
	}
}

func persistManualReviewResultDB(
	ctx context.Context,
	db *sql.DB,
	reviewID int64,
	summary string,
	comments []*models.ReviewComment,
	quiz []models.QuizQuestion,
	diffClassification *models.DiffClassification,
	failureReason string,
) {
	persistManualReviewResultDBWithStatus(ctx, db, reviewID, summary, comments, quiz, diffClassification, failureReason, "")
}

func persistManualReviewResultDBWithStatus(
	ctx context.Context,
	db *sql.DB,
	reviewID int64,
	summary string,
	comments []*models.ReviewComment,
	quiz []models.QuizQuestion,
	diffClassification *models.DiffClassification,
	failureReason string,
	targetStatus string,
) bool {
	metaBytes, mErr := marshalManualReviewResultMeta(summary, comments, quiz, diffClassification, failureReason)
	if mErr != nil {
		log.Printf("[WARN] ProcessManualReview: failed to marshal review %d result: %v", reviewID, mErr)
		return false
	}
	if targetStatus != "" {
		res, updErr := db.ExecContext(ctx, `
			UPDATE public.reviews
			SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb,
				status = CASE WHEN status NOT IN ('completed', 'failed') THEN $3 ELSE status END,
				completed_at = CASE WHEN status NOT IN ('completed', 'failed') THEN NOW() ELSE completed_at END
			WHERE id = $2
		`, metaBytes, reviewID, targetStatus)
		if updErr != nil {
			log.Printf("[WARN] ProcessManualReview: failed to update review %d metadata and status: %v", reviewID, updErr)
			return false
		}
		rows, _ := res.RowsAffected()
		return rows > 0
	}
	res, updErr := db.ExecContext(ctx, `
		UPDATE public.reviews
		SET metadata = COALESCE(metadata, '{}'::jsonb) || $1::jsonb
		WHERE id = $2
	`, metaBytes, reviewID)
	if updErr != nil {
		log.Printf("[WARN] ProcessManualReview: failed to update review %d metadata: %v", reviewID, updErr)
		return false
	}
	rows, _ := res.RowsAffected()
	return rows > 0
}

func marshalManualReviewResultMeta(
	summary string,
	comments []*models.ReviewComment,
	quiz []models.QuizQuestion,
	diffClassification *models.DiffClassification,
	failureReason string,
) ([]byte, error) {
	reviewResultPayload := map[string]interface{}{
		"summary":            summary,
		"comments":           comments,
		"quiz":               quiz,
		"diffClassification": diffClassification,
	}
	meta := map[string]interface{}{
		"review_result":  reviewResultPayload,
		"ai_review_done": true,
	}
	if failureReason != "" {
		meta["failure_reason"] = failureReason
	}
	return json.Marshal(meta)
}

func mustMarshalFailureMeta(failReason string) []byte {
	b, _ := json.Marshal(map[string]interface{}{
		"ai_review_done": true,
		"failure_reason": failReason,
	})
	return b
}

func boundedPostContext(ctx context.Context, reserve time.Duration) (context.Context, context.CancelFunc) {
	deadline, hasDeadline := ctx.Deadline()
	if !hasDeadline {
		return context.WithTimeout(ctx, 2*time.Minute)
	}
	budget := time.Until(deadline) - reserve
	if budget < time.Second {
		budget = time.Second
	}
	return context.WithTimeout(ctx, budget)
}

func persistenceGraceContext(ctx context.Context, fallbackTimeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), fallbackTimeout)
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
