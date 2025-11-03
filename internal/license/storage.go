package license

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Storage provides DB operations for the license state.
// We keep it minimal; can be extended later if historical audit is added.
type Storage struct {
	db *sql.DB
}

func NewStorage(db *sql.DB) *Storage { return &Storage{db: db} }

// GetLicenseState returns the singleton row or nil if not present.
func (s *Storage) GetLicenseState(ctx context.Context) (*LicenseState, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, token, kid, subject, app_name, seat_count, unlimited, issued_at, expires_at, last_validated_at, last_validation_error_code, validation_failures, status, grace_started_at, created_at, updated_at FROM license_state WHERE id=1`)

	var st LicenseState
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
func (s *Storage) UpsertLicenseState(ctx context.Context, st *LicenseState) error {
	if st == nil {
		return errors.New("nil LicenseState")
	}
	// Ensure ID is always 1
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
func (s *Storage) UpdateValidationResult(ctx context.Context, success bool, errorCode *string, newStatus string, lastValidated *sql.NullTime, failureCount int, graceStarted *sql.NullTime) error {
	// We update only relevant subset
	_, err := s.db.ExecContext(ctx, `UPDATE license_state SET
        last_validation_error_code = $1,
        last_validated_at = CASE WHEN $2 THEN now() ELSE last_validated_at END,
        validation_failures = $3,
        status = $4,
        grace_started_at = CASE WHEN $5 THEN $6 ELSE grace_started_at END,
        updated_at = now()
      WHERE id=1`,
		errorCode,
		success,
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
func (s *Storage) DeleteLicenseState(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM license_state WHERE id=1`)
	if err != nil {
		return fmt.Errorf("delete license_state: %w", err)
	}
	return nil
}
