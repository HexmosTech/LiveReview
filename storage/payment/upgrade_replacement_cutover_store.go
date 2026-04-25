package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	UpgradeReplacementCutoverStatusPendingProvisioning      = "pending_provisioning"
	UpgradeReplacementCutoverStatusReplacementCreated       = "replacement_created"
	UpgradeReplacementCutoverStatusOldCancellationScheduled = "old_cancellation_scheduled"
	UpgradeReplacementCutoverStatusRetryPending             = "retry_pending"
	UpgradeReplacementCutoverStatusManualReviewRequired     = "manual_review_required"
	UpgradeReplacementCutoverStatusCompleted                = "completed"
)

var ErrUpgradeReplacementCutoverNotFound = errors.New("upgrade replacement cutover not found")

type UpgradeReplacementCutoverStore struct {
	db *sql.DB
}

type UpgradeReplacementCutover struct {
	ID                                int64
	UpgradeRequestID                  string
	OrgID                             int64
	OwnerUserID                       int64
	OldLocalSubscriptionID            int64
	OldRazorpaySubscriptionID         string
	ReplacementLocalSubscriptionID    sql.NullInt64
	ReplacementRazorpaySubscriptionID sql.NullString
	TargetPlanCode                    string
	TargetQuantity                    int
	Currency                          string
	CutoverAt                         time.Time
	OldCancellationScheduled          bool
	Status                            string
	RetryCount                        int
	NextRetryAt                       sql.NullTime
	LastError                         sql.NullString
	LastAttemptedAt                   sql.NullTime
	ResolvedAt                        sql.NullTime
	CreatedAt                         time.Time
	UpdatedAt                         time.Time
}

type CreateUpgradeReplacementCutoverInput struct {
	UpgradeRequestID          string
	OrgID                     int64
	OwnerUserID               int64
	OldLocalSubscriptionID    int64
	OldRazorpaySubscriptionID string
	TargetPlanCode            string
	TargetQuantity            int
	Currency                  string
	CutoverAt                 time.Time
}

type MarkReplacementProvisionedInput struct {
	UpgradeRequestID                  string
	ReplacementLocalSubscriptionID    int64
	ReplacementRazorpaySubscriptionID string
}

func NewUpgradeReplacementCutoverStore(db *sql.DB) *UpgradeReplacementCutoverStore {
	return &UpgradeReplacementCutoverStore{db: db}
}

func (s *UpgradeReplacementCutoverStore) CreateOrGetPending(ctx context.Context, input CreateUpgradeReplacementCutoverInput) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO upgrade_replacement_cutovers (
			upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, status, next_retry_at, last_attempted_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		ON CONFLICT (upgrade_request_id)
		DO UPDATE SET
			updated_at = NOW()
		RETURNING
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(input.UpgradeRequestID),
		input.OrgID,
		input.OwnerUserID,
		input.OldLocalSubscriptionID,
		strings.TrimSpace(input.OldRazorpaySubscriptionID),
		strings.TrimSpace(input.TargetPlanCode),
		input.TargetQuantity,
		strings.ToUpper(strings.TrimSpace(input.Currency)),
		input.CutoverAt.UTC(),
		UpgradeReplacementCutoverStatusPendingProvisioning,
	)

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		return UpgradeReplacementCutover{}, fmt.Errorf("insert upgrade replacement cutover: %w", err)
	}
	return cutover, nil
}

func (s *UpgradeReplacementCutoverStore) GetByUpgradeRequestID(ctx context.Context, upgradeRequestID string) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at
		FROM upgrade_replacement_cutovers
		WHERE upgrade_request_id = $1
		LIMIT 1`, strings.TrimSpace(upgradeRequestID))

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeReplacementCutover{}, ErrUpgradeReplacementCutoverNotFound
		}
		return UpgradeReplacementCutover{}, fmt.Errorf("query upgrade replacement cutover by request id: %w", err)
	}
	return cutover, nil
}

func (s *UpgradeReplacementCutoverStore) MarkReplacementProvisioned(ctx context.Context, input MarkReplacementProvisionedInput) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE upgrade_replacement_cutovers
		SET
			replacement_local_subscription_id = CASE WHEN $2 > 0 THEN $2 ELSE replacement_local_subscription_id END,
			replacement_razorpay_subscription_id = COALESCE(NULLIF($3, ''), replacement_razorpay_subscription_id),
			status = $4,
			last_error = NULL,
			last_attempted_at = NOW(),
			next_retry_at = NULL,
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(input.UpgradeRequestID),
		input.ReplacementLocalSubscriptionID,
		strings.TrimSpace(input.ReplacementRazorpaySubscriptionID),
		UpgradeReplacementCutoverStatusReplacementCreated,
	)

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeReplacementCutover{}, ErrUpgradeReplacementCutoverNotFound
		}
		return UpgradeReplacementCutover{}, fmt.Errorf("update replacement provisioned state: %w", err)
	}
	return cutover, nil
}

func (s *UpgradeReplacementCutoverStore) MarkOldCancellationScheduled(ctx context.Context, upgradeRequestID string) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE upgrade_replacement_cutovers
		SET
			old_cancellation_scheduled = TRUE,
			status = $2,
			last_error = NULL,
			last_attempted_at = NOW(),
			next_retry_at = NULL,
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(upgradeRequestID),
		UpgradeReplacementCutoverStatusOldCancellationScheduled,
	)

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeReplacementCutover{}, ErrUpgradeReplacementCutoverNotFound
		}
		return UpgradeReplacementCutover{}, fmt.Errorf("update old cancellation scheduled state: %w", err)
	}
	return cutover, nil
}

func (s *UpgradeReplacementCutoverStore) MarkRetryPending(ctx context.Context, upgradeRequestID string, failureReason string, nextRetryAt time.Time) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE upgrade_replacement_cutovers
		SET
			status = $2,
			retry_count = retry_count + 1,
			last_error = NULLIF($3, ''),
			last_attempted_at = NOW(),
			next_retry_at = $4,
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(upgradeRequestID),
		UpgradeReplacementCutoverStatusRetryPending,
		strings.TrimSpace(failureReason),
		nextRetryAt.UTC(),
	)

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeReplacementCutover{}, ErrUpgradeReplacementCutoverNotFound
		}
		return UpgradeReplacementCutover{}, fmt.Errorf("update retry pending state: %w", err)
	}
	return cutover, nil
}

func (s *UpgradeReplacementCutoverStore) MarkManualReviewRequired(ctx context.Context, upgradeRequestID string, failureReason string) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE upgrade_replacement_cutovers
		SET
			status = $2,
			last_error = NULLIF($3, ''),
			last_attempted_at = NOW(),
			next_retry_at = NULL,
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(upgradeRequestID),
		UpgradeReplacementCutoverStatusManualReviewRequired,
		strings.TrimSpace(failureReason),
	)

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeReplacementCutover{}, ErrUpgradeReplacementCutoverNotFound
		}
		return UpgradeReplacementCutover{}, fmt.Errorf("update manual review required state: %w", err)
	}
	return cutover, nil
}

func (s *UpgradeReplacementCutoverStore) MarkCompleted(ctx context.Context, upgradeRequestID string) (UpgradeReplacementCutover, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE upgrade_replacement_cutovers
		SET
			status = $2,
			last_error = NULL,
			last_attempted_at = NOW(),
			next_retry_at = NULL,
			resolved_at = NOW(),
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id,
			owner_user_id,
			old_local_subscription_id, old_razorpay_subscription_id,
			replacement_local_subscription_id, replacement_razorpay_subscription_id,
			target_plan_code, target_quantity, currency,
			cutover_at, old_cancellation_scheduled, status,
			retry_count, next_retry_at, last_error, last_attempted_at, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(upgradeRequestID),
		UpgradeReplacementCutoverStatusCompleted,
	)

	cutover, err := scanUpgradeReplacementCutover(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeReplacementCutover{}, ErrUpgradeReplacementCutoverNotFound
		}
		return UpgradeReplacementCutover{}, fmt.Errorf("update completed state: %w", err)
	}
	return cutover, nil
}

func scanUpgradeReplacementCutover(scanner interface {
	Scan(dest ...interface{}) error
}) (UpgradeReplacementCutover, error) {
	var row UpgradeReplacementCutover
	if err := scanner.Scan(
		&row.ID,
		&row.UpgradeRequestID,
		&row.OrgID,
		&row.OwnerUserID,
		&row.OldLocalSubscriptionID,
		&row.OldRazorpaySubscriptionID,
		&row.ReplacementLocalSubscriptionID,
		&row.ReplacementRazorpaySubscriptionID,
		&row.TargetPlanCode,
		&row.TargetQuantity,
		&row.Currency,
		&row.CutoverAt,
		&row.OldCancellationScheduled,
		&row.Status,
		&row.RetryCount,
		&row.NextRetryAt,
		&row.LastError,
		&row.LastAttemptedAt,
		&row.ResolvedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
	); err != nil {
		return UpgradeReplacementCutover{}, err
	}
	return row, nil
}
