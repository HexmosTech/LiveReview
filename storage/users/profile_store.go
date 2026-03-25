package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var ErrEmailInUse = errors.New("email already in use")

type ProfileStore struct {
	db *sql.DB
}

type UserProfileRecord struct {
	ID               int64
	Email            string
	FirstName        *string
	LastName         *string
	IsActive         bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastLoginAt      *time.Time
	PlanType         *string
	LicenseExpiresAt *time.Time
}

type UpdateProfileInput struct {
	UserID    int64
	FirstName *string
	LastName  *string
	Email     *string
}

type UserOrgRoleRecord struct {
	OrgID   int64
	OrgName string
	Role    string
	RoleID  int64
}

func NewProfileStore(db *sql.DB) *ProfileStore {
	return &ProfileStore{db: db}
}

func (s *ProfileStore) GetUserProfile(ctx context.Context, userID int64) (*UserProfileRecord, error) {
	rec := &UserProfileRecord{}
	err := s.db.QueryRowContext(ctx, `
		SELECT
			u.id, u.email, u.first_name, u.last_name, u.is_active,
			u.created_at, u.updated_at, u.last_login_at,
			ur.plan_type, ur.license_expires_at
		FROM users u
		LEFT JOIN LATERAL (
			SELECT plan_type, license_expires_at
			FROM user_roles
			WHERE user_id = u.id
			ORDER BY updated_at DESC
			LIMIT 1
		) ur ON true
		WHERE u.id = $1
	`, userID).Scan(
		&rec.ID, &rec.Email, &rec.FirstName, &rec.LastName,
		&rec.IsActive, &rec.CreatedAt, &rec.UpdatedAt, &rec.LastLoginAt,
		&rec.PlanType, &rec.LicenseExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (s *ProfileStore) UpdateUserProfile(ctx context.Context, input UpdateProfileInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if input.Email != nil {
		var existingUserID int64
		err = tx.QueryRowContext(ctx, "SELECT id FROM users WHERE email = $1 AND id != $2", *input.Email, input.UserID).Scan(&existingUserID)
		if err == nil {
			return ErrEmailInUse
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("failed to check existing email: %w", err)
		}

	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users
		SET first_name = COALESCE($1, first_name),
			last_name = COALESCE($2, last_name),
			email = COALESCE($3, email),
			updated_at = NOW()
		WHERE id = $4
	`, input.FirstName, input.LastName, input.Email, input.UserID); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return ErrEmailInUse
		}
		return err
	}

	return tx.Commit()
}

func (s *ProfileStore) GetCurrentPasswordHash(ctx context.Context, userID int64) (string, error) {
	var currentHash string
	err := s.db.QueryRowContext(ctx, "SELECT password_hash FROM users WHERE id = $1", userID).Scan(&currentHash)
	if err != nil {
		return "", err
	}
	return currentHash, nil
}

func (s *ProfileStore) UpdatePasswordIfCurrentHash(ctx context.Context, userID int64, currentHash, passwordHash string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = $1, password_reset_required = false, updated_at = NOW()
		WHERE id = $2 AND password_hash = $3
	`, passwordHash, userID, currentHash)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *ProfileStore) ListUserOrganizations(ctx context.Context, userID int64) ([]UserOrgRoleRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.id, o.name, r.name as role, ur.role_id
		FROM orgs o
		JOIN user_roles ur ON o.id = ur.org_id
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = $1
		ORDER BY o.name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	organizations := make([]UserOrgRoleRecord, 0)
	for rows.Next() {
		var org UserOrgRoleRecord
		if err := rows.Scan(&org.OrgID, &org.OrgName, &org.Role, &org.RoleID); err != nil {
			return nil, err
		}
		organizations = append(organizations, org)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return organizations, nil
}
