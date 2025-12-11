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
		Entity json.RawMessage `json:"entity"`
	} `json:"subscription"`
	Payment struct {
		Entity json.RawMessage `json:"entity"`
	} `json:"payment"`
}

// HandleWebhook processes incoming Razorpay webhook events
func (h *RazorpayWebhookHandler) HandleWebhook(c echo.Context) error {
	// Read the raw body
	body, err := io.ReadAll(c.Request().Body)
	if err != nil {
		fmt.Printf("[WEBHOOK] ERROR: Failed to read request body: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "failed to read request body",
		})
	}

	// Verify webhook signature
	signature := c.Request().Header.Get("X-Razorpay-Signature")
	if !h.verifySignature(body, signature) {
		fmt.Printf("[WEBHOOK] ERROR: Invalid webhook signature\n")
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid webhook signature",
		})
	}

	// Parse the event
	var event RazorpayWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		fmt.Printf("[WEBHOOK] ERROR: Failed to parse JSON payload: %v\n", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid JSON payload",
		})
	}

	fmt.Printf("[WEBHOOK] ✓ Received event: %s (account: %s)\n", event.Event, event.AccountID)
	fmt.Printf("[WEBHOOK] Raw payload: %s\n", string(event.Payload))

	// Process the event based on type
	if err := h.processEvent(&event); err != nil {
		// Log the error but return 200 to prevent Razorpay from retrying
		// We log the failure in the database for manual review
		fmt.Printf("[WEBHOOK] ✗ FAILED to process %s: %v\n", event.Event, err)
		_ = h.logFailedEvent(&event, err)
		return c.JSON(http.StatusOK, map[string]string{
			"status": "error logged",
		})
	}

	fmt.Printf("[WEBHOOK] ✓ Successfully processed: %s\n", event.Event)
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
	case "subscription.authenticated":
		// subscription.authenticated is sent when first payment is authorized
		// This event contains both subscription and payment data
		return h.handleSubscriptionAuthenticated(event)
	case "payment.authorized":
		return h.handlePaymentAuthorized(event)
	case "payment.captured":
		return h.handlePaymentCaptured(event)
	case "payment.failed":
		return h.handlePaymentFailed(event)
	default:
		// Unknown event type - log but don't fail
		return h.logUnknownEvent(event)
	}
}

// handleSubscriptionAuthenticated handles subscription.authenticated event
// This event contains both subscription and payment data (first payment)
func (h *RazorpayWebhookHandler) handleSubscriptionAuthenticated(event *RazorpayWebhookEvent) error {
	fmt.Printf("[SUBSCRIPTION.AUTHENTICATED] Processing subscription authentication...\n")

	// Extract both subscription and payment from the event
	sub, err := h.extractSubscription(event)
	if err != nil {
		fmt.Printf("[SUBSCRIPTION.AUTHENTICATED] ✗ Failed to extract subscription: %v\n", err)
		return err
	}

	payment, err := h.extractPayment(event)
	if err != nil {
		// Payment extraction might fail if not yet captured, that's okay
		fmt.Printf("[SUBSCRIPTION.AUTHENTICATED] ⚠ Could not extract payment (might not be captured yet): %v\n", err)
	}

	fmt.Printf("[SUBSCRIPTION.AUTHENTICATED] Subscription ID: %s, Status: %s\n", sub.ID, sub.Status)
	if payment != nil {
		fmt.Printf("[SUBSCRIPTION.AUTHENTICATED] Payment ID: %s, Status: %s\n", payment.ID, payment.Status)
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details
	var subscriptionID int64
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT id, owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&subscriptionID, &ownerUserID, &orgID)
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
		WHERE id = $4`,
		sub.Status,
		time.Unix(sub.CurrentStart, 0),
		time.Unix(sub.CurrentEnd, 0),
		subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// If payment is present and authorized, record it
	if payment != nil {
		paymentDataJSON, _ := json.Marshal(payment)
		_, err = tx.Exec(`
			INSERT INTO subscription_payments (
				subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
				amount, currency, status, method, authorized_at, razorpay_data, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, NOW(), NOW())
			ON CONFLICT (razorpay_payment_id) DO UPDATE SET
				status = $7,
				authorized_at = NOW(),
				razorpay_data = $9,
				updated_at = NOW()`,
			subscriptionID, payment.ID, payment.OrderID, payment.InvoiceID,
			payment.Amount, payment.Currency, payment.Status, payment.Method, paymentDataJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to store payment: %w", err)
		}
	}

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"event":           event.Event,
	}
	if payment != nil {
		metadata["payment_id"] = payment.ID
		metadata["payment_status"] = payment.Status
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_authenticated",
		"Subscription authenticated with first payment",
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("[SUBSCRIPTION.AUTHENTICATED] ✓ SUCCESS: Subscription %s authenticated\n", sub.ID)
	return nil
}

// handleSubscriptionActivated handles subscription activation
func (h *RazorpayWebhookHandler) handleSubscriptionActivated(event *RazorpayWebhookEvent) error {
	fmt.Printf("[SUBSCRIPTION.ACTIVATED] Processing subscription activation...\n")

	sub, err := h.extractSubscription(event)
	if err != nil {
		fmt.Printf("[SUBSCRIPTION.ACTIVATED] ✗ Failed to extract subscription: %v\n", err)
		return err
	}

	fmt.Printf("[SUBSCRIPTION.ACTIVATED] Subscription ID: %s, Status: %s\n", sub.ID, sub.Status)

	// Try to extract payment from webhook (subscription.activated includes payment)
	payment, err := h.extractPayment(event)
	if err != nil {
		fmt.Printf("[SUBSCRIPTION.ACTIVATED] ⚠ No payment in webhook (non-blocking): %v\n", err)
		payment = nil // Continue without payment
	} else {
		fmt.Printf("[SUBSCRIPTION.ACTIVATED] ✓ Payment extracted: %s, Status: %s, Captured: %v\n",
			payment.ID, payment.Status, payment.Captured.Bool())
	}

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details to find user and org
	var subscriptionID int64
	var ownerUserID, orgID int
	var planType string
	err = tx.QueryRow(`
		SELECT id, owner_user_id, org_id, plan_type
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&subscriptionID, &ownerUserID, &orgID, &planType)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Update subscription status
	updateQuery := `
		UPDATE subscriptions
		SET status = $1,
		    current_period_start = $2,
		    current_period_end = $3,
		    updated_at = NOW()`
	updateArgs := []interface{}{
		sub.Status,
		time.Unix(sub.CurrentStart, 0),
		time.Unix(sub.CurrentEnd, 0),
	}

	// If payment was captured, mark as verified
	if payment != nil && payment.Captured.Bool() && payment.Status == "captured" {
		fmt.Printf("[SUBSCRIPTION.ACTIVATED] ✓ Payment captured, marking subscription as PAID\n")
		updateQuery += `,
		    last_payment_id = $4,
		    last_payment_status = $5,
		    last_payment_received_at = NOW(),
		    payment_verified = TRUE`
		updateArgs = append(updateArgs, payment.ID, payment.Status)

		// Also store in subscription_payments table
		_, err = tx.Exec(`
			INSERT INTO subscription_payments (
				subscription_id, razorpay_payment_id, amount, currency, 
				status, captured, method, created_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
			ON CONFLICT (razorpay_payment_id) DO UPDATE SET
				status = EXCLUDED.status,
				captured = EXCLUDED.captured,
				updated_at = NOW()`,
			subscriptionID, payment.ID, payment.Amount, payment.Currency,
			payment.Status, payment.Captured.Bool(), payment.Method,
		)
		if err != nil {
			fmt.Printf("[SUBSCRIPTION.ACTIVATED] ⚠ Failed to store payment (non-blocking): %v\n", err)
		}
	}

	updateQuery += ` WHERE razorpay_subscription_id = $` + fmt.Sprintf("%d", len(updateArgs)+1)
	updateArgs = append(updateArgs, sub.ID)

	_, err = tx.Exec(updateQuery, updateArgs...)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          sub.Status,
		"event":           event.Event,
	}
	if payment != nil {
		metadata["payment_id"] = payment.ID
		metadata["payment_status"] = payment.Status
		metadata["payment_captured"] = payment.Captured.Bool()
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
	fmt.Printf("[SUBSCRIPTION.CHARGED] Processing subscription charge...\n")

	sub, err := h.extractSubscription(event)
	if err != nil {
		fmt.Printf("[SUBSCRIPTION.CHARGED] ✗ Failed to extract subscription: %v\n", err)
		return err
	}

	fmt.Printf("[SUBSCRIPTION.CHARGED] Subscription ID: %s\n", sub.ID)

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details (get internal ID first)
	var subscriptionID int64
	var ownerUserID, orgID int
	var planType string
	var currentLicenseExpiry time.Time
	err = tx.QueryRow(`
		SELECT id, owner_user_id, org_id, plan_type, license_expires_at
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&subscriptionID, &ownerUserID, &orgID, &planType, &currentLicenseExpiry)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	fmt.Printf("[SUBSCRIPTION.CHARGED] Found internal subscription ID: %d\n", subscriptionID)

	// Try to extract payment data (subscription.charged contains payment info)
	payment, err := h.extractPayment(event)
	if err != nil {
		fmt.Printf("[SUBSCRIPTION.CHARGED] ⚠ Could not extract payment: %v\n", err)
	} else {
		fmt.Printf("[SUBSCRIPTION.CHARGED] Payment ID: %s, Status: %s, Captured: %t\n",
			payment.ID, payment.Status, payment.Captured)

		// Store payment record
		paymentDataJSON, _ := json.Marshal(payment)
		_, err = tx.Exec(`
			INSERT INTO subscription_payments (
				subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
				amount, currency, status, method, captured_at, razorpay_data, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, NOW(), NOW())
			ON CONFLICT (razorpay_payment_id) DO UPDATE SET
				status = $7,
				captured_at = NOW(),
				razorpay_data = $9,
				updated_at = NOW()`,
			subscriptionID, payment.ID, payment.OrderID, payment.InvoiceID,
			payment.Amount, payment.Currency, payment.Status, payment.Method, paymentDataJSON,
		)
		if err != nil {
			fmt.Printf("[SUBSCRIPTION.CHARGED] ⚠ Failed to store payment: %v\n", err)
		}

		// If payment is captured, mark subscription as paid
		if payment.Captured || payment.Status == "captured" {
			fmt.Printf("[SUBSCRIPTION.CHARGED] ✓ Payment captured, marking subscription as PAID\n")
			_, err = tx.Exec(`
				UPDATE subscriptions
				SET last_payment_id = $1,
				    last_payment_status = $2,
				    last_payment_received_at = NOW(),
				    payment_verified = TRUE,
				    updated_at = NOW()
				WHERE id = $3`,
				payment.ID, payment.Status, subscriptionID,
			)
			if err != nil {
				return fmt.Errorf("failed to update subscription payment status: %w", err)
			}
		}
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
		WHERE id = $4`,
		newExpiry,
		time.Unix(sub.CurrentStart, 0),
		time.Unix(sub.CurrentEnd, 0),
		subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Update all users with this subscription (use internal ID)
	_, err = tx.Exec(`
		UPDATE user_roles
		SET license_expires_at = $1,
		    updated_at = NOW()
		WHERE active_subscription_id = $2`,
		newExpiry, subscriptionID,
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
	if payment != nil {
		metadata["payment_id"] = payment.ID
		metadata["payment_status"] = payment.Status
		metadata["payment_captured"] = payment.Captured.Bool()
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

	fmt.Printf("[DEBUG] Subscription raw JSON: %s\n", string(payload.Subscription.Entity))

	var sub RazorpaySubscription
	if err := json.Unmarshal(payload.Subscription.Entity, &sub); err != nil {
		fmt.Printf("[DEBUG] Failed to unmarshal subscription, error: %v\n", err)
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

// handlePaymentAuthorized handles payment authorization events
func (h *RazorpayWebhookHandler) handlePaymentAuthorized(event *RazorpayWebhookEvent) error {
	fmt.Printf("[PAYMENT.AUTHORIZED] Processing payment authorization...\n")

	payment, err := h.extractPayment(event)
	if err != nil {
		fmt.Printf("[PAYMENT.AUTHORIZED] ✗ Failed to extract payment: %v\n", err)
		return err
	}

	fmt.Printf("[PAYMENT.AUTHORIZED] Payment ID: %s, Amount: %d %s\n",
		payment.ID, payment.Amount, payment.Currency)

	// Find subscription by invoice ID or order ID from payment notes
	subscriptionID, err := h.findSubscriptionFromPayment(payment)
	if err != nil {
		fmt.Printf("[PAYMENT.AUTHORIZED] ✗ Failed to find subscription: %v\n", err)
		return fmt.Errorf("failed to find subscription for payment %s: %w", payment.ID, err)
	}

	fmt.Printf("[PAYMENT.AUTHORIZED] ✓ Payment authorized for subscription ID: %d (waiting for capture)\n", subscriptionID)

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store payment record
	paymentDataJSON, _ := json.Marshal(payment)
	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
			amount, currency, status, method, authorized_at, razorpay_data, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, NOW(), NOW())
		ON CONFLICT (razorpay_payment_id) DO UPDATE SET
			status = $7,
			authorized_at = NOW(),
			razorpay_data = $9,
			updated_at = NOW()`,
		subscriptionID, payment.ID, payment.OrderID, payment.InvoiceID,
		payment.Amount, payment.Currency, payment.Status, payment.Method, paymentDataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to store payment: %w", err)
	}

	// Log event
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE id = $1`,
		subscriptionID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %d", subscriptionID)
	}

	metadata := map[string]interface{}{
		"subscription_id": subscriptionID,
		"payment_id":      payment.ID,
		"amount":          payment.Amount,
		"currency":        payment.Currency,
		"status":          payment.Status,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "payment_authorized",
		fmt.Sprintf("Payment %s authorized for amount %d %s", payment.ID, payment.Amount, payment.Currency),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// handlePaymentCaptured handles payment capture events (money received!)
func (h *RazorpayWebhookHandler) handlePaymentCaptured(event *RazorpayWebhookEvent) error {
	fmt.Printf("[PAYMENT.CAPTURED] Processing payment capture...\n")

	payment, err := h.extractPayment(event)
	if err != nil {
		fmt.Printf("[PAYMENT.CAPTURED] ✗ Failed to extract payment: %v\n", err)
		return err
	}

	fmt.Printf("[PAYMENT.CAPTURED] Payment ID: %s, Amount: %d %s, Status: %s\n",
		payment.ID, payment.Amount, payment.Currency, payment.Status)

	// Find subscription by invoice ID or order ID from payment notes
	subscriptionID, err := h.findSubscriptionFromPayment(payment)
	if err != nil {
		fmt.Printf("[PAYMENT.CAPTURED] ✗ Failed to find subscription for payment %s: %v\n", payment.ID, err)
		return fmt.Errorf("failed to find subscription for payment %s: %w", payment.ID, err)
	}

	fmt.Printf("[PAYMENT.CAPTURED] Found subscription ID: %d\n", subscriptionID)

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store/update payment record
	paymentDataJSON, _ := json.Marshal(payment)
	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
			amount, currency, status, method, captured_at, razorpay_data, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, NOW(), NOW())
		ON CONFLICT (razorpay_payment_id) DO UPDATE SET
			status = $7,
			captured_at = NOW(),
			razorpay_data = $9,
			updated_at = NOW()`,
		subscriptionID, payment.ID, payment.OrderID, payment.InvoiceID,
		payment.Amount, payment.Currency, payment.Status, payment.Method, paymentDataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to store payment: %w", err)
	}

	// Update subscription with payment verification
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET last_payment_id = $1,
		    last_payment_status = $2,
		    last_payment_received_at = NOW(),
		    payment_verified = TRUE,
		    updated_at = NOW()
		WHERE id = $3`,
		payment.ID, payment.Status, subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription payment status: %w", err)
	}

	// Log event
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE id = $1`,
		subscriptionID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %d", subscriptionID)
	}

	metadata := map[string]interface{}{
		"subscription_id": subscriptionID,
		"payment_id":      payment.ID,
		"amount":          payment.Amount,
		"currency":        payment.Currency,
		"status":          payment.Status,
		"event":           event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "payment_captured",
		fmt.Sprintf("Payment %s captured - amount %d %s received", payment.ID, payment.Amount, payment.Currency),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		fmt.Printf("[PAYMENT.CAPTURED] ✗ Failed to commit transaction: %v\n", err)
		return err
	}

	fmt.Printf("[PAYMENT.CAPTURED] ✓ SUCCESS: Payment %s captured, subscription %d verified and marked as PAID\n",
		payment.ID, subscriptionID)
	return nil
}

// handlePaymentFailed handles payment failure events
func (h *RazorpayWebhookHandler) handlePaymentFailed(event *RazorpayWebhookEvent) error {
	fmt.Printf("[PAYMENT.FAILED] Processing payment failure...\n")

	payment, err := h.extractPayment(event)
	if err != nil {
		fmt.Printf("[PAYMENT.FAILED] ✗ Failed to extract payment: %v\n", err)
		return err
	}

	fmt.Printf("[PAYMENT.FAILED] Payment ID: %s, Error: %s - %s\n",
		payment.ID, payment.ErrorCode, payment.ErrorDescription)

	// Find subscription by invoice ID or order ID from payment notes
	subscriptionID, err := h.findSubscriptionFromPayment(payment)
	if err != nil {
		fmt.Printf("[PAYMENT.FAILED] ⚠ Could not find subscription, logging as orphan\n")
		// If we can't find the subscription, just log the event
		return h.logPaymentFailureWithoutSubscription(payment, event)
	}

	fmt.Printf("[PAYMENT.FAILED] ✗ Payment failed for subscription ID: %d\n", subscriptionID)

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Store payment record
	paymentDataJSON, _ := json.Marshal(payment)
	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
			amount, currency, status, method, failed_at, error_code, error_description,
			razorpay_data, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), $9, $10, $11, NOW(), NOW())
		ON CONFLICT (razorpay_payment_id) DO UPDATE SET
			status = $7,
			failed_at = NOW(),
			error_code = $9,
			error_description = $10,
			razorpay_data = $11,
			updated_at = NOW()`,
		subscriptionID, payment.ID, payment.OrderID, payment.InvoiceID,
		payment.Amount, payment.Currency, payment.Status, payment.Method,
		payment.ErrorCode, payment.ErrorDescription, paymentDataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to store payment: %w", err)
	}

	// Update subscription with failed payment status
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET last_payment_id = $1,
		    last_payment_status = $2,
		    updated_at = NOW()
		WHERE id = $3`,
		payment.ID, payment.Status, subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription payment status: %w", err)
	}

	// Log event
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE id = $1`,
		subscriptionID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %d", subscriptionID)
	}

	metadata := map[string]interface{}{
		"subscription_id":   subscriptionID,
		"payment_id":        payment.ID,
		"amount":            payment.Amount,
		"currency":          payment.Currency,
		"status":            payment.Status,
		"error_code":        payment.ErrorCode,
		"error_description": payment.ErrorDescription,
		"event":             event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "payment_failed",
		fmt.Sprintf("Payment %s failed: %s", payment.ID, payment.ErrorDescription),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	return tx.Commit()
}

// extractPayment extracts the payment object from webhook payload
func (h *RazorpayWebhookHandler) extractPayment(event *RazorpayWebhookEvent) (*RazorpayPayment, error) {
	var payload RazorpayWebhookPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("[DEBUG] Payment raw JSON: %s\n", string(payload.Payment.Entity))

	var payment RazorpayPayment
	if err := json.Unmarshal(payload.Payment.Entity, &payment); err != nil {
		fmt.Printf("[DEBUG] Failed to unmarshal payment, error: %v\n", err)
		return nil, fmt.Errorf("failed to parse payment: %w", err)
	}

	return &payment, nil
}

// findSubscriptionFromPayment finds the subscription ID associated with a payment
func (h *RazorpayWebhookHandler) findSubscriptionFromPayment(payment *RazorpayPayment) (int64, error) {
	var subscriptionID int64

	// Try to find by invoice ID (most reliable for subscription payments)
	if payment.InvoiceID != "" {
		// Invoice ID typically contains the subscription ID in Razorpay
		// We need to query subscriptions to find a match
		err := h.db.QueryRow(`
			SELECT id FROM subscriptions 
			WHERE razorpay_data->>'invoice_id' = $1 
			   OR razorpay_subscription_id IN (
				SELECT jsonb_object_keys(razorpay_data->'invoices')
				FROM subscriptions
				WHERE razorpay_data->'invoices' ? $1
			)
			LIMIT 1`,
			payment.InvoiceID,
		).Scan(&subscriptionID)
		if err == nil {
			return subscriptionID, nil
		}
	}

	// Try to find by order ID
	if payment.OrderID != "" {
		err := h.db.QueryRow(`
			SELECT id FROM subscriptions 
			WHERE razorpay_data->>'order_id' = $1
			LIMIT 1`,
			payment.OrderID,
		).Scan(&subscriptionID)
		if err == nil {
			return subscriptionID, nil
		}
	}

	// Try to find by notes in payment
	notes := payment.GetPaymentNotesMap()
	if notes != nil {
		if subID, ok := notes["subscription_id"]; ok {
			err := h.db.QueryRow(`
				SELECT id FROM subscriptions 
				WHERE razorpay_subscription_id = $1
				LIMIT 1`,
				subID,
			).Scan(&subscriptionID)
			if err == nil {
				return subscriptionID, nil
			}
		}
	}

	// Try to find by customer_id - get the most recent active subscription
	if payment.CustomerID != "" {
		err := h.db.QueryRow(`
			SELECT id FROM subscriptions 
			WHERE razorpay_customer_id = $1
			  AND status IN ('active', 'authenticated')
			ORDER BY created_at DESC
			LIMIT 1`,
			payment.CustomerID,
		).Scan(&subscriptionID)
		if err == nil {
			return subscriptionID, nil
		}
	}

	return 0, fmt.Errorf("could not find subscription for payment %s (customer: %s, invoice: %s, order: %s)",
		payment.ID, payment.CustomerID, payment.InvoiceID, payment.OrderID)
}

// logPaymentFailureWithoutSubscription logs payment failures when subscription can't be found
func (h *RazorpayWebhookHandler) logPaymentFailureWithoutSubscription(payment *RazorpayPayment, event *RazorpayWebhookEvent) error {
	paymentDataJSON, _ := json.Marshal(payment)
	metadata := map[string]interface{}{
		"payment_id":        payment.ID,
		"amount":            payment.Amount,
		"currency":          payment.Currency,
		"status":            payment.Status,
		"error_code":        payment.ErrorCode,
		"error_description": payment.ErrorDescription,
		"event":             event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)

	_, err := h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, NULL, $1, $2, $3, NOW())`,
		"payment_failed_orphan",
		fmt.Sprintf("Payment %s failed (no subscription found): %s", payment.ID, payment.ErrorDescription),
		metadataJSON,
	)

	// Also try to store in subscription_payments with NULL subscription
	_, _ = h.db.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
			amount, currency, status, method, failed_at, error_code, error_description,
			razorpay_data, created_at, updated_at
		) VALUES (NULL, $1, $2, $3, $4, $5, $6, $7, NOW(), $8, $9, $10, NOW(), NOW())
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		payment.ID, payment.OrderID, payment.InvoiceID,
		payment.Amount, payment.Currency, payment.Status, payment.Method,
		payment.ErrorCode, payment.ErrorDescription, paymentDataJSON,
	)

	return err
}
