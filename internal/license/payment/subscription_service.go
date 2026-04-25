package payment

import (
	"context"
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

	"github.com/livereview/internal/aidefault"
	"github.com/livereview/internal/license"
	networkpayment "github.com/livereview/network/payment"
	storagelicense "github.com/livereview/storage/license"
	storagepayment "github.com/livereview/storage/payment"
	"golang.org/x/crypto/bcrypt"
)

const (
	PricingProfileActual         = "actual"
	PricingProfileLowPricingTest = "low_pricing_test"
	CurrencyUSD                  = "USD"
	CurrencyINR                  = "INR"
	firstPurchaseTrialDays       = 7
)

// NormalizeCurrency validates and returns a supported billing currency.
func NormalizeCurrency(raw string) (string, error) {
	currency := strings.ToUpper(strings.TrimSpace(raw))
	switch currency {
	case CurrencyUSD, CurrencyINR:
		return currency, nil
	default:
		return "", fmt.Errorf("unsupported currency %q (allowed: %s, %s)", raw, CurrencyUSD, CurrencyINR)
	}
}

// ResolvePricingProfile validates the active pricing profile for live mode.
func ResolvePricingProfile() (string, error) {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("LIVEREVIEW_PRICING_PROFILE")))
	switch profile {
	case PricingProfileActual, PricingProfileLowPricingTest:
		return profile, nil
	default:
		return "", fmt.Errorf("LIVEREVIEW_PRICING_PROFILE must be set to '%s' or '%s'", PricingProfileActual, PricingProfileLowPricingTest)
	}
}

// GetPlanID returns the appropriate Razorpay plan ID based on mode, pricing profile, and currency.
func GetPlanID(mode, planType, currency string) (string, error) {
	planType = strings.ToLower(strings.TrimSpace(planType))
	if planType != "monthly" && planType != "yearly" {
		return "", fmt.Errorf("invalid plan type: %s", planType)
	}

	normalizedCurrency, err := NormalizeCurrency(currency)
	if err != nil {
		return "", err
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "test":
		switch normalizedCurrency {
		case CurrencyUSD:
			if planType == "monthly" {
				return strings.TrimSpace(os.Getenv("RAZORPAY_TEST_MONTHLY_PLAN_ID_USD")), nil
			}
			return strings.TrimSpace(os.Getenv("RAZORPAY_TEST_YEARLY_PLAN_ID_USD")), nil
		case CurrencyINR:
			if planType == "monthly" {
				return strings.TrimSpace(os.Getenv("RAZORPAY_TEST_MONTHLY_PLAN_ID_INR")), nil
			}
			return strings.TrimSpace(os.Getenv("RAZORPAY_TEST_YEARLY_PLAN_ID_INR")), nil
		default:
			return "", fmt.Errorf("unsupported test currency: %s", normalizedCurrency)
		}
	case "live":
		profile, err := ResolvePricingProfile()
		if err != nil {
			return "", err
		}

		switch profile {
		case PricingProfileActual:
			switch normalizedCurrency {
			case CurrencyUSD:
				if planType == "monthly" {
					return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_USD")), nil
				}
				return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_USD")), nil
			case CurrencyINR:
				if planType == "monthly" {
					return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID_INR")), nil
				}
				return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID_INR")), nil
			default:
				return "", fmt.Errorf("unsupported live currency: %s", normalizedCurrency)
			}
		case PricingProfileLowPricingTest:
			switch normalizedCurrency {
			case CurrencyUSD:
				if planType == "monthly" {
					return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_USD")), nil
				}
				return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_USD")), nil
			case CurrencyINR:
				if planType == "monthly" {
					return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID_INR")), nil
				}
				return strings.TrimSpace(os.Getenv("RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID_INR")), nil
			default:
				return "", fmt.Errorf("unsupported live currency: %s", normalizedCurrency)
			}
		default:
			return "", fmt.Errorf("unsupported pricing profile: %s", profile)
		}
	default:
		return "", fmt.Errorf("invalid mode: %s (must be 'test' or 'live')", mode)
	}
}

// SubscriptionService handles business logic for subscriptions, wrapping the payment package
type SubscriptionService struct {
	db *sql.DB
}

var ErrCancellationNotVerified = errors.New("cancellation not verified with razorpay")
var ErrKeepPlanNotVerified = errors.New("keep plan not verified with razorpay")

const (
	cancelVerificationMaxAttempts = 5
	cancelVerificationBaseDelay   = 600 * time.Millisecond
	confirmFetchMaxAttempts       = 4
	confirmFetchBaseDelay         = 400 * time.Millisecond
)

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

func generateTrialReservationToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate trial reservation token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func computeTrialWindow(now time.Time, days int) (int64, int64, int64) {
	trialStart := now.UTC()
	if days <= 0 {
		days = firstPurchaseTrialDays
	}
	trialEnd := trialStart.AddDate(0, 0, days)
	return trialEnd.Unix(), trialStart.Unix(), trialEnd.Unix()
}

func (s *SubscriptionService) lookupUserEmail(ctx context.Context, userID int) (string, error) {
	if userID <= 0 {
		return "", fmt.Errorf("owner user id must be > 0")
	}

	var email sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT email
		FROM users
		WHERE id = $1
		LIMIT 1`, userID,
	).Scan(&email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("owner user not found: %d", userID)
		}
		return "", fmt.Errorf("query owner user email: %w", err)
	}

	trimmed := strings.TrimSpace(email.String)
	if trimmed == "" {
		return "", fmt.Errorf("owner user email is empty for user id: %d", userID)
	}

	return trimmed, nil
}

func (s *SubscriptionService) findRecentPendingTrialCheckout(
	ctx context.Context,
	ownerUserID,
	orgID int,
	planCode,
	normalizedEmail,
	reservationToken,
	expectedCurrency,
	expectedPlanID string,
) (*RazorpaySubscription, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("missing db handle")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	trimmedPlanCode := strings.TrimSpace(planCode)
	trimmedEmail := strings.ToLower(strings.TrimSpace(normalizedEmail))
	trimmedToken := strings.TrimSpace(reservationToken)
	trimmedCurrency := strings.ToUpper(strings.TrimSpace(expectedCurrency))
	trimmedPlanID := strings.TrimSpace(expectedPlanID)

	if trimmedPlanCode == "" || trimmedEmail == "" || trimmedCurrency == "" || trimmedPlanID == "" {
		return nil, fmt.Errorf("plan code, normalized email, expected currency, and expected plan id are required")
	}

	args := []interface{}{ownerUserID, orgID, trimmedPlanCode, trimmedEmail, trimmedCurrency, trimmedPlanID}
	query := `
		SELECT razorpay_subscription_id,
		       status,
		       short_url,
		       notes,
		       quantity
		FROM subscriptions
		WHERE owner_user_id = $1
		  AND org_id = $2
		  AND plan_type = $3
		  AND LOWER(TRIM(COALESCE(notes::jsonb ->> 'trial_email', ''))) = $4
		  AND UPPER(TRIM(COALESCE(notes::jsonb ->> 'currency', ''))) = $5
		  AND TRIM(COALESCE(razorpay_plan_id, '')) = $6
		  AND LOWER(TRIM(COALESCE(status, ''))) IN ('created', 'authenticated', 'active', 'pending')`

	if trimmedToken != "" {
		query += `
		  AND TRIM(COALESCE(notes::jsonb ->> 'trial_reservation_token', '')) = $7`
		args = append(args, trimmedToken)
	}

	query += `
		ORDER BY created_at DESC
		LIMIT 1`

	var sub RazorpaySubscription
	var notesBytes []byte
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&sub.ID, &sub.Status, &sub.ShortURL, &notesBytes, &sub.Quantity)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query recent pending trial checkout: %w", err)
	}
	if len(notesBytes) > 0 {
		sub.Notes = json.RawMessage(notesBytes)
	}

	return &sub, nil
}

func (s *SubscriptionService) recoverReservedTrialCheckout(
	ctx context.Context,
	ownerUserID int,
	orgID int,
	planCode string,
	normalizedEmail string,
	reservationState storagelicense.TrialEligibilityState,
	expectedCurrency string,
	expectedPlanID string,
	mode string,
) (*RazorpaySubscription, error) {
	reservationToken := ""
	if reservationState.ReservationToken.Valid {
		reservationToken = reservationState.ReservationToken.String
	}

	localSub, err := s.findRecentPendingTrialCheckout(
		ctx,
		ownerUserID,
		orgID,
		planCode,
		normalizedEmail,
		reservationToken,
		expectedCurrency,
		expectedPlanID,
	)
	if err != nil {
		return nil, err
	}
	if localSub == nil {
		return nil, nil
	}

	providerSub, providerErr := GetSubscriptionByID(mode, localSub.ID)
	if providerErr != nil {
		fmt.Printf("[PAYMENT.SUBSCRIPTION] reuse_pending_trial_checkout_provider_lookup_failed org_id=%d owner_user_id=%d sub_id=%s err=%v\n", orgID, ownerUserID, localSub.ID, providerErr)
		return localSub, nil
	}

	if strings.TrimSpace(providerSub.PlanID) != strings.TrimSpace(expectedPlanID) {
		fmt.Printf("[PAYMENT.SUBSCRIPTION] skip_reuse_pending_trial_checkout_plan_mismatch org_id=%d owner_user_id=%d sub_id=%s expected_plan_id=%s got_plan_id=%s\n", orgID, ownerUserID, localSub.ID, strings.TrimSpace(expectedPlanID), strings.TrimSpace(providerSub.PlanID))
		return nil, nil
	}

	if len(providerSub.Notes) == 0 && len(localSub.Notes) > 0 {
		providerSub.Notes = localSub.Notes
	}
	if strings.TrimSpace(providerSub.Status) == "" && strings.TrimSpace(localSub.Status) != "" {
		providerSub.Status = localSub.Status
	}
	if strings.TrimSpace(providerSub.ShortURL) == "" && strings.TrimSpace(localSub.ShortURL) != "" {
		providerSub.ShortURL = localSub.ShortURL
	}
	if providerSub.Quantity <= 0 && localSub.Quantity > 0 {
		providerSub.Quantity = localSub.Quantity
	}

	return providerSub, nil
}

func isPendingCheckoutStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "created", "authenticated", "active", "pending":
		return true
	default:
		return false
	}
}

func findProviderPendingTrialCheckoutByReservation(mode, normalizedEmail, reservationToken, expectedCurrency, expectedPlanID string) (*RazorpaySubscription, error) {
	trimmedToken := strings.TrimSpace(reservationToken)
	trimmedEmail := strings.ToLower(strings.TrimSpace(normalizedEmail))
	trimmedCurrency := strings.ToUpper(strings.TrimSpace(expectedCurrency))
	trimmedPlanID := strings.TrimSpace(expectedPlanID)

	if trimmedToken == "" || trimmedEmail == "" || trimmedCurrency == "" || trimmedPlanID == "" {
		return nil, nil
	}

	subList, err := GetAllSubscriptions(mode)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions for trial reservation recovery: %w", err)
	}
	if subList == nil || len(subList.Items) == 0 {
		return nil, nil
	}

	for idx := range subList.Items {
		item := subList.Items[idx]
		if !isPendingCheckoutStatus(item.Status) {
			continue
		}
		if strings.TrimSpace(item.PlanID) != trimmedPlanID {
			continue
		}

		notes := item.GetNotesMap()
		if notes == nil {
			continue
		}

		noteToken := strings.TrimSpace(notes["trial_reservation_token"])
		noteEmail := strings.ToLower(strings.TrimSpace(notes["trial_email"]))
		noteCurrency := strings.ToUpper(strings.TrimSpace(notes["currency"]))
		if noteToken != trimmedToken || noteEmail != trimmedEmail || noteCurrency != trimmedCurrency {
			continue
		}

		copied := item
		return &copied, nil
	}

	return nil, nil
}

func (s *SubscriptionService) ensureRecoveredCheckoutPersisted(
	ctx context.Context,
	sub *RazorpaySubscription,
	ownerUserID,
	orgID,
	quantity int,
	dbPlanType,
	razorpayPlanID string,
) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing db handle")
	}
	if sub == nil {
		return fmt.Errorf("subscription payload is required")
	}

	trimmedSubID := strings.TrimSpace(sub.ID)
	if trimmedSubID == "" {
		return fmt.Errorf("subscription id is required")
	}

	if ctx == nil {
		ctx = context.Background()
	}

	var existingID int64
	err := s.db.QueryRowContext(ctx, `
		SELECT id
		FROM subscriptions
		WHERE razorpay_subscription_id = $1
		LIMIT 1`, trimmedSubID,
	).Scan(&existingID)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check recovered subscription persistence: %w", err)
	}

	notes := sub.GetNotesMap()
	if notes == nil {
		notes = map[string]string{}
	}

	persistQuantity := sub.Quantity
	if persistQuantity <= 0 {
		persistQuantity = quantity
	}

	currentPeriodStart := time.Now().UTC()
	if sub.CurrentStart > 0 {
		currentPeriodStart = time.Unix(sub.CurrentStart, 0).UTC()
	}
	currentPeriodEnd := currentPeriodStart.AddDate(0, 1, 0)
	if sub.CurrentEnd > 0 {
		currentPeriodEnd = time.Unix(sub.CurrentEnd, 0).UTC()
	}

	persistStatus := strings.TrimSpace(sub.Status)
	if persistStatus == "" {
		persistStatus = "created"
	}

	store := storagepayment.NewSubscriptionStore(s.db)
	if err := store.CreateTeamSubscriptionRecord(storagepayment.CreateTeamSubscriptionRecordInput{
		SubscriptionID:     trimmedSubID,
		OwnerUserID:        ownerUserID,
		OrgID:              orgID,
		DBPlanType:         dbPlanType,
		Quantity:           persistQuantity,
		Status:             persistStatus,
		RazorpayPlanID:     razorpayPlanID,
		CurrentPeriodStart: currentPeriodStart,
		CurrentPeriodEnd:   currentPeriodEnd,
		LicenseExpiresAt:   currentPeriodEnd,
		ShortURL:           strings.TrimSpace(sub.ShortURL),
		Notes:              notes,
	}); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate key") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil
		}
		return fmt.Errorf("persist recovered trial checkout: %w", err)
	}

	return nil
}

// CreateTeamSubscription creates a new monthly LOC slab subscription via Razorpay and persists to DB.
func (s *SubscriptionService) CreateTeamSubscription(ownerUserID, orgID int, planCode string, mode, currency string) (*RazorpaySubscription, error) {
	persistedPlanCode := license.PlanType(strings.TrimSpace(planCode))
	if !persistedPlanCode.IsValid() {
		return nil, fmt.Errorf("invalid plan_code: %s", planCode)
	}
	if persistedPlanCode.GetLimits().MonthlyPriceUSD <= 0 {
		return nil, fmt.Errorf("plan_code must be a paid LOC slab: %s", planCode)
	}

	resolvedCurrency, err := NormalizeCurrency(currency)
	if err != nil {
		return nil, err
	}

	quantity, err := planCodeToMonthlyQuantity(persistedPlanCode)
	if err != nil {
		return nil, err
	}

	// All LOC slab checkout in this migration is monthly-only.
	razorpayPlanID, err := GetPlanID(mode, "monthly", resolvedCurrency)
	if err != nil {
		return nil, err
	}

	if razorpayPlanID == "" {
		return nil, fmt.Errorf("razorpay monthly plan ID not configured in %s mode", mode)
	}

	resolvedPlan, err := GetPlanByID(mode, razorpayPlanID)
	if err != nil {
		return nil, fmt.Errorf("load razorpay monthly plan details: %w", err)
	}

	planCurrency := strings.ToUpper(strings.TrimSpace(resolvedPlan.Item.Currency))
	if planCurrency == "" {
		return nil, fmt.Errorf("razorpay plan %s returned empty currency", razorpayPlanID)
	}
	if !strings.EqualFold(planCurrency, resolvedCurrency) {
		return nil, fmt.Errorf("razorpay plan currency mismatch: expected %s got %s", resolvedCurrency, planCurrency)
	}

	planUnitMinor := int64(resolvedPlan.Item.Amount)
	if planUnitMinor <= 0 {
		return nil, fmt.Errorf("razorpay plan %s returned invalid amount %d", razorpayPlanID, resolvedPlan.Item.Amount)
	}

	recurringMinor := planUnitMinor * int64(quantity)

	ctx := context.Background()
	ownerEmail, err := s.lookupUserEmail(ctx, ownerUserID)
	if err != nil {
		return nil, err
	}

	normalizedEmail, err := storagelicense.NormalizeTrialEligibilityEmail(ownerEmail)
	if err != nil {
		return nil, err
	}

	trialStore := storagelicense.NewTrialEligibilityStore(s.db)
	trialReservationToken, err := generateTrialReservationToken()
	if err != nil {
		return nil, err
	}

	ownerUserIDInt64 := int64(ownerUserID)
	orgIDInt64 := int64(orgID)
	reservationState, err := trialStore.ReserveFirstPurchaseTrial(ctx, storagelicense.ReserveFirstPurchaseTrialInput{
		Email:            normalizedEmail,
		ReservationToken: trialReservationToken,
		ReservationTTL:   30 * time.Minute,
		ReservedUserID:   &ownerUserIDInt64,
		ReservedOrgID:    &orgIDInt64,
		ReservedPlanCode: persistedPlanCode.String(),
	})
	reserveInput := storagelicense.ReserveFirstPurchaseTrialInput{
		Email:            normalizedEmail,
		ReservationToken: trialReservationToken,
		ReservationTTL:   30 * time.Minute,
		ReservedUserID:   &ownerUserIDInt64,
		ReservedOrgID:    &orgIDInt64,
		ReservedPlanCode: persistedPlanCode.String(),
	}
	if err != nil {
		if errors.Is(err, storagelicense.ErrTrialEligibilityConsumed) {
			reservationState.Consumed = true
		} else if errors.Is(err, storagelicense.ErrTrialEligibilityReserved) {
			recoveredSub, recoverErr := s.recoverReservedTrialCheckout(
				ctx,
				ownerUserID,
				orgID,
				persistedPlanCode.String(),
				normalizedEmail,
				reservationState,
				resolvedCurrency,
				razorpayPlanID,
				mode,
			)
			if recoverErr != nil {
				return nil, fmt.Errorf("recover reserved trial checkout: %w", recoverErr)
			}
			if recoveredSub != nil {
				if persistErr := s.ensureRecoveredCheckoutPersisted(ctx, recoveredSub, ownerUserID, orgID, quantity, persistedPlanCode.String(), razorpayPlanID); persistErr != nil {
					return nil, fmt.Errorf("ensure recovered checkout persisted: %w", persistErr)
				}

				now := time.Now().UTC()
				trialAppliedFromNotes, _, _, trialStartedAt, trialEndsAt := extractTrialConfirmationDetails(recoveredSub, now)
				recoveredSub.TrialApplied = trialAppliedFromNotes
				if recoveredSub.TrialApplied {
					recoveredSub.TrialDays = firstPurchaseTrialDays
					recoveredSub.TrialStartsAt = trialStartedAt.Unix()
					recoveredSub.TrialEndsAt = trialEndsAt.Unix()
				}
				recoveredSub.PlanCurrency = planCurrency
				recoveredSub.PlanUnitMinor = planUnitMinor
				recoveredSub.RecurringMinor = recurringMinor
				recoveredSub.RecurringCurrency = resolvedCurrency
				recoveredSub.CheckoutAuthorizationMayApply = recoveredSub.TrialApplied

				fmt.Printf("[PAYMENT.SUBSCRIPTION] reusing_pending_trial_checkout org_id=%d owner_user_id=%d plan_code=%s subscription_id=%s\n", orgID, ownerUserID, persistedPlanCode.String(), recoveredSub.ID)
				return recoveredSub, nil
			}

			providerRecoveredSub, providerRecoverErr := findProviderPendingTrialCheckoutByReservation(
				mode,
				normalizedEmail,
				reservationState.ReservationToken.String,
				resolvedCurrency,
				razorpayPlanID,
			)
			if providerRecoverErr != nil {
				return nil, fmt.Errorf("recover provider trial checkout: %w", providerRecoverErr)
			}
			if providerRecoveredSub != nil {
				if persistErr := s.ensureRecoveredCheckoutPersisted(ctx, providerRecoveredSub, ownerUserID, orgID, quantity, persistedPlanCode.String(), razorpayPlanID); persistErr != nil {
					return nil, fmt.Errorf("ensure provider recovered checkout persisted: %w", persistErr)
				}

				now := time.Now().UTC()
				trialAppliedFromNotes, _, _, trialStartedAt, trialEndsAt := extractTrialConfirmationDetails(providerRecoveredSub, now)
				providerRecoveredSub.TrialApplied = trialAppliedFromNotes
				if providerRecoveredSub.TrialApplied {
					providerRecoveredSub.TrialDays = firstPurchaseTrialDays
					providerRecoveredSub.TrialStartsAt = trialStartedAt.Unix()
					providerRecoveredSub.TrialEndsAt = trialEndsAt.Unix()
				}
				providerRecoveredSub.PlanCurrency = planCurrency
				providerRecoveredSub.PlanUnitMinor = planUnitMinor
				providerRecoveredSub.RecurringMinor = recurringMinor
				providerRecoveredSub.RecurringCurrency = resolvedCurrency
				providerRecoveredSub.CheckoutAuthorizationMayApply = providerRecoveredSub.TrialApplied

				fmt.Printf("[PAYMENT.SUBSCRIPTION] reusing_provider_pending_trial_checkout org_id=%d owner_user_id=%d plan_code=%s subscription_id=%s\n", orgID, ownerUserID, persistedPlanCode.String(), providerRecoveredSub.ID)
				return providerRecoveredSub, nil
			}

			staleToken := strings.TrimSpace(reservationState.ReservationToken.String)
			if staleToken != "" {
				releaseErr := trialStore.ReleaseTrialReservation(ctx, storagelicense.ReleaseTrialReservationInput{
					Email:            normalizedEmail,
					ReservationToken: staleToken,
				})
				if releaseErr != nil && !errors.Is(releaseErr, storagelicense.ErrTrialEligibilityReservationMismatch) {
					return nil, fmt.Errorf("release stale trial reservation: %w", releaseErr)
				}

				reservationState, err = trialStore.ReserveFirstPurchaseTrial(ctx, reserveInput)
				if err != nil {
					if errors.Is(err, storagelicense.ErrTrialEligibilityConsumed) {
						reservationState.Consumed = true
					} else if errors.Is(err, storagelicense.ErrTrialEligibilityReserved) {
						return nil, fmt.Errorf("trial eligibility reservation already in progress for this email; retry shortly")
					} else {
						return nil, fmt.Errorf("reserve trial eligibility after stale release: %w", err)
					}
				}

				fmt.Printf("[PAYMENT.SUBSCRIPTION] reset_stale_trial_reservation org_id=%d owner_user_id=%d plan_code=%s\n", orgID, ownerUserID, persistedPlanCode.String())
			} else {
				return nil, fmt.Errorf("trial eligibility reservation already in progress for this email; retry shortly")
			}
		} else {
			return nil, fmt.Errorf("reserve trial eligibility: %w", err)
		}
	}

	trialApplied := !reservationState.Consumed
	var trialStartAtUnix int64
	var trialWindowStartUnix int64
	var trialWindowEndUnix int64
	if trialApplied {
		trialStartAtUnix, trialWindowStartUnix, trialWindowEndUnix = computeTrialWindow(time.Now().UTC(), firstPurchaseTrialDays)
	}

	fmt.Printf(
		"[PAYMENT.SUBSCRIPTION] create_team_subscription_prepare org_id=%d owner_user_id=%d plan_code=%s mode=%s currency=%s plan_id=%s quantity=%d plan_unit_minor=%d recurring_minor=%d trial_applied=%t trial_start_at=%d\n",
		orgID,
		ownerUserID,
		persistedPlanCode.String(),
		mode,
		resolvedCurrency,
		razorpayPlanID,
		quantity,
		planUnitMinor,
		recurringMinor,
		trialApplied,
		trialStartAtUnix,
	)

	// Create notes for the subscription
	notes := map[string]string{
		"owner_user_id": fmt.Sprintf("%d", ownerUserID),
		"org_id":        fmt.Sprintf("%d", orgID),
		"plan_type":     persistedPlanCode.String(),
		"currency":      resolvedCurrency,
		"trial_applied": fmt.Sprintf("%t", trialApplied),
		"trial_email":   normalizedEmail,
	}
	if trialApplied {
		notes["trial_days"] = fmt.Sprintf("%d", firstPurchaseTrialDays)
		notes["trial_reservation_token"] = trialReservationToken
		notes["trial_window_start_unix"] = fmt.Sprintf("%d", trialWindowStartUnix)
		notes["trial_window_end_unix"] = fmt.Sprintf("%d", trialWindowEndUnix)
	}

	// Create subscription in Razorpay
	sub, err := CreateSubscriptionAt(mode, razorpayPlanID, quantity, notes, trialStartAtUnix)
	if err != nil {
		if trialApplied {
			_ = trialStore.ReleaseTrialReservation(ctx, storagelicense.ReleaseTrialReservationInput{
				Email:            normalizedEmail,
				ReservationToken: trialReservationToken,
			})
		}
		return nil, fmt.Errorf("failed to create razorpay subscription: %w", err)
	}

	fmt.Printf(
		"[PAYMENT.SUBSCRIPTION] create_team_subscription_created org_id=%d owner_user_id=%d plan_code=%s razorpay_subscription_id=%s status=%s recurring_minor=%d currency=%s trial_applied=%t\n",
		orgID,
		ownerUserID,
		persistedPlanCode.String(),
		sub.ID,
		sub.Status,
		recurringMinor,
		resolvedCurrency,
		trialApplied,
	)

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
		if trialApplied {
			_ = trialStore.ReleaseTrialReservation(ctx, storagelicense.ReleaseTrialReservationInput{
				Email:            normalizedEmail,
				ReservationToken: trialReservationToken,
			})
		}
		return nil, fmt.Errorf("failed to persist subscription: %w", err)
	}

	sub.TrialApplied = trialApplied
	if trialApplied {
		sub.TrialDays = firstPurchaseTrialDays
		sub.TrialStartsAt = trialWindowStartUnix
		sub.TrialEndsAt = trialWindowEndUnix
	}
	sub.PlanCurrency = planCurrency
	sub.PlanUnitMinor = planUnitMinor
	sub.RecurringMinor = recurringMinor
	sub.RecurringCurrency = resolvedCurrency
	sub.CheckoutAuthorizationMayApply = trialApplied

	return sub, nil
}

// UpdateQuantity updates the quantity of an existing subscription
func (s *SubscriptionService) UpdateQuantity(subscriptionID string, quantity int, scheduleChangeAt int64, mode string) (*RazorpaySubscription, error) {
	// Update in Razorpay
	sub, err := UpdateSubscriptionQuantity(mode, subscriptionID, quantity, scheduleChangeAt)
	if err != nil {
		return nil, fmt.Errorf("failed to update razorpay subscription: %w", err)
	}
	persistedScheduleChangeAt := scheduleChangeAt
	if persistedScheduleChangeAt < 0 {
		persistedScheduleChangeAt = 0
	}

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.UpdateSubscriptionQuantityRecord(storagepayment.UpdateSubscriptionQuantityRecordInput{
		SubscriptionID:      subscriptionID,
		Quantity:            sub.Quantity,
		ScheduleChangeAt:    persistedScheduleChangeAt,
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
	return s.CancelSubscriptionWithContext(context.Background(), subscriptionID, immediate, mode)
}

func (s *SubscriptionService) CancelSubscriptionWithContext(ctx context.Context, subscriptionID string, immediate bool, mode string) (*RazorpaySubscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	preCancelSub, err := GetSubscriptionByID(mode, subscriptionID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch razorpay subscription before cancellation: %w", err)
	}

	// Cancel in Razorpay
	sub, err := CancelSubscription(mode, subscriptionID, !immediate)
	if err != nil {
		return nil, fmt.Errorf("failed to cancel razorpay subscription: %w", err)
	}

	postCancelSub, verificationReason, err := verifyCancellationWithRetry(ctx, preCancelSub, sub, immediate, cancelVerificationMaxAttempts, func() (*RazorpaySubscription, error) {
		postSub, getErr := GetSubscriptionByID(mode, subscriptionID)
		if getErr != nil {
			return nil, getErr
		}

		if immediate || hasCycleEndMarkerSignal(postSub) {
			return postSub, nil
		}

		scheduledSub, scheduledErr := RetrieveScheduledChangesByID(mode, subscriptionID)
		if scheduledErr == nil {
			fmt.Printf("[SUBSCRIPTION.CANCEL] retrieve_scheduled_changes returned provider scheduled update for %s\n", subscriptionID)
			return scheduledSub, nil
		}

		if errors.Is(scheduledErr, ErrNoPendingScheduledChange) {
			fmt.Printf("[SUBSCRIPTION.CANCEL] retrieve_scheduled_changes reports no pending update for %s\n", subscriptionID)
			return postSub, nil
		}

		fmt.Printf("[SUBSCRIPTION.CANCEL] retrieve_scheduled_changes failed for %s: %v\n", subscriptionID, scheduledErr)
		return postSub, nil
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to verify cancellation against razorpay: %w", err)
	}

	if verificationReason != "" {
		return nil, fmt.Errorf("%w: %s", ErrCancellationNotVerified, verificationReason)
	}

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.CancelSubscriptionRecord(storagepayment.CancelSubscriptionRecordInput{
		SubscriptionID: subscriptionID,
		Immediate:      immediate,
		Status:         postCancelSub.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist cancellation: %w", err)
	}

	return postCancelSub, nil
}

// KeepPlan clears a scheduled cancellation so the current paid plan continues.
func (s *SubscriptionService) KeepPlan(subscriptionID string, mode string) (*RazorpaySubscription, error) {
	return s.KeepPlanWithContext(context.Background(), subscriptionID, mode)
}

func (s *SubscriptionService) KeepPlanWithContext(ctx context.Context, subscriptionID string, mode string) (*RazorpaySubscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	keepPlanResp, err := CancelScheduledChangesByID(mode, subscriptionID)
	if err != nil && !errors.Is(err, ErrNoPendingScheduledChange) {
		return nil, fmt.Errorf("failed to cancel scheduled razorpay changes: %w", err)
	}

	postKeepPlanSub, verificationReason, err := verifyKeepPlanWithRetry(ctx, cancelVerificationMaxAttempts, func() (*RazorpaySubscription, error) {
		return GetSubscriptionByID(mode, subscriptionID)
	}, func() (*RazorpaySubscription, error) {
		return RetrieveScheduledChangesByID(mode, subscriptionID)
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to verify keep-plan against razorpay: %w", err)
	}

	if verificationReason != "" {
		return nil, fmt.Errorf("%w: %s", ErrKeepPlanNotVerified, verificationReason)
	}

	persistStatus := ""
	if keepPlanResp != nil {
		persistStatus = strings.TrimSpace(keepPlanResp.Status)
	}
	if persistStatus == "" && postKeepPlanSub != nil {
		persistStatus = strings.TrimSpace(postKeepPlanSub.Status)
	}

	store := storagepayment.NewSubscriptionStore(s.db)
	err = store.KeepPlanRecord(ctx, storagepayment.KeepPlanRecordInput{
		SubscriptionID: subscriptionID,
		Status:         persistStatus,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to persist keep-plan action: %w", err)
	}

	if postKeepPlanSub != nil {
		return postKeepPlanSub, nil
	}

	if keepPlanResp != nil {
		return keepPlanResp, nil
	}

	return nil, fmt.Errorf("%w: no provider payload available after verification", ErrKeepPlanNotVerified)
}

func normalizeSubscriptionStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func hasTerminalCancellationSignal(sub *RazorpaySubscription) bool {
	if sub == nil {
		return false
	}
	status := normalizeSubscriptionStatus(sub.Status)
	if status == "cancelled" || status == "completed" || status == "expired" {
		return true
	}
	return sub.EndedAt > 0
}

func hasCycleEndMarkerSignal(sub *RazorpaySubscription) bool {
	if sub == nil {
		return false
	}
	if sub.HasScheduledChanges || sub.ChangeScheduledAt > 0 {
		return true
	}
	return sub.CancelAtCycleEnd || sub.CancelAt > 0
}

func hasCycleEndDelta(pre, post *RazorpaySubscription) bool {
	if pre == nil || post == nil {
		return false
	}
	if pre.EndAt > 0 && post.EndAt > 0 && post.EndAt < pre.EndAt {
		return true
	}
	if post.ChargeAt != 0 && pre.ChargeAt != post.ChargeAt {
		return true
	}
	if pre.RemainingCount > 0 && post.RemainingCount > 0 && post.RemainingCount != pre.RemainingCount {
		return true
	}
	if strings.TrimSpace(post.Status) != "" && normalizeSubscriptionStatus(pre.Status) != normalizeSubscriptionStatus(post.Status) {
		return true
	}
	return false
}

func hasCycleEndCancellationSignal(pre, cancelResponse, post *RazorpaySubscription) bool {
	if hasCycleEndMarkerSignal(cancelResponse) || hasCycleEndMarkerSignal(post) {
		return true
	}
	if hasCycleEndDelta(pre, cancelResponse) || hasCycleEndDelta(pre, post) {
		return true
	}
	return false
}

func safeSubscriptionStatus(sub *RazorpaySubscription) string {
	if sub == nil {
		return ""
	}
	return normalizeSubscriptionStatus(sub.Status)
}

func safeSubscriptionID(sub *RazorpaySubscription) string {
	if sub == nil {
		return ""
	}
	return strings.TrimSpace(sub.ID)
}

func cancellationVerified(preCancelSub, cancelResponseSub, postCancelSub *RazorpaySubscription, immediate bool) (bool, string) {
	if immediate {
		if hasTerminalCancellationSignal(cancelResponseSub) || hasTerminalCancellationSignal(postCancelSub) {
			return true, ""
		}
		return false, "immediate cancellation did not yield terminal cancellation markers"
	}

	if hasTerminalCancellationSignal(cancelResponseSub) || hasTerminalCancellationSignal(postCancelSub) {
		return true, ""
	}
	if hasCycleEndCancellationSignal(preCancelSub, cancelResponseSub, postCancelSub) {
		return true, ""
	}
	return false, "cycle-end cancellation produced no verifiable provider-side state transition"
}

func verifyCancellationWithRetry(
	ctx context.Context,
	preCancelSub, cancelResponseSub *RazorpaySubscription,
	immediate bool,
	maxAttempts int,
	fetchPostCancel func() (*RazorpaySubscription, error),
	sleepFn func(time.Duration),
) (*RazorpaySubscription, string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var lastFetchErr error
	lastReason := "cycle-end cancellation produced no verifiable provider-side state transition"

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}

		postCancelSub, err := fetchPostCancel()
		if err != nil {
			lastFetchErr = err
			fmt.Printf("[SUBSCRIPTION.CANCEL] verification attempt %d/%d failed to fetch post-cancel state: %v\n", attempt, maxAttempts, err)
		} else {
			lastFetchErr = nil
			if ok, reason := cancellationVerified(preCancelSub, cancelResponseSub, postCancelSub, immediate); ok {
				fmt.Printf("[SUBSCRIPTION.CANCEL] verification attempt %d/%d succeeded (immediate=%t, cancel_status=%s, post_status=%s)\n", attempt, maxAttempts, immediate, safeSubscriptionStatus(cancelResponseSub), safeSubscriptionStatus(postCancelSub))
				return postCancelSub, "", nil
			} else {
				lastReason = reason
				fmt.Printf("[SUBSCRIPTION.CANCEL] verification attempt %d/%d not yet verified (immediate=%t, reason=%s, cancel_status=%s, post_status=%s)\n", attempt, maxAttempts, immediate, reason, safeSubscriptionStatus(cancelResponseSub), safeSubscriptionStatus(postCancelSub))
			}
		}

		if attempt < maxAttempts {
			delay := time.Duration(attempt) * cancelVerificationBaseDelay
			if sleepFn != nil {
				sleepFn(delay)
			} else {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, "", ctx.Err()
				case <-timer.C:
				}
			}
		}
	}

	if lastFetchErr != nil {
		return nil, "", lastFetchErr
	}

	fetchErrState := "none"
	if cancelResponseSub != nil {
		fmt.Printf("[SUBSCRIPTION.CANCEL] verification exhausted after %d attempts (immediate=%t, reason=%s, cancel_status=%s, cancel_id=%s, fetch_err=%s)\n", maxAttempts, immediate, lastReason, safeSubscriptionStatus(cancelResponseSub), safeSubscriptionID(cancelResponseSub), fetchErrState)
	} else {
		fmt.Printf("[SUBSCRIPTION.CANCEL] verification exhausted after %d attempts (immediate=%t, reason=%s, cancel_response=nil, fetch_err=%s)\n", maxAttempts, immediate, lastReason, fetchErrState)
	}

	return nil, lastReason, nil
}

func keepPlanVerified(postKeepPlanSub, scheduledSub *RazorpaySubscription, scheduledErr error) (bool, string) {
	if scheduledErr == nil && scheduledSub != nil {
		return false, "provider still reports scheduled changes"
	}

	if scheduledErr != nil && !errors.Is(scheduledErr, ErrNoPendingScheduledChange) {
		return false, "unable to confirm scheduled-change removal from provider"
	}

	if postKeepPlanSub == nil {
		return true, ""
	}

	if hasTerminalCancellationSignal(postKeepPlanSub) {
		return false, "subscription is already terminally cancelled"
	}

	if postKeepPlanSub.CancelAtCycleEnd || postKeepPlanSub.CancelAt > 0 {
		return false, "subscription still has cycle-end cancellation markers"
	}

	return true, ""
}

func verifyKeepPlanWithRetry(
	ctx context.Context,
	maxAttempts int,
	fetchPostKeepPlanSub func() (*RazorpaySubscription, error),
	fetchScheduledSub func() (*RazorpaySubscription, error),
	sleepFn func(time.Duration),
) (*RazorpaySubscription, string, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if ctx == nil {
		ctx = context.Background()
	}

	lastReason := "provider still reports scheduled changes"
	var lastPostErr error
	var lastScheduledErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}

		postKeepPlanSub, postErr := fetchPostKeepPlanSub()
		if postErr != nil {
			lastPostErr = postErr
			fmt.Printf("[SUBSCRIPTION.KEEP_PLAN] verification attempt %d/%d failed to fetch subscription: %v\n", attempt, maxAttempts, postErr)
		} else {
			lastPostErr = nil
		}

		scheduledSub, scheduledErr := fetchScheduledSub()
		if scheduledErr != nil {
			lastScheduledErr = scheduledErr
		} else {
			lastScheduledErr = nil
		}

		if postErr == nil {
			if ok, reason := keepPlanVerified(postKeepPlanSub, scheduledSub, scheduledErr); ok {
				fmt.Printf("[SUBSCRIPTION.KEEP_PLAN] verification attempt %d/%d succeeded (post_status=%s, sub_id=%s)\n", attempt, maxAttempts, safeSubscriptionStatus(postKeepPlanSub), safeSubscriptionID(postKeepPlanSub))
				return postKeepPlanSub, "", nil
			} else {
				lastReason = reason
				fmt.Printf("[SUBSCRIPTION.KEEP_PLAN] verification attempt %d/%d not yet verified (reason=%s, post_status=%s, scheduled_err=%v)\n", attempt, maxAttempts, reason, safeSubscriptionStatus(postKeepPlanSub), scheduledErr)
			}
		}

		if attempt < maxAttempts {
			delay := time.Duration(attempt) * cancelVerificationBaseDelay
			if sleepFn != nil {
				sleepFn(delay)
			} else {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, "", ctx.Err()
				case <-timer.C:
				}
			}
		}
	}

	if lastPostErr != nil {
		return nil, "", lastPostErr
	}

	if lastScheduledErr != nil && !errors.Is(lastScheduledErr, ErrNoPendingScheduledChange) {
		return nil, "", lastScheduledErr
	}

	fetchErrState := "none"
	if lastScheduledErr != nil {
		fetchErrState = lastScheduledErr.Error()
	}
	fmt.Printf("[SUBSCRIPTION.KEEP_PLAN] verification exhausted after %d attempts (reason=%s, fetch_err=%s)\n", maxAttempts, lastReason, fetchErrState)

	return nil, lastReason, nil
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

func extractTrialConfirmationDetails(sub *RazorpaySubscription, now time.Time) (bool, string, string, time.Time, time.Time) {
	if sub == nil {
		return false, "", "", time.Time{}, time.Time{}
	}

	notes := sub.GetNotesMap()
	if notes == nil || !isTrueTrialNote(notes["trial_applied"]) {
		return false, "", "", time.Time{}, time.Time{}
	}

	trialStartAt, ok := parseTrialUnixNote(notes["trial_window_start_unix"])
	if !ok {
		trialStartAt = now.UTC()
	}

	trialEndAt, ok := parseTrialUnixNote(notes["trial_window_end_unix"])
	if !ok && sub.StartAt > 0 {
		trialEndAt = time.Unix(sub.StartAt, 0).UTC()
		ok = true
	}
	if !ok {
		trialEndAt = trialStartAt.AddDate(0, 0, firstPurchaseTrialDays)
	}
	if !trialEndAt.After(trialStartAt) {
		trialEndAt = trialStartAt.AddDate(0, 0, firstPurchaseTrialDays)
	}

	return true,
		strings.TrimSpace(notes["trial_email"]),
		strings.TrimSpace(notes["trial_reservation_token"]),
		trialStartAt,
		trialEndAt
}

func shouldRetryConfirmProviderRead(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "status 400") || strings.Contains(msg, "status 401") || strings.Contains(msg, "status 403") {
		return false
	}

	return true
}

func fetchRazorpayWithRetry[T any](
	ctx context.Context,
	op string,
	maxAttempts int,
	baseDelay time.Duration,
	fetch func() (*T, error),
	sleepFn func(time.Duration),
) (*T, error) {
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if baseDelay <= 0 {
		baseDelay = 250 * time.Millisecond
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		result, err := fetch()
		if err == nil {
			if attempt > 1 {
				fmt.Printf("[PURCHASE.CONFIRM] %s fetch succeeded on attempt %d/%d\n", op, attempt, maxAttempts)
			}
			return result, nil
		}

		lastErr = err
		retryable := shouldRetryConfirmProviderRead(err)
		if !retryable {
			fmt.Printf("[PURCHASE.CONFIRM] %s fetch failed with non-retryable error on attempt %d/%d: %v\n", op, attempt, maxAttempts, err)
			return nil, err
		}
		if attempt == maxAttempts {
			fmt.Printf("[PURCHASE.CONFIRM] %s fetch exhausted after %d attempts: %v\n", op, maxAttempts, err)
			return nil, err
		}

		delay := time.Duration(attempt) * baseDelay
		fmt.Printf("[PURCHASE.CONFIRM] %s fetch attempt %d/%d failed: %v (retrying in %s)\n", op, attempt, maxAttempts, err, delay)
		if sleepFn != nil {
			sleepFn(delay)
			continue
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}

	return nil, lastErr
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

	now := time.Now().UTC()
	ctx := context.Background()

	// Fetch payment details from Razorpay to check if it's captured
	payment, err := fetchRazorpayWithRetry(ctx, "payment", confirmFetchMaxAttempts, confirmFetchBaseDelay, func() (*RazorpayPayment, error) {
		return GetPaymentByID(mode, req.RazorpayPaymentID)
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch payment from Razorpay: %w", err)
	}

	checkoutSubscription, err := fetchRazorpayWithRetry(ctx, "subscription", confirmFetchMaxAttempts, confirmFetchBaseDelay, func() (*RazorpaySubscription, error) {
		return GetSubscriptionByID(mode, req.RazorpaySubscriptionID)
	}, nil)
	if err != nil {
		return fmt.Errorf("failed to fetch subscription from Razorpay: %w", err)
	}

	trialAppliedFromNotes, trialEmailFromNotes, trialReservationToken, trialStartedAt, trialEndsAt := extractTrialConfirmationDetails(checkoutSubscription, now)

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

	resolvedSubscriptionStatus := strings.TrimSpace(checkoutSubscription.Status)

	// Update subscription with payment info
	// Set payment_verified=TRUE if payment is captured
	paymentVerified := bool(payment.Captured)
	_, err = tx.Exec(`
		UPDATE subscriptions
		SET status = CASE WHEN NULLIF($1, '') IS NULL THEN status ELSE $1 END,
		    last_payment_id = $2,
		    last_payment_status = $3,
		    last_payment_received_at = NOW(),
		    payment_verified = $4,
		    updated_at = NOW()
		WHERE id = $5`,
		resolvedSubscriptionStatus, payment.ID, payment.Status, paymentVerified, dbSubscriptionID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription with payment info: %w", err)
	}

	if trialAppliedFromNotes {
		trialEmail := strings.TrimSpace(trialEmailFromNotes)
		if trialEmail == "" {
			trialEmail, err = s.lookupUserEmail(context.Background(), ownerUserID)
			if err != nil {
				return fmt.Errorf("resolve trial email for confirmation: %w", err)
			}
		}

		normalizedEmail, normErr := storagelicense.NormalizeTrialEligibilityEmail(trialEmail)
		if normErr != nil {
			return fmt.Errorf("normalize trial email for confirmation: %w", normErr)
		}

		if trialReservationToken != "" {
			trialStore := storagelicense.NewTrialEligibilityStore(s.db)
			ownerUserIDInt64 := int64(ownerUserID)
			orgIDInt64 := int64(orgID)
			firstSubscriptionID := dbSubscriptionID
			_, consumeErr := trialStore.ConsumeReservedTrialTx(context.Background(), tx, storagelicense.ConsumeReservedTrialInput{
				Email:               normalizedEmail,
				ReservationToken:    trialReservationToken,
				FirstUserID:         &ownerUserIDInt64,
				FirstOrgID:          &orgIDInt64,
				FirstSubscriptionID: &firstSubscriptionID,
				FirstPlanCode:       resolvedPlanCode.String(),
				ConsumedAt:          now,
			})
			if consumeErr != nil && !errors.Is(consumeErr, storagelicense.ErrTrialEligibilityReservationMismatch) && !errors.Is(consumeErr, storagelicense.ErrTrialEligibilityNotFound) {
				return fmt.Errorf("consume trial eligibility during confirmation: %w", consumeErr)
			}
		}
	}

	if paymentVerified || trialAppliedFromNotes {
		periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		periodEnd := periodStart.AddDate(0, 1, 0)
		if checkoutSubscription.CurrentStart > 0 {
			periodStart = time.Unix(checkoutSubscription.CurrentStart, 0).UTC()
		}
		if checkoutSubscription.CurrentEnd > 0 {
			periodEnd = time.Unix(checkoutSubscription.CurrentEnd, 0).UTC()
		}

		var trialStartedAtValue interface{}
		var trialEndsAtValue interface{}
		if trialAppliedFromNotes {
			trialStartedAtValue = trialStartedAt
			trialEndsAtValue = trialEndsAt
		}

		_, err = tx.Exec(`
			INSERT INTO org_billing_state (
				org_id,
				current_plan_code,
				billing_period_start,
				billing_period_end,
				loc_used_month,
				loc_blocked,
				trial_started_at,
				trial_ends_at,
				trial_readonly,
				last_reset_at,
				updated_at
			) VALUES ($1, $2, $3, $4, 0, FALSE, $5, $6, FALSE, NOW(), NOW())
			ON CONFLICT (org_id) DO UPDATE SET
				current_plan_code = EXCLUDED.current_plan_code,
				billing_period_start = EXCLUDED.billing_period_start,
				billing_period_end = EXCLUDED.billing_period_end,
				trial_started_at = COALESCE(org_billing_state.trial_started_at, EXCLUDED.trial_started_at),
				trial_ends_at = CASE
					WHEN EXCLUDED.trial_ends_at IS NULL THEN org_billing_state.trial_ends_at
					WHEN org_billing_state.trial_ends_at IS NULL THEN EXCLUDED.trial_ends_at
					WHEN org_billing_state.trial_ends_at < EXCLUDED.trial_ends_at THEN EXCLUDED.trial_ends_at
					ELSE org_billing_state.trial_ends_at
				END,
				scheduled_plan_code = NULL,
				scheduled_plan_effective_at = NULL,
				upgrade_loc_grant_current_cycle = 0,
				upgrade_loc_grant_expires_at = NULL,
				trial_readonly = FALSE,
				loc_blocked = FALSE,
				updated_at = NOW()
		`, orgID, resolvedPlanCode.String(), periodStart, periodEnd, trialStartedAtValue, trialEndsAtValue)
		if err != nil {
			return fmt.Errorf("failed to update org billing state for confirmed purchase: %w", err)
		}

		// Provision the default AI connector ("LiveReview AI Model") for the organization
		var exists bool
		err = tx.QueryRow(fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM ai_connectors WHERE org_id = $1 AND provider_name = '%s')`, aidefault.ProviderName), orgID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to check managed AI connector existence: %w", err)
		}

		if !exists {
			// Shift existing connectors down to make room at display_order = 1
			_, err = tx.Exec(`UPDATE ai_connectors SET display_order = display_order + 1 WHERE org_id = $1`, orgID)
			if err != nil {
				return fmt.Errorf("failed to shift existing AI connectors: %w", err)
			}

			// Insert the default connector at position 1
			_, err = tx.Exec(fmt.Sprintf(`
				INSERT INTO ai_connectors (
					provider_name, api_key, connector_name, selected_model, display_order, org_id,
					created_at, updated_at
				) VALUES ('%s', 'system_managed', 'LiveReview AI Model', 'default', 1, $1, NOW(), NOW())
			`, aidefault.ProviderName), orgID)
			if err != nil {
				return fmt.Errorf("failed to provision managed AI connector: %w", err)
			}
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
		"subscription_id":     req.RazorpaySubscriptionID,
		"payment_id":          payment.ID,
		"amount":              payment.Amount,
		"status":              payment.Status,
		"captured":            payment.Captured,
		"resolved_plan_code":  resolvedPlanCode.String(),
		"subscription_status": resolvedSubscriptionStatus,
		"trial_applied":       trialAppliedFromNotes,
	}
	if trialAppliedFromNotes {
		metadata["trial_starts_at"] = trialStartedAt.Format(time.RFC3339)
		metadata["trial_ends_at"] = trialEndsAt.Format(time.RFC3339)
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
	razorpayPlanID, err := GetPlanID(mode, "yearly", CurrencyUSD)
	if err != nil {
		return nil, err
	}

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
