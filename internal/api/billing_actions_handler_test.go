package api

import (
	"context"
	"database/sql"
	"testing"

	"github.com/livereview/internal/license"
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
