package discordbot

import (
	"context"
	"database/sql"
	"time"
)

type DiscordConfig struct {
	ID        int64     `json:"id"`
	OrgID     int64     `json:"org_id"`
	BotToken  string    `json:"-"`
	APIKey    string    `json:"api_key,omitempty"`
	GuildID   string    `json:"guild_id"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Storage struct {
	db *sql.DB
}

func NewStorage(db *sql.DB) *Storage {
	return &Storage{db: db}
}

func (s *Storage) GetDiscordConfig(ctx context.Context, orgID int64) (*DiscordConfig, error) {
	query := `
		SELECT id, org_id, bot_token, api_key, guild_id, enabled, created_at, updated_at
		FROM org_discord_configs
		WHERE org_id = $1`

	cfg := &DiscordConfig{}
	err := s.db.QueryRowContext(ctx, query, orgID).Scan(
		&cfg.ID, &cfg.OrgID, &cfg.BotToken, &cfg.APIKey, &cfg.GuildID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Storage) UpsertDiscordConfig(ctx context.Context, orgID int64, botToken, apiKey string) (*DiscordConfig, error) {
	query := `
		INSERT INTO org_discord_configs (org_id, bot_token, api_key, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, true, NOW(), NOW())
		ON CONFLICT (org_id)
		DO UPDATE SET bot_token = $2, api_key = $3, enabled = true, updated_at = NOW()
		RETURNING id, org_id, bot_token, api_key, guild_id, enabled, created_at, updated_at`

	cfg := &DiscordConfig{}
	err := s.db.QueryRowContext(ctx, query, orgID, botToken, apiKey).Scan(
		&cfg.ID, &cfg.OrgID, &cfg.BotToken, &cfg.APIKey, &cfg.GuildID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Storage) UpdateGuildID(ctx context.Context, orgID int64, guildID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE org_discord_configs SET guild_id = $1, updated_at = NOW() WHERE org_id = $2`, guildID, orgID)
	return err
}

func (s *Storage) DeleteDiscordConfig(ctx context.Context, orgID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM org_discord_configs WHERE org_id = $1`, orgID)
	return err
}

func (s *Storage) GetAllEnabledConfigs(ctx context.Context) ([]DiscordConfig, error) {
	query := `
		SELECT id, org_id, bot_token, api_key, guild_id, enabled, created_at, updated_at
		FROM org_discord_configs
		WHERE enabled = true
		ORDER BY org_id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []DiscordConfig
	for rows.Next() {
		var cfg DiscordConfig
		if err := rows.Scan(&cfg.ID, &cfg.OrgID, &cfg.BotToken, &cfg.APIKey, &cfg.GuildID, &cfg.Enabled, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, err
		}
		configs = append(configs, cfg)
	}
	return configs, rows.Err()
}
