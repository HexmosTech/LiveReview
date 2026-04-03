package license

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type AdminBillingPortfolioStore struct {
	db *sql.DB
}

type AdminBillingPortfolioSummary struct {
	TotalOrgs         int64
	ActiveOrgs        int64
	TotalBillableLOC  int64
	TotalOperations   int64
	LastAccountedAt   sql.NullTime
	NetCollectedCents int64
	FailedPayments    int64
}

type AdminBillingPortfolioOrg struct {
	OrgID             int64
	OrgName           string
	CurrentPlanCode   sql.NullString
	LOCUsedMonth      sql.NullInt64
	LOCBlocked        sql.NullBool
	BillingPeriodEnd  sql.NullTime
	TotalBillableLOC  int64
	OperationCount    int64
	LastAccountedAt   sql.NullTime
	NetCollectedCents int64
	FailedPayments    int64
}

func NewAdminBillingPortfolioStore(db *sql.DB) *AdminBillingPortfolioStore {
	return &AdminBillingPortfolioStore{db: db}
}

func (s *AdminBillingPortfolioStore) GetSummary(ctx context.Context) (AdminBillingPortfolioSummary, error) {
	var summary AdminBillingPortfolioSummary
	err := s.db.QueryRowContext(ctx, `
		WITH usage AS (
			SELECT
				COALESCE(SUM(lul.billable_loc), 0) AS total_billable_loc,
				COUNT(lul.id) AS total_operations,
				MAX(lul.accounted_at) AS last_accounted_at
			FROM loc_usage_ledger lul
			JOIN org_billing_state obs ON obs.org_id = lul.org_id
			WHERE lul.status = 'accounted'
			  AND lul.accounted_at >= obs.billing_period_start
			  AND lul.accounted_at < obs.billing_period_end
		),
		payments AS (
			SELECT
				COALESCE(SUM(CASE WHEN status IN ('execute_applied', 'payment_captured') THEN amount_cents ELSE 0 END), 0) AS net_collected_cents,
				COALESCE(SUM(CASE WHEN status = 'payment_failed' THEN 1 ELSE 0 END), 0) AS failed_payments
			FROM upgrade_payment_attempts
		)
		SELECT
			(SELECT COUNT(*) FROM orgs) AS total_orgs,
			(SELECT COUNT(*) FROM orgs WHERE is_active = TRUE) AS active_orgs,
			u.total_billable_loc,
			u.total_operations,
			u.last_accounted_at,
			p.net_collected_cents,
			p.failed_payments
		FROM usage u
		CROSS JOIN payments p
	`).Scan(
		&summary.TotalOrgs,
		&summary.ActiveOrgs,
		&summary.TotalBillableLOC,
		&summary.TotalOperations,
		&summary.LastAccountedAt,
		&summary.NetCollectedCents,
		&summary.FailedPayments,
	)
	if err != nil {
		return AdminBillingPortfolioSummary{}, fmt.Errorf("get admin billing portfolio summary: %w", err)
	}

	return summary, nil
}

func (s *AdminBillingPortfolioStore) ListOrganizations(ctx context.Context, limit, offset int) ([]AdminBillingPortfolioOrg, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			o.id,
			o.name,
			obs.current_plan_code,
			obs.loc_used_month,
			obs.loc_blocked,
			obs.billing_period_end,
			COALESCE(usage.total_billable_loc, 0) AS total_billable_loc,
			COALESCE(usage.operation_count, 0) AS operation_count,
			usage.last_accounted_at,
			COALESCE(payments.net_collected_cents, 0) AS net_collected_cents,
			COALESCE(payments.failed_payments, 0) AS failed_payments
		FROM orgs o
		LEFT JOIN org_billing_state obs ON obs.org_id = o.id
		LEFT JOIN LATERAL (
			SELECT
				SUM(lul.billable_loc) AS total_billable_loc,
				COUNT(*) AS operation_count,
				MAX(lul.accounted_at) AS last_accounted_at
			FROM loc_usage_ledger lul
			WHERE lul.org_id = o.id
			  AND lul.status = 'accounted'
			  AND (
				obs.org_id IS NULL
				OR (
					lul.accounted_at >= obs.billing_period_start
					AND lul.accounted_at < obs.billing_period_end
				)
			  )
		) usage ON TRUE
		LEFT JOIN LATERAL (
			SELECT
				SUM(CASE WHEN upa.status IN ('execute_applied', 'payment_captured') THEN upa.amount_cents ELSE 0 END) AS net_collected_cents,
				SUM(CASE WHEN upa.status = 'payment_failed' THEN 1 ELSE 0 END) AS failed_payments
			FROM upgrade_payment_attempts upa
			WHERE upa.org_id = o.id
		) payments ON TRUE
		WHERE o.is_active = TRUE
		ORDER BY total_billable_loc DESC, o.id ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list admin billing portfolio orgs: %w", err)
	}
	defer rows.Close()

	items := make([]AdminBillingPortfolioOrg, 0, limit)
	for rows.Next() {
		var item AdminBillingPortfolioOrg
		if err := rows.Scan(
			&item.OrgID,
			&item.OrgName,
			&item.CurrentPlanCode,
			&item.LOCUsedMonth,
			&item.LOCBlocked,
			&item.BillingPeriodEnd,
			&item.TotalBillableLOC,
			&item.OperationCount,
			&item.LastAccountedAt,
			&item.NetCollectedCents,
			&item.FailedPayments,
		); err != nil {
			return nil, fmt.Errorf("scan admin billing portfolio org: %w", err)
		}
		if item.LastAccountedAt.Valid {
			item.LastAccountedAt.Time = item.LastAccountedAt.Time.UTC()
		}
		if item.BillingPeriodEnd.Valid {
			item.BillingPeriodEnd.Time = item.BillingPeriodEnd.Time.UTC()
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate admin billing portfolio orgs: %w", err)
	}

	return items, nil
}

func (s *AdminBillingPortfolioStore) OrganizationExists(ctx context.Context, orgID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM orgs WHERE id = $1)`, orgID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check org existence: %w", err)
	}
	return exists, nil
}

func (s *AdminBillingPortfolioStore) OlderThan(updatedAt time.Time, threshold time.Duration) bool {
	if threshold <= 0 {
		return false
	}
	return time.Since(updatedAt) >= threshold
}
