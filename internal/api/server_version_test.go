package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestGetVersionIncludesContractMarkers(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	s := &Server{
		versionInfo: &VersionInfo{
			Version:   "v-test",
			GitCommit: "abc123",
			BuildTime: "2026-03-30T00:00:00Z",
			Dirty:     false,
		},
	}

	if err := s.getVersion(c); err != nil {
		t.Fatalf("getVersion returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if body["subscriptionContractVersion"] != "slab_plan_code_v1" {
		t.Fatalf("unexpected subscriptionContractVersion: %v", body["subscriptionContractVersion"])
	}
	if body["billingTransitionSafetyLevel"] != "v1_compensation" {
		t.Fatalf("unexpected billingTransitionSafetyLevel: %v", body["billingTransitionSafetyLevel"])
	}
}
