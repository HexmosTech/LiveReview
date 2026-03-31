package payment

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/livereview/internal/license"
	networkpayment "github.com/livereview/network/payment"
	storagepayment "github.com/livereview/storage/payment"
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

func planCodeToMonthlyQuantity(planCode license.PlanType) (int, error) {
	switch planCode {
	case license.PlanTeam32USD:
		return 1, nil
	case license.PlanLOC200K:
		return 2, nil
	case license.PlanLOC400K:
		return 4, nil
	case license.PlanLOC800K:
		return 8, nil
	case license.PlanLOC1600K:
		return 16, nil
	case license.PlanLOC3200K:
		return 32, nil
	default:
		return 0, fmt.Errorf("unsupported paid plan code: %s", planCode)
	}
}

func normalizePersistedPlanCode(raw string) license.PlanType {
	normalized := license.PlanType(strings.TrimSpace(raw))
	if normalized.IsValid() {
		return normalized
	}

	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "team", "team_monthly", "team_annual", "team_yearly", "monthly", "yearly":
		return license.PlanTeam32USD
	case "free":
		return license.PlanFree30K
	default:
		return license.PlanTeam32USD
	}
}

// CreateTeamSubscription creates a new monthly LOC slab subscription via Razorpay and persists to DB.
func (s *SubscriptionService) CreateTeamSubscription(ownerUserID, orgID int, planCode string, mode string) (*RazorpaySubscription, error) {
	persistedPlanCode := license.PlanType(strings.TrimSpace(planCode))
	if !persistedPlanCode.IsValid() {
		return nil, fmt.Errorf("invalid plan_code: %s", planCode)
	}
	if persistedPlanCode.GetLimits().MonthlyPriceUSD <= 0 {
		return nil, fmt.Errorf("plan_code must be a paid LOC slab: %s", planCode)
	}

	quantity, err := planCodeToMonthlyQuantity(persistedPlanCode)
	if err != nil {
		return nil, err
	}

	// All LOC slab checkout in this migration is monthly-only.
	razorpayPlanID := GetPlanID(mode, "monthly")

	if razorpayPlanID == "" {
		return nil, fmt.Errorf("razorpay monthly plan ID not configured in %s mode", mode)
	}

	// Create notes for the subscription
	notes := map[string]string{
		"owner_user_id": fmt.Sprintf("%d", ownerUserID),
		"org_id":        fmt.Sprintf("%d", orgID),
		"plan_type":     persistedPlanCode.String(),
	}

	// Create subscription in Razorpay
	sub, err := CreateSubscription(mode, razorpayPlanID, quantity, notes)
	if err != nil {
		return nil, fmt.Errorf("failed to create razorpay subscription: %w", err)
	}

	// Calculate license expiration for monthly cycle.
	var licenseExpiresAt time.Time
	dbPlanType := persistedPlanCode.String()

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
		currentPeriodEnd = currentPeriodStart.AddDate(0, 1, 0) // 1 month
	}
	licenseExpiresAt = currentPeriodEnd

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.CreateTeamSubscriptionRecord(storagepayment.CreateTeamSubscriptionRecordInput{
		SubscriptionID:     sub.ID,
		OwnerUserID:        ownerUserID,
		OrgID:              orgID,
		DBPlanType:         dbPlanType,
		Quantity:           quantity,
		Status:             sub.Status,
		RazorpayPlanID:     razorpayPlanID,
		CurrentPeriodStart: currentPeriodStart,
		CurrentPeriodEnd:   currentPeriodEnd,
		LicenseExpiresAt:   licenseExpiresAt,
		ShortURL:           sub.ShortURL,
		Notes:              notes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist subscription: %w", err)
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

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.UpdateSubscriptionQuantityRecord(storagepayment.UpdateSubscriptionQuantityRecordInput{
		SubscriptionID:      subscriptionID,
		Quantity:            sub.Quantity,
		ScheduleChangeAt:    scheduleChangeAt,
		Status:              sub.Status,
		HasScheduledChanges: sub.HasScheduledChanges,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist quantity update: %w", err)
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

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.CancelSubscriptionRecord(storagepayment.CancelSubscriptionRecordInput{
		SubscriptionID: subscriptionID,
		Immediate:      immediate,
		Status:         sub.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist cancellation: %w", err)
	}

	return sub, nil
}

// SubscriptionDetails holds subscription info from both DB and Razorpay
type SubscriptionDetails struct {
	// From DB
	ID                     int64     `json:"id"`
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
	store := storagepayment.NewSubscriptionStore(s.db)
	row, err := store.GetSubscriptionDetailsRow(subscriptionID)
	if err != nil {
		if errors.Is(err, storagepayment.ErrSubscriptionNotFound) {
			return nil, fmt.Errorf("subscription not found: %s", subscriptionID)
		}
		return nil, err
	}

	var details SubscriptionDetails
	details.ID = row.ID
	details.RazorpaySubscriptionID = row.RazorpaySubscriptionID
	details.OwnerUserID = row.OwnerUserID
	details.OrgID = row.OrgID
	details.PlanType = row.PlanType
	details.Quantity = row.Quantity
	details.AssignedSeats = row.AssignedSeats
	details.Status = row.Status
	details.LicenseExpiresAt = row.LicenseExpiresAt
	details.CreatedAt = row.CreatedAt
	details.UpdatedAt = row.UpdatedAt
	details.PaymentVerified = row.PaymentVerified

	// Set nullable payment fields
	if row.LastPaymentID.Valid {
		details.LastPaymentID = row.LastPaymentID.String
	}
	if row.LastPaymentStatus.Valid {
		details.LastPaymentStatus = row.LastPaymentStatus.String
	}
	if row.LastPaymentReceivedAt.Valid {
		details.LastPaymentReceivedAt = &row.LastPaymentReceivedAt.Time
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
	store := storagepayment.NewSubscriptionStore(s.db)
	return store.AssignLicense(storagepayment.AssignLicenseInput{
		SubscriptionID: subscriptionID,
		UserID:         userID,
		OrgID:          orgID,
	})
}

func verifyCheckoutSignature(req *PurchaseConfirmationRequest, mode string) error {
	_, secretKey, err := GetRazorpayKeys(mode)
	if err != nil {
		return fmt.Errorf("failed to load Razorpay keys for signature verification: %w", err)
	}

	payload := req.RazorpayPaymentID + "|" + req.RazorpaySubscriptionID
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	provided := strings.ToLower(strings.TrimSpace(req.RazorpaySignature))
	if provided == "" {
		return fmt.Errorf("invalid razorpay signature")
	}

	if !hmac.Equal([]byte(expectedSignature), []byte(provided)) {
		return fmt.Errorf("invalid razorpay signature")
	}

	return nil
}

// RevokeLicense removes a license from a user

func (s *SubscriptionService) RevokeLicense(subscriptionID string, userID, orgID int) error {
	store := storagepayment.NewSubscriptionStore(s.db)
	return store.RevokeLicense(storagepayment.RevokeLicenseInput{
		SubscriptionID: subscriptionID,
		UserID:         userID,
		OrgID:          orgID,
	})
}

// ConfirmPurchase is called by the frontend immediately after a successful purchase
// to pre-populate the database with subscription and payment relationship
// This prevents race conditions where Razorpay webhooks arrive before the subscription
// is recorded in our database
func (s *SubscriptionService) ConfirmPurchase(req *PurchaseConfirmationRequest, mode string) error {
	if err := verifyCheckoutSignature(req, mode); err != nil {
		return err
	}

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
	var persistedPlanType string
	err = tx.QueryRow(`
		SELECT id, owner_user_id, org_id, plan_type
		FROM subscriptions
		WHERE razorpay_subscription_id = $1`,
		req.RazorpaySubscriptionID,
	).Scan(&dbSubscriptionID, &ownerUserID, &orgID, &persistedPlanType)
	if err != nil {
		return fmt.Errorf("subscription not found: %w", err)
	}
	resolvedPlanCode := normalizePersistedPlanCode(persistedPlanType)

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

	if paymentVerified {
		now := time.Now().UTC()
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 1, 0)
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
				scheduled_plan_code = NULL,
				scheduled_plan_effective_at = NULL,
				trial_readonly = FALSE,
				loc_blocked = FALSE,
				updated_at = NOW()
		`, orgID, resolvedPlanCode.String(), periodStart, periodEnd)
		if err != nil {
			return fmt.Errorf("failed to update org billing state for confirmed purchase: %w", err)
		}
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
	store := storagepayment.NewSubscriptionStore(s.db)

	userID, err := store.GetUserIDByEmail(email)
	if err == nil {
		return userID, nil
	}
	if !errors.Is(err, storagepayment.ErrUserNotFound) {
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

	userID, err = store.CreateShadowUser(email, string(hashedPassword))
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

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.CreateSelfHostedSubscriptionRecord(storagepayment.CreateSelfHostedSubscriptionRecordInput{
		SubscriptionID:     sub.ID,
		UserID:             userID,
		Quantity:           quantity,
		Status:             sub.Status,
		RazorpayPlanID:     razorpayPlanID,
		CurrentPeriodStart: currentPeriodStart,
		CurrentPeriodEnd:   currentPeriodEnd,
		LicenseExpiresAt:   licenseExpiresAt,
		ShortURL:           sub.ShortURL,
		Notes:              notes,
		Email:              email,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist self-hosted subscription record: %w", err)
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

	store := storagepayment.NewSubscriptionStore(s.db)
	seed, err := store.GetSelfHostedConfirmationSeed(subscriptionID)
	if err != nil {
		return "", fmt.Errorf("subscription not found: %w", err)
	}

	email := seed.Email
	quantity := seed.Quantity

	// Issue JWT license via fw-parse with the purchased quantity
	jwtToken, err := s.issueSelfHostedJWT(email, quantity)
	if err != nil {
		// Log error but don't fail the purchase
		fmt.Printf("Warning: Failed to issue JWT license: %v\n", err)
		// Generate fallback key for tracking
		licenseKey := fmt.Sprintf("LR-SELFHOSTED-%s-%d", subscriptionID[:8], time.Now().Unix())

		paymentJSON, _ := json.Marshal(payment)
		err = store.PersistSelfHostedFallback(storagepayment.PersistSelfHostedFallbackInput{
			SubscriptionDBID: seed.SubscriptionDBID,
			PaymentID:        payment.ID,
			PaymentStatus:    payment.Status,
			PaymentAmount:    payment.Amount,
			PaymentCurrency:  payment.Currency,
			PaymentCaptured:  bool(payment.Captured),
			PaymentMethod:    payment.Method,
			PaymentJSON:      paymentJSON,
			LicenseKey:       licenseKey,
		})
		if err != nil {
			return "", fmt.Errorf("failed to insert payment record: %w", err)
		}

		return "Payment confirmed. License generation pending. Please contact support@hexmos.com", nil
	}

	paymentJSON, _ := json.Marshal(payment)
	err = store.PersistSelfHostedJWT(storagepayment.PersistSelfHostedJWTInput{
		SubscriptionID:   subscriptionID,
		SubscriptionDBID: seed.SubscriptionDBID,
		PaymentID:        payment.ID,
		PaymentStatus:    payment.Status,
		PaymentAmount:    payment.Amount,
		PaymentCurrency:  payment.Currency,
		PaymentCaptured:  bool(payment.Captured),
		PaymentMethod:    payment.Method,
		PaymentJSON:      paymentJSON,
		JWTToken:         jwtToken,
		Email:            email,
	})
	if err != nil {
		return "", fmt.Errorf("failed to persist self-hosted JWT, payment record, and log entry: %w", err)
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

	fmt.Printf("[issueSelfHostedJWT] Calling fw-parse at https://parse.apps.hexmos.com/jwtLicence/issue\n")

	statusCode, respBody, err := networkpayment.IssueSelfHostedJWTRequest(secret, payloadBytes)
	if err != nil {
		fmt.Printf("[issueSelfHostedJWT] ERROR: Failed to call fw-parse: %v\n", err)
		return "", fmt.Errorf("failed to call fw-parse: %w", err)
	}

	fmt.Printf("[issueSelfHostedJWT] fw-parse response status: %d\n", statusCode)
	fmt.Printf("[issueSelfHostedJWT] fw-parse response body: %s\n", string(respBody))

	if statusCode != 200 {
		return "", fmt.Errorf("fw-parse returned status %d: %s", statusCode, string(respBody))
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
