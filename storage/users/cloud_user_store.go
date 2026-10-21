package users

import (
	"database/sql"
	"errors"
)

// Queries behind cloud user provisioning (internal/api/auth EnsureCloudUser). They are moved verbatim from the
// handler so the HTTP endpoint and the GitHub App callback share exactly one provisioning path.

// FindUserByEmail returns the user's id and password hash; sql.ErrNoRows when there is no such user.
func (s *UserStore) FindUserByEmail(tx *sql.Tx, email string) (userID int64, passwordHash string, err error) {
	err = tx.QueryRow(`SELECT id, password_hash FROM users WHERE email = $1`, email).Scan(&userID, &passwordHash)
	return userID, passwordHash, err
}

// InsertCloudUser creates a user whose password is a random, unused hash (cloud users sign in via Hexmos).
// A nil firstName/lastName stores NULL.
func (s *UserStore) InsertCloudUser(tx *sql.Tx, email, passwordHash string, firstName, lastName interface{}) (int64, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO users (email, password_hash, first_name, last_name, password_reset_required, created_at, updated_at)
		VALUES ($1, $2, $3, $4, false, NOW(), NOW()) RETURNING id
	`, email, passwordHash, firstName, lastName).Scan(&id)
	return id, err
}

// FindOrgIDByName returns the org with this exact name; sql.ErrNoRows when none exists.
func (s *UserStore) FindOrgIDByName(tx *sql.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM orgs WHERE name = $1`, name).Scan(&id)
	return id, err
}

// InsertPersonalOrg creates an org named after the user's email (callers also InsertPromptApplicationContext).
func (s *UserStore) InsertPersonalOrg(tx *sql.Tx, email string, createdByUserID int64) (int64, error) {
	var orgID int64
	err := tx.QueryRow(`
		INSERT INTO orgs (name, description, created_by_user_id, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW()) RETURNING id
	`, email, "Personal organization for "+email, createdByUserID).Scan(&orgID)
	if err != nil {
		return 0, err
	}
	return orgID, nil
}

// InsertPromptApplicationContext creates the per-org prompt context row a new org needs.
func (s *UserStore) InsertPromptApplicationContext(tx *sql.Tx, orgID int64) error {
	_, err := tx.Exec(`INSERT INTO prompt_application_context (org_id, created_at, updated_at) VALUES ($1, NOW(), NOW())`, orgID)
	return err
}

// SetDefaultOrgIfUnset points the user at this org unless they already have a default org.
func (s *UserStore) SetDefaultOrgIfUnset(tx *sql.Tx, userID, orgID int64) error {
	_, err := tx.Exec(`
		UPDATE users
		SET default_org_id = COALESCE(default_org_id, $1)
		WHERE id = $2
	`, orgID, userID)
	return err
}

func (s *UserStore) SetOnboardingAPIKey(tx *sql.Tx, userID int64, key string) error {
	_, err := tx.Exec(`UPDATE users SET onboarding_api_key = $1 WHERE id = $2`, key, userID)
	return err
}

// OwnerRoleID is the id of the 'owner' role (assigned to the creator of a personal org).
func (s *UserStore) OwnerRoleID(tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT id FROM roles WHERE name = 'owner' LIMIT 1`).Scan(&id)
	return id, err
}

// UserRoleID returns the user's role in the org; sql.ErrNoRows when they have none.
func (s *UserStore) UserRoleID(tx *sql.Tx, userID, orgID int64) (int64, error) {
	var id int64
	err := tx.QueryRow(`SELECT role_id FROM user_roles WHERE user_id = $1 AND org_id = $2`, userID, orgID).Scan(&id)
	return id, err
}

func (s *UserStore) InsertFreeUserRole(tx *sql.Tx, userID, orgID, roleID int64) error {
	_, err := tx.Exec(`
		INSERT INTO user_roles (user_id, org_id, role_id, plan_type, created_at, updated_at)
		VALUES ($1, $2, $3, 'free', NOW(), NOW())
	`, userID, orgID, roleID)
	return err
}

func (s *UserStore) UpdateUserRole(tx *sql.Tx, userID, orgID, roleID int64) error {
	_, err := tx.Exec(`UPDATE user_roles SET role_id = $1, updated_at = NOW() WHERE user_id = $2 AND org_id = $3`, roleID, userID, orgID)
	return err
}

// DefaultOrgID reads the user's default org (nil when unset).
func (s *UserStore) DefaultOrgID(userID int64) (*int64, error) {
	var id *int64
	err := s.db.QueryRow(`SELECT default_org_id FROM users WHERE id = $1`, userID).Scan(&id)
	return id, err
}

// SetOrgSignupPath records how an org was created (e.g. "gh_app"); it never overwrites an existing value.
func (s *UserStore) SetOrgSignupPath(orgID int64, path string) error {
	_, err := s.db.Exec(`UPDATE orgs SET signup_path = $1 WHERE id = $2 AND signup_path IS NULL`, path, orgID)
	return err
}

// OrgSignupPath returns the org's signup path ("" when unset).
func (s *UserStore) OrgSignupPath(orgID int64) (string, error) {
	var p sql.NullString
	err := s.db.QueryRow(`SELECT signup_path FROM orgs WHERE id = $1`, orgID).Scan(&p)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return p.String, err
}
