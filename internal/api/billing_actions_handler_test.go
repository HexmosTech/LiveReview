package api

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/license"
	"github.com/livereview/pkg/models"
	storagelicense "github.com/livereview/storage/license"
)

func TestLOCPlanToQuantity(t *testing.T) {
	tests := []struct {
		name string
		plan license.PlanType
		want int
	}{
		{name: "free maps to minimum quantity", plan: license.PlanFree30K, want: 1},
		{name: "team maps to expected quantity", plan: license.PlanTeam32USD, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := locPlanToQuantity(tt.plan)
			if got != tt.want {
				t.Fatalf("locPlanToQuantity(%s) = %d, want %d", tt.plan, got, tt.want)
			}
		})
	}
}

func TestGetSortedLOCPlansAscendingByLimit(t *testing.T) {
	plans := getSortedLOCPlans()
	if len(plans) < 2 {
		t.Fatalf("expected at least two plans, got %d", len(plans))
	}

	prev := -1
	for i, plan := range plans {
		limit, ok := plan["monthly_loc_limit"].(int)
		if !ok {
			t.Fatalf("plan %d has unexpected monthly_loc_limit type %T", i, plan["monthly_loc_limit"])
		}
		if limit < prev {
			t.Fatalf("plans are not sorted: index %d has %d after %d", i, limit, prev)
		}
		prev = limit
	}
}

func TestApplyDueDowngradeWithRazorpayRejectsMissingDeps(t *testing.T) {
	tr := storagelicense.DueTransition{OrgID: 1, TargetPlanCode: license.PlanTeam32USD.String()}

	err := applyDueDowngradeWithRazorpay(context.Background(), nil, nil, tr)
	if err == nil || err.Error() != "missing db handle" {
		t.Fatalf("expected missing db handle error, got %v", err)
	}
}

func TestApplyDueDowngradeWithRazorpayRejectsInvalidPlan(t *testing.T) {
	store := &storagelicense.PlanChangeStore{}
	tr := storagelicense.DueTransition{OrgID: 1, TargetPlanCode: "invalid_plan_code"}

	err := applyDueDowngradeWithRazorpay(context.Background(), &sql.DB{}, store, tr)
	if err == nil {
		t.Fatalf("expected invalid plan error")
	}
	if err.Error() != "invalid target plan code: invalid_plan_code" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComputeProratedDeltaCentsMidCycle(t *testing.T) {
	cycleStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cycleEnd := cycleStart.AddDate(0, 1, 0)
	now := cycleStart.Add(cycleEnd.Sub(cycleStart) / 2)

	chargeCents, fraction := computeProratedDeltaCents(32, 64, cycleStart, cycleEnd, now)
	if chargeCents != 1600 {
		t.Fatalf("expected half-cycle delta 1600 cents, got %d", chargeCents)
	}
	if fraction < 0.49 || fraction > 0.51 {
		t.Fatalf("expected fraction around 0.5, got %.4f", fraction)
	}
}

func TestComputeProratedDeltaCentsNoUpgrade(t *testing.T) {
	cycleStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cycleEnd := cycleStart.AddDate(0, 1, 0)

	chargeCents, fraction := computeProratedDeltaCents(64, 32, cycleStart, cycleEnd, cycleStart)
	if chargeCents != 0 {
		t.Fatalf("expected zero charge for downgrade path, got %d", chargeCents)
	}
	if fraction != 0 {
		t.Fatalf("expected zero fraction for non-upgrade, got %.4f", fraction)
	}
}

func TestComputeRemainingCycleFractionUsesActualCycleWindow(t *testing.T) {
	cycleStart := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cycleEnd := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC) // 31-day cycle
	now := time.Date(2026, 3, 24, 0, 0, 0, 0, time.UTC)

	got := computeRemainingCycleFraction(cycleStart, cycleEnd, now)
	want := cycleEnd.Sub(now).Seconds() / cycleEnd.Sub(cycleStart).Seconds()
	if diff := math.Abs(got - want); diff > 0.000001 {
		t.Fatalf("remaining fraction mismatch: got %.8f want %.8f", got, want)
	}
}

func TestComputeTargetProratedChargeCentsTargetBased(t *testing.T) {
	got := computeTargetProratedChargeCents(64, 8.0/30.0)
	if got != 1707 {
		t.Fatalf("expected target-based prorated charge 1707 cents, got %d", got)
	}
}

func TestComputeTargetProratedLOCGrantNearestWhole(t *testing.T) {
	got := computeTargetProratedLOCGrant(200000, 8.0/30.0)
	if got != 53333 {
		t.Fatalf("expected nearest whole loc grant 53333, got %d", got)
	}
}

func TestIsUPIPaymentMethod(t *testing.T) {
	tests := []struct {
		name          string
		paymentMethod string
		want          bool
	}{
		{name: "upi method is detected", paymentMethod: "upi", want: true},
		{name: "upi method is case-insensitive", paymentMethod: " UPI ", want: true},
		{name: "card method is not upi", paymentMethod: "card", want: false},
		{name: "empty method is not upi", paymentMethod: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUPIPaymentMethod(tt.paymentMethod)
			if got != tt.want {
				t.Fatalf("isUPIPaymentMethod(%q) = %v, want %v", tt.paymentMethod, got, tt.want)
			}
		})
	}
}

func TestIsRazorpayUPISubscriptionUpdateError(t *testing.T) {
	err := errors.New("update razorpay subscription quantity: failed to update razorpay subscription: razorpay API error (status 400): {\"error\":{\"code\":\"BAD_REQUEST_ERROR\",\"description\":\"subscriptions cannot be updated when payment mode is upi\"}}")
	if !isRazorpayUPISubscriptionUpdateError(err) {
		t.Fatalf("expected UPI subscription update error to be detected")
	}

	if isRazorpayUPISubscriptionUpdateError(errors.New("razorpay API error (status 400): unknown request")) {
		t.Fatalf("expected non-UPI error to not be detected")
	}
}

func TestBuildTrialEligibilityViewMissingContext(t *testing.T) {
	h := &BillingActionsHandler{}
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	view := h.buildTrialEligibilityView(context.Background(), c, time.Now().UTC())
	if got := view["status"]; got != "unknown" {
		t.Fatalf("status = %v, want unknown", got)
	}
	if got := view["reason"]; got != "user_context_missing" {
		t.Fatalf("reason = %v, want user_context_missing", got)
	}
}

func TestBuildTrialEligibilityViewBlankEmail(t *testing.T) {
	h := &BillingActionsHandler{}
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{User: &models.User{Email: "   "}})

	view := h.buildTrialEligibilityView(context.Background(), c, time.Now().UTC())
	if got := view["status"]; got != "unknown" {
		t.Fatalf("status = %v, want unknown", got)
	}
	if got := view["reason"]; got != "user_email_missing" {
		t.Fatalf("reason = %v, want user_email_missing", got)
	}
}

func TestBuildTrialEligibilityViewLookupFailure(t *testing.T) {
	h := &BillingActionsHandler{}
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{User: &models.User{Email: "trial@example.com"}})

	view := h.buildTrialEligibilityView(context.Background(), c, time.Now().UTC())
	if got := view["status"]; got != "unknown" {
		t.Fatalf("status = %v, want unknown", got)
	}
	if got := view["reason"]; got != "eligibility_lookup_failed" {
		t.Fatalf("reason = %v, want eligibility_lookup_failed", got)
	}
}
