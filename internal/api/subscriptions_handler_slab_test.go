package api

import (
	"testing"

	"github.com/livereview/internal/license"
)

func TestResolvePlanCodeFromRequestWithPlanCode(t *testing.T) {
	plan, err := resolvePlanCodeFromRequest(CreateSubscriptionRequest{PlanCode: "loc_400k"})
	if err != nil {
		t.Fatalf("resolvePlanCodeFromRequest returned error: %v", err)
	}
	if plan != license.PlanLOC400K {
		t.Fatalf("expected %s, got %s", license.PlanLOC400K, plan)
	}
}

func TestResolvePlanCodeFromRequestRejectsYearly(t *testing.T) {
	_, err := resolvePlanCodeFromRequest(CreateSubscriptionRequest{PlanType: "team_annual", Quantity: 1})
	if err == nil {
		t.Fatalf("expected yearly request to be rejected")
	}
}

func TestResolvePlanCodeFromRequestLegacyQuantityMap(t *testing.T) {
	plan, err := resolvePlanCodeFromRequest(CreateSubscriptionRequest{Quantity: 8})
	if err != nil {
		t.Fatalf("resolvePlanCodeFromRequest returned error: %v", err)
	}
	if plan != license.PlanLOC800K {
		t.Fatalf("expected %s, got %s", license.PlanLOC800K, plan)
	}
}
