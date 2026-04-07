package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	storagepayment "github.com/livereview/storage/payment"
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
	case "subscription.expired":
		return h.handleSubscriptionExpired(event)
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

	if err := tx.Commit(); err != nil {
		return err
	}

	if handled, confirmErr := h.tryMarkUpgradeRequestSubscriptionConfirmed(sub.ID, event.Event, map[string]interface{}{"subscription_status": sub.Status}); confirmErr != nil {
		fmt.Printf("[SUBSCRIPTION.ACTIVATED] ⚠ Failed to mark upgrade subscription confirmation for %s: %v\n", sub.ID, confirmErr)
	} else if handled {
		fmt.Printf("[SUBSCRIPTION.ACTIVATED] ✓ Upgrade request subscription confirmation recorded for %s\n", sub.ID)
	}

	return nil
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
	resolvedPlanCode := normalizePersistedPlanCode(planType)

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

			periodStart := time.Now().UTC()
			periodEnd := periodStart.AddDate(0, 1, 0)
			if sub.CurrentStart > 0 {
				periodStart = time.Unix(sub.CurrentStart, 0).UTC()
			}
			if sub.CurrentEnd > 0 {
				periodEnd = time.Unix(sub.CurrentEnd, 0).UTC()
			}

			_, err = tx.Exec(`
				INSERT INTO org_billing_state (
					org_id,
					current_plan_code,
					billing_period_start,
					billing_period_end,
					loc_used_month,
					loc_blocked,
					trial_readonly,
					last_reset_at,
					updated_at
				) VALUES ($1, $2, $3, $4, 0, FALSE, FALSE, NOW(), NOW())
				ON CONFLICT (org_id) DO UPDATE SET
					current_plan_code = EXCLUDED.current_plan_code,
					billing_period_start = EXCLUDED.billing_period_start,
					billing_period_end = EXCLUDED.billing_period_end,
					scheduled_plan_code = NULL,
					scheduled_plan_effective_at = NULL,
					upgrade_loc_grant_current_cycle = 0,
					upgrade_loc_grant_expires_at = NULL,
					trial_readonly = FALSE,
					loc_blocked = FALSE,
					updated_at = NOW()`,
				orgID, resolvedPlanCode.String(), periodStart, periodEnd,
			)
			if err != nil {
				return fmt.Errorf("failed to update org billing state after captured charge: %w", err)
			}
		}
	}

	// Extend license based on plan type
	var newExpiry time.Time
	if resolvedPlanCode.GetLimits().MonthlyPriceUSD > 0 {
		newExpiry = currentLicenseExpiry.AddDate(0, 1, 0)
	} else {
		newExpiry = currentLicenseExpiry.AddDate(1, 0, 0)
	}

	// Update subscription with new expiry
	// Also set status to 'active' (in case it was halted) and clear cancel_at_period_end (renewal = not cancelled)
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET license_expires_at = $1,
		    current_period_start = $2,
		    current_period_end = $3,
		    status = 'active',
		    cancel_at_period_end = FALSE,
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

	fmt.Printf("[SUBSCRIPTION.CHARGED] ✓ Updated subscription to active status, license extended to %s\n", newExpiry.Format("2006-01-02"))

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

	if err := tx.Commit(); err != nil {
		return err
	}

	if handled, confirmErr := h.tryMarkUpgradeRequestSubscriptionConfirmed(sub.ID, event.Event, map[string]interface{}{"subscription_status": sub.Status}); confirmErr != nil {
		fmt.Printf("[SUBSCRIPTION.CHARGED] ⚠ Failed to mark upgrade subscription confirmation for %s: %v\n", sub.ID, confirmErr)
	} else if handled {
		fmt.Printf("[SUBSCRIPTION.CHARGED] ✓ Upgrade request subscription confirmation recorded for %s\n", sub.ID)
	}

	return nil
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

	pendingCancel := strings.EqualFold(strings.TrimSpace(sub.Status), "active")

	// Update subscription status
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    cancel_at_period_end = $2,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $3`,
		sub.Status, pendingCancel, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	// Revert users to free plan ONLY if subscription is not active
	// If it's active but cancelled, it means it's scheduled for cancellation at period end
	if sub.Status != "active" {
		_, err = tx.Exec(`
			UPDATE user_roles
			SET plan_type = 'free',
			    license_expires_at = NULL,
			    active_subscription_id = NULL,
			    updated_at = NOW()
			WHERE active_subscription_id = $1`,
			subscriptionID,
		)
		if err != nil {
			return fmt.Errorf("failed to revert users to free plan: %w", err)
		}
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
		ownerUserID, orgID, "subscription_cancelled",
		"Subscription cancellation webhook processed",
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
		subscriptionID,
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
// If the subscription has passed its expiry date, it should be expired and users downgraded
func (h *RazorpayWebhookHandler) handleSubscriptionHalted(event *RazorpayWebhookEvent) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	fmt.Printf("[SUBSCRIPTION.HALTED] Processing halted subscription: %s\n", sub.ID)

	tx, err := h.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details including expiry
	var subscriptionID int64
	var ownerUserID, orgID int
	var currentPeriodEnd time.Time
	err = tx.QueryRow(`
		SELECT id, owner_user_id, org_id, current_period_end
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	).Scan(&subscriptionID, &ownerUserID, &orgID, &currentPeriodEnd)
	if err != nil {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}

	// Check if subscription has expired (current_period_end is in the past)
	now := time.Now()
	hasExpired := currentPeriodEnd.Before(now)

	if hasExpired {
		fmt.Printf("[SUBSCRIPTION.HALTED] ⚠ Subscription has expired (period_end: %s), expiring subscription\n",
			currentPeriodEnd.Format("2006-01-02 15:04:05"))

		// Update subscription status to expired
		_, err = tx.Exec(`
			UPDATE subscriptions
			SET status = 'expired',
			    updated_at = NOW()
			WHERE razorpay_subscription_id = $1`,
			sub.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update subscription to expired: %w", err)
		}

		// Revert ALL users on this subscription to free plan
		result, err := tx.Exec(`
			UPDATE user_roles
			SET plan_type = 'free',
			    license_expires_at = NULL,
			    active_subscription_id = NULL,
			    updated_at = NOW()
			WHERE active_subscription_id = $1`,
			subscriptionID,
		)
		if err != nil {
			return fmt.Errorf("failed to revert users to free plan: %w", err)
		}

		rowsAffected, _ := result.RowsAffected()
		fmt.Printf("[SUBSCRIPTION.HALTED] ✓ Expired subscription, reverted %d user(s) to free plan\n", rowsAffected)

		// Log the expiration event
		metadata := map[string]interface{}{
			"subscription_id": sub.ID,
			"status":          "expired",
			"event":           event.Event,
			"reason":          "halted_past_expiry",
			"users_affected":  rowsAffected,
			"period_end":      currentPeriodEnd,
		}
		metadataJSON, _ := json.Marshal(metadata)
		_, err = tx.Exec(`
			INSERT INTO license_log (
				user_id, org_id, event_type, description, metadata, created_at
			) VALUES ($1, $2, $3, $4, $5, NOW())`,
			ownerUserID, orgID, "subscription_expired",
			fmt.Sprintf("Subscription halted and expired (payment failed), %d user(s) reverted to free plan", rowsAffected),
			metadataJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to log event: %w", err)
		}

		fmt.Printf("[SUBSCRIPTION.HALTED] ✓ SUCCESS: Subscription %s expired due to payment failure\n", sub.ID)
	} else {
		fmt.Printf("[SUBSCRIPTION.HALTED] ⚠ Subscription halted but not expired yet (period_end: %s), keeping users active\n",
			currentPeriodEnd.Format("2006-01-02 15:04:05"))

		// Update status to halted but keep users active until period_end
		_, err = tx.Exec(`
			UPDATE subscriptions
			SET status = 'halted',
			    updated_at = NOW()
			WHERE razorpay_subscription_id = $1`,
			sub.ID,
		)
		if err != nil {
			return fmt.Errorf("failed to update subscription to halted: %w", err)
		}

		// Log the halted event
		metadata := map[string]interface{}{
			"subscription_id":   sub.ID,
			"status":            "halted",
			"event":             event.Event,
			"period_end":        currentPeriodEnd,
			"days_until_expiry": int(currentPeriodEnd.Sub(now).Hours() / 24),
		}
		metadataJSON, _ := json.Marshal(metadata)
		_, err = tx.Exec(`
			INSERT INTO license_log (
				user_id, org_id, event_type, description, metadata, created_at
			) VALUES ($1, $2, $3, $4, $5, NOW())`,
			ownerUserID, orgID, "subscription_halted",
			fmt.Sprintf("Subscription halted (payment failed), will expire on %s if not resolved", currentPeriodEnd.Format("2006-01-02")),
			metadataJSON,
		)
		if err != nil {
			return fmt.Errorf("failed to log event: %w", err)
		}

		fmt.Printf("[SUBSCRIPTION.HALTED] ✓ Subscription %s marked as halted, users retain access until %s\n",
			sub.ID, currentPeriodEnd.Format("2006-01-02"))
	}

	return tx.Commit()
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

	if handled, err := h.tryHandleUpgradeOrderPaymentAuthorized(payment, event); err != nil {
		return err
	} else if handled {
		return nil
	}

	// Find subscription by invoice ID or order ID from payment notes
	subscriptionID, err := h.findSubscriptionFromPayment(payment)
	if err != nil {
		fmt.Printf("[PAYMENT.AUTHORIZED] ⚠ Could not find subscription for payment %s: %v (recording pending reconciliation)\n", payment.ID, err)
		return h.logPaymentAuthorizedWithoutSubscription(payment, event)
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

func (h *RazorpayWebhookHandler) tryHandleUpgradeOrderPaymentAuthorized(payment *RazorpayPayment, event *RazorpayWebhookEvent) (bool, error) {
	requestStore := storagepayment.NewUpgradeRequestStore(h.db)
	request, err := h.lookupUpgradeRequestForPayment(context.Background(), requestStore, payment)
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("lookup upgrade request for authorized webhook: %w", err)
	}

	orderID := strings.TrimSpace(payment.OrderID)
	if orderID != "" {
		_, _ = h.db.Exec(`
			UPDATE upgrade_payment_attempts
			SET razorpay_payment_id = COALESCE(NULLIF($2, ''), razorpay_payment_id),
			    updated_at = NOW()
			WHERE razorpay_order_id = $1`,
			orderID,
			strings.TrimSpace(payment.ID),
		)
	}

	metadata := map[string]interface{}{
		"upgrade_request_id": request.UpgradeRequestID,
		"payment_id":         payment.ID,
		"order_id":           payment.OrderID,
		"amount":             payment.Amount,
		"currency":           payment.Currency,
		"status":             payment.Status,
		"event":              event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, _ = h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, $1, $2, $3, $4, NOW())`,
		request.OrgID,
		"upgrade_payment_authorized",
		fmt.Sprintf("Upgrade payment %s authorized for request %s", payment.ID, request.UpgradeRequestID),
		metadataJSON,
	)

	fmt.Printf("[PAYMENT.AUTHORIZED] ✓ Upgrade payment authorized (org=%d, request=%s, order=%s)\n", request.OrgID, request.UpgradeRequestID, payment.OrderID)
	return true, nil
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

	if handled, err := h.tryHandleUpgradeOrderPaymentCaptured(payment, event); err != nil {
		return err
	} else if handled {
		return nil
	}

	// Find subscription by invoice ID or order ID from payment notes
	subscriptionID, err := h.findSubscriptionFromPayment(payment)
	if err != nil {
		fmt.Printf("[PAYMENT.CAPTURED] ⚠ Could not find subscription for payment %s: %v (recording pending reconciliation)\n", payment.ID, err)
		return h.logPaymentCapturedWithoutSubscription(payment, event)
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

	if handled, err := h.tryHandleUpgradeOrderPaymentFailure(payment, event); err != nil {
		return err
	} else if handled {
		return nil
	}

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
	razorpayMode := strings.TrimSpace(os.Getenv("RAZORPAY_MODE"))
	if razorpayMode == "" {
		razorpayMode = "test"
	}

	findByQuery := func(query string, args ...interface{}) (int64, error) {
		var id int64
		err := h.db.QueryRow(query, args...).Scan(&id)
		if err != nil {
			return 0, err
		}
		return id, nil
	}

	// Try to find by invoice ID (most reliable for subscription payments)
	if payment.InvoiceID != "" {
		if id, err := findByQuery(`
			SELECT sp.subscription_id
			FROM subscription_payments sp
			WHERE sp.razorpay_invoice_id = $1
			  AND sp.subscription_id IS NOT NULL
			ORDER BY sp.updated_at DESC, sp.created_at DESC
			LIMIT 1`, payment.InvoiceID); err == nil {
			return id, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ invoice_id lookup via subscription_payments failed: %v\n", err)
		}

		if id, err := findByQuery(`
			SELECT s.id
			FROM subscriptions s
			WHERE s.razorpay_data->>'invoice_id' = $1
			LIMIT 1`, payment.InvoiceID); err == nil {
			return id, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ invoice_id lookup via subscriptions.razorpay_data failed: %v\n", err)
		}

		if invoice, err := GetInvoiceByID(razorpayMode, payment.InvoiceID); err == nil {
			if subID := strings.TrimSpace(invoice.SubscriptionID); subID != "" {
				if id, lookupErr := findByQuery(`
					SELECT s.id
					FROM subscriptions s
					WHERE s.razorpay_subscription_id = $1
					LIMIT 1`, subID); lookupErr == nil {
					return id, nil
				} else if !errors.Is(lookupErr, sql.ErrNoRows) {
					fmt.Printf("[PAYMENT.CORRELATION] ⚠ invoice subscription lookup failed: %v\n", lookupErr)
				}
			}
		} else {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ invoice API lookup failed for %s: %v\n", payment.InvoiceID, err)
		}
	}

	// Try to find by order ID
	if payment.OrderID != "" {
		if id, err := findByQuery(`
			SELECT sp.subscription_id
			FROM subscription_payments sp
			WHERE sp.razorpay_order_id = $1
			  AND sp.subscription_id IS NOT NULL
			ORDER BY sp.updated_at DESC, sp.created_at DESC
			LIMIT 1`, payment.OrderID); err == nil {
			return id, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ order_id lookup via subscription_payments failed: %v\n", err)
		}

		if id, err := findByQuery(`
			SELECT s.id
			FROM subscriptions s
			WHERE s.razorpay_data->>'order_id' = $1
			LIMIT 1`, payment.OrderID); err == nil {
			return id, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ order_id lookup via subscriptions.razorpay_data failed: %v\n", err)
		}
	}

	// Try to find by notes in payment
	notes := payment.GetPaymentNotesMap()
	if notes != nil {
		if subID, ok := notes["subscription_id"]; ok {
			if id, err := findByQuery(`
				SELECT s.id
				FROM subscriptions s
				WHERE s.razorpay_subscription_id = $1
				LIMIT 1`, subID); err == nil {
				return id, nil
			} else if !errors.Is(err, sql.ErrNoRows) {
				fmt.Printf("[PAYMENT.CORRELATION] ⚠ subscription_id note lookup failed: %v\n", err)
			}
		}
	}

	// Try to find by customer_id in persisted payload fields.
	if payment.CustomerID != "" {
		if id, err := findByQuery(`
			SELECT s.id
			FROM subscriptions s
			WHERE (s.razorpay_data->>'customer_id' = $1 OR s.notes->>'customer_id' = $1)
			ORDER BY
				CASE WHEN lower(s.status) IN ('active', 'authenticated') THEN 0 ELSE 1 END,
				s.updated_at DESC,
				s.created_at DESC
			LIMIT 1`, payment.CustomerID); err == nil {
			return id, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ customer_id payload lookup failed: %v\n", err)
		}
	}

	// Final fallback: match owner email to subscription owner if available.
	if email := strings.TrimSpace(payment.Email); email != "" {
		if id, err := findByQuery(`
			SELECT s.id
			FROM subscriptions s
			JOIN users u ON u.id = s.owner_user_id
			WHERE lower(u.email) = lower($1)
			ORDER BY
				CASE WHEN lower(s.status) IN ('active', 'authenticated') THEN 0 ELSE 1 END,
				s.updated_at DESC,
				s.created_at DESC
			LIMIT 1`, email); err == nil {
			return id, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("[PAYMENT.CORRELATION] ⚠ owner email lookup failed: %v\n", err)
		}
	}

	return 0, fmt.Errorf("could not find subscription for payment %s (customer: %s, invoice: %s, order: %s)",
		payment.ID, payment.CustomerID, payment.InvoiceID, payment.OrderID)
}

func (h *RazorpayWebhookHandler) tryHandleUpgradeOrderPaymentCaptured(payment *RazorpayPayment, event *RazorpayWebhookEvent) (bool, error) {
	requestStore := storagepayment.NewUpgradeRequestStore(h.db)
	request, err := h.lookupUpgradeRequestForPayment(context.Background(), requestStore, payment)
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("lookup upgrade request for captured webhook: %w", err)
	}

	orderID := strings.TrimSpace(payment.OrderID)
	attemptStore := storagepayment.NewUpgradePaymentAttemptStore(h.db)
	if orderID != "" {
		if err := attemptStore.MarkPaymentCapturedByOrderID(context.Background(), orderID, payment.ID); err != nil {
			if !errors.Is(err, storagepayment.ErrUpgradePaymentAttemptNotFound) {
				return false, fmt.Errorf("mark upgrade payment attempt captured from webhook: %w", err)
			}
		}
	}

	if _, err := requestStore.MarkPaymentCaptureConfirmed(context.Background(), storagepayment.MarkUpgradePaymentCaptureInput{
		UpgradeRequestID:  request.UpgradeRequestID,
		RazorpayPaymentID: strings.TrimSpace(payment.ID),
		RazorpayOrderID:   orderID,
		Metadata: map[string]interface{}{
			"source":     "webhook.payment.captured",
			"event":      event.Event,
			"amount":     payment.Amount,
			"currency":   payment.Currency,
			"payment_id": payment.ID,
			"order_id":   payment.OrderID,
			"status":     payment.Status,
		},
	}); err != nil && !errors.Is(err, storagepayment.ErrUpgradeRequestTransitionRejected) {
		return false, fmt.Errorf("mark upgrade request payment captured from webhook: %w", err)
	}

	metadata := map[string]interface{}{
		"upgrade_request_id": request.UpgradeRequestID,
		"payment_id":         payment.ID,
		"order_id":           payment.OrderID,
		"amount":             payment.Amount,
		"currency":           payment.Currency,
		"status":             payment.Status,
		"event":              event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, _ = h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, $1, $2, $3, $4, NOW())`,
		request.OrgID,
		"upgrade_payment_captured",
		fmt.Sprintf("Upgrade payment %s captured for request %s", payment.ID, request.UpgradeRequestID),
		metadataJSON,
	)

	fmt.Printf("[PAYMENT.CAPTURED] ✓ Upgrade payment captured (org=%d, request=%s, order=%s)\n", request.OrgID, request.UpgradeRequestID, payment.OrderID)
	return true, nil
}

func (h *RazorpayWebhookHandler) tryHandleUpgradeOrderPaymentFailure(payment *RazorpayPayment, event *RazorpayWebhookEvent) (bool, error) {
	requestStore := storagepayment.NewUpgradeRequestStore(h.db)
	request, err := h.lookupUpgradeRequestForPayment(context.Background(), requestStore, payment)
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("lookup upgrade request for failed webhook: %w", err)
	}

	orderID := strings.TrimSpace(payment.OrderID)
	attemptStore := storagepayment.NewUpgradePaymentAttemptStore(h.db)
	if orderID != "" {
		if err := attemptStore.MarkPaymentFailedByOrderID(context.Background(), storagepayment.MarkUpgradePaymentFailedInput{
			RazorpayOrderID:   orderID,
			RazorpayPaymentID: payment.ID,
			ErrorCode:         payment.ErrorCode,
			ErrorReason:       payment.ErrorReason,
			ErrorDescription:  payment.ErrorDescription,
			ErrorSource:       payment.ErrorSource,
			ErrorStep:         payment.ErrorStep,
		}); err != nil {
			if !errors.Is(err, storagepayment.ErrUpgradePaymentAttemptNotFound) {
				return false, fmt.Errorf("mark upgrade payment attempt failed from webhook: %w", err)
			}
		}
	}

	updatedRequest, reqErr := requestStore.MarkUpgradeRequestFailed(context.Background(), storagepayment.MarkUpgradeRequestFailedInput{
		UpgradeRequestID: request.UpgradeRequestID,
		FailureReason:    fmt.Sprintf("payment_failed:%s:%s", strings.TrimSpace(payment.ErrorCode), strings.TrimSpace(payment.ErrorDescription)),
		Metadata: map[string]interface{}{
			"source":            "webhook.payment.failed",
			"payment_id":        payment.ID,
			"order_id":          payment.OrderID,
			"error_code":        payment.ErrorCode,
			"error_reason":      payment.ErrorReason,
			"error_description": payment.ErrorDescription,
			"error_source":      payment.ErrorSource,
			"error_step":        payment.ErrorStep,
		},
	})
	if reqErr == nil {
		h.enqueueUpgradeFailureNotifications(context.Background(), updatedRequest, map[string]interface{}{
			"source":            "webhook.payment.failed",
			"payment_id":        payment.ID,
			"order_id":          payment.OrderID,
			"error_code":        payment.ErrorCode,
			"error_reason":      payment.ErrorReason,
			"error_description": payment.ErrorDescription,
			"error_source":      payment.ErrorSource,
			"error_step":        payment.ErrorStep,
		})
	}

	metadata := map[string]interface{}{
		"upgrade_request_id": request.UpgradeRequestID,
		"payment_id":         payment.ID,
		"order_id":           payment.OrderID,
		"amount":             payment.Amount,
		"currency":           payment.Currency,
		"status":             payment.Status,
		"error_code":         payment.ErrorCode,
		"error_reason":       payment.ErrorReason,
		"error_description":  payment.ErrorDescription,
		"error_source":       payment.ErrorSource,
		"error_step":         payment.ErrorStep,
		"event":              event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, _ = h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, $1, $2, $3, $4, NOW())`,
		request.OrgID,
		"upgrade_payment_failed",
		fmt.Sprintf("Upgrade payment %s failed for request %s: %s", payment.ID, request.UpgradeRequestID, payment.ErrorDescription),
		metadataJSON,
	)

	fmt.Printf("[PAYMENT.FAILED] ✓ Upgrade payment failure recorded (org=%d, request=%s, order=%s)\n", request.OrgID, request.UpgradeRequestID, payment.OrderID)
	return true, nil
}

func (h *RazorpayWebhookHandler) enqueueUpgradeFailureNotifications(ctx context.Context, request storagepayment.UpgradeRequest, metadata map[string]interface{}) {
	store := storagepayment.NewBillingNotificationOutboxStore(h.db)

	payload := map[string]interface{}{
		"upgrade_request_id": request.UpgradeRequestID,
		"org_id":             request.OrgID,
		"from_plan_code":     request.FromPlanCode,
		"to_plan_code":       request.ToPlanCode,
		"status":             request.CurrentStatus,
		"event_type":         "upgrade_payment_failed",
		"support_reference":  request.UpgradeRequestID,
		"triggered_at":       time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range metadata {
		payload[k] = v
	}

	var recipientUserID *int64
	if request.ActorUserID > 0 {
		uid := request.ActorUserID
		recipientUserID = &uid
	}

	base := fmt.Sprintf("upgrade_payment_failed:%s", strings.TrimSpace(request.UpgradeRequestID))
	if _, err := store.Enqueue(ctx, storagepayment.CreateBillingNotificationInput{
		OrgID:           request.OrgID,
		EventType:       "upgrade_payment_failed",
		Channel:         "in_app",
		DedupeKey:       base + ":in_app",
		Payload:         payload,
		RecipientUserID: recipientUserID,
	}); err != nil {
		fmt.Printf("[PAYMENT.FAILED] warning: enqueue in_app notification failed request=%s: %v\n", request.UpgradeRequestID, err)
	}

	if recipientUserID == nil {
		return
	}
	email, err := store.GetUserEmailByID(ctx, *recipientUserID)
	if err != nil {
		fmt.Printf("[PAYMENT.FAILED] warning: resolve recipient email failed request=%s user=%d: %v\n", request.UpgradeRequestID, *recipientUserID, err)
		return
	}
	if strings.TrimSpace(email) == "" {
		return
	}

	if _, err := store.Enqueue(ctx, storagepayment.CreateBillingNotificationInput{
		OrgID:           request.OrgID,
		EventType:       "upgrade_payment_failed",
		Channel:         "email",
		DedupeKey:       base + ":email",
		Payload:         payload,
		RecipientUserID: recipientUserID,
		RecipientEmail:  email,
	}); err != nil {
		fmt.Printf("[PAYMENT.FAILED] warning: enqueue email notification failed request=%s: %v\n", request.UpgradeRequestID, err)
	}
}

func (h *RazorpayWebhookHandler) lookupUpgradeRequestForPayment(ctx context.Context, requestStore *storagepayment.UpgradeRequestStore, payment *RazorpayPayment) (storagepayment.UpgradeRequest, error) {
	if requestStore == nil {
		requestStore = storagepayment.NewUpgradeRequestStore(h.db)
	}

	if notes := payment.GetPaymentNotesMap(); notes != nil {
		if requestID := strings.TrimSpace(notes["upgrade_request_id"]); requestID != "" {
			return requestStore.GetUpgradeRequestByID(ctx, requestID)
		}
	}

	if orderID := strings.TrimSpace(payment.OrderID); orderID != "" {
		request, err := requestStore.GetUpgradeRequestByOrderID(ctx, orderID)
		if err == nil {
			return request, nil
		}
		if !errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return storagepayment.UpgradeRequest{}, fmt.Errorf("lookup upgrade request by order id: %w", err)
		}

		// If request-level order correlation is missing, fall back to deterministic
		// attempt-level order correlation to recover the owning upgrade request.
		attemptStore := storagepayment.NewUpgradePaymentAttemptStore(h.db)
		attempt, attemptErr := attemptStore.GetAttemptByOrderID(ctx, orderID)
		if attemptErr != nil {
			if errors.Is(attemptErr, storagepayment.ErrUpgradePaymentAttemptNotFound) {
				return storagepayment.UpgradeRequest{}, storagepayment.ErrUpgradeRequestNotFound
			}
			return storagepayment.UpgradeRequest{}, fmt.Errorf("lookup upgrade payment attempt by order id: %w", attemptErr)
		}

		if !attempt.UpgradeRequestID.Valid || strings.TrimSpace(attempt.UpgradeRequestID.String) == "" {
			return storagepayment.UpgradeRequest{}, storagepayment.ErrUpgradeRequestNotFound
		}

		requestID := strings.TrimSpace(attempt.UpgradeRequestID.String)
		return requestStore.GetUpgradeRequestByID(ctx, requestID)
	}

	return storagepayment.UpgradeRequest{}, storagepayment.ErrUpgradeRequestNotFound
}

func (h *RazorpayWebhookHandler) tryMarkUpgradeRequestSubscriptionConfirmed(razorpaySubscriptionID string, eventName string, metadata map[string]interface{}) (bool, error) {
	requestStore := storagepayment.NewUpgradeRequestStore(h.db)
	request, err := requestStore.GetLatestPendingByRazorpaySubscriptionID(context.Background(), strings.TrimSpace(razorpaySubscriptionID))
	if err != nil {
		if errors.Is(err, storagepayment.ErrUpgradeRequestNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("load pending upgrade request by razorpay subscription id: %w", err)
	}

	payload := metadata
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["source_event"] = eventName
	payload["razorpay_subscription_id"] = strings.TrimSpace(razorpaySubscriptionID)

	_, err = requestStore.MarkSubscriptionChangeConfirmed(context.Background(), storagepayment.MarkUpgradeSubscriptionConfirmedInput{
		UpgradeRequestID:       request.UpgradeRequestID,
		RazorpaySubscriptionID: strings.TrimSpace(razorpaySubscriptionID),
		Metadata:               payload,
	})
	if err != nil && !errors.Is(err, storagepayment.ErrUpgradeRequestTransitionRejected) {
		return false, err
	}

	return true, nil
}

func (h *RazorpayWebhookHandler) logPaymentCapturedWithoutSubscription(payment *RazorpayPayment, event *RazorpayWebhookEvent) error {
	paymentDataJSON, _ := json.Marshal(payment)
	metadata := map[string]interface{}{
		"payment_id": payment.ID,
		"amount":     payment.Amount,
		"currency":   payment.Currency,
		"status":     payment.Status,
		"order_id":   payment.OrderID,
		"invoice_id": payment.InvoiceID,
		"event":      event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)

	_, err := h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, NULL, $1, $2, $3, NOW())`,
		"payment_captured_pending_reconciliation",
		fmt.Sprintf("Payment %s captured but subscription correlation pending", payment.ID),
		metadataJSON,
	)

	_, _ = h.db.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
			amount, currency, status, method, captured_at,
			razorpay_data, created_at, updated_at
		) VALUES (NULL, $1, $2, $3, $4, $5, $6, $7, NOW(), $8, NOW(), NOW())
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		payment.ID, payment.OrderID, payment.InvoiceID,
		payment.Amount, payment.Currency, payment.Status, payment.Method,
		paymentDataJSON,
	)

	return err
}

func (h *RazorpayWebhookHandler) logPaymentAuthorizedWithoutSubscription(payment *RazorpayPayment, event *RazorpayWebhookEvent) error {
	paymentDataJSON, _ := json.Marshal(payment)
	metadata := map[string]interface{}{
		"payment_id": payment.ID,
		"amount":     payment.Amount,
		"currency":   payment.Currency,
		"status":     payment.Status,
		"order_id":   payment.OrderID,
		"invoice_id": payment.InvoiceID,
		"event":      event.Event,
	}
	metadataJSON, _ := json.Marshal(metadata)

	_, err := h.db.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES (NULL, NULL, $1, $2, $3, NOW())`,
		"payment_authorized_pending_reconciliation",
		fmt.Sprintf("Payment %s authorized but subscription correlation pending", payment.ID),
		metadataJSON,
	)

	_, _ = h.db.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, razorpay_order_id, razorpay_invoice_id,
			amount, currency, status, method, authorized_at,
			razorpay_data, created_at, updated_at
		) VALUES (NULL, $1, $2, $3, $4, $5, $6, $7, NOW(), $8, NOW(), NOW())
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		payment.ID, payment.OrderID, payment.InvoiceID,
		payment.Amount, payment.Currency, payment.Status, payment.Method,
		paymentDataJSON,
	)

	return err
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

// handleSubscriptionExpired handles subscription expiration
// This is sent by Razorpay when a subscription expires (either naturally or after cancellation period ends)
func (h *RazorpayWebhookHandler) handleSubscriptionExpired(event *RazorpayWebhookEvent) error {
	sub, err := h.extractSubscription(event)
	if err != nil {
		return err
	}

	fmt.Printf("[SUBSCRIPTION.EXPIRED] Processing expiration for subscription: %s\n", sub.ID)

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

	// Update subscription status to expired
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = 'expired',
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $1`,
		sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	fmt.Printf("[SUBSCRIPTION.EXPIRED] ✓ Updated subscription %s to expired status\n", sub.ID)

	// Revert ALL users on this subscription to free plan
	result, err := tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'free',
		    license_expires_at = NULL,
		    active_subscription_id = NULL,
		    updated_at = NOW()
		WHERE active_subscription_id = (
			SELECT id FROM subscriptions WHERE razorpay_subscription_id = $1
		)`,
		sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to revert users to free plan: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("[SUBSCRIPTION.EXPIRED] ✓ Reverted %d user(s) to free plan\n", rowsAffected)

	// Log the event
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"status":          "expired",
		"event":           event.Event,
		"users_affected":  rowsAffected,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_expired",
		fmt.Sprintf("Subscription expired, %d user(s) reverted to free plan", rowsAffected),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}

	fmt.Printf("[SUBSCRIPTION.EXPIRED] ✓ SUCCESS: Subscription %s expired and users downgraded\n", sub.ID)

	return tx.Commit()
}
