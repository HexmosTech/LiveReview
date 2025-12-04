# Razorpay Subscription Implementation Plan

## Executive Summary

This document outlines a comprehensive, end-to-end implementation plan for Razorpay subscription integration in LiveReview to support the Team plan pricing model:
- **Monthly Plan**: $6/user/month
- **Annual Plan**: $60/user/year ($5/month equivalent, 17% discount)

**Key Design Principles:**
1. **No Fallbacks**: Clear, predictable behavior - explicit failures over silent degradation
2. **JWT-Based Enforcement**: License validation at the authentication layer
3. **First Principles Approach**: Build from ground up rather than copying competitors
4. **Incremental Implementation**: Start with enforcement, then build purchasing flow

---

## 1. System Architecture Overview

### 1.1 Core Concepts

**License Ownership Model:**
- **License Owner**: User who purchases subscription (can apply licenses across all orgs they own)
- **Available Licenses**: Total seats purchased (quantity in subscription)
- **Applied Licenses**: Licenses assigned to specific users across owner's organizations
- **Unassigned Licenses**: Available - Applied

**License States:**
```
Free Plan → Purchase → Active Team License → Expired → Back to Free Plan
                ↓                               ↓
         Apply to Users                  Users lose access
```

**Plan Enforcement:**

Enforced at JWT level to avoid database lookups on every request. The JWT contains only the minimal information needed to make authorization decisions:

**JWT Claims for Plan Enforcement:**
1. **`planType`** (string): Current plan tier (`"free"` | `"team"` | `"enterprise"`)
   - **Purpose**: Determines which features the user can access
   - **Example**: Free users blocked from unlimited reviews, Team users allowed

2. **`licenseExpiresAt`** (int64 | null): Unix timestamp when license expires
   - **Purpose**: Enables instant expiry checks without DB lookup
   - **Value**: `null` for free plan (never expires), epoch timestamp for paid plans
   - **Example**: `1735689600` = expires Dec 31, 2024 23:59:59 UTC

3. **`subscriptionID`** (int64 | null): Reference to active subscription
   - **Purpose**: Track which subscription provides this license (for auditing/debugging)
   - **Value**: `null` for free users, subscription ID for paid users
   - **Note**: Not used for enforcement, only for operational tracking

**Examples:**
- Free user: `{planType: "free", licenseExpiresAt: null, subscriptionID: null}`
- Team user: `{planType: "team", licenseExpiresAt: 1735689600, subscriptionID: 42}`

**Enforcement Flow:**
1. User makes API request
2. JWT middleware extracts claims
3. Check `licenseExpiresAt`: if expired → reject with 402 Payment Required
4. Check `planType`: lookup feature requirements → if not allowed → reject with 403 Forbidden
5. For review creation: check daily limit based on `planType` (free=3, team=unlimited)

**Why not include more data?**
- **No seat count**: Not needed per-request, only for assignment operations
- **No org list**: User can be in multiple orgs, would bloat token
- **No feature list**: Derived from planType to keep token small
- **No user metadata**: Name, email already in base claims

---

## 2. Database Schema Design

### 2.1 New Tables

```sql
-- Subscriptions: Track Razorpay subscriptions
-- Note: Licenses from a subscription can be applied to users across all orgs owned by owner_user_id
CREATE TABLE subscriptions (
    id BIGSERIAL PRIMARY KEY,
    
    -- Razorpay Integration
    razorpay_subscription_id VARCHAR(255) UNIQUE NOT NULL,
    razorpay_plan_id VARCHAR(255) NOT NULL,
    
    -- Ownership
    owner_user_id BIGINT NOT NULL REFERENCES users(id),
    org_id BIGINT NOT NULL REFERENCES organizations(id), -- Primary org for billing, but licenses can be applied across owner's orgs
    
    -- Subscription Details
    plan_type VARCHAR(50) NOT NULL, -- 'team_monthly' | 'team_annual'
    quantity INT NOT NULL, -- number of seats
    status VARCHAR(50) NOT NULL, -- 'created' | 'active' | 'cancelled' | 'expired'
    
    -- Billing Cycle
    current_period_start TIMESTAMP,
    current_period_end TIMESTAMP,
    
    -- Lifecycle Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    activated_at TIMESTAMP,
    cancelled_at TIMESTAMP,
    expired_at TIMESTAMP,
    
    -- Razorpay Metadata (JSON for flexibility)
    razorpay_data JSONB,
    
    CONSTRAINT valid_quantity CHECK (quantity > 0)
);

CREATE INDEX idx_subscriptions_owner ON subscriptions(owner_user_id);
CREATE INDEX idx_subscriptions_org ON subscriptions(org_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_razorpay ON subscriptions(razorpay_subscription_id);


-- License Assignments: Track which users have licenses applied
-- Users can be assigned licenses in any org owned by the subscription owner
CREATE TABLE license_assignments (
    id BIGSERIAL PRIMARY KEY,
    
    -- Relationships
    subscription_id BIGINT NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL REFERENCES organizations(id), -- The org where this license is being used
    
    -- Assignment Metadata
    assigned_by_user_id BIGINT NOT NULL REFERENCES users(id),
    assigned_at TIMESTAMP NOT NULL DEFAULT NOW(),
    
    -- Revocation Tracking
    revoked_at TIMESTAMP,
    revoked_by_user_id BIGINT REFERENCES users(id),
    
    -- Validity Period (synced from subscription)
    valid_from TIMESTAMP NOT NULL,
    valid_until TIMESTAMP, -- NULL for active, set when subscription ends
    
    -- Status
    status VARCHAR(50) NOT NULL DEFAULT 'active', -- 'active' | 'revoked' | 'expired'
    
    CONSTRAINT unique_active_assignment UNIQUE (subscription_id, user_id, status),
    CONSTRAINT no_self_assignment CHECK (user_id != assigned_by_user_id)
);

CREATE INDEX idx_license_assignments_user ON license_assignments(user_id, status);
CREATE INDEX idx_license_assignments_subscription ON license_assignments(subscription_id);
CREATE INDEX idx_license_assignments_org ON license_assignments(org_id);


-- Payment Events: Audit trail for all payment activities
CREATE TABLE payment_events (
    id BIGSERIAL PRIMARY KEY,
    
    -- Event Details
    event_type VARCHAR(100) NOT NULL, -- 'subscription.created', 'payment.captured', etc.
    razorpay_event_id VARCHAR(255) UNIQUE,
    
    -- Relationships
    subscription_id BIGINT REFERENCES subscriptions(id),
    user_id BIGINT REFERENCES users(id),
    
    -- Event Data
    payload JSONB NOT NULL,
    
    -- Processing Status
    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP,
    error_message TEXT,
    
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_payment_events_type ON payment_events(event_type);
CREATE INDEX idx_payment_events_subscription ON payment_events(subscription_id);
CREATE INDEX idx_payment_events_processed ON payment_events(processed);
```

### 2.2 User Table Extension

```sql
-- Add to existing users table
ALTER TABLE users ADD COLUMN plan_type VARCHAR(50) DEFAULT 'free' NOT NULL;
ALTER TABLE users ADD COLUMN license_expires_at TIMESTAMP;
ALTER TABLE users ADD COLUMN active_subscription_id BIGINT REFERENCES subscriptions(id);

CREATE INDEX idx_users_plan_type ON users(plan_type);
CREATE INDEX idx_users_license_expires ON users(license_expires_at);
```

---

## 3. Plan Limits & Enforcement

### 3.1 Plan Definitions

```go
// internal/license/plans.go
package license

type PlanType string

const (
    PlanFree       PlanType = "free"
    PlanTeam       PlanType = "team"
    PlanEnterprise PlanType = "enterprise"
)

type PlanLimits struct {
    PlanType          PlanType
    MaxReviewsPerDay  int    // -1 for unlimited
    MaxOrganizations  int
    MaxUsers          int    // per org
    Features          []string
}

var PlanDefinitions = map[PlanType]PlanLimits{
    PlanFree: {
        PlanType:         PlanFree,
        MaxReviewsPerDay: 3,
        MaxOrganizations: 1,
        MaxUsers:         1,
        Features:         []string{"basic_review", "email_support"},
    },
    PlanTeam: {
        PlanType:         PlanTeam,
        MaxReviewsPerDay: -1, // unlimited
        MaxOrganizations: -1, // unlimited
        MaxUsers:         -1, // unlimited (based on seats purchased)
        Features:         []string{"unlimited_reviews", "multiple_orgs", "cloud_ai", "email_support", "priority_support"},
    },
}

func (p PlanType) GetLimits() PlanLimits {
    return PlanDefinitions[p]
}
```

### 3.2 JWT Claims Extension

```go
// internal/api/auth/types.go
type CustomClaims struct {
    UserID           int64     `json:"uid"`
    Email            string    `json:"email"`
    
    // Plan & License Information
    PlanType         string    `json:"plan_type"`          // "free" | "team" | "enterprise"
    LicenseExpiresAt *int64    `json:"license_expires_at"` // Unix timestamp, null for free
    SubscriptionID   *int64    `json:"subscription_id"`    // Reference to active subscription
    
    jwt.RegisteredClaims
}
```

### 3.3 Enforcement Middleware

```go
// internal/api/middleware/plan_enforcement.go
package middleware

import (
    "net/http"
    "time"
    
    "github.com/labstack/echo/v4"
    "github.com/livereview/internal/license"
)

// EnforcePlan checks if user's plan allows the requested action
func EnforcePlan(requiredFeature string) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            claims := c.Get("claims").(*auth.CustomClaims)
            
            // Check expiration
            if claims.LicenseExpiresAt != nil {
                expiryTime := time.Unix(*claims.LicenseExpiresAt, 0)
                if time.Now().After(expiryTime) {
                    return echo.NewHTTPError(http.StatusPaymentRequired, 
                        "Your license has expired. Please renew to continue.")
                }
            }
            
            // Check plan limits
            planType := license.PlanType(claims.PlanType)
            limits := planType.GetLimits()
            
            // Feature check
            hasFeature := false
            for _, f := range limits.Features {
                if f == requiredFeature {
                    hasFeature = true
                    break
                }
            }
            
            if !hasFeature {
                return echo.NewHTTPError(http.StatusForbidden,
                    "This feature requires an upgrade to Team plan")
            }
            
            return next(c)
        }
    }
}

// CheckReviewLimit enforces daily review limits
func CheckReviewLimit(db *sql.DB) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            claims := c.Get("claims").(*auth.CustomClaims)
            planType := license.PlanType(claims.PlanType)
            limits := planType.GetLimits()
            
            if limits.MaxReviewsPerDay == -1 {
                return next(c) // unlimited
            }
            
            // Count today's reviews
            count, err := countTodayReviews(db, claims.UserID)
            if err != nil {
                return err
            }
            
            if count >= limits.MaxReviewsPerDay {
                return echo.NewHTTPError(http.StatusTooManyRequests,
                    "Daily review limit reached. Upgrade to Team plan for unlimited reviews.")
            }
            
            return next(c)
        }
    }
}
```

---

## 4. API Design

### 4.1 Subscription Management APIs

```go
// POST /api/v1/subscriptions
// Create a new subscription (returns Razorpay subscription for checkout)
type CreateSubscriptionRequest struct {
    PlanType    string `json:"plan_type" validate:"required,oneof=team_monthly team_annual"`
    Quantity    int    `json:"quantity" validate:"required,min=1"`
    OrgID       int64  `json:"org_id" validate:"required"`
}

type CreateSubscriptionResponse struct {
    SubscriptionID         int64  `json:"subscription_id"`
    RazorpaySubscriptionID string `json:"razorpay_subscription_id"`
    RazorpayKeyID          string `json:"razorpay_key_id"`
    ShortURL               string `json:"short_url"`
    Amount                 int    `json:"amount"`
    Currency               string `json:"currency"`
}


// GET /api/v1/subscriptions
// List all subscriptions for the user's orgs
type ListSubscriptionsResponse struct {
    Subscriptions []SubscriptionSummary `json:"subscriptions"`
}

type SubscriptionSummary struct {
    ID                     int64     `json:"id"`
    RazorpaySubscriptionID string    `json:"razorpay_subscription_id"`
    OrgID                  int64     `json:"org_id"`
    OrgName                string    `json:"org_name"`
    PlanType               string    `json:"plan_type"`
    Status                 string    `json:"status"`
    Quantity               int       `json:"quantity"`
    AssignedSeats          int       `json:"assigned_seats"`
    AvailableSeats         int       `json:"available_seats"`
    CurrentPeriodEnd       time.Time `json:"current_period_end"`
}


// GET /api/v1/subscriptions/:id
// Get detailed subscription info
type GetSubscriptionResponse struct {
    Subscription      SubscriptionDetail    `json:"subscription"`
    Assignments       []LicenseAssignment   `json:"assignments"`        // All assignments across owner's orgs
    AvailableUsers    []UserSummary         `json:"available_users"`    // Users from all owner's orgs
    OwnerOrgs         []OrgSummary          `json:"owner_orgs"`         // All orgs owned by subscription owner
}


// PATCH /api/v1/subscriptions/:id/quantity
// Update seat count (triggers Razorpay update)
type UpdateQuantityRequest struct {
    Quantity          int   `json:"quantity" validate:"required,min=1"`
    ScheduleChangeAt  int64 `json:"schedule_change_at"` // 0 for immediate, timestamp for scheduled
}


// POST /api/v1/subscriptions/:id/cancel
// Cancel subscription
type CancelSubscriptionRequest struct {
    CancelAtCycleEnd bool `json:"cancel_at_cycle_end"`
}
```

### 4.2 License Assignment APIs

```go
// POST /api/v1/subscriptions/:id/licenses
// Assign licenses to users
type AssignLicensesRequest struct {
    UserIDs []int64 `json:"user_ids" validate:"required,min=1"`
}

type AssignLicensesResponse struct {
    Assigned   []int64 `json:"assigned"`
    Failed     []AssignmentError `json:"failed"`
}

type AssignmentError struct {
    UserID int64  `json:"user_id"`
    Reason string `json:"reason"`
}


// DELETE /api/v1/subscriptions/:id/licenses/:user_id
// Revoke license from user
type RevokeLicenseResponse struct {
    Success bool   `json:"success"`
    Message string `json:"message"`
}


// GET /api/v1/users/:id/license
// Get license info for a specific user
type GetUserLicenseResponse struct {
    UserID           int64     `json:"user_id"`
    PlanType         string    `json:"plan_type"`
    LicenseExpiresAt *time.Time `json:"license_expires_at"`
    SubscriptionID   *int64    `json:"subscription_id"`
    AssignedBy       *UserInfo `json:"assigned_by"`
    AssignedAt       *time.Time `json:"assigned_at"`
}
```

### 4.3 Webhook Handler

```go
// POST /api/v1/webhooks/razorpay
// Handle Razorpay webhooks
// Events to handle:
// - subscription.activated
// - subscription.charged
// - subscription.completed
// - subscription.halted
// - subscription.cancelled
// - subscription.paused
// - subscription.resumed
// - payment.captured
// - payment.failed
```

---

## 5. Implementation Phases

### Phase 1: Enforcement Infrastructure (Week 1)
**Goal:** Make the system aware of plans and enforce limits

1. **Database Schema** ✅
   - Create migration files for new tables
   - Run migrations
   - Add indices

2. **Plan Definitions** ✅
   - Create `internal/license/plans.go`
   - Define plan limits
   - Write unit tests

3. **JWT Integration** ✅
   - Extend JWT claims with plan info
   - Update token generation logic
   - Update token validation

4. **Enforcement Middleware** ✅
   - Create plan enforcement middleware
   - Add review limit checks
   - Wire up to existing routes

5. **Testing** ✅
   - Test free plan limits
   - Test enforcement behavior
   - Test JWT generation/validation

**Deliverable:** System enforces plan limits, free users are blocked at 3 reviews/day

---

### Phase 2: Razorpay Integration (Week 2)
**Goal:** Connect to Razorpay, create/manage subscriptions

1. **Plan Setup in Razorpay** ✅
   - Create Team Monthly plan ($6/user/month)
   - Create Team Annual plan ($60/user/year - $5/month equivalent, 17% discount)
   - Store plan IDs in config

2. **Subscription Service** ✅
   - Create `internal/license/subscription_service.go`
   - Implement CreateSubscription
   - Implement GetSubscription
   - Implement UpdateQuantity
   - Implement CancelSubscription

3. **Webhook Handler** ✅
   - Create webhook endpoint
   - Implement signature verification
   - Handle subscription.activated
   - Handle payment.captured
   - Handle subscription.cancelled

4. **Database Layer** ✅
   - Implement subscription CRUD operations
   - Add transaction support
   - Error handling

**Deliverable:** Can create subscriptions in Razorpay via API

---

### Phase 3: License Assignment (Week 3)
**Goal:** Owners can assign purchased licenses to users

1. **Assignment Service** ✅
   - Create `internal/license/assignment_service.go`
   - Implement AssignLicense
   - Implement RevokeLicense
   - Implement GetAssignments

2. **User Sync** ✅
   - Update user's `plan_type` on assignment
   - Update user's `license_expires_at`
   - Issue new JWT with updated claims

3. **API Endpoints** ✅
   - POST /api/v1/subscriptions/:id/licenses
   - DELETE /api/v1/subscriptions/:id/licenses/:user_id
   - GET /api/v1/users/:id/license

4. **Validation Logic** ✅
   - Check seat availability
   - Only subscription owner can assign licenses
   - Can assign to users in any org owned by subscription owner
   - User must be a member of the target org

**Deliverable:** Subscription owners can assign/revoke licenses to users across all their organizations

---

### Phase 4: UI Integration (Week 4)
**Goal:** Complete user-facing purchase and management flow

1. **Subscription Page (`/subscribe`)** ✅
   - Display plan pricing
   - "Get Team Plan" button
   - Seat quantity selector
   - Razorpay checkout integration

2. **Checkout Flow** ✅
   - Initialize Razorpay checkout
   - Handle success callback
   - Handle failure callback
   - Redirect after payment

3. **License Management Page** ✅
   - View active subscription
   - See assigned/available seats
   - Assign licenses to org members
   - Revoke licenses
   - Update seat count

4. **User Dashboard** ✅
   - Show current plan
   - Show expiration date
   - Upgrade prompts for free users

**Deliverable:** Full UI for purchasing and managing subscriptions

---

### Phase 5: Renewal & Lifecycle (Week 5)
**Goal:** Handle subscription renewals and expirations gracefully

1. **Expiration Handling** ✅
   - Scheduled job to check expiring licenses
   - Email notifications (7 days, 3 days, 1 day before)
   - Auto-downgrade on expiry

2. **Grace Period** ✅
   - 7-day grace period after expiry
   - Read-only access during grace
   - Clear messaging

3. **Renewal Flow** ✅
   - Automatic renewal via Razorpay
   - Manual renewal option
   - Payment retry logic

4. **Cancellation Flow** ✅
   - Immediate vs end-of-cycle cancellation
   - Refund handling (if applicable)
   - Data retention policy

**Deliverable:** Automated lifecycle management

---

### Phase 6: Testing & Polish (Week 6)
**Goal:** Ensure reliability and handle edge cases

1. **Integration Tests** ✅
   - Full purchase flow
   - Assignment flow
   - Webhook processing
   - Expiration handling

2. **Edge Cases** ✅
   - Payment failures
   - Webhook replay attacks
   - Concurrent assignments
   - Subscription downgrades

3. **Admin Tools** ✅
   - Manual license grants (for support)
   - Subscription override
   - Audit logs

4. **Documentation** ✅
   - API documentation
   - Integration guide
   - Troubleshooting guide

**Deliverable:** Production-ready subscription system

---

## 6. Key Implementation Details

### 6.1 Razorpay Plan IDs (from payment.go)

```go
const (
    // Test Mode
    TeamMonthlyTestPlanID = "plan_test_monthly_team"  // $6/user/month
    TeamAnnualTestPlanID  = "plan_test_annual_team"   // $60/user/year (17% discount)
    
    // Live Mode
    TeamMonthlyLivePlanID = "plan_live_monthly_team"  // $6/user/month
    TeamAnnualLivePlanID  = "plan_live_annual_team"   // $60/user/year (17% discount)
)

func GetPlanID(mode, planType string) (string, error) {
    switch mode {
    case "test":
        if planType == "team_monthly" {
            return TeamMonthlyTestPlanID, nil
        }
        return TeamAnnualTestPlanID, nil
    case "live":
        if planType == "team_monthly" {
            return TeamMonthlyLivePlanID, nil
        }
        return TeamAnnualLivePlanID, nil
    }
    return "", fmt.Errorf("invalid mode or plan type")
}
```

### 6.2 Checkout Integration (UI)

```typescript
// ui/src/pages/Subscribe.tsx
const handleCheckout = async (planType: string, quantity: number) => {
    // 1. Create subscription via API
    const response = await fetch('/api/v1/subscriptions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plan_type: planType, quantity, org_id: currentOrgId })
    });
    
    const data = await response.json();
    
    // 2. Initialize Razorpay checkout
    const options = {
        key: data.razorpay_key_id,
        subscription_id: data.razorpay_subscription_id,
        name: "LiveReview",
        description: `${planType} - ${quantity} seats`,
        handler: async (response) => {
            // 3. Verify payment on backend
            await fetch('/api/v1/subscriptions/verify-payment', {
                method: 'POST',
                body: JSON.stringify({
                    razorpay_payment_id: response.razorpay_payment_id,
                    razorpay_subscription_id: response.razorpay_subscription_id,
                    razorpay_signature: response.razorpay_signature
                })
            });
            
            // 4. Redirect to license management
            window.location.href = '/dashboard/licenses';
        },
        theme: { color: "#3B82F6" }
    };
    
    const rzp = new Razorpay(options);
    rzp.open();
};
```

### 6.3 Webhook Security

```go
// internal/api/webhooks/razorpay.go
func VerifyWebhookSignature(payload []byte, signature, secret string) bool {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    expectedSignature := hex.EncodeToString(mac.Sum(nil))
    return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func HandleRazorpayWebhook(c echo.Context) error {
    signature := c.Request().Header.Get("X-Razorpay-Signature")
    body, _ := ioutil.ReadAll(c.Request().Body)
    
    if !VerifyWebhookSignature(body, signature, webhookSecret) {
        return echo.NewHTTPError(http.StatusUnauthorized, "Invalid signature")
    }
    
    var event RazorpayWebhookEvent
    json.Unmarshal(body, &event)
    
    // Process event asynchronously
    go processWebhookEvent(event)
    
    return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
```

### 6.4 Assignment Logic

```go
// internal/license/assignment_service.go
func (s *AssignmentService) AssignLicense(subscriptionID, userID, assignedByUserID int64) error {
    tx, _ := s.db.Begin()
    defer tx.Rollback()
    
    // 1. Check subscription exists and has available seats
    sub, err := s.getSubscription(tx, subscriptionID)
    if err != nil {
        return err
    }
    
    assignedCount, _ := s.countAssignedSeats(tx, subscriptionID)
    if assignedCount >= sub.Quantity {
        return errors.New("no available seats")
    }
    
    // 2. Verify assigner is the subscription owner
    if assignedByUserID != sub.OwnerUserID {
        return errors.New("only subscription owner can assign licenses")
    }
    
    // 3. Check user is in an org owned by subscription owner
    userOrgID, err := s.getUserOrgID(tx, userID)
    if err != nil {
        return errors.New("user not found or not in any organization")
    }
    
    if !s.userOwnsOrg(tx, sub.OwnerUserID, userOrgID) {
        return errors.New("can only assign licenses to users in organizations you own")
    }
    
    // 4. Create assignment (org_id is the user's org, not subscription's org)
    _, err = tx.Exec(`
        INSERT INTO license_assignments 
        (subscription_id, user_id, org_id, assigned_by_user_id, valid_from, status)
        VALUES ($1, $2, $3, $4, NOW(), 'active')
    `, subscriptionID, userID, userOrgID, assignedByUserID)
    
    if err != nil {
        return err
    }
    
    // 5. Update user's plan
    _, err = tx.Exec(`
        UPDATE users 
        SET plan_type = 'team', 
            license_expires_at = $1,
            active_subscription_id = $2
        WHERE id = $3
    `, sub.CurrentPeriodEnd, subscriptionID, userID)
    
    if err != nil {
        return err
    }
    
    tx.Commit()
    
    // 6. Invalidate user's existing JWTs (optional - force re-login)
    s.invalidateUserTokens(userID)
    
    return nil
}
```

---

## 7. API Specification (Refined)

### 7.1 Create Subscription

```
POST /api/v1/subscriptions

Request:
{
    "plan_type": "team_annual",
    "quantity": 10,
    "org_id": 5
}

Response (201):
{
    "subscription_id": 42,
    "razorpay_subscription_id": "sub_ABC123XYZ",
    "razorpay_key_id": "REDACTED_LIVE_KEY",
    "short_url": "https://rzp.io/l/ABC123",
    "amount": 60000,  // $600 total for 10 seats/year ($60/seat * 10 seats)
    "currency": "USD",
    "quantity": 10,
    "status": "created"
}
```

### 7.2 Assign License

```
POST /api/v1/subscriptions/:id/licenses

Request:
{
    "user_ids": [101, 102, 103]
}

Response (200):
{
    "assigned": [101, 102],
    "failed": [
        {
            "user_id": 103,
            "reason": "User not in organization"
        }
    ]
}
```

### 7.3 Get User License

```
GET /api/v1/users/:id/license

Response (200):
{
    "user_id": 101,
    "plan_type": "team",
    "license_expires_at": "2025-12-03T00:00:00Z",
    "subscription_id": 42,
    "assigned_by": {
        "user_id": 1,
        "email": "owner@company.com",
        "name": "John Doe"
    },
    "assigned_at": "2025-01-15T10:30:00Z"
}

OR (for free user):

{
    "user_id": 201,
    "plan_type": "free",
    "license_expires_at": null,
    "subscription_id": null
}
```

---

## 8. Testing Strategy

### 8.1 Unit Tests
- Plan limit calculations
- JWT claim generation/validation
- Assignment business logic
- Webhook signature verification

### 8.2 Integration Tests
- Full purchase flow (test mode)
- License assignment/revocation
- Webhook event processing
- Expiration handling

### 8.3 Manual Testing Checklist
- [ ] Purchase Team Monthly (test mode)
- [ ] Assign licenses to 5 users in same org
- [ ] Create second org owned by same user
- [ ] Assign licenses to users in second org (cross-org)
- [ ] Verify JWT contains correct plan info
- [ ] Trigger review as free user (should block at 3)
- [ ] Trigger review as team user (should be unlimited)
- [ ] Attempt to assign license to user in non-owned org (should fail)
- [ ] Revoke license, verify user downgraded
- [ ] Cancel subscription (end of cycle)
- [ ] Test webhook: subscription.activated
- [ ] Test webhook: payment.captured
- [ ] Test seat count update

---

## 9. Security Considerations

1. **Webhook Verification**: ALWAYS verify Razorpay signatures
2. **Authorization**: Only subscription owner can assign/revoke licenses
3. **Cross-Org Validation**: Verify subscription owner owns the target org before assignment
4. **JWT Validation**: Verify plan claims on sensitive operations
5. **Idempotency**: Handle duplicate webhook events
6. **Rate Limiting**: Protect payment endpoints
7. **Audit Logging**: Log all license changes with org_id for cross-org tracking

---

## 10. Deployment Checklist

### Pre-Production
- [ ] Create live Razorpay plans
- [ ] Set webhook URL in Razorpay dashboard
- [ ] Configure webhook secret in .env
- [ ] Run database migrations
- [ ] Test payment flow in test mode

### Production Launch
- [ ] Switch to live Razorpay keys
- [ ] Monitor webhook delivery
- [ ] Set up alerts for payment failures
- [ ] Enable subscription notifications
- [ ] Document support procedures

---

## 11. Future Enhancements

1. **Proration**: Handle mid-cycle upgrades/downgrades
2. **Trials**: 14-day free trial for Team plan
3. **Coupons**: Discount codes via Razorpay
4. **Invoicing**: Automatic invoice generation
5. **Usage Analytics**: Track feature usage by plan
6. **Self-Service**: Allow users to manage billing
7. **Enterprise Plan**: Custom pricing, SSO, dedicated support

---

## 12. Key Differences from Your Initial Design

### What Changed:
1. **Subscription Creation**: Returns Razorpay subscription object ready for checkout (not just ID)
2. **License Assignment**: Separate from subscription creation (owner assigns after purchase)
3. **User Plan Field**: Stored directly on users table (denormalized for JWT performance)
4. **Enforcement**: JWT-based (no DB lookup), with middleware guards
5. **Incremental Approach**: Start with enforcement (Phase 1), not purchasing

### What Stayed:
1. **First Principles**: Build from ground up ✅
2. **Available vs Applied Licenses**: Core concept maintained ✅
3. **Org Owner Control**: Only owners manage subscriptions ✅
4. **JWT Integration**: Plan enforcement at auth layer ✅
5. **No Fallbacks**: Explicit failures ✅

---

## Summary

This plan provides a complete, incremental path to implementing Razorpay subscriptions:

1. **Week 1**: Enforcement (make limits work)
2. **Week 2**: Razorpay integration (create subscriptions)
3. **Week 3**: License assignment (apply to users)
4. **Week 4**: UI (complete user flow)
5. **Week 5**: Lifecycle (renewals, expiry)
6. **Week 6**: Testing & polish

Each phase builds on the previous, with clear deliverables and testing checkpoints. The system is designed for reliability, security, and maintainability from day one.
