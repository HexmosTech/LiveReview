package users

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// UserService handles core user management operations
type UserService struct {
	db *sql.DB
}

// NewUserService creates a new user service
func NewUserService(db *sql.DB) *UserService {
	return &UserService{
		db: db,
	}
}

// UserWithRole represents a user with their role in a specific organization
type UserWithRole struct {
	ID                    int64      `json:"id"`
	Email                 string     `json:"email"`
	FirstName             *string    `json:"first_name"`
	LastName              *string    `json:"last_name"`
	IsActive              bool       `json:"is_active"`
	LastLoginAt           *time.Time `json:"last_login_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	CreatedByUserID       *int64     `json:"created_by_user_id"`
	DeactivatedAt         *time.Time `json:"deactivated_at"`
	DeactivatedByUserID   *int64     `json:"deactivated_by_user_id"`
	PasswordResetRequired bool       `json:"password_reset_required"`
	Role                  string     `json:"role"`
	RoleID                int64      `json:"role_id"`
	OrgID                 int64      `json:"org_id"`
}

// CreateUserRequest represents the request to create a new user
type CreateUserRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"required,min=8"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	RoleID    int64  `json:"role_id" validate:"required"`
}

// UpdateUserRequest represents the request to update a user
type UpdateUserRequest struct {
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	IsActive  *bool   `json:"is_active"`
	RoleID    *int64  `json:"role_id"`
}

// CreateUserInOrg creates a new user in the specified organization
func (us *UserService) CreateUserInOrg(orgID, createdByUserID int64, req CreateUserRequest) (*UserWithRole, error) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Start transaction
	tx, err := us.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if email already exists
	var existingUserID int64
	err = tx.QueryRow("SELECT id FROM users WHERE email = $1", req.Email).Scan(&existingUserID)
	if err != sql.ErrNoRows {
		if err == nil {
			return nil, fmt.Errorf("user with email %s already exists", req.Email)
		}
		return nil, fmt.Errorf("failed to check existing email: %w", err)
	}

	// Create user
	var userID int64
	err = tx.QueryRow(`
		INSERT INTO users (email, password_hash, first_name, last_name, created_by_user_id, password_reset_required, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW())
		RETURNING id
	`, req.Email, string(hashedPassword), req.FirstName, req.LastName, createdByUserID).Scan(&userID)

	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Add user role
	_, err = tx.Exec(`
		INSERT INTO user_roles (user_id, org_id, role_id, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, userID, orgID, req.RoleID)

	if err != nil {
		return nil, fmt.Errorf("failed to assign user role: %w", err)
	}

	// Add audit trail
	err = us.addUserAuditLog(tx, orgID, userID, createdByUserID, "created", map[string]interface{}{
		"role_id": req.RoleID,
		"email":   req.Email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add audit log: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get the created user with role
	return us.GetUserInOrg(orgID, userID)
}

// GetUserInOrg gets a user in a specific organization with their role
func (us *UserService) GetUserInOrg(orgID, userID int64) (*UserWithRole, error) {
	user := &UserWithRole{}
	err := us.db.QueryRow(`
		SELECT u.id, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at,
		       u.created_at, u.updated_at, u.created_by_user_id, u.deactivated_at,
		       u.deactivated_by_user_id, u.password_reset_required,
		       r.name as role, r.id as role_id, ur.org_id
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		JOIN roles r ON ur.role_id = r.id
		WHERE u.id = $1 AND ur.org_id = $2
	`, userID, orgID).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.IsActive,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.CreatedByUserID,
		&user.DeactivatedAt, &user.DeactivatedByUserID, &user.PasswordResetRequired,
		&user.Role, &user.RoleID, &user.OrgID,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found in organization")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// ListUsersInOrg lists all users in an organization with pagination
func (us *UserService) ListUsersInOrg(orgID int64, offset, limit int) ([]*UserWithRole, int, error) {
	// Get total count
	var totalCount int
	err := us.db.QueryRow(`
		SELECT COUNT(*)
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		WHERE ur.org_id = $1
	`, orgID).Scan(&totalCount)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user count: %w", err)
	}

	// Get users with pagination
	rows, err := us.db.Query(`
		SELECT u.id, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at,
		       u.created_at, u.updated_at, u.created_by_user_id, u.deactivated_at,
		       u.deactivated_by_user_id, u.password_reset_required,
		       r.name as role, r.id as role_id, ur.org_id
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.org_id = $1
		ORDER BY u.created_at DESC
		LIMIT $2 OFFSET $3
	`, orgID, limit, offset)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to query users: %w", err)
	}
	defer rows.Close()

	var users []*UserWithRole
	for rows.Next() {
		user := &UserWithRole{}
		err := rows.Scan(
			&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.IsActive,
			&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.CreatedByUserID,
			&user.DeactivatedAt, &user.DeactivatedByUserID, &user.PasswordResetRequired,
			&user.Role, &user.RoleID, &user.OrgID,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, totalCount, nil
}

// UpdateUserInOrg updates a user in a specific organization
func (us *UserService) UpdateUserInOrg(orgID, userID, updatedByUserID int64, req UpdateUserRequest) (*UserWithRole, error) {
	// Start transaction
	tx, err := us.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Build dynamic update query
	setParts := []string{"updated_at = NOW()"}
	args := []interface{}{}
	argIndex := 1

	auditDetails := make(map[string]interface{})

	if req.FirstName != nil {
		setParts = append(setParts, fmt.Sprintf("first_name = $%d", argIndex))
		args = append(args, *req.FirstName)
		auditDetails["first_name"] = *req.FirstName
		argIndex++
	}

	if req.LastName != nil {
		setParts = append(setParts, fmt.Sprintf("last_name = $%d", argIndex))
		args = append(args, *req.LastName)
		auditDetails["last_name"] = *req.LastName
		argIndex++
	}

	if req.IsActive != nil {
		setParts = append(setParts, fmt.Sprintf("is_active = $%d", argIndex))
		args = append(args, *req.IsActive)
		auditDetails["is_active"] = *req.IsActive
		argIndex++

		if !*req.IsActive {
			setParts = append(setParts, fmt.Sprintf("deactivated_at = NOW(), deactivated_by_user_id = $%d", argIndex))
			args = append(args, updatedByUserID)
			argIndex++
		}
	}

	// Update user if there are changes
	if len(setParts) > 1 { // More than just updated_at
		query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
			strings.Join(setParts, ", "), argIndex)
		args = append(args, userID)

		_, err = tx.Exec(query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}

		// Add audit log for user update
		err = us.addUserAuditLog(tx, orgID, userID, updatedByUserID, "updated", auditDetails)
		if err != nil {
			return nil, fmt.Errorf("failed to add audit log: %w", err)
		}
	}

	// Handle role change separately
	if req.RoleID != nil {
		// Get current role for audit trail
		var currentRoleID int64
		err = tx.QueryRow(`
			SELECT role_id FROM user_roles WHERE user_id = $1 AND org_id = $2
		`, userID, orgID).Scan(&currentRoleID)

		if err != nil {
			return nil, fmt.Errorf("failed to get current role: %w", err)
		}

		if currentRoleID != *req.RoleID {
			// Update role
			_, err = tx.Exec(`
				UPDATE user_roles SET role_id = $1, updated_at = NOW()
				WHERE user_id = $2 AND org_id = $3
			`, *req.RoleID, userID, orgID)

			if err != nil {
				return nil, fmt.Errorf("failed to update user role: %w", err)
			}

			// Add role change to audit history
			_, err = tx.Exec(`
				INSERT INTO user_role_history (user_id, org_id, old_role_id, new_role_id, changed_by_user_id, created_at)
				VALUES ($1, $2, $3, $4, $5, NOW())
			`, userID, orgID, currentRoleID, *req.RoleID, updatedByUserID)

			if err != nil {
				return nil, fmt.Errorf("failed to add role history: %w", err)
			}

			// Add audit log for role change
			err = us.addUserAuditLog(tx, orgID, userID, updatedByUserID, "role_changed", map[string]interface{}{
				"old_role_id": currentRoleID,
				"new_role_id": *req.RoleID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to add role change audit log: %w", err)
			}
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get updated user
	return us.GetUserInOrg(orgID, userID)
}

// DeactivateUserInOrg deactivates a user in a specific organization
func (us *UserService) DeactivateUserInOrg(orgID, userID, deactivatedByUserID int64) error {
	// Start transaction
	tx, err := us.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Deactivate user
	_, err = tx.Exec(`
		UPDATE users 
		SET is_active = false, deactivated_at = NOW(), deactivated_by_user_id = $1, updated_at = NOW()
		WHERE id = $2
	`, deactivatedByUserID, userID)

	if err != nil {
		return fmt.Errorf("failed to deactivate user: %w", err)
	}

	// Add audit log
	err = us.addUserAuditLog(tx, orgID, userID, deactivatedByUserID, "deactivated", map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("failed to add audit log: %w", err)
	}

	// Commit transaction
	return tx.Commit()
}

// ForcePasswordReset forces a user to reset their password on next login
func (us *UserService) ForcePasswordReset(orgID, userID, updatedByUserID int64) error {
	// Start transaction
	tx, err := us.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Set password reset required
	_, err = tx.Exec(`
		UPDATE users 
		SET password_reset_required = true, updated_at = NOW()
		WHERE id = $1
	`, userID)

	if err != nil {
		return fmt.Errorf("failed to set password reset required: %w", err)
	}

	// Add audit log
	err = us.addUserAuditLog(tx, orgID, userID, updatedByUserID, "password_reset", map[string]interface{}{})
	if err != nil {
		return fmt.Errorf("failed to add audit log: %w", err)
	}

	// Commit transaction
	return tx.Commit()
}

// GetUserAuditLog gets the audit log for a user
func (us *UserService) GetUserAuditLog(orgID, userID int64, offset, limit int) ([]UserAuditEntry, error) {
	rows, err := us.db.Query(`
		SELECT uma.id, uma.action, uma.details, uma.created_at,
		       performer.email as performed_by_email
		FROM user_management_audit uma
		JOIN users performer ON uma.performed_by_user_id = performer.id
		WHERE uma.org_id = $1 AND uma.target_user_id = $2
		ORDER BY uma.created_at DESC
		LIMIT $3 OFFSET $4
	`, orgID, userID, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("failed to query audit log: %w", err)
	}
	defer rows.Close()

	var entries []UserAuditEntry
	for rows.Next() {
		entry := UserAuditEntry{}
		err := rows.Scan(&entry.ID, &entry.Action, &entry.Details, &entry.CreatedAt, &entry.PerformedByEmail)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// UserAuditEntry represents an audit log entry
type UserAuditEntry struct {
	ID               int64                  `json:"id"`
	Action           string                 `json:"action"`
	Details          map[string]interface{} `json:"details"`
	CreatedAt        time.Time              `json:"created_at"`
	PerformedByEmail string                 `json:"performed_by_email"`
}

// addUserAuditLog adds an entry to the user management audit log
func (us *UserService) addUserAuditLog(tx *sql.Tx, orgID, targetUserID, performedByUserID int64, action string, details map[string]interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("failed to marshal audit details: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO user_management_audit (org_id, target_user_id, performed_by_user_id, action, details, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`, orgID, targetUserID, performedByUserID, action, string(detailsJSON))

	return err
}

// ============================================================================
// SUPER ADMIN GLOBAL MANAGEMENT METHODS
// ============================================================================

// GlobalUserWithOrg represents a user with their organization info (for super admin views)
type GlobalUserWithOrg struct {
	UserWithRole
	OrgName string `json:"org_name"`
}

// ListAllUsers lists all users across all organizations (super admin only)
func (us *UserService) ListAllUsers(offset, limit int) ([]*GlobalUserWithOrg, int, error) {
	// Get total count
	var totalCount int
	err := us.db.QueryRow(`
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
	`).Scan(&totalCount)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total user count: %w", err)
	}

	// Get users with pagination
	rows, err := us.db.Query(`
		SELECT u.id, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at,
		       u.created_at, u.updated_at, u.created_by_user_id, u.deactivated_at,
		       u.deactivated_by_user_id, u.password_reset_required,
		       r.name as role, r.id as role_id, ur.org_id, o.name as org_name
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		JOIN roles r ON ur.role_id = r.id
		JOIN orgs o ON ur.org_id = o.id
		ORDER BY u.created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to query all users: %w", err)
	}
	defer rows.Close()

	var users []*GlobalUserWithOrg
	for rows.Next() {
		user := &GlobalUserWithOrg{}
		err := rows.Scan(
			&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.IsActive,
			&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.CreatedByUserID,
			&user.DeactivatedAt, &user.DeactivatedByUserID, &user.PasswordResetRequired,
			&user.Role, &user.RoleID, &user.OrgID, &user.OrgName,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, user)
	}

	return users, totalCount, nil
}

// CreateUserInAnyOrg creates a user in any organization (super admin only)
func (us *UserService) CreateUserInAnyOrg(targetOrgID, createdByUserID int64, req CreateUserRequest) (*UserWithRole, error) {
	// Verify target organization exists
	var orgExists bool
	err := us.db.QueryRow("SELECT EXISTS(SELECT 1 FROM orgs WHERE id = $1)", targetOrgID).Scan(&orgExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check organization existence: %w", err)
	}
	if !orgExists {
		return nil, fmt.Errorf("organization with ID %d does not exist", targetOrgID)
	}

	// Use existing CreateUserInOrg method
	return us.CreateUserInOrg(targetOrgID, createdByUserID, req)
}

// TransferUserToOrg transfers a user from one organization to another (super admin only)
func (us *UserService) TransferUserToOrg(userID, newOrgID, performedByUserID int64, newRoleID int64) (*UserWithRole, error) {
	// Start transaction
	tx, err := us.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Verify user exists
	var currentOrgID int64
	var currentRoleID int64
	err = tx.QueryRow(`
		SELECT ur.org_id, ur.role_id 
		FROM user_roles ur 
		WHERE ur.user_id = $1
	`, userID).Scan(&currentOrgID, &currentRoleID)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get current user org: %w", err)
	}

	// Verify target organization exists
	var orgExists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM orgs WHERE id = $1)", newOrgID).Scan(&orgExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check target organization: %w", err)
	}
	if !orgExists {
		return nil, fmt.Errorf("target organization does not exist")
	}

	// Verify target role exists
	var roleExists bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1)", newRoleID).Scan(&roleExists)
	if err != nil {
		return nil, fmt.Errorf("failed to check target role: %w", err)
	}
	if !roleExists {
		return nil, fmt.Errorf("target role does not exist")
	}

	// Update user's organization and role
	_, err = tx.Exec(`
		UPDATE user_roles 
		SET org_id = $1, role_id = $2, updated_at = NOW()
		WHERE user_id = $3
	`, newOrgID, newRoleID, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to transfer user: %w", err)
	}

	// Add audit log for the transfer
	err = us.addUserAuditLog(tx, newOrgID, userID, performedByUserID, "transferred", map[string]interface{}{
		"old_org_id":  currentOrgID,
		"new_org_id":  newOrgID,
		"old_role_id": currentRoleID,
		"new_role_id": newRoleID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add transfer audit log: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get updated user
	return us.GetUserInOrg(newOrgID, userID)
}

// GetUserAnalytics provides basic analytics about users (super admin only)
func (us *UserService) GetUserAnalytics() (*UserAnalytics, error) {
	analytics := &UserAnalytics{}

	// Total users count
	err := us.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&analytics.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get total users: %w", err)
	}

	// Active users count
	err = us.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = true").Scan(&analytics.ActiveUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users: %w", err)
	}

	// Users requiring password reset
	err = us.db.QueryRow("SELECT COUNT(*) FROM users WHERE password_reset_required = true").Scan(&analytics.UsersNeedingPasswordReset)
	if err != nil {
		return nil, fmt.Errorf("failed to get users needing password reset: %w", err)
	}

	// Users by organization
	rows, err := us.db.Query(`
		SELECT o.name, COUNT(ur.user_id) 
		FROM orgs o
		LEFT JOIN user_roles ur ON o.id = ur.org_id
		GROUP BY o.id, o.name
		ORDER BY COUNT(ur.user_id) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by org: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var orgStat OrgUserCount
		err := rows.Scan(&orgStat.OrgName, &orgStat.UserCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan org user count: %w", err)
		}
		analytics.UsersByOrg = append(analytics.UsersByOrg, orgStat)
	}

	// Users by role
	rows, err = us.db.Query(`
		SELECT r.name, COUNT(ur.user_id) 
		FROM roles r
		LEFT JOIN user_roles ur ON r.id = ur.role_id
		GROUP BY r.id, r.name
		ORDER BY COUNT(ur.user_id) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to get users by role: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var roleStat RoleUserCount
		err := rows.Scan(&roleStat.RoleName, &roleStat.UserCount)
		if err != nil {
			return nil, fmt.Errorf("failed to scan role user count: %w", err)
		}
		analytics.UsersByRole = append(analytics.UsersByRole, roleStat)
	}

	return analytics, nil
}

// UserAnalytics represents user analytics data for super admins
type UserAnalytics struct {
	TotalUsers                int             `json:"total_users"`
	ActiveUsers               int             `json:"active_users"`
	UsersNeedingPasswordReset int             `json:"users_needing_password_reset"`
	UsersByOrg                []OrgUserCount  `json:"users_by_org"`
	UsersByRole               []RoleUserCount `json:"users_by_role"`
}

// OrgUserCount represents user count per organization
type OrgUserCount struct {
	OrgName   string `json:"org_name"`
	UserCount int    `json:"user_count"`
}

// RoleUserCount represents user count per role
type RoleUserCount struct {
	RoleName  string `json:"role_name"`
	UserCount int    `json:"user_count"`
}
