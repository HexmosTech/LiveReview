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

	chatExportsMu.Lock()
	chatExports[id] = &chatExportEntry{
		Path:      path,
		TmpDir:    tmpDir,
		CreatedAt: time.Now(),
		Filename:  "export.csv",
		OrgID:     orgID,
	}
	chatExportsMu.Unlock()
	t.Cleanup(func() {
		chatExportsMu.Lock()
		delete(chatExports, id)
		chatExportsMu.Unlock()
	})
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
