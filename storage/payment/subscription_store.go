package payment

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var ErrSubscriptionNotFound = errors.New("subscription not found")
var ErrUserNotFound = errors.New("user not found")
var ErrUserRoleNotFound = errors.New("user role not found")

const teamRoleID = 3

// SubscriptionStore handles persistence for subscription lifecycle operations.
type SubscriptionStore struct {
	db *sql.DB
}

func NewSubscriptionStore(db *sql.DB) *SubscriptionStore {
	return &SubscriptionStore{db: db}
}

type CreateTeamSubscriptionRecordInput struct {
	SubscriptionID     string
	OwnerUserID        int
	OrgID              int
	DBPlanType         string
	Quantity           int
	Status             string
	RazorpayPlanID     string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	LicenseExpiresAt   time.Time
	ShortURL           string
	Notes              map[string]string
}

func (s *SubscriptionStore) CreateTeamSubscriptionRecord(input CreateTeamSubscriptionRecordInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	notesJSON, err := json.Marshal(input.Notes)
	if err != nil {
		return fmt.Errorf("failed to marshal notes: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO subscriptions (
			razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			current_period_start, current_period_end, license_expires_at,
			short_url, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())`,
		input.SubscriptionID, input.OwnerUserID, input.OrgID, input.DBPlanType,
		input.Quantity, 0, input.Status, input.RazorpayPlanID,
		input.CurrentPeriodStart, input.CurrentPeriodEnd, input.LicenseExpiresAt,
		input.ShortURL, notesJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert subscription: %w", err)
	}

	metadata := map[string]interface{}{
		"subscription_id": input.SubscriptionID,
		"plan_id":         input.RazorpayPlanID,
		"quantity":        input.Quantity,
		"status":          input.Status,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		input.OwnerUserID, input.OrgID, "subscription_created",
		fmt.Sprintf("Created %s subscription with %d seats", input.DBPlanType, input.Quantity),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log subscription creation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

type UpdateSubscriptionQuantityRecordInput struct {
	SubscriptionID      string
	Quantity            int
	ScheduleChangeAt    int64
	Status              string
	HasScheduledChanges bool
}

func (s *SubscriptionStore) UpdateSubscriptionQuantityRecord(input UpdateSubscriptionQuantityRecordInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		input.SubscriptionID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: %s", ErrSubscriptionNotFound, input.SubscriptionID)
		}
		return fmt.Errorf("failed to get subscription details: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE subscriptions
		SET quantity = $1,
		    status = $2,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $3`,
		input.Quantity, input.Status, input.SubscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	metadata := map[string]interface{}{
		"subscription_id":      input.SubscriptionID,
		"new_quantity":         input.Quantity,
		"schedule_change_at":   input.ScheduleChangeAt,
		"has_scheduled_change": input.HasScheduledChanges,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_quantity_updated",
		fmt.Sprintf("Updated subscription quantity to %d", input.Quantity),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log quantity update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

type CancelSubscriptionRecordInput struct {
	SubscriptionID string
	Immediate      bool
	Status         string
}

func (s *SubscriptionStore) CancelSubscriptionRecord(input CancelSubscriptionRecordInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var ownerUserID, orgID int
	err = tx.QueryRow(`
		SELECT owner_user_id, org_id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		input.SubscriptionID,
	).Scan(&ownerUserID, &orgID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: %s", ErrSubscriptionNotFound, input.SubscriptionID)
		}
		return fmt.Errorf("failed to get subscription details: %w", err)
	}

	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = $1,
		    cancel_at_period_end = $2,
		    updated_at = NOW()
		WHERE razorpay_subscription_id = $3`,
		input.Status, !input.Immediate, input.SubscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	if input.Immediate {
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
			return fmt.Errorf("failed to update user_roles: %w", err)
		}
	}

	metadata := map[string]interface{}{
		"subscription_id": input.SubscriptionID,
		"immediate":       input.Immediate,
		"status":          input.Status,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		ownerUserID, orgID, "subscription_cancelled",
		fmt.Sprintf("Cancelled subscription (immediate: %t)", input.Immediate),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log cancellation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

type SubscriptionDetailsRow struct {
	ID                     int64
	RazorpaySubscriptionID string
	OwnerUserID            int
	OrgID                  int
	PlanType               string
	Quantity               int
	AssignedSeats          int
	Status                 string
	LicenseExpiresAt       time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	PaymentVerified        bool
	LastPaymentID          sql.NullString
	LastPaymentStatus      sql.NullString
	LastPaymentReceivedAt  sql.NullTime
}

type OrgSubscriptionRow struct {
	RazorpaySubscriptionID string
	Status                 string
	PlanType               string
	Quantity               int
	CurrentPeriodEnd       time.Time
}

func (s *SubscriptionStore) ListSubscriptionsByOrgID(orgID int) ([]OrgSubscriptionRow, error) {
	rows, err := s.db.Query(`
		SELECT razorpay_subscription_id, status, plan_type, quantity, current_period_end
		FROM subscriptions
		WHERE org_id = $1
		ORDER BY updated_at DESC, created_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions by org: %w", err)
	}
	defer rows.Close()

	out := make([]OrgSubscriptionRow, 0)
	for rows.Next() {
		var row OrgSubscriptionRow
		if err := rows.Scan(&row.RazorpaySubscriptionID, &row.Status, &row.PlanType, &row.Quantity, &row.CurrentPeriodEnd); err != nil {
			return nil, fmt.Errorf("failed to scan org subscription row: %w", err)
		}
		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate org subscriptions: %w", err)
	}

	return out, nil
}

func (s *SubscriptionStore) GetSubscriptionDetailsRow(subscriptionID string) (*SubscriptionDetailsRow, error) {
	var row SubscriptionDetailsRow
	err := s.db.QueryRow(`
		SELECT s.id, s.razorpay_subscription_id, s.owner_user_id, s.org_id, s.plan_type, s.quantity,
		       COALESCE((SELECT COUNT(*) FROM user_roles ur WHERE ur.active_subscription_id = s.id AND ur.plan_type = 'team'), 0) as assigned_seats,
		       s.status, s.license_expires_at, s.created_at, s.updated_at,
		       s.payment_verified, s.last_payment_id, s.last_payment_status, s.last_payment_received_at
		FROM subscriptions s
		WHERE s.razorpay_subscription_id = $1`,
		subscriptionID,
	).Scan(
		&row.ID, &row.RazorpaySubscriptionID, &row.OwnerUserID, &row.OrgID, &row.PlanType,
		&row.Quantity, &row.AssignedSeats, &row.Status,
		&row.LicenseExpiresAt, &row.CreatedAt, &row.UpdatedAt,
		&row.PaymentVerified, &row.LastPaymentID, &row.LastPaymentStatus, &row.LastPaymentReceivedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%w: %s", ErrSubscriptionNotFound, subscriptionID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get subscription from DB: %w", err)
	}
	return &row, nil
}

type AssignLicenseInput struct {
	SubscriptionID string
	UserID         int
	OrgID          int
}

func (s *SubscriptionStore) AssignLicense(input AssignLicenseInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var quantity int
	var dbSubscriptionID int64
	var licenseExpiresAt time.Time
	var assignedSeats int
	var paymentVerified bool
	var lastPaymentStatus sql.NullString
	err = tx.QueryRow(`
		SELECT s.id, s.quantity, s.license_expires_at, s.payment_verified, s.last_payment_status,
		       COALESCE((SELECT COUNT(*) FROM user_roles ur WHERE ur.active_subscription_id = s.id AND ur.plan_type = 'team'), 0) as assigned_seats
		FROM subscriptions s
		WHERE s.razorpay_subscription_id = $1
		FOR UPDATE`,
		input.SubscriptionID,
	).Scan(&dbSubscriptionID, &quantity, &licenseExpiresAt, &paymentVerified, &lastPaymentStatus, &assignedSeats)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: %s", ErrSubscriptionNotFound, input.SubscriptionID)
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	if !paymentVerified {
		return fmt.Errorf("payment pending - licenses cannot be assigned until payment is received. Check back in 5-10 minutes")
	}

	if assignedSeats >= quantity {
		return fmt.Errorf("subscription at capacity: %d/%d seats used", assignedSeats, quantity)
	}

	var existingSubID sql.NullInt64
	var existingRazorpaySubID sql.NullString
	err = tx.QueryRow(`
		SELECT ur.active_subscription_id, s.razorpay_subscription_id
		FROM user_roles ur
		LEFT JOIN subscriptions s ON ur.active_subscription_id = s.id
		WHERE ur.user_id = $1 AND ur.org_id = $2 AND ur.plan_type = 'team'`,
		input.UserID, input.OrgID,
	).Scan(&existingSubID, &existingRazorpaySubID)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check existing subscription: %w", err)
	}

	if existingSubID.Valid && existingSubID.Int64 != dbSubscriptionID {
		return fmt.Errorf("user already has an active license from subscription %s - please revoke that first", existingRazorpaySubID.String)
	}

	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'team',
		    license_expires_at = $1,
		    active_subscription_id = $2,
		    updated_at = NOW()
		WHERE user_id = $3 AND org_id = $4`,
		licenseExpiresAt, dbSubscriptionID, input.UserID, input.OrgID,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23503" {
			_, err = tx.Exec(`
				INSERT INTO user_roles (
					user_id, org_id, role_id, plan_type, license_expires_at, active_subscription_id, created_at, updated_at
				) VALUES ($1, $2, $3, 'team', $4, $5, NOW(), NOW())`,
				input.UserID, input.OrgID, teamRoleID, licenseExpiresAt, dbSubscriptionID,
			)
			if err != nil {
				return fmt.Errorf("failed to create user_roles: %w", err)
			}
		} else {
			return fmt.Errorf("failed to update user_roles: %w", err)
		}
	}

	metadata := map[string]interface{}{
		"subscription_id": input.SubscriptionID,
		"assigned_to":     input.UserID,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		input.UserID, input.OrgID, "license_assigned",
		fmt.Sprintf("License assigned from subscription %s", input.SubscriptionID),
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

type RevokeLicenseInput struct {
	SubscriptionID string
	UserID         int
	OrgID          int
}

func (s *SubscriptionStore) RevokeLicense(input RevokeLicenseInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var dbSubscriptionID int64
	err = tx.QueryRow(`
		SELECT id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		input.SubscriptionID,
	).Scan(&dbSubscriptionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: %s", ErrSubscriptionNotFound, input.SubscriptionID)
		}
		return fmt.Errorf("failed to get subscription: %w", err)
	}

	var currentSubID sql.NullInt64
	err = tx.QueryRow(`
		SELECT active_subscription_id
		FROM user_roles
		WHERE user_id = $1 AND org_id = $2`,
		input.UserID, input.OrgID,
	).Scan(&currentSubID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("%w: user_id=%d org_id=%d", ErrUserRoleNotFound, input.UserID, input.OrgID)
		}
		return fmt.Errorf("failed to get user_roles: %w", err)
	}

	if !currentSubID.Valid || currentSubID.Int64 != dbSubscriptionID {
		return fmt.Errorf("user %d does not have subscription %s", input.UserID, input.SubscriptionID)
	}

	_, err = tx.Exec(`
		UPDATE user_roles
		SET plan_type = 'free',
		    license_expires_at = NULL,
		    active_subscription_id = NULL,
		    updated_at = NOW()
		WHERE user_id = $1 AND org_id = $2`,
		input.UserID, input.OrgID,
	)
	if err != nil {
		return fmt.Errorf("failed to update user_roles: %w", err)
	}

	metadata := map[string]interface{}{
		"subscription_id": input.SubscriptionID,
		"revoked_from":    input.UserID,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		input.UserID, input.OrgID, "license_revoked",
		fmt.Sprintf("License revoked from subscription %s", input.SubscriptionID),
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

func (s *SubscriptionStore) GetUserIDByEmail(email string) (int64, error) {
	var userID int64
	err := s.db.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrUserNotFound
		}
		return 0, err
	}
	return userID, nil
}

func (s *SubscriptionStore) CreateShadowUser(email, passwordHash string) (int64, error) {
	var userID int64
	err := s.db.QueryRow(`
		INSERT INTO users (email, password_hash, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id`,
		email, passwordHash,
	).Scan(&userID)
	if err != nil {
		return 0, fmt.Errorf("failed to create shadow user: %w", err)
	}
	return userID, nil
}

type CreateSelfHostedSubscriptionRecordInput struct {
	SubscriptionID     string
	UserID             int64
	Quantity           int
	Status             string
	RazorpayPlanID     string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	LicenseExpiresAt   time.Time
	ShortURL           string
	Notes              map[string]string
	Email              string
}

func (s *SubscriptionStore) CreateSelfHostedSubscriptionRecord(input CreateSelfHostedSubscriptionRecordInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	notesJSON, err := json.Marshal(input.Notes)
	if err != nil {
		return fmt.Errorf("failed to marshal notes: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO subscriptions (
			razorpay_subscription_id, owner_user_id, org_id, plan_type,
			quantity, assigned_seats, status, razorpay_plan_id,
			current_period_start, current_period_end, license_expires_at,
			short_url, notes, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())`,
		input.SubscriptionID, input.UserID, nil, "selfhosted_annual",
		input.Quantity, 0, input.Status, input.RazorpayPlanID,
		input.CurrentPeriodStart, input.CurrentPeriodEnd, input.LicenseExpiresAt,
		input.ShortURL, notesJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert subscription: %w", err)
	}

	metadata := map[string]interface{}{
		"subscription_id": input.SubscriptionID,
		"plan_id":         input.RazorpayPlanID,
		"email":           input.Email,
		"quantity":        input.Quantity,
		"status":          input.Status,
		"purpose":         "self_hosted_purchase",
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		nil, nil, "selfhosted_subscription_created",
		fmt.Sprintf("Created self-hosted subscription for email: %s", input.Email),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log subscription creation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

type SelfHostedConfirmationSeed struct {
	SubscriptionDBID int64
	Email            string
	Quantity         int
}

func (s *SubscriptionStore) GetSelfHostedConfirmationSeed(subscriptionID string) (SelfHostedConfirmationSeed, error) {
	var seed SelfHostedConfirmationSeed
	var notesJSON []byte
	err := s.db.QueryRow(`
		SELECT id, notes, quantity
		FROM subscriptions
		WHERE razorpay_subscription_id = $1 AND plan_type = 'selfhosted_annual'`,
		subscriptionID,
	).Scan(&seed.SubscriptionDBID, &notesJSON, &seed.Quantity)
	if err != nil {
		if err == sql.ErrNoRows {
			return SelfHostedConfirmationSeed{}, fmt.Errorf("%w: %s", ErrSubscriptionNotFound, subscriptionID)
		}
		return SelfHostedConfirmationSeed{}, fmt.Errorf("failed to fetch self-hosted confirmation seed: %w", err)
	}

	var notes map[string]string
	if err := json.Unmarshal(notesJSON, &notes); err != nil {
		return SelfHostedConfirmationSeed{}, fmt.Errorf("failed to decode subscription notes: %w", err)
	}
	seed.Email = notes["email"]

	return seed, nil
}

type PersistSelfHostedFallbackInput struct {
	SubscriptionDBID int64
	PaymentID        string
	PaymentStatus    string
	PaymentAmount    int64
	PaymentCurrency  string
	PaymentCaptured  bool
	PaymentMethod    string
	PaymentJSON      []byte
	LicenseKey       string
}

func (s *SubscriptionStore) PersistSelfHostedFallback(input PersistSelfHostedFallbackInput) error {
	if input.LicenseKey == "" {
		return fmt.Errorf("license key cannot be empty")
	}
	if len(input.LicenseKey) > 8192 {
		return fmt.Errorf("license key too large")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE subscriptions
		SET last_payment_id = $1,
		    last_payment_status = $2,
		    last_payment_received_at = NOW(),
		    payment_verified = TRUE,
		    notes = jsonb_set(COALESCE(notes, '{}'::jsonb), '{license_key}', to_jsonb($3::text)),
		    updated_at = NOW()
		WHERE id = $4`,
		input.PaymentID, input.PaymentStatus, input.LicenseKey, input.SubscriptionDBID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, amount, currency,
			status, captured, method, created_at, razorpay_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		input.SubscriptionDBID, input.PaymentID, input.PaymentAmount, input.PaymentCurrency,
		input.PaymentStatus, input.PaymentCaptured, input.PaymentMethod, input.PaymentJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert payment record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

type PersistSelfHostedJWTInput struct {
	SubscriptionID   string
	SubscriptionDBID int64
	PaymentID        string
	PaymentStatus    string
	PaymentAmount    int64
	PaymentCurrency  string
	PaymentCaptured  bool
	PaymentMethod    string
	PaymentJSON      []byte
	JWTToken         string
	Email            string
}

func (s *SubscriptionStore) PersistSelfHostedJWT(input PersistSelfHostedJWTInput) error {
	if input.JWTToken == "" {
		return fmt.Errorf("jwt token cannot be empty")
	}
	if len(input.JWTToken) > 8192 {
		return fmt.Errorf("jwt token too large")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE subscriptions
		SET last_payment_id = $1,
		    last_payment_status = $2,
		    last_payment_received_at = NOW(),
		    payment_verified = TRUE,
		    notes = jsonb_set(COALESCE(notes, '{}'::jsonb), '{jwt_token}', to_jsonb($3::text)),
		    updated_at = NOW()
		WHERE id = $4`,
		input.PaymentID, input.PaymentStatus, input.JWTToken, input.SubscriptionDBID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO subscription_payments (
			subscription_id, razorpay_payment_id, amount, currency,
			status, captured, method, created_at, razorpay_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), $8)
		ON CONFLICT (razorpay_payment_id) DO NOTHING`,
		input.SubscriptionDBID, input.PaymentID, input.PaymentAmount, input.PaymentCurrency,
		input.PaymentStatus, input.PaymentCaptured, input.PaymentMethod, input.PaymentJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to insert payment record: %w", err)
	}

	metadata := map[string]interface{}{
		"subscription_id": input.SubscriptionID,
		"payment_id":      input.PaymentID,
		"email":           input.Email,
		"jwt_issued":      true,
		"amount":          input.PaymentAmount,
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal log metadata: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO license_log (
			user_id, org_id, event_type, description, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, NOW())`,
		nil, nil, "selfhosted_license_generated",
		fmt.Sprintf("Generated self-hosted JWT license for %s", input.Email),
		metadataJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to log license generation: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
