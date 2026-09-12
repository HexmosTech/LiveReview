package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// PreloadedChangesArchivalSettingsConfig holds user-facing configuration for the preloaded_changes archival cron job.
type PreloadedChangesArchivalSettingsConfig struct {
	Enabled        bool   `json:"enabled"`
	CronExpression string `json:"cron_expression"`
	RetentionDays  int    `json:"retention_days"`
}

// PreloadedChangesArchivalSettingsResponse includes config plus a human-readable schedule description.
type PreloadedChangesArchivalSettingsResponse struct {
	PreloadedChangesArchivalSettingsConfig
	ScheduleHuman string `json:"schedule_human"`
}

func defaultPreloadedChangesArchivalSettingsConfig() PreloadedChangesArchivalSettingsConfig {
	return PreloadedChangesArchivalSettingsConfig{
		Enabled:        true,
		CronExpression: defaultArchivalCronExpr,
		RetentionDays:  defaultArchivalRetentionDays,
	}
}

// GetPreloadedChangesArchivalSettings returns the current preloaded_changes archival configuration from system_settings.
func (server *Server) GetPreloadedChangesArchivalSettings(echoContext echo.Context) error {
	requestContext := echoContext.Request().Context()
	config := defaultPreloadedChangesArchivalSettingsConfig()

	var data []byte
	err := server.db.QueryRowContext(requestContext, "SELECT data FROM system_settings WHERE name = 'preloaded_changes_archival_settings'").Scan(&data)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &config)
	}

	if config.RetentionDays <= 0 {
		config.RetentionDays = defaultArchivalRetentionDays
	}
	if strings.TrimSpace(config.CronExpression) == "" {
		config.CronExpression = defaultArchivalCronExpr
	}

	return echoContext.JSON(http.StatusOK, PreloadedChangesArchivalSettingsResponse{
		PreloadedChangesArchivalSettingsConfig: config,
		ScheduleHuman:                          describeCronSchedule(config.CronExpression),
	})
}

// UpdatePreloadedChangesArchivalSettings saves preloaded_changes archival configuration to system_settings
// and hot-reloads the running PreloadedChangesArchivalManager without a server restart.
func (server *Server) UpdatePreloadedChangesArchivalSettings(echoContext echo.Context) error {
	var request PreloadedChangesArchivalSettingsConfig
	if err := echoContext.Bind(&request); err != nil {
		return echoContext.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if request.RetentionDays <= 0 {
		request.RetentionDays = defaultArchivalRetentionDays
	}
	if strings.TrimSpace(request.CronExpression) == "" {
		request.CronExpression = defaultArchivalCronExpr
	}

	// Persist only the user-facing fields; internal tuning params (batch_size, delay_ms) are left untouched.
	data, err := json.Marshal(request)
	if err != nil {
		return echoContext.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to serialize settings"})
	}

	settingName := "preloaded_changes_archival_settings"
	_, err = server.db.ExecContext(echoContext.Request().Context(), `
		INSERT INTO system_settings (name, data)
		VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, settingName, data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save preloaded_changes archival settings")
		return echoContext.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save preloaded_changes archival settings"})
	}

	// Hot-reload the running manager so the new schedule/retention takes effect immediately.
	if server.preloadedChangesArchivalManager != nil {
		server.preloadedChangesArchivalManager.UpdateConfig(
			request.Enabled,
			request.CronExpression,
			request.RetentionDays,
			defaultArchivalBatchSize,
			defaultArchivalInterBatchDelayMs,
		)
	}

	return echoContext.JSON(http.StatusOK, map[string]string{"message": "Preloaded changes archival settings updated successfully"})
}

// RunPreloadedChangesArchivalNow triggers an immediate preloaded_changes archival cycle in the background.
func (server *Server) RunPreloadedChangesArchivalNow(echoContext echo.Context) error {
	if server.preloadedChangesArchivalManager == nil {
		return echoContext.JSON(http.StatusBadRequest, map[string]string{"error": "Preloaded changes archival manager is not initialized"})
	}

	go server.preloadedChangesArchivalManager.TriggerManualCycle()

	return echoContext.JSON(http.StatusOK, map[string]string{"message": "Preloaded changes archival started in the background"})
}
