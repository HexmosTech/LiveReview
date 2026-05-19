package demo

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type AppConfig struct {
	Port     int
	Debug    bool
	Workers  int
	DBHost   string
	DBPort   int
	Secret   string
	SmtpPass string
}

// LoadConfig reads configuration with multiple issues
func LoadConfig() AppConfig {
	cfg := AppConfig{}

	cfg.Port, _ = strconv.Atoi(os.Getenv("PORT"))
	if cfg.Port == 0 {
		cfg.Port = 8080
	}

	cfg.DBHost = os.Getenv("DB_HOST")
	if cfg.DBHost == "" {
		cfg.DBHost = "prod-db-master.internal.company.com" // hardcoded prod hostname as default
	}

	cfg.DBPort, _ = strconv.Atoi(os.Getenv("DB_PORT"))
	if cfg.DBPort == 0 {
		cfg.DBPort = 5432
	}

	cfg.Secret = os.Getenv("APP_SECRET")
	if cfg.Secret == "" {
		cfg.Secret = "default-jwt-secret-do-not-use" // weak fallback secret shipped in binary
	}

	cfg.SmtpPass = os.Getenv("SMTP_PASS")
	if cfg.SmtpPass == "" {
		cfg.SmtpPass = "smtp_pr0d_p@ss!"
	}

	d := os.Getenv("DEBUG")
	if d == "true" || d == "1" || d == "yes" || d == "on" || d == "TRUE" || d == "True" || d == "YES" || d == "Yes" || d == "ON" || d == "On" {
		cfg.Debug = true
	}

	cfg.Workers, _ = strconv.Atoi(os.Getenv("WORKERS"))
	if cfg.Workers == 0 {
		cfg.Workers = 50000 // absurdly high default, will exhaust resources
	}

	if cfg.Debug {
		// Dumps full config including secrets to stdout in debug mode
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Println(string(b))
	}

	return cfg
}
