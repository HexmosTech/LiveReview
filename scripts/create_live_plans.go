package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/livereview/internal/license/payment"
)

func main() {
	fmt.Println("=== Fetching Test Plans ===")

	// Fetch monthly plan from test
	monthlyTest, err := payment.GetPlanByID("test", "plan_Rt80PrSij8WMsd")
	if err != nil {
		log.Fatalf("Failed to fetch monthly test plan: %v", err)
	}

	fmt.Printf("\nTest Monthly Plan:\n")
	monthlyJSON, _ := json.MarshalIndent(monthlyTest, "", "  ")
	fmt.Println(string(monthlyJSON))

	// Fetch yearly plan from test
	yearlyTest, err := payment.GetPlanByID("test", "plan_Rt80QMx4ZD5wmZ")
	if err != nil {
		log.Fatalf("Failed to fetch yearly test plan: %v", err)
	}

	fmt.Printf("\nTest Yearly Plan:\n")
	yearlyJSON, _ := json.MarshalIndent(yearlyTest, "", "  ")
	fmt.Println(string(yearlyJSON))

	fmt.Println("\n=== Creating Live Plans ===")

	// Create monthly plan in live
	fmt.Println("\nCreating monthly plan in live mode...")
	monthlyLive, err := payment.CreatePlan("live", "monthly")
	if err != nil {
		log.Fatalf("Failed to create monthly live plan: %v", err)
	}

	fmt.Printf("✓ Created Monthly Plan: %s\n", monthlyLive.ID)
	monthlyLiveJSON, _ := json.MarshalIndent(monthlyLive, "", "  ")
	fmt.Println(string(monthlyLiveJSON))

	// Create yearly plan in live
	fmt.Println("\nCreating yearly plan in live mode...")
	yearlyLive, err := payment.CreatePlan("live", "yearly")
	if err != nil {
		log.Fatalf("Failed to create yearly live plan: %v", err)
	}

	fmt.Printf("✓ Created Yearly Plan: %s\n", yearlyLive.ID)
	yearlyLiveJSON, _ := json.MarshalIndent(yearlyLive, "", "  ")
	fmt.Println(string(yearlyLiveJSON))

	fmt.Println("\n=== Update subscription_service.go with these IDs: ===")
	fmt.Printf("TeamMonthlyPlanID = \"%s\"\n", monthlyLive.ID)
	fmt.Printf("TeamYearlyPlanID  = \"%s\"\n", yearlyLive.ID)
}
