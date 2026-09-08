package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// DiffArchivalSettingsConfig holds user-facing configuration for the diff archival cron job.
type DiffArchivalSettingsConfig struct {
	Enabled        bool   `json:"enabled"`
	CronExpression string `json:"cron_expression"`
	RetentionDays  int    `json:"retention_days"`
}

// DiffArchivalSettingsResponse includes config plus a human-readable schedule description.
type DiffArchivalSettingsResponse struct {
	DiffArchivalSettingsConfig
	ScheduleHuman string `json:"schedule_human"`
}

func defaultDiffArchivalSettingsConfig() DiffArchivalSettingsConfig {
	return DiffArchivalSettingsConfig{
		Enabled:        true,
		CronExpression: defaultArchivalCronExpr,
		RetentionDays:  defaultArchivalRetentionDays,
	}
}

// GetDiffArchivalSettings returns the current diff archival configuration from system_settings.
func (s *Server) GetDiffArchivalSettings(c echo.Context) error {
	ctx := c.Request().Context()
	cfg := defaultDiffArchivalSettingsConfig()

	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM system_settings WHERE name = 'diff_archival_settings'").Scan(&data)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &cfg)
	}

	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = defaultArchivalRetentionDays
	}
	if strings.TrimSpace(cfg.CronExpression) == "" {
		cfg.CronExpression = defaultArchivalCronExpr
	}

	return c.JSON(http.StatusOK, DiffArchivalSettingsResponse{
		DiffArchivalSettingsConfig: cfg,
		ScheduleHuman:              describeCronSchedule(cfg.CronExpression),
	})
}

// UpdateDiffArchivalSettings saves diff archival configuration to system_settings
// and hot-reloads the running DiffArchivalManager without a server restart.
func (s *Server) UpdateDiffArchivalSettings(c echo.Context) error {
	var req DiffArchivalSettingsConfig
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if req.RetentionDays <= 0 {
		req.RetentionDays = defaultArchivalRetentionDays
	}
	if strings.TrimSpace(req.CronExpression) == "" {
		req.CronExpression = defaultArchivalCronExpr
	}

	// Persist only the user-facing fields; internal tuning params (batch_size, delay_ms) are left untouched.
	data, err := json.Marshal(req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to serialize settings"})
	}

	_, err = s.db.ExecContext(c.Request().Context(), `
		INSERT INTO system_settings (name, data)
		VALUES ('diff_archival_settings', $1)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save diff archival settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save diff archival settings"})
	}

	// Hot-reload the running manager so the new schedule/retention takes effect immediately.
	if s.diffArchivalManager != nil {
		s.diffArchivalManager.UpdateConfig(
			req.Enabled,
			req.CronExpression,
			req.RetentionDays,
			defaultArchivalBatchSize,
			defaultArchivalInterBatchDelayMs,
		)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Diff archival settings updated successfully"})
}

// RunDiffArchivalNow triggers an immediate diff archival cycle in the background.
func (s *Server) RunDiffArchivalNow(c echo.Context) error {
	if s.diffArchivalManager == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Diff archival manager is not initialized"})
	}

	go s.diffArchivalManager.TriggerManualCycle()

	return c.JSON(http.StatusOK, map[string]string{"message": "Diff archival started in the background"})
}
