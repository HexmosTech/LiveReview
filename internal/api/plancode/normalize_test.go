package plancode

import "testing"

func TestNormalizePlanTypeCode(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "free legacy", in: "free", want: "free_30k"},
		{name: "free canonical", in: "free_30k", want: "free_30k"},
		{name: "team legacy", in: "team", want: "team_32usd"},
		{name: "team annual", in: "team_annual", want: "team_32usd"},
		{name: "loc slab", in: "loc_400k", want: "loc_400k"},
		{name: "unknown defaults free", in: "mystery", want: "free_30k"},
		{name: "empty defaults free", in: "", want: "free_30k"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizePlanTypeCode(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizePlanTypeCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
