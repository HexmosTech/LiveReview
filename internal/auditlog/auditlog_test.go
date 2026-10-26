package auditlog

import (
	"encoding/json"
	"github.com/livereview/pkg/models"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestSanitizerRedactsSecretsAndKeepsCounts(t *testing.T) {
	in := Fields{
		"password":      "hunter2",
		"new_password":  "hunter3",
		"github_token":  "ghp_secret",
		"api_key":       "sk-live-secret",
		"client_secret": "abc",
		"authorization": "Bearer x",
		"prompt":        "raw source code",
		"input_tokens":  42,
		"output_tokens": 7,
		"prompt_hash":   "deadbeef",
		"role":          "owner",
		"nested": Fields{
			"refresh_token": "r1",
			"ok":            "yes",
		},
	}
	out := SanitizeFields(in)
	if out["password"] != redactedValue || out["new_password"] != redactedValue {
		t.Fatalf("password not redacted: %#v", out)
	}
	if out["github_token"] != redactedValue || out["api_key"] != redactedValue || out["client_secret"] != redactedValue {
		t.Fatalf("secret keys not redacted: %#v", out)
	}
	if out["authorization"] != redactedValue || out["prompt"] != redactedValue {
		t.Fatalf("sensitive keys not redacted: %#v", out)
	}
	if out["input_tokens"] != 42 || out["output_tokens"] != 7 {
		t.Fatalf("token counts must be preserved: %#v", out)
	}
	if out["prompt_hash"] != "deadbeef" {
		t.Fatalf("prompt hash must be preserved: %#v", out)
	}
	nested, ok := out["nested"].(Fields)
	if !ok {
		t.Fatalf("nested type = %T", out["nested"])
	}
	if nested["refresh_token"] != redactedValue || nested["ok"] != "yes" {
		t.Fatalf("nested sanitization failed: %#v", nested)
	}
	// The caller's map must not be mutated.
	if in["password"] != "hunter2" {
		t.Fatalf("SanitizeFields mutated its input")
	}
}

func TestSanitizeTruncatesLongStrings(t *testing.T) {
	long := strings.Repeat("a", maxStringLen+100)
	out := SanitizeFields(Fields{"note": long})
	got, _ := out["note"].(string)
	if len(got) >= len(long) || !strings.HasSuffix(got, "...[truncated]") {
		t.Fatalf("long string not truncated: len=%d", len(got))
	}
}

func TestChangesOnlyIncludesChangedFields(t *testing.T) {
	before := Fields{"role": "member", "active": true, "old": "x"}
	after := Fields{"role": "owner", "active": true, "new": "y"}
	out := Changes(before, after)
	changes, ok := out["changes"].(Fields)
	if !ok {
		t.Fatalf("changes missing: %#v", out)
	}
	if _, ok := changes["active"]; ok {
		t.Fatalf("unchanged field included")
	}
	if _, ok := changes["role"]; !ok {
		t.Fatalf("changed field missing")
	}
	if _, ok := changes["old"]; !ok {
		t.Fatalf("removed field missing")
	}
	if _, ok := changes["new"]; !ok {
		t.Fatalf("added field missing")
	}
}

func TestCursorRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	c := encodeCursor(now, 12345)
	gotT, gotID, err := decodeCursor(c)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !gotT.Equal(now) || gotID != 12345 {
		t.Fatalf("round trip = %v/%d", gotT, gotID)
	}
	if _, _, err := decodeCursor("not-a-cursor"); err == nil {
		t.Fatalf("expected error for invalid cursor")
	}
}

func TestOwnerScopeHidesAdminCapacity(t *testing.T) {
	q, err := NewQuery(Scope{OrgID: 7, UserID: 1, IsOwner: true}, Filters{})
	if err != nil {
		t.Fatal(err)
	}
	where, args := q.Where()
	if !strings.Contains(where, "org_id = $1") {
		t.Fatalf("org scope missing: %s", where)
	}
	if !strings.Contains(where, "actor_capacity <> 'admin'") {
		t.Fatalf("owner should hide admin capacity: %s", where)
	}
	if !strings.Contains(where, "actor_kind <> 'super_admin'") {
		t.Fatalf("owner should hide super-admin actors: %s", where)
	}
	if len(args) != 1 || args[0] != int64(7) {
		t.Fatalf("args = %#v", args)
	}
}

func TestMemberScopeIsSelfOnly(t *testing.T) {
	q, err := NewQuery(Scope{OrgID: 7, UserID: 42}, Filters{})
	if err != nil {
		t.Fatal(err)
	}
	where, args := q.Where()
	if !strings.Contains(where, "actor_user_id = $2") {
		t.Fatalf("member self filter missing: %s", where)
	}
	if len(args) != 2 || args[1] != int64(42) {
		t.Fatalf("args = %#v", args)
	}
}

func TestSuperAdminInOrgSeesEverything(t *testing.T) {
	q, err := NewQuery(Scope{OrgID: 7, UserID: 1, IsSuperAdmin: true}, Filters{})
	if err != nil {
		t.Fatal(err)
	}
	where, _ := q.Where()
	if strings.Contains(where, "actor_capacity <> 'admin'") || strings.Contains(where, "actor_user_id =") {
		t.Fatalf("super admin should be unrestricted in org view: %s", where)
	}
	if !strings.Contains(where, "org_id = ") {
		t.Fatalf("super admin org view must still scope to the org: %s", where)
	}
}

func TestAdminViewRequiresSuperAdmin(t *testing.T) {
	if _, err := NewQuery(Scope{OrgID: 7, UserID: 1, AdminView: true}, Filters{}); err == nil {
		t.Fatalf("expected admin view to require super admin")
	}
	q, err := NewQuery(Scope{UserID: 1, IsSuperAdmin: true, AdminView: true}, Filters{})
	if err != nil {
		t.Fatal(err)
	}
	where, _ := q.Where()
	if strings.Contains(where, "org_id = ") {
		t.Fatalf("admin view without org filter should be global: %s", where)
	}
}

func TestMissingScopeRejected(t *testing.T) {
	if _, err := NewQuery(Scope{}, Filters{}); err == nil {
		t.Fatalf("expected missing scope to be rejected")
	}
}

func TestEscapeLike(t *testing.T) {
	if got := escapeLike(`50%_off\`); got != `50\%\_off\\` {
		t.Fatalf("escapeLike = %q", got)
	}
}

func TestSanitizeJSONRoundTrip(t *testing.T) {
	out := SanitizeFields(Fields{"a": []any{"x", "y"}, "n": json.Number("3"), "when": time.Now()})
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("sanitized output not JSON-marshalable: %v", err)
	}
}

func TestRouteCoverageRequiresSpec(t *testing.T) {
	e := echo.New()
	h := func(c echo.Context) error { return nil }
	e.POST("/api/v1/orgs/:org_id/widgets", h)
	e.GET("/api/v1/orgs/:org_id/widgets", h)
	reg := NewRegistry()

	v := ValidateRouteCoverage(e.Routes(), map[string]string{}, reg)
	if len(v) != 1 || v[0].Method != "POST" {
		t.Fatalf("expected one POST violation, got %#v", v)
	}

	reg.Register("POST", "/api/v1/orgs/:org_id/widgets", Spec{Action: ActionUserCreate})
	if v := ValidateRouteCoverage(e.Routes(), map[string]string{}, reg); len(v) != 0 {
		t.Fatalf("expected no violations, got %#v", v)
	}
}

func TestRouteCoverageAllowlist(t *testing.T) {
	e := echo.New()
	h := func(c echo.Context) error { return nil }
	e.PUT("/api/v1/orgs/:org_id/legacy/:id", h)
	reg := NewRegistry()
	allow := map[string]string{"PUT /api/v1/orgs/:org_id/legacy/:id": "predates audit module"}
	if v := ValidateRouteCoverage(e.Routes(), allow, reg); len(v) != 0 {
		t.Fatalf("allowlist not honored: %#v", v)
	}
}

func TestActionCatalogDocumented(t *testing.T) {
	b, err := os.ReadFile("../../docs/audit-events.md")
	if err != nil {
		t.Fatalf("docs/audit-events.md missing: %v", err)
	}
	doc := string(b)
	for _, a := range AllActions() {
		if !strings.Contains(doc, "`"+string(a.Action)+"`") {
			t.Errorf("action %q is missing from docs/audit-events.md", a.Action)
		}
	}
	if !strings.Contains(doc, "SIEM") {
		t.Errorf("docs should explain the SIEM contract")
	}
}

func TestMemberScopeIsStrictlySelfOnly(t *testing.T) {
	q, err := NewQuery(Scope{OrgID: 7, UserID: 42, ActorEmail: "Person@Example.com"}, Filters{})
	if err != nil {
		t.Fatal(err)
	}
	where, _ := q.Where()
	if strings.Contains(where, "auth.login.failure") || strings.Contains(where, "lower(actor_email)") {
		t.Fatalf("member scope must not include failed-login attempts by email: %s", where)
	}
}

func TestSuperAdminOrgActionGetsAdminCapacity(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/orgs/1/api-keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/orgs/:org_id/api-keys")
	c.Set("user_role", "super_admin")
	c.Set("user", &models.User{ID: 9, Email: "admin@example.com"})
	c.Set("auth_method", "session")

	var ev Event
	EnrichFromEcho(c, &ev)
	if ev.ActorKind != ActorKindSuperAdmin {
		t.Fatalf("actor kind = %q, want super_admin", ev.ActorKind)
	}
	if ev.ActorCapacity != ActorCapacityAdmin {
		t.Fatalf("actor capacity = %q, want admin", ev.ActorCapacity)
	}
}

func TestSuperAdminAPIActionGetsAdminCapacity(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/orgs/1/api-keys", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/orgs/:org_id/api-keys")
	c.Set("user_role", "super_admin")
	c.Set("user", &models.User{ID: 9, Email: "admin@example.com"})
	c.Set("auth_method", "api_key")

	var ev Event
	EnrichFromEcho(c, &ev)
	if ev.ActorKind != ActorKindAPIKey {
		t.Fatalf("actor kind = %q, want api_key", ev.ActorKind)
	}
	if ev.ActorCapacity != ActorCapacityAdmin {
		t.Fatalf("super-admin key capacity = %q, want admin", ev.ActorCapacity)
	}
}

func TestOwnerOrgActionGetsMemberCapacity(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/orgs/1/users/2", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/orgs/:org_id/users/:user_id")
	c.Set("user_role", "owner")
	c.Set("user", &models.User{ID: 3, Email: "owner@example.com"})
	c.Set("auth_method", "session")

	var ev Event
	EnrichFromEcho(c, &ev)
	if ev.ActorKind != ActorKindMember || ev.ActorCapacity != ActorCapacityMember {
		t.Fatalf("owner actor = %q/%q, want member/member", ev.ActorKind, ev.ActorCapacity)
	}
}
