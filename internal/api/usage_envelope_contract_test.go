package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/license"
)

func TestJSONWithEnvelope_ContractFields(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set(apimiddleware.PlanContextKey, apimiddleware.PlanContext{
		PlanType: license.PlanFree30K,
		Limits:   license.PlanFree30K.GetLimits(),
	})
	c.Set(EnvelopeOperationTypeContextKey, "diff_review")
	c.Set(EnvelopeTriggerSourceContextKey, "api")
	c.Set(EnvelopeUsagePercentContextKey, 44)
	c.Set(EnvelopeBlockedContextKey, false)
	c.Set(EnvelopeTrialReadOnlyContextKey, false)

	if err := JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "ok"}); err != nil {
		t.Fatalf("JSONWithEnvelope returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	assertContains(t, body, `"status":"ok"`)
	assertContains(t, body, `"envelope"`)
	assertContains(t, body, `"plan_code":"free_30k"`)
	assertContains(t, body, `"operation_type":"diff_review"`)
	assertContains(t, body, `"usage_percent":44`)
	assertContains(t, body, `"blocked":false`)
}

func TestJSONErrorWithEnvelope_ContractFields(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	c.Set(apimiddleware.PlanContextKey, apimiddleware.PlanContext{
		PlanType: license.PlanFree30K,
		Limits:   license.PlanFree30K.GetLimits(),
	})
	c.Set(EnvelopeBlockedContextKey, true)
	c.Set(EnvelopeTrialReadOnlyContextKey, true)

	if err := JSONErrorWithEnvelope(c, http.StatusTooManyRequests, "quota exceeded"); err != nil {
		t.Fatalf("JSONErrorWithEnvelope returned error: %v", err)
	}

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	body := rec.Body.String()
	assertContains(t, body, `"error":"quota exceeded"`)
	assertContains(t, body, `"envelope"`)
	assertContains(t, body, `"plan_code":"free_30k"`)
	assertContains(t, body, `"blocked":true`)
	assertContains(t, body, `"trial_readonly":true`)
}

func assertContains(t *testing.T, s, needle string) {
	t.Helper()
	if !contains(s, needle) {
		t.Fatalf("expected %q in %q", needle, s)
	}
}

func contains(s, needle string) bool {
	return len(needle) == 0 || (len(s) >= len(needle) && indexOf(s, needle) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
