package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/livereview/storage"
)

// ConfigCheckResult holds the result of configuration validation
type ConfigCheckResult struct {
	Missing  []string          // Required variables that are missing
	Present  map[string]string // Variables that are set (masked values)
	Warnings []string          // Non-fatal warnings
	IsCloud  bool              // Whether cloud mode is detected
}

// CheckRequiredConfig validates that required environment variables are set
func CheckRequiredConfig(isCloud bool) *ConfigCheckResult {
	result := &ConfigCheckResult{
		Missing:  []string{},
		Present:  make(map[string]string),
		Warnings: []string{},
		IsCloud:  isCloud,
	}

	// Always required
	requiredVars := []string{
		"DATABASE_URL",
		"JWT_SECRET",
	}

	// Cloud mode requires additional secrets
	if isCloud {
		requiredVars = append(requiredVars, "CLOUD_JWT_SECRET")
		requiredVars = append(requiredVars, "RAZORPAY_MODE", "RAZORPAY_WEBHOOK_SECRET")

		mode := strings.ToLower(strings.TrimSpace(os.Getenv("RAZORPAY_MODE")))
		switch mode {
		case "test":
			requiredVars = append(requiredVars,
				"RAZORPAY_TEST_KEY",
				"RAZORPAY_TEST_SECRET",
				"RAZORPAY_TEST_MONTHLY_PLAN_ID",
				"RAZORPAY_TEST_YEARLY_PLAN_ID",
			)
		case "live":
			requiredVars = append(requiredVars,
				"LIVEREVIEW_PRICING_PROFILE",
			)

			pricingProfile := strings.ToLower(strings.TrimSpace(os.Getenv("LIVEREVIEW_PRICING_PROFILE")))
			requiredVars = append(requiredVars,
				"RAZORPAY_LIVE_KEY",
				"RAZORPAY_LIVE_SECRET",
			)

			switch pricingProfile {
			case "actual":
				requiredVars = append(requiredVars,
					"RAZORPAY_LIVE_ACTUAL_MONTHLY_PLAN_ID",
					"RAZORPAY_LIVE_ACTUAL_YEARLY_PLAN_ID",
				)
			case "low_pricing_test":
				requiredVars = append(requiredVars,
					"RAZORPAY_LIVE_LOW_PRICING_MONTHLY_PLAN_ID",
					"RAZORPAY_LIVE_LOW_PRICING_YEARLY_PLAN_ID",
				)
			default:
				result.Warnings = append(result.Warnings, "LIVEREVIEW_PRICING_PROFILE should be set to actual or low_pricing_test when RAZORPAY_MODE=live")
			}
		default:
			result.Warnings = append(result.Warnings, "RAZORPAY_MODE should be set to test or live")
		}
	}

	for _, v := range requiredVars {
		val := os.Getenv(v)
		if val == "" {
			result.Missing = append(result.Missing, v)
		} else {
			result.Present[v] = maskSecret(val)
		}
	}

	// Optional but good to check
	optionalVars := []string{
		"RAZORPAY_TEST_KEY",
		"RAZORPAY_LIVE_KEY",
	}

	for _, v := range optionalVars {
		val := os.Getenv(v)
		if val != "" {
			result.Present[v] = maskSecret(val)
		}
	}

	return result
}

// PrintConfigCheck prints the configuration check results
func PrintConfigCheck(result *ConfigCheckResult) {
	fmt.Println("=== Configuration Check ===")

	if result.IsCloud {
		fmt.Println("Mode: Cloud")
	} else {
		fmt.Println("Mode: Self-hosted")
	}

	fmt.Println("")

	if len(result.Missing) > 0 {
		fmt.Println("❌ Missing required variables:")
		for _, v := range result.Missing {
			fmt.Printf("   - %s\n", v)
		}
		fmt.Println("")
	}

	if len(result.Present) > 0 {
		fmt.Println("✓ Configured variables:")
		for k, v := range result.Present {
			fmt.Printf("   - %s = %s\n", k, v)
		}
		fmt.Println("")
	}

	for _, w := range result.Warnings {
		fmt.Printf("⚠ Warning: %s\n", w)
	}

	if len(result.Missing) == 0 {
		fmt.Println("✓ All required configuration is present")
	}

	fmt.Println("============================")
}

// maskSecret masks a secret value for display, showing only first and last 2 chars
func maskSecret(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}

// IsCloudModeEnabled checks if cloud mode is enabled via environment
func IsCloudModeEnabled() bool {
	mode := os.Getenv("LIVEREVIEW_MODE")
	return mode == "cloud"
}

// LoadEnvFile loads environment variables from a file, overwriting existing ones.
func LoadEnvFile(filename string) error {
	file, err := storage.CmdEnvOpen(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		// Overwrite environment variable
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set env var %s: %w", key, err)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
