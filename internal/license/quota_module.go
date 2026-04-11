package license

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	storagelicense "github.com/livereview/storage/license"
)

type QuotaModule struct {
	accountingService *LOCAccountingService
	quotaStore        *storagelicense.QuotaStore
}

type QuotaPreflightInput struct {
	OrgID       int64
	RequiredLOC int64
	PlanCode    PlanType
}

type QuotaPolicySnapshot struct {
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

type QuotaBatchInput struct {
	PlanCode                 PlanType
	Provider                 string
	RawLOCBatch              int64
	ContextCharsBatch        *int64
	ContextTokensBatch       *int64
	ProviderTotalInputTokens *int64
	OutputTokensBatch        *int64
	Policy                   *QuotaPolicySnapshot
}

type QuotaBatchSettlement struct {
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
}

type QuotaRecordBatchInput struct {
	OrgID          int64
	ReviewID       *int64
	OperationType  string
	TriggerSource  string
	OperationID    string
	IdempotencyKey string
	BatchIndex     int64
	Batch          QuotaBatchInput
}

type QuotaFinalizeResult struct {
	PlanCode                  string
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
}

type QuotaFinalizeInput struct {
	OrgID          int64
	ReviewID       *int64
	ActorUserID    *int64
	ActorEmail     string
	OperationType  string
	TriggerSource  string
	OperationID    string
	IdempotencyKey string
	Provider       string
	Model          string
	BatchFallback  *QuotaBatchInput
}

func NewQuotaModule(db *sql.DB) *QuotaModule {
	if db == nil {
		return &QuotaModule{}
	}
	return &QuotaModule{
		accountingService: NewLOCAccountingService(db),
		quotaStore:        storagelicense.NewQuotaStore(db),
	}
}

func (m *QuotaModule) PreflightCheck(ctx context.Context, input QuotaPreflightInput) (LOCPreflightResult, error) {
	if m == nil || m.accountingService == nil {
		return LOCPreflightResult{}, fmt.Errorf("quota module is not initialized")
	}

	return m.accountingService.CheckPreflight(ctx, LOCPreflightInput{
		OrgID:       input.OrgID,
		RequiredLOC: input.RequiredLOC,
		PlanCode:    input.PlanCode,
	})
}

func (m *QuotaModule) BuildBatchSettlement(input QuotaBatchInput) (QuotaBatchSettlement, error) {
	if input.RawLOCBatch < 0 {
		return QuotaBatchSettlement{}, fmt.Errorf("raw LOC batch must be >= 0")
	}

	policy, err := m.resolvePolicy(context.Background(), input)
	if err != nil {
		return QuotaBatchSettlement{}, err
	}

	if policy.MonthlyLOCLimit < 0 {
		return QuotaBatchSettlement{}, fmt.Errorf("plan %s has no finite LOC limit", policy.PlanCode)
	}
	if policy.InputCharsPerLOC <= 0 || policy.CharsPerToken <= 0 {
		return QuotaBatchSettlement{}, fmt.Errorf("invalid token conversion policy for plan=%s provider=%s", policy.PlanCode, policy.ProviderKey)
	}

	inputRatePerTokenUSD := policy.InputCostPerMillionTokensUSD / 1_000_000.0
	outputRatePerTokenUSD := policy.OutputCostPerMillionTokensUSD / 1_000_000.0

	contextBudgetUSD := float64(policy.MonthlyPriceUSD) * policy.ContextBudgetRatio
	contextTokensPerLOCAllowance := 0.0
	if policy.MonthlyLOCLimit > 0 && inputRatePerTokenUSD > 0 {
		contextTotalTokens := contextBudgetUSD / inputRatePerTokenUSD
		contextTokensPerLOCAllowance = contextTotalTokens / float64(policy.MonthlyLOCLimit)
	}

	diffInputTokens := input.RawLOCBatch * (policy.InputCharsPerLOC / policy.CharsPerToken)

	providerInputTokens := int64(0)
	if input.ProviderTotalInputTokens != nil {
		providerInputTokens = *input.ProviderTotalInputTokens
	}

	contextTokens := int64(0)
	if input.ContextTokensBatch != nil {
		contextTokens = *input.ContextTokensBatch
	} else if providerInputTokens > diffInputTokens {
		contextTokens = providerInputTokens - diffInputTokens
	}

	contextChars := int64(0)
	if input.ContextCharsBatch != nil {
		contextChars = *input.ContextCharsBatch
	} else {
		contextChars = contextTokens * policy.CharsPerToken
	}

	allowedContextTokens := int64(0)
	if contextTokensPerLOCAllowance > 0 && input.RawLOCBatch > 0 {
		allowedContextTokens = int64(math.Floor(float64(input.RawLOCBatch) * contextTokensPerLOCAllowance))
	}

	extraContextTokens := int64(0)
	if contextTokens > allowedContextTokens {
		extraContextTokens = contextTokens - allowedContextTokens
	}

	extraEffectiveLOC := int64(0)
	if extraContextTokens > 0 && contextTokensPerLOCAllowance > 0 {
		extraEffectiveLOC = int64(math.Ceil(float64(extraContextTokens) / contextTokensPerLOCAllowance))
	}
	effectiveLOC := input.RawLOCBatch + extraEffectiveLOC

	outputTokens := int64(0)
	if input.OutputTokensBatch != nil {
		outputTokens = *input.OutputTokensBatch
	}

	inputCostUSD := roundAt(float64(diffInputTokens+contextTokens)*inputRatePerTokenUSD, policy.RoundingScale)
	outputCostUSD := roundAt(float64(outputTokens)*outputRatePerTokenUSD, policy.RoundingScale)
	totalCostUSD := roundAt(inputCostUSD+outputCostUSD, policy.RoundingScale)

	return QuotaBatchSettlement{
		PlanCode:                     policy.PlanCode,
		PolicyProviderKey:            policy.ProviderKey,
		PricingVersion:               "quota_v1_deterministic_diff",
		RawLOCBatch:                  input.RawLOCBatch,
		EffectiveLOCBatch:            effectiveLOC,
		ExtraEffectiveLOCBatch:       extraEffectiveLOC,
		DiffInputTokensBatch:         diffInputTokens,
		ContextCharsBatch:            contextChars,
		ContextTokensBatch:           contextTokens,
		AllowedContextTokensBatch:    allowedContextTokens,
		ExtraContextTokensBatch:      extraContextTokens,
		ProviderInputTokensBatch:     providerInputTokens,
		OutputTokensBatch:            outputTokens,
		InputCostUSDBatch:            inputCostUSD,
		OutputCostUSDBatch:           outputCostUSD,
		TotalCostUSDBatch:            totalCostUSD,
		ContextTokensPerLOCAllowance: contextTokensPerLOCAllowance,
	}, nil
}

func (m *QuotaModule) RecordBatch(ctx context.Context, input QuotaRecordBatchInput) (QuotaBatchSettlement, error) {
	if m == nil || m.quotaStore == nil {
		return QuotaBatchSettlement{}, fmt.Errorf("quota module is not initialized")
	}
	if input.OrgID <= 0 {
		return QuotaBatchSettlement{}, fmt.Errorf("org id must be > 0")
	}
	if strings.TrimSpace(input.OperationType) == "" {
		return QuotaBatchSettlement{}, fmt.Errorf("operation type is required")
	}
	if strings.TrimSpace(input.TriggerSource) == "" {
		return QuotaBatchSettlement{}, fmt.Errorf("trigger source is required")
	}
	if strings.TrimSpace(input.OperationID) == "" {
		return QuotaBatchSettlement{}, fmt.Errorf("operation id is required")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return QuotaBatchSettlement{}, fmt.Errorf("idempotency key is required")
	}
	batchIndex := input.BatchIndex
	if batchIndex <= 0 {
		batchIndex = 1
	}

	settlement, err := m.BuildBatchSettlement(input.Batch)
	if err != nil {
		return QuotaBatchSettlement{}, err
	}

	err = m.quotaStore.UpsertBatchSettlement(ctx, storagelicense.QuotaBatchSettlementRecord{
		OrgID:                        input.OrgID,
		ReviewID:                     input.ReviewID,
		OperationType:                strings.TrimSpace(input.OperationType),
		TriggerSource:                strings.TrimSpace(input.TriggerSource),
		OperationID:                  strings.TrimSpace(input.OperationID),
		IdempotencyKey:               strings.TrimSpace(input.IdempotencyKey),
		BatchIndex:                   batchIndex,
		PlanCode:                     settlement.PlanCode,
		PolicyProviderKey:            settlement.PolicyProviderKey,
		PricingVersion:               settlement.PricingVersion,
		RawLOCBatch:                  settlement.RawLOCBatch,
		EffectiveLOCBatch:            settlement.EffectiveLOCBatch,
		ExtraEffectiveLOCBatch:       settlement.ExtraEffectiveLOCBatch,
		DiffInputTokensBatch:         settlement.DiffInputTokensBatch,
		ContextCharsBatch:            settlement.ContextCharsBatch,
		ContextTokensBatch:           settlement.ContextTokensBatch,
		AllowedContextTokensBatch:    settlement.AllowedContextTokensBatch,
		ExtraContextTokensBatch:      settlement.ExtraContextTokensBatch,
		ProviderInputTokensBatch:     settlement.ProviderInputTokensBatch,
		OutputTokensBatch:            settlement.OutputTokensBatch,
		InputCostUSDBatch:            settlement.InputCostUSDBatch,
		OutputCostUSDBatch:           settlement.OutputCostUSDBatch,
		TotalCostUSDBatch:            settlement.TotalCostUSDBatch,
		ContextTokensPerLOCAllowance: settlement.ContextTokensPerLOCAllowance,
	})
	if err != nil {
		return QuotaBatchSettlement{}, err
	}

	return settlement, nil
}

func (m *QuotaModule) FinalizeOperation(ctx context.Context, input QuotaFinalizeInput) (QuotaFinalizeResult, error) {
	if m == nil || m.accountingService == nil || m.quotaStore == nil {
		return QuotaFinalizeResult{}, fmt.Errorf("quota module is not initialized")
	}
	if input.OrgID <= 0 {
		return QuotaFinalizeResult{}, fmt.Errorf("org id must be > 0")
	}
	if strings.TrimSpace(input.OperationType) == "" {
		return QuotaFinalizeResult{}, fmt.Errorf("operation type is required")
	}
	if strings.TrimSpace(input.TriggerSource) == "" {
		return QuotaFinalizeResult{}, fmt.Errorf("trigger source is required")
	}
	if strings.TrimSpace(input.OperationID) == "" {
		return QuotaFinalizeResult{}, fmt.Errorf("operation id is required")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return QuotaFinalizeResult{}, fmt.Errorf("idempotency key is required")
	}

	aggregate, err := m.quotaStore.BuildAggregateFromBatches(ctx, input.OrgID, strings.TrimSpace(input.IdempotencyKey))
	if err == sql.ErrNoRows && input.BatchFallback != nil {
		_, recordErr := m.RecordBatch(ctx, QuotaRecordBatchInput{
			OrgID:          input.OrgID,
			ReviewID:       input.ReviewID,
			OperationType:  input.OperationType,
			TriggerSource:  input.TriggerSource,
			OperationID:    input.OperationID,
			IdempotencyKey: input.IdempotencyKey,
			BatchIndex:     1,
			Batch:          *input.BatchFallback,
		})
		if recordErr != nil {
			return QuotaFinalizeResult{}, recordErr
		}
		aggregate, err = m.quotaStore.BuildAggregateFromBatches(ctx, input.OrgID, strings.TrimSpace(input.IdempotencyKey))
	}
	if err != nil {
		return QuotaFinalizeResult{}, err
	}

	inputTokens := aggregate.DiffInputTokensTotal + aggregate.ContextTokensTotal
	outputTokens := aggregate.OutputTokensTotal
	costUSD := aggregate.TotalCostUSDTotal

	err = m.accountingService.AccountSuccess(ctx, LOCAccountSuccessInput{
		OrgID:          input.OrgID,
		ReviewID:       input.ReviewID,
		ActorUserID:    input.ActorUserID,
		ActorEmail:     strings.TrimSpace(input.ActorEmail),
		OperationType:  strings.TrimSpace(input.OperationType),
		TriggerSource:  strings.TrimSpace(input.TriggerSource),
		OperationID:    strings.TrimSpace(input.OperationID),
		IdempotencyKey: strings.TrimSpace(input.IdempotencyKey),
		BillableLOC:    aggregate.EffectiveLOCTotal,
		PlanCode:       PlanType(aggregate.PlanCode),
		Provider:       strings.TrimSpace(input.Provider),
		Model:          strings.TrimSpace(input.Model),
		PricingVersion: aggregate.PricingVersion,
		InputTokens:    &inputTokens,
		OutputTokens:   &outputTokens,
		CostUSD:        &costUSD,
	})
	if err != nil {
		return QuotaFinalizeResult{}, err
	}

	err = m.quotaStore.UpsertOperationAggregate(ctx, storagelicense.QuotaOperationAggregateRecord{
		OrgID:                     input.OrgID,
		ReviewID:                  input.ReviewID,
		OperationType:             strings.TrimSpace(input.OperationType),
		TriggerSource:             strings.TrimSpace(input.TriggerSource),
		OperationID:               strings.TrimSpace(input.OperationID),
		IdempotencyKey:            strings.TrimSpace(input.IdempotencyKey),
		PlanCode:                  aggregate.PlanCode,
		Provider:                  strings.TrimSpace(input.Provider),
		Model:                     strings.TrimSpace(input.Model),
		PricingVersion:            aggregate.PricingVersion,
		BatchCount:                aggregate.BatchCount,
		RawLOCTotal:               aggregate.RawLOCTotal,
		EffectiveLOCTotal:         aggregate.EffectiveLOCTotal,
		ExtraEffectiveLOCTotal:    aggregate.ExtraEffectiveLOCTotal,
		DiffInputTokensTotal:      aggregate.DiffInputTokensTotal,
		ContextCharsTotal:         aggregate.ContextCharsTotal,
		ContextTokensTotal:        aggregate.ContextTokensTotal,
		AllowedContextTokensTotal: aggregate.AllowedContextTokensTotal,
		ExtraContextTokensTotal:   aggregate.ExtraContextTokensTotal,
		ProviderInputTokensTotal:  aggregate.ProviderInputTokensTotal,
		OutputTokensTotal:         aggregate.OutputTokensTotal,
		InputCostUSDTotal:         aggregate.InputCostUSDTotal,
		OutputCostUSDTotal:        aggregate.OutputCostUSDTotal,
		TotalCostUSDTotal:         aggregate.TotalCostUSDTotal,
	})
	if err != nil {
		return QuotaFinalizeResult{}, err
	}

	return QuotaFinalizeResult{
		PlanCode:                  aggregate.PlanCode,
		PricingVersion:            aggregate.PricingVersion,
		BatchCount:                aggregate.BatchCount,
		RawLOCTotal:               aggregate.RawLOCTotal,
		EffectiveLOCTotal:         aggregate.EffectiveLOCTotal,
		ExtraEffectiveLOCTotal:    aggregate.ExtraEffectiveLOCTotal,
		DiffInputTokensTotal:      aggregate.DiffInputTokensTotal,
		ContextCharsTotal:         aggregate.ContextCharsTotal,
		ContextTokensTotal:        aggregate.ContextTokensTotal,
		AllowedContextTokensTotal: aggregate.AllowedContextTokensTotal,
		ExtraContextTokensTotal:   aggregate.ExtraContextTokensTotal,
		ProviderInputTokensTotal:  aggregate.ProviderInputTokensTotal,
		OutputTokensTotal:         aggregate.OutputTokensTotal,
		InputCostUSDTotal:         aggregate.InputCostUSDTotal,
		OutputCostUSDTotal:        aggregate.OutputCostUSDTotal,
		TotalCostUSDTotal:         aggregate.TotalCostUSDTotal,
	}, nil
}

func (m *QuotaModule) resolvePolicy(ctx context.Context, input QuotaBatchInput) (QuotaPolicySnapshot, error) {
	if input.Policy != nil {
		return *input.Policy, nil
	}
	if m == nil || m.quotaStore == nil {
		return QuotaPolicySnapshot{}, fmt.Errorf("quota module policy store is not initialized")
	}

	planCode := strings.TrimSpace(input.PlanCode.String())
	if planCode == "" {
		return QuotaPolicySnapshot{}, fmt.Errorf("plan code is required for quota policy resolution")
	}

	resolved, err := m.quotaStore.ResolvePolicy(ctx, planCode, input.Provider)
	if err != nil {
		return QuotaPolicySnapshot{}, err
	}

	return QuotaPolicySnapshot{
		PlanCode:                      resolved.PlanCode,
		ProviderKey:                   resolved.ProviderKey,
		InputCharsPerLOC:              resolved.InputCharsPerLOC,
		OutputCharsPerLOC:             resolved.OutputCharsPerLOC,
		CharsPerToken:                 resolved.CharsPerToken,
		LOCBudgetRatio:                resolved.LOCBudgetRatio,
		ContextBudgetRatio:            resolved.ContextBudgetRatio,
		OpsReservedRatio:              resolved.OpsReservedRatio,
		InputCostPerMillionTokensUSD:  resolved.InputCostPerMillionTokensUSD,
		OutputCostPerMillionTokensUSD: resolved.OutputCostPerMillionTokensUSD,
		RoundingScale:                 resolved.RoundingScale,
		MonthlyPriceUSD:               resolved.MonthlyPriceUSD,
		MonthlyLOCLimit:               resolved.MonthlyLOCLimit,
	}, nil
}

func roundAt(value float64, scale int64) float64 {
	if scale < 0 {
		scale = 0
	}
	factor := math.Pow10(int(scale))
	return math.Round(value*factor) / factor
}
