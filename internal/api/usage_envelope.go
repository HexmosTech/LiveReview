package api

import (
	"github.com/labstack/echo/v4"
	apimiddleware "github.com/livereview/internal/api/middleware"
)

const EnvelopeVersionV1 = "v1"

const (
	EnvelopeOperationTypeContextKey        = "envelope_operation_type"
	EnvelopeTriggerSourceContextKey        = "envelope_trigger_source"
	EnvelopeOperationBillableLOCContextKey = "envelope_operation_billable_loc"
	EnvelopeOperationIDContextKey          = "envelope_operation_id"
	EnvelopeIdempotencyKeyContextKey       = "envelope_idempotency_key"
	EnvelopeAccountedAtContextKey          = "envelope_accounted_at"
	EnvelopeLOCUsedMonthContextKey         = "envelope_loc_used_month"
	EnvelopeLOCRemainMonthContextKey       = "envelope_loc_remaining_month"
	EnvelopeUsagePercentContextKey         = "envelope_usage_percent"
	EnvelopeBillingPeriodStartContextKey   = "envelope_billing_period_start"
	EnvelopeBillingPeriodEndContextKey     = "envelope_billing_period_end"
	EnvelopeResetAtContextKey              = "envelope_reset_at"
	EnvelopeThresholdStateContextKey       = "envelope_threshold_state"
	EnvelopeBlockedContextKey              = "envelope_blocked"
	EnvelopeTrialReadOnlyContextKey        = "envelope_trial_readonly"
)

// PlanUsageEnvelope is the standardized payload for plan and usage transparency.
// The economic fields can be hidden by policy in future phases.
type PlanUsageEnvelope struct {
	EnvelopeVersion string `json:"envelope_version"`

	PlanCode       string `json:"plan_code"`
	PlanName       string `json:"plan_name"`
	PlanRank       int    `json:"plan_rank"`
	PriceUSD       *int   `json:"price_usd,omitempty"`
	LOCLimitMonth  *int64 `json:"loc_limit_month,omitempty"`
	LOCUsedMonth   *int64 `json:"loc_used_month,omitempty"`
	LOCRemainMonth *int64 `json:"loc_remaining_month,omitempty"`
	UsagePercent   *int   `json:"usage_percent,omitempty"`

	BillingPeriodStart string `json:"billing_period_start,omitempty"`
	BillingPeriodEnd   string `json:"billing_period_end,omitempty"`
	ResetAt            string `json:"reset_at,omitempty"`

	ThresholdState string `json:"threshold_state,omitempty"`
	Blocked        bool   `json:"blocked"`
	TrialReadOnly  bool   `json:"trial_readonly"`

	OperationType        string `json:"operation_type,omitempty"`
	TriggerSource        string `json:"trigger_source,omitempty"`
	OperationBillableLOC *int64 `json:"operation_billable_loc,omitempty"`
	OperationID          string `json:"operation_id,omitempty"`
	IdempotencyKey       string `json:"idempotency_key,omitempty"`
	AccountedAt          string `json:"accounted_at,omitempty"`

	UpgradeURL string `json:"upgrade_url,omitempty"`
}

func NewPlanUsageEnvelope(planCode string) PlanUsageEnvelope {
	return PlanUsageEnvelope{
		EnvelopeVersion: EnvelopeVersionV1,
		PlanCode:        planCode,
	}
}

// BuildEnvelopeFromContext returns a best-effort envelope using plan metadata already
// attached to request context by middleware.
func BuildEnvelopeFromContext(c echo.Context) PlanUsageEnvelope {
	envelope := NewPlanUsageEnvelope("free")

	planCtx, ok := c.Get(apimiddleware.PlanContextKey).(apimiddleware.PlanContext)
	if !ok {
		return envelope
	}

	envelope.PlanCode = planCtx.PlanType.String()
	envelope.PlanName = planCtx.PlanType.String()
	price := planCtx.Limits.MonthlyPriceUSD
	envelope.PriceUSD = &price

	if planCtx.Limits.MonthlyLOCLimit >= 0 {
		limit := int64(planCtx.Limits.MonthlyLOCLimit)
		envelope.LOCLimitMonth = &limit
	}

	if operationType, ok := c.Get(EnvelopeOperationTypeContextKey).(string); ok {
		envelope.OperationType = operationType
	}
	if triggerSource, ok := c.Get(EnvelopeTriggerSourceContextKey).(string); ok {
		envelope.TriggerSource = triggerSource
	}
	switch v := c.Get(EnvelopeOperationBillableLOCContextKey).(type) {
	case int64:
		envelope.OperationBillableLOC = &v
	case int:
		value := int64(v)
		envelope.OperationBillableLOC = &value
	}
	if operationID, ok := c.Get(EnvelopeOperationIDContextKey).(string); ok {
		envelope.OperationID = operationID
	}
	if idempotencyKey, ok := c.Get(EnvelopeIdempotencyKeyContextKey).(string); ok {
		envelope.IdempotencyKey = idempotencyKey
	}
	if accountedAt, ok := c.Get(EnvelopeAccountedAtContextKey).(string); ok {
		envelope.AccountedAt = accountedAt
	}
	switch v := c.Get(EnvelopeLOCUsedMonthContextKey).(type) {
	case int64:
		envelope.LOCUsedMonth = &v
	case int:
		value := int64(v)
		envelope.LOCUsedMonth = &value
	}
	switch v := c.Get(EnvelopeLOCRemainMonthContextKey).(type) {
	case int64:
		envelope.LOCRemainMonth = &v
	case int:
		value := int64(v)
		envelope.LOCRemainMonth = &value
	}
	if usagePercent, ok := c.Get(EnvelopeUsagePercentContextKey).(int); ok {
		envelope.UsagePercent = &usagePercent
	}
	if periodStart, ok := c.Get(EnvelopeBillingPeriodStartContextKey).(string); ok {
		envelope.BillingPeriodStart = periodStart
	}
	if periodEnd, ok := c.Get(EnvelopeBillingPeriodEndContextKey).(string); ok {
		envelope.BillingPeriodEnd = periodEnd
	}
	if resetAt, ok := c.Get(EnvelopeResetAtContextKey).(string); ok {
		envelope.ResetAt = resetAt
	}
	if thresholdState, ok := c.Get(EnvelopeThresholdStateContextKey).(string); ok {
		envelope.ThresholdState = thresholdState
	}
	if blocked, ok := c.Get(EnvelopeBlockedContextKey).(bool); ok {
		envelope.Blocked = blocked
	}
	if trialReadOnly, ok := c.Get(EnvelopeTrialReadOnlyContextKey).(bool); ok {
		envelope.TrialReadOnly = trialReadOnly
	}

	return envelope
}

// JSONWithEnvelope returns a JSON payload with envelope attached unless caller
// already provided one.
func JSONWithEnvelope(c echo.Context, code int, payload map[string]interface{}) error {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if _, exists := payload["envelope"]; !exists {
		payload["envelope"] = BuildEnvelopeFromContext(c)
	}
	return c.JSON(code, payload)
}

// JSONErrorWithEnvelope standardizes envelope-aware error payloads.
func JSONErrorWithEnvelope(c echo.Context, code int, message string) error {
	return JSONWithEnvelope(c, code, map[string]interface{}{"error": message})
}
