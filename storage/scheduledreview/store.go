package scheduledreview

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Config represents a per-repo scheduled review configuration; ProjectFullName/ConnectorID are joined in from repositories, not stored here.
type Config struct {
	ID              int64
	OrgID           int64
	RepositoryID    int64
	ProjectFullName string
	ConnectorID     int64
	Enabled         bool
	CronExpression  string
	DefaultBranch   sql.NullString
	LastSyncedSHA   sql.NullString
	LastRunAt       sql.NullTime
	NextRunAt       time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// baseSelect joins in the identifying fields (full_name, connector_id) from repositories.
const baseSelect = `
	SELECT src.id, src.org_id, src.repository_id, r.full_name, r.connector_id, src.enabled, src.cron_expression,
		src.default_branch, src.last_synced_sha, src.last_run_at, src.next_run_at, src.created_at, src.updated_at
	FROM scheduled_review_configs src
	JOIN repositories r ON r.id = src.repository_id
`

func scanConfig(row interface {
	Scan(dest ...interface{}) error
}) (*Config, error) {
	var c Config
	err := row.Scan(
		&c.ID, &c.OrgID, &c.RepositoryID, &c.ProjectFullName, &c.ConnectorID, &c.Enabled, &c.CronExpression,
		&c.DefaultBranch, &c.LastSyncedSHA, &c.LastRunAt, &c.NextRunAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListDue returns all enabled configs whose next_run_at has passed.
func (s *Store) ListDue(ctx context.Context) ([]*Config, error) {
	query := baseSelect + `WHERE src.enabled = true AND src.next_run_at <= NOW() ORDER BY src.next_run_at ASC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*Config
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// NextDueTime returns the soonest next_run_at among enabled configs, if any - used by the scheduler to size its sleep.
func (s *Store) NextDueTime(ctx context.Context) (time.Time, bool, error) {
	var next sql.NullTime
	err := s.db.QueryRowContext(ctx, `SELECT MIN(next_run_at) FROM scheduled_review_configs WHERE enabled = true`).Scan(&next)
	if err != nil {
		return time.Time{}, false, err
	}
	if !next.Valid {
		return time.Time{}, false, nil
	}
	return next.Time, true, nil
}

// GetByID fetches a single config by its ID.
func (s *Store) GetByID(ctx context.Context, id int64) (*Config, error) {
	query := baseSelect + `WHERE src.id = $1`
	return scanConfig(s.db.QueryRowContext(ctx, query, id))
}

// GetByRepository fetches a repo's existing config, if any; returns (nil, nil) when no row exists yet.
func (s *Store) GetByRepository(ctx context.Context, repositoryID int64) (*Config, error) {
	query := baseSelect + `WHERE src.repository_id = $1`
	cfg, err := scanConfig(s.db.QueryRowContext(ctx, query, repositoryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// ListByConnector returns all repo configs under a given connector, for the connector's repo list UI.
func (s *Store) ListByConnector(ctx context.Context, connectorID int64) ([]*Config, error) {
	query := baseSelect + `WHERE r.connector_id = $1`
	rows, err := s.db.QueryContext(ctx, query, connectorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []*Config
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// Upsert creates or updates a repo's scheduled-review config; nextRunAt is computed by the caller (see SetScheduledReview), not here.
func (s *Store) Upsert(ctx context.Context, orgID, repositoryID int64, enabled bool, cronExpression string, nextRunAt time.Time) (*Config, error) {
	query := `
		INSERT INTO scheduled_review_configs (org_id, repository_id, enabled, cron_expression, next_run_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (repository_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			cron_expression = EXCLUDED.cron_expression,
			next_run_at = EXCLUDED.next_run_at,
			updated_at = NOW()
		RETURNING id
	`
	var id int64
	if err := s.db.QueryRowContext(ctx, query, orgID, repositoryID, enabled, cronExpression, nextRunAt).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

// UpdateCheckpoint records a completed run's branch/SHA checkpoint and the next due time.
func (s *Store) UpdateCheckpoint(ctx context.Context, id int64, defaultBranch, lastSyncedSHA string, ranAt, nextRunAt time.Time) error {
	query := `
		UPDATE scheduled_review_configs
		SET default_branch = $1, last_synced_sha = $2, last_run_at = $3, next_run_at = $4, updated_at = NOW()
		WHERE id = $5
	`
	_, err := s.db.ExecContext(ctx, query, defaultBranch, lastSyncedSHA, ranAt, nextRunAt, id)
	return err
}

// Claim pushes next_run_at forward without touching the checkpoint, so it isn't re-enqueued while its job is still running.
func (s *Store) Claim(ctx context.Context, id int64, until time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scheduled_review_configs SET next_run_at = $1, updated_at = NOW() WHERE id = $2`, until, id)
	return err
}
