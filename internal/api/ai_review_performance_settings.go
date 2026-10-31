package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/internal/reviewperf"
	"github.com/rs/zerolog/log"
)

// GetAIReviewPerformanceSettings fetches the instance-wide review
// concurrency/timeout ceiling from system_settings. Absent config (no row
// yet) returns reviewperf.DefaultSettings(), matching reviewperf.Load's own
// zero-config behavior - mirrors GetStorageSettings in storage_settings.go.
func (s *Server) GetAIReviewPerformanceSettings(c echo.Context) error {
	settings, err := reviewperf.Load(c.Request().Context(), s.db)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch AI review performance settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch AI review performance settings"})
	}
	return c.JSON(http.StatusOK, settings)
}

// UpdateAIReviewPerformanceSettings saves the instance-wide ceiling.
// internal/aiselection.GetReviewAISelection reads this fresh on every
// review (no caching), so a change here takes effect for the very next
// review - no restart required.
func (s *Server) UpdateAIReviewPerformanceSettings(c echo.Context) error {
	var req reviewperf.Settings
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}
	if err := req.Validate(); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := reviewperf.Save(c.Request().Context(), s.db, req); err != nil {
		log.Error().Err(err).Msg("Failed to save AI review performance settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save AI review performance settings"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "AI review performance settings updated successfully"})
}
