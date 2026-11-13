package license

import (
	"testing"
)

func TestGetToolCreditsLimit(t *testing.T) {
	tests := []struct {
		plan     PlanType
		expected float64
	}{
		{PlanFree30K, 0.0},
		{PlanEnterpriseSelfhosted, 0.0},
		{PlanTeam32USD, 50000.0},
		{PlanLOC200K, 100000.0},
		{PlanLOC400K, 200000.0},
		{PlanLOC800K, 400000.0},
		{PlanLOC1600K, 800000.0},
		{PlanLOC3200K, 1600000.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.plan), func(t *testing.T) {
			got := tt.plan.GetToolCreditsLimit()
			if got != tt.expected {
				t.Errorf("plan %s: expected credits limit %.1f, got %.1f", tt.plan, tt.expected, got)
			}
		})
	}
}
