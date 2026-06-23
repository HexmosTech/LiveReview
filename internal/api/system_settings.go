package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/livereview/network/email"
	"github.com/rs/zerolog/log"
)

type SMTPSettings struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Sender     string `json:"sender"`
	SenderName string `json:"sender_name"`
	SkipTLS    bool   `json:"skip_tls"`
}

// GetSMTPSettings fetches the global SMTP configuration from system_settings
func (s *Server) GetSMTPSettings(c echo.Context) error {
	var data []byte
	err := s.db.QueryRow("SELECT data FROM system_settings WHERE name = 'smtp'").Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			// Return empty settings if not configured
			return c.JSON(http.StatusOK, SMTPSettings{})
		}
		log.Error().Err(err).Msg("Failed to fetch SMTP settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to fetch SMTP settings"})
	}

	var settings SMTPSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Error().Err(err).Msg("Failed to parse SMTP settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to parse SMTP settings"})
	}

	return c.JSON(http.StatusOK, settings)
}

// UpdateSMTPSettings saves the global SMTP configuration to system_settings
func (s *Server) UpdateSMTPSettings(c echo.Context) error {
	var settings SMTPSettings
	if err := c.Bind(&settings); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}

	data, err := json.Marshal(settings)
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal SMTP settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to process settings"})
	}

	_, err = s.db.Exec(`
		INSERT INTO system_settings (name, data) 
		VALUES ('smtp', $1)
		ON CONFLICT (name) DO UPDATE SET data = EXCLUDED.data, updated_at = CURRENT_TIMESTAMP
	`, data)
	
	if err != nil {
		log.Error().Err(err).Msg("Failed to save SMTP settings")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save SMTP settings"})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "SMTP settings updated successfully"})
}

// TestSMTPSettings attempts to send a test email using the provided credentials
func (s *Server) TestSMTPSettings(c echo.Context) error {
	var settings SMTPSettings
	if err := c.Bind(&settings); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid request payload"})
	}

	// Make sure we have an email to send to
	userEmail := c.Get("email").(string)
	if userEmail == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Could not determine admin email for test"})
	}

	// Create a test message
	err := email.SendTestEmailSMTP(
		settings.Host,
		settings.Port,
		settings.Username,
		settings.Password,
		settings.Sender,
		settings.SenderName,
		settings.SkipTLS,
		userEmail,
	)

	if err != nil {
		log.Error().Err(err).Msg("SMTP Test failed")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "SMTP Connection Failed: " + err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]string{"message": "Test email sent successfully to " + userEmail})
}
