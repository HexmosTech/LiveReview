package auth

import (
	"errors"
)

// Common auth errors
var (
	ErrInvalidToken            = errors.New("invalid or expired token")
	ErrInsufficientPermissions = errors.New("insufficient permissions")
	ErrUserNotFound            = errors.New("user not found")
	ErrInvalidCredentials      = errors.New("invalid email or password")
	ErrTokenExpired            = errors.New("token has expired")
	ErrRefreshTokenInvalid     = errors.New("refresh token is invalid or expired")
	ErrUserNotInOrganization   = errors.New("user is not a member of this organization")
	ErrOrganizationNotFound    = errors.New("organization not found")
)

// Role constants
const (
	RoleSuperAdmin = "super_admin"
	RoleOwner      = "owner"
	RoleMember     = "member"
)

// IsValidRole checks if a role name is valid
func IsValidRole(role string) bool {
	switch role {
	case RoleSuperAdmin, RoleOwner, RoleMember:
		return true
	default:
		return false
	}
}

// GetRoleHierarchy returns the role hierarchy level (higher number = more permissions)
func GetRoleHierarchy(role string) int {
	switch role {
	case RoleSuperAdmin:
		return 3
	case RoleOwner:
		return 2
	case RoleMember:
		return 1
	default:
		return 0
	}
}

// CanRoleManageRole checks if one role can manage another role
func CanRoleManageRole(managerRole, targetRole string) bool {
	return GetRoleHierarchy(managerRole) > GetRoleHierarchy(targetRole)
}
