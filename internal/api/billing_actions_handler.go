package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/license/payment"
	storagelicense "github.com/livereview/storage/license"
	storagepayment "github.com/livereview/storage/payment"
)

type BillingActionsHandler struct {
	store      *storagelicense.PlanChangeStore
	usageStore *storagelicense.OrgUsageStore
	db         *sql.DB
}

var errRazorpayCheckoutRequired = errors.New("razorpay checkout required")

func NewBillingActionsHandler(db *sql.DB) *BillingActionsHandler {
	return &BillingActionsHandler{
		store:      storagelicense.NewPlanChangeStore(db),
		usageStore: storagelicense.NewOrgUsageStore(db),
		db:         db,
	}
}

type planChangeRequest struct {
	TargetPlanCode string `json:"target_plan_code"`
}

func (h *BillingActionsHandler) GetBillingStatus(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok || orgID <= 0 {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "organization context required")
	}

	if err := h.store.EnsureOrgBillingState(c.Request().Context(), orgID, license.PlanFree30K.String()); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to initialize billing state: %v", err))
	}

	state, err := h.store.GetOrgBillingState(c.Request().Context(), orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to fetch billing state: %v", err))
	}

	plans := getSortedLOCPlans()
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"billing": map[string]interface{}{
			"current_plan_code":           state.CurrentPlanCode,
			"billing_period_start":        state.BillingPeriodStart.Format(time.RFC3339),
			"billing_period_end":          state.BillingPeriodEnd.Format(time.RFC3339),
			"loc_used_month":              state.LOCUsedMonth,
			"trial_readonly":              state.TrialReadOnly,
			"scheduled_plan_code":         nullString(state.ScheduledPlanCode),
			"scheduled_plan_effective_at": nullTime(state.ScheduledPlanEffectiveAt),
		},
		"available_plans": plans,
	})
}

func (h *BillingActionsHandler) UpgradePlan(c echo.Context) error {
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
	if targetPlan.GetLimits().MonthlyLOCLimit <= currentPlan.GetLimits().MonthlyLOCLimit {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "target_plan_code must be a higher LOC tier for upgrade")
	}

	payload := map[string]interface{}{
		"from_plan_code": currentPlan.String(),
		"to_plan_code":   targetPlan.String(),
		"mode":           "immediate",
		"proration":      "razorpay_scheduled",
	}
	if err := h.syncRazorpayTransition(ctx, orgID, targetPlan, true, state.BillingPeriodEnd); err != nil {
		if errors.Is(err, errRazorpayCheckoutRequired) {
			return JSONWithEnvelope(c, http.StatusConflict, map[string]interface{}{
				"message":           "organization requires paid checkout before upgrade",
				"checkout_required": true,
				"checkout_path":     "/checkout/team?period=monthly",
			})
		}
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, fmt.Sprintf("razorpay transition failed: %v", err))
	}
	if err := h.store.ApplyImmediatePlanUpgrade(ctx, orgID, targetPlan.String(), actorUserID, payload); err != nil {
		rollbackErr := h.syncRazorpayTransition(ctx, orgID, currentPlan, true, state.BillingPeriodEnd)
		if rollbackErr != nil {
			return JSONErrorWithEnvelope(
				c,
				http.StatusInternalServerError,
				fmt.Sprintf("failed to upgrade plan after razorpay transition and rollback also failed (forward_err=%v rollback_err=%v)", err, rollbackErr),
			)
		}
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to upgrade plan and razorpay transition was rolled back: %v", err))
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"message":       "plan upgraded successfully",
		"plan_code":     targetPlan.String(),
		"proration_job": "queued",
	})
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

func runBillingTransitionScheduler(ctx context.Context, db *sql.DB, interval time.Duration) {
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	store := storagelicense.NewPlanChangeStore(db)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			due, err := store.ListDueScheduledDowngrades(ctx, time.Now().UTC(), 100)
			if err != nil {
				log.Printf("[billing-transition-scheduler] list due downgrades failed: %v", err)
				continue
			}
			for _, tr := range due {
				transitionCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				err := applyDueDowngradeWithRazorpay(transitionCtx, db, store, tr)
				cancel()
				if err != nil {
					log.Printf("[billing-transition-scheduler] org=%d target_plan=%s reconcile failed: %v", tr.OrgID, tr.TargetPlanCode, err)
				}
			}
		}
	}
}

func applyDueDowngradeWithRazorpay(ctx context.Context, db *sql.DB, store *storagelicense.PlanChangeStore, tr storagelicense.DueTransition) error {
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

	if err := store.ApplyScheduledDowngrade(ctx, tr); err != nil {
		return fmt.Errorf("apply scheduled downgrade: %w", err)
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

	limit := 25
	if v := strings.TrimSpace(c.QueryParam("limit")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid limit")
		}
		limit = parsed
	}

	offset := 0
	if v := strings.TrimSpace(c.QueryParam("offset")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid offset")
		}
		offset = parsed
	}

	ops, err := h.usageStore.ListCurrentPeriodOperations(c.Request().Context(), orgID, limit, offset)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, fmt.Sprintf("failed to load usage operations: %v", err))
	}

	rows := make([]map[string]interface{}, 0, len(ops))
	for _, op := range ops {
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
		rows = append(rows, row)
	}

	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{
		"operations": rows,
		"limit":      limit,
		"offset":     offset,
		"count":      len(rows),
	})
}

func (h *BillingActionsHandler) syncRazorpayTransition(ctx context.Context, orgID int64, targetPlan license.PlanType, immediate bool, periodEnd time.Time) error {
	return syncRazorpayTransitionWithDB(ctx, h.db, orgID, targetPlan, immediate, periodEnd)
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

	if _, err := payment.GetSubscriptionByID(mode, active.RazorpaySubscriptionID); err != nil {
		return fmt.Errorf("fetch razorpay subscription: %w", err)
	}

	quantity := locPlanToQuantity(targetPlan)
	scheduleAt := int64(0)
	if immediate {
		scheduleAt = time.Now().UTC().Unix()
	} else {
		scheduleAt = periodEnd.UTC().Unix()
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
