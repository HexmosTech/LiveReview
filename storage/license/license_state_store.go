package license

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LicenseStateRecord represents the persisted singleton row in license_state.
type LicenseStateRecord struct {
	ID                    int
	Token                 *string
	Kid                   *string
	Subject               *string
	AppName               *string
	SeatCount             *int
	Unlimited             bool
	IssuedAt              *time.Time
	ExpiresAt             *time.Time
	LastValidatedAt       *time.Time
	LastValidationErrCode *string
	ValidationFailures    int
	Status                string
	GraceStartedAt        *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// LicenseStateStore centralizes DB operations for license state persistence.
type LicenseStateStore struct {
	db *sql.DB
}

func NewLicenseStateStore(db *sql.DB) *LicenseStateStore {
	return &LicenseStateStore{db: db}
}

// GetLicenseState returns the singleton row or nil if not present.
func (s *LicenseStateStore) GetLicenseState(ctx context.Context) (*LicenseStateRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, token, kid, subject, app_name, seat_count, unlimited, issued_at, expires_at, last_validated_at, last_validation_error_code, validation_failures, status, grace_started_at, created_at, updated_at FROM license_state WHERE id=1`)

	var st LicenseStateRecord
	err := row.Scan(&st.ID, &st.Token, &st.Kid, &st.Subject, &st.AppName, &st.SeatCount, &st.Unlimited, &st.IssuedAt, &st.ExpiresAt, &st.LastValidatedAt, &st.LastValidationErrCode, &st.ValidationFailures, &st.Status, &st.GraceStartedAt, &st.CreatedAt, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query license_state: %w", err)
	}
	return &st, nil
}

// UpsertLicenseState inserts or updates the singleton license state.
func (s *LicenseStateStore) UpsertLicenseState(ctx context.Context, st *LicenseStateRecord) error {
	if st == nil {
		return errors.New("nil LicenseStateRecord")
	}

	st.ID = 1
	_, err := s.db.ExecContext(ctx, `INSERT INTO license_state (id, token, kid, subject, app_name, seat_count, unlimited, issued_at, expires_at, last_validated_at, last_validation_error_code, validation_failures, status, grace_started_at)
        VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
        ON CONFLICT (id) DO UPDATE SET
            token=EXCLUDED.token,
            kid=EXCLUDED.kid,
            subject=EXCLUDED.subject,
            app_name=EXCLUDED.app_name,
            seat_count=EXCLUDED.seat_count,
            unlimited=EXCLUDED.unlimited,
            issued_at=EXCLUDED.issued_at,
            expires_at=EXCLUDED.expires_at,
            last_validated_at=EXCLUDED.last_validated_at,
            last_validation_error_code=EXCLUDED.last_validation_error_code,
            validation_failures=EXCLUDED.validation_failures,
            status=EXCLUDED.status,
            grace_started_at=EXCLUDED.grace_started_at,
            updated_at=now();`,
		st.Token, st.Kid, st.Subject, st.AppName, st.SeatCount, st.Unlimited, st.IssuedAt, st.ExpiresAt, st.LastValidatedAt, st.LastValidationErrCode, st.ValidationFailures, st.Status, st.GraceStartedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert license_state: %w", err)
	}
	return nil
}

// UpdateValidationResult updates validation-related fields after an online attempt.
func (s *LicenseStateStore) UpdateValidationResult(ctx context.Context, success bool, errorCode *string, newStatus string, lastValidated *sql.NullTime, failureCount int, graceStarted *sql.NullTime) error {
	if graceStarted == nil {
		graceStarted = &sql.NullTime{Valid: false}
	}
	if lastValidated == nil {
		lastValidated = &sql.NullTime{Valid: false}
	}

	_, err := s.db.ExecContext(ctx, `UPDATE license_state SET
        last_validation_error_code = $1,
        last_validated_at = CASE WHEN $2 THEN COALESCE($3, now()) ELSE last_validated_at END,
        validation_failures = $4,
        status = $5,
        grace_started_at = CASE WHEN $6 THEN $7 ELSE grace_started_at END,
        updated_at = now()
      WHERE id=1`,
		errorCode,
		success,
		lastValidated,
		failureCount,
		newStatus,
		graceStarted.Valid,
		graceStarted,
	)
	if err != nil {
		return fmt.Errorf("update validation result: %w", err)
	}
	return nil
}

// DeleteLicenseState deletes the license state record.
func (s *LicenseStateStore) DeleteLicenseState(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM license_state WHERE id=1`)
	if err != nil {
		return fmt.Errorf("delete license_state: %w", err)
	}
	return nil
}
