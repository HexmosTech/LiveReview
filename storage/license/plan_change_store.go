package license

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type OrgBillingState struct {
	OrgID                    int64
	CurrentPlanCode          string
	BillingPeriodStart       time.Time
	BillingPeriodEnd         time.Time
	LOCUsedMonth             int64
	UpgradeLOCGrantCurrent   int64
	UpgradeLOCGrantExpiresAt sql.NullTime
	TrialStartedAt           sql.NullTime
	TrialEndsAt              sql.NullTime
	TrialReadOnly            bool
	ScheduledPlanCode        sql.NullString
	ScheduledPlanEffectiveAt sql.NullTime
}

type PlanChangeStore struct {
	db *sql.DB
}

func NewPlanChangeStore(db *sql.DB) *PlanChangeStore {
	return &PlanChangeStore{db: db}
}

func (s *PlanChangeStore) EnsureOrgBillingState(ctx context.Context, orgID int64, defaultPlanCode string) error {
	now := time.Now().UTC()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO org_billing_state (
			org_id,
			current_plan_code,
			billing_period_start,
			billing_period_end,
			loc_used_month,
			loc_blocked,
			trial_readonly,
			last_reset_at
		) VALUES ($1, $2, $3, $4, 0, FALSE, FALSE, NOW())
		ON CONFLICT (org_id) DO NOTHING
	`, orgID, defaultPlanCode, periodStart, periodEnd)
	if err != nil {
		return fmt.Errorf("ensure org billing state: %w", err)
	}
	return nil
}

func (s *PlanChangeStore) GetOrgBillingState(ctx context.Context, orgID int64) (OrgBillingState, error) {
	var row OrgBillingState
	err := s.db.QueryRowContext(ctx, `
		SELECT
			org_id,
			current_plan_code,
			billing_period_start,
			billing_period_end,
			loc_used_month,
			upgrade_loc_grant_current_cycle,
			upgrade_loc_grant_expires_at,
			trial_started_at,
			trial_ends_at,
			trial_readonly,
			scheduled_plan_code,
			scheduled_plan_effective_at
		FROM org_billing_state
		WHERE org_id = $1
	`, orgID).Scan(
		&row.OrgID,
		&row.CurrentPlanCode,
		&row.BillingPeriodStart,
		&row.BillingPeriodEnd,
		&row.LOCUsedMonth,
		&row.UpgradeLOCGrantCurrent,
		&row.UpgradeLOCGrantExpiresAt,
		&row.TrialStartedAt,
		&row.TrialEndsAt,
		&row.TrialReadOnly,
		&row.ScheduledPlanCode,
		&row.ScheduledPlanEffectiveAt,
	)
	if err != nil {
		return OrgBillingState{}, err
	}
	return row, nil
}

func (s *PlanChangeStore) ApplyImmediatePlanUpgrade(ctx context.Context, orgID int64, targetPlanCode string, actorUserID int64, payload map[string]interface{}) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upgrade tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state
		SET current_plan_code = $1,
			scheduled_plan_code = NULL,
			scheduled_plan_effective_at = NULL,
			upgrade_loc_grant_current_cycle = 0,
			upgrade_loc_grant_expires_at = NULL,
			trial_readonly = FALSE,
			updated_at = NOW()
		WHERE org_id = $2
	`, targetPlanCode, orgID); err != nil {
		return fmt.Errorf("update immediate upgrade: %w", err)
	}

	if err := insertLifecycleEventTx(ctx, tx, orgID, "plan_upgraded", targetPlanCode, actorUserID, payload, fmt.Sprintf("plan-upgraded:%d:%s:%d", orgID, targetPlanCode, time.Now().UTC().Unix())); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upgrade tx: %w", err)
	}
	return nil
}

func (s *PlanChangeStore) ScheduleDowngrade(ctx context.Context, orgID int64, targetPlanCode string, effectiveAt time.Time, actorUserID int64, payload map[string]interface{}) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schedule downgrade tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state
		SET scheduled_plan_code = $1,
			scheduled_plan_effective_at = $2,
			updated_at = NOW()
		WHERE org_id = $3
	`, targetPlanCode, effectiveAt.UTC(), orgID); err != nil {
		return fmt.Errorf("update scheduled downgrade: %w", err)
	}

	if err := insertLifecycleEventTx(ctx, tx, orgID, "plan_downgrade_scheduled", targetPlanCode, actorUserID, payload, fmt.Sprintf("plan-downgrade-scheduled:%d:%s:%d", orgID, targetPlanCode, effectiveAt.UTC().Unix())); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schedule downgrade tx: %w", err)
	}
	return nil
}

func (s *PlanChangeStore) ScheduleUpgradeWithCurrentCycleGrant(ctx context.Context, orgID int64, targetPlanCode string, effectiveAt time.Time, currentCycleLOCGrant int64, actorUserID int64, payload map[string]interface{}) error {
	if currentCycleLOCGrant < 0 {
		currentCycleLOCGrant = 0
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schedule upgrade tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state
		SET scheduled_plan_code = $1,
			scheduled_plan_effective_at = $2,
			upgrade_loc_grant_current_cycle = $3,
			upgrade_loc_grant_expires_at = $2,
			trial_readonly = FALSE,
			updated_at = NOW()
		WHERE org_id = $4
	`, targetPlanCode, effectiveAt.UTC(), currentCycleLOCGrant, orgID); err != nil {
		return fmt.Errorf("update scheduled upgrade: %w", err)
	}

	if err := insertLifecycleEventTx(ctx, tx, orgID, "plan_upgrade_scheduled", targetPlanCode, actorUserID, payload, fmt.Sprintf("plan-upgrade-scheduled:%d:%s:%d", orgID, targetPlanCode, effectiveAt.UTC().Unix())); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schedule upgrade tx: %w", err)
	}
	return nil
}

func (s *PlanChangeStore) CancelScheduledDowngrade(ctx context.Context, orgID int64, actorUserID int64, payload map[string]interface{}) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin cancel downgrade tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state
		SET scheduled_plan_code = NULL,
			scheduled_plan_effective_at = NULL,
			updated_at = NOW()
		WHERE org_id = $1
	`, orgID); err != nil {
		return fmt.Errorf("cancel scheduled downgrade: %w", err)
	}

	if err := insertLifecycleEventTx(ctx, tx, orgID, "plan_downgrade_cancelled", "", actorUserID, payload, fmt.Sprintf("plan-downgrade-cancelled:%d:%d", orgID, time.Now().UTC().Unix())); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit cancel downgrade tx: %w", err)
	}
	return nil
}

type DueTransition struct {
	OrgID          int64
	FromPlanCode   string
	TargetPlanCode string
	EffectiveAt    time.Time
}

func (s *PlanChangeStore) ListDueScheduledDowngrades(ctx context.Context, asOf time.Time, limit int) ([]DueTransition, error) {
	return s.ListDueScheduledPlanChanges(ctx, asOf, limit)
}

func (s *PlanChangeStore) ListDueScheduledPlanChanges(ctx context.Context, asOf time.Time, limit int) ([]DueTransition, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT org_id, current_plan_code, scheduled_plan_code, scheduled_plan_effective_at
		FROM org_billing_state
		WHERE scheduled_plan_code IS NOT NULL
		  AND scheduled_plan_effective_at IS NOT NULL
		  AND scheduled_plan_effective_at <= $1
		ORDER BY scheduled_plan_effective_at ASC
		LIMIT $2
	`, asOf.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list due scheduled plan changes: %w", err)
	}
	defer rows.Close()

	out := make([]DueTransition, 0)
	for rows.Next() {
		var d DueTransition
		if err := rows.Scan(&d.OrgID, &d.FromPlanCode, &d.TargetPlanCode, &d.EffectiveAt); err != nil {
			return nil, fmt.Errorf("scan due plan change: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *PlanChangeStore) ApplyScheduledDowngrade(ctx context.Context, tr DueTransition) error {
	return s.ApplyScheduledPlanChange(ctx, tr)
}

func (s *PlanChangeStore) ApplyScheduledPlanChange(ctx context.Context, tr DueTransition) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin apply scheduled plan change tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		UPDATE org_billing_state
		SET current_plan_code = $1,
			scheduled_plan_code = NULL,
			scheduled_plan_effective_at = NULL,
			upgrade_loc_grant_current_cycle = 0,
			upgrade_loc_grant_expires_at = NULL,
			updated_at = NOW()
		WHERE org_id = $2
	`, tr.TargetPlanCode, tr.OrgID); err != nil {
		return fmt.Errorf("apply scheduled plan change: %w", err)
	}

	payload := map[string]interface{}{
		"from_plan_code": tr.FromPlanCode,
		"to_plan_code":   tr.TargetPlanCode,
		"effective_at":   tr.EffectiveAt.UTC().Format(time.RFC3339),
	}
	if err := insertLifecycleEventTx(ctx, tx, tr.OrgID, "plan_change_applied", tr.TargetPlanCode, 0, payload, fmt.Sprintf("plan-change-applied:%d:%s:%d", tr.OrgID, tr.TargetPlanCode, tr.EffectiveAt.UTC().Unix())); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit apply scheduled plan change tx: %w", err)
	}
	return nil
}

func insertLifecycleEventTx(ctx context.Context, tx *sql.Tx, orgID int64, eventType, planCode string, actorUserID int64, payload map[string]interface{}, eventKey string) error {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if actorUserID > 0 {
		payload["actor_user_id"] = actorUserID
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal lifecycle payload: %w", err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO loc_lifecycle_log (
			org_id,
			event_type,
			plan_code,
			event_key,
			payload,
			notified_email,
			created_at
		) VALUES ($1, $2, NULLIF($3, ''), $4, $5, FALSE, NOW())
		ON CONFLICT (org_id, event_key) DO NOTHING
	`, orgID, eventType, planCode, eventKey, payloadJSON)
	if err != nil {
		return fmt.Errorf("insert lifecycle event: %w", err)
	}
	return nil
}
