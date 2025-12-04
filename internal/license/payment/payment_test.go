package payment

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestGetRazoarPayKeys(t *testing.T) {
	accessKey, secretKey, err := GetRazorpayKeys("test")
	if err != nil {
		t.Errorf("Error getting Razorpay keys: %v", err)
	}

	fmt.Printf("%s %s\n", accessKey, secretKey)

	if accessKey == "" || secretKey == "" {
		t.Error("Razorpay keys should not be empty")
	}
}

func TestCreateMonthlyPlan(t *testing.T) {
	plan, err := CreatePlan("test", "monthly")
	if err != nil {
		t.Errorf("Error creating monthly plan: %v", err)
		return
	}

	fmt.Println("\n=== Monthly Plan Created ===")
	printPlan(plan)

	if plan.ID == "" {
		t.Error("Plan ID should not be empty")
	}
	if plan.Period != "monthly" {
		t.Errorf("Expected period to be 'monthly', got '%s'", plan.Period)
	}
	if plan.Item.Amount != 600 {
		t.Errorf("Expected amount to be 600 cents ($6), got %d", plan.Item.Amount)
	}
}

func TestCreateYearlyPlan(t *testing.T) {
	plan, err := CreatePlan("test", "yearly")
	if err != nil {
		t.Errorf("Error creating yearly plan: %v", err)
		return
	}

	fmt.Println("\n=== Yearly Plan Created ===")
	printPlan(plan)

	if plan.ID == "" {
		t.Error("Plan ID should not be empty")
	}
	if plan.Period != "yearly" {
		t.Errorf("Expected period to be 'yearly', got '%s'", plan.Period)
	}
	if plan.Item.Amount != 6000 {
		t.Errorf("Expected amount to be 6000 cents ($60), got %d", plan.Item.Amount)
	}
}

func TestGetAllPlans(t *testing.T) {
	planList, err := GetAllPlans("test")
	if err != nil {
		t.Errorf("Error fetching all plans: %v", err)
		return
	}

	fmt.Printf("\n=== All Plans (Total: %d) ===\n", planList.Count)
	for i, plan := range planList.Items {
		fmt.Printf("\n--- Plan %d ---\n", i+1)
		printPlan(&plan)
	}

	if planList.Count < 0 {
		t.Error("Plan count should not be negative")
	}
}

func TestGetPlanByID(t *testing.T) {
	// First, get all plans to get a valid plan ID
	planList, err := GetAllPlans("test")
	if err != nil {
		t.Errorf("Error fetching all plans: %v", err)
		return
	}

	if planList.Count == 0 {
		t.Skip("No plans available to test GetPlanByID")
		return
	}

	// Get the first plan's ID
	planID := planList.Items[0].ID
	fmt.Printf("\n=== Fetching Plan by ID: %s ===\n", planID)

	plan, err := GetPlanByID("test", planID)
	if err != nil {
		t.Errorf("Error fetching plan by ID: %v", err)
		return
	}

	printPlan(plan)

	if plan.ID != planID {
		t.Errorf("Expected plan ID '%s', got '%s'", planID, plan.ID)
	}
}

// Helper function to print plan details in a readable format
func printPlan(plan *RazorpayPlan) {
	planJSON, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting plan: %v\n", err)
		return
	}

	fmt.Println(string(planJSON))

	// Also print in a more human-readable format
	fmt.Printf("\nPlan Summary:\n")
	fmt.Printf("  ID: %s\n", plan.ID)
	fmt.Printf("  Name: %s\n", plan.Item.Name)
	fmt.Printf("  Period: %s (every %d period)\n", plan.Period, plan.Interval)
	fmt.Printf("  Amount: $%.2f %s\n", float64(plan.Item.Amount)/100, plan.Item.Currency)
	fmt.Printf("  Description: %s\n", plan.Description)

	// Print notes from API response
	notesMap := plan.GetNotesMap()
	if len(notesMap) > 0 {
		fmt.Printf("  Notes:\n")
		for key, value := range notesMap {
			fmt.Printf("    %s: %s\n", key, value)
		}
	}

	// Print internal NotesMap if set
	if len(plan.NotesMap) > 0 {
		fmt.Printf("  NotesMap (internal):\n")
		for key, value := range plan.NotesMap {
			fmt.Printf("    %s: %s\n", key, value)
		}
	}
	fmt.Println()
}
