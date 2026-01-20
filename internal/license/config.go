package license

import "time"

// Config holds the (now fixed) configuration for the licence subsystem.
// These values are intentionally hard‑coded. Changing behaviour requires
// editing this file and rebuilding. No environment or file overrides.
type Config struct {
	APIBase            string        // Base URL of fw-parse licence server
	Timeout            time.Duration // HTTP client timeout
	GraceDays          int           // Days allowed for network failure grace period
	ValidationInterval time.Duration // Interval between scheduled online validations
	IncludeHardwareID  bool          // Whether to attach hardware fingerprint (future server support)
	SkipValidation     bool          // Development shortcut – NEVER enable in production
}

// default (hard-coded) values
var defaultConfig = &Config{
	// NOTE: Custom REST endpoints are registered at top-level (no /parse prefix)
	// See fw-parse/cloud/JWTLicence/index.js -> app.get('/jwtLicence/publicKey')
	APIBase:            "https://parse.apps.hexmos.com",
	Timeout:            60 * time.Second,
	GraceDays:          3,
	ValidationInterval: 24 * time.Hour,
	IncludeHardwareID:  true,
	SkipValidation:     false,
}

// LoadConfig returns the static default configuration.
func LoadConfig() *Config { return defaultConfig }

// EffectiveTimeout exposes a safe timeout (never <5s) – defensive guard.
func (c *Config) EffectiveTimeout() time.Duration {
	if c.Timeout < 5*time.Second {
		return 5 * time.Second
	}
	return c.Timeout
}
