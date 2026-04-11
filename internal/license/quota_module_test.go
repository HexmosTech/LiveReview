package license

import "testing"

func testPolicy(planCode string, monthlyPriceUSD, monthlyLOCLimit int64) *QuotaPolicySnapshot {
	return &QuotaPolicySnapshot{
		PlanCode:                      planCode,
		ProviderKey:                   "gemini",
		InputCharsPerLOC:              120,
		OutputCharsPerLOC:             87,
		CharsPerToken:                 4,
		LOCBudgetRatio:                1.0 / 3.0,
		ContextBudgetRatio:            1.0 / 3.0,
		OpsReservedRatio:              1.0 / 3.0,
		InputCostPerMillionTokensUSD:  0.3,
		OutputCostPerMillionTokensUSD: 2.5,
		RoundingScale:                 6,
		MonthlyPriceUSD:               monthlyPriceUSD,
		MonthlyLOCLimit:               monthlyLOCLimit,
	}
}

func TestQuotaModuleBuildBatchSettlement_DeterministicDiffTokens(t *testing.T) {
	m := &QuotaModule{}
	providerInput := int64(1200)
	outputTokens := int64(100)
	result, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:                 PlanTeam32USD,
		Provider:                 "gemini",
		RawLOCBatch:              10,
		ProviderTotalInputTokens: &providerInput,
		OutputTokensBatch:        &outputTokens,
		Policy:                   testPolicy(PlanTeam32USD.String(), 32, 100000),
	})
	if err != nil {
		t.Fatalf("BuildBatchSettlement returned error: %v", err)
	}

	if result.DiffInputTokensBatch != 300 {
		t.Fatalf("expected deterministic diff tokens 300, got %d", result.DiffInputTokensBatch)
	}
	if result.ContextTokensBatch != 900 {
		t.Fatalf("expected derived context tokens 900, got %d", result.ContextTokensBatch)
	}
	if result.EffectiveLOCBatch < result.RawLOCBatch {
		t.Fatalf("expected effective LOC >= raw LOC, got raw=%d effective=%d", result.RawLOCBatch, result.EffectiveLOCBatch)
	}
}

func TestQuotaModuleBuildBatchSettlement_ContextOverrunAddsEffectiveLOC(t *testing.T) {
	m := &QuotaModule{}
	contextTokens := int64(10000)
	outputTokens := int64(2000)

	result, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:           PlanTeam32USD,
		Provider:           "gemini",
		RawLOCBatch:        10,
		ContextTokensBatch: &contextTokens,
		OutputTokensBatch:  &outputTokens,
		Policy:             testPolicy(PlanTeam32USD.String(), 32, 100000),
	})
	if err != nil {
		t.Fatalf("BuildBatchSettlement returned error: %v", err)
	}

	if result.ExtraContextTokensBatch <= 0 {
		t.Fatalf("expected context overrun, got extra context tokens=%d", result.ExtraContextTokensBatch)
	}
	if result.ExtraEffectiveLOCBatch <= 0 {
		t.Fatalf("expected extra effective LOC > 0, got %d", result.ExtraEffectiveLOCBatch)
	}
	if result.EffectiveLOCBatch != result.RawLOCBatch+result.ExtraEffectiveLOCBatch {
		t.Fatalf("expected effective LOC to equal raw + extra, got raw=%d extra=%d effective=%d", result.RawLOCBatch, result.ExtraEffectiveLOCBatch, result.EffectiveLOCBatch)
	}

	if result.InputCostUSDBatch != 0.00309 {
		t.Fatalf("expected input cost 0.00309, got %f", result.InputCostUSDBatch)
	}
	if result.OutputCostUSDBatch != 0.005 {
		t.Fatalf("expected output cost 0.005, got %f", result.OutputCostUSDBatch)
	}
	if result.TotalCostUSDBatch != 0.00809 {
		t.Fatalf("expected total cost 0.00809, got %f", result.TotalCostUSDBatch)
	}
}

func TestQuotaModuleBuildBatchSettlement_FreePlanHasNoContextAllowance(t *testing.T) {
	m := &QuotaModule{}
	contextTokens := int64(500)
	result, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:           PlanFree30K,
		Provider:           "gemini",
		RawLOCBatch:        5,
		ContextTokensBatch: &contextTokens,
		Policy:             testPolicy(PlanFree30K.String(), 0, 30000),
	})
	if err != nil {
		t.Fatalf("BuildBatchSettlement returned error: %v", err)
	}

	if result.ContextTokensPerLOCAllowance != 0 {
		t.Fatalf("expected no context allowance for free plan, got %f", result.ContextTokensPerLOCAllowance)
	}
	if result.ExtraEffectiveLOCBatch != 0 {
		t.Fatalf("expected no extra effective LOC when allowance is disabled, got %d", result.ExtraEffectiveLOCBatch)
	}
	if result.EffectiveLOCBatch != 5 {
		t.Fatalf("expected effective LOC to equal raw LOC, got %d", result.EffectiveLOCBatch)
	}
}

func TestQuotaModuleBuildBatchSettlement_RejectsNegativeRawLOC(t *testing.T) {
	m := &QuotaModule{}
	_, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:    PlanTeam32USD,
		Provider:    "gemini",
		RawLOCBatch: -1,
		Policy:      testPolicy(PlanTeam32USD.String(), 32, 100000),
	})
	if err == nil {
		t.Fatalf("expected error for negative raw LOC")
	}
}
