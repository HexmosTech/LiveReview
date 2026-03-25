package license

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	storagelicense "github.com/livereview/storage/license"
)

// Storage provides DB operations for the license state.
// We keep it minimal; can be extended later if historical audit is added.
type Storage struct {
	store *storagelicense.LicenseStateStore
}

func NewStorage(db *sql.DB) *Storage {
	return &Storage{store: storagelicense.NewLicenseStateStore(db)}
}

// GetLicenseState returns the singleton row or nil if not present.
func (s *Storage) GetLicenseState(ctx context.Context) (*LicenseState, error) {
	rec, err := s.store.GetLicenseState(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get license state from storage: %w", err)
	}
	if rec == nil {
		return nil, nil
	}
	return fromStorageRecord(rec), nil
}

// UpsertLicenseState inserts or updates the singleton license state.
func (s *Storage) UpsertLicenseState(ctx context.Context, st *LicenseState) error {
	if st == nil {
		return errors.New("nil LicenseState")
	}
	return s.store.UpsertLicenseState(ctx, toStorageRecord(st))
}

// UpdateValidationResult updates validation-related fields after an online attempt.
func (s *Storage) UpdateValidationResult(ctx context.Context, success bool, errorCode *string, newStatus string, lastValidated *sql.NullTime, failureCount int, graceStarted *sql.NullTime) error {
	return s.store.UpdateValidationResult(ctx, success, errorCode, newStatus, lastValidated, failureCount, graceStarted)
}

// DeleteLicenseState deletes the license state record.
func (s *Storage) DeleteLicenseState(ctx context.Context) error {
	return s.store.DeleteLicenseState(ctx)
}

func toStorageRecord(st *LicenseState) *storagelicense.LicenseStateRecord {
	if st == nil {
		return nil
	}
	return &storagelicense.LicenseStateRecord{
		ID:                    st.ID,
		Token:                 st.Token,
		Kid:                   st.Kid,
		Subject:               st.Subject,
		AppName:               st.AppName,
		SeatCount:             st.SeatCount,
		Unlimited:             st.Unlimited,
		IssuedAt:              st.IssuedAt,
		ExpiresAt:             st.ExpiresAt,
		LastValidatedAt:       st.LastValidatedAt,
		LastValidationErrCode: st.LastValidationErrCode,
		ValidationFailures:    st.ValidationFailures,
		Status:                st.Status,
		GraceStartedAt:        st.GraceStartedAt,
		CreatedAt:             st.CreatedAt,
		UpdatedAt:             st.UpdatedAt,
	}
}

func fromStorageRecord(rec *storagelicense.LicenseStateRecord) *LicenseState {
	if rec == nil {
		return nil
	}
	return &LicenseState{
		ID:                    rec.ID,
		Token:                 rec.Token,
		Kid:                   rec.Kid,
		Subject:               rec.Subject,
		AppName:               rec.AppName,
		SeatCount:             rec.SeatCount,
		Unlimited:             rec.Unlimited,
		IssuedAt:              rec.IssuedAt,
		ExpiresAt:             rec.ExpiresAt,
		LastValidatedAt:       rec.LastValidatedAt,
		LastValidationErrCode: rec.LastValidationErrCode,
		ValidationFailures:    rec.ValidationFailures,
		Status:                rec.Status,
		GraceStartedAt:        rec.GraceStartedAt,
		CreatedAt:             rec.CreatedAt,
		UpdatedAt:             rec.UpdatedAt,
	}
}
