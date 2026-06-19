package jobqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/livereview/internal/license"
	reviewprocessor "github.com/livereview/internal/review_processor"
	"github.com/riverqueue/river"
)

// UpdateOrgUsageJobArgs represents the arguments for an asynchronous billing finalization job
type UpdateOrgUsageJobArgs struct {
	OrgID          int64                   `json:"org_id"`
	ReviewID       *int64                  `json:"review_id,omitempty"`
	ActorUserID    *int64                  `json:"actor_user_id,omitempty"`
	ActorEmail     string                  `json:"actor_email,omitempty"`
	OperationType  string                  `json:"operation_type"`
	TriggerSource  string                  `json:"trigger_source"`
	OperationID    string                  `json:"operation_id"`
	IdempotencyKey string                  `json:"idempotency_key"`
	Provider       string                  `json:"provider"`
	Model          string                  `json:"model"`
	Batch          license.QuotaBatchInput `json:"batch"`
	ExtraMeta      map[string]any          `json:"extra_meta,omitempty"`
}

func (UpdateOrgUsageJobArgs) Kind() string {
	return "update_org_usage"
}

// UpdateOrgUsageWorker handles async updates to organization billing state
type UpdateOrgUsageWorker struct {
	river.WorkerDefaults[UpdateOrgUsageJobArgs]
	db   *sql.DB
	pool *pgxpool.Pool
}

func (w *UpdateOrgUsageWorker) Timeout(job *river.Job[UpdateOrgUsageJobArgs]) time.Duration {
	return 2 * time.Minute
}

func (w *UpdateOrgUsageWorker) Work(ctx context.Context, job *river.Job[UpdateOrgUsageJobArgs]) error {
	args := job.Args

	quotaModule := license.NewQuotaModule(w.db)

	// 1. Record the batch in the ledger asynchronously
	_, err := quotaModule.RecordBatch(ctx, license.QuotaRecordBatchInput{
		OrgID:          args.OrgID,
		ReviewID:       args.ReviewID,
		OperationType:  args.OperationType,
		TriggerSource:  args.TriggerSource,
		OperationID:    args.OperationID,
		IdempotencyKey: args.IdempotencyKey,
		BatchIndex:     1,
		Batch:          args.Batch,
	})
	if err != nil {
		log.Printf("[ERROR] UpdateOrgUsageWorker: RecordBatch failed for OrgID %d, IdempotencyKey %s: %v", args.OrgID, args.IdempotencyKey, err)
		return fmt.Errorf("ledger recording failed: %w", err)
	}

	// 2. Finalize the operation (updates org_billing_state and queues outbox notifications)
	finalized, err := quotaModule.FinalizeOperation(ctx, license.QuotaFinalizeInput{
		OrgID:          args.OrgID,
		ReviewID:       args.ReviewID,
		ActorUserID:    args.ActorUserID,
		ActorEmail:     args.ActorEmail,
		OperationType:  args.OperationType,
		TriggerSource:  args.TriggerSource,
		OperationID:    args.OperationID,
		IdempotencyKey: args.IdempotencyKey,
		Provider:       args.Provider,
		Model:          args.Model,
		BatchFallback:  nil,
	})
	if err != nil {
		log.Printf("[ERROR] UpdateOrgUsageWorker: FinalizeOperation failed for OrgID %d, IdempotencyKey %s: %v", args.OrgID, args.IdempotencyKey, err)
		return fmt.Errorf("billing finalization failed: %w", err)
	}

	if args.ReviewID != nil {
		rm := reviewprocessor.NewReviewManager(w.db)
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
			"operation_id":           args.OperationID,
			"idempotency_key":        args.IdempotencyKey,
			"accounted_at":           time.Now().UTC().Format(time.RFC3339),
		}
		for k, v := range args.ExtraMeta {
			meta[k] = v
		}
		if err := rm.MergeReviewMetadata(*args.ReviewID, meta); err != nil {
			log.Printf("[WARN] UpdateOrgUsageWorker: failed to store metadata for review %d: %v", *args.ReviewID, err)
		}
	}

	return nil
}
