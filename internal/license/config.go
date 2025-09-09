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
	EnforcementMode    string        // off|soft|strict
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
	EnforcementMode:    "soft", // soft = warn but don't block
}

// LoadConfig returns the static default configuration.
func LoadConfig() *Config { return defaultConfig }

// IsStrict returns true if enforcement mode blocks usage on invalid / expired.
func (c *Config) IsStrict() bool { return c.EnforcementMode == "strict" }

// IsSoft returns true if enforcement mode warns but allows usage.
func (c *Config) IsSoft() bool { return c.EnforcementMode == "soft" }

// EffectiveTimeout exposes a safe timeout (never <5s) – defensive guard.
func (c *Config) EffectiveTimeout() time.Duration {
	if c.Timeout < 5*time.Second {
		return 5 * time.Second
	}
	return c.Timeout
}
