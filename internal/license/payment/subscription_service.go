package license

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
	if planType == "monthly" {
		licenseExpiresAt = time.Now().AddDate(0, 1, 0) // 1 month
	} else {
		licenseExpiresAt = time.Now().AddDate(1, 0, 0) // 1 year
	}

	// Start transaction
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into subscriptions table
	notesJSON, _ := json.Marshal(notes)
	_, err = tx.Exec(`
		INSERT INTO subscriptions (
			razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			current_period_start, current_period_end, license_expires_at,
			notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW())`,
		sub.ID, ownerUserID, orgID, dbPlanType,
		quantity, 0, sub.Status, razorpayPlanID,
		time.Unix(sub.CurrentStart, 0), time.Unix(sub.CurrentEnd, 0), licenseExpiresAt,
		notesJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert subscription: %w", err)
	}

	// Update user_roles to set this as active subscription
	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'team',
		    license_expires_at = $1,
		    active_subscription_id = $2,
		    updated_at = NOW()
		WHERE user_id = $3 AND org_id = $4`,
		licenseExpiresAt, sub.ID, ownerUserID, orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update user_roles: %w", err)
	}

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
	ID               int       `json:"id"`
	OwnerUserID      int       `json:"owner_user_id"`
	OrgID            int       `json:"org_id"`
	PlanType         string    `json:"plan_type"`
	Quantity         int       `json:"quantity"`
	AssignedSeats    int       `json:"assigned_seats"`
	Status           string    `json:"status"`
	LicenseExpiresAt time.Time `json:"license_expires_at"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// From Razorpay
	RazorpaySubscription *RazorpaySubscription `json:"razorpay_subscription,omitempty"`
}

// GetSubscriptionDetails retrieves subscription details from both DB and Razorpay
func (s *SubscriptionService) GetSubscriptionDetails(subscriptionID string, mode string) (*SubscriptionDetails, error) {
	// Get from DB
	var details SubscriptionDetails
	err := s.db.QueryRow(`
		SELECT id, owner_user_id, org_id, plan_type, quantity, assigned_seats,
		       status, license_expires_at, created_at, updated_at
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(
		&details.ID, &details.OwnerUserID, &details.OrgID, &details.PlanType,
		&details.Quantity, &details.AssignedSeats, &details.Status,
		&details.LicenseExpiresAt, &details.CreatedAt, &details.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("subscription not found: %s", subscriptionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription from DB: %w", err)
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

	// Check subscription capacity
	var quantity, assignedSeats int
	var ownerUserID int
	var licenseExpiresAt time.Time
	err = tx.QueryRow(`
		SELECT quantity, assigned_seats, owner_user_id, license_expires_at
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(&quantity, &assignedSeats, &ownerUserID, &licenseExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if assignedSeats >= quantity {
		return fmt.Errorf("subscription at capacity: %d/%d seats used", assignedSeats, quantity)
	}

	// Increment assigned_seats
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET assigned_seats = assigned_seats + 1,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to increment assigned_seats: %w", err)
	}

	// Update user_roles
	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'team',
		    license_expires_at = $1,
		    active_subscription_id = $2,
		    updated_at = NOW()
		WHERE user_id = $3 AND org_id = $4`,
		licenseExpiresAt, subscriptionID, userID, orgID,
	)
	if err != nil {
		// Handle case where user_roles entry doesn't exist yet
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23503" {
			// Insert new user_roles entry
			_, err = tx.Exec(`
				INSERT INTO user_roles (
					user_id, org_id, role, plan_type, license_expires_at, active_subscription_id, created_at, updated_at
				) VALUES ($1, $2, 'member', 'team', $3, $4, NOW(), NOW())`,
				userID, orgID, licenseExpiresAt, subscriptionID,
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

	// Verify user has this subscription
	var currentSubID sql.NullString
	err = tx.QueryRow(`
		SELECT active_subscription_id
		FROM user_roles
		WHERE user_id = $1 AND org_id = $2`,
		userID, orgID,
	).Scan(&currentSubID)
	if err != nil {
		return fmt.Errorf("failed to get user_roles: %w", err)
	}

	if !currentSubID.Valid || currentSubID.String != subscriptionID {
		return fmt.Errorf("user %d does not have subscription %s", userID, subscriptionID)
	}

	// Decrement assigned_seats
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET assigned_seats = assigned_seats - 1,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $1`,
		subscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to decrement assigned_seats: %w", err)
	}

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
