package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AuditSettings is the runtime audit policy stored in system_settings under
// the "audit_settings" key. Code defaults are safe if the row is missing.
type AuditSettings struct {
	Enabled          bool   `json:"enabled"`
	HotDays          int    `json:"hot_days"`
	RetentionDays    int    `json:"retention_days"`
	ArchiveEnabled   bool   `json:"archive_enabled"`
	AnchorEnabled    bool   `json:"anchor_enabled"`
	HashChainEnabled bool   `json:"hash_chain_enabled"`
	CronExpression   string `json:"cron_expression"`
}

func DefaultAuditSettings() AuditSettings {
	return AuditSettings{
		Enabled:          true,
		HotDays:          90,
		RetentionDays:    365,
		ArchiveEnabled:   true,
		AnchorEnabled:    true,
		HashChainEnabled: true,
		CronExpression:   "15 3 * * *",
	}
}

func (s AuditSettings) Validate() error {
	if s.HotDays < 1 || s.HotDays > 3650 {
		return fmt.Errorf("hot_days must be between 1 and 3650")
	}
	if s.RetentionDays < s.HotDays || s.RetentionDays > 36500 {
		return fmt.Errorf("retention_days must be at least hot_days and at most 36500")
	}
	if strings.TrimSpace(s.CronExpression) == "" {
		return fmt.Errorf("cron_expression is required")
	}
	return nil
}

// loadAuditSettings reads the policy, falling back to defaults on a missing row
// or malformed JSON. It never fails the caller: audit retention must not stop
// the server from booting.
func loadAuditSettings(ctx context.Context, db *sql.DB) AuditSettings {
	cfg := DefaultAuditSettings()
	if db == nil {
		return cfg
	}
	var data []byte
	err := db.QueryRowContext(ctx, `SELECT data FROM system_settings WHERE name = 'audit_settings'`).Scan(&data)
	if err != nil || len(data) == 0 {
		return cfg
	}
	var parsed AuditSettings
	if err := json.Unmarshal(data, &parsed); err != nil {
		return cfg
	}
	// Zero values in stored JSON should not disable the feature accidentally.
	if parsed.HotDays <= 0 {
		parsed.HotDays = cfg.HotDays
	}
	if parsed.RetentionDays <= 0 {
		parsed.RetentionDays = cfg.RetentionDays
	}
	if strings.TrimSpace(parsed.CronExpression) == "" {
		parsed.CronExpression = cfg.CronExpression
	}
	return parsed
}

func saveAuditSettings(ctx context.Context, tx *sql.Tx, cfg AuditSettings) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO system_settings (name, data, updated_at)
		VALUES ('audit_settings', $1, NOW())
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()`, data)
	return err
}

// auditSettingsForResponse is the non-secret settings shape returned to admins.
func auditSettingsForResponse(cfg AuditSettings) AuditSettings {
	return cfg
}

// auditRetentionMeta is the read-only shape shown to org owners.
type auditRetentionMeta struct {
	HotDays        int       `json:"hot_days"`
	RetentionDays  int       `json:"retention_days"`
	ArchiveEnabled bool      `json:"archive_enabled"`
	AnchorEnabled  bool      `json:"anchor_enabled"`
	CanManage      bool      `json:"can_manage"`
	Now            time.Time `json:"now"`
}
