package users

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
	"github.com/livereview/internal/auditlog"
	storageusers "github.com/livereview/storage/users"
)

// ProfileHandlers contains the profile management handler methods
type ProfileHandlers struct {
	profileService *ProfileService
	db             *sql.DB
	audit          *auditlog.Recorder
}

// NewProfileHandlers creates a new profile handlers instance
func NewProfileHandlers(profileService *ProfileService, db *sql.DB, audit *auditlog.Recorder) *ProfileHandlers {
	return &ProfileHandlers{
		profileService: profileService,
		db:             db,
		audit:          audit,
	}
}

// requireSession rejects API-key-authenticated requests. Self-service profile
// and password changes are credential/account changes, so per the scoping
// guardrails they require an active user session (JWT), never an API key.
func requireSession(c echo.Context) error {
	if method, _ := c.Get("auth_method").(string); method == "api_key" {
		return echo.NewHTTPError(http.StatusForbidden, "this action requires an active user session")
	}
	return nil
}

// actorFor reports the audit actor kind/capacity for a self-service action,
// mirroring AuthHandlers.actorFor: these routes aren't org-scoped, so the
// generic echo-context enrichment can't infer super-admin status on its own.
func (ph *ProfileHandlers) actorFor(userID int64) (auditlog.ActorKind, auditlog.ActorCapacity) {
	var isSuperAdmin bool
	_ = ph.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles r ON ur.role_id = r.id
			WHERE ur.user_id = $1 AND r.name = $2
		)`, userID, auth.RoleSuperAdmin).Scan(&isSuperAdmin)
	if isSuperAdmin {
		return auditlog.ActorKindSuperAdmin, auditlog.ActorCapacityAdmin
	}
	return auditlog.ActorKindMember, auditlog.ActorCapacityMember
}

// GetProfile handles getting the current user's profile
func (ph *ProfileHandlers) GetProfile(c echo.Context) error {
	// Get user from auth context
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Get user profile
	profile, err := ph.profileService.GetUserProfile(c.Request().Context(), user.ID)
	if err != nil {
		if err.Error() == "user not found" {
			return echo.NewHTTPError(http.StatusNotFound, "User profile not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user profile")
	}

	// Get user organizations
	organizations, err := ph.profileService.GetUserOrganizations(c.Request().Context(), user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user organizations")
	}

	response := map[string]interface{}{
		"profile":       profile,
		"organizations": organizations,
	}

	return c.JSON(http.StatusOK, response)
}

// UpdateProfile handles updating the current user's profile. Session-only,
// and the write and its audit event commit atomically (fail-closed).
func (ph *ProfileHandlers) UpdateProfile(c echo.Context) error {
	if err := requireSession(c); err != nil {
		return err
	}

	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	var req UpdateProfileRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	tx, err := ph.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update user profile")
	}
	defer func() { _ = tx.Rollback() }()

	if err := ph.profileService.UpdateUserProfile(c.Request().Context(), tx, user.ID, req); err != nil {
		if errors.Is(err, storageusers.ErrEmailInUse) {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		switch err.Error() {
		case "email cannot be empty", "email is invalid", "first_name is too long", "last_name is too long":
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update user profile")
	}

	if ph.audit != nil {
		uid := user.ID
		actorKind, actorCapacity := ph.actorFor(user.ID)
		if err := ph.audit.RecordTxEcho(c, tx, auditlog.Event{
			ActorUserID:   &uid,
			ActorEmail:    user.Email,
			ActorKind:     actorKind,
			ActorCapacity: actorCapacity,
			AuthMethod:    "session",
			Action:        auditlog.ActionUserUpdate,
			ResourceType:  "user",
			ResourceID:    strconv.FormatInt(user.ID, 10),
			Outcome:       auditlog.OutcomeSuccess,
			Severity:      auditlog.SeverityWarning,
			Details:       auditlog.Details("self_service", true),
		}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to record audit event")
		}
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update user profile")
	}

	profile, err := ph.profileService.GetUserProfile(c.Request().Context(), user.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user profile")
	}

	return c.JSON(http.StatusOK, profile)
}

// ChangePassword handles changing the current user's password. Session-only,
// and the write and its audit event commit atomically (fail-closed).
func (ph *ProfileHandlers) ChangePassword(c echo.Context) error {
	if err := requireSession(c); err != nil {
		return err
	}

	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Parse request
	var req ChangePasswordRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Validate password requirements
	if len(req.NewPassword) < 8 {
		return echo.NewHTTPError(http.StatusBadRequest, "New password must be at least 8 characters long")
	}

	tx, err := ph.db.BeginTx(c.Request().Context(), nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to change password")
	}
	defer func() { _ = tx.Rollback() }()

	if err := ph.profileService.ChangePassword(c.Request().Context(), tx, user.ID, req); err != nil {
		if err.Error() == "user not found" {
			return echo.NewHTTPError(http.StatusNotFound, "User not found")
		}
		if err.Error() == "current password is incorrect" {
			return echo.NewHTTPError(http.StatusUnauthorized, "Current password is incorrect")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to change password")
	}

	if ph.audit != nil {
		uid := user.ID
		actorKind, actorCapacity := ph.actorFor(user.ID)
		if err := ph.audit.RecordTxEcho(c, tx, auditlog.Event{
			ActorUserID:   &uid,
			ActorEmail:    user.Email,
			ActorKind:     actorKind,
			ActorCapacity: actorCapacity,
			AuthMethod:    "session",
			Action:        auditlog.ActionPasswordChange,
			ResourceType:  "user",
			ResourceID:    strconv.FormatInt(user.ID, 10),
			Outcome:       auditlog.OutcomeSuccess,
			Severity:      auditlog.SeverityWarning,
			Details:       auditlog.Details("method", "password"),
		}); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to record audit event")
		}
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to change password")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Password changed successfully. Please log in again with your new password.",
	})
}

// UpdateTimeZone saves the browser's IANA time zone so PR tables show the user's local time.
// It is a no-op when the zone is unchanged, so repeated calls write nothing.
func (ph *ProfileHandlers) UpdateTimeZone(c echo.Context) error {
	if err := requireSession(c); err != nil {
		return err
	}
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	var req struct {
		TimeZone string `json:"time_zone"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	if len(req.TimeZone) > 64 || req.TimeZone == "" || req.TimeZone == "Local" {
		return echo.NewHTTPError(http.StatusBadRequest, "time_zone must be a valid IANA zone, e.g. Asia/Kolkata")
	}
	if _, err := time.LoadLocation(req.TimeZone); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "time_zone must be a valid IANA zone, e.g. Asia/Kolkata")
	}

	if _, err := ph.db.ExecContext(c.Request().Context(),
		`UPDATE users SET time_zone = $1 WHERE id = $2 AND time_zone IS DISTINCT FROM $1`,
		req.TimeZone, user.ID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to save time zone")
	}
	return c.NoContent(http.StatusNoContent)
}
