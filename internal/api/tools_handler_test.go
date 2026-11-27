package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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
		if resp["error"] != "SAST tools are only available in cloud mode" {
			t.Fatalf("expected error message %q, got %q", "SAST tools are only available in cloud mode", resp["error"])
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
		if resp["error"] != "SAST tools are only available in cloud mode" {
			t.Fatalf("expected error message %q, got %q", "SAST tools are only available in cloud mode", resp["error"])
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
		if resp["error"] != "SAST tools are only available in cloud mode" {
			t.Fatalf("expected error message %q, got %q", "SAST tools are only available in cloud mode", resp["error"])
		}
	})

	t.Run("GetRepositoryTools returns 403 when non-cloud", func(t *testing.T) {
		s := &Server{deploymentConfig: &DeploymentConfig{IsCloud: false}}
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/repositories/1/tools", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("repoId")
		c.SetParamValues("1")
		c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: 1, IsMember: true})

		if err := s.GetRepositoryTools(c); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", rec.Code)
		}
	})

	t.Run("UpdateRepositoryTools enforces owner role and validates mode", func(t *testing.T) {
		s := &Server{deploymentConfig: &DeploymentConfig{IsCloud: true}}
		e := echo.New()

		// Member must receive 403
		reqMember := httptest.NewRequest(http.MethodPut, "/api/v1/repositories/1/tools", strings.NewReader(`{"mode":"custom"}`))
		reqMember.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		recMember := httptest.NewRecorder()
		cm := e.NewContext(reqMember, recMember)
		cm.SetParamNames("repoId")
		cm.SetParamValues("1")
		cm.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: 1, IsMember: true})
		if err := s.UpdateRepositoryTools(cm); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if recMember.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for member, got %d", recMember.Code)
		}

		// Owner with invalid mode must receive 400
		reqBadMode := httptest.NewRequest(http.MethodPut, "/api/v1/repositories/1/tools", strings.NewReader(`{"mode":"always"}`))
		reqBadMode.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		recBadMode := httptest.NewRecorder()
		cb := e.NewContext(reqBadMode, recBadMode)
		cb.SetParamNames("repoId")
		cb.SetParamValues("1")
		cb.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: 1, IsOwner: true})
		if err := s.UpdateRepositoryTools(cb); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if recBadMode.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid mode, got %d", recBadMode.Code)
		}
	})
}

func TestRepositoryToolsEndpoints_DBRoundTripAndOrgScoping(t *testing.T) {
	server := setupBotUserTestServer(t)
	server.deploymentConfig = &DeploymentConfig{IsCloud: true}
	orgID := getAnyOrgID(t, server.db)
	connectorID := insertIntegrationToken(t, server.db, "github", "https://github.com", "pat", nil)
	repoID, _, _ := seedRepoAndPRsForAPITest(t, server, orgID, connectorID)

	// Ensure at least one tool exists in available_tools for the test.
	var toolID int64
	err := server.db.QueryRow(`SELECT id FROM available_tools ORDER BY id LIMIT 1`).Scan(&toolID)
	var toolName string
	if err != nil {
		err = server.db.QueryRow(`
			INSERT INTO available_tools (name, description, lambda_arn, multiplier)
			VALUES ('test-linter-repo-api', 'test', 'arn:test', 1.0)
			RETURNING id, name
		`).Scan(&toolID, &toolName)
		if err != nil {
			t.Fatalf("seed tool: %v", err)
		}
		t.Cleanup(func() { _, _ = server.db.Exec(`DELETE FROM available_tools WHERE id = $1`, toolID) })
	} else {
		_ = server.db.QueryRow(`SELECT name FROM available_tools WHERE id = $1`, toolID).Scan(&toolName)
	}

	e := echo.New()
	repoIDStr := strconv.FormatInt(repoID, 10)

	// 1. Default GET returns inherit mode
	cGet, recGet := newTestEchoContext(http.MethodGet, "/api/v1/repositories/:repoId/tools", orgID, []string{"repoId"}, []string{repoIDStr})
	cGet.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID, IsMember: true})
	if err := server.GetRepositoryTools(cGet); err != nil {
		t.Fatalf("GetRepositoryTools: %v", err)
	}
	if recGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recGet.Code, recGet.Body.String())
	}
	var getResp struct {
		Mode  string `json:"mode"`
		Tools []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(recGet.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal getResp: %v", err)
	}
	if getResp.Mode != "inherit" {
		t.Fatalf("expected inherit mode initially, got %q", getResp.Mode)
	}

	// 2. Owner PUT updates to custom mode with tool enabled
	putBody := `{"mode":"custom","tools":{"` + toolName + `":true}}`
	reqPut := httptest.NewRequest(http.MethodPut, "/api/v1/repositories/"+repoIDStr+"/tools", strings.NewReader(putBody))
	reqPut.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recPut := httptest.NewRecorder()
	cPut := e.NewContext(reqPut, recPut)
	cPut.SetParamNames("repoId")
	cPut.SetParamValues(repoIDStr)
	cPut.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID, IsOwner: true})
	if err := server.UpdateRepositoryTools(cPut); err != nil {
		t.Fatalf("UpdateRepositoryTools: %v", err)
	}
	if recPut.Code != http.StatusOK {
		t.Fatalf("expected 200 on PUT, got %d: %s", recPut.Code, recPut.Body.String())
	}

	// 3. Verify GET returns custom and enabled tool
	cGet2, recGet2 := newTestEchoContext(http.MethodGet, "/api/v1/repositories/:repoId/tools", orgID, []string{"repoId"}, []string{repoIDStr})
	cGet2.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID, IsMember: true})
	if err := server.GetRepositoryTools(cGet2); err != nil {
		t.Fatalf("GetRepositoryTools after PUT: %v", err)
	}
	if err := json.Unmarshal(recGet2.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal getResp2: %v", err)
	}
	if getResp.Mode != "custom" {
		t.Fatalf("expected custom mode after PUT, got %q", getResp.Mode)
	}
	foundEnabled := false
	for _, tl := range getResp.Tools {
		if tl.Name == toolName && tl.Enabled {
			foundEnabled = true
		}
	}
	if !foundEnabled {
		t.Fatalf("expected %s to be enabled in GET response after PUT", toolName)
	}

	// 4. Wrong org returns 404 on GET and preview
	cWrong, recWrong := newTestEchoContext(http.MethodGet, "/api/v1/repositories/:repoId/tools", orgID+999999, []string{"repoId"}, []string{repoIDStr})
	cWrong.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID + 999999, IsOwner: true})
	if err := server.GetRepositoryTools(cWrong); err != nil {
		t.Fatalf("GetRepositoryTools wrong org: %v", err)
	}
	if recWrong.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong org, got %d", recWrong.Code)
	}

	// 5. Preview endpoint validates tools.toml against go-toml and reports unknown tools + errors
	reqPrevBad := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/"+repoIDStr+"/tools/preview", strings.NewReader(`{"tools_toml":"[ruff]\nenabled = maybe\n"}`))
	reqPrevBad.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recPrevBad := httptest.NewRecorder()
	cPrevBad := e.NewContext(reqPrevBad, recPrevBad)
	cPrevBad.SetParamNames("repoId")
	cPrevBad.SetParamValues(repoIDStr)
	cPrevBad.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID, IsMember: true})
	if err := server.PreviewRepositoryTools(cPrevBad); err != nil {
		t.Fatalf("PreviewRepositoryTools bad: %v", err)
	}
	if recPrevBad.Code != http.StatusOK {
		t.Fatalf("expected 200 on preview, got %d: %s", recPrevBad.Code, recPrevBad.Body.String())
	}
	var prevResp struct {
		Valid        bool     `json:"valid"`
		Error        string   `json:"error"`
		ErrorLine    int      `json:"error_line"`
		UnknownTools []string `json:"unknown_tools"`
	}
	if err := json.Unmarshal(recPrevBad.Body.Bytes(), &prevResp); err != nil {
		t.Fatalf("unmarshal prevBad: %v", err)
	}
	if prevResp.Valid || prevResp.ErrorLine != 2 || prevResp.Error == "" {
		t.Fatalf("expected valid=false error_line=2, got %+v", prevResp)
	}

	reqPrevOk := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/"+repoIDStr+"/tools/preview", strings.NewReader(`{"tools_toml":"[`+toolName+`]\nenabled = true\n"}`))
	reqPrevOk.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recPrevOk := httptest.NewRecorder()
	cPrevOk := e.NewContext(reqPrevOk, recPrevOk)
	cPrevOk.SetParamNames("repoId")
	cPrevOk.SetParamValues(repoIDStr)
	cPrevOk.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID, IsMember: true})
	if err := server.PreviewRepositoryTools(cPrevOk); err != nil {
		t.Fatalf("PreviewRepositoryTools ok: %v", err)
	}
	if err := json.Unmarshal(recPrevOk.Body.Bytes(), &prevResp); err != nil {
		t.Fatalf("unmarshal prevOk: %v", err)
	}
	if !prevResp.Valid || len(prevResp.UnknownTools) != 0 {
		t.Fatalf("expected valid=true no unknown tools, got %+v", prevResp)
	}

	reqPrevWrong := httptest.NewRequest(http.MethodPost, "/api/v1/repositories/"+repoIDStr+"/tools/preview", strings.NewReader(`{"tools_toml":"[ruff]\nenabled = true\n"}`))
	reqPrevWrong.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recPrevWrong := httptest.NewRecorder()
	cPrevWrong := e.NewContext(reqPrevWrong, recPrevWrong)
	cPrevWrong.SetParamNames("repoId")
	cPrevWrong.SetParamValues(repoIDStr)
	cPrevWrong.Set(string(auth.PermissionContextKey), &auth.PermissionContext{OrgID: orgID + 999999, IsMember: true})
	if err := server.PreviewRepositoryTools(cPrevWrong); err != nil {
		t.Fatalf("PreviewRepositoryTools wrong org: %v", err)
	}
	if recPrevWrong.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong org preview, got %d", recPrevWrong.Code)
	}
}
