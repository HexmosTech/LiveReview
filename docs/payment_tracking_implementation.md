# Payment Tracking Implementation

## Overview
This implementation adds comprehensive payment tracking to LiveReview subscriptions, ensuring that license assignments are only allowed after payment has been successfully received and verified.

## Key Features

### 1. Database Schema Changes
**Migration:** `20251209_add_payment_tracking.sql`

#### Subscriptions Table Updates
Added payment tracking fields to the `subscriptions` table:
- `last_payment_id` - Razorpay payment ID from most recent payment
- `last_payment_status` - Status of last payment (authorized, captured, failed, refunded)
- `last_payment_received_at` - Timestamp when payment was actually received (captured)
- `payment_verified` - Boolean flag indicating if any payment has been successfully received

#### New subscription_payments Table
Complete audit trail of all payments:
- Tracks all payment lifecycle events (authorized, captured, failed, refunded)
- Stores Razorpay payment details (payment_id, order_id, invoice_id)
- Records payment amounts, currency, and methods
- Captures error information for failed payments
- Maintains timestamps for each payment state transition

### 2. Webhook Handlers
**File:** `internal/license/payment/webhook_handler.go`

Implemented three new webhook handlers:

#### payment.authorized
- Triggered when payment is authorized but not yet captured
- Records payment in `subscription_payments` table
- Logs event in `license_log` for audit trail
- Does NOT mark subscription as payment_verified (waiting for capture)

#### payment.captured (CRITICAL)
- Triggered when payment is successfully captured (money received!)
- Records/updates payment in `subscription_payments` table
- **Updates subscription with payment_verified = TRUE**
- Updates last_payment_id, last_payment_status, last_payment_received_at
- This event enables license assignments

#### payment.failed
- Triggered when payment fails
- Records failure details including error codes and descriptions
- Updates subscription status but does NOT mark as verified
- Prevents license assignments

### 3. Payment Verification in License Assignment
**File:** `internal/license/payment/subscription_service.go`

#### AssignLicense Changes
**CRITICAL CHANGE:** Added payment verification check before allowing license assignment:

```go
if !paymentVerified {
    return fmt.Errorf("cannot assign license: payment not yet received for subscription %s (status: %s)", 
        subscriptionID, lastPaymentStatus.String)
}
```

This ensures:
- No licenses can be assigned until payment.captured webhook is received
- Users cannot use subscriptions they haven't paid for
- Clear error message indicating payment status

### 4. Subscription Details Enhancement
**File:** `internal/license/payment/subscription_service.go`

Enhanced `SubscriptionDetails` struct with payment information:
- `payment_verified` - Whether payment has been received
- `last_payment_id` - Razorpay payment ID for reference
- `last_payment_status` - Current payment status
- `last_payment_received_at` - When payment was captured

This information is displayed on the subscription details page, allowing users to:
- See their Razorpay payment ID
- Check payment status
- Verify when payment was received

## Payment Flow

### Successful Payment Flow
1. User purchases subscription → Razorpay creates subscription
2. Razorpay attempts first payment → `payment.authorized` webhook
   - Payment recorded in database
   - subscription.payment_verified = FALSE (cannot assign yet)
3. Payment successfully captured → `payment.captured` webhook
   - Payment updated in database
   - **subscription.payment_verified = TRUE**
   - **subscription.last_payment_received_at = NOW()**
   - Licenses can now be assigned!

### Failed Payment Flow
1. User purchases subscription → Razorpay creates subscription
2. Payment fails → `payment.failed` webhook
   - Failure recorded with error details
   - subscription.payment_verified = FALSE
   - License assignment blocked with clear error message

### Renewal Flow
Each billing cycle:
1. Razorpay attempts renewal → `payment.authorized`
2. Success → `payment.captured`
   - Updates last_payment_id and last_payment_received_at
   - Existing licenses remain valid
3. Failure → `payment.failed`
   - subscription.last_payment_status updated to "failed"
   - Existing licenses continue until expiry (grace period)

## API Integration

### Webhook Endpoint
The existing webhook endpoint handles all payment events:
```
POST /api/webhooks/razorpay
```

Razorpay must be configured to send these webhooks:
- payment.authorized
- payment.captured  
- payment.failed

### Subscription Details API
The subscription details endpoint now returns payment information:
```json
{
  "id": 123,
  "razorpay_subscription_id": "sub_...",
  "payment_verified": true,
  "last_payment_id": "pay_...",
  "last_payment_status": "captured",
  "last_payment_received_at": "2024-12-09T10:30:00Z",
  ...
}
```

## Testing Recommendations

1. **Test Payment Capture:**
   - Create subscription
   - Verify payment_verified = FALSE initially
   - Trigger payment.captured webhook
   - Verify payment_verified = TRUE
   - Verify license assignment now works

2. **Test Payment Verification:**
   - Create subscription without payment capture
   - Attempt license assignment
   - Should fail with "payment not yet received" error

3. **Test Failed Payments:**
   - Trigger payment.failed webhook
   - Verify error details are captured
   - Verify license assignment is blocked

4. **Test Subscription Details:**
   - View subscription details page
   - Verify Razorpay payment ID is displayed
   - Verify payment status is shown
   - Verify payment received timestamp is shown

## Security Considerations

1. **Webhook Signature Verification:** All webhooks are verified using HMAC signature
2. **Payment State Transitions:** Only captured payments enable license assignments
3. **Idempotency:** Payment webhooks can be safely replayed (ON CONFLICT DO UPDATE)
4. **Audit Trail:** All payment events logged in license_log table

## Configuration

### Razorpay Webhook Setup
Configure these webhook events in Razorpay dashboard:
1. payment.authorized
2. payment.captured
3. payment.failed

### Webhook URL
Point Razorpay to: `https://your-domain.com/api/webhooks/razorpay`

### Webhook Secret
Set in environment: `RAZORPAY_WEBHOOK_SECRET`

## Monitoring

### Database Queries
Check payment verification status:
```sql
SELECT razorpay_subscription_id, payment_verified, last_payment_status, last_payment_received_at
FROM subscriptions
WHERE payment_verified = FALSE;
```

View payment history:
```sql
SELECT sp.*, s.razorpay_subscription_id
FROM subscription_payments sp
JOIN subscriptions s ON sp.subscription_id = s.id
ORDER BY sp.created_at DESC;
```

Check failed payments:
```sql
SELECT * FROM subscription_payments
WHERE status = 'failed'
ORDER BY failed_at DESC;
```

### License Log
All payment events are logged:
```sql
SELECT * FROM license_log
WHERE event_type IN ('payment_authorized', 'payment_captured', 'payment_failed')
ORDER BY created_at DESC;
```

## Future Enhancements

1. **Payment Retry Logic:** Handle automatic retry for failed payments
2. **Payment Method Tracking:** Display payment method used (card, UPI, etc.)
3. **Refund Handling:** Add webhook handler for payment.refunded
4. **Payment History UI:** Show all payments in subscription details
5. **Grace Period:** Configure grace period for failed renewals
6. **Payment Notifications:** Email notifications for payment events

## Related Documentation

- [Razorpay Webhooks Documentation](https://razorpay.com/docs/webhooks/payments/)
- [Razorpay Payment Entity](https://razorpay.com/docs/api/payments/)
- Database Schema: `db/migrations/20251209_add_payment_tracking.sql`
