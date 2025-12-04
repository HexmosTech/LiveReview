# Razorpay Payment Integration

This package provides a complete Razorpay REST API integration for LiveReview subscription management.

## Files

### Core Implementation
- **payment.go** - Plan management functions (create, fetch, list)
- **payment_types.go** - Type definitions for Razorpay plans
- **subscription.go** - Subscription management functions (create, fetch, update, cancel)
- **subscription_types.go** - Type definitions for Razorpay subscriptions

### Tests
- **payment_test.go** - Tests for plan operations
- **subscription_test.go** - Tests for subscription operations

## Features

### Plan Management
- ✅ Create monthly plan ($6/user/month)
- ✅ Create yearly plan ($60/user/year, 17% discount)
- ✅ Fetch all plans
- ✅ Fetch plan by ID
- ✅ Polymorphic notes handling (object or array)

### Subscription Management
- ✅ Create subscription with quantity
- ✅ Fetch all subscriptions
- ✅ Fetch subscription by ID
- ✅ Update subscription quantity (requires active/authenticated subscription)
- ✅ Cancel subscription (immediate or at cycle end)
- ✅ Custom notes support

## Usage Examples

### Create a Plan
```go
import "github.com/livereview/internal/license/payment"

// Create monthly plan ($6/user/month)
plan, err := CreatePlan("test", "monthly")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Plan ID: %s\n", plan.ID)

// Create yearly plan ($60/user/year)
yearlyPlan, err := CreatePlan("test", "yearly")
```

### Fetch Plans
```go
// Get all plans
planList, err := GetAllPlans("test")
for _, plan := range planList.Items {
    notesMap := plan.GetNotesMap()
    fmt.Printf("%s: %s - $%.2f\n", plan.ID, plan.Item.Name, 
        float64(plan.Item.Amount)/100)
}

// Get specific plan
plan, err := GetPlanByID("test", "plan_abc123")
```

### Create a Subscription
```go
// Create subscription with 5 users
subscription, err := CreateSubscription("test", planID, 5, map[string]string{
    "org_name": "Acme Corp",
    "team": "Engineering",
})
fmt.Printf("Subscription ID: %s\n", subscription.ID)
fmt.Printf("Status: %s\n", subscription.Status)
fmt.Printf("Payment URL: %s\n", subscription.ShortURL)
```

### Fetch Subscriptions
```go
// Get all subscriptions
subList, err := GetAllSubscriptions("test")
for _, sub := range subList.Items {
    fmt.Printf("%s: %s (Qty: %d)\n", sub.ID, sub.Status, sub.Quantity)
}

// Get specific subscription
sub, err := GetSubscriptionByID("test", "sub_xyz789")
```

### Update Subscription Quantity
```go
// Update quantity to 8 users (scheduled at end of current cycle)
updatedSub, err := UpdateSubscriptionQuantity("test", subscriptionID, 8, 0)
if err != nil {
    log.Fatal(err)
}
fmt.Printf("New quantity: %d\n", updatedSub.Quantity)
if updatedSub.HasScheduledChanges {
    fmt.Printf("Changes scheduled for: %s\n", 
        time.Unix(updatedSub.ChangeScheduledAt, 0))
}
```

### Cancel Subscription
```go
// Cancel immediately
cancelled, err := CancelSubscription("test", subscriptionID, false)

// Cancel at end of billing cycle (requires active subscription)
cancelled, err := CancelSubscription("test", subscriptionID, true)
```

## API Credentials

Credentials are managed in `payment.go`:
- **Test Mode**: `REDACTED_TEST_KEY` / `REDACTED_TEST_SECRET`
- **Live Mode**: `REDACTED_LIVE_KEY` / `REDACTED_LIVE_SECRET`

Use "test" or "live" mode parameter in all functions.

## Plan Configuration

### Monthly Plan
- **Price**: $6 per user per month (600 cents)
- **Name**: "LiveReview Team - Monthly"
- **Period**: monthly
- **Interval**: 1

### Yearly Plan
- **Price**: $60 per user per year (6000 cents)
- **Name**: "LiveReview Team - Annual"
- **Period**: yearly
- **Interval**: 1
- **Discount**: 17% compared to monthly ($72/year vs $60/year)

## Subscription States

Razorpay subscriptions can be in the following states:
- **created**: Subscription created but not authenticated
- **authenticated**: Payment method verified but not charged
- **active**: Subscription active with successful payments
- **pending**: Payment pending
- **halted**: Subscription paused due to failed payment
- **cancelled**: Subscription cancelled
- **completed**: All billing cycles completed
- **expired**: Subscription expired
- **paused**: Subscription paused by user

## Notes on Testing

1. **Update Quantity**: Requires subscription to be in `authenticated` or `active` state. Test mode subscriptions in `created` state will fail.

2. **Cancel at Cycle End**: Requires an active billing cycle. New subscriptions without payments cannot be scheduled for cancellation and will fail. Use immediate cancellation instead.

3. **Polymorphic Notes**: The `notes` field can be either:
   - An object (map) when notes are present: `{"app_name": "LiveReview"}`
   - An empty array when no notes: `[]`
   
   Use `GetNotesMap()` method to safely retrieve notes as a map.

4. **Customer Notify**: Set to `false` (0) in test mode to avoid sending emails.

## Running Tests

```bash
# Run all tests
cd internal/license/payment
go test -v

# Run specific test
go test -v -run TestCreateSubscription

# Run plan tests only
go test -v -run "TestCreate.*Plan|TestGet.*Plan"

# Run subscription tests only
go test -v -run "TestCreate.*Subscription|TestGet.*Subscription|TestUpdate|TestCancel"
```

## Notes

- All monetary amounts are in the smallest currency unit (cents for USD, paise for INR)
- Timestamps are Unix timestamps (seconds since epoch)
- Default subscription length: 12 billing cycles
- Payment links are automatically generated and available in `ShortURL` field
- The API uses Basic Authentication with key_id:key_secret
