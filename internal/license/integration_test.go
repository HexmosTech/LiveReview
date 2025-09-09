package license

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

// TestLicenseAPIIntegration provides a light integration test for the license endpoints.
// It requires DATABASE_URL in environment and existing migrations applied.
func TestLicenseAPIIntegration(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	e := echo.New()
	// Minimal server stub wiring only what we need.
	// We'll create service directly.
	cfg := LoadConfig()
	svc := NewService(cfg, db)

	e.GET("/api/v1/license/status", func(c echo.Context) error {
		st, err := svc.LoadOrInit(c.Request().Context())
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal"})
		}
		return c.JSON(http.StatusOK, map[string]any{"status": st.Status, "unlimited": st.Unlimited})
	})
	e.POST("/api/v1/license/update", func(c echo.Context) error {
		var body struct {
			Token string `json:"token"`
		}
		if err := c.Bind(&body); err != nil || body.Token == "" {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_token_required"})
		}
		if _, err := svc.EnterLicense(c.Request().Context(), body.Token); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "license_invalid"})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "active"})
	})

	// 1. Status initially missing
	req := httptest.NewRequest(http.MethodGet, "/api/v1/license/status", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
	var stResp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &stResp)
	if stResp["status"] != StatusMissing {
		t.Fatalf("expected status missing got %v", stResp["status"])
	}

	// 2. Invalid update attempt
	body := bytes.NewBufferString(`{"token":"bogus"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/license/update", body)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", rec2.Code)
	}
}
