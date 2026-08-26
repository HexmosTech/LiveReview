package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// CompactionSettingsConfig holds configuration for event log compaction.
type CompactionSettingsConfig struct {
	Enabled        bool   `json:"enabled"`
	CronExpression string `json:"cron_expression"`
	RetentionDays  int    `json:"retention_days"`
}

// CompactionSettingsResponse includes config plus schedule description.
type CompactionSettingsResponse struct {
	CompactionSettingsConfig
	ScheduleHuman string `json:"schedule_human"`
}

func defaultCompactionSettingsConfig() CompactionSettingsConfig {
	return CompactionSettingsConfig{
		Enabled:        true,
		CronExpression: "0 2 * * *", // daily at 2 AM server time
		RetentionDays:  30,
	}
}

// GetCompactionSettings fetches ONLY the config (instant, single system_settings query).
func (s *Server) GetCompactionSettings(c echo.Context) error {
	ctx := c.Request().Context()
	cfg := defaultCompactionSettingsConfig()

	var data []byte
	err := s.db.QueryRowContext(ctx, "SELECT data FROM system_settings WHERE name = 'event_compaction_settings'").Scan(&data)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &cfg)
	}

	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}
	if strings.TrimSpace(cfg.CronExpression) == "" {
		cfg.CronExpression = "0 2 * * *"
	}

	return c.JSON(http.StatusOK, CompactionSettingsResponse{
		CompactionSettingsConfig: cfg,
		ScheduleHuman:            describeCronSchedule(cfg.CronExpression),
	})
}

// UpdateCompactionSettings updates event log compaction configuration in system_settings.
func (s *Server) UpdateCompactionSettings(c echo.Context) error {
	var cfg CompactionSettingsConfig
	if err := c.Bind(&cfg); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = 30
	}
	if strings.TrimSpace(cfg.CronExpression) == "" {
		cfg.CronExpression = "0 2 * * *"
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to serialize settings"})
	}

	_, err = s.db.ExecContext(c.Request().Context(), `
		INSERT INTO system_settings (name, data)
		VALUES ('event_compaction_settings', $1)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save event compaction settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save compaction settings"})
	}

	if s.eventCompactionManager != nil {
		s.eventCompactionManager.UpdateConfig(cfg.Enabled, cfg.CronExpression, cfg.RetentionDays)
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Event compaction settings updated successfully"})
}

// RunCompactionNow triggers an immediate compaction pass.
func (s *Server) RunCompactionNow(c echo.Context) error {
	if s.eventCompactionManager == nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Event compaction manager is not initialized"})
	}

	go s.eventCompactionManager.TriggerManualCycle()

	return c.JSON(http.StatusOK, map[string]string{"message": "Log compaction started in the background"})
}

// describeCronSchedule converts a cron expression to a plain English description.
func describeCronSchedule(expr string) string {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return expr
	}

	minutePart := parts[0]
	hourPart := parts[1]
	dom := parts[2]
	month := parts[3]
	dow := parts[4]

	// Simple daily: M H * * *
	if dom == "*" && month == "*" && dow == "*" && !strings.Contains(hourPart, ",") && !strings.Contains(hourPart, "/") {
		hour, errH := strconv.Atoi(hourPart)
		minute, errM := strconv.Atoi(minutePart)
		if errH == nil && errM == nil {
			return fmt.Sprintf("Daily at %02d:%02d UTC", hour, minute)
		}
	}

	// Every N hours: 0 */N * * *
	if dom == "*" && month == "*" && dow == "*" && strings.HasPrefix(hourPart, "*/") {
		n := strings.TrimPrefix(hourPart, "*/")
		return fmt.Sprintf("Every %s hours", n)
	}

	// Multiple times a day: 0 H1,H2 * * *
	if dom == "*" && month == "*" && dow == "*" && strings.Contains(hourPart, ",") {
		return fmt.Sprintf("Multiple times daily (%s:00 UTC)", strings.ReplaceAll(hourPart, ",", ":00, "))
	}

	return expr
}
