package tools

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type CreditUsage struct {
	CreditsUsedMonth  float64 `json:"credits_used_month"`
	CreditsLimitMonth float64 `json:"credits_limit_month"`
	ReviewsRemaining  int     `json:"reviews_remaining"`
	Blocked           bool    `json:"blocked"`
}

type CreditStore struct {
	db *sql.DB
}

func NewCreditStore(db *sql.DB) *CreditStore {
	return &CreditStore{db: db}
}

func (s *CreditStore) ensureAndLockBillingState(ctx context.Context, tx *sql.Tx, orgID int64) (float64, float64, error) {
	now := time.Now().UTC()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	endOfMonth := startOfMonth.AddDate(0, 1, 0)

	_, err := tx.ExecContext(ctx, `
		INSERT INTO public.org_tool_billing_state (
			org_id, credits_used_month, credits_limit_month, billing_period_start, billing_period_end
		) VALUES ($1, 0.0, 50000.0, $2, $3)
		ON CONFLICT (org_id) DO NOTHING
	`, orgID, startOfMonth, endOfMonth)
	if err != nil {
		return 0, 0, fmt.Errorf("ensure tool billing state: %w", err)
	}

	var currentUsed, currentLimit float64
	var periodStart, periodEnd time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT credits_used_month, credits_limit_month, billing_period_start, billing_period_end
		FROM public.org_tool_billing_state
		WHERE org_id = $1
		FOR UPDATE
	`, orgID).Scan(&currentUsed, &currentLimit, &periodStart, &periodEnd)
	if err != nil {
		return 0, 0, fmt.Errorf("lock tool billing state: %w", err)
	}

	// Reset if billing period has ended
	if !now.Before(periodEnd) {
		currentUsed = 0.0
		_, err = tx.ExecContext(ctx, `
			UPDATE public.org_tool_billing_state
			SET credits_used_month = 0.0,
			    billing_period_start = $1,
			    billing_period_end = $2,
			    updated_at = NOW()
			WHERE org_id = $3
		`, startOfMonth, endOfMonth, orgID)
		if err != nil {
			return 0, 0, fmt.Errorf("reset tool billing state: %w", err)
		}
	}

	return currentUsed, currentLimit, nil
}

// GetCreditUsage retrieves the current credit usage for an organization
func (s *CreditStore) GetCreditUsage(ctx context.Context, orgID int64, currentMultiplier float64) (CreditUsage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreditUsage{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	currentUsed, currentLimit, err := s.ensureAndLockBillingState(ctx, tx, orgID)
	if err != nil {
		return CreditUsage{}, err
	}

	if err := tx.Commit(); err != nil {
		return CreditUsage{}, fmt.Errorf("failed to commit credit usage transaction: %w", err)
	}

	remainingCredits := currentLimit - currentUsed
	if remainingCredits < 0 {
		remainingCredits = 0
	}

	reviewsRemaining := 0
	if currentMultiplier > 0 {
		reviewsRemaining = int(remainingCredits / currentMultiplier)
	}

	return CreditUsage{
		CreditsUsedMonth:  currentUsed,
		CreditsLimitMonth: currentLimit,
		ReviewsRemaining:  reviewsRemaining,
		Blocked:           currentMultiplier > 0 && remainingCredits < currentMultiplier,
	}, nil
}

// CheckCreditPreflight checks if the organization has enough credits for the required multiplier
func (s *CreditStore) CheckCreditPreflight(ctx context.Context, orgID int64, requiredMultiplier float64) error {
	usage, err := s.GetCreditUsage(ctx, orgID, requiredMultiplier)
	if err != nil {
		return err
	}
	if usage.Blocked {
		return fmt.Errorf("insufficient tool credits: review requires %.2f credits, but only %.2f remaining this month", requiredMultiplier, usage.CreditsLimitMonth-usage.CreditsUsedMonth)
	}
	return nil
}

// DeductCredits securely deducts the credits and writes to the ledger
func (s *CreditStore) DeductCredits(ctx context.Context, orgID int64, reviewID int64, multiplier float64) error {
	if multiplier <= 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin deduct tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	currentUsed, currentLimit, err := s.ensureAndLockBillingState(ctx, tx, orgID)
	if err != nil {
		return err
	}

	// Double check we have enough credits
	if currentLimit-currentUsed < multiplier {
		return fmt.Errorf("insufficient tool credits during deduction")
	}

	idempotencyKey := fmt.Sprintf("tool_review_%d", reviewID)

	var ledgerID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO public.tool_credit_ledger (
			org_id, review_id, credits_deducted, idempotency_key, created_at
		) VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (org_id, idempotency_key) DO NOTHING
		RETURNING id
	`, orgID, reviewID, multiplier, idempotencyKey).Scan(&ledgerID)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("insert ledger: %w", err)
	}

	// If ledgerID != 0, it means we actually inserted a new ledger row (not a duplicate)
	if ledgerID != 0 {
		newUsed := currentUsed + multiplier
		_, err = tx.ExecContext(ctx, `
			UPDATE public.org_tool_billing_state
			SET credits_used_month = $1,
			    updated_at = NOW()
			WHERE org_id = $2
		`, newUsed, orgID)
		if err != nil {
			return fmt.Errorf("update tool billing state: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit deduct tx: %w", err)
	}

	return nil
}
