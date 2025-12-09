package payment

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// Razorpay Plan IDs - these should be set after creating plans in Razorpay
// Run: go test -v ./internal/license/payment -run TestSetupPlans
var (
	TeamMonthlyPlanID = "plan_RnY4EmRoj5bsRl"
	TeamYearlyPlanID  = "plan_RnY4FSDAPXQhRa"
)

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

	// Get the corresponding Razorpay plan ID
	var razorpayPlanID string
	if planType == "monthly" {
		razorpayPlanID = TeamMonthlyPlanID
	} else {
		razorpayPlanID = TeamYearlyPlanID
	}

	if razorpayPlanID == "" {
		return nil, fmt.Errorf("razorpay plan ID not configured for %s", planType)
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
			notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())
		RETURNING id`,
		sub.ID, ownerUserID, orgID, dbPlanType,
		quantity, 0, sub.Status, razorpayPlanID,
		currentPeriodStart, currentPeriodEnd, licenseExpiresAt,
		notesJSON,
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
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $2`,
		sub.Status, subscriptionID,
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
