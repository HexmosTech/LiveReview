package teamsbot

import (
	"context"
	"database/sql"
	"time"
)

type TeamsConfig struct {
	ID          int64     `json:"id"`
	OrgID       int64     `json:"org_id"`
	BotAppID    string    `json:"bot_app_id"`
	BotPassword string    `json:"-"`
	APIKey      string    `json:"api_key,omitempty"`
	TenantID    string    `json:"tenant_id"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Storage struct {
	db *sql.DB
}

func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) GetTeamsConfig(ctx context.Context, orgID int64) (*TeamsConfig, error) {
	query := `
		SELECT id, org_id, bot_app_id, bot_password, api_key, tenant_id, enabled, created_at, updated_at
		FROM org_teams_configs
		WHERE org_id = $1`

	cfg := &TeamsConfig{}
	err := s.db.QueryRowContext(ctx, query, orgID).Scan(
		&cfg.ID, &cfg.OrgID, &cfg.BotAppID, &cfg.BotPassword, &cfg.APIKey, &cfg.TenantID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Storage) UpsertTeamsConfig(ctx context.Context, orgID int64, botAppID, botPassword, apiKey string) (*TeamsConfig, error) {
	query := `
		INSERT INTO org_teams_configs (org_id, bot_app_id, bot_password, api_key, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, true, NOW(), NOW())
		ON CONFLICT (org_id)
		DO UPDATE SET bot_app_id = $2, bot_password = $3, api_key = $4, enabled = true, updated_at = NOW()
		RETURNING id, org_id, bot_app_id, bot_password, api_key, tenant_id, enabled, created_at, updated_at`

	cfg := &TeamsConfig{}
	err := s.db.QueryRowContext(ctx, query, orgID, botAppID, botPassword, apiKey).Scan(
		&cfg.ID, &cfg.OrgID, &cfg.BotAppID, &cfg.BotPassword, &cfg.APIKey, &cfg.TenantID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Storage) UpdateTenantID(ctx context.Context, orgID int64, tenantID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE org_teams_configs SET tenant_id = $1, updated_at = NOW() WHERE org_id = $2`, tenantID, orgID)
	return err
}

func (s *Storage) DeleteTeamsConfig(ctx context.Context, orgID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM org_teams_configs WHERE org_id = $1`, orgID)
	return err
}

func (s *Storage) GetAllEnabledConfigs(ctx context.Context) ([]TeamsConfig, error) {
	query := `
		SELECT id, org_id, bot_app_id, bot_password, api_key, tenant_id, enabled, created_at, updated_at
		FROM org_teams_configs
		WHERE enabled = true
		ORDER BY org_id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []TeamsConfig
	for rows.Next() {
		var cfg TeamsConfig
		if err := rows.Scan(&cfg.ID, &cfg.OrgID, &cfg.BotAppID, &cfg.BotPassword, &cfg.APIKey, &cfg.TenantID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}
