package auth

import (
	"github.com/livereview/pkg/models"
)

// PermissionContext holds the permission context for a request
// This is built by middleware and passed to handlers
type PermissionContext struct {
	User         *models.User `json:"user"`
	CurrentOrg   *models.Org  `json:"current_org,omitempty"`
	Role         string       `json:"role,omitempty"`
	IsSuperAdmin bool         `json:"is_super_admin"`
	IsOwner      bool         `json:"is_owner"`
	IsMember     bool         `json:"is_member"`
	OrgID        int64        `json:"org_id,omitempty"`
}

// Permission represents a specific permission
type Permission string

const (
	// User management permissions
	PermissionViewUsers   Permission = "view_users"
	PermissionCreateUsers Permission = "create_users"
	PermissionEditUsers   Permission = "edit_users"
	PermissionDeleteUsers Permission = "delete_users"
	PermissionManageRoles Permission = "manage_roles"

	// Organization permissions
	PermissionViewOrg   Permission = "view_org"
	PermissionEditOrg   Permission = "edit_org"
	PermissionDeleteOrg Permission = "delete_org"

	// Review permissions
	PermissionViewReviews   Permission = "view_reviews"
	PermissionCreateReviews Permission = "create_reviews"
	PermissionEditReviews   Permission = "edit_reviews"
	PermissionDeleteReviews Permission = "delete_reviews"

	// Global admin permissions
	PermissionSuperAdmin Permission = "super_admin"
)

// GetOrgID returns the current organization ID
func (pc *PermissionContext) GetOrgID() int64 {
	return pc.OrgID
}

// GetUserID returns the current user ID
func (pc *PermissionContext) GetUserID() int64 {
	if pc.User == nil {
		return 0
	}
	return pc.User.ID
}

// HasPermission checks if the user has a specific permission
func (pc *PermissionContext) HasPermission(permission Permission) bool {
	// Super admin has all permissions
	if pc.IsSuperAdmin {
		return true
	}

	switch permission {
	case PermissionSuperAdmin:
		return pc.IsSuperAdmin

	// User management permissions
	case PermissionViewUsers:
		return pc.IsOwner || pc.IsMember || pc.IsSuperAdmin
	case PermissionCreateUsers, PermissionEditUsers, PermissionDeleteUsers, PermissionManageRoles:
		return pc.IsOwner || pc.IsSuperAdmin

	// Organization permissions
	case PermissionViewOrg:
		return pc.IsOwner || pc.IsMember || pc.IsSuperAdmin
	case PermissionEditOrg:
		return pc.IsOwner || pc.IsSuperAdmin
	case PermissionDeleteOrg:
		return pc.IsSuperAdmin

	// Review permissions - all org members can manage reviews
	case PermissionViewReviews, PermissionCreateReviews, PermissionEditReviews, PermissionDeleteReviews:
		return pc.IsOwner || pc.IsMember || pc.IsSuperAdmin

	default:
		return false
	}
}

// CanManageUsersInOrg checks if user can manage users in the current org
func (pc *PermissionContext) CanManageUsersInOrg() bool {
	return pc.HasPermission(PermissionCreateUsers)
}

// CanViewUsersInOrg checks if user can view users in the current org
func (pc *PermissionContext) CanViewUsersInOrg() bool {
	return pc.HasPermission(PermissionViewUsers)
}

// RequirePermission checks if user has permission and returns error if not
func (pc *PermissionContext) RequirePermission(permission Permission) error {
	if !pc.HasPermission(permission) {
		return ErrInsufficientPermissions
	}
	return nil
}

// RequireSuperAdmin checks if user is super admin
func (pc *PermissionContext) RequireSuperAdmin() error {
	if !pc.IsSuperAdmin {
		return ErrInsufficientPermissions
	}
	return nil
}

// RequireOrgOwner checks if user is org owner or super admin
func (pc *PermissionContext) RequireOrgOwner() error {
	if !pc.IsOwner && !pc.IsSuperAdmin {
		return ErrInsufficientPermissions
	}
	return nil
}
