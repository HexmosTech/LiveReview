package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	networkpayment "github.com/livereview/network/payment"
	storagepayment "github.com/livereview/storage/payment"
)

const maxBillingNotificationRetries = 6

func dispatchBillingNotificationOutboxBatch(ctx context.Context, db *sql.DB, limit int) error {
	if db == nil {
		return fmt.Errorf("missing db handle")
	}
	if limit <= 0 {
		limit = 50
	}

	store := storagepayment.NewBillingNotificationOutboxStore(db)
	items, err := store.ClaimDispatchBatch(ctx, limit)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	failedCount := 0
	for _, item := range items {
		if err := dispatchOneBillingNotification(ctx, store, item); err != nil {
			failedCount++
			log.Printf("[billing-notify-dispatch] id=%d org=%d channel=%s event=%s failed: %v", item.ID, item.OrgID, item.Channel, item.EventType, err)
		}
	}

	if failedCount > 0 {
		return fmt.Errorf("billing notification dispatch completed with %d failure(s)", failedCount)
	}

	return nil
}

func dispatchOneBillingNotification(ctx context.Context, store *storagepayment.BillingNotificationOutboxStore, item storagepayment.BillingNotificationOutboxItem) error {
	channel := strings.ToLower(strings.TrimSpace(item.Channel))
	switch channel {
	case "in_app":
		return store.MarkSent(ctx, item.ID)
	case "email":
		recipient := strings.TrimSpace(item.RecipientEmail.String)
		if recipient == "" {
			return store.MarkCancelled(ctx, item.ID, "missing recipient_email")
		}

		payload := item.Payload
		if len(payload) == 0 || !json.Valid(payload) {
			payload = json.RawMessage("{}")
		}

		err := networkpayment.SendBillingNotificationEmailPlaceholder(ctx, networkpayment.BillingEmailMessage{
			ToEmail:   recipient,
			OrgID:     item.OrgID,
			EventType: item.EventType,
			Payload:   payload,
		})
		if err == nil {
			return store.MarkSent(ctx, item.ID)
		}

		nextRetryCount := item.RetryCount + 1
		if nextRetryCount >= maxBillingNotificationRetries {
			return store.MarkCancelled(ctx, item.ID, fmt.Sprintf("email dispatch retry limit reached: %v", err))
		}
		return store.MarkFailed(ctx, item.ID, err.Error(), nextOutboxRetryTime(nextRetryCount))
	default:
		return store.MarkCancelled(ctx, item.ID, fmt.Sprintf("unsupported channel: %s", channel))
	}
}

func nextOutboxRetryTime(retryCount int) time.Time {
	if retryCount < 1 {
		retryCount = 1
	}

	exponent := retryCount - 1
	if exponent > 6 {
		exponent = 6
	}

	delay := time.Minute * time.Duration(1<<exponent)
	if delay > 2*time.Hour {
		delay = 2 * time.Hour
	}
	return time.Now().UTC().Add(delay)
}
