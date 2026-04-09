package api

import (
	"testing"

	"github.com/livereview/internal/api/plancode"
)

func TestNormalizeQuotaPlanCode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "free legacy", in: "free", want: "free_30k"},
		{name: "free canonical", in: "free_30k", want: "free_30k"},
		{name: "team legacy", in: "team", want: "team_32usd"},
		{name: "team monthly", in: "team_monthly", want: "team_32usd"},
		{name: "loc slab", in: "loc_400k", want: "loc_400k"},
		{name: "unknown defaults free", in: "unknown", want: "free_30k"},
		{name: "empty defaults free", in: "", want: "free_30k"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := plancode.NormalizePlanTypeCode(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizePlanTypeCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveQuotaPlanType(t *testing.T) {
	tests := []struct {
		name            string
		envelopePlan    string
		contextPlanType string
		want            string
	}{
		{name: "envelope takes precedence", envelopePlan: "loc_400k", contextPlanType: "free_30k", want: "loc_400k"},
		{name: "context used when envelope missing", envelopePlan: "", contextPlanType: "team", want: "team_32usd"},
		{name: "defaults free when both missing", envelopePlan: "", contextPlanType: "", want: "free_30k"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveQuotaPlanType(tc.envelopePlan, tc.contextPlanType)
			if got != tc.want {
				t.Fatalf("resolveQuotaPlanType(%q, %q) = %q, want %q", tc.envelopePlan, tc.contextPlanType, got, tc.want)
			}
		})
	}
}

func TestIsQuotaFreePlan(t *testing.T) {
	if !isQuotaFreePlan("free") {
		t.Fatalf("expected legacy free to be treated as free plan")
	}
	if !isQuotaFreePlan("free_30k") {
		t.Fatalf("expected free_30k to be treated as free plan")
	}
	if isQuotaFreePlan("team") {
		t.Fatalf("expected team to be treated as paid plan")
	}
	if isQuotaFreePlan("loc_400k") {
		t.Fatalf("expected loc_400k to be treated as paid plan")
	}
}
