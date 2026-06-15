package reviewprocessor

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
)

// WebhookReviewHandler defines the signature for processing a webhook event asynchronously.
type WebhookReviewHandler func(ctx context.Context, db *sql.DB, orgID int64, connectorID int64, eventJSON string, scenarioType string) error

var (
	webhookReviewHandler      WebhookReviewHandler
	webhookReviewHandlerMutex sync.RWMutex
)

// RegisterWebhookReviewHandler registers a webhook review handler implementation.
func RegisterWebhookReviewHandler(handler WebhookReviewHandler) {
	webhookReviewHandlerMutex.Lock()
	defer webhookReviewHandlerMutex.Unlock()
	webhookReviewHandler = handler
}

// ProcessWebhookReview routes the webhook review task to the registered handler.
func ProcessWebhookReview(ctx context.Context, db *sql.DB, orgID int64, connectorID int64, eventJSON string, scenarioType string) error {
	webhookReviewHandlerMutex.RLock()
	handler := webhookReviewHandler
	webhookReviewHandlerMutex.RUnlock()

	if handler == nil {
		log.Printf("[ERROR] ProcessWebhookReview: Webhook review handler not registered")
		return fmt.Errorf("webhook review handler not registered")
	}
	return handler(ctx, db, orgID, connectorID, eventJSON, scenarioType)
}

