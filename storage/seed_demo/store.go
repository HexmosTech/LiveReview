// Package seed_demo holds every DB operation for internal/seed_demo's demo-activity
// seeder, per this repo's storage/business-logic boundary rule (see CLAUDE.md):
// storage/ owns DB access, internal/seed_demo owns the picking/mutation logic and
// calls into this package.
package seed_demo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// SourceReview is one historical review pulled from the org's own real data, along
// with the fields from its linked pull_requests and review_commits rows needed to
// clone a realistic new review against the same PR.
type SourceReview struct {
	ID             int64
	Repository     string
	Branch         string
	PrMrURL        string
	ConnectorID    sql.NullInt64
	TriggerType    string
	UserEmail      sql.NullString
	Provider       sql.NullString
	Metadata       json.RawMessage
	MRTitle        sql.NullString
	AuthorName     sql.NullString
	AuthorUsername sql.NullString
	FriendlyName   sql.NullString
	PullRequestID  sql.NullInt64

	RepositoryID sql.NullInt64 // from review_commits, nil if the source has no commit row

	// Accounting fields, from the source review's loc_usage_ledger row (if any).
	BillableLOC  sql.NullInt64
	InputTokens  sql.NullInt64
	OutputTokens sql.NullInt64
	LLMCostUSD   sql.NullFloat64
	LOCProvider  sql.NullString
	LOCModel     sql.NullString
	LOCPricing   sql.NullString
	ActorUserID  sql.NullInt64
}

// Store centralizes DB access for the seed_demo feature.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// LoadSourcePool returns the org's own completed, non-synthetic reviews to clone from.
// Excludes rows already tagged seed_demo=true so the pool only ever grows from genuine
// historical usage, never from previously generated synthetic rows. Also requires
// metadata to still have both "review_result" and "preloaded_changes" keys: reviews
// whose diff has been offloaded to blob storage by internal/jobqueue's preloaded_changes
// archival sweep (which strips only "preloaded_changes", not "review_result") keep that
// blob keyed to the ORIGINAL review's id - a clone under a new id can't reach it, so
// cloning a source missing either key produces a review the UI can't render.
func (s *Store) LoadSourcePool(ctx context.Context, orgID int64) ([]SourceReview, error) {
	const q = `
		SELECT
			r.id, r.repository, COALESCE(r.branch, ''), COALESCE(r.pr_mr_url, ''),
			r.connector_id, r.trigger_type, r.user_email, r.provider, r.metadata,
			r.mr_title, r.author_name, r.author_username, r.friendly_name, r.pull_request_id,
			rc.repository_id,
			l.billable_loc, l.input_tokens, l.output_tokens, l.llm_cost_usd,
			l.provider, l.model, l.pricing_version, l.user_id
		FROM reviews r
		LEFT JOIN LATERAL (
			SELECT repository_id FROM review_commits WHERE review_id = r.id LIMIT 1
		) rc ON true
		LEFT JOIN LATERAL (
			SELECT billable_loc, input_tokens, output_tokens, llm_cost_usd, provider, model, pricing_version, user_id
			FROM loc_usage_ledger WHERE review_id = r.id LIMIT 1
		) l ON true
		WHERE r.org_id = $1
		  AND r.status = 'completed'
		  AND COALESCE(r.metadata->>'seed_demo', 'false') = 'false'
		  AND r.metadata ? 'review_result'
		  AND r.metadata ? 'preloaded_changes'
		ORDER BY r.id
	`
	rows, err := s.db.QueryContext(ctx, q, orgID)
	if err != nil {
		return nil, fmt.Errorf("query source pool: %w", err)
	}
	defer rows.Close()

	var pool []SourceReview
	for rows.Next() {
		var r SourceReview
		if err := rows.Scan(
			&r.ID, &r.Repository, &r.Branch, &r.PrMrURL,
			&r.ConnectorID, &r.TriggerType, &r.UserEmail, &r.Provider, &r.Metadata,
			&r.MRTitle, &r.AuthorName, &r.AuthorUsername, &r.FriendlyName, &r.PullRequestID,
			&r.RepositoryID,
			&r.BillableLOC, &r.InputTokens, &r.OutputTokens, &r.LLMCostUSD,
			&r.LOCProvider, &r.LOCModel, &r.LOCPricing, &r.ActorUserID,
		); err != nil {
			return nil, fmt.Errorf("scan source pool row: %w", err)
		}
		pool = append(pool, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate source pool: %w", err)
	}
	if len(pool) == 0 {
		return nil, fmt.Errorf("no completed historical reviews found for org %d to seed from", orgID)
	}
	return pool, nil
}

// StageEvent is one review_events "log" row to insert alongside a cloned review.
type StageEvent struct {
	Timestamp time.Time
	Level     string
	Message   string
}

// NewReview is the full set of column values for one synthetic review clone, built by
// internal/seed_demo from a SourceReview plus its mutations.
type NewReview struct {
	OrgID          int64
	Repository     string
	Branch         string
	PrMrURL        string
	ConnectorID    sql.NullInt64
	TriggerType    string
	UserEmail      sql.NullString
	Provider       sql.NullString
	Metadata       json.RawMessage
	MRTitle        sql.NullString
	AuthorName     sql.NullString
	AuthorUsername sql.NullString
	FriendlyName   sql.NullString
	PullRequestID  sql.NullInt64

	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time

	CommitSHA    string
	RepositoryID sql.NullInt64

	ActivityEventData []byte // pre-marshaled JSON for recent_activity.event_data

	StageEvents []StageEvent
}

// CloneReview inserts one new synthetic review, its review_commits, recent_activity,
// and review_events rows, all in a single transaction so a failure partway through
// never leaves an orphaned/partial review behind. Returns the new review's id.
func (s *Store) CloneReview(ctx context.Context, in NewReview) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var reviewID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO reviews (
			repository, branch, commit_hash, pr_mr_url, connector_id, status, trigger_type,
			user_email, provider, created_at, started_at, completed_at, metadata, org_id,
			mr_title, author_name, author_username, friendly_name, pull_request_id
		) VALUES ($1,$2,'',$3,$4,'completed',$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING id
	`,
		in.Repository, in.Branch, in.PrMrURL, in.ConnectorID, in.TriggerType,
		in.UserEmail, in.Provider, in.CreatedAt, in.StartedAt, in.CompletedAt, in.Metadata, in.OrgID,
		in.MRTitle, in.AuthorName, in.AuthorUsername, in.FriendlyName, in.PullRequestID,
	).Scan(&reviewID)
	if err != nil {
		return 0, fmt.Errorf("insert review: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO review_commits (review_id, org_id, repository_id, ref, ref_type, created_at)
		VALUES ($1, $2, $3, $4, 'commit', $5)
		ON CONFLICT (review_id, ref) DO NOTHING
	`, reviewID, in.OrgID, in.RepositoryID, in.CommitSHA, in.CreatedAt); err != nil {
		return 0, fmt.Errorf("insert review commit: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO recent_activity (activity_type, event_data, review_id, org_id, created_at)
		VALUES ('review_triggered', $1, $2, $3, $4)
	`, in.ActivityEventData, reviewID, in.OrgID, in.CreatedAt); err != nil {
		return 0, fmt.Errorf("insert recent_activity: %w", err)
	}

	for _, e := range in.StageEvents {
		data, err := json.Marshal(map[string]interface{}{"message": e.Message})
		if err != nil {
			return 0, fmt.Errorf("marshal stage event data: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO review_events (review_id, org_id, ts, event_type, level, data)
			VALUES ($1, $2, $3, 'log', $4, $5)
		`, reviewID, in.OrgID, e.Timestamp, e.Level, data); err != nil {
			return 0, fmt.Errorf("insert stage event %q: %w", e.Message, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return reviewID, nil
}

// DeleteReview removes a review and every row CloneReview wrote for it. Used to
// compensate when accounting (storage/license.AccountSuccess, which must run after
// CloneReview commits since its idempotency key is scoped to the new review's id)
// fails, so a partially-seeded review never lingers without its LOC accounting.
func (s *Store) DeleteReview(ctx context.Context, reviewID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// review_commits and review_events cascade-delete via reviews_id_fkey ON DELETE
	// CASCADE; recent_activity is ON DELETE SET NULL, so it's deleted explicitly here
	// rather than left behind as an orphaned activity row with a null review_id.
	if _, err := tx.ExecContext(ctx, `DELETE FROM recent_activity WHERE review_id = $1`, reviewID); err != nil {
		return fmt.Errorf("delete recent_activity: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM reviews WHERE id = $1`, reviewID); err != nil {
		return fmt.Errorf("delete review: %w", err)
	}
	return tx.Commit()
}

// CurrentPlan returns the org's current billing plan code and its monthly LOC limit,
// so accounting calls always pass the real limit instead of defaulting to 0 (which
// storage/license.AccountSuccess would treat as "always over quota", incorrectly
// flipping org_billing_state.loc_blocked to true).
func (s *Store) CurrentPlan(ctx context.Context, orgID int64) (planCode string, monthlyLOCLimit int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT bs.current_plan_code, pc.monthly_loc_limit
		FROM org_billing_state bs
		JOIN plan_catalog pc ON pc.plan_code = bs.current_plan_code
		WHERE bs.org_id = $1
	`, orgID).Scan(&planCode, &monthlyLOCLimit)
	if err != nil {
		return "free", 0, fmt.Errorf("load current plan for org %d: %w", orgID, err)
	}
	return planCode, monthlyLOCLimit, nil
}
