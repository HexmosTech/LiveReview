package license

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrTrialEligibilityNotFound = errors.New("trial eligibility not found")
var ErrTrialEligibilityConsumed = errors.New("trial eligibility already consumed")
var ErrTrialEligibilityReserved = errors.New("trial eligibility currently reserved")
var ErrTrialEligibilityReservationMismatch = errors.New("trial eligibility reservation mismatch")

type TrialEligibilityStore struct {
	db *sql.DB
}

type TrialEligibilityState struct {
	ID                  int64
	NormalizedEmail     string
	FirstUserID         sql.NullInt64
	FirstOrgID          sql.NullInt64
	FirstSubscriptionID sql.NullInt64
	FirstPlanCode       sql.NullString
	ReservationToken    sql.NullString
	ReservationExpires  sql.NullTime
	Consumed            bool
	ConsumedAt          sql.NullTime
}

type ReserveFirstPurchaseTrialInput struct {
	Email            string
	ReservationToken string
	ReservationTTL   time.Duration
	ReservedUserID   *int64
	ReservedOrgID    *int64
	ReservedPlanCode string
}

type ConsumeReservedTrialInput struct {
	Email               string
	ReservationToken    string
	FirstUserID         *int64
	FirstOrgID          *int64
	FirstSubscriptionID *int64
	FirstPlanCode       string
	ConsumedAt          time.Time
}

type ReleaseTrialReservationInput struct {
	Email            string
	ReservationToken string
}

func NewTrialEligibilityStore(db *sql.DB) *TrialEligibilityStore {
	return &TrialEligibilityStore{db: db}
}

func NormalizeTrialEligibilityEmail(email string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(email))
	if normalized == "" {
		return "", fmt.Errorf("email is required")
	}
	return normalized, nil
}

func (s *TrialEligibilityStore) ReserveFirstPurchaseTrial(ctx context.Context, input ReserveFirstPurchaseTrialInput) (TrialEligibilityState, error) {
	if s == nil || s.db == nil {
		return TrialEligibilityState{}, fmt.Errorf("missing db handle")
	}
	normalizedEmail, err := NormalizeTrialEligibilityEmail(input.Email)
	if err != nil {
		return TrialEligibilityState{}, err
	}
	reservationToken := strings.TrimSpace(input.ReservationToken)
	if reservationToken == "" {
		return TrialEligibilityState{}, fmt.Errorf("reservation token is required")
	}

	ttl := input.ReservationTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return TrialEligibilityState{}, fmt.Errorf("begin reserve trial tx: %w", err)
	}
	defer tx.Rollback()

	state, err := queryTrialEligibilityForUpdate(ctx, tx, normalizedEmail)
	if err != nil && !errors.Is(err, ErrTrialEligibilityNotFound) {
		return TrialEligibilityState{}, err
	}

	if errors.Is(err, ErrTrialEligibilityNotFound) {
		state, err = insertTrialEligibilityReservation(ctx, tx, normalizedEmail, reservationToken, expiresAt, input)
		if err != nil {
			return TrialEligibilityState{}, err
		}
		if err := tx.Commit(); err != nil {
			return TrialEligibilityState{}, fmt.Errorf("commit reserve trial insert tx: %w", err)
		}
		return state, nil
	}

	if state.Consumed {
		return state, ErrTrialEligibilityConsumed
	}

	if state.ReservationToken.Valid && state.ReservationExpires.Valid && now.Before(state.ReservationExpires.Time.UTC()) {
		existingToken := strings.TrimSpace(state.ReservationToken.String)
		if existingToken != "" && existingToken != reservationToken {
			return state, ErrTrialEligibilityReserved
		}
	}

	state, err = updateTrialEligibilityReservation(ctx, tx, state.ID, reservationToken, expiresAt, input)
	if err != nil {
		return TrialEligibilityState{}, err
	}

	if err := tx.Commit(); err != nil {
		return TrialEligibilityState{}, fmt.Errorf("commit reserve trial update tx: %w", err)
	}
	return state, nil
}

func (s *TrialEligibilityStore) ConsumeReservedTrial(ctx context.Context, input ConsumeReservedTrialInput) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("missing db handle")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin consume trial tx: %w", err)
	}
	defer tx.Rollback()

	consumed, err := consumeReservedTrialTx(ctx, tx, input)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit consume trial tx: %w", err)
	}
	return consumed, nil
}

func (s *TrialEligibilityStore) ConsumeReservedTrialTx(ctx context.Context, tx *sql.Tx, input ConsumeReservedTrialInput) (bool, error) {
	if tx == nil {
		return false, fmt.Errorf("transaction is required")
	}
	return consumeReservedTrialTx(ctx, tx, input)
}

func consumeReservedTrialTx(ctx context.Context, tx *sql.Tx, input ConsumeReservedTrialInput) (bool, error) {
	normalizedEmail, err := NormalizeTrialEligibilityEmail(input.Email)
	if err != nil {
		return false, err
	}
	reservationToken := strings.TrimSpace(input.ReservationToken)
	if reservationToken == "" {
		return false, fmt.Errorf("reservation token is required")
	}

	state, err := queryTrialEligibilityForUpdate(ctx, tx, normalizedEmail)
	if err != nil {
		return false, err
	}

	if state.Consumed {
		return false, nil
	}

	if !state.ReservationToken.Valid || strings.TrimSpace(state.ReservationToken.String) == "" {
		return false, ErrTrialEligibilityReservationMismatch
	}
	if strings.TrimSpace(state.ReservationToken.String) != reservationToken {
		return false, ErrTrialEligibilityReservationMismatch
	}

	consumedAt := input.ConsumedAt.UTC()
	if consumedAt.IsZero() {
		consumedAt = time.Now().UTC()
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE trial_eligibility
		SET consumed = TRUE,
		    consumed_at = $2,
		    first_user_id = COALESCE(first_user_id, $3),
		    first_org_id = COALESCE(first_org_id, $4),
		    first_subscription_id = COALESCE(first_subscription_id, $5),
		    first_plan_code = COALESCE(NULLIF(first_plan_code, ''), NULLIF($6, '')),
		    reservation_token = NULL,
		    reservation_expires_at = NULL,
		    reserved_user_id = NULL,
		    reserved_org_id = NULL,
		    reserved_plan_code = NULL,
		    updated_at = NOW()
		WHERE id = $1`,
		state.ID,
		consumedAt,
		nullInt64Ptr(input.FirstUserID),
		nullInt64Ptr(input.FirstOrgID),
		nullInt64Ptr(input.FirstSubscriptionID),
		strings.TrimSpace(input.FirstPlanCode),
	)
	if err != nil {
		return false, fmt.Errorf("update trial eligibility consume state: %w", err)
	}

	return true, nil
}

func (s *TrialEligibilityStore) ReleaseTrialReservation(ctx context.Context, input ReleaseTrialReservationInput) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing db handle")
	}
	normalizedEmail, err := NormalizeTrialEligibilityEmail(input.Email)
	if err != nil {
		return err
	}
	reservationToken := strings.TrimSpace(input.ReservationToken)
	if reservationToken == "" {
		return fmt.Errorf("reservation token is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin release trial reservation tx: %w", err)
	}
	defer tx.Rollback()

	state, err := queryTrialEligibilityForUpdate(ctx, tx, normalizedEmail)
	if err != nil {
		if errors.Is(err, ErrTrialEligibilityNotFound) {
			return nil
		}
		return err
	}
	if state.Consumed {
		return nil
	}
	if !state.ReservationToken.Valid || strings.TrimSpace(state.ReservationToken.String) == "" {
		return nil
	}
	if strings.TrimSpace(state.ReservationToken.String) != reservationToken {
		return ErrTrialEligibilityReservationMismatch
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE trial_eligibility
		SET reservation_token = NULL,
		    reservation_expires_at = NULL,
		    reserved_user_id = NULL,
		    reserved_org_id = NULL,
		    reserved_plan_code = NULL,
		    updated_at = NOW()
		WHERE id = $1`,
		state.ID,
	)
	if err != nil {
		return fmt.Errorf("clear trial reservation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit release trial reservation tx: %w", err)
	}
	return nil
}

func queryTrialEligibilityForUpdate(ctx context.Context, tx *sql.Tx, normalizedEmail string) (TrialEligibilityState, error) {
	state, err := scanTrialEligibility(tx.QueryRowContext(ctx, `
		SELECT id,
		       normalized_email,
		       first_user_id,
		       first_org_id,
		       first_subscription_id,
		       first_plan_code,
		       reservation_token,
		       reservation_expires_at,
		       consumed,
		       consumed_at
		FROM trial_eligibility
		WHERE normalized_email = $1
		FOR UPDATE`, normalizedEmail))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TrialEligibilityState{}, ErrTrialEligibilityNotFound
		}
		return TrialEligibilityState{}, fmt.Errorf("query trial eligibility: %w", err)
	}
	return state, nil
}

func insertTrialEligibilityReservation(ctx context.Context, tx *sql.Tx, normalizedEmail, reservationToken string, expiresAt time.Time, input ReserveFirstPurchaseTrialInput) (TrialEligibilityState, error) {
	state, err := scanTrialEligibility(tx.QueryRowContext(ctx, `
		INSERT INTO trial_eligibility (
			normalized_email,
			reservation_token,
			reservation_expires_at,
			reserved_user_id,
			reserved_org_id,
			reserved_plan_code,
			consumed
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), FALSE)
		RETURNING id,
		          normalized_email,
		          first_user_id,
		          first_org_id,
		          first_subscription_id,
		          first_plan_code,
		          reservation_token,
		          reservation_expires_at,
		          consumed,
		          consumed_at`,
		normalizedEmail,
		reservationToken,
		expiresAt,
		nullInt64Ptr(input.ReservedUserID),
		nullInt64Ptr(input.ReservedOrgID),
		strings.TrimSpace(input.ReservedPlanCode),
	))
	if err != nil {
		return TrialEligibilityState{}, fmt.Errorf("insert trial reservation: %w", err)
	}
	return state, nil
}

func updateTrialEligibilityReservation(ctx context.Context, tx *sql.Tx, id int64, reservationToken string, expiresAt time.Time, input ReserveFirstPurchaseTrialInput) (TrialEligibilityState, error) {
	state, err := scanTrialEligibility(tx.QueryRowContext(ctx, `
		UPDATE trial_eligibility
		SET reservation_token = $2,
		    reservation_expires_at = $3,
		    reserved_user_id = $4,
		    reserved_org_id = $5,
		    reserved_plan_code = NULLIF($6, ''),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id,
		          normalized_email,
		          first_user_id,
		          first_org_id,
		          first_subscription_id,
		          first_plan_code,
		          reservation_token,
		          reservation_expires_at,
		          consumed,
		          consumed_at`,
		id,
		reservationToken,
		expiresAt,
		nullInt64Ptr(input.ReservedUserID),
		nullInt64Ptr(input.ReservedOrgID),
		strings.TrimSpace(input.ReservedPlanCode),
	))
	if err != nil {
		return TrialEligibilityState{}, fmt.Errorf("update trial reservation: %w", err)
	}
	return state, nil
}

func scanTrialEligibility(row *sql.Row) (TrialEligibilityState, error) {
	var state TrialEligibilityState
	err := row.Scan(
		&state.ID,
		&state.NormalizedEmail,
		&state.FirstUserID,
		&state.FirstOrgID,
		&state.FirstSubscriptionID,
		&state.FirstPlanCode,
		&state.ReservationToken,
		&state.ReservationExpires,
		&state.Consumed,
		&state.ConsumedAt,
	)
	if err != nil {
		return TrialEligibilityState{}, err
	}
	return state, nil
}

func nullInt64Ptr(v *int64) interface{} {
	if v == nil {
		return nil
	}
	return *v
}
