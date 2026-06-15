package reviewprocessor

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// WebhookReviewHandler defines the signature for processing a webhook event asynchronously.
type WebhookReviewHandler func(ctx context.Context, db *sql.DB, orgID int64, connectorID int64, eventJSON string, scenarioType string) error

var webhookReviewHandler WebhookReviewHandler

// RegisterWebhookReviewHandler registers a webhook review handler implementation.
func RegisterWebhookReviewHandler(handler WebhookReviewHandler) {
	webhookReviewHandler = handler
}

// ProcessWebhookReview routes the webhook review task to the registered handler.
func ProcessWebhookReview(ctx context.Context, db *sql.DB, orgID int64, connectorID int64, eventJSON string, scenarioType string) error {
	if webhookReviewHandler == nil {
		log.Printf("[ERROR] ProcessWebhookReview: Webhook review handler not registered")
		return fmt.Errorf("webhook review handler not registered")
	}
	return webhookReviewHandler(ctx, db, orgID, connectorID, eventJSON, scenarioType)
}
