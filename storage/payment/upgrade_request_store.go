package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	UpgradeRequestStatusCreated                     = "created"
	UpgradeRequestStatusPaymentOrderCreated         = "payment_order_created"
	UpgradeRequestStatusWaitingForCapture           = "waiting_for_capture"
	UpgradeRequestStatusPaymentCaptureConfirmed     = "payment_capture_confirmed"
	UpgradeRequestStatusSubscriptionUpdateRequested = "subscription_update_requested"
	UpgradeRequestStatusWaitingForSubscription      = "waiting_for_subscription_confirm"
	UpgradeRequestStatusSubscriptionConfirmed       = "subscription_change_confirmed"
	UpgradeRequestStatusReconciliationRetrying      = "reconciliation_retrying"
	UpgradeRequestStatusManualReviewRequired        = "manual_review_required"
	UpgradeRequestStatusResolved                    = "resolved"
	UpgradeRequestStatusFailed                      = "failed"
)

var ErrUpgradeRequestNotFound = errors.New("upgrade request not found")
var ErrUpgradeRequestTransitionRejected = errors.New("upgrade request transition rejected")

type UpgradeRequestStore struct {
	db *sql.DB
}

type UpgradeRequest struct {
	ID                            int64
	UpgradeRequestID              string
	OrgID                         int64
	ActorUserID                   int64
	FromPlanCode                  string
	ToPlanCode                    string
	ExpectedAmountCents           int64
	Currency                      string
	PreviewTokenSHA256            string
	RazorpayMode                  sql.NullString
	RazorpayOrderID               sql.NullString
	RazorpayPaymentID             sql.NullString
	LocalSubscriptionID           sql.NullInt64
	RazorpaySubscriptionID        sql.NullString
	TargetQuantity                sql.NullInt64
	PaymentCaptureConfirmed       bool
	PaymentCaptureConfirmedAt     sql.NullTime
	SubscriptionChangeConfirmed   bool
	SubscriptionChangeConfirmedAt sql.NullTime
	PlanGrantApplied              bool
	PlanGrantAppliedAt            sql.NullTime
	CurrentStatus                 string
	FailureReason                 sql.NullString
	ResolvedAt                    sql.NullTime
	CreatedAt                     time.Time
	UpdatedAt                     time.Time
}

type UpgradeRequestEvent struct {
	ID               int64
	UpgradeRequestID string
	OrgID            int64
	EventSource      string
	EventType        string
	FromStatus       sql.NullString
	ToStatus         sql.NullString
	EventPayload     json.RawMessage
	EventTime        time.Time
	CreatedAt        time.Time
}

type CreateUpgradeRequestInput struct {
	UpgradeRequestID    string
	OrgID               int64
	ActorUserID         int64
	FromPlanCode        string
	ToPlanCode          string
	ExpectedAmountCents int64
	Currency            string
	PreviewToken        string
}

type MarkUpgradeOrderPreparedInput struct {
	UpgradeRequestID string
	OrgID            int64
	RazorpayMode     string
	RazorpayOrderID  string
	AmountCents      int64
	Currency         string
	Metadata         map[string]interface{}
}

type MarkUpgradePaymentCaptureInput struct {
	UpgradeRequestID  string
	RazorpayPaymentID string
	RazorpayOrderID   string
	Metadata          map[string]interface{}
}

type MarkUpgradeSubscriptionUpdateInput struct {
	UpgradeRequestID       string
	LocalSubscriptionID    int64
	RazorpaySubscriptionID string
	TargetQuantity         int
	Metadata               map[string]interface{}
}

type MarkUpgradeSubscriptionConfirmedInput struct {
	UpgradeRequestID       string
	RazorpaySubscriptionID string
	Metadata               map[string]interface{}
}

type MarkUpgradeRequestFailedInput struct {
	UpgradeRequestID string
	FailureReason    string
	Metadata         map[string]interface{}
}

func NewUpgradeRequestStore(db *sql.DB) *UpgradeRequestStore {
	return &UpgradeRequestStore{db: db}
}

func isTerminalUpgradeRequestStatus(status string) bool {
	n := strings.TrimSpace(strings.ToLower(status))
	return n == UpgradeRequestStatusResolved || n == UpgradeRequestStatusFailed || n == UpgradeRequestStatusManualReviewRequired
}

func (s *UpgradeRequestStore) CreateUpgradeRequest(ctx context.Context, input CreateUpgradeRequestInput) (UpgradeRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("begin create upgrade request tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		INSERT INTO upgrade_requests (
			upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256, current_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at`,
		strings.TrimSpace(input.UpgradeRequestID),
		input.OrgID,
		input.ActorUserID,
		strings.TrimSpace(input.FromPlanCode),
		strings.TrimSpace(input.ToPlanCode),
		input.ExpectedAmountCents,
		strings.ToUpper(strings.TrimSpace(input.Currency)),
		HashUpgradePreviewToken(input.PreviewToken),
		UpgradeRequestStatusCreated,
	)

	request, err := scanUpgradeRequest(row)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("insert upgrade request: %w", err)
	}

	if err := s.insertEventTx(ctx, tx, request.UpgradeRequestID, request.OrgID, "api_preview", "request_created", "", request.CurrentStatus, map[string]interface{}{
		"from_plan_code": request.FromPlanCode,
		"to_plan_code":   request.ToPlanCode,
		"amount_cents":   request.ExpectedAmountCents,
		"currency":       request.Currency,
	}); err != nil {
		return UpgradeRequest{}, err
	}

	if err := tx.Commit(); err != nil {
		return UpgradeRequest{}, fmt.Errorf("commit create upgrade request tx: %w", err)
	}

	return request, nil
}

func (s *UpgradeRequestStore) GetUpgradeRequestByIDForOrg(ctx context.Context, orgID int64, upgradeRequestID string) (UpgradeRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE org_id = $1 AND upgrade_request_id = $2
		LIMIT 1`,
		orgID,
		strings.TrimSpace(upgradeRequestID),
	)

	request, err := scanUpgradeRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("query upgrade request by id and org: %w", err)
	}
	return request, nil
}

func (s *UpgradeRequestStore) GetUpgradeRequestByID(ctx context.Context, upgradeRequestID string) (UpgradeRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE upgrade_request_id = $1
		LIMIT 1`,
		strings.TrimSpace(upgradeRequestID),
	)

	request, err := scanUpgradeRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("query upgrade request by id: %w", err)
	}
	return request, nil
}

func (s *UpgradeRequestStore) GetUpgradeRequestByOrderID(ctx context.Context, razorpayOrderID string) (UpgradeRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE razorpay_order_id = $1
		LIMIT 1`,
		strings.TrimSpace(razorpayOrderID),
	)

	request, err := scanUpgradeRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("query upgrade request by order id: %w", err)
	}
	return request, nil
}

func (s *UpgradeRequestStore) GetLatestUpgradeRequestByOrg(ctx context.Context, orgID int64) (UpgradeRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT 1`,
		orgID,
	)

	request, err := scanUpgradeRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("query latest upgrade request by org: %w", err)
	}
	return request, nil
}

func (s *UpgradeRequestStore) MarkOrderPrepared(ctx context.Context, input MarkUpgradeOrderPreparedInput) (UpgradeRequest, error) {
	meta := input.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["razorpay_order_id"] = strings.TrimSpace(input.RazorpayOrderID)
	meta["amount_cents"] = input.AmountCents
	meta["currency"] = strings.ToUpper(strings.TrimSpace(input.Currency))
	meta["razorpay_mode"] = strings.TrimSpace(input.RazorpayMode)

	return s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(input.UpgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusCreated,
			UpgradeRequestStatusPaymentOrderCreated,
			UpgradeRequestStatusWaitingForCapture,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusFailed,
		},
		ToStatus: UpgradeRequestStatusWaitingForCapture,
		SetClauses: []string{
			"razorpay_mode = $%d",
			"razorpay_order_id = $%d",
			"expected_amount_cents = $%d",
			"currency = $%d",
			"failure_reason = NULL",
		},
		SetValues: []interface{}{
			strings.TrimSpace(input.RazorpayMode),
			strings.TrimSpace(input.RazorpayOrderID),
			input.AmountCents,
			strings.ToUpper(strings.TrimSpace(input.Currency)),
		},
		EventSource:  "api_prepare_payment",
		EventType:    "payment_order_created",
		EventPayload: meta,
	})
}

func (s *UpgradeRequestStore) MarkPaymentCaptureConfirmed(ctx context.Context, input MarkUpgradePaymentCaptureInput) (UpgradeRequest, error) {
	meta := input.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["razorpay_order_id"] = strings.TrimSpace(input.RazorpayOrderID)
	meta["razorpay_payment_id"] = strings.TrimSpace(input.RazorpayPaymentID)

	request, err := s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(input.UpgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusWaitingForCapture,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusSubscriptionUpdateRequested,
			UpgradeRequestStatusWaitingForSubscription,
			UpgradeRequestStatusSubscriptionConfirmed,
			UpgradeRequestStatusReconciliationRetrying,
		},
		ToStatus: UpgradeRequestStatusPaymentCaptureConfirmed,
		SetClauses: []string{
			"razorpay_order_id = COALESCE(NULLIF($%d, ''), razorpay_order_id)",
			"razorpay_payment_id = COALESCE(NULLIF($%d, ''), razorpay_payment_id)",
			"payment_capture_confirmed = TRUE",
			"payment_capture_confirmed_at = NOW()",
			"failure_reason = NULL",
		},
		SetValues: []interface{}{
			strings.TrimSpace(input.RazorpayOrderID),
			strings.TrimSpace(input.RazorpayPaymentID),
		},
		EventSource:  "webhook_payment_captured",
		EventType:    "payment_capture_confirmed",
		EventPayload: meta,
	})
	if err != nil {
		return UpgradeRequest{}, err
	}

	if request.SubscriptionChangeConfirmed {
		return s.ResolveUpgradeRequest(ctx, request.UpgradeRequestID, "auto_resolve_after_capture", map[string]interface{}{"reason": "both_confirmed"})
	}
	return request, nil
}

func (s *UpgradeRequestStore) MarkSubscriptionUpdateRequested(ctx context.Context, input MarkUpgradeSubscriptionUpdateInput) (UpgradeRequest, error) {
	meta := input.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["local_subscription_id"] = input.LocalSubscriptionID
	meta["razorpay_subscription_id"] = strings.TrimSpace(input.RazorpaySubscriptionID)
	meta["target_quantity"] = input.TargetQuantity

	return s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(input.UpgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusWaitingForCapture,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusSubscriptionUpdateRequested,
			UpgradeRequestStatusWaitingForSubscription,
			UpgradeRequestStatusReconciliationRetrying,
		},
		ToStatus: UpgradeRequestStatusWaitingForSubscription,
		SetClauses: []string{
			"local_subscription_id = CASE WHEN $%d > 0 THEN $%d ELSE local_subscription_id END",
			"razorpay_subscription_id = COALESCE(NULLIF($%d, ''), razorpay_subscription_id)",
			"target_quantity = CASE WHEN $%d > 0 THEN $%d ELSE target_quantity END",
			"failure_reason = NULL",
		},
		SetValues: []interface{}{
			input.LocalSubscriptionID,
			input.LocalSubscriptionID,
			strings.TrimSpace(input.RazorpaySubscriptionID),
			input.TargetQuantity,
			input.TargetQuantity,
		},
		EventSource:  "api_execute",
		EventType:    "subscription_update_requested",
		EventPayload: meta,
	})
}

func (s *UpgradeRequestStore) MarkSubscriptionChangeConfirmed(ctx context.Context, input MarkUpgradeSubscriptionConfirmedInput) (UpgradeRequest, error) {
	meta := input.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["razorpay_subscription_id"] = strings.TrimSpace(input.RazorpaySubscriptionID)

	request, err := s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(input.UpgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusWaitingForSubscription,
			UpgradeRequestStatusSubscriptionUpdateRequested,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusSubscriptionConfirmed,
			UpgradeRequestStatusReconciliationRetrying,
		},
		ToStatus: UpgradeRequestStatusSubscriptionConfirmed,
		SetClauses: []string{
			"subscription_change_confirmed = TRUE",
			"subscription_change_confirmed_at = NOW()",
			"razorpay_subscription_id = COALESCE(NULLIF($%d, ''), razorpay_subscription_id)",
			"failure_reason = NULL",
		},
		SetValues: []interface{}{
			strings.TrimSpace(input.RazorpaySubscriptionID),
		},
		EventSource:  "reconciliation_subscription",
		EventType:    "subscription_change_confirmed",
		EventPayload: meta,
	})
	if err != nil {
		return UpgradeRequest{}, err
	}

	if request.PaymentCaptureConfirmed {
		return s.ResolveUpgradeRequest(ctx, request.UpgradeRequestID, "auto_resolve_after_subscription_confirm", map[string]interface{}{"reason": "both_confirmed"})
	}
	return request, nil
}

func (s *UpgradeRequestStore) MarkReconciliationRetrying(ctx context.Context, upgradeRequestID string, metadata map[string]interface{}) (UpgradeRequest, error) {
	return s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(upgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusWaitingForCapture,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusWaitingForSubscription,
			UpgradeRequestStatusSubscriptionConfirmed,
			UpgradeRequestStatusReconciliationRetrying,
		},
		ToStatus:     UpgradeRequestStatusReconciliationRetrying,
		EventSource:  "reconciler",
		EventType:    "reconciliation_retrying",
		EventPayload: metadata,
	})
}

func (s *UpgradeRequestStore) MarkManualReviewRequired(ctx context.Context, upgradeRequestID string, failureReason string, metadata map[string]interface{}) (UpgradeRequest, error) {
	return s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(upgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusWaitingForCapture,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusWaitingForSubscription,
			UpgradeRequestStatusSubscriptionConfirmed,
			UpgradeRequestStatusReconciliationRetrying,
		},
		ToStatus:     UpgradeRequestStatusManualReviewRequired,
		SetClauses:   []string{"failure_reason = $%d"},
		SetValues:    []interface{}{strings.TrimSpace(failureReason)},
		EventSource:  "reconciler",
		EventType:    "manual_review_required",
		EventPayload: metadata,
	})
}

func (s *UpgradeRequestStore) MarkUpgradeRequestFailed(ctx context.Context, input MarkUpgradeRequestFailedInput) (UpgradeRequest, error) {
	meta := input.Metadata
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["failure_reason"] = strings.TrimSpace(input.FailureReason)

	return s.updateUpgradeRequestStatus(ctx, updateUpgradeRequestStatusInput{
		UpgradeRequestID: strings.TrimSpace(input.UpgradeRequestID),
		AllowedFrom: []string{
			UpgradeRequestStatusCreated,
			UpgradeRequestStatusPaymentOrderCreated,
			UpgradeRequestStatusWaitingForCapture,
			UpgradeRequestStatusPaymentCaptureConfirmed,
			UpgradeRequestStatusSubscriptionUpdateRequested,
			UpgradeRequestStatusWaitingForSubscription,
			UpgradeRequestStatusSubscriptionConfirmed,
			UpgradeRequestStatusReconciliationRetrying,
		},
		ToStatus:     UpgradeRequestStatusFailed,
		SetClauses:   []string{"failure_reason = $%d"},
		SetValues:    []interface{}{strings.TrimSpace(input.FailureReason)},
		EventSource:  "api_or_webhook",
		EventType:    "request_failed",
		EventPayload: meta,
	})
}

func (s *UpgradeRequestStore) ResolveUpgradeRequest(ctx context.Context, upgradeRequestID string, source string, metadata map[string]interface{}) (UpgradeRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("begin resolve upgrade request tx: %w", err)
	}
	defer tx.Rollback()

	currentRow := tx.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE upgrade_request_id = $1
		FOR UPDATE`,
		strings.TrimSpace(upgradeRequestID),
	)
	current, err := scanUpgradeRequest(currentRow)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("load upgrade request for resolve: %w", err)
	}

	if isTerminalUpgradeRequestStatus(current.CurrentStatus) {
		if strings.EqualFold(current.CurrentStatus, UpgradeRequestStatusResolved) {
			if err := tx.Commit(); err != nil {
				return UpgradeRequest{}, fmt.Errorf("commit resolve no-op tx: %w", err)
			}
			return current, nil
		}
		return UpgradeRequest{}, ErrUpgradeRequestTransitionRejected
	}

	if !current.PaymentCaptureConfirmed || !current.SubscriptionChangeConfirmed {
		return UpgradeRequest{}, ErrUpgradeRequestTransitionRejected
	}

	updatedRow := tx.QueryRowContext(ctx, `
		UPDATE upgrade_requests
		SET current_status = $2,
			resolved_at = NOW(),
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at`,
		current.UpgradeRequestID,
		UpgradeRequestStatusResolved,
	)
	updated, err := scanUpgradeRequest(updatedRow)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("update upgrade request to resolved: %w", err)
	}

	if err := s.insertEventTx(ctx, tx, updated.UpgradeRequestID, updated.OrgID, source, "request_resolved", current.CurrentStatus, updated.CurrentStatus, metadata); err != nil {
		return UpgradeRequest{}, err
	}

	if err := tx.Commit(); err != nil {
		return UpgradeRequest{}, fmt.Errorf("commit resolve upgrade request tx: %w", err)
	}

	return updated, nil
}

func (s *UpgradeRequestStore) MarkPlanGrantApplied(ctx context.Context, upgradeRequestID string, metadata map[string]interface{}) (UpgradeRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("begin mark plan grant applied tx: %w", err)
	}
	defer tx.Rollback()

	currentRow := tx.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE upgrade_request_id = $1
		FOR UPDATE`,
		strings.TrimSpace(upgradeRequestID),
	)
	current, err := scanUpgradeRequest(currentRow)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("load request for mark plan grant applied: %w", err)
	}

	if !strings.EqualFold(current.CurrentStatus, UpgradeRequestStatusResolved) {
		return UpgradeRequest{}, ErrUpgradeRequestTransitionRejected
	}

	if current.PlanGrantApplied {
		if err := tx.Commit(); err != nil {
			return UpgradeRequest{}, fmt.Errorf("commit mark plan grant already applied tx: %w", err)
		}
		return current, nil
	}

	updatedRow := tx.QueryRowContext(ctx, `
		UPDATE upgrade_requests
		SET plan_grant_applied = TRUE,
			plan_grant_applied_at = NOW(),
			updated_at = NOW()
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at`,
		current.UpgradeRequestID,
	)
	updated, err := scanUpgradeRequest(updatedRow)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("update request plan grant applied: %w", err)
	}

	if err := s.insertEventTx(ctx, tx, updated.UpgradeRequestID, updated.OrgID, "billing_apply", "plan_grant_applied", current.CurrentStatus, updated.CurrentStatus, metadata); err != nil {
		return UpgradeRequest{}, err
	}

	if err := tx.Commit(); err != nil {
		return UpgradeRequest{}, fmt.Errorf("commit mark plan grant applied tx: %w", err)
	}

	return updated, nil
}

func (s *UpgradeRequestStore) GetLatestPendingByRazorpaySubscriptionID(ctx context.Context, razorpaySubscriptionID string) (UpgradeRequest, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE razorpay_subscription_id = $1
		  AND current_status IN ($2, $3, $4, $5, $6)
		ORDER BY created_at DESC
		LIMIT 1`,
		strings.TrimSpace(razorpaySubscriptionID),
		UpgradeRequestStatusWaitingForCapture,
		UpgradeRequestStatusPaymentCaptureConfirmed,
		UpgradeRequestStatusWaitingForSubscription,
		UpgradeRequestStatusSubscriptionConfirmed,
		UpgradeRequestStatusReconciliationRetrying,
	)
	request, err := scanUpgradeRequest(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("query latest pending request by razorpay subscription id: %w", err)
	}
	return request, nil
}

func (s *UpgradeRequestStore) ListRequestsForReconciliation(ctx context.Context, limit int, staleBefore time.Time) ([]UpgradeRequest, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE (
			current_status IN ($1, $2, $3, $4, $5)
			OR (current_status = $6 AND plan_grant_applied = FALSE)
		)
		  AND updated_at <= $7
		ORDER BY updated_at ASC
		LIMIT $8`,
		UpgradeRequestStatusWaitingForCapture,
		UpgradeRequestStatusPaymentCaptureConfirmed,
		UpgradeRequestStatusWaitingForSubscription,
		UpgradeRequestStatusSubscriptionConfirmed,
		UpgradeRequestStatusReconciliationRetrying,
		UpgradeRequestStatusResolved,
		staleBefore.UTC(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list upgrade requests for reconciliation: %w", err)
	}
	defer rows.Close()

	out := make([]UpgradeRequest, 0)
	for rows.Next() {
		item, scanErr := scanUpgradeRequest(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan reconciliation request: %w", scanErr)
		}
		out = append(out, item)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate reconciliation requests: %w", rowsErr)
	}
	return out, nil
}

func (s *UpgradeRequestStore) ListUpgradeRequestEvents(ctx context.Context, upgradeRequestID string, limit int) ([]UpgradeRequestEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, event_source, event_type,
			from_status, to_status, event_payload, event_time, created_at
		FROM upgrade_request_events
		WHERE upgrade_request_id = $1
		ORDER BY event_time DESC
		LIMIT $2`,
		strings.TrimSpace(upgradeRequestID),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list upgrade request events: %w", err)
	}
	defer rows.Close()

	out := make([]UpgradeRequestEvent, 0)
	for rows.Next() {
		var item UpgradeRequestEvent
		var rawPayload []byte
		scanErr := rows.Scan(
			&item.ID,
			&item.UpgradeRequestID,
			&item.OrgID,
			&item.EventSource,
			&item.EventType,
			&item.FromStatus,
			&item.ToStatus,
			&rawPayload,
			&item.EventTime,
			&item.CreatedAt,
		)
		if scanErr != nil {
			return nil, fmt.Errorf("scan upgrade request event: %w", scanErr)
		}
		if rawPayload != nil {
			item.EventPayload = json.RawMessage(rawPayload)
		}
		out = append(out, item)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate upgrade request events: %w", rowsErr)
	}
	return out, nil
}

type updateUpgradeRequestStatusInput struct {
	UpgradeRequestID string
	AllowedFrom      []string
	ToStatus         string
	SetClauses       []string
	SetValues        []interface{}
	EventSource      string
	EventType        string
	EventPayload     map[string]interface{}
}

func (s *UpgradeRequestStore) updateUpgradeRequestStatus(ctx context.Context, input updateUpgradeRequestStatusInput) (UpgradeRequest, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("begin update upgrade request status tx: %w", err)
	}
	defer tx.Rollback()

	currentRow := tx.QueryRowContext(ctx, `
		SELECT
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at
		FROM upgrade_requests
		WHERE upgrade_request_id = $1
		FOR UPDATE`,
		input.UpgradeRequestID,
	)
	current, err := scanUpgradeRequest(currentRow)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpgradeRequest{}, ErrUpgradeRequestNotFound
		}
		return UpgradeRequest{}, fmt.Errorf("load current upgrade request status: %w", err)
	}

	allowed := false
	for _, from := range input.AllowedFrom {
		if strings.EqualFold(strings.TrimSpace(from), strings.TrimSpace(current.CurrentStatus)) {
			allowed = true
			break
		}
	}
	if !allowed {
		if strings.EqualFold(strings.TrimSpace(current.CurrentStatus), strings.TrimSpace(input.ToStatus)) {
			if err := tx.Commit(); err != nil {
				return UpgradeRequest{}, fmt.Errorf("commit no-op upgrade request status tx: %w", err)
			}
			return current, nil
		}
		return UpgradeRequest{}, ErrUpgradeRequestTransitionRejected
	}

	setParts := make([]string, 0, len(input.SetClauses)+2)
	values := make([]interface{}, 0, len(input.SetValues)+2)
	values = append(values, input.UpgradeRequestID)
	values = append(values, strings.TrimSpace(input.ToStatus))
	setParts = append(setParts, "current_status = $2")

	nextParam := 3
	for _, clause := range input.SetClauses {
		if strings.Contains(clause, "%d") {
			count := strings.Count(clause, "%d")
			if count == 1 {
				setParts = append(setParts, fmt.Sprintf(clause, nextParam))
				nextParam++
			} else if count == 2 {
				setParts = append(setParts, fmt.Sprintf(clause, nextParam, nextParam+1))
				nextParam += 2
			} else {
				return UpgradeRequest{}, fmt.Errorf("unsupported placeholder count in clause %q", clause)
			}
		} else {
			setParts = append(setParts, clause)
		}
	}
	values = append(values, input.SetValues...)
	setParts = append(setParts, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE upgrade_requests
		SET %s
		WHERE upgrade_request_id = $1
		RETURNING
			id, upgrade_request_id, org_id, actor_user_id,
			from_plan_code, to_plan_code,
			expected_amount_cents, currency,
			preview_token_sha256,
			razorpay_mode, razorpay_order_id, razorpay_payment_id,
			local_subscription_id, razorpay_subscription_id, target_quantity,
			payment_capture_confirmed, payment_capture_confirmed_at,
			subscription_change_confirmed, subscription_change_confirmed_at,
			plan_grant_applied, plan_grant_applied_at,
			current_status, failure_reason, resolved_at,
			created_at, updated_at`,
		strings.Join(setParts, ",\n\t\t\t"),
	)

	updatedRow := tx.QueryRowContext(ctx, query, values...)
	updated, err := scanUpgradeRequest(updatedRow)
	if err != nil {
		return UpgradeRequest{}, fmt.Errorf("update upgrade request status row: %w", err)
	}

	if err := s.insertEventTx(ctx, tx, updated.UpgradeRequestID, updated.OrgID, input.EventSource, input.EventType, current.CurrentStatus, updated.CurrentStatus, input.EventPayload); err != nil {
		return UpgradeRequest{}, err
	}

	if err := tx.Commit(); err != nil {
		return UpgradeRequest{}, fmt.Errorf("commit update upgrade request status tx: %w", err)
	}

	return updated, nil
}

func (s *UpgradeRequestStore) insertEventTx(ctx context.Context, tx *sql.Tx, upgradeRequestID string, orgID int64, source string, eventType string, fromStatus string, toStatus string, payload map[string]interface{}) error {
	var payloadJSON []byte
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal upgrade request event payload: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO upgrade_request_events (
			upgrade_request_id, org_id,
			event_source, event_type,
			from_status, to_status,
			event_payload, event_time
		) VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), $7, NOW())`,
		strings.TrimSpace(upgradeRequestID),
		orgID,
		strings.TrimSpace(source),
		strings.TrimSpace(eventType),
		strings.TrimSpace(fromStatus),
		strings.TrimSpace(toStatus),
		payloadJSON,
	)
	if err != nil {
		return fmt.Errorf("insert upgrade request event: %w", err)
	}
	return nil
}

func scanUpgradeRequest(row interface {
	Scan(dest ...interface{}) error
}) (UpgradeRequest, error) {
	var item UpgradeRequest
	err := row.Scan(
		&item.ID,
		&item.UpgradeRequestID,
		&item.OrgID,
		&item.ActorUserID,
		&item.FromPlanCode,
		&item.ToPlanCode,
		&item.ExpectedAmountCents,
		&item.Currency,
		&item.PreviewTokenSHA256,
		&item.RazorpayMode,
		&item.RazorpayOrderID,
		&item.RazorpayPaymentID,
		&item.LocalSubscriptionID,
		&item.RazorpaySubscriptionID,
		&item.TargetQuantity,
		&item.PaymentCaptureConfirmed,
		&item.PaymentCaptureConfirmedAt,
		&item.SubscriptionChangeConfirmed,
		&item.SubscriptionChangeConfirmedAt,
		&item.PlanGrantApplied,
		&item.PlanGrantAppliedAt,
		&item.CurrentStatus,
		&item.FailureReason,
		&item.ResolvedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return UpgradeRequest{}, err
	}
	return item, nil
}
