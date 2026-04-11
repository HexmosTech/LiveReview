package license

import (
	"math"
	"math/rand"
	"testing"
)

func TestQuotaModuleSimulation_InvariantsHold(t *testing.T) {
	t.Parallel()

	m := &QuotaModule{}
	policy := testPolicy(PlanTeam32USD.String(), 32, 100000)
	rng := rand.New(rand.NewSource(1337))

	var cumulativeRaw int64
	var cumulativeEffective int64
	var cumulativeExtra int64

	for i := 0; i < 2000; i++ {
		rawLOC := int64(rng.Intn(300) + 1)
		diffTokens := rawLOC * (policy.InputCharsPerLOC / policy.CharsPerToken)

		// Generate workloads that include both normal and pathological context growth.
		baseContext := int64(rng.Intn(2000))
		if i%17 == 0 {
			baseContext += int64(rng.Intn(8000))
		}
		providerInput := diffTokens + baseContext
		outputTokens := int64(rng.Intn(3000))

		result, err := m.BuildBatchSettlement(QuotaBatchInput{
			PlanCode:                 PlanTeam32USD,
			Provider:                 "gemini",
			RawLOCBatch:              rawLOC,
			ProviderTotalInputTokens: &providerInput,
			OutputTokensBatch:        &outputTokens,
			Policy:                   policy,
		})
		if err != nil {
			t.Fatalf("simulation iteration %d failed: %v", i, err)
		}

		if result.EffectiveLOCBatch < result.RawLOCBatch {
			t.Fatalf("iteration %d: effective LOC < raw LOC (%d < %d)", i, result.EffectiveLOCBatch, result.RawLOCBatch)
		}
		if result.ExtraEffectiveLOCBatch != result.EffectiveLOCBatch-result.RawLOCBatch {
			t.Fatalf("iteration %d: extra LOC mismatch", i)
		}
		if result.ContextTokensBatch <= result.AllowedContextTokensBatch && result.ExtraEffectiveLOCBatch != 0 {
			t.Fatalf("iteration %d: extra effective LOC should be zero when context within allowance", i)
		}
		if result.ContextTokensBatch > result.AllowedContextTokensBatch && result.ExtraEffectiveLOCBatch == 0 {
			t.Fatalf("iteration %d: expected extra effective LOC for over-allowance context", i)
		}
		if math.Abs(result.TotalCostUSDBatch-(result.InputCostUSDBatch+result.OutputCostUSDBatch)) > 1e-9 {
			t.Fatalf("iteration %d: total cost mismatch", i)
		}

		cumulativeRaw += result.RawLOCBatch
		cumulativeEffective += result.EffectiveLOCBatch
		cumulativeExtra += result.ExtraEffectiveLOCBatch
	}

	if cumulativeEffective < cumulativeRaw {
		t.Fatalf("cumulative effective LOC must be >= cumulative raw LOC")
	}
	if cumulativeExtra != cumulativeEffective-cumulativeRaw {
		t.Fatalf("cumulative extra LOC mismatch")
	}
}

func TestQuotaModuleSimulation_LinearPlanScaling(t *testing.T) {
	t.Parallel()

	m := &QuotaModule{}
	rawLOC := int64(200)
	contextTokens := int64(5000)
	outputTokens := int64(1500)

	policy1x := testPolicy(PlanTeam32USD.String(), 32, 100000)
	policy2x := testPolicy(PlanLOC200K.String(), 64, 200000)
	policy4x := testPolicy(PlanLOC400K.String(), 128, 400000)

	result1x, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:           PlanTeam32USD,
		Provider:           "gemini",
		RawLOCBatch:        rawLOC,
		ContextTokensBatch: &contextTokens,
		OutputTokensBatch:  &outputTokens,
		Policy:             policy1x,
	})
	if err != nil {
		t.Fatalf("1x policy settlement failed: %v", err)
	}

	result2x, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:           PlanLOC200K,
		Provider:           "gemini",
		RawLOCBatch:        rawLOC,
		ContextTokensBatch: &contextTokens,
		OutputTokensBatch:  &outputTokens,
		Policy:             policy2x,
	})
	if err != nil {
		t.Fatalf("2x policy settlement failed: %v", err)
	}

	result4x, err := m.BuildBatchSettlement(QuotaBatchInput{
		PlanCode:           PlanLOC400K,
		Provider:           "gemini",
		RawLOCBatch:        rawLOC,
		ContextTokensBatch: &contextTokens,
		OutputTokensBatch:  &outputTokens,
		Policy:             policy4x,
	})
	if err != nil {
		t.Fatalf("4x policy settlement failed: %v", err)
	}

	if result1x.ContextTokensPerLOCAllowance != result2x.ContextTokensPerLOCAllowance {
		t.Fatalf("expected linear scaling to preserve context allowance per LOC: 1x=%f 2x=%f", result1x.ContextTokensPerLOCAllowance, result2x.ContextTokensPerLOCAllowance)
	}
	if result2x.ContextTokensPerLOCAllowance != result4x.ContextTokensPerLOCAllowance {
		t.Fatalf("expected linear scaling to preserve context allowance per LOC: 2x=%f 4x=%f", result2x.ContextTokensPerLOCAllowance, result4x.ContextTokensPerLOCAllowance)
	}
}
