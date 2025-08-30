package api

import (
	"database/sql"
	"fmt"

	"github.com/livereview/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

// UserService handles user and organization management
type UserService struct {
	db *sql.DB
}

// NewUserService creates a new UserService
func NewUserService(db *sql.DB) *UserService {
	return &UserService{db: db}
}

// CreateFirstAdminUser creates the default org, roles, and super admin user
// This replaces the SetAdminPassword functionality
func (s *UserService) CreateFirstAdminUser(email, password string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Check if any users already exist
	var userCount int
	err = tx.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("failed to check existing users: %v", err)
	}
	if userCount > 0 {
		return fmt.Errorf("admin user already exists")
	}

	// 1. Create default org
	var orgID int64
	err = tx.QueryRow(`
		INSERT INTO orgs (id, name, description) 
		VALUES (1, 'Default Organization', 'Default organization for self-hosted deployment')
		RETURNING id
	`).Scan(&orgID)
	if err != nil {
		return fmt.Errorf("failed to create default org: %v", err)
	}

	// 2. Create default roles
	roles := []string{models.RoleSuperAdmin, models.RoleOwner, models.RoleMember}
	roleIDs := make(map[string]int64)

	for _, roleName := range roles {
		var roleID int64
		err = tx.QueryRow(`
			INSERT INTO roles (name) VALUES ($1) RETURNING id
		`, roleName).Scan(&roleID)
		if err != nil {
			return fmt.Errorf("failed to create role %s: %v", roleName, err)
		}
		roleIDs[roleName] = roleID
	}

	// 3. Create super admin user
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %v", err)
	}

	var userID int64
	err = tx.QueryRow(`
		INSERT INTO users (email, password_hash) 
		VALUES ($1, $2) 
		RETURNING id
	`, email, string(hashedPassword)).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	// 4. Assign super_admin role to user in default org
	_, err = tx.Exec(`
		INSERT INTO user_roles (user_id, role_id, org_id) 
		VALUES ($1, $2, $3)
	`, userID, roleIDs[models.RoleSuperAdmin], orgID)
	if err != nil {
		return fmt.Errorf("failed to assign super admin role: %v", err)
	}

	// 5. Reset sequence to ensure new orgs get proper IDs
	_, err = tx.Exec("SELECT setval('orgs_id_seq', 1, true)")
	if err != nil {
		return fmt.Errorf("failed to reset org sequence: %v", err)
	}

	return tx.Commit()
}

// MigrateExistingAdminPassword converts existing admin password to super admin user
func (s *UserService) MigrateExistingAdminPassword() error {
	// Check if migration is needed
	var userCount int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("failed to check existing users: %v", err)
	}
	if userCount > 0 {
		return nil // Already migrated
	}

	// Get existing admin password
	var adminPassword string
	err = s.db.QueryRow("SELECT admin_password FROM instance_details WHERE admin_password IS NOT NULL LIMIT 1").Scan(&adminPassword)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil // No existing password to migrate
		}
		return fmt.Errorf("failed to get existing admin password: %v", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	// Create default org if it doesn't exist
	_, err = tx.Exec(`
		INSERT INTO orgs (id, name, description) 
		VALUES (1, 'Default Organization', 'Default organization for self-hosted deployment')
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to create default org: %v", err)
	}

	// Create default roles if they don't exist
	roles := []string{models.RoleSuperAdmin, models.RoleOwner, models.RoleMember}
	roleIDs := make(map[string]int64)

	for _, roleName := range roles {
		var roleID int64
		err = tx.QueryRow(`
			INSERT INTO roles (name) VALUES ($1) 
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id
		`, roleName).Scan(&roleID)
		if err != nil {
			return fmt.Errorf("failed to create role %s: %v", roleName, err)
		}
		roleIDs[roleName] = roleID
	}

	// Create super admin user with existing password
	var userID int64
	err = tx.QueryRow(`
		INSERT INTO users (email, password_hash) 
		VALUES ('admin@localhost', $1) 
		RETURNING id
	`, adminPassword).Scan(&userID)
	if err != nil {
		return fmt.Errorf("failed to create admin user: %v", err)
	}

	// Assign super_admin role
	_, err = tx.Exec(`
		INSERT INTO user_roles (user_id, role_id, org_id) 
		VALUES ($1, $2, 1)
	`, userID, roleIDs[models.RoleSuperAdmin])
	if err != nil {
		return fmt.Errorf("failed to assign super admin role: %v", err)
	}

	return tx.Commit()
}

// CheckSetupStatus checks if any admin user exists (replaces CheckAdminPasswordStatus)
func (s *UserService) CheckSetupStatus() (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) 
		FROM users u 
		JOIN user_roles ur ON u.id = ur.user_id 
		JOIN roles r ON ur.role_id = r.id 
		WHERE r.name = $1
	`, models.RoleSuperAdmin).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check setup status: %v", err)
	}
	return count > 0, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(`
		SELECT id, email, password_hash, created_at, updated_at 
		FROM users 
		WHERE email = $1
	`, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserOrgs returns all orgs a user belongs to with their roles
func (s *UserService) GetUserOrgs(userID int64) ([]*models.UserOrgInfo, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.email, u.created_at, u.updated_at,
		       r.id, r.name, r.created_at, r.updated_at,
		       o.id, o.name, o.description, o.created_at, o.updated_at,
		       ur.created_at
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		JOIN roles r ON ur.role_id = r.id
		JOIN orgs o ON ur.org_id = o.id
		WHERE u.id = $1
		ORDER BY o.name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user orgs: %v", err)
	}
	defer rows.Close()

	var result []*models.UserOrgInfo
	for rows.Next() {
		info := &models.UserOrgInfo{
			User: &models.User{},
			Role: &models.Role{},
			Org:  &models.Org{},
		}

		err := rows.Scan(
			&info.User.ID, &info.User.Email, &info.User.CreatedAt, &info.User.UpdatedAt,
			&info.Role.ID, &info.Role.Name, &info.Role.CreatedAt, &info.Role.UpdatedAt,
			&info.Org.ID, &info.Org.Name, &info.Org.Description, &info.Org.CreatedAt, &info.Org.UpdatedAt,
			&info.JoinedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user org info: %v", err)
		}
		result = append(result, info)
	}

	return result, nil
}

// GetUserRoleInOrg returns the user's role in a specific org
func (s *UserService) GetUserRoleInOrg(userID, orgID int64) (*models.Role, error) {
	role := &models.Role{}
	err := s.db.QueryRow(`
		SELECT r.id, r.name, r.created_at, r.updated_at
		FROM roles r
		JOIN user_roles ur ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND ur.org_id = $2
	`, userID, orgID).Scan(&role.ID, &role.Name, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return role, nil
}

// HasRole checks if a user has a specific role in an org
func (s *UserService) HasRole(userID, orgID int64, roleName string) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND ur.org_id = $2 AND r.name = $3
	`, userID, orgID, roleName).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// IsSuperAdmin checks if a user is a super admin (can access any org)
func (s *UserService) IsSuperAdmin(userID int64) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.name = $2
	`, userID, models.RoleSuperAdmin).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// VerifyPassword verifies a user's password
func (s *UserService) VerifyPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil
}
