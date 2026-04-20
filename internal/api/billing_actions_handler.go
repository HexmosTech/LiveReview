package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/license/payment"
	storagelicense "github.com/livereview/storage/license"
	storagepayment "github.com/livereview/storage/payment"
)

type BillingActionsHandler struct {
	store               *storagelicense.PlanChangeStore
	usageStore          *storagelicense.OrgUsageStore
	portfolioStore      *storagelicense.AdminBillingPortfolioStore
	notificationStore   *storagepayment.BillingNotificationOutboxStore
	paymentAttemptStore *storagepayment.UpgradePaymentAttemptStore
	upgradeRequestStore *storagepayment.UpgradeRequestStore
	replacementStore    *storagepayment.UpgradeReplacementCutoverStore
	db                  *sql.DB
}

var errRazorpayCheckoutRequired = errors.New("razorpay checkout required")

func NewBillingActionsHandler(db *sql.DB) *BillingActionsHandler {
	return &BillingActionsHandler{
		store:               storagelicense.NewPlanChangeStore(db),
		usageStore:          storagelicense.NewOrgUsageStore(db),
		portfolioStore:      storagelicense.NewAdminBillingPortfolioStore(db),
		notificationStore:   storagepayment.NewBillingNotificationOutboxStore(db),
		paymentAttemptStore: storagepayment.NewUpgradePaymentAttemptStore(db),
		upgradeRequestStore: storagepayment.NewUpgradeRequestStore(db),
		replacementStore:    storagepayment.NewUpgradeReplacementCutoverStore(db),
		db:                  db,
	}
}

type planChangeRequest struct {
	TargetPlanCode string `json:"target_plan_code"`
	Currency       string `json:"currency,omitempty"`
}

type upgradePreparePaymentRequest struct {
	TargetPlanCode   string `json:"target_plan_code"`
	PreviewToken     string `json:"preview_token"`
	UpgradeRequestID string `json:"upgrade_request_id"`
}

type upgradeExecuteRequest struct {
	TargetPlanCode        string `json:"target_plan_code"`
	PreviewToken          string `json:"preview_token"`
	RazorpayOrderID       string `json:"razorpay_order_id"`
	RazorpayPaymentID     string `json:"razorpay_payment_id"`
	RazorpaySignature     string `json:"razorpay_signature"`
	ExecuteIdempotencyKey string `json:"execute_idempotency_key"`
	ModalVersion          string `json:"modal_version"`
	ModalAcknowledgedAt   string `json:"modal_acknowledged_at"`
	UpgradeRequestID      string `json:"upgrade_request_id"`
}

type signedUpgradePreview struct {
	UpgradeRequestID        string `json:"upgrade_request_id"`
	ActorUserID             int64  `json:"actor_user_id"`
	OrgID                   int64  `json:"org_id"`
	FromPlanCode            string `json:"from_plan_code"`
	ToPlanCode              string `json:"to_plan_code"`
	CycleStartUnix          int64  `json:"cycle_start_unix"`
	CycleEndUnix            int64  `json:"cycle_end_unix"`
	RemainingFractionBP     int64  `json:"remaining_fraction_bp"`
	ImmediateChargeCents    int64  `json:"immediate_charge_cents"`
	ImmediateChargeCurrency string `json:"immediate_charge_currency"`
	ImmediateLOCGrant       int64  `json:"immediate_loc_grant"`
	NextCyclePriceCents     int64  `json:"next_cycle_price_cents"`
	NextCycleLOCLimit       int64  `json:"next_cycle_loc_limit"`
	ExpiresAtUnix           int64  `json:"expires_at_unix"`
}

func resolveRazorpayModeForBilling() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("RAZORPAY_MODE")))
	if mode == "" {
		return "live"
	}
	return mode
}

func previewTokenSecret() string {
	return strings.TrimSpace(os.Getenv("JWT_SECRET"))
}

func signUpgradePreviewToken(data signedUpgradePreview) (string, error) {
	secret := previewTokenSecret()
	if secret == "" {
		return "", fmt.Errorf("JWT_SECRET must be set for upgrade preview signing")
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal preview token payload: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encoded))
	signature := hex.EncodeToString(mac.Sum(nil))

	return encoded + "." + signature, nil
}

func parseAndVerifyUpgradePreviewToken(token string) (signedUpgradePreview, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 2 {
		return signedUpgradePreview{}, fmt.Errorf("invalid preview token")
	}

	secret := previewTokenSecret()
	if secret == "" {
		return signedUpgradePreview{}, fmt.Errorf("JWT_SECRET must be set for upgrade preview verification")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0]))
	expected := hex.EncodeToString(mac.Sum(nil))
	provided := strings.ToLower(strings.TrimSpace(parts[1]))
	if !hmac.Equal([]byte(expected), []byte(provided)) {
		return signedUpgradePreview{}, fmt.Errorf("invalid preview token signature")
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return signedUpgradePreview{}, fmt.Errorf("decode preview token payload: %w", err)
	}

	var payload signedUpgradePreview
	if err := json.Unmarshal(raw, &payload); err != nil {
		return signedUpgradePreview{}, fmt.Errorf("unmarshal preview token payload: %w", err)
	}

	if payload.ExpiresAtUnix <= 0 || time.Now().UTC().Unix() > payload.ExpiresAtUnix {
		return signedUpgradePreview{}, fmt.Errorf("preview token expired")
	}

	return payload, nil
}

func computeRemainingCycleFraction(cycleStart, cycleEnd, now time.Time) float64 {
	if !cycleEnd.After(cycleStart) {
		return 1
	}

	if now.Before(cycleStart) {
		now = cycleStart
	}
	if !now.Before(cycleEnd) {
		return 0
	}

	cycleSeconds := cycleEnd.Sub(cycleStart).Seconds()
	remainingSeconds := cycleEnd.Sub(now).Seconds()
	if cycleSeconds <= 0 || remainingSeconds <= 0 {
		return 0
	}

	fraction := remainingSeconds / cycleSeconds
	if fraction < 0 {
		return 0
	}
	if fraction > 1 {
		return 1
	}
	return fraction
}

func computeTargetProratedChargeCents(targetMonthlyCents int64, fraction float64) int64 {
	if targetMonthlyCents <= 0 || fraction <= 0 {
		return 0
	}
	charge := int64(math.Round(float64(targetMonthlyCents) * fraction))
	if charge < 0 {
		return 0
	}
	return charge
}

func computeTargetProratedLOCGrant(targetMonthlyLOC int, fraction float64) int64 {
	if targetMonthlyLOC <= 0 || fraction <= 0 {
		return 0
	}
	grant := int64(math.Round(float64(targetMonthlyLOC) * fraction))
	if grant < 0 {
		return 0
	}
	return grant
}

func (h *BillingActionsHandler) buildUpgradePreview(ctx context.Context, orgID int64, currentPlan, targetPlan license.PlanType, fallbackCycleStart, fallbackCycleEnd time.Time, currency string) (signedUpgradePreview, map[string]interface{}, error) {
	if h.db == nil {
		return signedUpgradePreview{}, nil, fmt.Errorf("missing db handle")
	}

	subStore := storagepayment.NewSubscriptionStore(h.db)
	subscriptions, err := subStore.ListSubscriptionsByOrgID(int(orgID))
	if err != nil {
		return signedUpgradePreview{}, nil, fmt.Errorf("load org subscriptions: %w", err)
	}
	if len(subscriptions) == 0 {
		return signedUpgradePreview{}, nil, fmt.Errorf("%w: organization has no active subscription", errRazorpayCheckoutRequired)
	}

	active := subscriptions[0]
	for _, s := range subscriptions {
		if strings.EqualFold(s.Status, "active") {
			active = s
			break
		}
	}
	if strings.TrimSpace(active.RazorpaySubscriptionID) == "" {
		return signedUpgradePreview{}, nil, fmt.Errorf("%w: no razorpay subscription id", errRazorpayCheckoutRequired)
	}

	mode := resolveRazorpayModeForBilling()

	cycleStart := fallbackCycleStart.UTC()
	cycleEnd := fallbackCycleEnd.UTC()
	razorpaySub, err := payment.GetSubscriptionByID(mode, active.RazorpaySubscriptionID)
	if err != nil {
		return signedUpgradePreview{}, nil, fmt.Errorf("load razorpay subscription: %w", err)
	}
	if razorpaySub.CurrentStart > 0 {
		cycleStart = time.Unix(razorpaySub.CurrentStart, 0).UTC()
	}
	if razorpaySub.CurrentEnd > 0 {
		cycleEnd = time.Unix(razorpaySub.CurrentEnd, 0).UTC()
	}

	effectiveMonthlyPlanID, err := payment.GetPlanID(mode, "monthly", currency)
	if err != nil {
		return signedUpgradePreview{}, nil, fmt.Errorf("resolve monthly plan id: %w", err)
	}
	effectiveMonthlyPlanID = strings.TrimSpace(effectiveMonthlyPlanID)
	if effectiveMonthlyPlanID == "" {
		return signedUpgradePreview{}, nil, fmt.Errorf("monthly plan id is empty for mode=%s", mode)
	}

	effectiveMonthlyPlan, err := payment.GetPlanByID(mode, effectiveMonthlyPlanID)
	if err != nil {
		return signedUpgradePreview{}, nil, fmt.Errorf("load razorpay monthly plan for pricing profile: %w", err)
	}

	targetMonthlyCents := int64(locPlanToQuantity(targetPlan)) * int64(effectiveMonthlyPlan.Item.Amount)
	if targetMonthlyCents <= 0 {
		return signedUpgradePreview{}, nil, fmt.Errorf("computed target monthly cents must be positive for plan=%s", targetPlan)
	}

	chargeCurrency := strings.ToUpper(strings.TrimSpace(effectiveMonthlyPlan.Item.Currency))
	if chargeCurrency == "" {
		return signedUpgradePreview{}, nil, fmt.Errorf("razorpay plan %s has empty currency", effectiveMonthlyPlanID)
	}
	if !strings.EqualFold(chargeCurrency, currency) {
		return signedUpgradePreview{}, nil, fmt.Errorf("razorpay plan currency mismatch: expected %s got %s", currency, chargeCurrency)
	}

	now := time.Now().UTC()
	remainingFraction := computeRemainingCycleFraction(cycleStart, cycleEnd, now)
	chargeCents := computeTargetProratedChargeCents(targetMonthlyCents, remainingFraction)
	locGrant := computeTargetProratedLOCGrant(targetPlan.GetLimits().MonthlyLOCLimit, remainingFraction)

	tokenPayload := signedUpgradePreview{
		OrgID:                   orgID,
		FromPlanCode:            currentPlan.String(),
		ToPlanCode:              targetPlan.String(),
		CycleStartUnix:          cycleStart.Unix(),
		CycleEndUnix:            cycleEnd.Unix(),
		RemainingFractionBP:     int64(math.Round(remainingFraction * 10000)),
		ImmediateChargeCents:    chargeCents,
		ImmediateChargeCurrency: chargeCurrency,
		ImmediateLOCGrant:       locGrant,
		NextCyclePriceCents:     targetMonthlyCents,
		NextCycleLOCLimit:       int64(targetPlan.GetLimits().MonthlyLOCLimit),
		ExpiresAtUnix:           now.Add(5 * time.Minute).Unix(),
	}

	preview := map[string]interface{}{
		"from_plan_code":              currentPlan.String(),
		"to_plan_code":                targetPlan.String(),
		"cycle_start":                 cycleStart.Format(time.RFC3339),
		"cycle_end":                   cycleEnd.Format(time.RFC3339),
		"remaining_cycle_fraction":    math.Round(remainingFraction*10000) / 10000,
		"immediate_charge_cents":      chargeCents,
		"immediate_charge_currency":   chargeCurrency,
		"immediate_loc_grant":         locGrant,
		"next_cycle_price_cents":      tokenPayload.NextCyclePriceCents,
		"next_cycle_loc_limit":        tokenPayload.NextCycleLOCLimit,
		"charge_timing":               "immediate_one_time_order",
		"plan_switch_timing":          "immediate",
		"rounding_policy_money":       "nearest_cent_half_up",
		"rounding_policy_loc":         "nearest_whole_loc",
		"fraction_basis":              "exact_utc_seconds",
		"current_cycle_duration_secs": cycleEnd.Sub(cycleStart).Seconds(),
		"final_payable_cents":         chargeCents,
	}

	return tokenPayload, preview, nil
}

func (h *BillingActionsHandler) PreviewUpgrade(c echo.Context) error {
	orgID, actorUserID, err := h.requirePlanManager(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", httpErr.Message)
			if msg == "" {
				msg = http.StatusText(httpErr.Code)
			}
			return JSONErrorWithEnvelope(c, httpErr.Code, msg)
		}
		return err
	}

	var req planChangeRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}

	targetPlan := license.PlanType(strings.TrimSpace(req.TargetPlanCode))
	if !targetPlan.IsValid() {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid target_plan_code")
	}

	resolvedCurrency, err := resolvePurchaseCurrency(req.Currency, c.Request())
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, currencyErrorMessage(err))
	}

	ctx := c.Request().Context()
	if err := h.store.EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	state, err := h.store.GetOrgBillingState(ctx, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to fetch billing state: %v", err))
	}

	currentPlan := license.PlanType(state.CurrentPlanCode)
	if targetPlan.GetLimits().MonthlyLOCLimit <= currentPlan.GetLimits().MonthlyLOCLimit {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "target_plan_code must be a higher LOC tier for upgrade")
	}

	tokenPayload, preview, err := h.buildUpgradePreview(ctx, orgID, currentPlan, targetPlan, state.BillingPeriodStart, state.BillingPeriodEnd, resolvedCurrency)
	if err != nil {
		if errors.Is(err, errRazorpayCheckoutRequired) {
			return JSONWithEnvelope(c, http.StatusConflict, map[string]interface{}{
				"message":           "organization requires paid checkout before upgrade",
				"checkout_required": true,
				"checkout_path":     "/checkout/team?period=monthly",
			})
		}
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("failed to build upgrade preview: %v", err))
	}

	requestUUID, err := uuid.NewV7()
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to generate upgrade_request_id: %v", err))
	}
	tokenPayload.UpgradeRequestID = requestUUID.String()
	tokenPayload.ActorUserID = actorUserID

	previewToken, err := signUpgradePreviewToken(tokenPayload)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to sign preview token: %v", err))
	}

	if _, err := h.upgradeRequestStore.CreateUpgradeRequest(ctx, storagepayment.CreateUpgradeRequestInput{
		UpgradeRequestID:    tokenPayload.UpgradeRequestID,
		OrgID:               orgID,
		ActorUserID:         actorUserID,
		FromPlanCode:        tokenPayload.FromPlanCode,
		ToPlanCode:          tokenPayload.ToPlanCode,
		ExpectedAmountCents: tokenPayload.ImmediateChargeCents,
		Currency:            tokenPayload.ImmediateChargeCurrency,
		PreviewToken:        previewToken,
	}); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to create upgrade request: %v", err))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"preview":            preview,
		"preview_token":      previewToken,
		"preview_expires_at": time.Unix(tokenPayload.ExpiresAtUnix, 0).UTC().Format(time.RFC3339),
		"upgrade_request_id": tokenPayload.UpgradeRequestID,
	})
}

func (h *BillingActionsHandler) PrepareUpgradePayment(c echo.Context) error {
	orgID, _, err := h.requirePlanManager(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", httpErr.Message)
			if msg == "" {
				msg = http.StatusText(httpErr.Code)
			}
			return JSONErrorWithEnvelope(c, httpErr.Code, msg)
		}
		return err
	}

	var req upgradePreparePaymentRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}

	payload, err := parseAndVerifyUpgradePreviewToken(req.PreviewToken)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	if payload.OrgID != orgID {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token organization mismatch")
	}

	upgradeRequestID := strings.TrimSpace(payload.UpgradeRequestID)
	if upgradeRequestID == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token missing upgrade_request_id")
	}
	if strings.TrimSpace(req.UpgradeRequestID) != "" && strings.TrimSpace(req.UpgradeRequestID) != upgradeRequestID {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "upgrade_request_id mismatch")
	}

	targetPlanCode := strings.TrimSpace(req.TargetPlanCode)
	if targetPlanCode == "" {
		targetPlanCode = payload.ToPlanCode
	}
	if payload.ToPlanCode != targetPlanCode {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token target plan mismatch")
	}

	ctx := c.Request().Context()
	upgradeRequest, err := h.upgradeRequestStore.GetUpgradeRequestByIDForOrg(ctx, orgID, upgradeRequestID)
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "unknown upgrade_request_id")
		}
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load upgrade request: %v", err))
	}

	if upgradeRequest.FromPlanCode != payload.FromPlanCode || upgradeRequest.ToPlanCode != payload.ToPlanCode {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "upgrade request plan correlation mismatch")
	}
	if upgradeRequest.PreviewTokenSHA256 != storagepayment.HashUpgradePreviewToken(req.PreviewToken) {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "upgrade request preview token mismatch")
	}

	if err := h.store.EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}
	state, err := h.store.GetOrgBillingState(ctx, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to fetch billing state: %v", err))
	}

	currentPlan := license.PlanType(state.CurrentPlanCode)
	targetPlan := license.PlanType(payload.ToPlanCode)
	if targetPlan.IsValid() &&
		state.ScheduledPlanCode.Valid &&
		strings.TrimSpace(state.ScheduledPlanCode.String) == targetPlan.String() &&
		targetPlan.GetLimits().MonthlyLOCLimit > currentPlan.GetLimits().MonthlyLOCLimit {
		currency := strings.ToUpper(strings.TrimSpace(payload.ImmediateChargeCurrency))
		if currency == "" {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token missing immediate charge currency")
		}
		return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
			"payment_required":          false,
			"amount_cents":              int64(0),
			"currency":                  currency,
			"preview_token":             req.PreviewToken,
			"upgrade_request_id":        upgradeRequestID,
			"payment_already_collected": true,
		})
	}

	mode := resolveRazorpayModeForBilling()
	keyID, _, err := payment.GetRazorpayKeys(mode)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load razorpay keys: %v", err))
	}

	currency := strings.ToUpper(strings.TrimSpace(payload.ImmediateChargeCurrency))
	if currency == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token missing immediate charge currency")
	}

	if payload.ImmediateChargeCents <= 0 {
		return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
			"payment_required":   false,
			"amount_cents":       int64(0),
			"currency":           currency,
			"preview_token":      req.PreviewToken,
			"upgrade_request_id": upgradeRequestID,
		})
	}

	if upgradeRequest.RazorpayOrderID.Valid {
		priorOrderID := strings.TrimSpace(upgradeRequest.RazorpayOrderID.String)
		if priorOrderID != "" {
			attempt, attemptErr := h.paymentAttemptStore.GetAttemptByOrgRequestAndOrder(ctx, orgID, upgradeRequestID, priorOrderID)
			if attemptErr == nil {
				if attempt.AmountCents == payload.ImmediateChargeCents &&
					strings.EqualFold(strings.TrimSpace(attempt.Currency), currency) &&
					strings.EqualFold(strings.TrimSpace(attempt.RazorpayMode), mode) {
					return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
						"payment_required":   true,
						"razorpay_key_id":    keyID,
						"order_id":           attempt.RazorpayOrderID,
						"amount_cents":       attempt.AmountCents,
						"currency":           attempt.Currency,
						"preview_token":      req.PreviewToken,
						"upgrade_request_id": upgradeRequestID,
					})
				}
			} else if !errors.Is(attemptErr, storagepayment.ErrUpgradePaymentAttemptNotFound) {
				return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load existing upgrade payment attempt: %v", attemptErr))
			}
		}
	}

	receipt := fmt.Sprintf("upg_%d_%d", orgID, time.Now().UTC().Unix())
	order, err := payment.CreateOrder(mode, payload.ImmediateChargeCents, currency, receipt, map[string]string{
		"org_id":             fmt.Sprintf("%d", orgID),
		"from_plan_code":     payload.FromPlanCode,
		"to_plan_code":       payload.ToPlanCode,
		"upgrade_request_id": upgradeRequestID,
	})
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("failed to create upgrade order: %v", err))
	}

	if _, err := h.upgradeRequestStore.MarkOrderPrepared(ctx, storagepayment.MarkUpgradeOrderPreparedInput{
		UpgradeRequestID: upgradeRequestID,
		OrgID:            orgID,
		RazorpayMode:     mode,
		RazorpayOrderID:  order.ID,
		AmountCents:      payload.ImmediateChargeCents,
		Currency:         currency,
		Metadata: map[string]interface{}{
			"preview_token_sha256": storagepayment.HashUpgradePreviewToken(req.PreviewToken),
		},
	}); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to persist upgrade request order correlation: %v", err))
	}

	if _, err := h.paymentAttemptStore.CreateUpgradePaymentAttempt(ctx, storagepayment.CreateUpgradePaymentAttemptInput{
		OrgID:            orgID,
		UpgradeRequestID: upgradeRequestID,
		PreviewToken:     req.PreviewToken,
		FromPlanCode:     payload.FromPlanCode,
		ToPlanCode:       payload.ToPlanCode,
		AmountCents:      payload.ImmediateChargeCents,
		Currency:         currency,
		RazorpayMode:     mode,
		RazorpayOrderID:  order.ID,
	}); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to persist upgrade payment attempt: %v", err))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"payment_required":   true,
		"razorpay_key_id":    keyID,
		"order_id":           order.ID,
		"amount_cents":       order.Amount,
		"currency":           order.Currency,
		"preview_token":      req.PreviewToken,
		"upgrade_request_id": upgradeRequestID,
	})
}

func (h *BillingActionsHandler) ExecuteUpgrade(c echo.Context) error {
	orgID, _, err := h.requirePlanManager(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", httpErr.Message)
			if msg == "" {
				msg = http.StatusText(httpErr.Code)
			}
			return JSONErrorWithEnvelope(c, httpErr.Code, msg)
		}
		return err
	}

	var req upgradeExecuteRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}

	payload, err := parseAndVerifyUpgradePreviewToken(req.PreviewToken)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	if payload.OrgID != orgID {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token organization mismatch")
	}

	upgradeRequestID := strings.TrimSpace(payload.UpgradeRequestID)
	if upgradeRequestID == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token missing upgrade_request_id")
	}
	if strings.TrimSpace(req.UpgradeRequestID) != "" && strings.TrimSpace(req.UpgradeRequestID) != upgradeRequestID {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "upgrade_request_id mismatch")
	}
	if strings.TrimSpace(req.TargetPlanCode) != "" && strings.TrimSpace(req.TargetPlanCode) != payload.ToPlanCode {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token target plan mismatch")
	}

	executeIdempotencyKey := strings.TrimSpace(req.ExecuteIdempotencyKey)
	if executeIdempotencyKey == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "execute_idempotency_key is required")
	}

	orderID := strings.TrimSpace(req.RazorpayOrderID)
	paymentID := strings.TrimSpace(req.RazorpayPaymentID)
	signature := strings.TrimSpace(req.RazorpaySignature)

	ctx := c.Request().Context()
	upgradeRequest, err := h.upgradeRequestStore.GetUpgradeRequestByIDForOrg(ctx, orgID, upgradeRequestID)
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "unknown upgrade_request_id")
		}
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load upgrade request: %v", err))
	}

	if upgradeRequest.FromPlanCode != payload.FromPlanCode || upgradeRequest.ToPlanCode != payload.ToPlanCode {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "upgrade request plan correlation mismatch")
	}
	if upgradeRequest.PreviewTokenSHA256 != storagepayment.HashUpgradePreviewToken(req.PreviewToken) {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "upgrade request preview token mismatch")
	}

	if strings.EqualFold(upgradeRequest.CurrentStatus, storagepayment.UpgradeRequestStatusResolved) {
		if applyErr := h.applyResolvedUpgradeRequest(ctx, upgradeRequest); applyErr != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to finalize resolved upgrade request: %v", applyErr))
		}
		return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
			"message":            "upgrade already resolved",
			"idempotent_replay":  true,
			"upgrade_request_id": upgradeRequestID,
			"status":             storagepayment.UpgradeRequestStatusResolved,
		})
	}

	if err := h.store.EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}
	state, err := h.store.GetOrgBillingState(ctx, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to fetch billing state: %v", err))
	}

	if state.CurrentPlanCode != payload.FromPlanCode {
		return JSONErrorWithEnvelope(c, http.StatusConflict, "current plan changed since preview; refresh preview")
	}

	currentPlan := license.PlanType(state.CurrentPlanCode)
	targetPlan := license.PlanType(payload.ToPlanCode)
	if !targetPlan.IsValid() {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid target plan in preview token")
	}
	skipPaymentVerification := state.ScheduledPlanCode.Valid &&
		strings.TrimSpace(state.ScheduledPlanCode.String) == targetPlan.String() &&
		targetPlan.GetLimits().MonthlyLOCLimit > currentPlan.GetLimits().MonthlyLOCLimit

	mode := resolveRazorpayModeForBilling()
	paymentMethod := ""
	var attempt storagepayment.UpgradePaymentAttempt
	if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
		if orderID == "" || paymentID == "" || signature == "" {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "razorpay_order_id, razorpay_payment_id, and razorpay_signature are required")
		}

		attempt, err = h.paymentAttemptStore.GetAttemptByOrgRequestAndOrder(ctx, orgID, upgradeRequestID, orderID)
		if err != nil {
			if errors.Is(err, storagepayment.ErrUpgradePaymentAttemptNotFound) {
				return JSONErrorWithEnvelope(c, http.StatusBadRequest, "unknown upgrade payment attempt for provided order")
			}
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load upgrade payment attempt: %v", err))
		}

		if attempt.FromPlanCode != payload.FromPlanCode || attempt.ToPlanCode != payload.ToPlanCode {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment attempt plan correlation mismatch")
		}
		if attempt.AmountCents != payload.ImmediateChargeCents {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment attempt amount mismatch")
		}
		if !strings.EqualFold(strings.TrimSpace(attempt.Currency), strings.TrimSpace(payload.ImmediateChargeCurrency)) {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment attempt currency mismatch")
		}

		attempt, alreadyApplied, reserveErr := h.paymentAttemptStore.ReserveExecute(ctx, storagepayment.ReserveUpgradeExecuteInput{
			OrgID:                 orgID,
			UpgradeRequestID:      upgradeRequestID,
			PreviewToken:          req.PreviewToken,
			RazorpayOrderID:       orderID,
			RazorpayPaymentID:     paymentID,
			ExecuteIdempotencyKey: executeIdempotencyKey,
		})
		if reserveErr != nil {
			if errors.Is(reserveErr, storagepayment.ErrUpgradePaymentAttemptNotFound) {
				return JSONErrorWithEnvelope(c, http.StatusBadRequest, "unknown upgrade payment attempt for provided order")
			}
			if errors.Is(reserveErr, storagepayment.ErrUpgradePaymentAttemptIdempotencyMismatch) {
				return JSONErrorWithEnvelope(c, http.StatusConflict, "execute idempotency key mismatch for this upgrade payment attempt")
			}
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to reserve upgrade execute attempt: %v", reserveErr))
		}
		if alreadyApplied {
			stored, decodeErr := storagepayment.DecodeUpgradeExecuteResponse(attempt.ExecuteResponse)
			if decodeErr == nil && stored != nil {
				return JSONWithEnvelope(c, http.StatusOK, stored)
			}
			return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
				"message":           "upgrade already executed",
				"idempotent_replay": true,
				"order_id":          orderID,
				"payment_id":        paymentID,
			})
		}
	}

	if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
		currency := strings.ToUpper(strings.TrimSpace(payload.ImmediateChargeCurrency))
		if currency == "" {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "preview token missing immediate charge currency")
		}

		if err := payment.VerifyOrderPaymentSignature(mode, orderID, paymentID, signature); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
		}

		paid, err := payment.GetPaymentByID(mode, paymentID)
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("failed to fetch payment: %v", err))
		}
		if strings.TrimSpace(paid.OrderID) != orderID {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment order mismatch")
		}
		if !paid.Captured.Bool() {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment is not captured")
		}
		if paid.Amount != payload.ImmediateChargeCents {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment amount mismatch")
		}
		if !strings.EqualFold(strings.TrimSpace(paid.Currency), currency) {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "payment currency mismatch")
		}
		paymentMethod = strings.TrimSpace(paid.Method)
		if err := h.paymentAttemptStore.MarkPaymentCapturedByOrderID(ctx, orderID, paymentID); err != nil && !errors.Is(err, storagepayment.ErrUpgradePaymentAttemptNotFound) {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to persist captured upgrade payment attempt: %v", err))
		}
	}

	applyImmediateTransition := true
	if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
		applyImmediateTransition = !isUPIPaymentMethod(paymentMethod)
	}

	if err := h.syncRazorpayTransition(ctx, orgID, targetPlan, applyImmediateTransition, state.BillingPeriodEnd); err != nil {
		if isRazorpayUPISubscriptionUpdateError(err) {
			if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
				_, _ = h.upgradeRequestStore.MarkPaymentCaptureConfirmed(ctx, storagepayment.MarkUpgradePaymentCaptureInput{
					UpgradeRequestID:  upgradeRequestID,
					RazorpayPaymentID: paymentID,
					RazorpayOrderID:   orderID,
					Metadata: map[string]interface{}{
						"source":         "execute_sync_failure_upi",
						"payment_method": paymentMethod,
					},
				})
			}

			latestRequest, loadErr := h.upgradeRequestStore.GetUpgradeRequestByIDForOrg(ctx, orgID, upgradeRequestID)
			if loadErr != nil {
				return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to reload upgrade request for replacement cutover: %v", loadErr))
			}

			updatedRequest, cutoverErr := h.processUPIReplacementCutover(ctx, latestRequest, targetPlan, mode)
			if cutoverErr != nil {
				_, _ = h.upgradeRequestStore.MarkReconciliationRetrying(ctx, upgradeRequestID, map[string]interface{}{
					"source":           "execute_upi_replacement_retry",
					"payment_method":   paymentMethod,
					"retry_after_secs": 120,
					"error":            cutoverErr.Error(),
				})

				responsePayload := map[string]interface{}{
					"message":            "payment captured; scheduling replacement subscription cutover in progress",
					"upgrade_request_id": upgradeRequestID,
					"status":             storagepayment.UpgradeRequestStatusReconciliationRetrying,
					"reason_code":        "upi_replacement_cutover_pending",
				}

				if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
					if _, err := h.paymentAttemptStore.MarkExecuteApplied(ctx, storagepayment.MarkUpgradeExecuteAppliedInput{
						RazorpayOrderID:       orderID,
						RazorpayPaymentID:     paymentID,
						ExecuteIdempotencyKey: executeIdempotencyKey,
						ExecuteResponse:       responsePayload,
					}); err != nil {
						log.Printf("[billing-upgrade] warning: failed to persist execute_applied attempt org=%d order_id=%s: %v", orgID, orderID, err)
					}
				}

				return JSONWithEnvelope(c, http.StatusAccepted, responsePayload)
			}

			if strings.EqualFold(updatedRequest.CurrentStatus, storagepayment.UpgradeRequestStatusResolved) {
				if err := h.applyResolvedUpgradeRequest(ctx, updatedRequest); err != nil {
					return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to apply resolved replacement upgrade request: %v", err))
				}
			}

			responsePayload := map[string]interface{}{
				"message":            "upgrade request accepted; replacement subscription cutover scheduled",
				"transition_mode":    "replacement_subscription_cutover",
				"plan_code":          targetPlan.String(),
				"upgrade_request_id": upgradeRequestID,
				"status":             updatedRequest.CurrentStatus,
				"resolved":           strings.EqualFold(updatedRequest.CurrentStatus, storagepayment.UpgradeRequestStatusResolved),
			}

			if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
				if _, err := h.paymentAttemptStore.MarkExecuteApplied(ctx, storagepayment.MarkUpgradeExecuteAppliedInput{
					RazorpayOrderID:       orderID,
					RazorpayPaymentID:     paymentID,
					ExecuteIdempotencyKey: executeIdempotencyKey,
					ExecuteResponse:       responsePayload,
				}); err != nil {
					log.Printf("[billing-upgrade] warning: failed to persist execute_applied attempt org=%d order_id=%s: %v", orgID, orderID, err)
				}
			}

			return JSONWithEnvelope(c, http.StatusOK, responsePayload)
		}

		_, _ = h.upgradeRequestStore.MarkUpgradeRequestFailed(ctx, storagepayment.MarkUpgradeRequestFailedInput{
			UpgradeRequestID: upgradeRequestID,
			FailureReason:    fmt.Sprintf("subscription update failed: %v", err),
			Metadata: map[string]interface{}{
				"stage": "sync_razorpay_transition",
			},
		})
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("razorpay immediate transition failed: %v", err))
	}

	activeSubscription, subErr := resolveActiveOrgSubscription(h.db, orgID)
	if subErr != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to resolve active subscription for upgrade request: %v", subErr))
	}

	if _, err := h.upgradeRequestStore.MarkSubscriptionUpdateRequested(ctx, storagepayment.MarkUpgradeSubscriptionUpdateInput{
		UpgradeRequestID:       upgradeRequestID,
		LocalSubscriptionID:    activeSubscription.ID,
		RazorpaySubscriptionID: activeSubscription.RazorpaySubscriptionID,
		TargetQuantity:         locPlanToQuantity(targetPlan),
		Metadata: map[string]interface{}{
			"execute_idempotency_key": executeIdempotencyKey,
			"razorpay_order_id":       orderID,
			"razorpay_payment_id":     paymentID,
			"modal_version":           strings.TrimSpace(req.ModalVersion),
		},
	}); err != nil && !errors.Is(err, storagepayment.ErrUpgradeRequestTransitionRejected) {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to persist subscription update state: %v", err))
	}

	if payload.ImmediateChargeCents <= 0 || skipPaymentVerification {
		_, _ = h.upgradeRequestStore.MarkPaymentCaptureConfirmed(ctx, storagepayment.MarkUpgradePaymentCaptureInput{
			UpgradeRequestID:  upgradeRequestID,
			RazorpayPaymentID: paymentID,
			RazorpayOrderID:   orderID,
			Metadata: map[string]interface{}{
				"source": "execute_no_immediate_payment",
			},
		})
	}

	if _, recErr := h.reconcileUpgradeRequestNow(ctx, upgradeRequestID); recErr != nil {
		log.Printf("[billing-upgrade] reconcile-now warning request=%s org=%d: %v", upgradeRequestID, orgID, recErr)
	}

	updatedRequest, err := h.upgradeRequestStore.GetUpgradeRequestByIDForOrg(ctx, orgID, upgradeRequestID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to reload upgrade request status: %v", err))
	}

	if strings.EqualFold(updatedRequest.CurrentStatus, storagepayment.UpgradeRequestStatusResolved) {
		if err := h.applyResolvedUpgradeRequest(ctx, updatedRequest); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to apply resolved upgrade request: %v", err))
		}
	}

	responsePayload := map[string]interface{}{
		"message":            "upgrade request accepted; waiting for deterministic confirmations",
		"transition_mode":    "deterministic_process",
		"plan_code":          targetPlan.String(),
		"upgrade_request_id": upgradeRequestID,
		"status":             updatedRequest.CurrentStatus,
		"resolved":           strings.EqualFold(updatedRequest.CurrentStatus, storagepayment.UpgradeRequestStatusResolved),
		"proration": map[string]interface{}{
			"from_plan_code":           payload.FromPlanCode,
			"to_plan_code":             payload.ToPlanCode,
			"cycle_start":              time.Unix(payload.CycleStartUnix, 0).UTC().Format(time.RFC3339),
			"cycle_end":                time.Unix(payload.CycleEndUnix, 0).UTC().Format(time.RFC3339),
			"remaining_cycle_fraction": float64(payload.RemainingFractionBP) / 10000,
			"charge_amount_cents":      payload.ImmediateChargeCents,
			"charge_currency":          payload.ImmediateChargeCurrency,
			"charge_status": func() string {
				if payload.ImmediateChargeCents <= 0 {
					return "skipped"
				}
				if skipPaymentVerification {
					return "already_captured"
				}
				return "verification_pending"
			}(),
			"payment_id":             paymentID,
			"order_id":               orderID,
			"immediate_loc_grant":    payload.ImmediateLOCGrant,
			"next_cycle_price_cents": payload.NextCyclePriceCents,
			"next_cycle_loc_limit":   payload.NextCycleLOCLimit,
		},
	}

	if payload.ImmediateChargeCents > 0 && !skipPaymentVerification {
		if _, err := h.paymentAttemptStore.MarkExecuteApplied(ctx, storagepayment.MarkUpgradeExecuteAppliedInput{
			RazorpayOrderID:       orderID,
			RazorpayPaymentID:     paymentID,
			ExecuteIdempotencyKey: executeIdempotencyKey,
			ExecuteResponse:       responsePayload,
		}); err != nil {
			log.Printf("[billing-upgrade] warning: failed to persist execute_applied attempt org=%d order_id=%s: %v", orgID, orderID, err)
		}
	}

	return JSONWithEnvelope(c, http.StatusOK, responsePayload)
}

type activeOrgSubscription struct {
	ID                     int64
	OwnerUserID            int64
	OrgID                  int64
	RazorpaySubscriptionID string
	RazorpayPlanID         string
	Quantity               int
	Status                 string
	CurrentPeriodStart     time.Time
	CurrentPeriodEnd       time.Time
}

func resolveActiveOrgSubscription(db *sql.DB, orgID int64) (activeOrgSubscription, error) {
	var out activeOrgSubscription
	err := db.QueryRow(`
		SELECT id, owner_user_id, org_id, razorpay_subscription_id, razorpay_plan_id, quantity, status, current_period_start, current_period_end
		FROM subscriptions
		WHERE org_id = $1
		ORDER BY CASE WHEN status = 'active' THEN 0 ELSE 1 END, updated_at DESC, created_at DESC
		LIMIT 1`, orgID).Scan(
		&out.ID,
		&out.OwnerUserID,
		&out.OrgID,
		&out.RazorpaySubscriptionID,
		&out.RazorpayPlanID,
		&out.Quantity,
		&out.Status,
		&out.CurrentPeriodStart,
		&out.CurrentPeriodEnd,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return activeOrgSubscription{}, fmt.Errorf("no subscription found for org %d", orgID)
		}
		return activeOrgSubscription{}, fmt.Errorf("resolve active org subscription: %w", err)
	}
	return out, nil
}

func (h *BillingActionsHandler) applyResolvedUpgradeRequest(ctx context.Context, request storagepayment.UpgradeRequest) error {
	if request.PlanGrantApplied {
		return nil
	}

	if err := h.store.EnsureOrgBillingState(ctx, request.OrgID, license.PlanFree30K.String()); err != nil {
		return fmt.Errorf("ensure org billing state before resolved apply: %w", err)
	}

	state, err := h.store.GetOrgBillingState(ctx, request.OrgID)
	if err != nil {
		return fmt.Errorf("load org billing state before resolved apply: %w", err)
	}

	if strings.TrimSpace(state.CurrentPlanCode) != strings.TrimSpace(request.ToPlanCode) {
		payload := map[string]interface{}{
			"upgrade_request_id":        request.UpgradeRequestID,
			"from_plan_code":            request.FromPlanCode,
			"to_plan_code":              request.ToPlanCode,
			"expected_amount_cents":     request.ExpectedAmountCents,
			"currency":                  request.Currency,
			"payment_capture_confirmed": request.PaymentCaptureConfirmed,
			"subscription_confirmed":    request.SubscriptionChangeConfirmed,
			"resolved_at": func() string {
				if request.ResolvedAt.Valid {
					return request.ResolvedAt.Time.UTC().Format(time.RFC3339)
				}
				return ""
			}(),
		}
		if err := h.store.ApplyImmediatePlanUpgrade(ctx, request.OrgID, request.ToPlanCode, request.ActorUserID, payload); err != nil {
			return fmt.Errorf("apply resolved upgrade to org billing state: %w", err)
		}
	}

	if _, err := h.upgradeRequestStore.MarkPlanGrantApplied(ctx, request.UpgradeRequestID, map[string]interface{}{
		"source": "resolved_apply",
	}); err != nil {
		if !errors.Is(err, storagepayment.ErrUpgradeRequestTransitionRejected) {
			return fmt.Errorf("mark plan grant applied on upgrade request: %w", err)
		}
	}

	return nil
}

func (h *BillingActionsHandler) reconcileUpgradeRequestNow(ctx context.Context, upgradeRequestID string) (storagepayment.UpgradeRequest, error) {
	request, err := h.upgradeRequestStore.GetUpgradeRequestByID(ctx, upgradeRequestID)
	if err != nil {
		return storagepayment.UpgradeRequest{}, err
	}

	mode := strings.TrimSpace(request.RazorpayMode.String)
	if mode == "" {
		mode = resolveRazorpayModeForBilling()
	}

	if !request.PaymentCaptureConfirmed && request.RazorpayPaymentID.Valid {
		paymentID := strings.TrimSpace(request.RazorpayPaymentID.String)
		if paymentID != "" {
			paid, payErr := payment.GetPaymentByID(mode, paymentID)
			if payErr == nil && paid.Captured.Bool() {
				orderID := strings.TrimSpace(request.RazorpayOrderID.String)
				if strings.TrimSpace(paid.OrderID) == orderID {
					_, _ = h.upgradeRequestStore.MarkPaymentCaptureConfirmed(ctx, storagepayment.MarkUpgradePaymentCaptureInput{
						UpgradeRequestID:  request.UpgradeRequestID,
						RazorpayPaymentID: paymentID,
						RazorpayOrderID:   orderID,
						Metadata: map[string]interface{}{
							"source": "reconciler_payment_lookup",
						},
					})
				}
			}
		}
	}

	request, err = h.upgradeRequestStore.GetUpgradeRequestByID(ctx, upgradeRequestID)
	if err != nil {
		return storagepayment.UpgradeRequest{}, err
	}

	targetPlan := license.PlanType(request.ToPlanCode)
	if targetPlan.IsValid() {
		cutover, cutoverErr := h.replacementStore.GetByUpgradeRequestID(ctx, request.UpgradeRequestID)
		if cutoverErr == nil {
			if !strings.EqualFold(cutover.Status, storagepayment.UpgradeReplacementCutoverStatusCompleted) &&
				!strings.EqualFold(cutover.Status, storagepayment.UpgradeReplacementCutoverStatusManualReviewRequired) {
				if _, err := h.processUPIReplacementCutover(ctx, request, targetPlan, mode); err != nil {
					return storagepayment.UpgradeRequest{}, fmt.Errorf("process upi replacement cutover: %w", err)
				}
			}
		} else if !errors.Is(cutoverErr, storagepayment.ErrUpgradeReplacementCutoverNotFound) {
			return storagepayment.UpgradeRequest{}, fmt.Errorf("load replacement cutover for reconciliation: %w", cutoverErr)
		}
	}

	request, err = h.upgradeRequestStore.GetUpgradeRequestByID(ctx, upgradeRequestID)
	if err != nil {
		return storagepayment.UpgradeRequest{}, err
	}

	if !request.SubscriptionChangeConfirmed && request.RazorpaySubscriptionID.Valid && request.TargetQuantity.Valid {
		targetQty := int(request.TargetQuantity.Int64)
		if targetQty > 0 {
			rzpSub, subErr := payment.GetSubscriptionByID(mode, strings.TrimSpace(request.RazorpaySubscriptionID.String))
			if subErr == nil && rzpSub.Quantity >= targetQty {
				_, _ = h.upgradeRequestStore.MarkSubscriptionChangeConfirmed(ctx, storagepayment.MarkUpgradeSubscriptionConfirmedInput{
					UpgradeRequestID:       request.UpgradeRequestID,
					RazorpaySubscriptionID: request.RazorpaySubscriptionID.String,
					Metadata: map[string]interface{}{
						"source":          "reconciler_subscription_lookup",
						"quantity":        rzpSub.Quantity,
						"target_quantity": targetQty,
					},
				})
			}
		}
	}

	request, err = h.upgradeRequestStore.GetUpgradeRequestByID(ctx, upgradeRequestID)
	if err != nil {
		return storagepayment.UpgradeRequest{}, err
	}

	if strings.EqualFold(request.CurrentStatus, storagepayment.UpgradeRequestStatusResolved) {
		if err := h.applyResolvedUpgradeRequest(ctx, request); err != nil {
			return storagepayment.UpgradeRequest{}, err
		}
		request, err = h.upgradeRequestStore.GetUpgradeRequestByID(ctx, upgradeRequestID)
		if err != nil {
			return storagepayment.UpgradeRequest{}, err
		}
	}

	return request, nil
}

func (h *BillingActionsHandler) reconcilePendingUpgradeRequests(ctx context.Context, limit int) error {
	requests, err := h.upgradeRequestStore.ListRequestsForReconciliation(ctx, limit, time.Now().UTC().Add(-5*time.Second))
	if err != nil {
		return err
	}

	for _, req := range requests {
		if strings.EqualFold(req.CurrentStatus, storagepayment.UpgradeRequestStatusResolved) {
			if err := h.applyResolvedUpgradeRequest(ctx, req); err != nil {
				log.Printf("[upgrade-reconcile] apply-resolved failed request=%s org=%d: %v", req.UpgradeRequestID, req.OrgID, err)
			}
			continue
		}

		_, _ = h.upgradeRequestStore.MarkReconciliationRetrying(ctx, req.UpgradeRequestID, map[string]interface{}{"source": "scheduler_tick"})
		if _, recErr := h.reconcileUpgradeRequestNow(ctx, req.UpgradeRequestID); recErr != nil {
			log.Printf("[upgrade-reconcile] request=%s org=%d reconcile failed: %v", req.UpgradeRequestID, req.OrgID, recErr)
			if time.Since(req.CreatedAt) > 30*time.Minute {
				if _, cutoverErr := h.replacementStore.GetByUpgradeRequestID(ctx, req.UpgradeRequestID); cutoverErr == nil {
					_, _ = h.replacementStore.MarkManualReviewRequired(ctx, req.UpgradeRequestID, recErr.Error())
				}
				updated, markErr := h.upgradeRequestStore.MarkManualReviewRequired(ctx, req.UpgradeRequestID, recErr.Error(), map[string]interface{}{"source": "scheduler_timeout"})
				if markErr != nil {
					log.Printf("[upgrade-reconcile] request=%s org=%d mark manual review failed: %v", req.UpgradeRequestID, req.OrgID, markErr)
					continue
				}
				h.enqueueUpgradeFailureNotifications(ctx, updated, "manual_review_required", map[string]interface{}{
					"source": "scheduler_timeout",
					"error":  recErr.Error(),
				})
			}
		}
	}

	return nil
}

func (h *BillingActionsHandler) GetBillingStatus(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	if err := h.store.EnsureOrgBillingState(c.Request().Context(), orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	mode := resolveRazorpayModeForBilling()
	pricingProfile := ""
	if mode == "live" {
		profile, err := payment.ResolvePricingProfile()
		if err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("invalid pricing profile configuration: %v", err))
		}
		pricingProfile = profile
	}

	state, err := h.store.GetOrgBillingState(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to fetch billing state: %v", err))
	}
	now := time.Now().UTC()
	trialActive := false
	if state.TrialEndsAt.Valid {
		trialActive = now.Before(state.TrialEndsAt.Time.UTC())
	}
	trialCanCancel := trialActive && !state.TrialReadOnly

	plans := getSortedLOCPlans()
	defaultPurchaseCurrency := defaultPurchaseCurrencyForRequest(c.Request())
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"billing": map[string]interface{}{
			"current_plan_code":             state.CurrentPlanCode,
			"razorpay_mode":                 mode,
			"pricing_profile":               pricingProfile,
			"default_purchase_currency":     defaultPurchaseCurrency,
			"supported_purchase_currencies": supportedPurchaseCurrencies(),
			"billing_period_start":          state.BillingPeriodStart.Format(time.RFC3339),
			"billing_period_end":            state.BillingPeriodEnd.Format(time.RFC3339),
			"loc_used_month":                state.LOCUsedMonth,
			"trial_active":                  trialActive,
			"trial_started_at":              nullTime(state.TrialStartedAt),
			"trial_ends_at":                 nullTime(state.TrialEndsAt),
			"trial_readonly":                state.TrialReadOnly,
			"trial_can_cancel":              trialCanCancel,
			"scheduled_plan_code":           nullString(state.ScheduledPlanCode),
			"scheduled_plan_effective_at":   nullTime(state.ScheduledPlanEffectiveAt),
		},
		"available_plans": plans,
	})
}

func (h *BillingActionsHandler) GetUpgradeRequestStatus(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	ctx := c.Request().Context()
	upgradeRequestID := strings.TrimSpace(c.QueryParam("upgrade_request_id"))

	var request storagepayment.UpgradeRequest
	var err error
	if upgradeRequestID == "" {
		request, err = h.upgradeRequestStore.GetLatestUpgradeRequestByOrg(ctx, orgID)
	} else {
		request, err = h.upgradeRequestStore.GetUpgradeRequestByIDForOrg(ctx, orgID, upgradeRequestID)
	}
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
				"request": nil,
				"events":  []map[string]interface{}{},
			})
		}
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load upgrade request status: %v", err))
	}

	events, err := h.upgradeRequestStore.ListUpgradeRequestEvents(ctx, request.UpgradeRequestID, 50)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load upgrade request events: %v", err))
	}

	eventRows := make([]map[string]interface{}, 0, len(events))
	for _, ev := range events {
		row := map[string]interface{}{
			"event_source": ev.EventSource,
			"event_type":   ev.EventType,
			"event_time":   ev.EventTime.UTC().Format(time.RFC3339),
		}
		if ev.FromStatus.Valid {
			row["from_status"] = ev.FromStatus.String
		}
		if ev.ToStatus.Valid {
			row["to_status"] = ev.ToStatus.String
		}
		if len(ev.EventPayload) > 0 {
			var payload map[string]interface{}
			if unmarshalErr := json.Unmarshal(ev.EventPayload, &payload); unmarshalErr == nil {
				row["payload"] = payload
			}
		}
		eventRows = append(eventRows, row)
	}

	response := map[string]interface{}{
		"upgrade_request_id":               request.UpgradeRequestID,
		"org_id":                           request.OrgID,
		"from_plan_code":                   request.FromPlanCode,
		"to_plan_code":                     request.ToPlanCode,
		"expected_amount_cents":            request.ExpectedAmountCents,
		"currency":                         request.Currency,
		"status":                           request.CurrentStatus,
		"payment_capture_confirmed":        request.PaymentCaptureConfirmed,
		"subscription_change_confirmed":    request.SubscriptionChangeConfirmed,
		"plan_grant_applied":               request.PlanGrantApplied,
		"created_at":                       request.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":                       request.UpdatedAt.UTC().Format(time.RFC3339),
		"razorpay_order_id":                nullString(request.RazorpayOrderID),
		"razorpay_payment_id":              nullString(request.RazorpayPaymentID),
		"razorpay_subscription_id":         nullString(request.RazorpaySubscriptionID),
		"local_subscription_id":            nullInt64(request.LocalSubscriptionID),
		"target_quantity":                  nullInt64(request.TargetQuantity),
		"payment_capture_confirmed_at":     nullTime(request.PaymentCaptureConfirmedAt),
		"subscription_change_confirmed_at": nullTime(request.SubscriptionChangeConfirmedAt),
		"plan_grant_applied_at":            nullTime(request.PlanGrantAppliedAt),
		"resolved_at":                      nullTime(request.ResolvedAt),
	}

	customerState := h.buildCustomerUpgradeState(c.Request().Context(), request)
	for key, value := range customerState {
		response[key] = value
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"request": response,
		"events":  eventRows,
	})
}

func (h *BillingActionsHandler) UpgradePlan(c echo.Context) error {
	if _, _, err := h.requirePlanManager(c); err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", httpErr.Message)
			if msg == "" {
				msg = http.StatusText(httpErr.Code)
			}
			return JSONErrorWithEnvelope(c, httpErr.Code, msg)
		}
		return err
	}

	return JSONErrorWithEnvelope(c, http.StatusGone, "direct /billing/upgrade is deprecated; use /billing/upgrade/preview, /billing/upgrade/prepare-payment, and /billing/upgrade/execute")
}

func (h *BillingActionsHandler) ScheduleDowngrade(c echo.Context) error {
	orgID, actorUserID, err := h.requirePlanManager(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", httpErr.Message)
			if msg == "" {
				msg = http.StatusText(httpErr.Code)
			}
			return JSONErrorWithEnvelope(c, httpErr.Code, msg)
		}
		return err
	}

	var req planChangeRequest
	if err := c.Bind(&req); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid request body")
	}
	targetPlan := license.PlanType(strings.TrimSpace(req.TargetPlanCode))
	if !targetPlan.IsValid() {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid target_plan_code")
	}

	ctx := c.Request().Context()
	if err := h.store.EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}
	state, err := h.store.GetOrgBillingState(ctx, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to fetch billing state: %v", err))
	}

	currentPlan := license.PlanType(state.CurrentPlanCode)
	if targetPlan.GetLimits().MonthlyLOCLimit >= currentPlan.GetLimits().MonthlyLOCLimit {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "target_plan_code must be a lower LOC tier for downgrade")
	}

	effectiveAt := state.BillingPeriodEnd.UTC()
	payload := map[string]interface{}{
		"from_plan_code": currentPlan.String(),
		"to_plan_code":   targetPlan.String(),
		"effective_at":   effectiveAt.Format(time.RFC3339),
	}
	if err := h.syncRazorpayTransition(ctx, orgID, targetPlan, false, effectiveAt); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("razorpay transition scheduling failed: %v", err))
	}
	if err := h.store.ScheduleDowngrade(ctx, orgID, targetPlan.String(), effectiveAt, actorUserID, payload); err != nil {
		rollbackErr := h.syncRazorpayTransition(ctx, orgID, currentPlan, false, effectiveAt)
		if rollbackErr != nil {
			return JSONErrorWithEnvelope(
				c,
				http.StatusInternalServerError,
				fmt.Sprintf("failed to schedule downgrade after razorpay schedule update and rollback also failed (forward_err=%v rollback_err=%v)", err, rollbackErr),
			)
		}
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to schedule downgrade and razorpay scheduling was rolled back: %v", err))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"message":                "downgrade scheduled",
		"scheduled_plan_code":    targetPlan.String(),
		"scheduled_effective_at": effectiveAt.Format(time.RFC3339),
	})
}

func (h *BillingActionsHandler) CancelScheduledDowngrade(c echo.Context) error {
	orgID, actorUserID, err := h.requirePlanManager(c)
	if err != nil {
		if httpErr, ok := err.(*echo.HTTPError); ok {
			msg := fmt.Sprintf("%v", httpErr.Message)
			if msg == "" {
				msg = http.StatusText(httpErr.Code)
			}
			return JSONErrorWithEnvelope(c, httpErr.Code, msg)
		}
		return err
	}

	ctx := c.Request().Context()
	if err := h.store.EnsureOrgBillingState(ctx, orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	if err := h.store.CancelScheduledDowngrade(ctx, orgID, actorUserID, map[string]interface{}{"reason": "manual_cancel"}); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to cancel scheduled downgrade: %v", err))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"message": "scheduled downgrade cancelled",
	})
}

func (h *BillingActionsHandler) requirePlanManager(c echo.Context) (int64, int64, error) {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return 0, 0, echo.NewHTTPError(http.StatusBadRequest, "organization context required")
	}

	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil {
		return 0, 0, echo.NewHTTPError(http.StatusForbidden, "permission context required")
	}
	if !(permCtx.IsOwner || permCtx.IsSuperAdmin || strings.EqualFold(permCtx.Role, "admin")) {
		return 0, 0, echo.NewHTTPError(http.StatusForbidden, "only owner/admin can manage plan changes")
	}

	if permCtx.User == nil || permCtx.User.ID <= 0 {
		return 0, 0, echo.NewHTTPError(http.StatusForbidden, "authenticated user required")
	}

	return orgID, permCtx.User.ID, nil
}

func getSortedLOCPlans() []map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(license.PlanDefinitions))
	for code, limits := range license.PlanDefinitions {
		items = append(items, map[string]interface{}{
			"plan_code":         code.String(),
			"monthly_loc_limit": limits.MonthlyLOCLimit,
			"monthly_price_usd": limits.MonthlyPriceUSD,
			"trial_days":        limits.TrialDays,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		li, _ := items[i]["monthly_loc_limit"].(int)
		lj, _ := items[j]["monthly_loc_limit"].(int)
		return li < lj
	})
	return items
}

func nullString(v sql.NullString) interface{} {
	if !v.Valid || strings.TrimSpace(v.String) == "" {
		return nil
	}
	return v.String
}

func nullTime(v sql.NullTime) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Time.UTC().Format(time.RFC3339)
}

func nullInt64(v sql.NullInt64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

func runBillingTransitionScheduler(ctx context.Context, db *sql.DB, interval time.Duration) {
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	store := storagelicense.NewPlanChangeStore(db)
	subscriptionStore := storagepayment.NewSubscriptionStore(db)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expiryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			expired, expiryErr := subscriptionStore.ReconcileExpiredPendingCancellations(expiryCtx, 100)
			cancel()
			if expiryErr != nil {
				log.Printf("[billing-transition-scheduler] reconcile expired subscriptions failed: %v", expiryErr)
			} else if len(expired) > 0 {
				log.Printf("[billing-transition-scheduler] auto-reconciled %d expired subscription(s)", len(expired))
			}

			due, err := store.ListDueScheduledPlanChanges(ctx, time.Now().UTC(), 100)
			if err != nil {
				log.Printf("[billing-transition-scheduler] list due plan changes failed: %v", err)
				continue
			}

			handler := NewBillingActionsHandler(db)
			for _, tr := range due {
				transitionCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				err := applyDueScheduledPlanChangeWithRazorpay(transitionCtx, db, store, tr)
				cancel()
				if err != nil {
					log.Printf("[billing-transition-scheduler] org=%d target_plan=%s reconcile failed: %v", tr.OrgID, tr.TargetPlanCode, err)
				}
			}

			reconcileCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			if err := handler.reconcilePendingUpgradeRequests(reconcileCtx, 100); err != nil {
				log.Printf("[billing-transition-scheduler] upgrade request reconciliation failed: %v", err)
			}
			cancel()

			dispatchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			if err := dispatchBillingNotificationOutboxBatch(dispatchCtx, db, 100); err != nil {
				log.Printf("[billing-transition-scheduler] notification outbox dispatch finished with issues: %v", err)
			}
			cancel()
		}
	}
}

func applyDueDowngradeWithRazorpay(ctx context.Context, db *sql.DB, store *storagelicense.PlanChangeStore, tr storagelicense.DueTransition) error {
	return applyDueScheduledPlanChangeWithRazorpay(ctx, db, store, tr)
}

func applyDueScheduledPlanChangeWithRazorpay(ctx context.Context, db *sql.DB, store *storagelicense.PlanChangeStore, tr storagelicense.DueTransition) error {
	if db == nil {
		return fmt.Errorf("missing db handle")
	}
	if store == nil {
		return fmt.Errorf("missing plan change store")
	}

	targetPlan := license.PlanType(strings.TrimSpace(tr.TargetPlanCode))
	if !targetPlan.IsValid() {
		return fmt.Errorf("invalid target plan code: %s", tr.TargetPlanCode)
	}

	if err := syncRazorpayTransitionWithDB(ctx, db, tr.OrgID, targetPlan, false, tr.EffectiveAt); err != nil {
		return err
	}

	if err := store.ApplyScheduledPlanChange(ctx, tr); err != nil {
		return fmt.Errorf("apply scheduled plan change: %w", err)
	}

	return nil
}

func (h *BillingActionsHandler) GetUsageSummary(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	if err := h.store.EnsureOrgBillingState(c.Request().Context(), orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	summary, err := h.usageStore.GetCurrentPeriodSummary(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load usage summary: %v", err))
	}

	payload := map[string]interface{}{
		"period_start":         summary.PeriodStart.Format(time.RFC3339),
		"period_end":           summary.PeriodEnd.Format(time.RFC3339),
		"total_billable_loc":   summary.TotalBillableLOC,
		"total_input_tokens":   summary.TotalInputTokens,
		"total_output_tokens":  summary.TotalOutputTokens,
		"total_tokens":         summary.TotalInputTokens + summary.TotalOutputTokens,
		"total_cost_usd":       summary.TotalCostUSD,
		"accounted_operations": summary.AccountedOps,
		"token_tracked_ops":    summary.TokenTrackedOps,
	}
	if summary.LatestAccountedAt != nil {
		payload["latest_accounted_at"] = summary.LatestAccountedAt.UTC().Format(time.RFC3339)
	}

	return JSONWithEnvelope(c, http.StatusOK, payload)
}

func (h *BillingActionsHandler) GetUsageOperations(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil || permCtx.User == nil || permCtx.User.ID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "permission context required")
	}

	limit, offset, err := usagePaginationFromQuery(c, 25)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	var scopedActorUserID *int64
	if !(permCtx.IsOwner || permCtx.IsSuperAdmin || strings.EqualFold(permCtx.Role, "admin")) {
		memberUserID := permCtx.User.ID
		scopedActorUserID = &memberUserID
	}

	ops, err := h.usageStore.ListCurrentPeriodOperations(c.Request().Context(), orgID, scopedActorUserID, limit, offset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load usage operations: %v", err))
	}

	rows := make([]map[string]interface{}, 0, len(ops))
	for _, op := range ops {
		rows = append(rows, usageOperationRow(op))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"operations": rows,
		"limit":      limit,
		"offset":     offset,
		"count":      len(rows),
	})
}

func (h *BillingActionsHandler) GetUsageMembers(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil || permCtx.User == nil || permCtx.User.ID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "permission context required")
	}
	if !isBillingManager(permCtx) {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "only owner/admin can view member usage totals")
	}

	limit, offset, err := usagePaginationFromQuery(c, 25)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	items, err := h.usageStore.ListCurrentPeriodMemberUsage(c.Request().Context(), orgID, limit, offset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load member usage summary: %v", err))
	}

	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		rows = append(rows, usageMemberSummaryRow(item))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"members": rows,
		"limit":   limit,
		"offset":  offset,
		"count":   len(rows),
	})
}

func (h *BillingActionsHandler) GetMyUsage(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil || permCtx.User == nil || permCtx.User.ID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "permission context required")
	}

	item, err := h.usageStore.GetCurrentPeriodUsageForActor(c.Request().Context(), orgID, permCtx.User.ID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load member usage: %v", err))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"member": usageMemberSummaryRow(item),
	})
}

func (h *BillingActionsHandler) GetMemberUsageOperations(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	permCtx := auth.GetPermissionContext(c)
	if permCtx == nil || permCtx.User == nil || permCtx.User.ID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "permission context required")
	}

	memberID, err := strconv.ParseInt(strings.TrimSpace(c.Param("member_id")), 10, 64)
	if err != nil || memberID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid member_id")
	}

	if !isBillingManager(permCtx) && permCtx.User.ID != memberID {
		return JSONErrorWithEnvelope(c, http.StatusForbidden, "members can only access their own usage operations")
	}

	limit, offset, err := usagePaginationFromQuery(c, 25)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	actorID := memberID
	ops, err := h.usageStore.ListCurrentPeriodOperations(c.Request().Context(), orgID, &actorID, limit, offset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load member usage operations: %v", err))
	}

	rows := make([]map[string]interface{}, 0, len(ops))
	for _, op := range ops {
		rows = append(rows, usageOperationRow(op))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"member_id":  memberID,
		"operations": rows,
		"limit":      limit,
		"offset":     offset,
		"count":      len(rows),
	})
}

func (h *BillingActionsHandler) GetAdminBillingPortfolioSummary(c echo.Context) error {
	summary, err := h.portfolioStore.GetSummary(c.Request().Context())
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load billing portfolio summary: %v", err))
	}

	payload := map[string]interface{}{
		"total_orgs":          summary.TotalOrgs,
		"active_orgs":         summary.ActiveOrgs,
		"total_billable_loc":  summary.TotalBillableLOC,
		"total_operations":    summary.TotalOperations,
		"net_collected_cents": summary.NetCollectedCents,
		"failed_payments":     summary.FailedPayments,
	}
	if summary.LastAccountedAt.Valid {
		payload["last_accounted_at"] = summary.LastAccountedAt.Time.UTC().Format(time.RFC3339)
	}

	return JSONWithEnvelope(c, http.StatusOK, payload)
}

func (h *BillingActionsHandler) ListAdminBillingPortfolioOrganizations(c echo.Context) error {
	limit, offset, err := usagePaginationFromQuery(c, 25)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	items, err := h.portfolioStore.ListOrganizations(c.Request().Context(), limit, offset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load billing portfolio organizations: %v", err))
	}

	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		row := map[string]interface{}{
			"org_id":              item.OrgID,
			"org_name":            item.OrgName,
			"current_plan_code":   nullString(item.CurrentPlanCode),
			"loc_used_month":      nullInt64(item.LOCUsedMonth),
			"loc_blocked":         boolValue(item.LOCBlocked),
			"billing_period_end":  nullTime(item.BillingPeriodEnd),
			"total_billable_loc":  item.TotalBillableLOC,
			"operation_count":     item.OperationCount,
			"last_accounted_at":   nullTime(item.LastAccountedAt),
			"net_collected_cents": item.NetCollectedCents,
			"failed_payments":     item.FailedPayments,
		}
		rows = append(rows, row)
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"organizations": rows,
		"limit":         limit,
		"offset":        offset,
		"count":         len(rows),
	})
}

func (h *BillingActionsHandler) GetAdminOrganizationBillingMembers(c echo.Context) error {
	orgID, err := strconv.ParseInt(strings.TrimSpace(c.Param("org_id")), 10, 64)
	if err != nil || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid org_id")
	}

	exists, err := h.portfolioStore.OrganizationExists(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to validate organization: %v", err))
	}
	if !exists {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "organization not found")
	}

	if err := h.store.EnsureOrgBillingState(c.Request().Context(), orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	limit, offset, err := usagePaginationFromQuery(c, 25)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	items, err := h.usageStore.ListCurrentPeriodMemberUsage(c.Request().Context(), orgID, limit, offset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load org member usage summary: %v", err))
	}

	rows := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		rows = append(rows, usageMemberSummaryRow(item))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"org_id":  orgID,
		"members": rows,
		"limit":   limit,
		"offset":  offset,
		"count":   len(rows),
	})
}

func (h *BillingActionsHandler) GetAdminOrganizationBillingUsage(c echo.Context) error {
	orgID, err := strconv.ParseInt(strings.TrimSpace(c.Param("org_id")), 10, 64)
	if err != nil || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid org_id")
	}

	exists, err := h.portfolioStore.OrganizationExists(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to validate organization: %v", err))
	}
	if !exists {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "organization not found")
	}

	if err := h.store.EnsureOrgBillingState(c.Request().Context(), orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	opsLimit, opsOffset, err := usagePaginationFromQuery(c, 25)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, err.Error())
	}

	summary, err := h.usageStore.GetCurrentPeriodSummary(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load org usage summary: %v", err))
	}

	memberUsage, err := h.usageStore.ListCurrentPeriodMemberUsage(c.Request().Context(), orgID, 20, 0)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load org member usage summary: %v", err))
	}

	ops, err := h.usageStore.ListCurrentPeriodOperations(c.Request().Context(), orgID, nil, opsLimit, opsOffset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load org usage operations: %v", err))
	}

	memberRows := make([]map[string]interface{}, 0, len(memberUsage))
	for _, item := range memberUsage {
		memberRows = append(memberRows, usageMemberSummaryRow(item))
	}

	operationRows := make([]map[string]interface{}, 0, len(ops))
	for _, op := range ops {
		operationRows = append(operationRows, usageOperationRow(op))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"org_id": orgID,
		"summary": map[string]interface{}{
			"period_start":         summary.PeriodStart.Format(time.RFC3339),
			"period_end":           summary.PeriodEnd.Format(time.RFC3339),
			"total_billable_loc":   summary.TotalBillableLOC,
			"total_input_tokens":   summary.TotalInputTokens,
			"total_output_tokens":  summary.TotalOutputTokens,
			"total_tokens":         summary.TotalInputTokens + summary.TotalOutputTokens,
			"total_cost_usd":       summary.TotalCostUSD,
			"accounted_operations": summary.AccountedOps,
			"token_tracked_ops":    summary.TokenTrackedOps,
			"latest_accounted_at":  timeValue(summary.LatestAccountedAt),
		},
		"members": map[string]interface{}{
			"items": memberRows,
			"count": len(memberRows),
		},
		"operations": map[string]interface{}{
			"items":  operationRows,
			"limit":  opsLimit,
			"offset": opsOffset,
			"count":  len(operationRows),
		},
	})
}

func usagePaginationFromQuery(c echo.Context, defaultLimit int) (int, int, error) {
	limit := defaultLimit
	if limit <= 0 {
		limit = 25
	}
	if v := strings.TrimSpace(c.QueryParam("limit")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("invalid limit")
		}
		limit = parsed
	}

	offset := 0
	if v := strings.TrimSpace(c.QueryParam("offset")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("invalid offset")
		}
		offset = parsed
	}

	return limit, offset, nil
}

func usageOperationRow(op storagelicense.OrgUsageOperation) map[string]interface{} {
	row := map[string]interface{}{
		"operation_type": op.OperationType,
		"trigger_source": op.TriggerSource,
		"operation_id":   op.OperationID,
		"billable_loc":   op.BillableLOC,
		"accounted_at":   op.AccountedAt.Format(time.RFC3339),
	}
	if op.ReviewID.Valid {
		row["review_id"] = op.ReviewID.Int64
	}
	if op.UserID.Valid {
		row["user_id"] = op.UserID.Int64
	}
	if op.ActorEmail.Valid {
		row["actor_email"] = op.ActorEmail.String
	}
	if op.ActorKind.Valid {
		row["actor_kind"] = op.ActorKind.String
	}
	if op.Provider.Valid {
		row["provider"] = op.Provider.String
	}
	if op.Model.Valid {
		row["model"] = op.Model.String
	}
	if op.InputTokens.Valid {
		row["input_tokens"] = op.InputTokens.Int64
	}
	if op.OutputTokens.Valid {
		row["output_tokens"] = op.OutputTokens.Int64
	}
	if op.CostUSD.Valid {
		row["cost_usd"] = op.CostUSD.Float64
	}
	return row
}

func usageMemberSummaryRow(item storagelicense.OrgMemberUsageSummary) map[string]interface{} {
	share := 0.0
	if item.OrgTotalBillableLOC > 0 {
		share = (float64(item.TotalBillableLOC) / float64(item.OrgTotalBillableLOC)) * 100.0
	}

	return map[string]interface{}{
		"user_id":                nullInt64(item.UserID),
		"actor_email":            nullString(item.ActorEmail),
		"actor_kind":             item.ActorKind,
		"total_billable_loc":     item.TotalBillableLOC,
		"operation_count":        item.OperationCount,
		"last_accounted_at":      nullTime(item.LastAccountedAt),
		"org_total_billable_loc": item.OrgTotalBillableLOC,
		"usage_share_percent":    share,
	}
}

func isBillingManager(permCtx *auth.PermissionContext) bool {
	if permCtx == nil {
		return false
	}
	return permCtx.IsOwner || permCtx.IsSuperAdmin || strings.EqualFold(permCtx.Role, "admin")
}

func boolValue(v sql.NullBool) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Bool
}

func timeValue(v *time.Time) interface{} {
	if v == nil {
		return nil
	}
	return v.UTC().Format(time.RFC3339)
}

func (h *BillingActionsHandler) enqueueUpgradeFailureNotifications(ctx context.Context, request storagepayment.UpgradeRequest, eventType string, metadata map[string]interface{}) {
	if h.notificationStore == nil {
		return
	}

	payload := map[string]interface{}{
		"upgrade_request_id": request.UpgradeRequestID,
		"org_id":             request.OrgID,
		"from_plan_code":     request.FromPlanCode,
		"to_plan_code":       request.ToPlanCode,
		"status":             request.CurrentStatus,
		"event_type":         strings.TrimSpace(eventType),
		"support_reference":  request.UpgradeRequestID,
		"triggered_at":       time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range metadata {
		payload[k] = v
	}

	var recipientUserID *int64
	if request.ActorUserID > 0 {
		uid := request.ActorUserID
		recipientUserID = &uid
	}

	base := fmt.Sprintf("%s:%s", strings.TrimSpace(eventType), strings.TrimSpace(request.UpgradeRequestID))
	if _, err := h.notificationStore.Enqueue(ctx, storagepayment.CreateBillingNotificationInput{
		OrgID:           request.OrgID,
		EventType:       strings.TrimSpace(eventType),
		Channel:         "in_app",
		DedupeKey:       base + ":in_app",
		Payload:         payload,
		RecipientUserID: recipientUserID,
	}); err != nil {
		log.Printf("[billing-notify] enqueue in_app failed request=%s org=%d: %v", request.UpgradeRequestID, request.OrgID, err)
	}

	if recipientUserID == nil {
		return
	}

	email, err := h.notificationStore.GetUserEmailByID(ctx, *recipientUserID)
	if err != nil {
		log.Printf("[billing-notify] resolve recipient email failed request=%s user=%d: %v", request.UpgradeRequestID, *recipientUserID, err)
		return
	}
	if strings.TrimSpace(email) == "" {
		return
	}

	if _, err := h.notificationStore.Enqueue(ctx, storagepayment.CreateBillingNotificationInput{
		OrgID:           request.OrgID,
		EventType:       strings.TrimSpace(eventType),
		Channel:         "email",
		DedupeKey:       base + ":email",
		Payload:         payload,
		RecipientUserID: recipientUserID,
		RecipientEmail:  email,
	}); err != nil {
		log.Printf("[billing-notify] enqueue email failed request=%s org=%d: %v", request.UpgradeRequestID, request.OrgID, err)
	}
}

func (h *BillingActionsHandler) buildCustomerUpgradeState(ctx context.Context, request storagepayment.UpgradeRequest) map[string]interface{} {
	now := time.Now().UTC()
	state := map[string]interface{}{
		"customer_state": "processing",
		"action_required": map[string]interface{}{
			"type": "none",
		},
	}

	cutover, cutoverErr := h.replacementStore.GetByUpgradeRequestID(ctx, request.UpgradeRequestID)
	hasCutover := cutoverErr == nil
	cutoverPending := hasCutover &&
		!strings.EqualFold(cutover.Status, storagepayment.UpgradeReplacementCutoverStatusCompleted) &&
		!strings.EqualFold(cutover.Status, storagepayment.UpgradeReplacementCutoverStatusManualReviewRequired)
	if hasCutover {
		state["fulfillment_strategy"] = "replacement_subscription_cutover"
		state["replacement_cutover"] = map[string]interface{}{
			"status":           cutover.Status,
			"cutover_at":       cutover.CutoverAt.UTC().Format(time.RFC3339),
			"target_plan_code": cutover.TargetPlanCode,
			"target_quantity":  cutover.TargetQuantity,
			"retry_count":      cutover.RetryCount,
			"next_retry_at":    nullTime(cutover.NextRetryAt),
			"last_error":       nullString(cutover.LastError),
		}
	} else if !errors.Is(cutoverErr, storagepayment.ErrUpgradeReplacementCutoverNotFound) {
		log.Printf("[billing-upgrade] warning: load replacement cutover state request=%s org=%d: %v", request.UpgradeRequestID, request.OrgID, cutoverErr)
	}

	status := strings.TrimSpace(strings.ToLower(request.CurrentStatus))
	delayedConfirmationStatus := false
	switch status {
	case storagepayment.UpgradeRequestStatusCreated,
		storagepayment.UpgradeRequestStatusPaymentOrderCreated,
		storagepayment.UpgradeRequestStatusWaitingForCapture:
		state["customer_state"] = "awaiting_payment"
	case storagepayment.UpgradeRequestStatusPaymentCaptureConfirmed,
		storagepayment.UpgradeRequestStatusSubscriptionUpdateRequested,
		storagepayment.UpgradeRequestStatusWaitingForSubscription,
		storagepayment.UpgradeRequestStatusSubscriptionConfirmed,
		storagepayment.UpgradeRequestStatusReconciliationRetrying:
		state["customer_state"] = "processing"
		delayedConfirmationStatus = true
	case storagepayment.UpgradeRequestStatusResolved:
		state["customer_state"] = "completed"
	case storagepayment.UpgradeRequestStatusFailed:
		state["customer_state"] = "failed"
		state["action_required"] = map[string]interface{}{
			"type":                      "contact_support",
			"sla_hours":                 24,
			"support_sla_business_days": 3,
		}
	case storagepayment.UpgradeRequestStatusManualReviewRequired:
		state["customer_state"] = "manual_review_required"
		state["action_required"] = map[string]interface{}{
			"type":                      "contact_support",
			"sla_hours":                 24,
			"support_sla_business_days": 3,
		}
	}

	actionNeededAt := request.UpdatedAt.UTC()
	attempt, err := h.paymentAttemptStore.GetLatestAttemptByUpgradeRequestID(ctx, request.UpgradeRequestID)
	if err == nil {
		if strings.EqualFold(strings.TrimSpace(attempt.Status), "payment_failed") {
			state["customer_state"] = "payment_failed"
			state["action_required"] = map[string]interface{}{
				"type":                      "retry_payment",
				"endpoint":                  "/api/v1/billing/upgrade/prepare-payment",
				"sla_hours":                 24,
				"support_sla_business_days": 3,
			}
			state["latest_payment_error"] = map[string]interface{}{
				"code":        nullString(attempt.ErrorCode),
				"reason":      nullString(attempt.ErrorReason),
				"description": nullString(attempt.ErrorDescription),
			}
			if attempt.PaymentFailedAt.Valid {
				actionNeededAt = attempt.PaymentFailedAt.Time.UTC()
			} else {
				actionNeededAt = request.UpdatedAt.UTC()
			}
		}
	}

	if delayedConfirmationStatus && !cutoverPending {
		delayedThresholdAt := request.UpdatedAt.UTC().Add(10 * time.Minute)
		if now.After(delayedThresholdAt) {
			state["customer_state"] = "action_needed"
			state["action_required"] = map[string]interface{}{
				"type":                      "confirm_payment_and_contact_support",
				"support_sla_business_days": 3,
				"retry_endpoint":            "/api/v1/billing/upgrade/request-status",
				"delay_minutes":             10,
			}
			actionNeededAt = delayedThresholdAt
		}
	}

	if cutoverPending {
		state["customer_state"] = "processing"
		state["action_required"] = map[string]interface{}{
			"type":           "wait_for_cutover",
			"retry_endpoint": "/api/v1/billing/upgrade/request-status",
		}
		actionNeededAt = cutover.CutoverAt.UTC()
	}

	state["action_needed_at"] = actionNeededAt.Format(time.RFC3339)
	state["support_reference"] = request.UpgradeRequestID
	state["support_context"] = map[string]interface{}{
		"upgrade_request_id":        request.UpgradeRequestID,
		"razorpay_order_id":         nullString(request.RazorpayOrderID),
		"razorpay_payment_id":       nullString(request.RazorpayPaymentID),
		"razorpay_subscription_id":  nullString(request.RazorpaySubscriptionID),
		"dispute_sla_business_days": 3,
	}

	return state
}

func (h *BillingActionsHandler) syncRazorpayTransition(ctx context.Context, orgID int64, targetPlan license.PlanType, immediate bool, periodEnd time.Time) error {
	return syncRazorpayTransitionWithDB(ctx, h.db, orgID, targetPlan, immediate, periodEnd)
}

func isUPIPaymentMethod(paymentMethod string) bool {
	return strings.EqualFold(strings.TrimSpace(paymentMethod), "upi")
}

func isRazorpayUPISubscriptionUpdateError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "subscriptions cannot be updated when payment mode is upi")
}

func (h *BillingActionsHandler) processUPIReplacementCutover(
	ctx context.Context,
	request storagepayment.UpgradeRequest,
	targetPlan license.PlanType,
	mode string,
) (storagepayment.UpgradeRequest, error) {
	activeSubscription, err := resolveActiveOrgSubscription(h.db, request.OrgID)
	if err != nil {
		return storagepayment.UpgradeRequest{}, fmt.Errorf("resolve active subscription for replacement cutover: %w", err)
	}

	cutoverAt := activeSubscription.CurrentPeriodEnd.UTC()
	if cutoverAt.IsZero() {
		cutoverAt = time.Now().UTC().Add(30 * 24 * time.Hour)
	}

	cutover, err := h.replacementStore.CreateOrGetPending(ctx, storagepayment.CreateUpgradeReplacementCutoverInput{
		UpgradeRequestID:          request.UpgradeRequestID,
		OrgID:                     request.OrgID,
		OwnerUserID:               activeSubscription.OwnerUserID,
		OldLocalSubscriptionID:    activeSubscription.ID,
		OldRazorpaySubscriptionID: activeSubscription.RazorpaySubscriptionID,
		TargetPlanCode:            targetPlan.String(),
		TargetQuantity:            locPlanToQuantity(targetPlan),
		Currency:                  request.Currency,
		CutoverAt:                 cutoverAt,
	})
	if err != nil {
		return storagepayment.UpgradeRequest{}, err
	}

	if strings.EqualFold(cutover.Status, storagepayment.UpgradeReplacementCutoverStatusCompleted) {
		updated, err := h.upgradeRequestStore.GetUpgradeRequestByID(ctx, request.UpgradeRequestID)
		if err != nil {
			return storagepayment.UpgradeRequest{}, fmt.Errorf("reload completed replacement cutover request: %w", err)
		}
		return updated, nil
	}
	if strings.EqualFold(cutover.Status, storagepayment.UpgradeReplacementCutoverStatusManualReviewRequired) {
		return storagepayment.UpgradeRequest{}, fmt.Errorf("replacement cutover requires manual review")
	}

	cutover, err = h.provisionUPIReplacementCutover(ctx, cutover, mode)
	if err != nil {
		_, _ = h.replacementStore.MarkRetryPending(ctx, request.UpgradeRequestID, err.Error(), time.Now().UTC().Add(2*time.Minute))
		return storagepayment.UpgradeRequest{}, err
	}

	localReplacementID := int64(0)
	if cutover.ReplacementLocalSubscriptionID.Valid {
		localReplacementID = cutover.ReplacementLocalSubscriptionID.Int64
	}
	replacementRazorpaySubscriptionID := strings.TrimSpace(cutover.ReplacementRazorpaySubscriptionID.String)
	if _, err := h.upgradeRequestStore.MarkSubscriptionUpdateRequested(ctx, storagepayment.MarkUpgradeSubscriptionUpdateInput{
		UpgradeRequestID:       request.UpgradeRequestID,
		LocalSubscriptionID:    localReplacementID,
		RazorpaySubscriptionID: replacementRazorpaySubscriptionID,
		TargetQuantity:         cutover.TargetQuantity,
		Metadata: map[string]interface{}{
			"source":             "upi_replacement_cutover",
			"fulfillment_mode":   "replacement_subscription_cutover",
			"old_subscription":   cutover.OldRazorpaySubscriptionID,
			"replacement_sub_id": replacementRazorpaySubscriptionID,
			"cutover_at":         cutover.CutoverAt.UTC().Format(time.RFC3339),
		},
	}); err != nil && !errors.Is(err, storagepayment.ErrUpgradeRequestTransitionRejected) {
		return storagepayment.UpgradeRequest{}, fmt.Errorf("mark subscription update requested for replacement cutover: %w", err)
	}

	if _, err := h.upgradeRequestStore.MarkSubscriptionChangeConfirmed(ctx, storagepayment.MarkUpgradeSubscriptionConfirmedInput{
		UpgradeRequestID:       request.UpgradeRequestID,
		RazorpaySubscriptionID: replacementRazorpaySubscriptionID,
		Metadata: map[string]interface{}{
			"source":           "upi_replacement_cutover",
			"fulfillment_mode": "replacement_subscription_cutover",
			"cutover_at":       cutover.CutoverAt.UTC().Format(time.RFC3339),
		},
	}); err != nil && !errors.Is(err, storagepayment.ErrUpgradeRequestTransitionRejected) {
		return storagepayment.UpgradeRequest{}, fmt.Errorf("mark subscription confirmed for replacement cutover: %w", err)
	}

	_, _ = h.replacementStore.MarkCompleted(ctx, request.UpgradeRequestID)

	updated, err := h.upgradeRequestStore.GetUpgradeRequestByID(ctx, request.UpgradeRequestID)
	if err != nil {
		return storagepayment.UpgradeRequest{}, fmt.Errorf("reload upgrade request after replacement cutover: %w", err)
	}

	return updated, nil
}

func (h *BillingActionsHandler) provisionUPIReplacementCutover(
	ctx context.Context,
	cutover storagepayment.UpgradeReplacementCutover,
	mode string,
) (storagepayment.UpgradeReplacementCutover, error) {
	current := cutover
	subStore := storagepayment.NewSubscriptionStore(h.db)

	if !current.ReplacementRazorpaySubscriptionID.Valid || strings.TrimSpace(current.ReplacementRazorpaySubscriptionID.String) == "" {
		planID, err := payment.GetPlanID(mode, "monthly", current.Currency)
		if err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("resolve replacement monthly plan id: %w", err)
		}
		planID = strings.TrimSpace(planID)
		if planID == "" {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("replacement monthly plan id is empty for mode=%s currency=%s", mode, current.Currency)
		}

		notes := map[string]string{
			"org_id":             strconv.FormatInt(current.OrgID, 10),
			"owner_user_id":      strconv.FormatInt(current.OwnerUserID, 10),
			"upgrade_request_id": current.UpgradeRequestID,
			"target_plan_code":   current.TargetPlanCode,
			"cutover_at":         current.CutoverAt.UTC().Format(time.RFC3339),
			"flow":               "replacement_subscription_cutover",
		}

		replacementSub, err := payment.CreateSubscriptionAt(mode, planID, current.TargetQuantity, notes, current.CutoverAt.UTC().Unix())
		if err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("create replacement subscription: %w", err)
		}

		periodStart := current.CutoverAt.UTC()
		if replacementSub.CurrentStart > 0 {
			periodStart = time.Unix(replacementSub.CurrentStart, 0).UTC()
		}
		periodEnd := periodStart.AddDate(0, 1, 0)
		if replacementSub.CurrentEnd > 0 {
			periodEnd = time.Unix(replacementSub.CurrentEnd, 0).UTC()
		}

		if err := subStore.CreateTeamSubscriptionRecord(storagepayment.CreateTeamSubscriptionRecordInput{
			SubscriptionID:     replacementSub.ID,
			OwnerUserID:        int(current.OwnerUserID),
			OrgID:              int(current.OrgID),
			DBPlanType:         current.TargetPlanCode,
			Quantity:           current.TargetQuantity,
			Status:             replacementSub.Status,
			RazorpayPlanID:     planID,
			CurrentPeriodStart: periodStart,
			CurrentPeriodEnd:   periodEnd,
			LicenseExpiresAt:   periodEnd,
			ShortURL:           replacementSub.ShortURL,
			Notes:              notes,
		}); err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("persist replacement subscription record: %w", err)
		}

		replacementDetails, err := subStore.GetSubscriptionDetailsRow(replacementSub.ID)
		if err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("load replacement subscription record: %w", err)
		}

		current, err = h.replacementStore.MarkReplacementProvisioned(ctx, storagepayment.MarkReplacementProvisionedInput{
			UpgradeRequestID:                  current.UpgradeRequestID,
			ReplacementLocalSubscriptionID:    replacementDetails.ID,
			ReplacementRazorpaySubscriptionID: replacementSub.ID,
		})
		if err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("mark replacement cutover provisioned: %w", err)
		}
	}

	if current.ReplacementLocalSubscriptionID.Valid && current.OldLocalSubscriptionID > 0 {
		if _, err := subStore.RepointOrgActiveSubscription(ctx, storagepayment.RepointOrgActiveSubscriptionInput{
			OrgID:                          current.OrgID,
			OldLocalSubscriptionID:         current.OldLocalSubscriptionID,
			ReplacementLocalSubscriptionID: current.ReplacementLocalSubscriptionID.Int64,
		}); err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("repoint active subscription to replacement: %w", err)
		}
	}

	if !current.OldCancellationScheduled {
		svc := payment.NewSubscriptionService(h.db)
		if _, err := svc.CancelSubscriptionWithContext(ctx, current.OldRazorpaySubscriptionID, false, mode); err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("schedule old subscription cancellation at cycle end: %w", err)
		}

		var err error
		current, err = h.replacementStore.MarkOldCancellationScheduled(ctx, current.UpgradeRequestID)
		if err != nil {
			return storagepayment.UpgradeReplacementCutover{}, fmt.Errorf("mark old cancellation scheduled: %w", err)
		}
	}

	return current, nil
}

func (h *BillingActionsHandler) isUpgradeBlockedByRecurringPaymentMethod(ctx context.Context, orgID int64) (bool, string, error) {
	activeSub, err := resolveActiveOrgSubscription(h.db, orgID)
	if err != nil {
		return false, "", err
	}

	subStore := storagepayment.NewSubscriptionStore(h.db)
	paymentMethod, err := subStore.GetLatestCapturedPaymentMethodBySubscriptionID(ctx, activeSub.ID)
	if err != nil {
		return false, "", err
	}

	if isUPIPaymentMethod(paymentMethod) {
		return true, strings.ToLower(strings.TrimSpace(paymentMethod)), nil
	}

	return false, strings.TrimSpace(paymentMethod), nil
}

func syncRazorpayTransitionWithDB(ctx context.Context, db *sql.DB, orgID int64, targetPlan license.PlanType, immediate bool, periodEnd time.Time) error {
	if db == nil {
		return fmt.Errorf("missing db handle")
	}
	subStore := storagepayment.NewSubscriptionStore(db)
	subscriptions, err := subStore.ListSubscriptionsByOrgID(int(orgID))
	if err != nil {
		return fmt.Errorf("load org subscriptions: %w", err)
	}
	if len(subscriptions) == 0 {
		return fmt.Errorf("%w: organization has no active subscription", errRazorpayCheckoutRequired)
	}

	active := subscriptions[0]
	for _, s := range subscriptions {
		if strings.EqualFold(s.Status, "active") {
			active = s
			break
		}
	}
	if strings.TrimSpace(active.RazorpaySubscriptionID) == "" {
		return fmt.Errorf("no razorpay subscription id for organization")
	}

	mode := strings.TrimSpace(os.Getenv("RAZORPAY_MODE"))
	if mode == "" {
		mode = "test"
	}

	razorpaySub, err := payment.GetSubscriptionByID(mode, active.RazorpaySubscriptionID)
	if err != nil {
		return fmt.Errorf("fetch razorpay subscription: %w", err)
	}

	quantity := locPlanToQuantity(targetPlan)
	scheduleAt := int64(0)
	if immediate {
		scheduleAt = -1
	} else {
		if razorpaySub != nil && razorpaySub.CurrentEnd > 0 {
			scheduleAt = razorpaySub.CurrentEnd
		} else {
			scheduleAt = periodEnd.UTC().Unix()
		}
	}

	svc := payment.NewSubscriptionService(db)
	if _, err := svc.UpdateQuantity(active.RazorpaySubscriptionID, quantity, scheduleAt, mode); err != nil {
		return fmt.Errorf("update razorpay subscription quantity: %w", err)
	}

	return nil
}
func locPlanToQuantity(plan license.PlanType) int {
	limits := plan.GetLimits()
	if limits.MonthlyPriceUSD <= 0 {
		return 1
	}
	q := limits.MonthlyPriceUSD / 32
	if q < 1 {
		q = 1
	}
	return q
}

func computeProratedDeltaCents(fromMonthlyUSD, toMonthlyUSD int, cycleStart, cycleEnd, now time.Time) (int64, float64) {
	deltaMonthlyCents := int64((toMonthlyUSD - fromMonthlyUSD) * 100)
	if deltaMonthlyCents <= 0 {
		return 0, 0
	}

	if !cycleEnd.After(cycleStart) {
		return deltaMonthlyCents, 1
	}

	if now.Before(cycleStart) {
		now = cycleStart
	}
	if !now.Before(cycleEnd) {
		return deltaMonthlyCents, 1
	}

	cycleSeconds := cycleEnd.Sub(cycleStart).Seconds()
	remainingSeconds := cycleEnd.Sub(now).Seconds()
	if cycleSeconds <= 0 || remainingSeconds <= 0 {
		return 0, 0
	}

	fraction := remainingSeconds / cycleSeconds
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}

	chargeCents := int64(math.Round(float64(deltaMonthlyCents) * fraction))
	if chargeCents <= 0 {
		chargeCents = 1
	}

	return chargeCents, fraction
}

func applyImmediateUpgradeProrationCharge(
	ctx context.Context,
	db *sql.DB,
	orgID int64,
	currentPlan license.PlanType,
	targetPlan license.PlanType,
	fallbackCycleStart time.Time,
	fallbackCycleEnd time.Time,
) (map[string]interface{}, error) {
	if db == nil {
		return nil, fmt.Errorf("missing db handle")
	}

	subStore := storagepayment.NewSubscriptionStore(db)
	subscriptions, err := subStore.ListSubscriptionsByOrgID(int(orgID))
	if err != nil {
		return nil, fmt.Errorf("load org subscriptions: %w", err)
	}
	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("%w: organization has no active subscription", errRazorpayCheckoutRequired)
	}

	active := subscriptions[0]
	for _, s := range subscriptions {
		if strings.EqualFold(s.Status, "active") {
			active = s
			break
		}
	}
	if strings.TrimSpace(active.RazorpaySubscriptionID) == "" {
		return nil, fmt.Errorf("%w: no razorpay subscription id", errRazorpayCheckoutRequired)
	}

	mode := strings.TrimSpace(os.Getenv("RAZORPAY_MODE"))
	if mode == "" {
		mode = "test"
	}

	cycleStart := fallbackCycleStart.UTC()
	cycleEnd := fallbackCycleEnd.UTC()
	razorpaySub, err := payment.GetSubscriptionByID(mode, active.RazorpaySubscriptionID)
	if err == nil {
		if razorpaySub.CurrentStart > 0 {
			cycleStart = time.Unix(razorpaySub.CurrentStart, 0).UTC()
		}
		if razorpaySub.CurrentEnd > 0 {
			cycleEnd = time.Unix(razorpaySub.CurrentEnd, 0).UTC()
		}
	}

	chargeCents, fraction := computeProratedDeltaCents(
		currentPlan.GetLimits().MonthlyPriceUSD,
		targetPlan.GetLimits().MonthlyPriceUSD,
		cycleStart,
		cycleEnd,
		time.Now().UTC(),
	)

	details := map[string]interface{}{
		"mode":                     "manual_prorated_addon",
		"from_plan_code":           currentPlan.String(),
		"to_plan_code":             targetPlan.String(),
		"cycle_start":              cycleStart.Format(time.RFC3339),
		"cycle_end":                cycleEnd.Format(time.RFC3339),
		"remaining_cycle_fraction": math.Round(fraction*10000) / 10000,
		"charge_amount_cents":      chargeCents,
		"charge_currency":          "USD",
	}

	if chargeCents <= 0 {
		details["charge_status"] = "skipped"
		return details, nil
	}

	addon, err := payment.CreateSubscriptionAddon(mode, active.RazorpaySubscriptionID, payment.RazorpayAddonItem{
		Name:        "LiveReview Prorated Upgrade",
		Amount:      chargeCents,
		Currency:    "USD",
		Description: fmt.Sprintf("Prorated upgrade from %s to %s", currentPlan.String(), targetPlan.String()),
	})
	if err != nil {
		return nil, fmt.Errorf("create prorated add-on charge: %w", err)
	}

	details["charge_status"] = "created"
	details["addon_id"] = addon.ID

	return details, nil
}
