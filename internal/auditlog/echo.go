package auditlog

import (
	"database/sql"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/pkg/models"
)

// EnrichFromEcho fills actor/org/request fields from the verified request
// context. It never trusts client-supplied ids: org_id and user come from the
// middleware chain, not from query/path parameters.
func EnrichFromEcho(c echo.Context, ev *Event) {
	if c == nil || ev == nil {
		return
	}
	if ev.OrgID == 0 {
		if v, ok := c.Get("org_id").(int64); ok {
			ev.OrgID = v
		}
	}
	if user, ok := c.Get("user").(*models.User); ok && user != nil {
		if ev.ActorUserID == nil && user.ID > 0 {
			id := user.ID
			ev.ActorUserID = &id
		}
		if ev.ActorEmail == "" {
			ev.ActorEmail = user.Email
		}
		if ev.ActorName == "" {
			ev.ActorName = user.FullName()
		}
	}
	if ev.AuthMethod == "" {
		if v, ok := c.Get("auth_method").(string); ok {
			ev.AuthMethod = v
		}
	}
	if ev.APIKeyID == nil {
		if v, ok := c.Get("api_key_id").(int64); ok && v > 0 {
			ev.APIKeyID = &v
		}
	}
	if ev.ActorKind == "" || ev.ActorKind == ActorKindSystem {
		switch {
		case ev.AuthMethod == "api_key":
			ev.ActorKind = ActorKindAPIKey
		case isSuperAdminRequest(c):
			ev.ActorKind = ActorKindSuperAdmin
		case ev.ActorUserID != nil:
			ev.ActorKind = ActorKindMember
		}
	}
	if ev.ActorCapacity == "" || ev.ActorCapacity == ActorCapacitySystem {
		if ev.ActorKind == ActorKindSystem || ev.ActorKind == ActorKindWebhook {
			ev.ActorCapacity = ActorCapacitySystem
		} else if isAdminRequest(c) || isSuperAdminRequest(c) || ev.ActorKind == ActorKindSuperAdmin {
			ev.ActorCapacity = ActorCapacityAdmin
		} else {
			ev.ActorCapacity = ActorCapacityMember
		}
	}
	if ev.IP == "" {
		ev.IP = c.RealIP()
	}
	if ev.UserAgent == "" {
		ev.UserAgent = c.Request().UserAgent()
	}
	if ev.RequestID == "" {
		ev.RequestID = RequestID(c)
	}
	if ev.Method == "" {
		ev.Method = c.Request().Method
	}
	if ev.Path == "" {
		ev.Path = c.Path()
	}
	if ev.Source == "" {
		ev.Source = SourceAPI
	}
}

func isAdminRequest(c echo.Context) bool {
	return strings.HasPrefix(c.Path(), "/api/v1/admin/")
}

func isSuperAdminRequest(c echo.Context) bool {
	if v, ok := c.Get("user_role").(string); ok && v == "super_admin" {
		return true
	}
	// An admin route is only reachable by a super admin.
	return isAdminRequest(c)
}

// RecordEcho writes an event in its own transaction and marks the request as
// audited so the middleware does not double-record it.
func (r *Recorder) RecordEcho(c echo.Context, ev Event) error {
	EnrichFromEcho(c, &ev)
	if err := r.Record(c.Request().Context(), ev); err != nil {
		return err
	}
	MarkRecorded(c)
	return nil
}

// RecordTxEcho writes an event inside the caller's transaction and marks the
// request as audited on success.
func (r *Recorder) RecordTxEcho(c echo.Context, tx *sql.Tx, ev Event) error {
	EnrichFromEcho(c, &ev)
	if err := r.RecordTx(c.Request().Context(), tx, ev); err != nil {
		return err
	}
	MarkRecorded(c)
	return nil
}

// ParamID reads a path parameter as a resource id string, returning "" when
// absent. A malformed id never breaks the audit write.
func ParamID(c echo.Context, name string) string {
	if c == nil || name == "" {
		return ""
	}
	return c.Param(name)
}

// OrgIDString is a small helper for details payloads.
func OrgIDString(id int64) string { return strconv.FormatInt(id, 10) }

// SensitiveRead records a data-exposure read: an export, a file download, or a
// view of the audit log itself. It is best-effort (the read must not fail
// because auditing failed) but marks the request as audited so the middleware
// does not double-record it.
func (r *Recorder) SensitiveRead(c echo.Context, action Action, details Fields) error {
	if r == nil || c == nil {
		return nil
	}
	ev := Event{
		Action:   action,
		Outcome:  OutcomeSuccess,
		Severity: SeverityInfo,
		Details:  details,
		Source:   SourceAPI,
	}
	EnrichFromEcho(c, &ev)
	if err := r.Record(c.Request().Context(), ev); err != nil {
		return err
	}
	MarkRecorded(c)
	return nil
}
