package payment

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type BillingNotificationOutboxStore struct {
	db *sql.DB
}

type CreateBillingNotificationInput struct {
	OrgID           int64
	EventType       string
	Channel         string
	DedupeKey       string
	Payload         map[string]interface{}
	RecipientUserID *int64
	RecipientEmail  string
	SendAfter       *time.Time
}

type BillingNotificationOutboxItem struct {
	ID              int64
	OrgID           int64
	EventType       string
	Channel         string
	DedupeKey       string
	Payload         json.RawMessage
	RecipientUserID sql.NullInt64
	RecipientEmail  sql.NullString
	Status          string
	RetryCount      int
	LastError       sql.NullString
	SendAfter       time.Time
	SentAt          sql.NullTime
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func NewBillingNotificationOutboxStore(db *sql.DB) *BillingNotificationOutboxStore {
	return &BillingNotificationOutboxStore{db: db}
}

func (s *BillingNotificationOutboxStore) Enqueue(ctx context.Context, input CreateBillingNotificationInput) (bool, error) {
	payload := input.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payloadRaw, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal billing notification payload: %w", err)
	}

	result, err := s.db.ExecContext(ctx, `
		INSERT INTO billing_notification_outbox (
			org_id,
			event_type,
			channel,
			dedupe_key,
			payload,
			recipient_user_id,
			recipient_email,
			send_after,
			status,
			created_at,
			updated_at
		) VALUES (
			$1,
			$2,
			$3,
			$4,
			$5::jsonb,
			$6,
			NULLIF($7, ''),
			COALESCE($8, NOW()),
			'pending',
			NOW(),
			NOW()
		)
		ON CONFLICT (channel, dedupe_key) DO NOTHING
	`,
		input.OrgID,
		strings.TrimSpace(input.EventType),
		strings.TrimSpace(input.Channel),
		strings.TrimSpace(input.DedupeKey),
		string(payloadRaw),
		nullInt64Ptr(input.RecipientUserID),
		strings.TrimSpace(input.RecipientEmail),
		input.SendAfter,
	)
	if err != nil {
		return false, fmt.Errorf("enqueue billing notification outbox: %w", err)
	}

	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (s *BillingNotificationOutboxStore) GetUserEmailByID(ctx context.Context, userID int64) (string, error) {
	if userID <= 0 {
		return "", nil
	}

	var email sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("get user email by id: %w", err)
	}
	if !email.Valid {
		return "", nil
	}
	return strings.TrimSpace(email.String), nil
}

func (s *BillingNotificationOutboxStore) ClaimDispatchBatch(ctx context.Context, limit int) ([]BillingNotificationOutboxItem, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH candidates AS (
			SELECT id
			FROM billing_notification_outbox
			WHERE status IN ('pending', 'failed')
			  AND send_after <= NOW()
			ORDER BY send_after ASC, created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE billing_notification_outbox b
		SET status = 'processing',
			updated_at = NOW()
		FROM candidates c
		WHERE b.id = c.id
		RETURNING
			b.id,
			b.org_id,
			b.event_type,
			b.channel,
			b.dedupe_key,
			b.payload,
			b.recipient_user_id,
			b.recipient_email,
			b.status,
			b.retry_count,
			b.last_error,
			b.send_after,
			b.sent_at,
			b.created_at,
			b.updated_at
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim billing notification dispatch batch: %w", err)
	}
	defer rows.Close()

	items := make([]BillingNotificationOutboxItem, 0, limit)
	for rows.Next() {
		var item BillingNotificationOutboxItem
		var payloadRaw []byte
		if err := rows.Scan(
			&item.ID,
			&item.OrgID,
			&item.EventType,
			&item.Channel,
			&item.DedupeKey,
			&payloadRaw,
			&item.RecipientUserID,
			&item.RecipientEmail,
			&item.Status,
			&item.RetryCount,
			&item.LastError,
			&item.SendAfter,
			&item.SentAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan billing notification dispatch row: %w", err)
		}
		item.Payload = json.RawMessage(payloadRaw)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate billing notification dispatch rows: %w", err)
	}

	return items, nil
}

func (s *BillingNotificationOutboxStore) MarkSent(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("notification id must be > 0")
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE billing_notification_outbox
		SET status = 'sent',
			sent_at = NOW(),
			last_error = NULL,
			updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return fmt.Errorf("mark billing notification sent: %w", err)
	}

	return nil
}

func (s *BillingNotificationOutboxStore) MarkFailed(ctx context.Context, id int64, lastError string, nextAttemptAt time.Time) error {
	if id <= 0 {
		return fmt.Errorf("notification id must be > 0")
	}
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().UTC().Add(5 * time.Minute)
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE billing_notification_outbox
		SET status = 'failed',
			retry_count = retry_count + 1,
			last_error = NULLIF($2, ''),
			send_after = $3,
			updated_at = NOW()
		WHERE id = $1
	`, id, strings.TrimSpace(lastError), nextAttemptAt.UTC())
	if err != nil {
		return fmt.Errorf("mark billing notification failed: %w", err)
	}

	return nil
}

func (s *BillingNotificationOutboxStore) MarkCancelled(ctx context.Context, id int64, reason string) error {
	if id <= 0 {
		return fmt.Errorf("notification id must be > 0")
	}

	_, err := s.db.ExecContext(ctx, `
		UPDATE billing_notification_outbox
		SET status = 'cancelled',
			last_error = NULLIF($2, ''),
			updated_at = NOW()
		WHERE id = $1
	`, id, strings.TrimSpace(reason))
	if err != nil {
		return fmt.Errorf("mark billing notification cancelled: %w", err)
	}

	return nil
}

func nullInt64Ptr(v *int64) interface{} {
	if v == nil || *v <= 0 {
		return nil
	}
	return *v
}
