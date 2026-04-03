package license

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type OrgUsageSummary struct {
	OrgID             int64
	PeriodStart       time.Time
	PeriodEnd         time.Time
	TotalBillableLOC  int64
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCostUSD      float64
	AccountedOps      int64
	TokenTrackedOps   int64
	LatestAccountedAt *time.Time
}

type OrgUsageOperation struct {
	ReviewID      sql.NullInt64
	UserID        sql.NullInt64
	ActorEmail    sql.NullString
	ActorKind     sql.NullString
	OperationType string
	TriggerSource string
	OperationID   string
	BillableLOC   int64
	Provider      sql.NullString
	Model         sql.NullString
	InputTokens   sql.NullInt64
	OutputTokens  sql.NullInt64
	CostUSD       sql.NullFloat64
	AccountedAt   time.Time
}

type OrgMemberUsageSummary struct {
	UserID              sql.NullInt64
	ActorEmail          sql.NullString
	ActorKind           string
	TotalBillableLOC    int64
	OperationCount      int64
	LastAccountedAt     sql.NullTime
	OrgTotalBillableLOC int64
}

type OrgUsageStore struct {
	db *sql.DB
}

func NewOrgUsageStore(db *sql.DB) *OrgUsageStore {
	return &OrgUsageStore{db: db}
}

func (s *OrgUsageStore) GetCurrentPeriodSummary(ctx context.Context, orgID int64) (OrgUsageSummary, error) {
	var summary OrgUsageSummary
	var latest sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT
			obs.org_id,
			obs.billing_period_start,
			obs.billing_period_end,
			COALESCE(SUM(lul.billable_loc), 0) AS total_billable_loc,
			COALESCE(SUM(COALESCE(lul.input_tokens,
				CASE WHEN jsonb_typeof(lul.metadata->'input_tokens') = 'number'
				THEN (lul.metadata->>'input_tokens')::bigint ELSE 0 END)), 0) AS total_input_tokens,
			COALESCE(SUM(COALESCE(lul.output_tokens,
				CASE WHEN jsonb_typeof(lul.metadata->'output_tokens') = 'number'
				THEN (lul.metadata->>'output_tokens')::bigint ELSE 0 END)), 0) AS total_output_tokens,
			COALESCE(SUM(COALESCE(lul.llm_cost_usd,
				CASE WHEN jsonb_typeof(lul.metadata->'llm_cost_usd') = 'number'
				THEN (lul.metadata->>'llm_cost_usd')::double precision ELSE 0 END)), 0) AS total_cost_usd,
			COUNT(lul.id) AS accounted_ops,
			SUM(CASE WHEN lul.input_tokens IS NOT NULL OR lul.output_tokens IS NOT NULL OR lul.llm_cost_usd IS NOT NULL
				OR jsonb_typeof(lul.metadata->'input_tokens') = 'number'
				OR jsonb_typeof(lul.metadata->'output_tokens') = 'number'
				OR jsonb_typeof(lul.metadata->'llm_cost_usd') = 'number'
				THEN 1 ELSE 0 END) AS token_tracked_ops,
			MAX(lul.accounted_at) AS latest_accounted_at
		FROM org_billing_state obs
		LEFT JOIN loc_usage_ledger lul
		  ON lul.org_id = obs.org_id
		 AND lul.status = 'accounted'
		 AND lul.accounted_at >= obs.billing_period_start
		 AND lul.accounted_at < obs.billing_period_end
		WHERE obs.org_id = $1
		GROUP BY obs.org_id, obs.billing_period_start, obs.billing_period_end
	`, orgID).Scan(
		&summary.OrgID,
		&summary.PeriodStart,
		&summary.PeriodEnd,
		&summary.TotalBillableLOC,
		&summary.TotalInputTokens,
		&summary.TotalOutputTokens,
		&summary.TotalCostUSD,
		&summary.AccountedOps,
		&summary.TokenTrackedOps,
		&latest,
	)
	if err != nil {
		return OrgUsageSummary{}, fmt.Errorf("get current period summary: %w", err)
	}

	if latest.Valid {
		t := latest.Time.UTC()
		summary.LatestAccountedAt = &t
	}
	return summary, nil
}

func (s *OrgUsageStore) ListCurrentPeriodOperations(ctx context.Context, orgID int64, actorUserID *int64, limit, offset int) ([]OrgUsageOperation, error) {
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
			lul.review_id,
			lul.user_id,
			COALESCE(u.email, lul.actor_email_snapshot, lul.metadata->>'actor_email') AS actor_email,
			COALESCE(NULLIF(lul.actor_kind, ''), NULLIF(lul.metadata->>'actor_kind', ''), CASE WHEN lul.user_id IS NULL THEN 'system' ELSE 'member' END) AS actor_kind,
			lul.operation_type,
			lul.trigger_source,
			lul.operation_id,
			lul.billable_loc,
			COALESCE(lul.provider, lul.metadata->>'provider') AS provider,
			COALESCE(lul.model, lul.metadata->>'model') AS model,
			COALESCE(lul.input_tokens,
				CASE WHEN jsonb_typeof(lul.metadata->'input_tokens') = 'number'
				THEN (lul.metadata->>'input_tokens')::bigint ELSE NULL END) AS input_tokens,
			COALESCE(lul.output_tokens,
				CASE WHEN jsonb_typeof(lul.metadata->'output_tokens') = 'number'
				THEN (lul.metadata->>'output_tokens')::bigint ELSE NULL END) AS output_tokens,
			COALESCE(lul.llm_cost_usd,
				CASE WHEN jsonb_typeof(lul.metadata->'llm_cost_usd') = 'number'
				THEN (lul.metadata->>'llm_cost_usd')::double precision ELSE NULL END) AS llm_cost_usd,
			lul.accounted_at
		FROM loc_usage_ledger lul
		JOIN org_billing_state obs
		  ON obs.org_id = lul.org_id
		LEFT JOIN users u
		  ON u.id = lul.user_id
		WHERE lul.org_id = $1
		  AND lul.status = 'accounted'
		  AND lul.accounted_at >= obs.billing_period_start
		  AND lul.accounted_at < obs.billing_period_end
		  AND ($4::bigint IS NULL OR lul.user_id = $4)
		ORDER BY lul.accounted_at DESC, lul.id DESC
		LIMIT $2 OFFSET $3
	`, orgID, limit, offset, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("list current period operations: %w", err)
	}
	defer rows.Close()

	ops := make([]OrgUsageOperation, 0, limit)
	for rows.Next() {
		var op OrgUsageOperation
		if err := rows.Scan(
			&op.ReviewID,
			&op.UserID,
			&op.ActorEmail,
			&op.ActorKind,
			&op.OperationType,
			&op.TriggerSource,
			&op.OperationID,
			&op.BillableLOC,
			&op.Provider,
			&op.Model,
			&op.InputTokens,
			&op.OutputTokens,
			&op.CostUSD,
			&op.AccountedAt,
		); err != nil {
			return nil, fmt.Errorf("scan current period operation: %w", err)
		}
		op.AccountedAt = op.AccountedAt.UTC()
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current period operations: %w", err)
	}

	return ops, nil
}

func (s *OrgUsageStore) ListCurrentPeriodMemberUsage(ctx context.Context, orgID int64, limit, offset int) ([]OrgMemberUsageSummary, error) {
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
		WITH grouped AS (
			SELECT
				lul.user_id,
				COALESCE(u.email, lul.actor_email_snapshot, lul.metadata->>'actor_email') AS actor_email,
				COALESCE(NULLIF(lul.actor_kind, ''), NULLIF(lul.metadata->>'actor_kind', ''), CASE WHEN lul.user_id IS NULL THEN 'system' ELSE 'member' END) AS actor_kind,
				SUM(lul.billable_loc) AS total_billable_loc,
				COUNT(*) AS operation_count,
				MAX(lul.accounted_at) AS last_accounted_at,
				SUM(SUM(lul.billable_loc)) OVER () AS org_total_billable_loc
			FROM loc_usage_ledger lul
			JOIN org_billing_state obs
			  ON obs.org_id = lul.org_id
			LEFT JOIN users u
			  ON u.id = lul.user_id
			WHERE lul.org_id = $1
			  AND lul.status = 'accounted'
			  AND lul.accounted_at >= obs.billing_period_start
			  AND lul.accounted_at < obs.billing_period_end
			GROUP BY
				lul.user_id,
				COALESCE(u.email, lul.actor_email_snapshot, lul.metadata->>'actor_email'),
				COALESCE(NULLIF(lul.actor_kind, ''), NULLIF(lul.metadata->>'actor_kind', ''), CASE WHEN lul.user_id IS NULL THEN 'system' ELSE 'member' END)
		)
		SELECT
			user_id,
			actor_email,
			actor_kind,
			total_billable_loc,
			operation_count,
			last_accounted_at,
			org_total_billable_loc
		FROM grouped
		ORDER BY total_billable_loc DESC, operation_count DESC, actor_email ASC
		LIMIT $2 OFFSET $3
	`, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list current period member usage: %w", err)
	}
	defer rows.Close()

	items := make([]OrgMemberUsageSummary, 0, limit)
	for rows.Next() {
		var item OrgMemberUsageSummary
		if err := rows.Scan(
			&item.UserID,
			&item.ActorEmail,
			&item.ActorKind,
			&item.TotalBillableLOC,
			&item.OperationCount,
			&item.LastAccountedAt,
			&item.OrgTotalBillableLOC,
		); err != nil {
			return nil, fmt.Errorf("scan current period member usage: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate current period member usage: %w", err)
	}

	return items, nil
}

func (s *OrgUsageStore) GetCurrentPeriodUsageForActor(ctx context.Context, orgID int64, actorUserID int64) (OrgMemberUsageSummary, error) {
	var item OrgMemberUsageSummary
	err := s.db.QueryRowContext(ctx, `
		WITH totals AS (
			SELECT COALESCE(SUM(lul.billable_loc), 0) AS org_total_billable_loc
			FROM loc_usage_ledger lul
			JOIN org_billing_state obs
			  ON obs.org_id = lul.org_id
			WHERE lul.org_id = $1
			  AND lul.status = 'accounted'
			  AND lul.accounted_at >= obs.billing_period_start
			  AND lul.accounted_at < obs.billing_period_end
		),
		member_usage AS (
			SELECT
				SUM(lul.billable_loc) AS total_billable_loc,
				COUNT(*) AS operation_count,
				MAX(lul.accounted_at) AS last_accounted_at
			FROM loc_usage_ledger lul
			JOIN org_billing_state obs
			  ON obs.org_id = lul.org_id
			WHERE lul.org_id = $1
			  AND lul.user_id = $2
			  AND lul.status = 'accounted'
			  AND lul.accounted_at >= obs.billing_period_start
			  AND lul.accounted_at < obs.billing_period_end
		)
		SELECT
			$2 AS user_id,
			u.email AS actor_email,
			'member' AS actor_kind,
			COALESCE(mu.total_billable_loc, 0) AS total_billable_loc,
			COALESCE(mu.operation_count, 0) AS operation_count,
			mu.last_accounted_at,
			COALESCE(t.org_total_billable_loc, 0) AS org_total_billable_loc
		FROM totals t
		LEFT JOIN member_usage mu ON TRUE
		LEFT JOIN users u ON u.id = $2
	`, orgID, actorUserID).Scan(
		&item.UserID,
		&item.ActorEmail,
		&item.ActorKind,
		&item.TotalBillableLOC,
		&item.OperationCount,
		&item.LastAccountedAt,
		&item.OrgTotalBillableLOC,
	)
	if err != nil {
		return OrgMemberUsageSummary{}, fmt.Errorf("get current period usage for actor: %w", err)
	}

	return item, nil
}
