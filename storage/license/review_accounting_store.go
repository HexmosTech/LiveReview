package license

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type ReviewAccountingTotals struct {
	TotalBillableLOC    int64
	AccountedOperations int64
	LastAccountedAt     *time.Time
	TotalInputTokens    *int64
	TotalOutputTokens   *int64
	TotalCostUSD        *float64
	TokenTrackedOps     int64
}

type ReviewAccountingOperation struct {
	OperationType  string
	TriggerSource  string
	OperationID    string
	IdempotencyKey string
	BillableLOC    int64
	AccountedAt    time.Time
	Provider       string
	Model          string
	PricingVersion string
	InputTokens    *int64
	OutputTokens   *int64
	CostUSD        *float64
	Metadata       string
}

type ReviewAccountingStore struct {
	db *sql.DB
}

func NewReviewAccountingStore(db *sql.DB) *ReviewAccountingStore {
	return &ReviewAccountingStore{db: db}
}

func (s *ReviewAccountingStore) GetReviewAccountingTotals(ctx context.Context, orgID, reviewID int64) (ReviewAccountingTotals, error) {
	var totals ReviewAccountingTotals
	var lastAccountedAt sql.NullTime
	var inputSum sql.NullInt64
	var outputSum sql.NullInt64
	var costSum sql.NullFloat64

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(billable_loc), 0) AS total_billable_loc,
			COUNT(*) AS accounted_operations,
			MAX(accounted_at) AS last_accounted_at,
			SUM(COALESCE(input_tokens,
				CASE WHEN jsonb_typeof(metadata->'input_tokens') = 'number'
					THEN (metadata->>'input_tokens')::bigint ELSE 0 END)) AS input_tokens_sum,
			SUM(COALESCE(output_tokens,
				CASE WHEN jsonb_typeof(metadata->'output_tokens') = 'number'
					THEN (metadata->>'output_tokens')::bigint ELSE 0 END)) AS output_tokens_sum,
			SUM(COALESCE(llm_cost_usd,
				CASE WHEN jsonb_typeof(metadata->'llm_cost_usd') = 'number'
					THEN (metadata->>'llm_cost_usd')::double precision ELSE 0 END)) AS cost_sum,
			SUM(CASE WHEN input_tokens IS NOT NULL
				OR output_tokens IS NOT NULL
				OR llm_cost_usd IS NOT NULL
				OR jsonb_typeof(metadata->'input_tokens') = 'number'
				OR jsonb_typeof(metadata->'output_tokens') = 'number'
				OR jsonb_typeof(metadata->'llm_cost_usd') = 'number'
				THEN 1 ELSE 0 END) AS token_tracked_ops
		FROM loc_usage_ledger
		WHERE org_id = $1 AND review_id = $2 AND status = 'accounted'
	`, orgID, reviewID).Scan(
		&totals.TotalBillableLOC,
		&totals.AccountedOperations,
		&lastAccountedAt,
		&inputSum,
		&outputSum,
		&costSum,
		&totals.TokenTrackedOps,
	)
	if err != nil {
		return ReviewAccountingTotals{}, fmt.Errorf("query review accounting totals: %w", err)
	}

	if lastAccountedAt.Valid {
		t := lastAccountedAt.Time.UTC()
		totals.LastAccountedAt = &t
	}
	if totals.TokenTrackedOps > 0 {
		if inputSum.Valid {
			v := inputSum.Int64
			totals.TotalInputTokens = &v
		}
		if outputSum.Valid {
			v := outputSum.Int64
			totals.TotalOutputTokens = &v
		}
		if costSum.Valid {
			v := costSum.Float64
			totals.TotalCostUSD = &v
		}
	}

	return totals, nil
}

func (s *ReviewAccountingStore) GetLatestReviewAccountingOperation(ctx context.Context, orgID, reviewID int64) (*ReviewAccountingOperation, error) {
	var op ReviewAccountingOperation
	var provider sql.NullString
	var model sql.NullString
	var pricingVersion sql.NullString
	var metadata sql.NullString
	var inputTokens sql.NullInt64
	var outputTokens sql.NullInt64
	var llmCostUSD sql.NullFloat64

	err := s.db.QueryRowContext(ctx, `
		SELECT
			operation_type,
			trigger_source,
			operation_id,
			idempotency_key,
			billable_loc,
			accounted_at,
			COALESCE(provider, metadata->>'provider') AS provider,
			COALESCE(model, metadata->>'model') AS model,
			COALESCE(pricing_version, metadata->>'pricing_version') AS pricing_version,
			COALESCE(input_tokens,
				CASE WHEN jsonb_typeof(metadata->'input_tokens') = 'number'
					THEN (metadata->>'input_tokens')::bigint ELSE NULL END) AS input_tokens,
			COALESCE(output_tokens,
				CASE WHEN jsonb_typeof(metadata->'output_tokens') = 'number'
					THEN (metadata->>'output_tokens')::bigint ELSE NULL END) AS output_tokens,
			COALESCE(llm_cost_usd,
				CASE WHEN jsonb_typeof(metadata->'llm_cost_usd') = 'number'
					THEN (metadata->>'llm_cost_usd')::double precision ELSE NULL END) AS llm_cost_usd,
			metadata::text
		FROM loc_usage_ledger
		WHERE org_id = $1 AND review_id = $2 AND status = 'accounted'
		ORDER BY accounted_at DESC, id DESC
		LIMIT 1
	`, orgID, reviewID).Scan(
		&op.OperationType,
		&op.TriggerSource,
		&op.OperationID,
		&op.IdempotencyKey,
		&op.BillableLOC,
		&op.AccountedAt,
		&provider,
		&model,
		&pricingVersion,
		&inputTokens,
		&outputTokens,
		&llmCostUSD,
		&metadata,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest review accounting operation: %w", err)
	}

	op.AccountedAt = op.AccountedAt.UTC()
	op.Provider = provider.String
	op.Model = model.String
	op.PricingVersion = pricingVersion.String
	op.Metadata = metadata.String
	if inputTokens.Valid {
		v := inputTokens.Int64
		op.InputTokens = &v
	}
	if outputTokens.Valid {
		v := outputTokens.Int64
		op.OutputTokens = &v
	}
	if llmCostUSD.Valid {
		v := llmCostUSD.Float64
		op.CostUSD = &v
	}

	return &op, nil
}
