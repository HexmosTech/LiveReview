package license

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	storagelicense "github.com/livereview/storage/license"
)

type LOCAccountSuccessInput struct {
	OrgID          int64
	ReviewID       int64
	ActorUserID    *int64
	ActorEmail     string
	OperationType  string
	TriggerSource  string
	OperationID    string
	IdempotencyKey string
	BillableLOC    int64
	PlanCode       PlanType
	Provider       string
	Model          string
	PricingVersion string
	InputTokens    *int64
	OutputTokens   *int64
	CostUSD        *float64
}

type LOCPreflightInput struct {
	OrgID       int64
	RequiredLOC int64
	PlanCode    PlanType
}

type LOCPreflightResult struct {
	PlanCode           PlanType
	BillingPeriodStart time.Time
	BillingPeriodEnd   time.Time
	LOCUsedMonth       int64
	LOCLimitMonth      int64
	LOCRemainingMonth  int64
	UsagePercent       int
	ThresholdState     string
	TrialReadOnly      bool
	TrialEndsAt        *time.Time
	BlockReason        string
	Blocked            bool
}

type LOCAccountingService struct {
	store *storagelicense.LOCAccountingStore
}

func NewLOCAccountingService(db *sql.DB) *LOCAccountingService {
	return &LOCAccountingService{store: storagelicense.NewLOCAccountingStore(db)}
}

func (s *LOCAccountingService) AccountSuccess(ctx context.Context, input LOCAccountSuccessInput) error {
	if input.OrgID <= 0 {
		return fmt.Errorf("org id must be > 0")
	}
	if input.ReviewID <= 0 {
		return fmt.Errorf("review id must be > 0")
	}
	if input.BillableLOC <= 0 {
		return nil
	}
	if input.OperationType == "" {
		return fmt.Errorf("operation type is required")
	}
	if input.TriggerSource == "" {
		return fmt.Errorf("trigger source is required")
	}
	if input.OperationID == "" {
		return fmt.Errorf("operation id is required")
	}
	if input.IdempotencyKey == "" {
		return fmt.Errorf("idempotency key is required")
	}

	planCode := input.PlanCode
	if planCode == "" {
		planCode = PlanFree30K
	}
	limits := planCode.GetLimits()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	return s.store.AccountSuccess(ctx, storagelicense.AccountSuccessRecord{
		OrgID:              input.OrgID,
		ReviewID:           input.ReviewID,
		ActorUserID:        input.ActorUserID,
		ActorEmail:         input.ActorEmail,
		OperationType:      input.OperationType,
		TriggerSource:      input.TriggerSource,
		OperationID:        input.OperationID,
		IdempotencyKey:     input.IdempotencyKey,
		BillableLOC:        input.BillableLOC,
		BillingPeriodStart: periodStart,
		BillingPeriodEnd:   periodEnd,
		PlanCode:           planCode.String(),
		MonthlyLOCLimit:    int64(limits.MonthlyLOCLimit),
		Provider:           input.Provider,
		Model:              input.Model,
		PricingVersion:     input.PricingVersion,
		InputTokens:        input.InputTokens,
		OutputTokens:       input.OutputTokens,
		CostUSD:            input.CostUSD,
	})
}

func (s *LOCAccountingService) CheckPreflight(ctx context.Context, input LOCPreflightInput) (LOCPreflightResult, error) {
	if input.OrgID <= 0 {
		return LOCPreflightResult{}, fmt.Errorf("org id must be > 0")
	}
	if input.RequiredLOC < 0 {
		return LOCPreflightResult{}, fmt.Errorf("required loc must be >= 0")
	}

	planCode := input.PlanCode
	if planCode == "" {
		planCode = PlanFree30K
	}
	limits := planCode.GetLimits()

	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	storeResult, err := s.store.CheckQuotaPreflight(
		ctx,
		input.OrgID,
		planCode.String(),
		int64(limits.MonthlyLOCLimit),
		input.RequiredLOC,
		periodStart,
		periodEnd,
	)
	if err != nil {
		return LOCPreflightResult{}, err
	}

	result := LOCPreflightResult{
		PlanCode:           planCode,
		BillingPeriodStart: storeResult.BillingPeriodStart,
		BillingPeriodEnd:   storeResult.BillingPeriodEnd,
		LOCUsedMonth:       storeResult.LOCUsedMonth,
		LOCLimitMonth:      storeResult.LOCLimitMonth,
		LOCRemainingMonth:  storeResult.LOCRemainingMonth,
		UsagePercent:       storeResult.UsagePercent,
		TrialReadOnly:      storeResult.TrialReadOnly,
		TrialEndsAt:        storeResult.TrialEndsAt,
		Blocked:            storeResult.Blocked,
	}

	result.ThresholdState = "normal"
	result.BlockReason = "quota_exceeded"
	if result.TrialReadOnly {
		result.ThresholdState = "trial_readonly"
		result.BlockReason = "trial_readonly"
		result.Blocked = true
		return result, nil
	}
	if result.UsagePercent >= 100 {
		result.ThresholdState = "100"
	} else if result.UsagePercent >= 90 {
		result.ThresholdState = "90"
	} else if result.UsagePercent >= 80 {
		result.ThresholdState = "80"
	}

	return result, nil
}
