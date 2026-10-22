package users

import (
	"errors"
	"testing"
)

func TestFreeMemberCapReached(t *testing.T) {
	cases := []struct {
		name     string
		plan     string
		cloud    bool
		active   int
		expected bool
	}{
		{"free_30k below cap", "free_30k", true, 2, false},
		{"free_30k at cap", "free_30k", true, 3, true},
		{"free_30k over cap (legacy org)", "free_30k", true, 4, true},
		{"legacy free code at cap", "free", true, 3, true},
		{"missing plan counts as free", "", true, 3, true},
		{"paid plan never capped", "team_32usd", true, 10, false},
		{"loc plan never capped", "loc_400k", true, 10, false},
		{"self-hosted never capped", "free_30k", false, 10, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := freeMemberCapReached(tc.plan, tc.cloud, tc.active); got != tc.expected {
				t.Fatalf("freeMemberCapReached(%q, %v, %d) = %v, want %v", tc.plan, tc.cloud, tc.active, got, tc.expected)
			}
		})
	}
}

func TestFriendlyBulkInviteErrorMemberCap(t *testing.T) {
	got := friendlyBulkInviteError(ErrFreePlanMemberLimit, "invite")
	if got != "Free plan member limit reached (3)" {
		t.Fatalf("unexpected message %q", got)
	}
	if !errors.Is(ErrFreePlanMemberLimit, ErrFreePlanMemberLimit) {
		t.Fatal("ErrFreePlanMemberLimit must match itself with errors.Is")
	}
}
