package users

import (
	"database/sql"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ProfileService handles user profile management operations
type ProfileService struct {
	db *sql.DB
}

// NewProfileService creates a new profile service
func NewProfileService(db *sql.DB) *ProfileService {
	return &ProfileService{
		db: db,
	}
}

// UserProfile represents user profile information
type UserProfile struct {
	ID               int64      `json:"id"`
	Email            string     `json:"email"`
	FirstName        *string    `json:"first_name"`
	LastName         *string    `json:"last_name"`
	IsActive         bool       `json:"is_active"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LastLoginAt      *time.Time `json:"last_login_at"`
	PlanType         *string    `json:"plan_type"`
	LicenseExpiresAt *time.Time `json:"license_expires_at"`
}

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
	FirstName *string `json:"first_name"`
	LastName  *string `json:"last_name"`
	Email     *string `json:"email"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" validate:"required"`
	NewPassword     string `json:"new_password" validate:"required,min=8"`
}

// GetUserProfile retrieves the user's profile information
func (ps *ProfileService) GetUserProfile(userID int64) (*UserProfile, error) {
	profile := &UserProfile{}

	err := ps.db.QueryRow(`
		SELECT 
			u.id, u.email, u.first_name, u.last_name, u.is_active, 
			u.created_at, u.updated_at, u.last_login_at,
			ur.plan_type, ur.license_expires_at
		FROM users u
		LEFT JOIN user_roles ur ON u.id = ur.user_id
		WHERE u.id = $1
	`, userID).Scan(
		&profile.ID, &profile.Email, &profile.FirstName, &profile.LastName,
		&profile.IsActive, &profile.CreatedAt, &profile.UpdatedAt, &profile.LastLoginAt,
		&profile.PlanType, &profile.LicenseExpiresAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user profile: %w", err)
	}

	return profile, nil
}

// UpdateUserProfile updates the user's profile information
func (ps *ProfileService) UpdateUserProfile(userID int64, req UpdateProfileRequest) (*UserProfile, error) {
	// Start transaction
	tx, err := ps.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Build dynamic update query
	setParts := []string{"updated_at = NOW()"}
	args := []interface{}{}
	argIndex := 1

	if req.FirstName != nil {
		setParts = append(setParts, fmt.Sprintf("first_name = $%d", argIndex))
		args = append(args, *req.FirstName)
		argIndex++
	}

	if req.LastName != nil {
		setParts = append(setParts, fmt.Sprintf("last_name = $%d", argIndex))
		args = append(args, *req.LastName)
		argIndex++
	}

	if req.Email != nil {
		// Check if email already exists for another user
		var existingUserID int64
		err = tx.QueryRow("SELECT id FROM users WHERE email = $1 AND id != $2", *req.Email, userID).Scan(&existingUserID)
		if err == nil {
			return nil, fmt.Errorf("email %s is already in use by another user", *req.Email)
		} else if err != sql.ErrNoRows {
			return nil, fmt.Errorf("failed to check existing email: %w", err)
		}

		setParts = append(setParts, fmt.Sprintf("email = $%d", argIndex))
		args = append(args, *req.Email)
		argIndex++
	}

	// Update user if there are changes
	if len(setParts) > 1 { // More than just updated_at
		query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
			fmt.Sprintf("%s", fmt.Sprintf("%s", fmt.Sprintf("%s", setParts[0]))),
			argIndex)

		// Properly join setParts
		var setClause string
		for i, part := range setParts {
			if i == 0 {
				setClause = part
			} else {
				setClause += ", " + part
			}
		}

		query = fmt.Sprintf("UPDATE users SET %s WHERE id = $%d", setClause, argIndex)
		args = append(args, userID)

		_, err = tx.Exec(query, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to update user profile: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Get updated profile
	return ps.GetUserProfile(userID)
}

// ChangePassword changes the user's password
func (ps *ProfileService) ChangePassword(userID int64, req ChangePasswordRequest) error {
	// Get current password hash
	var currentHash string
	err := ps.db.QueryRow("SELECT password_hash FROM users WHERE id = $1", userID).Scan(&currentHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("failed to get current password: %w", err)
	}

	// Verify current password
	err = bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword))
	if err != nil {
		return fmt.Errorf("current password is incorrect")
	}

	// Hash new password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// Update password and clear password reset flag
	_, err = ps.db.Exec(`
		UPDATE users 
		SET password_hash = $1, password_reset_required = false, updated_at = NOW()
		WHERE id = $2
	`, string(hashedPassword), userID)

	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

// GetUserOrganizations gets all organizations the user belongs to with their roles
func (ps *ProfileService) GetUserOrganizations(userID int64) ([]UserOrgRole, error) {
	rows, err := ps.db.Query(`
		SELECT o.id, o.name, r.name as role, ur.role_id
		FROM orgs o
		JOIN user_roles ur ON o.id = ur.org_id
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY o.name
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
	}
	defer rows.Close()

	var organizations []UserOrgRole
	for rows.Next() {
		var org UserOrgRole
		err := rows.Scan(&org.OrgID, &org.OrgName, &org.Role, &org.RoleID)
		if err != nil {
			return nil, fmt.Errorf("failed to scan organization: %w", err)
		}
		organizations = append(organizations, org)
	}

	return organizations, nil
}

// UserOrgRole represents a user's role in an organization
type UserOrgRole struct {
	OrgID   int64  `json:"org_id"`
	OrgName string `json:"org_name"`
	Role    string `json:"role"`
	RoleID  int64  `json:"role_id"`
}
