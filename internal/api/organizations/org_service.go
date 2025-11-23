package organizations

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/livereview/pkg/models"
)

type OrganizationService struct {
	db     *sql.DB
	logger *log.Logger
}

func NewOrganizationService(db *sql.DB, logger *log.Logger) *OrganizationService {
	return &OrganizationService{
		db:     db,
		logger: logger,
	}
}

// CreateOrganization creates a new organization (available to all authenticated users)
func (s *OrganizationService) CreateOrganization(createdByUserID int64, name, description string) (*models.Org, error) {
	// Start transaction to create org and assign creator as owner
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if organization with this name already exists
	var existingCount int
	checkQuery := `SELECT COUNT(*) FROM orgs WHERE LOWER(name) = LOWER($1) AND is_active = true`
	err = tx.QueryRow(checkQuery, name).Scan(&existingCount)
	if err != nil {
		s.logger.Printf("Error checking for existing organization: %v", err)
		return nil, fmt.Errorf("failed to check for existing organization: %w", err)
	}

	if existingCount > 0 {
		return nil, fmt.Errorf("organization with name '%s' already exists", name)
	}

	// Create the organization
	query := `
		INSERT INTO orgs (name, description, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		RETURNING id, name, description, is_active, created_at, updated_at, created_by_user_id
	`

	var org models.Org
	err = tx.QueryRow(query, name, description, createdByUserID).Scan(
		&org.ID,
		&org.Name,
		&org.Description,
		&org.IsActive,
		&org.CreatedAt,
		&org.UpdatedAt,
		&org.CreatedByUserID,
	)

	if err != nil {
		s.logger.Printf("Error creating organization: %v", err)
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Check if the creator is a super admin
	var isSuperAdmin bool
	superAdminCheckQuery := `
		SELECT EXISTS(
			SELECT 1 FROM user_roles ur
			JOIN roles r ON ur.role_id = r.id
			WHERE ur.user_id = $1 AND r.name = 'super_admin'
		)
	`
	err = tx.QueryRow(superAdminCheckQuery, createdByUserID).Scan(&isSuperAdmin)
	if err != nil {
		s.logger.Printf("Error checking super admin status: %v", err)
		return nil, fmt.Errorf("failed to check super admin status: %w", err)
	}

	// Only assign owner role if creator is NOT a super admin
	// Super admins already have global access to all organizations
	if !isSuperAdmin {
		// Get the owner role ID (role name = 'owner')
		var ownerRoleID int64
		roleQuery := `SELECT id FROM roles WHERE name = 'owner' LIMIT 1`
		err = tx.QueryRow(roleQuery).Scan(&ownerRoleID)
		if err != nil {
			s.logger.Printf("Error getting owner role ID: %v", err)
			return nil, fmt.Errorf("failed to get owner role: %w", err)
		}

		// Assign the creator as owner of the new organization
		userRoleQuery := `
			INSERT INTO user_roles (user_id, role_id, org_id, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
		`
		_, err = tx.Exec(userRoleQuery, createdByUserID, ownerRoleID, org.ID)
		if err != nil {
			s.logger.Printf("Error assigning creator as owner: %v", err)
			return nil, fmt.Errorf("failed to assign creator as owner: %w", err)
		}
	} else {
		s.logger.Printf("Skipping owner role assignment for super admin user %d (org %d)", createdByUserID, org.ID)
	}

	// Create default prompt application context for the new organization
	promptContextQuery := `
		INSERT INTO prompt_application_context (org_id, created_at, updated_at)
		VALUES ($1, NOW(), NOW())
	`
	_, err = tx.Exec(promptContextQuery, org.ID)
	if err != nil {
		s.logger.Printf("Error creating prompt application context: %v", err)
		return nil, fmt.Errorf("failed to create prompt application context: %w", err)
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		s.logger.Printf("Error committing transaction: %v", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Printf("Organization created: ID=%d, Name=%s, CreatedBy=%d (assigned as owner)", org.ID, org.Name, createdByUserID)
	return &org, nil
}

// GetUserOrganizations returns all organizations that the user has access to
func (s *OrganizationService) GetUserOrganizations(userID int64, isSuperAdmin bool) ([]*models.OrgWithRole, error) {
	var query string
	var args []interface{}

	if isSuperAdmin {
		// Super admin can see all organizations except Default Organization
		query = `
			SELECT o.id, o.name, o.description, o.is_active, o.created_at, o.updated_at,
			       o.created_by_user_id, o.settings, o.subscription_plan, o.max_users,
			       COALESCE(r.name, 'super_admin') as role_name,
			       creator.email as creator_email,
			       creator.first_name as creator_first_name,
			       creator.last_name as creator_last_name
			FROM orgs o
			LEFT JOIN user_roles ur ON o.id = ur.org_id AND ur.user_id = $1
			LEFT JOIN roles r ON ur.role_id = r.id
			LEFT JOIN users creator ON o.created_by_user_id = creator.id
			WHERE o.is_active = true AND o.name != 'Default Organization'
			ORDER BY o.name ASC
		`
		args = []interface{}{userID}
	} else {
		// Regular users can only see organizations they belong to, excluding Default Organization
		query = `
			SELECT o.id, o.name, o.description, o.is_active, o.created_at, o.updated_at,
			       o.created_by_user_id, o.settings, o.subscription_plan, o.max_users,
			       r.name as role_name,
			       creator.email as creator_email,
			       creator.first_name as creator_first_name,
			       creator.last_name as creator_last_name
			FROM orgs o
			INNER JOIN user_roles ur ON o.id = ur.org_id
			INNER JOIN roles r ON ur.role_id = r.id
			LEFT JOIN users creator ON o.created_by_user_id = creator.id
			WHERE ur.user_id = $1 AND o.is_active = true AND o.name != 'Default Organization'
			ORDER BY o.name ASC
		`
		args = []interface{}{userID}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		s.logger.Printf("Error querying user organizations: %v", err)
		return nil, fmt.Errorf("failed to get user organizations: %w", err)
	}
	defer rows.Close()

	var orgs []*models.OrgWithRole
	for rows.Next() {
		var org models.OrgWithRole
		var settings sql.NullString
		var createdByUserID sql.NullInt64
		var creatorEmail, creatorFirstName, creatorLastName sql.NullString

		err := rows.Scan(
			&org.ID,
			&org.Name,
			&org.Description,
			&org.IsActive,
			&org.CreatedAt,
			&org.UpdatedAt,
			&createdByUserID,
			&settings,
			&org.SubscriptionPlan,
			&org.MaxUsers,
			&org.RoleName,
			&creatorEmail,
			&creatorFirstName,
			&creatorLastName,
		)
		if err != nil {
			s.logger.Printf("Error scanning organization row: %v", err)
			continue
		}

		if createdByUserID.Valid {
			org.CreatedByUserID = &createdByUserID.Int64
		}
		if settings.Valid {
			org.Settings = settings.String
		} else {
			org.Settings = "{}"
		}
		if creatorEmail.Valid {
			org.CreatorEmail = &creatorEmail.String
		}
		if creatorFirstName.Valid {
			org.CreatorFirstName = &creatorFirstName.String
		}
		if creatorLastName.Valid {
			org.CreatorLastName = &creatorLastName.String
		}

		orgs = append(orgs, &org)
	}

	return orgs, nil
}

// GetOrganizationByID returns organization details
func (s *OrganizationService) GetOrganizationByID(orgID int64, userID int64, isSuperAdmin bool) (*models.OrgWithRole, error) {
	var query string
	var args []interface{}

	if isSuperAdmin {
		// Super admin can see any organization
		query = `
			SELECT o.id, o.name, o.description, o.is_active, o.created_at, o.updated_at,
			       o.created_by_user_id, o.settings, o.subscription_plan, o.max_users,
			       COALESCE(r.name, 'super_admin') as role_name
			FROM orgs o
			LEFT JOIN user_roles ur ON o.id = ur.org_id AND ur.user_id = $2
			LEFT JOIN roles r ON ur.role_id = r.id
			WHERE o.id = $1
		`
		args = []interface{}{orgID, userID}
	} else {
		// Regular users can only see organizations they belong to
		query = `
			SELECT o.id, o.name, o.description, o.is_active, o.created_at, o.updated_at,
			       o.created_by_user_id, o.settings, o.subscription_plan, o.max_users,
			       r.name as role_name
			FROM orgs o
			INNER JOIN user_roles ur ON o.id = ur.org_id
			INNER JOIN roles r ON ur.role_id = r.id
			WHERE o.id = $1 AND ur.user_id = $2
		`
		args = []interface{}{orgID, userID}
	}

	var org models.OrgWithRole
	var settings sql.NullString
	var createdByUserID sql.NullInt64

	err := s.db.QueryRow(query, args...).Scan(
		&org.ID,
		&org.Name,
		&org.Description,
		&org.IsActive,
		&org.CreatedAt,
		&org.UpdatedAt,
		&createdByUserID,
		&settings,
		&org.SubscriptionPlan,
		&org.MaxUsers,
		&org.RoleName,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("organization not found or access denied")
		}
		s.logger.Printf("Error getting organization: %v", err)
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if createdByUserID.Valid {
		org.CreatedByUserID = &createdByUserID.Int64
	}
	if settings.Valid {
		org.Settings = settings.String
	} else {
		org.Settings = "{}"
	}

	return &org, nil
}

// UpdateOrganization updates organization details (owners + super admin)
func (s *OrganizationService) UpdateOrganization(orgID int64, updatedByUserID int64, name, description *string, settings *string, subscriptionPlan *string, maxUsers *int) (*models.Org, error) {
	updates := []string{}
	args := []interface{}{}
	argIndex := 1

	if name != nil {
		updates = append(updates, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *name)
		argIndex++
	}

	if description != nil {
		updates = append(updates, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *description)
		argIndex++
	}

	if settings != nil {
		updates = append(updates, fmt.Sprintf("settings = $%d", argIndex))
		args = append(args, *settings)
		argIndex++
	}

	if subscriptionPlan != nil {
		updates = append(updates, fmt.Sprintf("subscription_plan = $%d", argIndex))
		args = append(args, *subscriptionPlan)
		argIndex++
	}

	if maxUsers != nil {
		updates = append(updates, fmt.Sprintf("max_users = $%d", argIndex))
		args = append(args, *maxUsers)
		argIndex++
	}

	if len(updates) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	updates = append(updates, "updated_at = NOW()")

	// Add WHERE clause arguments
	args = append(args, orgID) // $argIndex
	whereClause := fmt.Sprintf("WHERE id = $%d", argIndex)

	query := fmt.Sprintf(`
		UPDATE orgs 
		SET %s 
		%s
		RETURNING id, name, description, is_active, created_at, updated_at, created_by_user_id
	`,
		fmt.Sprintf("%s", fmt.Sprintf("%v", updates)[1:len(fmt.Sprintf("%v", updates))-1]), // Remove [ ]
		whereClause,
	)

	// Fix the query construction
	updateClause := ""
	for i, update := range updates {
		if i > 0 {
			updateClause += ", "
		}
		updateClause += update
	}

	query = fmt.Sprintf(`
		UPDATE orgs 
		SET %s 
		WHERE id = $%d
		RETURNING id, name, description, is_active, created_at, updated_at, created_by_user_id
	`, updateClause, argIndex)

	var org models.Org
	var createdByUserID sql.NullInt64

	err := s.db.QueryRow(query, args...).Scan(
		&org.ID,
		&org.Name,
		&org.Description,
		&org.IsActive,
		&org.CreatedAt,
		&org.UpdatedAt,
		&createdByUserID,
	)

	if err != nil {
		s.logger.Printf("Error updating organization: %v", err)
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}

	if createdByUserID.Valid {
		org.CreatedByUserID = &createdByUserID.Int64
	}

	s.logger.Printf("Organization updated: ID=%d, UpdatedBy=%d", org.ID, updatedByUserID)
	return &org, nil
}

// DeactivateOrganization soft-deletes an organization (super admin only)
func (s *OrganizationService) DeactivateOrganization(orgID int64, deactivatedByUserID int64) error {
	query := `
		UPDATE orgs 
		SET is_active = false, updated_at = NOW()
		WHERE id = $1 AND is_active = true
	`

	result, err := s.db.Exec(query, orgID)
	if err != nil {
		s.logger.Printf("Error deactivating organization: %v", err)
		return fmt.Errorf("failed to deactivate organization: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("organization not found or already deactivated")
	}

	s.logger.Printf("Organization deactivated: ID=%d, DeactivatedBy=%d", orgID, deactivatedByUserID)
	return nil
}

// GetOrganizationMembers returns all members of an organization with their roles
func (s *OrganizationService) GetOrganizationMembers(orgID int64, limit, offset int) ([]*models.UserWithRole, int64, error) {
	// Get total count
	countQuery := `
		SELECT COUNT(*)
		FROM users u
		INNER JOIN user_roles ur ON u.id = ur.user_id
		INNER JOIN roles r ON ur.role_id = r.id
		WHERE ur.org_id = $1 AND u.is_active = true
	`

	var totalCount int64
	err := s.db.QueryRow(countQuery, orgID).Scan(&totalCount)
	if err != nil {
		s.logger.Printf("Error counting organization members: %v", err)
		return nil, 0, fmt.Errorf("failed to count members: %w", err)
	}

	// Get members with pagination
	query := `
		SELECT u.id, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at,
		       u.created_at, u.updated_at, u.created_by_user_id, u.password_reset_required,
		       r.name as role, r.id as role_id, ur.org_id
		FROM users u
		INNER JOIN user_roles ur ON u.id = ur.user_id
		INNER JOIN roles r ON ur.role_id = r.id
		WHERE ur.org_id = $1 AND u.is_active = true
		ORDER BY u.email ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := s.db.Query(query, orgID, limit, offset)
	if err != nil {
		s.logger.Printf("Error querying organization members: %v", err)
		return nil, 0, fmt.Errorf("failed to get members: %w", err)
	}
	defer rows.Close()

	var members []*models.UserWithRole
	for rows.Next() {
		var user models.UserWithRole
		var firstName, lastName sql.NullString
		var lastLoginAt sql.NullTime
		var createdByUserID sql.NullInt64

		err := rows.Scan(
			&user.ID,
			&user.Email,
			&firstName,
			&lastName,
			&user.IsActive,
			&lastLoginAt,
			&user.CreatedAt,
			&user.UpdatedAt,
			&createdByUserID,
			&user.PasswordResetRequired,
			&user.Role,
			&user.RoleID,
			&user.OrgID,
		)
		if err != nil {
			s.logger.Printf("Error scanning member row: %v", err)
			continue
		}

		if firstName.Valid {
			user.FirstName = &firstName.String
		}
		if lastName.Valid {
			user.LastName = &lastName.String
		}
		if lastLoginAt.Valid {
			user.LastLoginAt = &lastLoginAt.Time
		}
		if createdByUserID.Valid {
			user.CreatedByUserID = &createdByUserID.Int64
		}

		members = append(members, &user)
	}

	return members, totalCount, nil
}

// ChangeUserRole changes a user's role in an organization (owners + super admin)
func (s *OrganizationService) ChangeUserRole(orgID, userID, newRoleID int64, changedByUserID int64) error {
	// First check if user exists in this organization
	checkQuery := `
		SELECT ur.role_id, r.name
		FROM user_roles ur
		INNER JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND ur.org_id = $2
	`

	var oldRoleID int64
	var oldRoleName string
	err := s.db.QueryRow(checkQuery, userID, orgID).Scan(&oldRoleID, &oldRoleName)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("user not found in organization")
		}
		return fmt.Errorf("failed to check current role: %w", err)
	}

	if oldRoleID == newRoleID {
		return fmt.Errorf("user already has this role")
	}

	// Start transaction for role change
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Update role
	updateQuery := `
		UPDATE user_roles 
		SET role_id = $1, updated_at = NOW()
		WHERE user_id = $2 AND org_id = $3
	`

	_, err = tx.Exec(updateQuery, newRoleID, userID, orgID)
	if err != nil {
		return fmt.Errorf("failed to update role: %w", err)
	}

	// Log the role change in audit trail
	auditQuery := `
		INSERT INTO user_role_history (user_id, org_id, old_role_id, new_role_id, changed_by_user_id, created_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
	`

	_, err = tx.Exec(auditQuery, userID, orgID, oldRoleID, newRoleID, changedByUserID)
	if err != nil {
		return fmt.Errorf("failed to log role change: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit role change: %w", err)
	}

	s.logger.Printf("Role changed: UserID=%d, OrgID=%d, OldRole=%s, NewRoleID=%d, ChangedBy=%d",
		userID, orgID, oldRoleName, newRoleID, changedByUserID)

	return nil
}

// GetOrganizationAnalytics returns analytics for an organization
func (s *OrganizationService) GetOrganizationAnalytics(orgID int64) (*models.OrgAnalytics, error) {
	analytics := &models.OrgAnalytics{
		OrgID: orgID,
	}

	// Get member counts by role
	roleQuery := `
		SELECT r.name, COUNT(ur.user_id) as count
		FROM roles r
		LEFT JOIN user_roles ur ON r.id = ur.role_id AND ur.org_id = $1
		LEFT JOIN users u ON ur.user_id = u.id AND u.is_active = true
		GROUP BY r.id, r.name
		ORDER BY r.name
	`

	rows, err := s.db.Query(roleQuery, orgID)
	if err != nil {
		s.logger.Printf("Error getting role analytics: %v", err)
		return nil, fmt.Errorf("failed to get role analytics: %w", err)
	}
	defer rows.Close()

	analytics.MembersByRole = make(map[string]int64)
	totalMembers := int64(0)

	for rows.Next() {
		var roleName string
		var count int64
		err := rows.Scan(&roleName, &count)
		if err != nil {
			continue
		}
		analytics.MembersByRole[roleName] = count
		totalMembers += count
	}

	analytics.TotalMembers = totalMembers

	// Get recent activity count (last 30 days)
	activityQuery := `
		SELECT COUNT(*)
		FROM user_management_audit
		WHERE org_id = $1 AND created_at >= NOW() - INTERVAL '30 days'
	`

	err = s.db.QueryRow(activityQuery, orgID).Scan(&analytics.RecentActivity)
	if err != nil {
		s.logger.Printf("Error getting activity analytics: %v", err)
		// Don't fail the whole request for this
		analytics.RecentActivity = 0
	}

	return analytics, nil
}
