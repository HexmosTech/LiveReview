package scheduledreview

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// RunOutcome is what happened during one scheduler attempt for a config.
type RunOutcome string

const (
	RunOutcomeReviewed                   RunOutcome = "reviewed"
	RunOutcomeNoChanges                  RunOutcome = "no_changes"
	RunOutcomeFailed                     RunOutcome = "failed"
	RunOutcomeSkippedUnsupportedProvider RunOutcome = "skipped_unsupported_provider"
	RunOutcomeQuotaBlocked               RunOutcome = "quota_blocked"
)

// Run is one row of scheduled_review_runs - a single scheduler attempt, whether or not it produced a review.
type Run struct {
	ID           int64
	ConfigID     int64
	ReviewID     sql.NullInt64
	Outcome      RunOutcome
	Branch       sql.NullString
	BaseSHA      sql.NullString
	HeadSHA      sql.NullString
	CommitCount  int
	ErrorMessage sql.NullString
	StartedAt    time.Time
	CompletedAt  sql.NullTime
}

// InsertRunParams are the fields the worker knows at the end of one attempt; zero values for optional fields are stored as NULL.
type InsertRunParams struct {
	ConfigID     int64
	ReviewID     *int64
	Outcome      RunOutcome
	Branch       string
	BaseSHA      string
	HeadSHA      string
	CommitCount  int
	ErrorMessage string
	StartedAt    time.Time
	CompletedAt  time.Time
}

func nullString(v string) sql.NullString {
	return sql.NullString{String: v, Valid: v != ""}
}

// InsertRun records one scheduler attempt; called from every exit path of ScheduledReviewWorker.Work, not just the successful-review path.
func (s *Store) InsertRun(ctx context.Context, p InsertRunParams) error {
	var reviewID sql.NullInt64
	if p.ReviewID != nil {
		reviewID = sql.NullInt64{Int64: *p.ReviewID, Valid: true}
	}
	query := `
		INSERT INTO scheduled_review_runs
			(config_id, review_id, outcome, branch, base_sha, head_sha, commit_count, error_message, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.db.ExecContext(ctx, query,
		p.ConfigID, reviewID, p.Outcome, nullString(p.Branch), nullString(p.BaseSHA), nullString(p.HeadSHA),
		p.CommitCount, nullString(p.ErrorMessage), p.StartedAt, p.CompletedAt,
	)
	return err
}

// ListRunsParams filters/sorts/paginates a config's run history; Outcomes empty means no outcome filter.
type ListRunsParams struct {
	ConfigID int64
	Outcomes []string
	Desc     bool
	Limit    int
	Offset   int
}

// ListRuns returns a config's run history plus the total count for pagination.
func (s *Store) ListRuns(ctx context.Context, p ListRunsParams) ([]*Run, int, error) {
	args := []interface{}{p.ConfigID}
	where := "WHERE config_id = $1"
	if len(p.Outcomes) > 0 {
		placeholders := make([]string, len(p.Outcomes))
		for i, o := range p.Outcomes {
			args = append(args, o)
			placeholders[i] = fmt.Sprintf("$%d", len(args))
		}
		where += fmt.Sprintf(" AND outcome IN (%s)", strings.Join(placeholders, ","))
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM scheduled_review_runs `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := "ASC"
	if p.Desc {
		order = "DESC"
	}
	args = append(args, p.Limit, p.Offset)
	query := fmt.Sprintf(`
		SELECT id, config_id, review_id, outcome, branch, base_sha, head_sha, commit_count, error_message, started_at, completed_at
		FROM scheduled_review_runs
		%s
		ORDER BY started_at %s
		LIMIT $%d OFFSET $%d
	`, where, order, len(args)-1, len(args))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	runs := make([]*Run, 0)
	for rows.Next() {
		var r Run
		if err := rows.Scan(
			&r.ID, &r.ConfigID, &r.ReviewID, &r.Outcome, &r.Branch, &r.BaseSHA, &r.HeadSHA, &r.CommitCount,
			&r.ErrorMessage, &r.StartedAt, &r.CompletedAt,
		); err != nil {
			return nil, 0, err
		}
		runs = append(runs, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return runs, total, nil
}
