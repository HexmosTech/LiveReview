package users

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"github.com/rs/zerolog/log"
	"github.com/livereview/network/email"
	storageusers "github.com/livereview/storage/users"
	"golang.org/x/crypto/bcrypt"
)

// getProductionURL fetches the production URL from instance_details table
func (us *UserService) getProductionURL() string {
	var url sql.NullString
	err := us.db.QueryRow("SELECT livereview_prod_url FROM instance_details LIMIT 1").Scan(&url)
	if err != nil || !url.Valid {
		return ""
	}
	return strings.TrimSpace(url.String)
}

// checkInvitationPrerequisites verifies that production URL and SMTP are configured
func (us *UserService) checkInvitationPrerequisites() error {
	var missing []string

	// Check production URL
	prodURL := us.getProductionURL()
	if prodURL == "" {
		missing = append(missing, "Production URL (Settings → Instance)")
	}

	// Check SMTP settings
	var data []byte
	err := us.db.QueryRow("SELECT data FROM system_settings WHERE name = 'smtp'").Scan(&data)
	if err != nil {
		missing = append(missing, "SMTP settings (Settings → SMTP)")
	} else {
		var settings struct {
			Host string `json:"host"`
		}
		if err := json.Unmarshal(data, &settings); err != nil || settings.Host == "" {
			missing = append(missing, "SMTP settings (Settings → SMTP)")
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("invitations require the following to be configured: %s", strings.Join(missing, ", "))
	}
	return nil
}

// APIKeyGeneratorTx defines a function type to generate onboarding keys within a transaction context
type APIKeyGeneratorTx func(tx *sql.Tx, userID, orgID int64) (string, error)

// UserService handles core user management operations
type UserService struct {
	db              *sql.DB
	store           *storageusers.UserStore
	apiKeyGenerator APIKeyGeneratorTx
}

// NewUserService creates a new user service
func NewUserService(db *sql.DB, apiKeyGenerator APIKeyGeneratorTx) *UserService {
	return &UserService{
		db:              db,
		store:           storageusers.NewUserStore(db),
		apiKeyGenerator: apiKeyGenerator,
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
	OnboardingAPIKey      string     `json:"onboarding_api_key,omitempty"`
}

// CreateUserRequest represents the request to create a new user
type CreateUserRequest struct {
	Email     string `json:"email" validate:"required,email"`
	Password  string `json:"password" validate:"omitempty,min=8"`
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
	Password  *string `json:"password,omitempty"`
}

// CreateUserInOrg creates a new user in the specified organization
func (us *UserService) CreateUserInOrg(orgID, createdByUserID int64, req CreateUserRequest) (*UserWithRole, error) {
	var hashedPassword []byte
	var err error
	if req.Password != "" {
		// Hash password
		hashedPassword, err = bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
	}

	var userID int64
	err = us.store.WithTx(func(tx *sql.Tx) error {
		// Check if email already exists globally
		var existingUserID int64
		err := us.store.TxQueryRow(tx, "SELECT id FROM users WHERE email = $1", req.Email).Scan(&existingUserID)
		
		if err == nil {
			// User exists globally. Check if they are already in THIS organization.
			var existsInOrg bool
			err = us.store.TxQueryRow(tx, "SELECT EXISTS(SELECT 1 FROM user_roles WHERE user_id = $1 AND org_id = $2)", existingUserID, orgID).Scan(&existsInOrg)
			if err != nil {
				return fmt.Errorf("failed to check existing user role: %w", err)
			}
			if existsInOrg {
				return fmt.Errorf("user with email %s is already a member of this organization", req.Email)
			}

			// Link existing user to this organization
			userID = existingUserID
		} else if err == sql.ErrNoRows {
			if len(hashedPassword) == 0 {
				return fmt.Errorf("password is required for new users")
			}
			// Create new user globally
			err = us.store.TxQueryRow(tx, `
				INSERT INTO users (email, password_hash, first_name, last_name, created_by_user_id, password_reset_required, created_at, updated_at)
				VALUES ($1, $2, $3, $4, $5, true, NOW(), NOW())
				RETURNING id
			`, req.Email, string(hashedPassword), req.FirstName, req.LastName, createdByUserID).Scan(&userID)
			if err != nil {
				return fmt.Errorf("failed to create user: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check existing email: %w", err)
		}

		// Add user role in this organization
		_, err = us.store.TxExec(tx, `
			INSERT INTO user_roles (user_id, org_id, role_id, created_at, updated_at)
			VALUES ($1, $2, $3, NOW(), NOW())
		`, userID, orgID, req.RoleID)
		if err != nil {
			return fmt.Errorf("failed to assign user role: %w", err)
		}

		// Update user's default_org_id if it is currently NULL
		_, err = us.store.TxExec(tx, `
			UPDATE users 
			SET default_org_id = COALESCE(default_org_id, $1) 
			WHERE id = $2
		`, orgID, userID)
		if err != nil {
			return fmt.Errorf("failed to update user default organization: %w", err)
		}

		// Generate onboarding API key if generator is configured
		if us.apiKeyGenerator != nil {
			newKey, err := us.apiKeyGenerator(tx, userID, orgID)
			if err != nil {
				return fmt.Errorf("failed to generate onboarding API key: %w", err)
			}
			_, err = us.store.TxExec(tx, `UPDATE users SET onboarding_api_key = $1 WHERE id = $2`, newKey, userID)
			if err != nil {
				return fmt.Errorf("failed to update onboarding API key in database: %w", err)
			}
		}

		// Add audit trail
		err = us.addUserAuditLog(tx, orgID, userID, createdByUserID, "created", map[string]interface{}{
			"role_id": req.RoleID,
			"email":   req.Email,
			"note":    "user linked to organization (existing global user)",
		})
		if err != nil {
			return fmt.Errorf("failed to add audit log: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Get the created user with role
	user, err := us.GetUserInOrg(orgID, userID)
	if err != nil {
		return nil, err
	}

	// Send invitation email asynchronously
	go us.sendInvitation(user, createdByUserID)

	return user, nil
}

func (us *UserService) sendInvitation(user *UserWithRole, invitedByUserID int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Msgf("Critical: panic recovered in invitation flow: %v", r)
		}
	}()

	invitedByName := us.getInvitedByUserName(invitedByUserID)

	invitedToName := user.Email
	if user.FirstName != nil && *user.FirstName != "" {
		invitedToName = *user.FirstName
	}

	prodURL := us.getProductionURL()

	installCmdLinux := ""
	installCmdWindows := ""
	if user.OnboardingAPIKey != "" && prodURL != "" {
		installCmdLinux = fmt.Sprintf("curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY=%q LRC_API_URL=%q bash", user.OnboardingAPIKey, prodURL)
		installCmdWindows = fmt.Sprintf("$env:LRC_API_KEY=%q; $env:LRC_API_URL=%q; iwr -useb https://hexmos.com/lrc-install.ps1 | iex", user.OnboardingAPIKey, prodURL)
	}

	err := email.SendInvitationEmail(us.db, email.InvitationParams{
		AppName:               "LiveReview",
		InvitedToName:         invitedToName,
		InvitedToEmail:        user.Email,
		InvitedByName:         invitedByName,
		URL:                   prodURL,
		InstallCommandLinux:   installCmdLinux,
		InstallCommandWindows: installCmdWindows,
	})
	if err != nil {
		log.Error().Err(err).Str("email", user.Email).Msg("Failed to send invitation email")
	}
}

func (us *UserService) getInvitedByUserName(userID int64) string {
	var firstName, lastName sql.NullString
	err := us.store.QueryRow("SELECT first_name, last_name FROM users WHERE id = $1", userID).Scan(&firstName, &lastName)
	if err != nil {
		return "An Admin"
	}

	name := ""
	if firstName.Valid {
		name = firstName.String
	}
	if lastName.Valid {
		if name != "" {
			name += " "
		}
		name += lastName.String
	}

	if name == "" {
		return "An Admin"
	}
	return name
}

// CheckUserByEmail checks if a user exists globally and returns basic info
func (us *UserService) CheckUserByEmail(email string) (*UserCheckResponse, error) {
	var id int64
	var firstName, lastName sql.NullString
	err := us.store.QueryRow(`
		SELECT id, first_name, last_name FROM users WHERE email = $1
	`, email).Scan(&id, &firstName, &lastName)

	if err != nil {
		if err == sql.ErrNoRows {
			return &UserCheckResponse{
				Exists: false,
			}, nil
		}
		return nil, fmt.Errorf("failed to check user: %w", err)
	}

	return &UserCheckResponse{
		Exists:    true,
		ID:        id,
		FirstName: firstName.String,
		LastName:  lastName.String,
	}, nil
}

// BulkCheckUserRow represents a single candidate row submitted for bulk invite verification
type BulkCheckUserRow struct {
	Email     string `json:"email" validate:"required,email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Role      string `json:"role"`
}

// BulkCheckResultRow is a BulkCheckUserRow enriched with existing-membership info.
// The Old* fields are only populated when the org already has a member with that
// email and the corresponding field differs from what was submitted.
type BulkCheckResultRow struct {
	Email          string `json:"email"`
	FirstName      string `json:"first_name"`
	LastName       string `json:"last_name"`
	Role           string `json:"role"`
	Exists         bool   `json:"exists"`
	ExistsGlobally bool   `json:"exists_globally"`
	OldEmail       string `json:"old_email,omitempty"`
	OldFirstName   string `json:"old_first_name,omitempty"`
	OldLastName    string `json:"old_last_name,omitempty"`
	OldRole        string `json:"old_role,omitempty"`
}

// BulkCheckUsersInOrg compares a batch of candidate rows (e.g. parsed from a CSV upload)
// against existing members of orgID, matched by email. It does not create or modify
// any users — it's a read-only preview used before an actual bulk invite is submitted.
func (us *UserService) BulkCheckUsersInOrg(orgID int64, rows []BulkCheckUserRow) ([]BulkCheckResultRow, error) {
	emails := make([]string, 0, len(rows))
	for _, r := range rows {
		emails = append(emails, strings.ToLower(strings.TrimSpace(r.Email)))
	}

	type existingMember struct {
		Email     string
		FirstName string
		LastName  string
		Role      string
	}
	existingByEmail := make(map[string]existingMember, len(rows))

	if len(emails) > 0 {
		dbRows, err := us.store.Query(`
			SELECT u.email, u.first_name, u.last_name, r.name
			FROM users u
			JOIN user_roles ur ON u.id = ur.user_id
			JOIN roles r ON ur.role_id = r.id
			WHERE ur.org_id = $1 AND LOWER(u.email) = ANY($2)
		`, orgID, pq.Array(emails))
		if err != nil {
			return nil, fmt.Errorf("failed to look up existing members: %w", err)
		}
		defer dbRows.Close()

		for dbRows.Next() {
			var m existingMember
			var firstName, lastName sql.NullString
			if err := dbRows.Scan(&m.Email, &firstName, &lastName, &m.Role); err != nil {
				return nil, fmt.Errorf("failed to scan existing member: %w", err)
			}
			m.FirstName = firstName.String
			m.LastName = lastName.String
			existingByEmail[strings.ToLower(m.Email)] = m
		}
	}

	globalEmails := make(map[string]bool, len(rows))
	if len(emails) > 0 {
		globalRows, err := us.store.Query(`SELECT LOWER(email) FROM users WHERE LOWER(email) = ANY($1)`, pq.Array(emails))
		if err != nil {
			return nil, fmt.Errorf("failed to look up global users: %w", err)
		}
		defer globalRows.Close()
		for globalRows.Next() {
			var email string
			if err := globalRows.Scan(&email); err != nil {
				return nil, fmt.Errorf("failed to scan global user: %w", err)
			}
			globalEmails[email] = true
		}
	}

	results := make([]BulkCheckResultRow, 0, len(rows))
	for _, r := range rows {
		result := BulkCheckResultRow{
			Email:     strings.TrimSpace(r.Email),
			FirstName: strings.TrimSpace(r.FirstName),
			LastName:  strings.TrimSpace(r.LastName),
			Role:      strings.TrimSpace(r.Role),
		}
		result.ExistsGlobally = globalEmails[strings.ToLower(result.Email)]

		if existing, ok := existingByEmail[strings.ToLower(result.Email)]; ok {
			result.Exists = true
			if existing.Email != result.Email {
				result.OldEmail = existing.Email
			}
			if existing.FirstName != result.FirstName {
				result.OldFirstName = existing.FirstName
			}
			if existing.LastName != result.LastName {
				result.OldLastName = existing.LastName
			}
			if !strings.EqualFold(existing.Role, result.Role) {
				result.OldRole = existing.Role
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// BulkInviteUserRow represents a single row submitted for a real bulk invite/update
type BulkInviteUserRow struct {
	Email           string `json:"email"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Role            string `json:"role"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// BulkInviteResultRow reports what happened to a single BulkInviteUserRow
type BulkInviteResultRow struct {
	Email            string `json:"email"`
	Status           string `json:"status"` // "invited" | "updated" | "unchanged" | "error"
	Message          string `json:"message,omitempty"`
	OnboardingAPIKey string `json:"onboarding_api_key,omitempty"`
}

// BulkInvitePermissions mirrors the finer-grained permissions the single-user
// endpoints check (CreateUser needs create_users, UpdateUser needs edit_users,
// ChangeUserRole needs manage_roles) so a future split of these permissions
// doesn't silently let a create-only actor also edit or re-role existing members.
type BulkInvitePermissions struct {
	CanCreate      bool
	CanEdit        bool
	CanManageRoles bool
}

// BulkInviteUsersInOrg processes a batch of rows (e.g. from a reviewed CSV upload),
// creating brand-new org members and updating existing ones. Rows are processed
// independently (best-effort) so one bad row doesn't block the rest of the batch.
func (us *UserService) BulkInviteUsersInOrg(orgID, actorUserID int64, rows []BulkInviteUserRow, perms BulkInvitePermissions) ([]BulkInviteResultRow, error) {
	roleIDsByName := make(map[string]int64)
	roleRows, err := us.store.Query(`SELECT id, name FROM roles`)
	if err != nil {
		return nil, fmt.Errorf("failed to load roles: %w", err)
	}
	for roleRows.Next() {
		var id int64
		var name string
		if err := roleRows.Scan(&id, &name); err != nil {
			roleRows.Close()
			return nil, fmt.Errorf("failed to scan role: %w", err)
		}
		roleIDsByName[strings.ToLower(name)] = id
	}
	roleRows.Close()

	type existingMember struct {
		UserID    int64
		FirstName string
		LastName  string
		RoleID    int64
		RoleName  string
	}
	emails := make([]string, 0, len(rows))
	for _, r := range rows {
		emails = append(emails, strings.ToLower(strings.TrimSpace(r.Email)))
	}
	existingByEmail := make(map[string]existingMember, len(rows))
	if len(emails) > 0 {
		dbRows, err := us.store.Query(`
			SELECT u.id, u.email, u.first_name, u.last_name, r.id, r.name
			FROM users u
			JOIN user_roles ur ON u.id = ur.user_id
			JOIN roles r ON ur.role_id = r.id
			WHERE ur.org_id = $1 AND LOWER(u.email) = ANY($2)
		`, orgID, pq.Array(emails))
		if err != nil {
			return nil, fmt.Errorf("failed to look up existing members: %w", err)
		}
		for dbRows.Next() {
			var m existingMember
			var email, firstName, lastName sql.NullString
			if err := dbRows.Scan(&m.UserID, &email, &firstName, &lastName, &m.RoleID, &m.RoleName); err != nil {
				dbRows.Close()
				return nil, fmt.Errorf("failed to scan existing member: %w", err)
			}
			m.FirstName = firstName.String
			m.LastName = lastName.String
			existingByEmail[strings.ToLower(email.String)] = m
		}
		dbRows.Close()
	}

	results := make([]BulkInviteResultRow, 0, len(rows))
	for _, r := range rows {
		email := strings.TrimSpace(r.Email)
		firstName := strings.TrimSpace(r.FirstName)
		lastName := strings.TrimSpace(r.LastName)
		roleName := strings.ToLower(strings.TrimSpace(r.Role))

		roleID, roleKnown := roleIDsByName[roleName]
		if !roleKnown {
			results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: fmt.Sprintf("Unknown role '%s'", r.Role)})
			continue
		}

		if existing, ok := existingByEmail[strings.ToLower(email)]; ok {
			updateReq := UpdateUserRequest{}
			nameChanging := false

			if firstName != existing.FirstName {
				updateReq.FirstName = &firstName
				nameChanging = true
			}
			if lastName != existing.LastName {
				updateReq.LastName = &lastName
				nameChanging = true
			}
			roleChanging := roleID != existing.RoleID

			if (nameChanging || roleChanging) && !perms.CanEdit {
				results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Permission denied: cannot update users"})
				continue
			}
			if roleChanging && !perms.CanManageRoles {
				results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Permission denied: cannot change user roles"})
				continue
			}

			if roleChanging {
				if strings.EqualFold(existing.RoleName, "owner") {
					var remainingOwners int
					err := us.store.QueryRow(`
						SELECT COUNT(*) FROM user_roles ur
						JOIN roles r ON ur.role_id = r.id
						WHERE ur.org_id = $1 AND LOWER(r.name) = 'owner' AND ur.user_id != $2
					`, orgID, existing.UserID).Scan(&remainingOwners)
					if err != nil {
						results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Failed to verify owner count"})
						continue
					}
					if remainingOwners == 0 {
						results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Cannot change role: this is the only owner of the organization"})
						continue
					}
				}
				updateReq.RoleID = &roleID
			}

			if !nameChanging && !roleChanging {
				results = append(results, BulkInviteResultRow{Email: email, Status: "unchanged"})
				continue
			}

			updatedUser, err := us.UpdateUserInOrg(orgID, existing.UserID, actorUserID, updateReq)
			if err != nil {
				results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: friendlyBulkInviteError(err, "update")})
				continue
			}
			results = append(results, BulkInviteResultRow{Email: email, Status: "updated", OnboardingAPIKey: updatedUser.OnboardingAPIKey})
			continue
		}

		if !perms.CanCreate {
			results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Permission denied: cannot create users"})
			continue
		}

		if r.Password != "" {
			if len(r.Password) < 8 {
				results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Password must be at least 8 characters"})
				continue
			}
			if r.Password != r.ConfirmPassword {
				results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: "Passwords do not match"})
				continue
			}
		}

		newUser, err := us.CreateUserInOrg(orgID, actorUserID, CreateUserRequest{
			Email:     email,
			Password:  r.Password,
			FirstName: firstName,
			LastName:  lastName,
			RoleID:    roleID,
		})
		if err != nil {
			results = append(results, BulkInviteResultRow{Email: email, Status: "error", Message: friendlyBulkInviteError(err, "invite")})
			continue
		}
		results = append(results, BulkInviteResultRow{Email: email, Status: "invited", OnboardingAPIKey: newUser.OnboardingAPIKey})
	}

	return results, nil
}

// friendlyBulkInviteError maps known CreateUserInOrg/UpdateUserInOrg error cases to a
// message safe to show an admin, logging the raw error for anything unexpected.
func friendlyBulkInviteError(err error, action string) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "already a member"):
		return "Already a member of this organization"
	case strings.Contains(msg, "password is required for new users"):
		return "Password is required for new users"
	case strings.Contains(msg, "passwords do not match"):
		return "Passwords do not match"
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return "Email already exists"
	}

	log.Error().Err(err).Str("action", action).Msg("Bulk invite row failed")
	if action == "update" {
		return "Failed to update user"
	}
	return "Failed to invite user"
}

// GetUserInOrg gets a user in a specific organization with their role
func (us *UserService) GetUserInOrg(orgID, userID int64) (*UserWithRole, error) {
	user := &UserWithRole{}
	var onboardingKey sql.NullString
	err := us.store.QueryRow(`
		SELECT u.id, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at,
		       u.created_at, u.updated_at, u.created_by_user_id, u.deactivated_at,
		       u.deactivated_by_user_id, u.password_reset_required,
		       r.name as role, r.id as role_id, ur.org_id, u.onboarding_api_key
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		JOIN roles r ON ur.role_id = r.id
		WHERE u.id = $1 AND ur.org_id = $2
	`, userID, orgID).Scan(
		&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.IsActive,
		&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.CreatedByUserID,
		&user.DeactivatedAt, &user.DeactivatedByUserID, &user.PasswordResetRequired,
		&user.Role, &user.RoleID, &user.OrgID, &onboardingKey,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found in organization")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if onboardingKey.Valid {
		user.OnboardingAPIKey = onboardingKey.String
	}

	return user, nil
}

// ListUsersInOrg lists all users in an organization with pagination
func (us *UserService) ListUsersInOrg(orgID int64, offset, limit int) ([]*UserWithRole, int, error) {
	// Get total count
	var totalCount int
	err := us.store.QueryRow(`
		SELECT COUNT(*)
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		WHERE ur.org_id = $1
	`, orgID).Scan(&totalCount)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get user count: %w", err)
	}

	// Get users with pagination
	rows, err := us.store.Query(`
		SELECT u.id, u.email, u.first_name, u.last_name, u.is_active, u.last_login_at,
		       u.created_at, u.updated_at, u.created_by_user_id, u.deactivated_at,
		       u.deactivated_by_user_id, u.password_reset_required,
		       r.name as role, r.id as role_id, ur.org_id, u.onboarding_api_key
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
		var onboardingKey sql.NullString
		err := rows.Scan(
			&user.ID, &user.Email, &user.FirstName, &user.LastName, &user.IsActive,
			&user.LastLoginAt, &user.CreatedAt, &user.UpdatedAt, &user.CreatedByUserID,
			&user.DeactivatedAt, &user.DeactivatedByUserID, &user.PasswordResetRequired,
			&user.Role, &user.RoleID, &user.OrgID, &onboardingKey,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan user: %w", err)
		}
		if onboardingKey.Valid {
			user.OnboardingAPIKey = onboardingKey.String
		}
		users = append(users, user)
	}

	return users, totalCount, nil
}

// UpdateUserInOrg updates a user in a specific organization
func (us *UserService) UpdateUserInOrg(orgID, userID, updatedByUserID int64, req UpdateUserRequest) (*UserWithRole, error) {
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

	if req.Password != nil && *req.Password != "" {
		hashedPassword, hashErr := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, fmt.Errorf("failed to hash password: %w", hashErr)
		}
		setParts = append(setParts, fmt.Sprintf("password_hash = $%d", argIndex))
		args = append(args, string(hashedPassword))
		auditDetails["password_changed"] = true
		argIndex++
	}

	err := us.store.WithTx(func(tx *sql.Tx) error {
		// Update user if there are changes
		if len(setParts) > 1 { // More than just updated_at
			query := fmt.Sprintf("UPDATE users SET %s WHERE id = $%d",
				strings.Join(setParts, ", "), argIndex)
			args = append(args, userID)

			_, err := us.store.TxExec(tx, query, args...)
			if err != nil {
				return fmt.Errorf("failed to update user: %w", err)
			}

			// Add audit log for user update
			err = us.addUserAuditLog(tx, orgID, userID, updatedByUserID, "updated", auditDetails)
			if err != nil {
				return fmt.Errorf("failed to add audit log: %w", err)
			}
		}

		// Handle role change separately
		if req.RoleID != nil {
			// Get current role for audit trail
			var currentRoleID int64
			err := us.store.TxQueryRow(tx, `
				SELECT role_id FROM user_roles WHERE user_id = $1 AND org_id = $2
			`, userID, orgID).Scan(&currentRoleID)
			if err != nil {
				return fmt.Errorf("failed to get current role: %w", err)
			}

			if currentRoleID != *req.RoleID {
				// Update role
				_, err = us.store.TxExec(tx, `
					UPDATE user_roles SET role_id = $1, updated_at = NOW()
					WHERE user_id = $2 AND org_id = $3
				`, *req.RoleID, userID, orgID)
				if err != nil {
					return fmt.Errorf("failed to update user role: %w", err)
				}

				// Add role change to audit history
				_, err = us.store.TxExec(tx, `
					INSERT INTO user_role_history (user_id, org_id, old_role_id, new_role_id, changed_by_user_id, created_at)
					VALUES ($1, $2, $3, $4, $5, NOW())
				`, userID, orgID, currentRoleID, *req.RoleID, updatedByUserID)
				if err != nil {
					return fmt.Errorf("failed to add role history: %w", err)
				}

				// Add audit log for role change
				err = us.addUserAuditLog(tx, orgID, userID, updatedByUserID, "role_changed", map[string]interface{}{
					"old_role_id": currentRoleID,
					"new_role_id": *req.RoleID,
				})
				if err != nil {
					return fmt.Errorf("failed to add role change audit log: %w", err)
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Get updated user
	return us.GetUserInOrg(orgID, userID)
}

// DeactivateUserInOrg deactivates a user in a specific organization
func (us *UserService) DeactivateUserInOrg(orgID, userID, deactivatedByUserID int64) error {
	return us.store.WithTx(func(tx *sql.Tx) error {
		// Deactivate user
		_, err := us.store.TxExec(tx, `
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

		return nil
	})
}

// ForcePasswordReset forces a user to reset their password on next login
func (us *UserService) ForcePasswordReset(orgID, userID, updatedByUserID int64) error {
	return us.store.WithTx(func(tx *sql.Tx) error {
		// Set password reset required
		_, err := us.store.TxExec(tx, `
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

		return nil
	})
}

// GetUserAuditLog gets the audit log for a user
func (us *UserService) GetUserAuditLog(orgID, userID int64, offset, limit int) ([]UserAuditEntry, error) {
	rows, err := us.store.Query(`
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

	_, err = us.store.TxExec(tx, `
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
	err := us.store.QueryRow(`
		SELECT COUNT(DISTINCT u.id)
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
	`).Scan(&totalCount)

	if err != nil {
		return nil, 0, fmt.Errorf("failed to get total user count: %w", err)
	}

	// Get users with pagination
	rows, err := us.store.Query(`
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
	err := us.store.QueryRow("SELECT EXISTS(SELECT 1 FROM orgs WHERE id = $1)", targetOrgID).Scan(&orgExists)
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
	err := us.store.WithTx(func(tx *sql.Tx) error {
		// Verify user exists
		var currentOrgID int64
		var currentRoleID int64
		err := us.store.TxQueryRow(tx, `
			SELECT ur.org_id, ur.role_id 
			FROM user_roles ur 
			WHERE ur.user_id = $1
		`, userID).Scan(&currentOrgID, &currentRoleID)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("user not found")
			}
			return fmt.Errorf("failed to get current user org: %w", err)
		}

		// Verify target organization exists
		var orgExists bool
		err = us.store.TxQueryRow(tx, "SELECT EXISTS(SELECT 1 FROM orgs WHERE id = $1)", newOrgID).Scan(&orgExists)
		if err != nil {
			return fmt.Errorf("failed to check target organization: %w", err)
		}
		if !orgExists {
			return fmt.Errorf("target organization does not exist")
		}

		// Verify target role exists
		var roleExists bool
		err = us.store.TxQueryRow(tx, "SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1)", newRoleID).Scan(&roleExists)
		if err != nil {
			return fmt.Errorf("failed to check target role: %w", err)
		}
		if !roleExists {
			return fmt.Errorf("target role does not exist")
		}

		// Update user's organization and role
		_, err = us.store.TxExec(tx, `
			UPDATE user_roles 
			SET org_id = $1, role_id = $2, updated_at = NOW()
			WHERE user_id = $3
		`, newOrgID, newRoleID, userID)
		if err != nil {
			return fmt.Errorf("failed to transfer user: %w", err)
		}

		// Add audit log for the transfer
		err = us.addUserAuditLog(tx, newOrgID, userID, performedByUserID, "transferred", map[string]interface{}{
			"old_org_id":  currentOrgID,
			"new_org_id":  newOrgID,
			"old_role_id": currentRoleID,
			"new_role_id": newRoleID,
		})
		if err != nil {
			return fmt.Errorf("failed to add transfer audit log: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Get updated user
	return us.GetUserInOrg(newOrgID, userID)
}

// GetUserAnalytics provides basic analytics about users (super admin only)
func (us *UserService) GetUserAnalytics() (*UserAnalytics, error) {
	analytics := &UserAnalytics{}

	// Total users count
	err := us.store.QueryRow("SELECT COUNT(*) FROM users").Scan(&analytics.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get total users: %w", err)
	}

	// Active users count
	err = us.store.QueryRow("SELECT COUNT(*) FROM users WHERE is_active = true").Scan(&analytics.ActiveUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users: %w", err)
	}

	// Users requiring password reset
	err = us.store.QueryRow("SELECT COUNT(*) FROM users WHERE password_reset_required = true").Scan(&analytics.UsersNeedingPasswordReset)
	if err != nil {
		return nil, fmt.Errorf("failed to get users needing password reset: %w", err)
	}

	// Users by organization
	rows, err := us.store.Query(`
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
	rows, err = us.store.Query(`
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

// UserCheckResponse represents the response for checking user existence
type UserCheckResponse struct {
	Exists    bool   `json:"exists"`
	ID        int64  `json:"id,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}
