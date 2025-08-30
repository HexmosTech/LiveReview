package models

import (
	"time"
)

// Multi-tenancy models

// Org represents an organization (top-level tenant)
type Org struct {
	ID               int64     `json:"id" db:"id"`
	Name             string    `json:"name" db:"name"`
	Description      *string   `json:"description" db:"description"`
	IsActive         bool      `json:"is_active" db:"is_active"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
	CreatedByUserID  *int64    `json:"created_by_user_id,omitempty" db:"created_by_user_id"`
	Settings         string    `json:"settings,omitempty" db:"settings"`
	SubscriptionPlan *string   `json:"subscription_plan,omitempty" db:"subscription_plan"`
	MaxUsers         *int      `json:"max_users,omitempty" db:"max_users"`
}

// OrgWithRole extends Org with the user's role in that organization
type OrgWithRole struct {
	Org
	RoleName string `json:"role_name"`
}

// OrgAnalytics contains analytics data for an organization
type OrgAnalytics struct {
	OrgID          int64            `json:"org_id"`
	TotalMembers   int64            `json:"total_members"`
	MembersByRole  map[string]int64 `json:"members_by_role"`
	RecentActivity int64            `json:"recent_activity"`
}

// User represents a user who can belong to multiple orgs
type User struct {
	ID                    int64      `json:"id" db:"id"`
	Email                 string     `json:"email" db:"email"`
	PasswordHash          string     `json:"-" db:"password_hash"` // Never expose password hash in JSON
	FirstName             *string    `json:"first_name,omitempty" db:"first_name"`
	LastName              *string    `json:"last_name,omitempty" db:"last_name"`
	IsActive              bool       `json:"is_active" db:"is_active"`
	LastLoginAt           *time.Time `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
	CreatedByUserID       *int64     `json:"created_by_user_id,omitempty" db:"created_by_user_id"`
	PasswordResetRequired bool       `json:"password_reset_required" db:"password_reset_required"`
}

// UserWithRole extends User with role information for a specific organization
type UserWithRole struct {
	User
	Role   string `json:"role"`
	RoleID int64  `json:"role_id"`
	OrgID  int64  `json:"org_id"`
}

// UserProfile represents user profile information for self-service updates
type UserProfile struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
}

// Role represents a role definition (super_admin, owner, member)
type Role struct {
	ID        int64     `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// UserRole represents the junction table: users ↔ roles within orgs
type UserRole struct {
	UserID    int64     `json:"user_id" db:"user_id"`
	RoleID    int64     `json:"role_id" db:"role_id"`
	OrgID     int64     `json:"org_id" db:"org_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// UserOrgInfo combines user info with their role in a specific org
type UserOrgInfo struct {
	User     *User     `json:"user"`
	Role     *Role     `json:"role"`
	Org      *Org      `json:"org"`
	JoinedAt time.Time `json:"joined_at"`
}

// Role constants
const (
	RoleSuperAdmin = "super_admin"
	RoleOwner      = "owner"
	RoleMember     = "member"
)

// InstanceDetails represents system-wide instance configuration
type InstanceDetails struct {
	ID            int
	DomainName    string
	AdminPassword string
	CreatedAt     string
	UpdatedAt     string
}

// CodeDiff represents a code diff from a merge/pull request
type CodeDiff struct {
	FilePath    string
	OldContent  string
	NewContent  string
	Hunks       []DiffHunk
	CommitID    string
	FileType    string
	IsDeleted   bool
	IsNew       bool
	IsRenamed   bool
	OldFilePath string // Only set if IsRenamed is true
}

// DiffHunk represents a single chunk of changes in a diff
type DiffHunk struct {
	OldStartLine int
	OldLineCount int
	NewStartLine int
	NewLineCount int
	Content      string
}

// ReviewResult contains the overall review result including summary and specific comments
type ReviewResult struct {
	Summary          string           // High-level summary of what the diff is about
	Comments         []*ReviewComment // External comments to be posted to the platform
	InternalComments []*ReviewComment // Internal comments used for synthesis only
}

// ReviewComment represents a single comment from the AI review
type ReviewComment struct {
	FilePath      string
	Line          int
	Content       string
	Severity      CommentSeverity
	Confidence    float64
	Category      string
	Suggestions   []string
	IsDeletedLine bool // True if comment is on a deleted line (old_line) rather than new_line
	IsInternal    bool // True if comment is for internal synthesis only, false if it should be posted to user
}

// CommentSeverity represents the severity level of a review comment
type CommentSeverity string

const (
	SeverityInfo     CommentSeverity = "info"
	SeverityWarning  CommentSeverity = "warning"
	SeverityCritical CommentSeverity = "critical"
)
