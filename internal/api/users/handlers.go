package users

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/api/auth"
)

// UserHandlers contains the user management handler methods
type UserHandlers struct {
	userService *UserService
	db          *sql.DB
}

// NewUserHandlers creates a new user handlers instance
func NewUserHandlers(userService *UserService, db *sql.DB) *UserHandlers {
	return &UserHandlers{
		userService: userService,
		db:          db,
	}
}

// CreateUser handles creating a new user in an organization
func (uh *UserHandlers) CreateUser(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to create users
	if !permCtx.HasPermission(auth.PermissionCreateUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot create users")
	}

	// Parse request
	var req CreateUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Create user
	user, err := uh.userService.CreateUserInOrg(permCtx.OrgID, permCtx.User.ID, req)
	if err != nil {
		if err.Error() == "user with email "+req.Email+" already exists" {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create user")
	}

	return c.JSON(http.StatusCreated, user)
}

// GetUser handles getting a specific user in an organization
func (uh *UserHandlers) GetUser(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Check permission to view users
	if !permCtx.HasPermission(auth.PermissionViewUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot view users")
	}

	// Get user
	user, err := uh.userService.GetUserInOrg(permCtx.OrgID, userID)
	if err != nil {
		if err.Error() == "user not found in organization" {
			return echo.NewHTTPError(http.StatusNotFound, "User not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user")
	}

	return c.JSON(http.StatusOK, user)
}

// ListUsers handles listing users in an organization with pagination
func (uh *UserHandlers) ListUsers(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to view users
	if !permCtx.HasPermission(auth.PermissionViewUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot view users")
	}

	// Parse pagination parameters
	offset := 0
	limit := 50 // Default page size

	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Get users
	users, totalCount, err := uh.userService.ListUsersInOrg(permCtx.OrgID, offset, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to list users")
	}

	response := map[string]interface{}{
		"users":       users,
		"total_count": totalCount,
		"offset":      offset,
		"limit":       limit,
	}

	return c.JSON(http.StatusOK, response)
}

// UpdateUser handles updating a user in an organization
func (uh *UserHandlers) UpdateUser(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to manage users
	if !permCtx.HasPermission(auth.PermissionEditUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot update users")
	}

	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Parse request
	var req UpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Update user
	user, err := uh.userService.UpdateUserInOrg(permCtx.OrgID, userID, permCtx.User.ID, req)
	if err != nil {
		if err.Error() == "user not found in organization" {
			return echo.NewHTTPError(http.StatusNotFound, "User not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to update user")
	}

	return c.JSON(http.StatusOK, user)
}

// DeactivateUser handles deactivating a user in an organization
func (uh *UserHandlers) DeactivateUser(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to manage users
	if !permCtx.HasPermission(auth.PermissionDeleteUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot deactivate users")
	}

	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Prevent users from deactivating themselves
	if userID == permCtx.User.ID {
		return echo.NewHTTPError(http.StatusBadRequest, "Cannot deactivate yourself")
	}

	// Deactivate user
	err = uh.userService.DeactivateUserInOrg(permCtx.OrgID, userID, permCtx.User.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to deactivate user")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "User deactivated successfully",
	})
}

// ChangeUserRole handles changing a user's role in an organization
func (uh *UserHandlers) ChangeUserRole(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to manage users
	if !permCtx.HasPermission(auth.PermissionManageRoles) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot change user roles")
	}

	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Parse request
	var req struct {
		RoleID int64 `json:"role_id" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Update user role
	updateReq := UpdateUserRequest{
		RoleID: &req.RoleID,
	}

	user, err := uh.userService.UpdateUserInOrg(permCtx.OrgID, userID, permCtx.User.ID, updateReq)
	if err != nil {
		if err.Error() == "user not found in organization" {
			return echo.NewHTTPError(http.StatusNotFound, "User not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to change user role")
	}

	return c.JSON(http.StatusOK, user)
}

// ForcePasswordReset handles forcing a user to reset their password
func (uh *UserHandlers) ForcePasswordReset(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to manage users
	if !permCtx.HasPermission(auth.PermissionEditUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot force password reset")
	}

	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Force password reset
	err = uh.userService.ForcePasswordReset(permCtx.OrgID, userID, permCtx.User.ID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to force password reset")
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "Password reset required - user will be prompted to change password on next login",
	})
}

// GetUserAuditLog handles getting the audit log for a user
func (uh *UserHandlers) GetUserAuditLog(c echo.Context) error {
	// Get permission context from middleware
	permCtx := auth.MustGetPermissionContext(c)

	// Check permission to view users (audit logs are sensitive)
	if !permCtx.HasPermission(auth.PermissionViewUsers) {
		return echo.NewHTTPError(http.StatusForbidden, "Permission denied: cannot view audit logs")
	}

	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Parse pagination parameters
	offset := 0
	limit := 20 // Default page size for audit logs

	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	// Get audit log
	auditEntries, err := uh.userService.GetUserAuditLog(permCtx.OrgID, userID, offset, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get audit log")
	}

	response := map[string]interface{}{
		"audit_entries": auditEntries,
		"offset":        offset,
		"limit":         limit,
	}

	return c.JSON(http.StatusOK, response)
}

// ============================================================================
// SUPER ADMIN GLOBAL MANAGEMENT HANDLERS
// ============================================================================

// ListAllUsers handles listing all users across all organizations (super admin only)
func (uh *UserHandlers) ListAllUsers(c echo.Context) error {
	// Parse pagination parameters
	offset := 0
	limit := 50 // Default page size

	if offsetStr := c.QueryParam("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	if limitStr := c.QueryParam("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Get all users (super admin endpoints have RequireSuperAdmin middleware)
	users, totalCount, err := uh.userService.ListAllUsers(offset, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to list all users")
	}

	response := map[string]interface{}{
		"users":       users,
		"total_count": totalCount,
		"offset":      offset,
		"limit":       limit,
	}

	return c.JSON(http.StatusOK, response)
}

// CreateUserInAnyOrg handles creating a user in any organization (super admin only)
func (uh *UserHandlers) CreateUserInAnyOrg(c echo.Context) error {
	// Get target org ID from URL
	orgIDStr := c.Param("org_id")
	orgID, err := strconv.ParseInt(orgIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid organization ID")
	}

	// Get super admin user from context (RequireSuperAdmin middleware ensures this exists)
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Parse request
	var req CreateUserRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Create user in target organization
	createdUser, err := uh.userService.CreateUserInAnyOrg(orgID, user.ID, req)
	if err != nil {
		if err.Error() == "user with email "+req.Email+" already exists" {
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		}
		if err.Error() == "organization with ID "+orgIDStr+" does not exist" {
			return echo.NewHTTPError(http.StatusNotFound, "Organization not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create user")
	}

	return c.JSON(http.StatusCreated, createdUser)
}

// TransferUserToOrg handles transferring a user to a different organization (super admin only)
func (uh *UserHandlers) TransferUserToOrg(c echo.Context) error {
	// Get user ID from URL
	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid user ID")
	}

	// Get super admin user from context
	user := auth.GetUser(c)
	if user == nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "User not found in context")
	}

	// Parse request
	var req struct {
		NewOrgID  int64 `json:"new_org_id" validate:"required"`
		NewRoleID int64 `json:"new_role_id" validate:"required"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}

	// Transfer user
	transferredUser, err := uh.userService.TransferUserToOrg(userID, req.NewOrgID, user.ID, req.NewRoleID)
	if err != nil {
		if err.Error() == "user not found" {
			return echo.NewHTTPError(http.StatusNotFound, "User not found")
		}
		if err.Error() == "target organization does not exist" {
			return echo.NewHTTPError(http.StatusNotFound, "Target organization not found")
		}
		if err.Error() == "target role does not exist" {
			return echo.NewHTTPError(http.StatusNotFound, "Target role not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to transfer user")
	}

	return c.JSON(http.StatusOK, transferredUser)
}

// GetUserAnalytics handles getting user analytics (super admin only)
func (uh *UserHandlers) GetUserAnalytics(c echo.Context) error {
	// Get analytics
	analytics, err := uh.userService.GetUserAnalytics()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get user analytics")
	}

	return c.JSON(http.StatusOK, analytics)
}
