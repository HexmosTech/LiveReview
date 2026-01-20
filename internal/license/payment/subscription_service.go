package payment

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

// GetPlanID returns the appropriate Razorpay plan ID based on mode and plan type
// Reads from environment variables for easy test/prod switching
func GetPlanID(mode, planType string) string {
	if mode == "test" {
		if planType == "monthly" {
			return os.Getenv("RAZORPAY_TEST_MONTHLY_PLAN_ID")
		}
		return os.Getenv("RAZORPAY_TEST_YEARLY_PLAN_ID")
	}
	// live mode
	if planType == "monthly" {
		return os.Getenv("RAZORPAY_LIVE_MONTHLY_PLAN_ID")
	}
	return os.Getenv("RAZORPAY_LIVE_YEARLY_PLAN_ID")
}

// SubscriptionService handles business logic for subscriptions, wrapping the payment package
type SubscriptionService struct {
	db *sql.DB
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService(db *sql.DB) *SubscriptionService {
	return &SubscriptionService{db: db}
}

// CreateTeamSubscription creates a new subscription via Razorpay and persists to DB
func (s *SubscriptionService) CreateTeamSubscription(ownerUserID, orgID int, planType string, quantity int, mode string) (*RazorpaySubscription, error) {
	// Validate plan type
	if planType != "monthly" && planType != "yearly" {
		return nil, fmt.Errorf("invalid plan type: %s (must be monthly or yearly)", planType)
	}

	// Get the corresponding Razorpay plan ID based on mode
	razorpayPlanID := GetPlanID(mode, planType)

	if razorpayPlanID == "" {
		return nil, fmt.Errorf("razorpay plan ID not configured for %s in %s mode", planType, mode)
	}

	// Create notes for the subscription
	notes := map[string]string{
		"owner_user_id": fmt.Sprintf("%d", ownerUserID),
		"org_id":        fmt.Sprintf("%d", orgID),
		"plan_type":     "team_" + planType, // Store as team_monthly or team_yearly
	}

	// Create subscription in Razorpay
	sub, err := CreateSubscription(mode, razorpayPlanID, quantity, notes)
	if err != nil {
		return nil, fmt.Errorf("failed to create razorpay subscription: %w", err)
	}

	// Calculate license expiration (30 days for monthly, 365 days for yearly)
	var licenseExpiresAt time.Time
	dbPlanType := "team_" + planType // Store as team_monthly or team_yearly in DB

	// Calculate current period start and end
	// For new subscriptions, Razorpay returns 0 for current_start/current_end
	// We'll use the creation time as start and calculate end based on plan type
	var currentPeriodStart, currentPeriodEnd time.Time
	if sub.CurrentStart > 0 {
		currentPeriodStart = time.Unix(sub.CurrentStart, 0)
	} else {
		currentPeriodStart = time.Now()
	}

	if sub.CurrentEnd > 0 {
		currentPeriodEnd = time.Unix(sub.CurrentEnd, 0)
	} else {
		// Calculate based on plan type
		if planType == "monthly" {
			currentPeriodEnd = currentPeriodStart.AddDate(0, 1, 0) // 1 month
		} else {
			currentPeriodEnd = currentPeriodStart.AddDate(1, 0, 0) // 1 year
		}
	}

	if planType == "monthly" {
		licenseExpiresAt = currentPeriodEnd
	} else {
		licenseExpiresAt = currentPeriodEnd
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into subscriptions table
	notesJSON, _ := json.Marshal(notes)
	var dbSubscriptionID int64
	err = tx.QueryRow(`
		INSERT INTO subscriptions (
			razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			current_period_start, current_period_end, license_expires_at,
			short_url, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		RETURNING id`,
		sub.ID, ownerUserID, orgID, dbPlanType,
		quantity, 0, sub.Status, razorpayPlanID,
		currentPeriodStart, currentPeriodEnd, licenseExpiresAt,
		sub.ShortURL, notesJSON,
	).Scan(&dbSubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert subscription: %w", err)
	}

	// Note: We do NOT automatically assign the license to the purchaser
	// The owner must explicitly assign licenses via the assignment UI

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"plan_id":         razorpayPlanID,
		"quantity":        quantity,
		"status":          sub.Status,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_created",
		fmt.Sprintf("Created %s subscription with %d seats", dbPlanType, quantity),
		metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to log subscription creation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return sub, nil
}

// UpdateQuantity updates the quantity of an existing subscription
func (s *SubscriptionService) UpdateQuantity(subscriptionID string, quantity int, scheduleChangeAt int64, mode string) (*RazorpaySubscription, error) {
	// Update in Razorpay
	sub, err := UpdateSubscriptionQuantity(mode, subscriptionID, quantity, scheduleChangeAt)
	if err != nil {
		return nil, fmt.Errorf("failed to update razorpay subscription: %w", err)
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current subscription details for logging
	var ownerUserID, orgID int
	var planType string
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id, plan_type
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&ownerUserID, &orgID, &planType)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription details: %w", err)
	}

	// Update subscriptions table
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET quantity = $1,
		    status = $2,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $3`,
		sub.Quantity, sub.Status, subscriptionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id":      subscriptionID,
		"new_quantity":         quantity,
		"schedule_change_at":   scheduleChangeAt,
		"has_scheduled_change": sub.HasScheduledChanges,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_quantity_updated",
		fmt.Sprintf("Updated subscription quantity to %d", quantity),
		metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to log quantity update: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return sub, nil
}

// CancelSubscription cancels an existing subscription
func (s *SubscriptionService) CancelSubscription(subscriptionID string, immediate bool, mode string) (*RazorpaySubscription, error) {
	// Cancel in Razorpay
	sub, err := CancelSubscription(mode, subscriptionID, !immediate)
	if err != nil {
		return nil, fmt.Errorf("failed to cancel razorpay subscription: %w", err)
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current subscription details for logging
	var ownerUserID, orgID int
	var planType string
	var licenseExpiresAt time.Time
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id, plan_type, license_expires_at
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&ownerUserID, &orgID, &planType, &licenseExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription details: %w", err)
	}

	// Update subscriptions table
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    cancel_at_period_end = $2,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $3`,
		sub.Status, !immediate, subscriptionID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update subscription: %w", err)
	}

	// If immediate cancellation, update user_roles to revert to free plan
	if immediate {
		_, err = tx.Exec(`
			UPDATE user_roles
			SET plan_type = 'free',
			    license_expires_at = NULL,
			    active_subscription_id = NULL,
			    updated_at = NOW()
			WHERE user_id = $1 AND org_id = $2`,
			ownerUserID, orgID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update user_roles: %w", err)
		}
	}

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id": subscriptionID,
		"immediate":       immediate,
		"status":          sub.Status,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_cancelled",
		fmt.Sprintf("Cancelled subscription (immediate: %t)", immediate),
		metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to log cancellation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return sub, nil
}

// SubscriptionDetails holds subscription info from both DB and Razorpay
type SubscriptionDetails struct {
	// From DB
	ID                     int       `json:"id"`
	RazorpaySubscriptionID string    `json:"razorpay_subscription_id"`
	OwnerUserID            int       `json:"owner_user_id"`
	OrgID                  int       `json:"org_id"`
	PlanType               string    `json:"plan_type"`
	Quantity               int       `json:"quantity"`
	AssignedSeats          int       `json:"assigned_seats"`
	Status                 string    `json:"status"`
	LicenseExpiresAt       time.Time `json:"license_expires_at"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
	// Payment Info
	PaymentVerified       bool       `json:"payment_verified"`
	LastPaymentID         string     `json:"last_payment_id,omitempty"`
	LastPaymentStatus     string     `json:"last_payment_status,omitempty"`
	LastPaymentReceivedAt *time.Time `json:"last_payment_received_at,omitempty"`
	// From Razorpay
	RazorpaySubscription *RazorpaySubscription `json:"razorpay_subscription,omitempty"`
}

// GetSubscriptionDetails retrieves subscription details from both DB and Razorpay
func (s *SubscriptionService) GetSubscriptionDetails(subscriptionID string, mode string) (*SubscriptionDetails, error) {
	// Get from DB with calculated assigned_seats from user_roles AND payment info
	var details SubscriptionDetails
	var lastPaymentID sql.NullString
	var lastPaymentStatus sql.NullString
	var lastPaymentReceivedAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT s.id, s.razorpay_subscription_id, s.owner_user_id, s.org_id, s.plan_type, s.quantity,
		       COALESCE((SELECT COUNT(*) FROM user_roles ur WHERE ur.active_subscription_id = s.id AND ur.plan_type = 'team'), 0) as assigned_seats,
		       s.status, s.license_expires_at, s.created_at, s.updated_at,
		       s.payment_verified, s.last_payment_id, s.last_payment_status, s.last_payment_received_at
		FROM subscriptions s
		WHERE s.razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(
		&details.ID, &details.RazorpaySubscriptionID, &details.OwnerUserID, &details.OrgID, &details.PlanType,
		&details.Quantity, &details.AssignedSeats, &details.Status,
		&details.LicenseExpiresAt, &details.CreatedAt, &details.UpdatedAt,
		&details.PaymentVerified, &lastPaymentID, &lastPaymentStatus, &lastPaymentReceivedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subscription not found: %s", subscriptionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription from DB: %w", err)
	}

	// Set nullable payment fields
	if lastPaymentID.Valid {
		details.LastPaymentID = lastPaymentID.String
	}
	if lastPaymentStatus.Valid {
		details.LastPaymentStatus = lastPaymentStatus.String
	}
	if lastPaymentReceivedAt.Valid {
		details.LastPaymentReceivedAt = &lastPaymentReceivedAt.Time
	}

	// Get from Razorpay
	sub, err := GetSubscriptionByID(mode, subscriptionID)
	if err != nil {
		// If Razorpay call fails, still return DB data
		return &details, nil
	}
	details.RazorpaySubscription = sub

	return &details, nil
}

// AssignLicense assigns a license to a user in the subscription
func (s *SubscriptionService) AssignLicense(subscriptionID string, userID, orgID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check subscription capacity and payment verification
	var quantity int
	var ownerUserID int
	var dbSubscriptionID int64
	var licenseExpiresAt time.Time
	var assignedSeats int
	var paymentVerified bool
	var lastPaymentStatus sql.NullString
	err = tx.QueryRow(`
		SELECT s.id, s.quantity, s.owner_user_id, s.license_expires_at, s.payment_verified, s.last_payment_status,
		       COALESCE((SELECT COUNT(*) FROM user_roles ur WHERE ur.active_subscription_id = s.id AND ur.plan_type = 'team'), 0) as assigned_seats
		FROM subscriptions s
		WHERE s.razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&dbSubscriptionID, &quantity, &ownerUserID, &licenseExpiresAt, &paymentVerified, &lastPaymentStatus, &assignedSeats)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// CRITICAL: Block assignment until payment is verified
	if !paymentVerified {
		return fmt.Errorf("payment pending - licenses cannot be assigned until payment is received. Check back in 5-10 minutes")
	}

	if assignedSeats >= quantity {
		return fmt.Errorf("subscription at capacity: %d/%d seats used", assignedSeats, quantity)
	}

	// Check if user already has an active subscription in this org
	var existingSubID sql.NullInt64
	var existingRazorpaySubID sql.NullString
	err = tx.QueryRow(`
		SELECT ur.active_subscription_id, s.razorpay_subscription_id
		FROM user_roles ur
		LEFT JOIN subscriptions s ON ur.active_subscription_id = s.id
		WHERE ur.user_id = $1 AND ur.org_id = $2 AND ur.plan_type = 'team'`,
		userID, orgID,
	).Scan(&existingSubID, &existingRazorpaySubID)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing subscription: %w", err)
	}

	if existingSubID.Valid && existingSubID.Int64 != dbSubscriptionID {
		return fmt.Errorf("user already has an active license from subscription %s - please revoke that first", existingRazorpaySubID.String)
	}

	// No need to increment assigned_seats counter - we calculate it dynamically

	// Update user_roles
	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'team',
		    license_expires_at = $1,
		    active_subscription_id = $2,
		    updated_at = NOW()
		WHERE user_id = $3 AND org_id = $4`,
		licenseExpiresAt, dbSubscriptionID, userID, orgID,
	)
	if err != nil {
		// Handle case where user_roles entry doesn't exist yet
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23503" {
			// Insert new user_roles entry
			_, err = tx.Exec(`
				INSERT INTO user_roles (
					user_id, org_id, role_id, plan_type, license_expires_at, active_subscription_id, created_at, updated_at
				) VALUES ($1, $2, 3, 'team', $3, $4, NOW(), NOW())`,
				userID, orgID, licenseExpiresAt, dbSubscriptionID,
			)
			if err != nil {
				return fmt.Errorf("failed to create user_roles: %w", err)
			}
		} else {
			return fmt.Errorf("failed to update user_roles: %w", err)
		}
	}

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id": subscriptionID,
		"assigned_to":     userID,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		userID, orgID, "license_assigned",
		fmt.Sprintf("License assigned from subscription %s", subscriptionID),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log license assignment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RevokeLicense removes a license from a user
func (s *SubscriptionService) RevokeLicense(subscriptionID string, userID, orgID int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the database ID for this subscription
	var dbSubscriptionID int64
	err = tx.QueryRow(`
		SELECT id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&dbSubscriptionID)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	// Verify user has this subscription
	var currentSubID sql.NullInt64
	err = tx.QueryRow(`
		SELECT active_subscription_id
		FROM user_roles
		WHERE user_id = $1 AND org_id = $2`,
		userID, orgID,
	).Scan(&currentSubID)
	if err != nil {
		return fmt.Errorf("failed to get user_roles: %w", err)
	}

	if !currentSubID.Valid || currentSubID.Int64 != dbSubscriptionID {
		return fmt.Errorf("user %d does not have subscription %s", userID, subscriptionID)
	}

	// No need to decrement assigned_seats counter - we calculate it dynamically

	// Revert user to free plan
	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'free',
		    license_expires_at = NULL,
		    active_subscription_id = NULL,
		    updated_at = NOW()
		WHERE user_id = $1 AND org_id = $2`,
		userID, orgID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user_roles: %w", err)
	}

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id": subscriptionID,
		"revoked_from":    userID,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		userID, orgID, "license_revoked",
		fmt.Sprintf("License revoked from subscription %s", subscriptionID),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log license revocation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// ConfirmPurchase is called by the frontend immediately after a successful purchase
// to pre-populate the database with subscription and payment relationship
// This prevents race conditions where Razorpay webhooks arrive before the subscription
// is recorded in our database
func (s *SubscriptionService) ConfirmPurchase(req *PurchaseConfirmationRequest, mode string) error {
	// Fetch payment details from Razorpay to check if it's captured
	payment, err := GetPaymentByID(mode, req.RazorpayPaymentID)
	if err != nil {
		return fmt.Errorf("failed to fetch payment from Razorpay: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get the subscription's internal ID and owner info
	var dbSubscriptionID int64
	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT id, owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		req.RazorpaySubscriptionID,
	).Scan(&dbSubscriptionID, &ownerUserID, &orgID)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}

	// Update subscription with payment info
	// Set payment_verified=TRUE if payment is captured
	paymentVerified := bool(payment.Captured)
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET last_payment_id = $1,
		    last_payment_status = $2,
		    last_payment_received_at = NOW(),
		    payment_verified = $3,
		    updated_at = NOW()
		WHERE id = $4`,
		payment.ID, payment.Status, paymentVerified, dbSubscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription with payment info: %w", err)
	}

	// Record in subscription_payments table for audit trail
	paymentJSON, _ := json.Marshal(payment)
	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, amount, currency,
			status, captured, method, created_at, razorpay_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		dbSubscriptionID, payment.ID, payment.Amount, payment.Currency,
		payment.Status, payment.Captured, payment.Method, paymentJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert into subscription_payments: %w", err)
	}

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id": req.RazorpaySubscriptionID,
		"payment_id":      payment.ID,
		"amount":          payment.Amount,
		"status":          payment.Status,
		"captured":        payment.Captured,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "purchase_confirmed",
		fmt.Sprintf("Purchase confirmed: payment %s (captured: %t)", payment.ID, payment.Captured),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log purchase confirmation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SelfHostedPurchaseRequest represents a self-hosted purchase request
type SelfHostedPurchaseRequest struct {
	Email    string `json:"email"`
	Quantity int    `json:"quantity"` // Should be 1 for self-hosted
}

// SelfHostedPurchaseResponse represents the response for self-hosted purchase
type SelfHostedPurchaseResponse struct {
	SubscriptionID string `json:"subscription_id"`
	ShortURL       string `json:"short_url"`
	LicenseKey     string `json:"license_key,omitempty"` // Sent after payment confirmation
}

// getOrCreateShadowUser retrieves an existing user by email or creates a new shadow user
func (s *SubscriptionService) getOrCreateShadowUser(email string) (int64, error) {
	var userID int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err == nil {
		return userID, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query user: %w", err)
	}

	// User doesn't exist, create shadow user
	// Generate secure random password
	passwordBytes := make([]byte, 32)
	if _, err := rand.Read(passwordBytes); err != nil {
		return 0, fmt.Errorf("failed to generate random password: %w", err)
	}
	password := hex.EncodeToString(passwordBytes)

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("failed to hash password: %w", err)
	}

	// Insert shadow user
	err = s.db.QueryRow(`
		INSERT INTO users (email, password_hash, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id`,
		email, string(hashedPassword),
	).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("failed to create shadow user: %w", err)
	}

	return userID, nil
}

// CreateSelfHostedPurchase creates a self-hosted purchase without requiring full user/org setup
func (s *SubscriptionService) CreateSelfHostedPurchase(email string, quantity int, mode string) (*SelfHostedPurchaseResponse, error) {
	// Use the annual plan for self-hosted, get the correct one based on mode
	razorpayPlanID := GetPlanID(mode, "yearly")

	if quantity < 1 {
		quantity = 1
	}

	// Get or create shadow user for this email
	userID, err := s.getOrCreateShadowUser(email)
	if err != nil {
		return nil, fmt.Errorf("failed to get or create shadow user: %w", err)
	}

	// Create notes for the subscription
	notes := map[string]string{
		"email":     email,
		"plan_type": "selfhosted_annual",
		"purpose":   "self_hosted_license",
	}

	// Create subscription in Razorpay
	sub, err := CreateSubscription(mode, razorpayPlanID, quantity, notes)
	if err != nil {
		return nil, fmt.Errorf("failed to create razorpay subscription: %w", err)
	}

	// Calculate license expiration (365 days for annual)
	currentPeriodStart := time.Now()
	currentPeriodEnd := currentPeriodStart.AddDate(1, 0, 0) // 1 year
	licenseExpiresAt := currentPeriodEnd

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into subscriptions table with a special marker for self-hosted
	notesJSON, _ := json.Marshal(notes)
	var dbSubscriptionID int64
	err = tx.QueryRow(`
		INSERT INTO subscriptions (
			razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			current_period_start, current_period_end, license_expires_at,
			short_url, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		RETURNING id`,
		sub.ID, userID, nil, "selfhosted_annual",
		quantity, 0, sub.Status, razorpayPlanID,
		currentPeriodStart, currentPeriodEnd, licenseExpiresAt,
		sub.ShortURL, notesJSON,
	).Scan(&dbSubscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert subscription: %w", err)
	}

	// Log to license_log
	metadata := map[string]interface{}{
		"subscription_id": sub.ID,
		"plan_id":         razorpayPlanID,
		"email":           email,
		"quantity":        quantity,
		"status":          sub.Status,
		"purpose":         "self_hosted_purchase",
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		nil, nil, "selfhosted_subscription_created",
		fmt.Sprintf("Created self-hosted subscription for email: %s", email),
		metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to log subscription creation: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &SelfHostedPurchaseResponse{
		SubscriptionID: sub.ID,
		ShortURL:       sub.ShortURL,
	}, nil
}

// ConfirmSelfHostedPurchase confirms payment and generates license key
func (s *SubscriptionService) ConfirmSelfHostedPurchase(subscriptionID, paymentID, mode string) (string, error) {
	// Fetch payment details from Razorpay
	payment, err := GetPaymentByID(mode, paymentID)
	if err != nil {
		return "", fmt.Errorf("failed to fetch payment from Razorpay: %w", err)
	}

	if !payment.Captured {
		return "", fmt.Errorf("payment not captured yet")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get subscription details
	var dbSubscriptionID int64
	var email string
	var notesJSON []byte
	var quantity int
	err = tx.QueryRow(`
		SELECT id, notes, quantity
		FROM subscriptions
		WHERE razorpay_subscription_id = $1 AND plan_type = 'selfhosted_annual'`,
		subscriptionID,
	).Scan(&dbSubscriptionID, &notesJSON, &quantity)
	if err != nil {
		return "", fmt.Errorf("subscription not found: %w", err)
	}

	// Extract email from notes
	var notes map[string]string
	if err := json.Unmarshal(notesJSON, &notes); err == nil {
		email = notes["email"]
	}

	// Issue JWT license via fw-parse with the purchased quantity
	jwtToken, err := s.issueSelfHostedJWT(email, quantity)
	if err != nil {
		// Log error but don't fail the purchase
		fmt.Printf("Warning: Failed to issue JWT license: %v\n", err)
		// Generate fallback key for tracking
		licenseKey := fmt.Sprintf("LR-SELFHOSTED-%s-%d", subscriptionID[:8], time.Now().Unix())

		// Update subscription with payment info only
		_, err = tx.Exec(`
			UPDATE subscriptions
			SET last_payment_id = $1,
			    last_payment_status = $2,
			    last_payment_received_at = NOW(),
			    payment_verified = TRUE,
			    notes = jsonb_set(notes, '{license_key}', to_jsonb($3::text)),
			    updated_at = NOW()
			WHERE id = $4`,
			payment.ID, payment.Status, licenseKey, dbSubscriptionID,
		)
		if err != nil {
			return "", fmt.Errorf("failed to update subscription: %w", err)
		}

		// Record payment
		paymentJSON, _ := json.Marshal(payment)
		_, err = tx.Exec(`
			INSERT INTO subscription_payments (
				subscription_id, razorpay_payment_id, amount, currency,
				status, captured, method, created_at, razorpay_data
			) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
			ON CONFLICT (razorpay_payment_id) DO NOTHING`,
			dbSubscriptionID, payment.ID, payment.Amount, payment.Currency,
			payment.Status, payment.Captured, payment.Method, paymentJSON,
		)
		if err != nil {
			return "", fmt.Errorf("failed to insert payment record: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return "", fmt.Errorf("failed to commit transaction: %w", err)
		}

		return "Payment confirmed. License generation pending. Please contact support@hexmos.com", nil
	}

	// Update subscription with payment info and JWT token
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET last_payment_id = $1,
		    last_payment_status = $2,
		    last_payment_received_at = NOW(),
		    payment_verified = TRUE,
		    notes = jsonb_set(notes, '{jwt_token}', to_jsonb($3::text)),
		    updated_at = NOW()
		WHERE id = $4`,
		payment.ID, payment.Status, jwtToken, dbSubscriptionID,
	)
	if err != nil {
		return "", fmt.Errorf("failed to update subscription: %w", err)
	}

	// Record in subscription_payments table
	paymentJSON, _ := json.Marshal(payment)
	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, amount, currency,
			status, captured, method, created_at, razorpay_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		dbSubscriptionID, payment.ID, payment.Amount, payment.Currency,
		payment.Status, payment.Captured, payment.Method, paymentJSON,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert payment record: %w", err)
	}

	// Log license generation
	metadata := map[string]interface{}{
		"subscription_id": subscriptionID,
		"payment_id":      payment.ID,
		"email":           email,
		"jwt_issued":      true,
		"amount":          payment.Amount,
	}
	metadataJSON, _ := json.Marshal(metadata)
	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		nil, nil, "selfhosted_license_generated",
		fmt.Sprintf("Generated self-hosted JWT license for %s", email),
		metadataJSON,
	)
	if err != nil {
		return "", fmt.Errorf("failed to log license generation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return jwtToken, nil
}

// issueSelfHostedJWT calls fw-parse to issue a JWT license
func (s *SubscriptionService) issueSelfHostedJWT(email string, seatCount int) (string, error) {
	secret := os.Getenv("FW_PARSE_ADMIN_SECRET")
	if secret == "" {
		fmt.Printf("[issueSelfHostedJWT] ERROR: FW_PARSE_ADMIN_SECRET not configured\n")
		return "", fmt.Errorf("FW_PARSE_ADMIN_SECRET not configured")
	}

	fmt.Printf("[issueSelfHostedJWT] Issuing JWT for email: %s, seatCount: %d\n", email, seatCount)

	// Build request payload
	// Note: durationDays is the parameter fw-parse expects for license duration
	payload := map[string]interface{}{
		"email":        email,
		"appName":      "LiveReview",
		"seatCount":    seatCount,
		"unlimited":    seatCount == 0, // unlimited if 0 seats specified
		"durationDays": 365,            // Annual license
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://parse.apps.hexmos.com/jwtLicence/issue", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Admin-Secret", secret)

	fmt.Printf("[issueSelfHostedJWT] Calling fw-parse at https://parse.apps.hexmos.com/jwtLicence/issue\n")

	// Send request with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[issueSelfHostedJWT] ERROR: Failed to call fw-parse: %v\n", err)
		return "", fmt.Errorf("failed to call fw-parse: %w", err)
	}
	defer resp.Body.Close()

	fmt.Printf("[issueSelfHostedJWT] fw-parse response status: %d\n", resp.StatusCode)

	// Read the full response body for logging
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	fmt.Printf("[issueSelfHostedJWT] fw-parse response body: %s\n", string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fw-parse returned status %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response - fw-parse returns {"data":{"token":"...","expiresAt":"...",...}}
	var result struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Printf("[issueSelfHostedJWT] ERROR: Failed to parse response: %v\n", err)
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Data.Token == "" {
		fmt.Printf("[issueSelfHostedJWT] ERROR: fw-parse error: %s\n", result.Error)
		return "", fmt.Errorf("fw-parse error: %s", result.Error)
	}

	fmt.Printf("[issueSelfHostedJWT] SUCCESS: JWT issued for %s\n", email)
	return result.Data.Token, nil
}
