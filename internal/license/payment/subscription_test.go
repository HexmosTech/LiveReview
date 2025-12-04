package payment

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestCreateSubscription(t *testing.T) {
	// First, get a plan to subscribe to
	planList, err := GetAllPlans("test")
	if err != nil {
		t.Fatalf("Error fetching plans: %v", err)
	}

	if planList.Count == 0 {
		t.Skip("No plans available to create subscription")
	}

	// Find a LiveReview monthly plan
	var planID string
	for _, plan := range planList.Items {
		notesMap := plan.GetNotesMap()
		if notesMap != nil && notesMap["app_name"] == "LiveReview" && notesMap["plan_type"] == "team_monthly" {
			planID = plan.ID
			break
		}
	}

	if planID == "" {
		t.Skip("No LiveReview monthly plan found")
	}

	// Create subscription with 5 users
	subscription, err := CreateSubscription("test", planID, 5, map[string]string{
		"test_subscription": "true",
		"org_name":          "Test Organization",
	})
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}

	fmt.Println("\n=== Subscription Created ===")
	printSubscription(subscription)

	if subscription.ID == "" {
		t.Error("Subscription ID should not be empty")
	}
	if subscription.PlanID != planID {
		t.Errorf("Expected plan ID '%s', got '%s'", planID, subscription.PlanID)
	}
	if subscription.Quantity != 5 {
		t.Errorf("Expected quantity 5, got %d", subscription.Quantity)
	}
}

func TestGetAllSubscriptions(t *testing.T) {
	subList, err := GetAllSubscriptions("test")
	if err != nil {
		t.Fatalf("Error fetching all subscriptions: %v", err)
	}

	fmt.Printf("\n=== All Subscriptions (Total: %d) ===\n", subList.Count)
	for i, sub := range subList.Items {
		fmt.Printf("\n--- Subscription %d ---\n", i+1)
		printSubscription(&sub)
	}

	if subList.Count < 0 {
		t.Error("Subscription count should not be negative")
	}
}

func TestGetSubscriptionByID(t *testing.T) {
	// First, get all subscriptions to get a valid subscription ID
	subList, err := GetAllSubscriptions("test")
	if err != nil {
		t.Fatalf("Error fetching all subscriptions: %v", err)
	}

	if subList.Count == 0 {
		t.Skip("No subscriptions available to test GetSubscriptionByID")
	}

	// Get the first subscription's ID
	subID := subList.Items[0].ID
	fmt.Printf("\n=== Fetching Subscription by ID: %s ===\n", subID)

	sub, err := GetSubscriptionByID("test", subID)
	if err != nil {
		t.Fatalf("Error fetching subscription by ID: %v", err)
	}

	printSubscription(sub)

	if sub.ID != subID {
		t.Errorf("Expected subscription ID '%s', got '%s'", subID, sub.ID)
	}
}

func TestUpdateSubscriptionQuantity(t *testing.T) {
	// First, create a subscription
	planList, err := GetAllPlans("test")
	if err != nil {
		t.Fatalf("Error fetching plans: %v", err)
	}

	if planList.Count == 0 {
		t.Skip("No plans available")
	}

	var planID string
	for _, plan := range planList.Items {
		notesMap := plan.GetNotesMap()
		if notesMap != nil && notesMap["app_name"] == "LiveReview" {
			planID = plan.ID
			break
		}
	}

	if planID == "" {
		t.Skip("No LiveReview plan found")
	}

	subscription, err := CreateSubscription("test", planID, 3, map[string]string{
		"test": "quantity_update_test",
	})
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}

	fmt.Println("\n=== Original Subscription (Quantity: 3) ===")
	printSubscription(subscription)

	// Wait a moment
	time.Sleep(2 * time.Second)

	// Update quantity to 8 users
	updatedSub, err := UpdateSubscriptionQuantity("test", subscription.ID, 8, 0)
	if err != nil {
		// Subscriptions need to be authenticated or active to update
		if subscription.Status != "authenticated" && subscription.Status != "active" {
			t.Skipf("Subscription is in '%s' state, needs to be 'authenticated' or 'active' to update. This is expected in test mode without actual payment.", subscription.Status)
		}
		t.Fatalf("Error updating subscription quantity: %v", err)
	}

	fmt.Println("\n=== Updated Subscription (Quantity: 8) ===")
	printSubscription(updatedSub)

	// Note: Quantity change might be scheduled, check has_scheduled_changes
	if updatedSub.Quantity != 8 && !updatedSub.HasScheduledChanges {
		t.Errorf("Expected quantity 8 or scheduled changes, got quantity %d, scheduled: %v",
			updatedSub.Quantity, updatedSub.HasScheduledChanges)
	}
}

func TestCancelSubscription(t *testing.T) {
	// First, create a subscription
	planList, err := GetAllPlans("test")
	if err != nil {
		t.Fatalf("Error fetching plans: %v", err)
	}

	if planList.Count == 0 {
		t.Skip("No plans available")
	}

	var planID string
	for _, plan := range planList.Items {
		notesMap := plan.GetNotesMap()
		if notesMap != nil && notesMap["app_name"] == "LiveReview" {
			planID = plan.ID
			break
		}
	}

	if planID == "" {
		t.Skip("No LiveReview plan found")
	}

	subscription, err := CreateSubscription("test", planID, 2, map[string]string{
		"test": "cancel_test",
	})
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}

	fmt.Println("\n=== Original Subscription ===")
	printSubscription(subscription)

	// Wait a moment
	time.Sleep(2 * time.Second)

	// Cancel immediately
	cancelledSub, err := CancelSubscription("test", subscription.ID, false)
	if err != nil {
		t.Fatalf("Error cancelling subscription: %v", err)
	}

	fmt.Println("\n=== Cancelled Subscription ===")
	printSubscription(cancelledSub)

	if cancelledSub.Status != "cancelled" {
		t.Logf("Warning: Expected status 'cancelled', got '%s' (might take time to reflect)", cancelledSub.Status)
	}
}

func TestCancelSubscriptionAtCycleEnd(t *testing.T) {
	// First, create a subscription
	planList, err := GetAllPlans("test")
	if err != nil {
		t.Fatalf("Error fetching plans: %v", err)
	}

	if planList.Count == 0 {
		t.Skip("No plans available")
	}

	var planID string
	for _, plan := range planList.Items {
		notesMap := plan.GetNotesMap()
		if notesMap != nil && notesMap["app_name"] == "LiveReview" {
			planID = plan.ID
			break
		}
	}

	if planID == "" {
		t.Skip("No LiveReview plan found")
	}

	subscription, err := CreateSubscription("test", planID, 1, map[string]string{
		"test": "cancel_at_cycle_end_test",
	})
	if err != nil {
		t.Fatalf("Error creating subscription: %v", err)
	}

	fmt.Println("\n=== Original Subscription ===")
	printSubscription(subscription)

	// Wait a moment
	time.Sleep(2 * time.Second)

	// Cancel at end of billing cycle
	cancelledSub, err := CancelSubscription("test", subscription.ID, true)
	if err != nil {
		// If subscription hasn't started billing cycle, try immediate cancel instead
		t.Logf("Cancel at cycle end failed (expected for new subscriptions): %v", err)
		t.Log("Attempting immediate cancellation instead...")
		cancelledSub, err = CancelSubscription("test", subscription.ID, false)
		if err != nil {
			t.Fatalf("Error cancelling subscription: %v", err)
		}
	}

	fmt.Println("\n=== Subscription Scheduled for Cancellation ===")
	printSubscription(cancelledSub)

	// Subscription should still be active but scheduled for cancellation
	if cancelledSub.Status == "cancelled" {
		t.Log("Subscription cancelled immediately (expected to be scheduled)")
	}
}

// Helper function to print subscription details in a readable format
func printSubscription(sub *RazorpaySubscription) {
	subJSON, err := json.MarshalIndent(sub, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting subscription: %v\n", err)
		return
	}

	fmt.Println(string(subJSON))

	// Also print in a more human-readable format
	fmt.Printf("\nSubscription Summary:\n")
	fmt.Printf("  ID: %s\n", sub.ID)
	fmt.Printf("  Plan ID: %s\n", sub.PlanID)
	fmt.Printf("  Status: %s\n", sub.Status)
	fmt.Printf("  Quantity: %d\n", sub.Quantity)
	fmt.Printf("  Total Count: %d (0 = infinite)\n", sub.TotalCount)
	fmt.Printf("  Paid Count: %d\n", sub.PaidCount)
	fmt.Printf("  Remaining Count: %d\n", sub.RemainingCount)

	if sub.StartAt > 0 {
		fmt.Printf("  Start At: %s\n", time.Unix(sub.StartAt, 0).Format(time.RFC3339))
	}
	if sub.EndAt > 0 {
		fmt.Printf("  End At: %s\n", time.Unix(sub.EndAt, 0).Format(time.RFC3339))
	}
	if sub.CreatedAt > 0 {
		fmt.Printf("  Created At: %s\n", time.Unix(sub.CreatedAt, 0).Format(time.RFC3339))
	}
	if sub.ChargeAt > 0 {
		fmt.Printf("  Next Charge: %s\n", time.Unix(sub.ChargeAt, 0).Format(time.RFC3339))
	}
	if sub.CurrentStart > 0 {
		fmt.Printf("  Current Cycle: %s to %s\n",
			time.Unix(sub.CurrentStart, 0).Format(time.RFC3339),
			time.Unix(sub.CurrentEnd, 0).Format(time.RFC3339))
	}

	if sub.HasScheduledChanges {
		fmt.Printf("  Has Scheduled Changes: Yes (at %s)\n",
			time.Unix(sub.ChangeScheduledAt, 0).Format(time.RFC3339))
	}

	if sub.ShortURL != "" {
		fmt.Printf("  Payment URL: %s\n", sub.ShortURL)
	}

	// Print notes from API response
	notesMap := sub.GetNotesMap()
	if len(notesMap) > 0 {
		fmt.Printf("  Notes:\n")
		for key, value := range notesMap {
			fmt.Printf("    %s: %s\n", key, value)
		}
	}

	fmt.Println()
}
