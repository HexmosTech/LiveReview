package main

import (
	"fmt"
	"log"

	"github.com/livereview/internal/license/payment"
)

func main() {
	fmt.Println("Creating Razorpay TEST plans...")

	// Create monthly test plan
	monthlyPlan, err := payment.CreatePlan("test", "monthly")
	if err != nil {
		log.Fatalf("Failed to create monthly test plan: %v", err)
	}
	fmt.Printf("✓ Created monthly test plan: %s\n", monthlyPlan.ID)

	// Create yearly test plan
	yearlyPlan, err := payment.CreatePlan("test", "yearly")
	if err != nil {
		log.Fatalf("Failed to create yearly test plan: %v", err)
	}
	fmt.Printf("✓ Created yearly test plan: %s\n", yearlyPlan.ID)

	fmt.Println("\nUpdate these in subscription_service.go:")
	fmt.Printf("  TeamMonthlyPlanIDTest = \"%s\"\n", monthlyPlan.ID)
	fmt.Printf("  TeamYearlyPlanIDTest  = \"%s\"\n", yearlyPlan.ID)
}
