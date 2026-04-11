package license

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type QuotaPolicyRecord struct {
	PlanCode                      string
	ProviderKey                   string
	InputCharsPerLOC              int64
	OutputCharsPerLOC             int64
	CharsPerToken                 int64
	LOCBudgetRatio                float64
	ContextBudgetRatio            float64
	OpsReservedRatio              float64
	InputCostPerMillionTokensUSD  float64
	OutputCostPerMillionTokensUSD float64
	RoundingScale                 int64
	MonthlyPriceUSD               int64
	MonthlyLOCLimit               int64
}

type QuotaBatchSettlementRecord struct {
	OrgID                        int64
	ReviewID                     *int64
	OperationType                string
	TriggerSource                string
	OperationID                  string
	IdempotencyKey               string
	BatchIndex                   int64
	PlanCode                     string
	PolicyProviderKey            string
	PricingVersion               string
	RawLOCBatch                  int64
	EffectiveLOCBatch            int64
	ExtraEffectiveLOCBatch       int64
	DiffInputTokensBatch         int64
	ContextCharsBatch            int64
	ContextTokensBatch           int64
	AllowedContextTokensBatch    int64
	ExtraContextTokensBatch      int64
	ProviderInputTokensBatch     int64
	OutputTokensBatch            int64
	InputCostUSDBatch            float64
	OutputCostUSDBatch           float64
	TotalCostUSDBatch            float64
	ContextTokensPerLOCAllowance float64
	AccountedAt                  time.Time
}

type QuotaBatchAggregate struct {
	BatchCount                int64
	PlanCode                  string
	PricingVersion            string
	RawLOCTotal               int64
	EffectiveLOCTotal         int64
	ExtraEffectiveLOCTotal    int64
	DiffInputTokensTotal      int64
	ContextCharsTotal         int64
	ContextTokensTotal        int64
	AllowedContextTokensTotal int64
	ExtraContextTokensTotal   int64
	ProviderInputTokensTotal  int64
	OutputTokensTotal         int64
	InputCostUSDTotal         float64
	OutputCostUSDTotal        float64
	TotalCostUSDTotal         float64
}

type QuotaOperationAggregateRecord struct {
	OrgID                     int64
	ReviewID                  *int64
	OperationType             string
	TriggerSource             string
	OperationID               string
	IdempotencyKey            string
	PlanCode                  string
	Provider                  string
	Model                     string
	PricingVersion            string
	BatchCount                int64
	RawLOCTotal               int64
	EffectiveLOCTotal         int64
	ExtraEffectiveLOCTotal    int64
	DiffInputTokensTotal      int64
	ContextCharsTotal         int64
	ContextTokensTotal        int64
	AllowedContextTokensTotal int64
	ExtraContextTokensTotal   int64
	ProviderInputTokensTotal  int64
	OutputTokensTotal         int64
	InputCostUSDTotal         float64
	OutputCostUSDTotal        float64
	TotalCostUSDTotal         float64
	FinalizedAt               time.Time
}

type QuotaStore struct {
	db *sql.DB
}

func NewQuotaStore(db *sql.DB) *QuotaStore {
	return &QuotaStore{db: db}
}

func (s *QuotaStore) ResolvePolicy(ctx context.Context, planCode string, provider string) (QuotaPolicyRecord, error) {
	providerKey := strings.ToLower(strings.TrimSpace(provider))
	if providerKey == "" {
		providerKey = "default"
	}

	query := `
		SELECT
			qpc.plan_code,
			qpc.provider_key,
			qpc.input_chars_per_loc,
			qpc.output_chars_per_loc,
			qpc.chars_per_token,
			qpc.loc_budget_ratio,
			qpc.context_budget_ratio,
			qpc.ops_reserved_ratio,
			qpc.input_cost_per_million_tokens_usd,
			qpc.output_cost_per_million_tokens_usd,
			qpc.rounding_scale,
			pc.monthly_price_usd,
			pc.monthly_loc_limit
		FROM quota_policy_catalog qpc
		JOIN plan_catalog pc ON pc.plan_code = qpc.plan_code
		WHERE qpc.plan_code = $1
		  AND qpc.active = TRUE
		  AND qpc.provider_key IN ($2, 'default')
		ORDER BY CASE WHEN qpc.provider_key = $2 THEN 0 ELSE 1 END
		LIMIT 1
	`

	var out QuotaPolicyRecord
	err := s.db.QueryRowContext(ctx, query, strings.TrimSpace(planCode), providerKey).Scan(
		&out.PlanCode,
		&out.ProviderKey,
		&out.InputCharsPerLOC,
		&out.OutputCharsPerLOC,
		&out.CharsPerToken,
		&out.LOCBudgetRatio,
		&out.ContextBudgetRatio,
		&out.OpsReservedRatio,
		&out.InputCostPerMillionTokensUSD,
		&out.OutputCostPerMillionTokensUSD,
		&out.RoundingScale,
		&out.MonthlyPriceUSD,
		&out.MonthlyLOCLimit,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return QuotaPolicyRecord{}, fmt.Errorf("no active quota policy found for plan=%s provider=%s", planCode, providerKey)
		}
		return QuotaPolicyRecord{}, fmt.Errorf("resolve quota policy: %w", err)
	}
	return out, nil
}

func (s *QuotaStore) UpsertBatchSettlement(ctx context.Context, rec QuotaBatchSettlementRecord) error {
	if rec.AccountedAt.IsZero() {
		rec.AccountedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO quota_batch_settlements (
			org_id,
			review_id,
			operation_type,
			trigger_source,
			operation_id,
			idempotency_key,
			batch_index,
			plan_code,
			policy_provider_key,
			pricing_version,
			raw_loc_batch,
			effective_loc_batch,
			extra_effective_loc_batch,
			diff_input_tokens_batch,
			context_chars_batch,
			context_tokens_batch,
			allowed_context_tokens_batch,
			extra_context_tokens_batch,
			provider_total_input_tokens_batch,
			output_tokens_batch,
			input_cost_usd_batch,
			output_cost_usd_batch,
			total_cost_usd_batch,
			context_tokens_per_loc_allowance,
			accounted_at,
			created_at,
			updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,NOW(),NOW()
		)
		ON CONFLICT (org_id, idempotency_key, batch_index)
		DO UPDATE SET
			review_id = EXCLUDED.review_id,
			operation_type = EXCLUDED.operation_type,
			trigger_source = EXCLUDED.trigger_source,
			operation_id = EXCLUDED.operation_id,
			plan_code = EXCLUDED.plan_code,
			policy_provider_key = EXCLUDED.policy_provider_key,
			pricing_version = EXCLUDED.pricing_version,
			raw_loc_batch = EXCLUDED.raw_loc_batch,
			effective_loc_batch = EXCLUDED.effective_loc_batch,
			extra_effective_loc_batch = EXCLUDED.extra_effective_loc_batch,
			diff_input_tokens_batch = EXCLUDED.diff_input_tokens_batch,
			context_chars_batch = EXCLUDED.context_chars_batch,
			context_tokens_batch = EXCLUDED.context_tokens_batch,
			allowed_context_tokens_batch = EXCLUDED.allowed_context_tokens_batch,
			extra_context_tokens_batch = EXCLUDED.extra_context_tokens_batch,
			provider_total_input_tokens_batch = EXCLUDED.provider_total_input_tokens_batch,
			output_tokens_batch = EXCLUDED.output_tokens_batch,
			input_cost_usd_batch = EXCLUDED.input_cost_usd_batch,
			output_cost_usd_batch = EXCLUDED.output_cost_usd_batch,
			total_cost_usd_batch = EXCLUDED.total_cost_usd_batch,
			context_tokens_per_loc_allowance = EXCLUDED.context_tokens_per_loc_allowance,
			accounted_at = EXCLUDED.accounted_at,
			updated_at = NOW()
	`,
		rec.OrgID,
		nullReviewID(rec.ReviewID),
		strings.TrimSpace(rec.OperationType),
		strings.TrimSpace(rec.TriggerSource),
		strings.TrimSpace(rec.OperationID),
		strings.TrimSpace(rec.IdempotencyKey),
		rec.BatchIndex,
		strings.TrimSpace(rec.PlanCode),
		strings.TrimSpace(rec.PolicyProviderKey),
		strings.TrimSpace(rec.PricingVersion),
		rec.RawLOCBatch,
		rec.EffectiveLOCBatch,
		rec.ExtraEffectiveLOCBatch,
		rec.DiffInputTokensBatch,
		rec.ContextCharsBatch,
		rec.ContextTokensBatch,
		rec.AllowedContextTokensBatch,
		rec.ExtraContextTokensBatch,
		rec.ProviderInputTokensBatch,
		rec.OutputTokensBatch,
		rec.InputCostUSDBatch,
		rec.OutputCostUSDBatch,
		rec.TotalCostUSDBatch,
		rec.ContextTokensPerLOCAllowance,
		rec.AccountedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert quota batch settlement: %w", err)
	}
	return nil
}

func (s *QuotaStore) BuildAggregateFromBatches(ctx context.Context, orgID int64, idempotencyKey string) (QuotaBatchAggregate, error) {
	query := `
		SELECT
			COUNT(*) AS batch_count,
			MAX(plan_code) AS plan_code,
			MAX(pricing_version) AS pricing_version,
			COALESCE(SUM(raw_loc_batch), 0) AS raw_loc_total,
			COALESCE(SUM(effective_loc_batch), 0) AS effective_loc_total,
			COALESCE(SUM(extra_effective_loc_batch), 0) AS extra_effective_loc_total,
			COALESCE(SUM(diff_input_tokens_batch), 0) AS diff_input_tokens_total,
			COALESCE(SUM(context_chars_batch), 0) AS context_chars_total,
			COALESCE(SUM(context_tokens_batch), 0) AS context_tokens_total,
			COALESCE(SUM(allowed_context_tokens_batch), 0) AS allowed_context_tokens_total,
			COALESCE(SUM(extra_context_tokens_batch), 0) AS extra_context_tokens_total,
			COALESCE(SUM(provider_total_input_tokens_batch), 0) AS provider_input_tokens_total,
			COALESCE(SUM(output_tokens_batch), 0) AS output_tokens_total,
			COALESCE(SUM(input_cost_usd_batch), 0) AS input_cost_usd_total,
			COALESCE(SUM(output_cost_usd_batch), 0) AS output_cost_usd_total,
			COALESCE(SUM(total_cost_usd_batch), 0) AS total_cost_usd_total
		FROM quota_batch_settlements
		WHERE org_id = $1 AND idempotency_key = $2
	`

	var out QuotaBatchAggregate
	err := s.db.QueryRowContext(ctx, query, orgID, strings.TrimSpace(idempotencyKey)).Scan(
		&out.BatchCount,
		&out.PlanCode,
		&out.PricingVersion,
		&out.RawLOCTotal,
		&out.EffectiveLOCTotal,
		&out.ExtraEffectiveLOCTotal,
		&out.DiffInputTokensTotal,
		&out.ContextCharsTotal,
		&out.ContextTokensTotal,
		&out.AllowedContextTokensTotal,
		&out.ExtraContextTokensTotal,
		&out.ProviderInputTokensTotal,
		&out.OutputTokensTotal,
		&out.InputCostUSDTotal,
		&out.OutputCostUSDTotal,
		&out.TotalCostUSDTotal,
	)
	if err != nil {
		return QuotaBatchAggregate{}, fmt.Errorf("build aggregate from batches: %w", err)
	}
	if out.BatchCount <= 0 {
		return QuotaBatchAggregate{}, sql.ErrNoRows
	}
	return out, nil
}

func (s *QuotaStore) UpsertOperationAggregate(ctx context.Context, rec QuotaOperationAggregateRecord) error {
	if rec.FinalizedAt.IsZero() {
		rec.FinalizedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO quota_operation_aggregates (
			org_id,
			review_id,
			operation_type,
			trigger_source,
			operation_id,
			idempotency_key,
			plan_code,
			provider,
			model,
			pricing_version,
			batch_count,
			raw_loc_total,
			effective_loc_total,
			extra_effective_loc_total,
			diff_input_tokens_total,
			context_chars_total,
			context_tokens_total,
			allowed_context_tokens_total,
			extra_context_tokens_total,
			provider_total_input_tokens_total,
			output_tokens_total,
			input_cost_usd_total,
			output_cost_usd_total,
			total_cost_usd_total,
			finalized_at,
			created_at,
			updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,NOW(),NOW()
		)
		ON CONFLICT (org_id, idempotency_key)
		DO UPDATE SET
			review_id = EXCLUDED.review_id,
			operation_type = EXCLUDED.operation_type,
			trigger_source = EXCLUDED.trigger_source,
			operation_id = EXCLUDED.operation_id,
			plan_code = EXCLUDED.plan_code,
			provider = EXCLUDED.provider,
			model = EXCLUDED.model,
			pricing_version = EXCLUDED.pricing_version,
			batch_count = EXCLUDED.batch_count,
			raw_loc_total = EXCLUDED.raw_loc_total,
			effective_loc_total = EXCLUDED.effective_loc_total,
			extra_effective_loc_total = EXCLUDED.extra_effective_loc_total,
			diff_input_tokens_total = EXCLUDED.diff_input_tokens_total,
			context_chars_total = EXCLUDED.context_chars_total,
			context_tokens_total = EXCLUDED.context_tokens_total,
			allowed_context_tokens_total = EXCLUDED.allowed_context_tokens_total,
			extra_context_tokens_total = EXCLUDED.extra_context_tokens_total,
			provider_total_input_tokens_total = EXCLUDED.provider_total_input_tokens_total,
			output_tokens_total = EXCLUDED.output_tokens_total,
			input_cost_usd_total = EXCLUDED.input_cost_usd_total,
			output_cost_usd_total = EXCLUDED.output_cost_usd_total,
			total_cost_usd_total = EXCLUDED.total_cost_usd_total,
			finalized_at = EXCLUDED.finalized_at,
			updated_at = NOW()
	`,
		rec.OrgID,
		nullReviewID(rec.ReviewID),
		strings.TrimSpace(rec.OperationType),
		strings.TrimSpace(rec.TriggerSource),
		strings.TrimSpace(rec.OperationID),
		strings.TrimSpace(rec.IdempotencyKey),
		strings.TrimSpace(rec.PlanCode),
		nullIfEmpty(strings.TrimSpace(rec.Provider)),
		nullIfEmpty(strings.TrimSpace(rec.Model)),
		strings.TrimSpace(rec.PricingVersion),
		rec.BatchCount,
		rec.RawLOCTotal,
		rec.EffectiveLOCTotal,
		rec.ExtraEffectiveLOCTotal,
		rec.DiffInputTokensTotal,
		rec.ContextCharsTotal,
		rec.ContextTokensTotal,
		rec.AllowedContextTokensTotal,
		rec.ExtraContextTokensTotal,
		rec.ProviderInputTokensTotal,
		rec.OutputTokensTotal,
		rec.InputCostUSDTotal,
		rec.OutputCostUSDTotal,
		rec.TotalCostUSDTotal,
		rec.FinalizedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert quota operation aggregate: %w", err)
	}
	return nil
}
