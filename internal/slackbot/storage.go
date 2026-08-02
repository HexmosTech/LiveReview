package slackbot

import (
	"context"
	"database/sql"
	"time"
)

// SlackConfig represents a per-org Slack bot configuration.
type SlackConfig struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	BotToken  string    `json:"bot_token"`
	APIKey    string    `json:"api_key,omitempty"`
	TeamID    string    `json:"team_id"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Storage provides DB access for Slack bot configs.
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new Storage.
func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

// GetSlackConfig returns the slack config for an org, or sql.ErrNoRows if none.
func (s *Storage) GetSlackConfig(ctx context.Context, orgID int64) (*SlackConfig, error) {
	query := `
		SELECT id, org_id, bot_token, api_key, team_id, enabled, created_at, updated_at
		FROM org_slack_configs
		WHERE org_id = $1`

	cfg := &SlackConfig{}
	err := s.db.QueryRowContext(ctx, query, orgID).Scan(
		&cfg.ID, &cfg.OrgID, &cfg.BotToken, &cfg.APIKey, &cfg.TeamID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// UpsertSlackConfig creates or updates the slack config for an org.
func (s *Storage) UpsertSlackConfig(ctx context.Context, orgID int64, botToken, apiKey string) (*SlackConfig, error) {
	query := `
		INSERT INTO org_slack_configs (org_id, bot_token, api_key, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, true, NOW(), NOW())
		ON CONFLICT (org_id)
		DO UPDATE SET bot_token = $2, api_key = $3, enabled = true, updated_at = NOW()
		RETURNING id, org_id, bot_token, api_key, team_id, enabled, created_at, updated_at`

	cfg := &SlackConfig{}
	err := s.db.QueryRowContext(ctx, query, orgID, botToken, apiKey).Scan(
		&cfg.ID, &cfg.OrgID, &cfg.BotToken, &cfg.APIKey, &cfg.TeamID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// UpdateTeamID stores the Slack workspace team_id for an org config.
func (s *Storage) UpdateTeamID(ctx context.Context, orgID int64, teamID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE org_slack_configs SET team_id = $1, updated_at = NOW() WHERE org_id = $2`, teamID, orgID)
	return err
}

// DeleteSlackConfig removes the slack config for an org.
func (s *Storage) DeleteSlackConfig(ctx context.Context, orgID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM org_slack_configs WHERE org_id = $1`, orgID)
	return err
}

// GetAllEnabledConfigs returns all enabled slack configs.
func (s *Storage) GetAllEnabledConfigs(ctx context.Context) ([]SlackConfig, error) {
	query := `
		SELECT id, org_id, bot_token, api_key, team_id, enabled, created_at, updated_at
		FROM org_slack_configs
		WHERE enabled = true
		ORDER BY org_id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []SlackConfig
	for rows.Next() {
		var cfg SlackConfig
		if err := rows.Scan(&cfg.ID, &cfg.OrgID, &cfg.BotToken, &cfg.APIKey, &cfg.TeamID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}
