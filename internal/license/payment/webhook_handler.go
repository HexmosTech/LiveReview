package payment

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// RazorpayWebhookHandler handles Razorpay webhook events
type RazorpayWebhookHandler struct {
	db            *sql.DB
	webhookSecret string
}

// NewRazorpayWebhookHandler creates a new Razorpay webhook handler
func NewRazorpayWebhookHandler(db *sql.DB, webhookSecret string) *RazorpayWebhookHandler {
	return &RazorpayWebhookHandler{
		db:            db,
		webhookSecret: webhookSecret,
	}
}

// RazorpayWebhookEvent represents a webhook event from Razorpay
type RazorpayWebhookEvent struct {
	Entity    string          `json:"entity"`
	AccountID string          `json:"account_id"`
	Event     string          `json:"event"`
	Contains  []string        `json:"contains"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt int64           `json:"created_at"`
}

// RazorpayWebhookPayload represents the payload structure
type RazorpayWebhookPayload struct {
	Subscription struct {
		Entity json.RawMessage `json:"subscription"`
	} `json:"subscription"`
	Payment struct {
		Entity json.RawMessage `json:"payment"`
	} `json:"payment"`
}

// HandleWebhook processes incoming Razorpay webhook events
func (h *RazorpayWebhookHandler) HandleWebhook(c echo.Context) error {
	// Read the raw body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "failed to read request body",
		})
	}

	// Verify webhook signature
	signature := c.Request().Header.Get("X-Razorpay-Signature")
	if !h.verifySignature(body, signature) {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid webhook signature",
		})
	}

	// Parse the event
	var event RazorpayWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid JSON payload",
		})
	}

	// Process the event based on type
	if err := h.processEvent(&event); err != nil {
		// Log the error but return 200 to prevent Razorpay from retrying
		// We log the failure in the database for manual review
		_ = h.logFailedEvent(&event, err)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "error logged",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status": "processed",
	})
}

// verifySignature verifies the Razorpay webhook signature
func (h *RazorpayWebhookHandler) verifySignature(body []byte, signature string) bool {
	if h.webhookSecret == "" {
		// In test mode, skip signature verification if no secret configured
		return true
	}

	mac := hmac.New(sha256.New, []byte(h.webhookSecret))
	mac.Write(body)
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	return subtle.ConstantTimeCompare([]byte(signature), []byte(expectedSignature)) == 1
}

// processEvent routes events to appropriate handlers
func (h *RazorpayWebhookHandler) processEvent(event *RazorpayWebhookEvent) error {
	switch event.Event {
	case "subscription.activated":
		return h.handleSubscriptionActivated(event)
	case "subscription.charged":
		return h.handleSubscriptionCharged(event)
	case "subscription.cancelled":
		return h.handleSubscriptionCancelled(event)
	case "subscription.completed":
		return h.handleSubscriptionCompleted(event)
	case "subscription.paused":
		return h.handleSubscriptionPaused(event)
	case "subscription.resumed":
		return h.handleSubscriptionResumed(event)
	case "subscription.pending":
		return h.handleSubscriptionPending(event)
	case "subscription.halted":
		return h.handleSubscriptionHalted(event)
	default:
		// Unknown event type - log but don't fail
		return h.logUnknownEvent(event)
	}
}

// handleSubscriptionActivated handles subscription activation
func (h *RazorpayWebhookHandler) handleSubscriptionActivated(event *RazorpayWebhookEvent) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details to find user and org
	var ownerUserID, orgID int
	var planType string
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id, plan_type
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&ownerUserID, &orgID, &planType)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Update subscription status
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    current_period_start = $2,
		    current_period_end = $3,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $4`,
		sub.Status,
		time.Unix(sub.CurrentStart, 0),
		time.Unix(sub.CurrentEnd, 0),
		sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_activated",
		"Subscription activated by Razorpay",
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// handleSubscriptionCharged handles successful subscription charges
func (h *RazorpayWebhookHandler) handleSubscriptionCharged(event *RazorpayWebhookEvent) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details
	var ownerUserID, orgID int
	var planType string
	var currentLicenseExpiry time.Time
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id, plan_type, license_expires_at
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&ownerUserID, &orgID, &planType, &currentLicenseExpiry)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Extend license based on plan type
	var newExpiry time.Time
	if planType == "team_monthly" {
		newExpiry = currentLicenseExpiry.AddDate(0, 1, 0)
	} else {
		newExpiry = currentLicenseExpiry.AddDate(1, 0, 0)
	}

	// Update subscription with new expiry
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET license_expires_at = $1,
		    current_period_start = $2,
		    current_period_end = $3,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $4`,
		newExpiry,
		time.Unix(sub.CurrentStart, 0),
		time.Unix(sub.CurrentEnd, 0),
		sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Update all users with this subscription
	_, err = tx.Exec(`
		UPDATE user_roles
		SET license_expires_at = $1,
		    updated_at = NOW()
		WHERE active_subscription_id = $2`,
		newExpiry, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user licenses: %w", err)
	}

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"new_expiry":      newExpiry,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_charged",
		fmt.Sprintf("Subscription charged, license extended to %s", newExpiry.Format("2006-01-02")),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// handleSubscriptionCancelled handles subscription cancellation
func (h *RazorpayWebhookHandler) handleSubscriptionCancelled(event *RazorpayWebhookEvent) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Update subscription status
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $2`,
		sub.Status, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Note: We don't immediately revoke licenses - they remain valid until license_expires_at
	// The user keeps access until the paid period ends

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_cancelled",
		"Subscription cancelled, access continues until license expiry",
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// handleSubscriptionCompleted handles subscription completion
func (h *RazorpayWebhookHandler) handleSubscriptionCompleted(event *RazorpayWebhookEvent) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Update subscription status
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $2`,
		sub.Status, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revert users to free plan (subscription has ended)
	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'free',
		    license_expires_at = NULL,
		    active_subscription_id = NULL,
		    updated_at = NOW()
		WHERE active_subscription_id = $1`,
		sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to revert users to free plan: %w", err)
	}

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_completed",
		"Subscription completed, users reverted to free plan",
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// handleSubscriptionPaused handles subscription pause
func (h *RazorpayWebhookHandler) handleSubscriptionPaused(event *RazorpayWebhookEvent) error {
	return h.updateSubscriptionStatus(event, "paused")
}

// handleSubscriptionResumed handles subscription resume
func (h *RazorpayWebhookHandler) handleSubscriptionResumed(event *RazorpayWebhookEvent) error {
	return h.updateSubscriptionStatus(event, "resumed")
}

// handleSubscriptionPending handles subscription pending state
func (h *RazorpayWebhookHandler) handleSubscriptionPending(event *RazorpayWebhookEvent) error {
	return h.updateSubscriptionStatus(event, "pending")
}

// handleSubscriptionHalted handles subscription halt (payment failures)
func (h *RazorpayWebhookHandler) handleSubscriptionHalted(event *RazorpayWebhookEvent) error {
	return h.updateSubscriptionStatus(event, "halted")
}

// updateSubscriptionStatus is a helper for simple status updates
func (h *RazorpayWebhookHandler) updateSubscriptionStatus(event *RazorpayWebhookEvent, eventType string) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Update status
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $2`,
		sub.Status, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_"+eventType,
		fmt.Sprintf("Subscription %s", eventType),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// extractSubscription extracts the subscription object from webhook payload
func (h *RazorpayWebhookHandler) extractSubscription(event *RazorpayWebhookEvent) (*RazorpaySubscription, error) {
	var payload RazorpayWebhookPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	var sub RazorpaySubscription
	if err := json.Unmarshal(payload.Subscription.Entity, &sub); err != nil {
		return nil, fmt.Errorf("failed to parse subscription: %w", err)
	}

	return &sub, nil
}

// logFailedEvent logs events that failed to process
func (h *RazorpayWebhookHandler) logFailedEvent(event *RazorpayWebhookEvent, processingError error) error {
	eventJSON, _ := json.Marshal(event)
	_, err := h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, NULL, $1, $2, $3, NOW())`,
		"webhook_error",
		fmt.Sprintf("Failed to process webhook: %v", processingError),
		eventJSON,
	)
	return err
}

// logUnknownEvent logs unknown event types
func (h *RazorpayWebhookHandler) logUnknownEvent(event *RazorpayWebhookEvent) error {
	eventJSON, _ := json.Marshal(event)
	_, err := h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, NULL, $1, $2, $3, NOW())`,
		"webhook_unknown",
		fmt.Sprintf("Unknown webhook event type: %s", event.Event),
		eventJSON,
	)
	return err
}
