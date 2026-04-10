package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/plancode"
)

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
			got := plancode.NormalizePlanTypeCode(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizePlanTypeCode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReadOrgIDFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set("org_id", int64(7))
	if got, ok := readOrgIDFromContext(c); !ok || got != 7 {
		t.Fatalf("expected int64 org_id 7, got %d ok=%v", got, ok)
	}

	c.Set("org_id", int(8))
	if got, ok := readOrgIDFromContext(c); !ok || got != 8 {
		t.Fatalf("expected int org_id 8, got %d ok=%v", got, ok)
	}

	c.Set("org_id", "9")
	if got, ok := readOrgIDFromContext(c); !ok || got != 9 {
		t.Fatalf("expected string org_id 9, got %d ok=%v", got, ok)
	}

	c.Set("org_id", "")
	if _, ok := readOrgIDFromContext(c); ok {
		t.Fatalf("expected empty org_id string to be rejected")
	}
}
