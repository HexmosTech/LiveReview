package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

// UserIdentityDTO is one user_identities row.
type UserIdentityDTO struct {
	ID      int64  `json:"id"`
	OrgID   int64  `json:"org_id"`
	UserID  int64  `json:"user_id"`
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	CreatedBy *int64 `json:"created_by,omitempty"`
}

// listUserIdentities handles GET /api/v1/orgs/:org_id/users/:user_id/identities.
func (s *Server) listUserIdentities(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	userID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid user id")
	}

	rows, err := s.db.QueryContext(c.Request().Context(), `
		SELECT id, org_id, user_id, kind, value, created_by
		FROM user_identities WHERE org_id = $1 AND user_id = $2 ORDER BY kind, value`, orgID, userID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "query failed: "+err.Error())
	}
	defer rows.Close()

	out := []UserIdentityDTO{}
	for rows.Next() {
		var d UserIdentityDTO
		if err := rows.Scan(&d.ID, &d.OrgID, &d.UserID, &d.Kind, &d.Value, &d.CreatedBy); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "scan failed: "+err.Error())
		}
		out = append(out, d)
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": out})
}

// addUserIdentity handles POST /api/v1/orgs/:org_id/users/:user_id/identities.
// Any member can add an identity for themselves; attaching to another user
// requires owner/admin. Session-only (no API key).
func (s *Server) addUserIdentity(c echo.Context) error {
	if err := requireSession(c); err != nil {
		return err
	}
	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()
	targetUserID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid user id")
	}

	if targetUserID != pc.GetUserID() {
		if err := pc.RequireOrgOwner(); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusForbidden, "only owners/admins can attach identities to another user")
		}
	}

	var body struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if err := c.Bind(&body); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid body")
	}
	body.Kind = strings.ToLower(strings.TrimSpace(body.Kind))
	body.Value = normalizeIdentityValue(body.Kind, body.Value)
	if body.Kind != "email" && body.Kind != "username" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "kind must be email or username")
	}
	if body.Value == "" {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "value is required")
	}

	var id int64
	var createdBy interface{}
	if pc.User != nil {
		createdBy = pc.User.ID
	}
	err = s.db.QueryRowContext(c.Request().Context(), `
		INSERT INTO user_identities (org_id, user_id, kind, value, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (org_id, kind, value) DO UPDATE SET user_id = EXCLUDED.user_id, updated_at = now()
		RETURNING id`, orgID, targetUserID, body.Kind, body.Value, createdBy).Scan(&id)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusConflict, "identity already maps to a different user in this org")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"id": id, "kind": body.Kind, "value": body.Value})
}

// removeUserIdentity handles DELETE /api/v1/orgs/:org_id/users/:user_id/identities/:id.
func (s *Server) removeUserIdentity(c echo.Context) error {
	if err := requireSession(c); err != nil {
		return err
	}
	pc := auth.MustGetPermissionContext(c)
	orgID := pc.GetOrgID()
	targetUserID, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid user id")
	}
	identityID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusBadRequest, "invalid identity id")
	}

	// Any member can remove their own identities; others require owner/admin.
	var owner int64
	if err := s.db.QueryRowContext(c.Request().Context(),
		`SELECT user_id FROM user_identities WHERE id = $1 AND org_id = $2`, identityID, orgID).Scan(&owner); err != nil {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "identity not found")
	}
	if owner != pc.GetUserID() && targetUserID != pc.GetUserID() {
		if err := pc.RequireOrgOwner(); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusForbidden, "only owners/admins can remove another user's identity")
		}
	}

	res, err := s.db.ExecContext(c.Request().Context(),
		`DELETE FROM user_identities WHERE id = $1 AND org_id = $2 AND user_id = $3`, identityID, orgID, targetUserID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "delete failed: "+err.Error())
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return JSONErrorWithEnvelope(c, http.StatusNotFound, "identity not found")
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"status": "deleted"})
}

// listOrgIdentities handles GET /api/v1/orgs/:org_id/identities — the full index
// used for author-mapping lookup/prefill.
func (s *Server) listOrgIdentities(c echo.Context) error {
	orgID, ok := auth.GetOrgIDFromContext(c)
	if !ok {
		return JSONErrorWithEnvelope(c, http.StatusUnauthorized, "org context required")
	}
	rows, err := s.db.QueryContext(c.Request().Context(), `
		SELECT id, org_id, user_id, kind, value, created_by
		FROM user_identities WHERE org_id = $1 ORDER BY kind, value`, orgID)
	if err != nil {
		return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "query failed: "+err.Error())
	}
	defer rows.Close()

	out := []UserIdentityDTO{}
	for rows.Next() {
		var d UserIdentityDTO
		if err := rows.Scan(&d.ID, &d.OrgID, &d.UserID, &d.Kind, &d.Value, &d.CreatedBy); err != nil {
			return JSONErrorWithEnvelope(c, http.StatusInternalServerError, "scan failed: "+err.Error())
		}
		out = append(out, d)
	}
	return JSONWithEnvelope(c, http.StatusOK, map[string]interface{}{"rows": out})
}

func normalizeIdentityValue(kind, value string) string {
	v := strings.TrimSpace(value)
	if kind == "email" {
		return strings.ToLower(v)
	}
	return strings.ToLower(v)
}
