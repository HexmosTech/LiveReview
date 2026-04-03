package license

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AccountSuccessRecord struct {
	OrgID              int64
	ReviewID           int64
	ActorUserID        *int64
	ActorEmail         string
	OperationType      string
	TriggerSource      string
	OperationID        string
	IdempotencyKey     string
	BillableLOC        int64
	BillingPeriodStart time.Time
	BillingPeriodEnd   time.Time
	PlanCode           string
	MonthlyLOCLimit    int64
	Provider           string
	Model              string
	PricingVersion     string
	InputTokens        *int64
	OutputTokens       *int64
	CostUSD            *float64
}

type PreflightQuotaResult struct {
	PlanCode           string
	BillingPeriodStart time.Time
	BillingPeriodEnd   time.Time
	LOCUsedMonth       int64
	LOCLimitMonth      int64
	LOCRemainingMonth  int64
	UsagePercent       int
	TrialReadOnly      bool
	TrialEndsAt        *time.Time
	Blocked            bool
}

// LOCAccountingStore centralizes DB accounting operations for LOC billing.
type LOCAccountingStore struct {
	db *sql.DB
}

func NewLOCAccountingStore(db *sql.DB) *LOCAccountingStore {
	return &LOCAccountingStore{db: db}
}

func (s *LOCAccountingStore) AccountSuccess(ctx context.Context, rec AccountSuccessRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin accounting tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO org_billing_state (
			org_id,
			current_plan_code,
			billing_period_start,
			billing_period_end,
			loc_used_month,
			loc_blocked,
			last_reset_at
		) VALUES ($1, $2, $3, $4, 0, FALSE, NOW())
		ON CONFLICT (org_id) DO NOTHING
	`, rec.OrgID, rec.PlanCode, rec.BillingPeriodStart, rec.BillingPeriodEnd)
	if err != nil {
		return fmt.Errorf("ensure org billing state: %w", err)
	}

	var currentUsed int64
	var currentCycleLOCGrant int64
	var currentCycleLOCGrantExpiresAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT loc_used_month, upgrade_loc_grant_current_cycle, upgrade_loc_grant_expires_at
		FROM org_billing_state
		WHERE org_id = $1
		FOR UPDATE
	`, rec.OrgID).Scan(&currentUsed, &currentCycleLOCGrant, &currentCycleLOCGrantExpiresAt); err != nil {
		return fmt.Errorf("lock org billing state: %w", err)
	}

	metadata := map[string]interface{}{"kind": "success_only_accounting"}
	if rec.Provider != "" {
		metadata["provider"] = rec.Provider
	}
	if rec.Model != "" {
		metadata["model"] = rec.Model
	}
	if rec.PricingVersion != "" {
		metadata["pricing_version"] = rec.PricingVersion
	}
	if rec.InputTokens != nil {
		metadata["input_tokens"] = *rec.InputTokens
	}
	if rec.OutputTokens != nil {
		metadata["output_tokens"] = *rec.OutputTokens
	}
	if rec.CostUSD != nil {
		metadata["llm_cost_usd"] = *rec.CostUSD
	}
	actorKind := "unknown"
	if rec.ActorEmail != "" {
		metadata["actor_email"] = rec.ActorEmail
	}
	if rec.ActorUserID != nil && *rec.ActorUserID > 0 {
		actorKind = "member"
		metadata["actor_kind"] = "member"
		metadata["actor_user_id"] = *rec.ActorUserID
	} else if rec.ActorEmail != "" {
		actorKind = "system"
		metadata["actor_kind"] = "system"
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal usage metadata: %w", err)
	}

	var ledgerID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO loc_usage_ledger (
			org_id,
			review_id,
			user_id,
			operation_type,
			trigger_source,
			operation_id,
			idempotency_key,
			billable_loc,
			provider,
			model,
			pricing_version,
			input_tokens,
			output_tokens,
			llm_cost_usd,
			actor_kind,
			actor_email_snapshot,
			accounted_at,
			billing_period_start,
			billing_period_end,
			status,
			metadata
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,NOW(),$17,$18,'accounted',$19
		)
		ON CONFLICT (org_id, idempotency_key) DO NOTHING
		RETURNING id
	`,
		rec.OrgID,
		rec.ReviewID,
		nullUserID(rec.ActorUserID),
		rec.OperationType,
		rec.TriggerSource,
		rec.OperationID,
		rec.IdempotencyKey,
		rec.BillableLOC,
		nullIfEmpty(rec.Provider),
		nullIfEmpty(rec.Model),
		nullIfEmpty(rec.PricingVersion),
		rec.InputTokens,
		rec.OutputTokens,
		rec.CostUSD,
		nullIfEmpty(actorKind),
		nullIfEmpty(strings.TrimSpace(rec.ActorEmail)),
		rec.BillingPeriodStart,
		rec.BillingPeriodEnd,
		metadataJSON,
	).Scan(&ledgerID)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("insert usage ledger: %w", err)
	}

	if ledgerID != 0 {
		newUsed := currentUsed + rec.BillableLOC
		effectiveLimit := rec.MonthlyLOCLimit
		if currentCycleLOCGrant > 0 && currentCycleLOCGrantExpiresAt.Valid && time.Now().UTC().Before(currentCycleLOCGrantExpiresAt.Time.UTC()) {
			effectiveLimit += currentCycleLOCGrant
		}
		locBlocked := effectiveLimit >= 0 && newUsed >= effectiveLimit
		if err := emitThresholdLifecycleEventsTx(ctx, tx, rec.OrgID, rec.PlanCode, rec.BillingPeriodStart, effectiveLimit, currentUsed, newUsed); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE org_billing_state
			SET loc_used_month = $1,
			    loc_blocked = $2,
			    updated_at = NOW()
			WHERE org_id = $3
		`, newUsed, locBlocked, rec.OrgID); err != nil {
			return fmt.Errorf("update org billing state usage: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit accounting tx: %w", err)
	}
	return nil
}

func (s *LOCAccountingStore) CheckQuotaPreflight(ctx context.Context, orgID int64, planCode string, monthlyLOCLimit int64, requiredLOC int64, billingPeriodStart time.Time, billingPeriodEnd time.Time) (PreflightQuotaResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PreflightQuotaResult{}, fmt.Errorf("begin preflight tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO org_billing_state (
			org_id,
			current_plan_code,
			billing_period_start,
			billing_period_end,
			loc_used_month,
			loc_blocked,
			last_reset_at
		) VALUES ($1, $2, $3, $4, 0, FALSE, NOW())
		ON CONFLICT (org_id) DO NOTHING
	`, orgID, planCode, billingPeriodStart, billingPeriodEnd)
	if err != nil {
		return PreflightQuotaResult{}, fmt.Errorf("ensure org billing state: %w", err)
	}

	var currentPlanCode string
	var currentPeriodStart time.Time
	var currentPeriodEnd time.Time
	var currentUsed int64
	var currentCycleLOCGrant int64
	var currentCycleLOCGrantExpiresAt sql.NullTime
	var currentTrialReadOnly bool
	var currentTrialEndsAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT current_plan_code, billing_period_start, billing_period_end, loc_used_month, upgrade_loc_grant_current_cycle, upgrade_loc_grant_expires_at, trial_readonly, trial_ends_at
		FROM org_billing_state
		WHERE org_id = $1
		FOR UPDATE
	`, orgID).Scan(&currentPlanCode, &currentPeriodStart, &currentPeriodEnd, &currentUsed, &currentCycleLOCGrant, &currentCycleLOCGrantExpiresAt, &currentTrialReadOnly, &currentTrialEndsAt); err != nil {
		return PreflightQuotaResult{}, fmt.Errorf("lock org billing state: %w", err)
	}

	now := time.Now().UTC()
	if now.Before(currentPeriodStart) || !now.Before(currentPeriodEnd) {
		if err := emitLifecycleEventTx(ctx, tx, orgID, "billing_period_reset", nil, planCode, map[string]interface{}{
			"previous_period_start": currentPeriodStart.Format(time.RFC3339),
			"previous_period_end":   currentPeriodEnd.Format(time.RFC3339),
			"new_period_start":      billingPeriodStart.Format(time.RFC3339),
			"new_period_end":        billingPeriodEnd.Format(time.RFC3339),
		}, fmt.Sprintf("billing-period-reset:%d:%d", orgID, billingPeriodStart.UTC().Unix())); err != nil {
			return PreflightQuotaResult{}, err
		}
		currentPeriodStart = billingPeriodStart
		currentPeriodEnd = billingPeriodEnd
		currentUsed = 0
		currentCycleLOCGrant = 0
		currentCycleLOCGrantExpiresAt = sql.NullTime{}
	}

	if currentPlanCode != planCode {
		currentPlanCode = planCode
	}

	trialReadOnly := currentTrialReadOnly
	var trialEndsAtPtr *time.Time
	if currentTrialEndsAt.Valid {
		trialEndsAt := currentTrialEndsAt.Time.UTC()
		trialEndsAtPtr = &trialEndsAt
		if !now.Before(trialEndsAt) {
			trialReadOnly = true
		}
	}

	effectiveLimit := monthlyLOCLimit
	if currentCycleLOCGrant > 0 && currentCycleLOCGrantExpiresAt.Valid && now.Before(currentCycleLOCGrantExpiresAt.Time.UTC()) {
		effectiveLimit += currentCycleLOCGrant
	} else {
		currentCycleLOCGrant = 0
		currentCycleLOCGrantExpiresAt = sql.NullTime{}
	}

	locBlockedState := effectiveLimit >= 0 && currentUsed >= effectiveLimit
	if _, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state
		SET current_plan_code = $1,
		    billing_period_start = $2,
		    billing_period_end = $3,
		    loc_used_month = $4,
		    loc_blocked = $5,
		    upgrade_loc_grant_current_cycle = $6,
		    upgrade_loc_grant_expires_at = $7,
		    trial_readonly = $8,
		    updated_at = NOW()
		WHERE org_id = $9
	`, currentPlanCode, currentPeriodStart, currentPeriodEnd, currentUsed, locBlockedState, currentCycleLOCGrant, currentCycleLOCGrantExpiresAt, trialReadOnly, orgID); err != nil {
		return PreflightQuotaResult{}, fmt.Errorf("update preflight state: %w", err)
	}

	remaining := int64(-1)
	usagePercent := 0
	blocked := false
	if effectiveLimit >= 0 {
		remaining = effectiveLimit - currentUsed
		if remaining < 0 {
			remaining = 0
		}
		if effectiveLimit > 0 {
			usagePercent = int((currentUsed * 100) / effectiveLimit)
			if usagePercent > 100 {
				usagePercent = 100
			}
		}
		blocked = remaining < requiredLOC
	}
	if trialReadOnly {
		if err := emitLifecycleEventTx(ctx, tx, orgID, "trial_readonly_active", nil, planCode, map[string]interface{}{
			"trial_ends_at": func() string {
				if trialEndsAtPtr == nil {
					return ""
				}
				return trialEndsAtPtr.UTC().Format(time.RFC3339)
			}(),
		}, fmt.Sprintf("trial-readonly:%d:%s", orgID, currentPeriodStart.UTC().Format("2006-01"))); err != nil {
			return PreflightQuotaResult{}, err
		}
		blocked = true
	}

	if err := tx.Commit(); err != nil {
		return PreflightQuotaResult{}, fmt.Errorf("commit preflight tx: %w", err)
	}

	return PreflightQuotaResult{
		PlanCode:           currentPlanCode,
		BillingPeriodStart: currentPeriodStart,
		BillingPeriodEnd:   currentPeriodEnd,
		LOCUsedMonth:       currentUsed,
		LOCLimitMonth:      effectiveLimit,
		LOCRemainingMonth:  remaining,
		UsagePercent:       usagePercent,
		TrialReadOnly:      trialReadOnly,
		TrialEndsAt:        trialEndsAtPtr,
		Blocked:            blocked,
	}, nil
}

func emitThresholdLifecycleEventsTx(ctx context.Context, tx *sql.Tx, orgID int64, planCode string, periodStart time.Time, limit int64, beforeUsed int64, afterUsed int64) error {
	if limit <= 0 {
		return nil
	}

	beforePct := int((beforeUsed * 100) / limit)
	afterPct := int((afterUsed * 100) / limit)
	for _, threshold := range []int{80, 95, 100} {
		if beforePct < threshold && afterPct >= threshold {
			th := threshold
			if err := emitLifecycleEventTx(ctx, tx, orgID, "usage_threshold_reached", &th, planCode, map[string]interface{}{
				"threshold_percent": threshold,
				"usage_before":      beforeUsed,
				"usage_after":       afterUsed,
				"monthly_limit":     limit,
				"period_start":      periodStart.UTC().Format(time.RFC3339),
			}, fmt.Sprintf("usage-threshold:%d:%s:%d", orgID, periodStart.UTC().Format("2006-01"), threshold)); err != nil {
				return err
			}

			if threshold == 80 || threshold == 95 {
				if err := enqueueQuotaThresholdNotificationsTx(ctx, tx, orgID, threshold, planCode, periodStart, beforeUsed, afterUsed, limit); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func enqueueQuotaThresholdNotificationsTx(ctx context.Context, tx *sql.Tx, orgID int64, threshold int, planCode string, periodStart time.Time, beforeUsed int64, afterUsed int64, limit int64) error {
	recipientRows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT u.id, COALESCE(NULLIF(trim(u.email), ''), '') AS email
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN users u ON u.id = ur.user_id
		WHERE ur.org_id = $1
		  AND u.is_active = TRUE
		  AND r.name IN ('owner', 'admin', 'super_admin')
	`, orgID)
	if err != nil {
		return fmt.Errorf("load quota notification recipients: %w", err)
	}
	defer recipientRows.Close()

	payload := map[string]interface{}{
		"event_type":        fmt.Sprintf("quota_%d", threshold),
		"org_id":            orgID,
		"threshold_percent": threshold,
		"usage_before":      beforeUsed,
		"usage_after":       afterUsed,
		"monthly_limit":     limit,
		"period_start":      periodStart.UTC().Format(time.RFC3339),
		"support_reference": fmt.Sprintf("quota-%d-%d-%s", threshold, orgID, periodStart.UTC().Format("2006-01")),
		"plan_code":         planCode,
		"triggered_at":      time.Now().UTC().Format(time.RFC3339),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal quota notification payload: %w", err)
	}

	for recipientRows.Next() {
		var userID int64
		var email string
		if err := recipientRows.Scan(&userID, &email); err != nil {
			return fmt.Errorf("scan quota notification recipient: %w", err)
		}

		baseDedupe := fmt.Sprintf("quota_%d:%d:%s:%d", threshold, orgID, periodStart.UTC().Format("2006-01"), userID)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO billing_notification_outbox (
				org_id,
				event_type,
				channel,
				dedupe_key,
				payload,
				recipient_user_id,
				status,
				send_after,
				created_at,
				updated_at
			) VALUES (
				$1,
				$2,
				'in_app',
				$3,
				$4::jsonb,
				$5,
				'pending',
				NOW(),
				NOW(),
				NOW()
			)
			ON CONFLICT (channel, dedupe_key) DO NOTHING
		`, orgID, fmt.Sprintf("quota_%d", threshold), baseDedupe+":in_app", string(payloadJSON), userID); err != nil {
			return fmt.Errorf("enqueue in_app quota notification: %w", err)
		}

		email = strings.TrimSpace(email)
		if email == "" {
			continue
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO billing_notification_outbox (
				org_id,
				event_type,
				channel,
				dedupe_key,
				payload,
				recipient_user_id,
				recipient_email,
				status,
				send_after,
				created_at,
				updated_at
			) VALUES (
				$1,
				$2,
				'email',
				$3,
				$4::jsonb,
				$5,
				$6,
				'pending',
				NOW(),
				NOW(),
				NOW()
			)
			ON CONFLICT (channel, dedupe_key) DO NOTHING
		`, orgID, fmt.Sprintf("quota_%d", threshold), baseDedupe+":email", string(payloadJSON), userID, email); err != nil {
			return fmt.Errorf("enqueue email quota notification: %w", err)
		}
	}

	if err := recipientRows.Err(); err != nil {
		return fmt.Errorf("iterate quota notification recipients: %w", err)
	}

	return nil
}

func emitLifecycleEventTx(ctx context.Context, tx *sql.Tx, orgID int64, eventType string, thresholdPercent *int, planCode string, payload map[string]interface{}, eventKey string) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal lifecycle payload: %w", err)
	}

	var thresholdValue interface{}
	if thresholdPercent != nil {
		thresholdValue = *thresholdPercent
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO loc_lifecycle_log (
			org_id,
			event_type,
			threshold_percent,
			plan_code,
			event_key,
			payload,
			notified_email,
			created_at
		) VALUES (
			$1,
			$2,
			$3,
			NULLIF($4, ''),
			$5,
			$6,
			FALSE,
			NOW()
		)
		ON CONFLICT (org_id, event_key) DO NOTHING
	`, orgID, eventType, thresholdValue, planCode, eventKey, payloadJSON)
	if err != nil {
		return fmt.Errorf("insert lifecycle log: %w", err)
	}
	return nil
}

func nullIfEmpty(v string) interface{} {
	if v == "" {
		return nil
	}
	return v
}

func nullUserID(userID *int64) interface{} {
	if userID == nil || *userID <= 0 {
		return nil
	}
	return *userID
}
