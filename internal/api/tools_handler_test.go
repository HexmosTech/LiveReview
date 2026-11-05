package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/storage/tools"
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

	t.Run("UpsertAvailableTool returns 403 when s.deploymentConfig.IsCloud == false", func(t *testing.T) {
		s := &Server{
			deploymentConfig: &DeploymentConfig{IsCloud: false},
		}

		body := `{"name":"custom-linter","lambda_arn":"arn:aws:lambda:us-east-1:123456789012:function:custom-linter","multiplier":1.5}`
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/tools", strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		if err := s.UpsertAvailableTool(c); err != nil {
			t.Fatalf("unexpected error from UpsertAvailableTool: %v", err)
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

	t.Run("getNextToolID generates sequential IDs (maxID + 1) without sequence dependency", func(t *testing.T) {
		// Empty catalog should start at ID 1
		catalog := []tools.AvailableTool{}
		if got := getNextToolID(catalog); got != 1 {
			t.Fatalf("expected ID 1 for empty catalog, got %d", got)
		}

		// Single element catalog
		catalog = []tools.AvailableTool{
			{ID: 1, Name: "tool-1"},
		}
		if got := getNextToolID(catalog); got != 2 {
			t.Fatalf("expected ID 2 for catalog with max ID 1, got %d", got)
		}

		// Multiple unordered elements with gaps
		catalog = []tools.AvailableTool{
			{ID: 12, Name: "tool-12"},
			{ID: 3, Name: "tool-3"},
			{ID: 99, Name: "tool-99"},
			{ID: 45, Name: "tool-45"},
		}
		if got := getNextToolID(catalog); got != 100 {
			t.Fatalf("expected ID 100 for catalog with max ID 99, got %d", got)
		}

		// Sequential additions simulation
		var seqCatalog []tools.AvailableTool
		for i := int64(1); i <= 10; i++ {
			nextID := getNextToolID(seqCatalog)
			if nextID != i {
				t.Fatalf("step %d: expected nextID %d, got %d", i, i, nextID)
			}
			seqCatalog = append(seqCatalog, tools.AvailableTool{
				ID:   nextID,
				Name: fmt.Sprintf("sequential-tool-%d", i),
			})
		}
	})
}
