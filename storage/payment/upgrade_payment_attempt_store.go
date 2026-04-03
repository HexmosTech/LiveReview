package payment

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrUpgradePaymentAttemptNotFound = errors.New("upgrade payment attempt not found")
var ErrUpgradePaymentAttemptIdempotencyMismatch = errors.New("upgrade payment attempt idempotency key mismatch")

type UpgradePaymentAttemptStore struct {
	db *sql.DB
}

type UpgradePaymentAttempt struct {
	ID                    int64
	OrgID                 int64
	UpgradeRequestID      sql.NullString
	PreviewTokenSHA256    string
	FromPlanCode          string
	ToPlanCode            string
	AmountCents           int64
	Currency              string
	RazorpayMode          string
	RazorpayOrderID       string
	RazorpayPaymentID     sql.NullString
	Status                string
	ExecuteIdempotencyKey sql.NullString
	ExecuteResponse       json.RawMessage
	ErrorCode             sql.NullString
	ErrorReason           sql.NullString
	ErrorDescription      sql.NullString
	ErrorSource           sql.NullString
	ErrorStep             sql.NullString
	PreparedAt            time.Time
	PaymentFailedAt       sql.NullTime
	PaymentCapturedAt     sql.NullTime
	ExecutedAt            sql.NullTime
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type CreateUpgradePaymentAttemptInput struct {
	OrgID            int64
	UpgradeRequestID string
	PreviewToken     string
	FromPlanCode     string
	ToPlanCode       string
	AmountCents      int64
	Currency         string
	RazorpayMode     string
	RazorpayOrderID  string
}

type MarkUpgradePaymentFailedInput struct {
	RazorpayOrderID   string
	RazorpayPaymentID string
	ErrorCode         string
	ErrorReason       string
	ErrorDescription  string
	ErrorSource       string
	ErrorStep         string
}

type ReserveUpgradeExecuteInput struct {
	OrgID                 int64
	UpgradeRequestID      string
	PreviewToken          string
	RazorpayOrderID       string
	RazorpayPaymentID     string
	ExecuteIdempotencyKey string
}

type MarkUpgradeExecuteAppliedInput struct {
	RazorpayOrderID       string
	RazorpayPaymentID     string
	ExecuteIdempotencyKey string
	ExecuteResponse       map[string]interface{}
}

func NewUpgradePaymentAttemptStore(db *sql.DB) *UpgradePaymentAttemptStore {
	return &UpgradePaymentAttemptStore{db: db}
}

func HashUpgradePreviewToken(previewToken string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(previewToken)))
	return hex.EncodeToString(sum[:])
}

func (s *UpgradePaymentAttemptStore) CreateUpgradePaymentAttempt(ctx context.Context, input CreateUpgradePaymentAttemptInput) (UpgradePaymentAttempt, error) {
	query := `
		INSERT INTO upgrade_payment_attempts (
			org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, status
		) VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, 'prepared')
		RETURNING
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at`

	row := s.db.QueryRowContext(
		ctx,
		query,
		input.OrgID,
		strings.TrimSpace(input.UpgradeRequestID),
		HashUpgradePreviewToken(input.PreviewToken),
		strings.TrimSpace(input.FromPlanCode),
		strings.TrimSpace(input.ToPlanCode),
		input.AmountCents,
		strings.ToUpper(strings.TrimSpace(input.Currency)),
		strings.TrimSpace(input.RazorpayMode),
		strings.TrimSpace(input.RazorpayOrderID),
	)

	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		return UpgradePaymentAttempt{}, fmt.Errorf("insert upgrade payment attempt: %w", err)
	}
	return attempt, nil
}

func (s *UpgradePaymentAttemptStore) GetReusablePreparedAttempt(ctx context.Context, orgID int64, previewToken string) (UpgradePaymentAttempt, error) {
	query := `
		SELECT
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at
		FROM upgrade_payment_attempts
		WHERE org_id = $1
		  AND preview_token_sha256 = $2
		  AND status IN ('prepared', 'payment_failed', 'payment_captured')
		ORDER BY created_at DESC
		LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, orgID, HashUpgradePreviewToken(previewToken))
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, fmt.Errorf("query reusable prepared attempt: %w", err)
	}
	return attempt, nil
}

func (s *UpgradePaymentAttemptStore) GetAttemptByOrgPreviewAndOrder(ctx context.Context, orgID int64, previewToken string, razorpayOrderID string) (UpgradePaymentAttempt, error) {
	query := `
		SELECT
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at
		FROM upgrade_payment_attempts
		WHERE org_id = $1
		  AND preview_token_sha256 = $2
		  AND razorpay_order_id = $3
		LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, orgID, HashUpgradePreviewToken(previewToken), strings.TrimSpace(razorpayOrderID))
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, fmt.Errorf("query upgrade payment attempt: %w", err)
	}
	return attempt, nil
}

func (s *UpgradePaymentAttemptStore) GetAttemptByOrderID(ctx context.Context, razorpayOrderID string) (UpgradePaymentAttempt, error) {
	query := `
		SELECT
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at
		FROM upgrade_payment_attempts
		WHERE razorpay_order_id = $1
		LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, strings.TrimSpace(razorpayOrderID))
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, fmt.Errorf("query upgrade payment attempt by order: %w", err)
	}
	return attempt, nil
}

func (s *UpgradePaymentAttemptStore) GetLatestAttemptByUpgradeRequestID(ctx context.Context, upgradeRequestID string) (UpgradePaymentAttempt, error) {
	query := `
		SELECT
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at
		FROM upgrade_payment_attempts
		WHERE upgrade_request_id = $1
		ORDER BY created_at DESC
		LIMIT 1`

	row := s.db.QueryRowContext(ctx, query, strings.TrimSpace(upgradeRequestID))
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, fmt.Errorf("query latest upgrade payment attempt by request id: %w", err)
	}
	return attempt, nil
}

func (s *UpgradePaymentAttemptStore) GetAttemptByOrgRequestAndOrder(ctx context.Context, orgID int64, upgradeRequestID string, razorpayOrderID string) (UpgradePaymentAttempt, error) {
	query := `
		SELECT
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at
		FROM upgrade_payment_attempts
		WHERE org_id = $1
		  AND upgrade_request_id = $2
		  AND razorpay_order_id = $3
		LIMIT 1`

	row := s.db.QueryRowContext(
		ctx,
		query,
		orgID,
		strings.TrimSpace(upgradeRequestID),
		strings.TrimSpace(razorpayOrderID),
	)
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, fmt.Errorf("query upgrade payment attempt by org, request, and order: %w", err)
	}
	return attempt, nil
}

func (s *UpgradePaymentAttemptStore) MarkPaymentCapturedByOrderID(ctx context.Context, razorpayOrderID string, razorpayPaymentID string) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE upgrade_payment_attempts
		 SET status = 'payment_captured',
		     razorpay_payment_id = COALESCE(NULLIF($2, ''), razorpay_payment_id),
		     payment_captured_at = NOW(),
		     updated_at = NOW()
		 WHERE razorpay_order_id = $1`,
		strings.TrimSpace(razorpayOrderID),
		strings.TrimSpace(razorpayPaymentID),
	)
	if err != nil {
		return fmt.Errorf("mark upgrade payment captured: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUpgradePaymentAttemptNotFound
	}
	return nil
}

func (s *UpgradePaymentAttemptStore) MarkPaymentFailedByOrderID(ctx context.Context, input MarkUpgradePaymentFailedInput) error {
	result, err := s.db.ExecContext(
		ctx,
		`UPDATE upgrade_payment_attempts
		 SET status = 'payment_failed',
		     razorpay_payment_id = COALESCE(NULLIF($2, ''), razorpay_payment_id),
		     error_code = NULLIF($3, ''),
		     error_reason = NULLIF($4, ''),
		     error_description = NULLIF($5, ''),
		     error_source = NULLIF($6, ''),
		     error_step = NULLIF($7, ''),
		     payment_failed_at = NOW(),
		     updated_at = NOW()
		 WHERE razorpay_order_id = $1`,
		strings.TrimSpace(input.RazorpayOrderID),
		strings.TrimSpace(input.RazorpayPaymentID),
		strings.TrimSpace(input.ErrorCode),
		strings.TrimSpace(input.ErrorReason),
		strings.TrimSpace(input.ErrorDescription),
		strings.TrimSpace(input.ErrorSource),
		strings.TrimSpace(input.ErrorStep),
	)
	if err != nil {
		return fmt.Errorf("mark upgrade payment failed: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrUpgradePaymentAttemptNotFound
	}
	return nil
}

func (s *UpgradePaymentAttemptStore) ReserveExecute(ctx context.Context, input ReserveUpgradeExecuteInput) (UpgradePaymentAttempt, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpgradePaymentAttempt{}, false, fmt.Errorf("begin tx for reserve execute: %w", err)
	}
	defer tx.Rollback()

	query := `
		SELECT
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at
		FROM upgrade_payment_attempts
		WHERE org_id = $1
		  AND preview_token_sha256 = $2
		  AND razorpay_order_id = $3
		  AND upgrade_request_id = $4
		FOR UPDATE`
	row := tx.QueryRowContext(
		ctx,
		query,
		input.OrgID,
		HashUpgradePreviewToken(input.PreviewToken),
		strings.TrimSpace(input.RazorpayOrderID),
		strings.TrimSpace(input.UpgradeRequestID),
	)
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, false, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, false, fmt.Errorf("load attempt for reserve execute: %w", err)
	}

	key := strings.TrimSpace(input.ExecuteIdempotencyKey)
	if key == "" {
		return UpgradePaymentAttempt{}, false, fmt.Errorf("execute idempotency key is required")
	}

	existingKey := ""
	if attempt.ExecuteIdempotencyKey.Valid {
		existingKey = strings.TrimSpace(attempt.ExecuteIdempotencyKey.String)
	}

	if strings.EqualFold(strings.TrimSpace(attempt.Status), "execute_applied") {
		if existingKey == key {
			if err := tx.Commit(); err != nil {
				return UpgradePaymentAttempt{}, false, fmt.Errorf("commit reserve execute tx: %w", err)
			}
			return attempt, true, nil
		}
		return UpgradePaymentAttempt{}, false, ErrUpgradePaymentAttemptIdempotencyMismatch
	}

	if existingKey != "" && existingKey != key {
		return UpgradePaymentAttempt{}, false, ErrUpgradePaymentAttemptIdempotencyMismatch
	}

	updateRow := tx.QueryRowContext(
		ctx,
		`UPDATE upgrade_payment_attempts
		 SET execute_idempotency_key = $2,
		     razorpay_payment_id = COALESCE(NULLIF($3, ''), razorpay_payment_id),
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at`,
		attempt.ID,
		key,
		strings.TrimSpace(input.RazorpayPaymentID),
	)
	updatedAttempt, err := scanUpgradePaymentAttempt(updateRow)
	if err != nil {
		return UpgradePaymentAttempt{}, false, fmt.Errorf("reserve execute key: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return UpgradePaymentAttempt{}, false, fmt.Errorf("commit reserve execute tx: %w", err)
	}

	return updatedAttempt, false, nil
}

func (s *UpgradePaymentAttemptStore) MarkExecuteApplied(ctx context.Context, input MarkUpgradeExecuteAppliedInput) (UpgradePaymentAttempt, error) {
	rawResponse, err := json.Marshal(input.ExecuteResponse)
	if err != nil {
		return UpgradePaymentAttempt{}, fmt.Errorf("marshal execute response: %w", err)
	}

	query := `
		UPDATE upgrade_payment_attempts
		SET status = 'execute_applied',
		    razorpay_payment_id = COALESCE(NULLIF($2, ''), razorpay_payment_id),
		    execute_idempotency_key = $3,
		    execute_response = $4::jsonb,
		    executed_at = NOW(),
		    payment_captured_at = COALESCE(payment_captured_at, NOW()),
		    updated_at = NOW()
		WHERE razorpay_order_id = $1
		RETURNING
			id, org_id, upgrade_request_id, preview_token_sha256, from_plan_code, to_plan_code,
			amount_cents, currency, razorpay_mode, razorpay_order_id, razorpay_payment_id,
			status, execute_idempotency_key, execute_response,
			error_code, error_reason, error_description, error_source, error_step,
			prepared_at, payment_failed_at, payment_captured_at, executed_at,
			created_at, updated_at`

	row := s.db.QueryRowContext(
		ctx,
		query,
		strings.TrimSpace(input.RazorpayOrderID),
		strings.TrimSpace(input.RazorpayPaymentID),
		strings.TrimSpace(input.ExecuteIdempotencyKey),
		string(rawResponse),
	)
	attempt, err := scanUpgradePaymentAttempt(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradePaymentAttempt{}, ErrUpgradePaymentAttemptNotFound
		}
		return UpgradePaymentAttempt{}, fmt.Errorf("mark execute applied: %w", err)
	}
	return attempt, nil
}

func DecodeUpgradeExecuteResponse(raw json.RawMessage) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func scanUpgradePaymentAttempt(row interface {
	Scan(dest ...interface{}) error
}) (UpgradePaymentAttempt, error) {
	var attempt UpgradePaymentAttempt
	var executeResponseRaw []byte
	err := row.Scan(
		&attempt.ID,
		&attempt.OrgID,
		&attempt.UpgradeRequestID,
		&attempt.PreviewTokenSHA256,
		&attempt.FromPlanCode,
		&attempt.ToPlanCode,
		&attempt.AmountCents,
		&attempt.Currency,
		&attempt.RazorpayMode,
		&attempt.RazorpayOrderID,
		&attempt.RazorpayPaymentID,
		&attempt.Status,
		&attempt.ExecuteIdempotencyKey,
		&executeResponseRaw,
		&attempt.ErrorCode,
		&attempt.ErrorReason,
		&attempt.ErrorDescription,
		&attempt.ErrorSource,
		&attempt.ErrorStep,
		&attempt.PreparedAt,
		&attempt.PaymentFailedAt,
		&attempt.PaymentCapturedAt,
		&attempt.ExecutedAt,
		&attempt.CreatedAt,
		&attempt.UpdatedAt,
	)
	if err != nil {
		return UpgradePaymentAttempt{}, err
	}
	if executeResponseRaw != nil {
		attempt.ExecuteResponse = json.RawMessage(executeResponseRaw)
	}
	return attempt, nil
}
