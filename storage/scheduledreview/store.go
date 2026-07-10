package scheduledreview

import (
	"context"
	"database/sql"
	"time"
)

// Config represents a per-repo scheduled review configuration.
type Config struct {
	ID                 int64
	OrgID              int64
	IntegrationTokenID int64
	ProjectFullName    string
	Enabled            bool
	IntervalHours      int
	DefaultBranch      sql.NullString
	LastSyncedSHA      sql.NullString
	LastRunAt          sql.NullTime
	NextRunAt          time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

const configColumns = `
	id, org_id, integration_token_id, project_full_name, enabled, interval_hours,
	default_branch, last_synced_sha, last_run_at, next_run_at, created_at, updated_at
`

func scanConfig(row interface {
	Scan(dest ...interface{}) error
}) (*Config, error) {
	var c Config
	err := row.Scan(
		&c.ID, &c.OrgID, &c.IntegrationTokenID, &c.ProjectFullName, &c.Enabled, &c.IntervalHours,
		&c.DefaultBranch, &c.LastSyncedSHA, &c.LastRunAt, &c.NextRunAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListDue returns all enabled configs whose next_run_at has passed.
func (s *Store) ListDue(ctx context.Context) ([]*Config, error) {
	query := `SELECT ` + configColumns + ` FROM scheduled_review_configs WHERE enabled = true AND next_run_at <= NOW() ORDER BY next_run_at ASC`
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

// GetByID fetches a single config by its ID.
func (s *Store) GetByID(ctx context.Context, id int64) (*Config, error) {
	query := `SELECT ` + configColumns + ` FROM scheduled_review_configs WHERE id = $1`
	return scanConfig(s.db.QueryRowContext(ctx, query, id))
}

// ListByConnector returns all repo configs for a given connector (integration token), for
// display in the connector's repo list UI.
func (s *Store) ListByConnector(ctx context.Context, integrationTokenID int64) ([]*Config, error) {
	query := `SELECT ` + configColumns + ` FROM scheduled_review_configs WHERE integration_token_id = $1`
	rows, err := s.db.QueryContext(ctx, query, integrationTokenID)
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

// Upsert enables/disables scheduled review for a repo. On first creation (or re-enable),
// next_run_at is set to NOW() so the first run is picked up on the next scheduler tick
// instead of waiting a full interval.
func (s *Store) Upsert(ctx context.Context, orgID, integrationTokenID int64, projectFullName string, enabled bool, intervalHours int) (*Config, error) {
	query := `
		INSERT INTO scheduled_review_configs (org_id, integration_token_id, project_full_name, enabled, interval_hours, next_run_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (integration_token_id, project_full_name) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			interval_hours = EXCLUDED.interval_hours,
			next_run_at = CASE WHEN scheduled_review_configs.enabled = false AND EXCLUDED.enabled = true THEN NOW() ELSE scheduled_review_configs.next_run_at END,
			updated_at = NOW()
		RETURNING ` + configColumns

	return scanConfig(s.db.QueryRowContext(ctx, query, orgID, integrationTokenID, projectFullName, enabled, intervalHours))
}

// UpdateCheckpoint records the result of a completed scheduled-review run: the branch/SHA
// checkpoint to diff from next time, and when the next run is due.
func (s *Store) UpdateCheckpoint(ctx context.Context, id int64, defaultBranch, lastSyncedSHA string, ranAt, nextRunAt time.Time) error {
	query := `
		UPDATE scheduled_review_configs
		SET default_branch = $1, last_synced_sha = $2, last_run_at = $3, next_run_at = $4, updated_at = NOW()
		WHERE id = $5
	`
	_, err := s.db.ExecContext(ctx, query, defaultBranch, lastSyncedSHA, ranAt, nextRunAt, id)
	return err
}

// Claim pushes next_run_at forward without touching the checkpoint, so a config already
// picked up by one scheduler tick isn't immediately re-enqueued by the next tick while its
// job is still running. The worker overwrites next_run_at with the real value (based on
// interval_hours) via UpdateCheckpoint once the run completes.
func (s *Store) Claim(ctx context.Context, id int64, until time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE scheduled_review_configs SET next_run_at = $1, updated_at = NOW() WHERE id = $2`, until, id)
	return err
}
