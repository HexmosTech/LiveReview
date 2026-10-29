package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	apimiddleware "github.com/livereview/internal/api/middleware"
	"github.com/livereview/internal/api/users"
	"github.com/livereview/internal/auditlog"
	"github.com/livereview/internal/license"
	"github.com/livereview/internal/scim"
)

// orgHasSCIMFeature mirrors orgHasSSOFeature (internal/api/sso_config.go) -
// checks the org's own plan rather than a hardcoded self-hosted bit, so
// adding "scim" to a cloud plan's Features later needs no code change here.
func (s *Server) orgHasSCIMFeature(orgID int64) bool {
	var planCode string
	_ = s.db.QueryRow(`SELECT current_plan_code FROM org_billing_state WHERE org_id = $1`, orgID).Scan(&planCode)
	return license.PlanType(planCode).HasFeature("scim")
}

// requireSCIMBearerAuth authenticates SCIM clients (Okta, Azure AD, Google
// Workspace) via their org's static bearer token - there is no user
// session on this path, per the SCIM 2.0 RFC. On success it sets
// "scim_org_id" in the echo context for the handlers below.
func (s *Server) requireSCIMBearerAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		authHeader := c.Request().Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			return c.JSON(http.StatusUnauthorized, scim.NewError(http.StatusUnauthorized, "missing bearer token"))
		}
		token := strings.TrimSpace(strings.TrimPrefix(authHeader, prefix))
		orgID, err := s.scimTokens.Authenticate(token)
		if err != nil {
			return c.JSON(http.StatusUnauthorized, scim.NewError(http.StatusUnauthorized, "invalid, expired, or revoked token"))
		}
		if !s.orgHasSCIMFeature(orgID) {
			return c.JSON(http.StatusForbidden, scim.NewError(http.StatusForbidden, "SCIM requires an enterprise self-hosted license"))
		}
		c.Set("scim_org_id", orgID)
		return next(c)
	}
}

func scimOrgID(c echo.Context) int64 {
	return c.Get("scim_org_id").(int64)
}

// scimResourceFromUser converts an internal user record into the SCIM
// shape IdPs expect.
func scimResourceFromUser(u *users.UserWithRole) scim.UserResource {
	first, last := "", ""
	if u.FirstName != nil {
		first = *u.FirstName
	}
	if u.LastName != nil {
		last = *u.LastName
	}
	return scim.UserResource{
		Schemas:  []string{scim.SchemaUser},
		ID:       strconv.FormatInt(u.ID, 10),
		UserName: u.Email,
		Name:     &scim.NameResource{GivenName: first, FamilyName: last},
		Emails:   []scim.EmailResource{{Value: u.Email, Primary: true}},
		Active:   u.IsActive,
		Meta: &scim.MetaResource{
			ResourceType: "User",
			Created:      u.CreatedAt.Format(time.RFC3339),
			LastModified: u.UpdatedAt.Format(time.RFC3339),
		},
	}
}

// primaryEmail extracts the identifying email from a SCIM UserResource -
// IdPs vary on whether they populate userName or emails[] with the address
// that matters, so this checks both.
func primaryEmail(r scim.UserResource) string {
	for _, e := range r.Emails {
		if e.Primary && e.Value != "" {
			return strings.ToLower(strings.TrimSpace(e.Value))
		}
	}
	if len(r.Emails) > 0 && r.Emails[0].Value != "" {
		return strings.ToLower(strings.TrimSpace(r.Emails[0].Value))
	}
	if strings.Contains(r.UserName, "@") {
		return strings.ToLower(strings.TrimSpace(r.UserName))
	}
	return ""
}

// parseEmailFilter extracts the target email from the simple SCIM filter
// expressions IdPs actually send when checking "does this user already
// exist" before creating - userName eq "x@y.com" or emails.value eq
// "x@y.com". Full SCIM filter grammar (and/or/not, other attributes) is not
// needed for this use case and isn't parsed.
func parseEmailFilter(filter string) string {
	filter = strings.TrimSpace(filter)
	for _, attr := range []string{"userName", "emails.value", "emails"} {
		prefix := attr + " eq "
		if idx := strings.Index(filter, prefix); idx != -1 {
			rest := strings.TrimSpace(filter[idx+len(prefix):])
			rest = strings.Trim(rest, `"`)
			return strings.ToLower(rest)
		}
	}
	return ""
}

// SCIMListUsers handles GET /scim/v2/Users. IdPs call this both to page
// through the directory and, far more commonly, with a `filter` to check
// whether a user already exists before POSTing a create.
func (s *Server) SCIMListUsers(c echo.Context) error {
	orgID := scimOrgID(c)
	email := parseEmailFilter(c.QueryParam("filter"))

	type row struct {
		id                   int64
		email                string
		isActive             bool
		firstName, lastName  sql.NullString
		createdAt, updatedAt time.Time
	}
	var rows []row

	if email != "" {
		var r row
		err := s.db.QueryRow(`
			SELECT u.id, u.email, u.is_active, u.first_name, u.last_name, u.created_at, u.updated_at
			FROM users u JOIN user_roles ur ON ur.user_id = u.id
			WHERE u.email = $1 AND ur.org_id = $2
		`, email, orgID).Scan(&r.id, &r.email, &r.isActive, &r.firstName, &r.lastName, &r.createdAt, &r.updatedAt)
		if err == nil {
			rows = append(rows, r)
		} else if err != sql.ErrNoRows {
			return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to query users"))
		}
	} else {
		sqlRows, err := s.db.Query(`
			SELECT u.id, u.email, u.is_active, u.first_name, u.last_name, u.created_at, u.updated_at
			FROM users u JOIN user_roles ur ON ur.user_id = u.id
			WHERE ur.org_id = $1 ORDER BY u.id LIMIT 200
		`, orgID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to query users"))
		}
		defer sqlRows.Close()
		for sqlRows.Next() {
			var r row
			if err := sqlRows.Scan(&r.id, &r.email, &r.isActive, &r.firstName, &r.lastName, &r.createdAt, &r.updatedAt); err != nil {
				return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to read users"))
			}
			rows = append(rows, r)
		}
	}

	resources := make([]scim.UserResource, 0, len(rows))
	for _, r := range rows {
		first, last := "", ""
		if r.firstName.Valid {
			first = r.firstName.String
		}
		if r.lastName.Valid {
			last = r.lastName.String
		}
		resources = append(resources, scim.UserResource{
			Schemas:  []string{scim.SchemaUser},
			ID:       strconv.FormatInt(r.id, 10),
			UserName: r.email,
			Name:     &scim.NameResource{GivenName: first, FamilyName: last},
			Emails:   []scim.EmailResource{{Value: r.email, Primary: true}},
			Active:   r.isActive,
			Meta: &scim.MetaResource{
				ResourceType: "User",
				Created:      r.createdAt.Format(time.RFC3339),
				LastModified: r.updatedAt.Format(time.RFC3339),
			},
		})
	}

	return c.JSON(http.StatusOK, scim.ListResponse{
		Schemas:      []string{scim.SchemaListResponse},
		TotalResults: len(resources),
		ItemsPerPage: len(resources),
		StartIndex:   1,
		Resources:    resources,
	})
}

// SCIMCreateUser handles POST /scim/v2/Users.
func (s *Server) SCIMCreateUser(c echo.Context) error {
	orgID := scimOrgID(c)

	var req scim.UserResource
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, scim.NewError(http.StatusBadRequest, "invalid request body"))
	}
	email := primaryEmail(req)
	if email == "" {
		return c.JSON(http.StatusBadRequest, scim.NewError(http.StatusBadRequest, "userName or emails is required"))
	}
	first, last := "", ""
	if req.Name != nil {
		first, last = req.Name.GivenName, req.Name.FamilyName
	}

	user, created, err := s.userService.ProvisionSCIMUser(orgID, email, first, last)
	if err != nil {
		if errors.Is(err, users.ErrSCIMAccountDeactivated) {
			return c.JSON(http.StatusConflict, scim.NewError(http.StatusConflict, "this account was deactivated by an organization admin"))
		}
		return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to provision user"))
	}
	if created && s.auditRecorder != nil {
		_ = s.auditRecorder.Webhook(c.Request().Context(), orgID, auditlog.ActionSCIMUserCreate, auditlog.Details("email", email))
	}
	return c.JSON(http.StatusCreated, scimResourceFromUser(user))
}

// SCIMGetUser handles GET /scim/v2/Users/:id.
func (s *Server) SCIMGetUser(c echo.Context) error {
	orgID := scimOrgID(c)
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, scim.NewError(http.StatusBadRequest, "invalid id"))
	}
	user, err := s.userService.GetUserInOrg(orgID, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, scim.NewError(http.StatusNotFound, "user not found"))
	}
	return c.JSON(http.StatusOK, scimResourceFromUser(user))
}

// SCIMPatchUser handles PATCH /scim/v2/Users/:id - the operation that
// matters most for offboarding: IdPs send
// {"Operations":[{"op":"replace","value":{"active":false}}]} to deprovision.
func (s *Server) SCIMPatchUser(c echo.Context) error {
	orgID := scimOrgID(c)
	userID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, scim.NewError(http.StatusBadRequest, "invalid id"))
	}
	if _, err := s.userService.GetUserInOrg(orgID, userID); err != nil {
		return c.JSON(http.StatusNotFound, scim.NewError(http.StatusNotFound, "user not found"))
	}

	var req scim.PatchRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, scim.NewError(http.StatusBadRequest, "invalid request body"))
	}

	for _, op := range req.Operations {
		active, ok := patchActiveValue(op)
		if !ok {
			continue
		}
		if active {
			if err := s.userService.ReactivateUserInOrgBySCIM(userID); err != nil {
				return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to reactivate user"))
			}
			if s.auditRecorder != nil {
				_ = s.auditRecorder.Webhook(c.Request().Context(), orgID, auditlog.ActionSCIMUserReactivate, auditlog.Details("user_id", userID))
			}
		} else {
			if err := s.userService.DeactivateUserInOrgBySCIM(userID); err != nil {
				return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to deactivate user"))
			}
			if s.auditRecorder != nil {
				_ = s.auditRecorder.Webhook(c.Request().Context(), orgID, auditlog.ActionSCIMUserDeactivate, auditlog.Details("user_id", userID))
			}
		}
	}

	user, err := s.userService.GetUserInOrg(orgID, userID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, scim.NewError(http.StatusInternalServerError, "failed to reload user"))
	}
	return c.JSON(http.StatusOK, scimResourceFromUser(user))
}

// patchActiveValue reports whether op sets the SCIM "active" attribute,
// and what it was set to. Handles both the common {"active": false}
// whole-object replace and the path-qualified {"path":"active","value":false} form.
func patchActiveValue(op scim.PatchOperation) (active bool, matched bool) {
	if !strings.EqualFold(op.Op, "replace") {
		return false, false
	}
	if strings.EqualFold(op.Path, "active") {
		if b, ok := op.Value.(bool); ok {
			return b, true
		}
		return false, false
	}
	if op.Path == "" {
		if m, ok := op.Value.(map[string]any); ok {
			if b, ok := m["active"].(bool); ok {
				return b, true
			}
		}
	}
	return false, false
}

// --- SCIM token management (session-authenticated, owner-only) ---

// requireSCIMConfigAccess mirrors requireSSOAccess (internal/api/sso_config.go):
// Sensitive Operations Gate + Session-Only Gating + self-hosted-enterprise
// feature check, for the org's SCIM token itself.
func (s *Server) requireSCIMConfigAccess(c echo.Context, orgID int64) error {
	if err := requireSession(c); err != nil {
		return err
	}
	pc := auth.GetPermissionContext(c)
	if pc == nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only org owners can manage SCIM provisioning"})
	}
	if err := pc.RequireOrgOwner(); err != nil {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only org owners can manage SCIM provisioning"})
	}
	if apimiddleware.IsCloudMode() {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SCIM is available on self-hosted enterprise deployments only"})
	}
	if !s.orgHasSCIMFeature(orgID) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "SCIM requires an enterprise self-hosted license"})
	}
	return nil
}

type scimTokenResponse struct {
	TokenPrefix string  `json:"token_prefix"`
	CreatedAt   string  `json:"created_at"`
	Token       *string `json:"token,omitempty"` // only populated once, on creation
}

// GetSCIMToken is GET /orgs/:org_id/scim-token - returns the existing
// token's metadata (never the plaintext, which is only ever shown at
// creation time).
func (s *Server) GetSCIMToken(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	if err := s.requireSCIMConfigAccess(c, orgID); err != nil {
		return err
	}
	tok, err := s.scimTokens.Get(orgID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load SCIM token"})
	}
	if tok == nil {
		return c.JSON(http.StatusOK, map[string]any{"exists": false})
	}
	return c.JSON(http.StatusOK, scimTokenResponse{TokenPrefix: tok.TokenPrefix, CreatedAt: tok.CreatedAt.Format(time.RFC3339)})
}

// CreateSCIMToken is POST /orgs/:org_id/scim-token - generates a new token,
// replacing (rotating out) any existing one. Registered via s.audited with
// ActionSCIMTokenCreate.
func (s *Server) CreateSCIMToken(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	if err := s.requireSCIMConfigAccess(c, orgID); err != nil {
		return err
	}
	pc := auth.GetPermissionContext(c)
	var actorUserID int64
	if pc != nil && pc.User != nil {
		actorUserID = pc.User.ID
	}

	if err := s.scimTokens.Revoke(orgID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to rotate SCIM token"})
	}
	plaintext, tok, err := s.scimTokens.Create(orgID, actorUserID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to create SCIM token"})
	}
	auditlog.MarkRecorded(c)
	return c.JSON(http.StatusOK, scimTokenResponse{
		TokenPrefix: tok.TokenPrefix,
		CreatedAt:   tok.CreatedAt.Format(time.RFC3339),
		Token:       &plaintext,
	})
}

// RevokeSCIMToken is DELETE /orgs/:org_id/scim-token.
func (s *Server) RevokeSCIMToken(c echo.Context) error {
	orgID, err := strconv.ParseInt(c.Param("org_id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid org_id"})
	}
	if err := s.requireSCIMConfigAccess(c, orgID); err != nil {
		return err
	}
	if err := s.scimTokens.Revoke(orgID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to revoke SCIM token"})
	}
	auditlog.MarkRecorded(c)
	return c.JSON(http.StatusOK, map[string]string{"message": "SCIM token revoked"})
}
