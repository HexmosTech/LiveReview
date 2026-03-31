package payment

import (
	"testing"

	"github.com/livereview/internal/license"
)

func TestPlanCodeToMonthlyQuantity(t *testing.T) {
	tests := []struct {
		plan license.PlanType
		want int
	}{
		{license.PlanTeam32USD, 1},
		{license.PlanLOC200K, 2},
		{license.PlanLOC400K, 4},
		{license.PlanLOC800K, 8},
		{license.PlanLOC1600K, 16},
		{license.PlanLOC3200K, 32},
	}

	for _, tc := range tests {
		got, err := planCodeToMonthlyQuantity(tc.plan)
		if err != nil {
			t.Fatalf("planCodeToMonthlyQuantity(%s) error: %v", tc.plan, err)
		}
		if got != tc.want {
			t.Fatalf("planCodeToMonthlyQuantity(%s) = %d, want %d", tc.plan, got, tc.want)
		}
	}
}

func TestNormalizePersistedPlanCode(t *testing.T) {
	if got := normalizePersistedPlanCode("team_monthly"); got != license.PlanTeam32USD {
		t.Fatalf("expected team_monthly to normalize to %s, got %s", license.PlanTeam32USD, got)
	}
	if got := normalizePersistedPlanCode("loc_1600k"); got != license.PlanLOC1600K {
		t.Fatalf("expected loc_1600k to remain same, got %s", got)
	}
}
