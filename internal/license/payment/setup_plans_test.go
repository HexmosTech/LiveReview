package license

import (
	"fmt"
	"testing"
)

// TestSetupPlans creates Razorpay Team plans and displays their IDs
// Run with: go test -v ./internal/license/payment -run TestSetupPlans
// Use -args test or -args live to specify mode (default: test)
func TestSetupPlans(t *testing.T) {
	// Get mode from test args (defaults to "test")
	mode := "test"
	// Note: In real usage, you'd pass mode via environment variable:
	// MODE=live go test -v ./internal/license/payment -run TestSetupPlans

	fmt.Printf("\n=== Setting up Razorpay Team plans in %s mode ===\n\n", mode)

	// Create monthly plan
	t.Log("Creating Team Monthly plan ($6/month)...")
	monthlyPlan, err := CreatePlan(mode, "monthly")
	if err != nil {
		t.Fatalf("Failed to create monthly plan: %v", err)
	}
	t.Logf("✓ Team Monthly Plan Created")
	t.Logf("  ID: %s", monthlyPlan.ID)
	t.Logf("  Amount: $%.2f/month", float64(monthlyPlan.Item.Amount)/100)
	t.Logf("  Period: %s (interval: %d)", monthlyPlan.Period, monthlyPlan.Interval)

	// Create yearly plan
	t.Log("\nCreating Team Yearly plan ($60/year)...")
	yearlyPlan, err := CreatePlan(mode, "yearly")
	if err != nil {
		t.Fatalf("Failed to create yearly plan: %v", err)
	}
	t.Logf("✓ Team Yearly Plan Created")
	t.Logf("  ID: %s", yearlyPlan.ID)
	t.Logf("  Amount: $%.2f/year", float64(yearlyPlan.Item.Amount)/100)
	t.Logf("  Period: %s (interval: %d)", yearlyPlan.Period, yearlyPlan.Interval)

	// Print instructions
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("NEXT STEPS:")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("\nAdd these plan IDs to internal/license/payment/subscription_service.go:")
	fmt.Println()
	fmt.Printf("var (\n")
	fmt.Printf("    TeamMonthlyPlanID = \"%s\"\n", monthlyPlan.ID)
	fmt.Printf("    TeamYearlyPlanID  = \"%s\"\n", yearlyPlan.ID)
	fmt.Printf(")\n")
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("\nPlans created in %s mode. Save these IDs before proceeding!\n", mode)
}
