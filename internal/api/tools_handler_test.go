package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

func TestToolsHandler(t *testing.T) {
	t.Run("ListAvailableTools returns 403 when s.deploymentConfig.IsCloud == false", func(t *testing.T) {
		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tools", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := s.ListAvailableTools(c); err != nil {
			t.Fatalf("unexpected error from ListAvailableTools: %v", err)
		}

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", rec.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}
		if resp["error"] != "Third-party tools are only available in cloud mode" {
			t.Fatalf("expected error message %q, got %q", "Third-party tools are only available in cloud mode", resp["error"])
		}
	})

	t.Run("ListOrgTools returns 403 when s.deploymentConfig.IsCloud == false", func(t *testing.T) {
		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/orgs/42/tools", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
			OrgID:    42,
			IsMember: true,
		})

		if err := s.ListOrgTools(c); err != nil {
			t.Fatalf("unexpected error from ListOrgTools: %v", err)
		}

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", rec.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}
		if resp["error"] != "Third-party tools are only available in cloud mode" {
			t.Fatalf("expected error message %q, got %q", "Third-party tools are only available in cloud mode", resp["error"])
		}
	})

	t.Run("UpdateOrgTool returns 403 when s.deploymentConfig.IsCloud == false", func(t *testing.T) {
		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		body := `{"enabled":true}`
		e := echo.New()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/orgs/42/tools/10", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("tool_id")
		c.SetParamValues("10")
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
			OrgID:   42,
			IsOwner: true,
		})

		if err := s.UpdateOrgTool(c); err != nil {
			t.Fatalf("unexpected error from UpdateOrgTool: %v", err)
		}

		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected status 403, got %d", rec.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response JSON: %v", err)
		}
		if resp["error"] != "Third-party tools are only available in cloud mode" {
			t.Fatalf("expected error message %q, got %q", "Third-party tools are only available in cloud mode", resp["error"])
		}
	})
}
