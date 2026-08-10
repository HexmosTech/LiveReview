package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/pkg/models"
)

// registerTestExport writes a real temp file and registers it exactly the way
// registerChatExports does, without going through the full agent pipeline.
func registerTestExport(t *testing.T, id string, orgID int64, data string) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "livereview-chat-export-test-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	path := tmpDir + "/export.csv"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	chartFilesMu.Lock()
	chartFiles[id] = &chartFileEntry{
		Kind:      chartFileKindExport,
		PNGPath:   path,
		TmpDir:    tmpDir,
		CreatedAt: time.Now(),
		Filename:  "export.csv",
		OrgID:     orgID,
	}
	chartFilesMu.Unlock()
	t.Cleanup(func() {
		chartFilesMu.Lock()
		delete(chartFiles, id)
		chartFilesMu.Unlock()
	})
}

// The unauthenticated chart PNG route must never be able to serve a CSV
// export, regardless of id, because that route carries no auth or org check
// at all - it exists only because <img> tags can't send headers. If this
// ever regresses, any org's bulk exported review data becomes fetchable by
// anyone who learns (or guesses/observes) an export id.
func TestServeChartPNGCannotServeAnExport(t *testing.T) {
	const id = "cross-endpoint-export-id"
	registerTestExport(t, id, 42, "id,repository,status\n1,repo,completed\n")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/charts/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)

	s := &Server{}
	if err := s.ServeChartPNG(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ServeChartPNG served an export entry: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// The authenticated export endpoint must still work for the org that owns it.
func TestServeChatCSVServesOwnOrgExport(t *testing.T) {
	const id = "own-org-export-id"
	registerTestExport(t, id, 7, "id,repository,status\n1,repo,completed\n")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/files/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
		User:  &models.User{Email: "owner@example.com"},
		OrgID: 7,
	})

	s := &Server{}
	if err := s.ServeChatCSV(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("ServeChatCSV rejected the owning org: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// A different org must not be able to fetch the export through the
// authenticated endpoint either - the id alone is not authorization.
func TestServeChatCSVRejectsWrongOrg(t *testing.T) {
	const id = "wrong-org-export-id"
	registerTestExport(t, id, 7, "id,repository,status\n1,repo,completed\n")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/chat/files/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	c.Set(string(auth.PermissionContextKey), &auth.PermissionContext{
		User:  &models.User{Email: "intruder@example.com"},
		OrgID: 99,
	})

	s := &Server{}
	if err := s.ServeChatCSV(c); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ServeChatCSV served a different org's export: status=%d body=%q", rec.Code, rec.Body.String())
	}
}
