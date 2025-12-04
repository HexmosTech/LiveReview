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
- **License Owner**: User who purchases subscription
- **Available Licenses**: Total seats purchased (quantity in subscription)
- **Applied Licenses**: Licenses assigned to specific user-org pairs via `user_roles` table
- **Unassigned Licenses**: Available - Applied

**Key Insight:** The same user can have different plans in different orgs (stored in `user_roles.plan_type`)

**License States:**
```
Free Plan → Purchase → Active Team License → Expired → Back to Free Plan
                ↓                               ↓
         Apply to Users                  Users lose access
```

**Plan Enforcement:**

Enforced at JWT level using the user's plan in the **current org context**. The JWT contains:

**JWT Claims for Plan Enforcement:**
1. **`currentOrgID`** (int64): Which org the user is currently acting in
   - **Purpose**: Determines which user_role record to use for plan lookup
   - **Example**: User switches org → new JWT issued with new orgID

2. **`planType`** (string): Plan for current org (`"free"` | `"team"` | `"enterprise"`)
   - **Purpose**: Cached from `user_roles.plan_type` for the current org
   - **Example**: User is `team` in Org A, `free` in Org B

3. **`licenseExpiresAt`** (int64 | null): When license expires in current org
   - **Purpose**: Instant expiry checks without DB lookup
   - **Value**: From `user_roles.license_expires_at` for current org

4. **`subscriptionID`** (int64 | null): Subscription providing license in current org
   - **Purpose**: Operational tracking only
   - **Value**: From `user_roles.active_subscription_id`

**Examples:**
- Free user in Org A: `{currentOrgID: 1, planType: "free", licenseExpiresAt: null, subscriptionID: null}`
- Team user in Org A: `{currentOrgID: 1, planType: "team", licenseExpiresAt: 1735689600, subscriptionID: 42}`
- Same user switches to Org B (where they're free): `{currentOrgID: 2, planType: "free", licenseExpiresAt: null, subscriptionID: null}`

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

### 2.1 Design Principles

**Core Principle: Immutable Audit Log First**

Every license state change follows this pattern:
1. Write to audit table (immutable log entry)
2. Update state table (current state)
3. Both in same transaction (atomic safety)

**Why this matters:**
- Audit table shows **how** we got to current state (timeline of changes)
- State table shows **what** the current state is (fast lookups)
- If transaction fails, neither update happens (consistency)

**Concrete Implementation:**

```sql
BEGIN TRANSACTION;
  -- Step 1: Log the action (immutable)
  INSERT INTO license_log (subscription_id, user_id, org_id, action, actor_id, timestamp)
  VALUES (42, 101, 5, 'assigned', 1, NOW());
  
  -- Step 2: Update current state (in user_roles table)
  UPDATE user_roles 
  SET plan_type = 'team', 
      license_expires_at = '2025-12-31',
      active_subscription_id = 42
  WHERE user_id = 101 AND org_id = 5;
COMMIT;

-- Later, on revocation:
BEGIN TRANSACTION;
  -- Step 1: Log the revocation
  INSERT INTO license_log (subscription_id, user_id, org_id, action, actor_id, timestamp)
  VALUES (42, 101, 5, 'revoked', 1, NOW());
  
  -- Step 2: Update current state (revert to free)
  UPDATE user_roles 
  SET plan_type = 'free',
      license_expires_at = NULL,
      active_subscription_id = NULL
  WHERE user_id = 101 AND org_id = 5;
COMMIT;
```

**Table Categories:**
1. **State Tables**: `subscriptions`, `user_roles` (extended) - Current state, frequently updated
2. **Audit Log Table**: `license_log` - Immutable log for ALL actions (license + payment), append-only, never updated
3. **Reference Tables**: `organizations`, `users`, `roles` - Stable data

---

### 2.2 Core Entities & Relationships

**Entity Model:**

```
┌─────────────────┐
│     users       │ (existing table)
│  - id           │
│  - email        │
│  - plan_type    │◄─────────────┐ (synced on assignment)
│  - license_     │              │
│   expires_at    │              │
└─────────────────┘              │
                                 │
┌─────────────────┐              │
│ organizations   │ (existing)   │
│  - id           │              │
│  - owner_id     │              │
└─────────────────┘              │
         ▲                       │
         │                       │
         │ owns                  │
         │                       │
┌─────────────────┐              │
│ subscriptions   │              │ updates user
│  - id           │              │ in same txn
│  - owner_user_id├──────────────┤
│  - org_id       │ (billing org)│
│  - quantity     │              │
│  - status       │              │
└────────┬────────┘              │
         │                       │
         │                       │
         │
         │ every change
         │ logged to
         ▼
┌──────────────────┐
│  license_log     │ (UNIFIED AUDIT LOG)
│  - subscription_id│
│  - user_id       │ (null for payment events)
│  - org_id        │ (null for payment events)
│  - action        │ ('assigned'|'revoked'|'payment.captured'|'subscription.activated')
│  - actor_id      │ (null for webhook events)
│  - razorpay_     │
│    event_id      │ (null for license actions)
│  - payload       │ (JSONB - details vary by action)
│  - timestamp     │
└──────────────────┘
```

**Key Relationships:**

1. **Subscription → Owner (users.id)**: Who purchased and can manage licenses
2. **User Roles → User, Org, Role**: Existing junction table linking users to orgs with roles
3. **License Assignment = User Role + Plan Fields**: License is "applied" by adding plan fields to user_roles record

**Cross-Org Example:**
- Alice owns Org A and Org B
- Alice buys subscription (owner: Alice, quantity: 10)
- Bob is a member of both Org A and Org B (two `user_roles` records)
- Alice assigns license to Bob in Org A → `UPDATE user_roles SET plan_type='team' WHERE user_id=Bob AND org_id=A`
- Bob remains free in Org B → `user_roles.plan_type='free' WHERE user_id=Bob AND org_id=B`
- Result: Bob sees Team features only when working in Org A

---

### 2.3 Table Schemas

#### 2.3.1 Subscriptions Table

**Purpose**: Current state of all subscriptions (what is active now)

**Key Fields:**
- `status`: Current lifecycle state - updated when state changes, not deleted
- `quantity`: Enforcement boundary (can't assign more licenses than this)
- `assigned_seats`: Denormalized counter - updated when licenses assigned/revoked (for fast queries)
- `razorpay_data`: JSONB for Razorpay's 20+ fields (avoid migrations when they add fields)

**Migration File:** `db/migrations/YYYYMMDDHHMMSS_add_subscription_tables.sql`

```sql
-- migrate:up
-- Subscriptions: Track Razorpay subscriptions owned by users
-- Licenses can be assigned to any user in any org owned by the subscription owner
CREATE TABLE subscriptions (
    id BIGSERIAL PRIMARY KEY,
    
    -- Razorpay Integration
    razorpay_subscription_id VARCHAR(255) UNIQUE NOT NULL,
    razorpay_plan_id VARCHAR(255) NOT NULL,
    
    -- Ownership
    owner_user_id BIGINT NOT NULL REFERENCES users(id),  -- User who purchased, can assign across all their orgs
    
    -- Subscription Details
    plan_type VARCHAR(50) NOT NULL, -- 'team_monthly' | 'team_annual'
    quantity INT NOT NULL, -- number of seats purchased
    assigned_seats INT DEFAULT 0 NOT NULL, -- number of seats currently assigned (denormalized counter)
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
    
    CONSTRAINT valid_quantity CHECK (quantity > 0),
    CONSTRAINT valid_assigned_seats CHECK (assigned_seats >= 0 AND assigned_seats <= quantity)
);

CREATE INDEX idx_subscriptions_owner ON subscriptions(owner_user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_razorpay ON subscriptions(razorpay_subscription_id);

-- migrate:down
DROP TABLE IF EXISTS subscriptions CASCADE;
```

**Index Justification:**
- `owner_user_id`: List all subscriptions owned by a user (management UI)
- `status`: Filter active vs expired subscriptions (cleanup jobs)
- `razorpay_subscription_id`: Lookup by Razorpay ID on webhook events (critical path)

---

#### 2.3.2 User Roles Table (Extended for Licensing)

**Purpose**: Existing junction table linking users to orgs - EXTENDED to store per-org license info

**Key Fields (NEW):**
- `plan_type`: User's plan in THIS org ('free'|'team' - defaults to 'free')
- `license_expires_at`: When license expires in THIS org (NULL for free)
- `active_subscription_id`: Which subscription provides license in THIS org

**Why extend user_roles instead of separate table?**
- User-org relationship already exists (users can be in multiple orgs)
- Natural place to store "what plan does this user have in this org?"
- Same user can have different plans in different orgs
- No additional joins needed for org context queries

```sql
-- migrate:up
-- Extend existing user_roles table (composite PK: user_id, role_id, org_id)
ALTER TABLE user_roles 
  ADD COLUMN plan_type VARCHAR(50) DEFAULT 'free' NOT NULL,
  ADD COLUMN license_expires_at TIMESTAMP WITH TIME ZONE,
  ADD COLUMN active_subscription_id BIGINT REFERENCES subscriptions(id);

CREATE INDEX idx_user_roles_plan_type ON user_roles(plan_type);
CREATE INDEX idx_user_roles_license_expires ON user_roles(license_expires_at) 
  WHERE license_expires_at IS NOT NULL;
CREATE INDEX idx_user_roles_subscription ON user_roles(active_subscription_id)
  WHERE active_subscription_id IS NOT NULL;

-- migrate:down
DROP INDEX IF EXISTS idx_user_roles_subscription;
DROP INDEX IF EXISTS idx_user_roles_license_expires;
DROP INDEX IF EXISTS idx_user_roles_plan_type;
ALTER TABLE user_roles 
  DROP COLUMN IF EXISTS active_subscription_id,
  DROP COLUMN IF EXISTS license_expires_at,
  DROP COLUMN IF EXISTS plan_type;
```

**Assignment Pattern:**
```sql
-- Assign license to user in specific org
UPDATE user_roles
SET plan_type = 'team',
    license_expires_at = '2025-12-31',
    active_subscription_id = 42
WHERE user_id = 101 AND org_id = 5;

-- Same user in different org remains free
SELECT plan_type FROM user_roles WHERE user_id = 101 AND org_id = 6;
-- Returns: 'free'
```

---

#### 2.3.3 License Log Table (Unified Audit Log)

**Purpose**: Single immutable audit log for ALL actions (license changes + payment events)

**Audit-First Pattern Examples:**

```sql
-- License Assignment
BEGIN TRANSACTION;
  -- Step 1: Log the action
  INSERT INTO license_log (subscription_id, user_id, org_id, action, actor_id, payload)
  VALUES (42, 101, 5, 'assigned', 1, '{"quantity": 10}');
  
  -- Step 2: Update state (in user_roles table)
  UPDATE user_roles
  SET plan_type = 'team', 
      license_expires_at = '2025-12-31',
      active_subscription_id = 42
  WHERE user_id = 101 AND org_id = 5;
COMMIT;

-- Payment Webhook
BEGIN TRANSACTION;
  -- Step 1: Log the webhook
  INSERT INTO license_log (subscription_id, action, razorpay_event_id, payload, processed)
  VALUES (42, 'subscription.activated', 'evt_ABC123', '{"razorpay_data": {...}}', false);
  
  -- Step 2: Update subscription state
  UPDATE subscriptions SET status = 'active', activated_at = NOW()
  WHERE id = 42;
COMMIT;

-- Later: Mark webhook as processed (only field we update)
UPDATE license_log SET processed = true, processed_at = NOW()
WHERE id = ?;
```

**Why single table for both?**
- Complete timeline of everything that happened to a subscription
- Simpler queries: one table to check for entire history
- Unified idempotency handling

```sql
-- migrate:up
-- License Log: Unified audit trail for license actions and payment events
CREATE TABLE license_log (
    id BIGSERIAL PRIMARY KEY,
    
    -- Relationships (nullable for payment-only events)
    subscription_id BIGINT REFERENCES subscriptions(id),
    user_id BIGINT REFERENCES users(id),  -- null for payment events
    org_id BIGINT REFERENCES organizations(id),  -- null for payment events
    
    -- Action Details
    action VARCHAR(100) NOT NULL,  -- 'assigned'|'revoked'|'expired'|'subscription.activated'|'payment.captured'|etc.
    actor_id BIGINT REFERENCES users(id),  -- null for webhook events
    
    -- Razorpay Integration (for payment events)
    razorpay_event_id VARCHAR(255) UNIQUE,  -- null for license actions, unique for webhooks
    
    -- Event Data
    payload JSONB,  -- flexible storage for action-specific data
    
    -- Processing Status (for webhook events)
    processed BOOLEAN DEFAULT TRUE,  -- false only for unprocessed webhooks
    processed_at TIMESTAMP,
    error_message TEXT,
    
    -- Timestamp
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_license_log_subscription ON license_log(subscription_id);
CREATE INDEX idx_license_log_user ON license_log(user_id);
CREATE INDEX idx_license_log_action ON license_log(action);
CREATE INDEX idx_license_log_processed ON license_log(processed) WHERE processed = false;
CREATE INDEX idx_license_log_razorpay ON license_log(razorpay_event_id) WHERE razorpay_event_id IS NOT NULL;

-- migrate:down
DROP TABLE IF EXISTS license_log CASCADE;
```

**Index Justification:**
- `subscription_id`: Complete history for a subscription (most common query)
- `user_id`: Timeline of license changes for a user
- `action`: Filter by event type (all assignments, all payment failures, etc.)
- `processed WHERE false`: Find unprocessed webhooks (background job)
- `razorpay_event_id WHERE NOT NULL`: Partial index for webhook deduplication (saves space)

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
    
    // Plan & License Information (org-specific)
    CurrentOrgID     int64     `json:"current_org_id"`     // Which org user is acting in
    PlanType         string    `json:"plan_type"`          // Plan in current org
    LicenseExpiresAt *int64    `json:"license_expires_at"` // Expiry in current org
    SubscriptionID   *int64    `json:"subscription_id"`    // Subscription for current org
    
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
// Assign licenses to users in specific org
type AssignLicensesRequest struct {
    UserIDs []int64 `json:"user_ids" validate:"required,min=1"`
    OrgID   int64   `json:"org_id" validate:"required"`  // Which org to assign licenses in
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
   - Create migration using dbmate: `dbmate new add_subscription_tables`
   - Write migration SQL for:
     * CREATE TABLE subscriptions
     * ALTER TABLE user_roles (add plan_type, license_expires_at, active_subscription_id)
     * CREATE TABLE license_log
   - Apply migration: `dbmate up`
   - Verify schema changes

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

**Migration Files Location:** `db/migrations/`

**Migration Commands:**
```bash
# Create new migration
dbmate new add_subscription_tables

# Apply migrations
dbmate up

# Rollback if needed
dbmate down

# Check migration status
dbmate status
```

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
   - Implement AssignLicense (updates user_roles)
   - Implement RevokeLicense (reverts user_roles to free)
   - Implement ListAssignedUsers (queries user_roles by subscription_id)

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

### 6.1 Razorpay Configuration (from payment.go)

**Credentials:**
```go
// Test Mode (Already Configured)
RAZORPAY_TEST_ACCESS_KEY := "REDACTED_TEST_KEY"
RAZORPAY_TEST_SECRET_KEY := "REDACTED_TEST_SECRET"

// Live Mode
RAZORPAY_ACCESS_KEY := "REDACTED_LIVE_KEY"
RAZORPAY_SECRET_KEY := "REDACTED_LIVE_SECRET"
```

**Plan Creation (internal/license/payment/payment.go):**
```go
// Create plans programmatically
monthlyPlan, err := CreatePlan("test", "monthly")
// Returns plan with ID to store in config
// Price: 600 cents ($6/month)

annualPlan, err := CreatePlan("test", "yearly")
// Returns plan with ID to store in config  
// Price: 6000 cents ($60/year, 17% discount)
```

**Store Plan IDs After Creation:**
```go
const (
    // Test Mode - Create these first via CreatePlan()
    TeamMonthlyTestPlanID = "plan_XXXXX"  // Get from CreatePlan() response
    TeamAnnualTestPlanID  = "plan_YYYYY"  // Get from CreatePlan() response
    
    // Live Mode - Create when going to production
    TeamMonthlyLivePlanID = "plan_ZZZZZ"
    TeamAnnualLivePlanID  = "plan_WWWWW"
)
```

### 6.2 Checkout Integration (UI)

```typescript
// ui/src/pages/Subscribe.tsx
const handleCheckout = async (planType: string, quantity: number) => {
    // 1. Create subscription via API
    const response = await fetch('/api/v1/subscriptions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plan_type: planType, quantity })
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
    
    // Log to license_log immediately (idempotency check via razorpay_event_id unique constraint)
    _, err := db.Exec(`
        INSERT INTO license_log (subscription_id, action, razorpay_event_id, payload, processed)
        VALUES ($1, $2, $3, $4, false)
        ON CONFLICT (razorpay_event_id) DO NOTHING
    `, event.SubscriptionID, event.Type, event.ID, event.Payload)
    
    if err != nil {
        return err  // duplicate event, safely ignored
    }
    
    // Process event asynchronously
    go processWebhookEvent(event)
    
    return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
```

### 6.4 Assignment Logic

```go
// internal/license/assignment_service.go
func (s *AssignmentService) AssignLicense(subscriptionID, userID, orgID, assignedByUserID int64) error {
    tx, _ := s.db.Begin()
    defer tx.Rollback()
    
    // 1. Check subscription exists and has available seats
    sub, err := s.getSubscription(tx, subscriptionID)
    if err != nil {
        return err
    }
    
    // Check if seats are available (using denormalized counter)
    if sub.AssignedSeats >= sub.Quantity {
        return errors.New("no available seats")
    }
    
    // 2. Verify assigner is the subscription owner
    if assignedByUserID != sub.OwnerUserID {
        return errors.New("only subscription owner can assign licenses")
    }
    
    // 3. Verify user is a member of the target org and owner owns that org
    var count int
    err = tx.QueryRow(`
        SELECT COUNT(*) FROM user_roles ur
        JOIN orgs o ON ur.org_id = o.id
        WHERE ur.user_id = $1 AND ur.org_id = $2 AND o.owner_id = $3
    `, userID, orgID, assignedByUserID).Scan(&count)
    
    if err != nil || count == 0 {
        return errors.New("user must be in org owned by subscription owner")
    }
    
    // 4. Update user_roles with license (tracks where the license is being used)
    result, err := tx.Exec(`
        UPDATE user_roles
        SET plan_type = 'team',
            license_expires_at = $1,
            active_subscription_id = $2
        WHERE user_id = $3 AND org_id = $4
    `, sub.CurrentPeriodEnd, subscriptionID, userID, orgID)
    
    if err != nil {
        return err
    }
    
    rowsAffected, _ := result.RowsAffected()
    if rowsAffected == 0 {
        return errors.New("user not found in org")
    }
    
    // 5. Increment assigned_seats counter
    _, err = tx.Exec(`
        UPDATE subscriptions
        SET assigned_seats = assigned_seats + 1
        WHERE id = $1
    `, subscriptionID)
    
    if err != nil {
        return err
    }
    
    tx.Commit()
    
    // 6. Invalidate user's existing JWTs for this org (optional - force re-login)
    s.invalidateUserTokensForOrg(userID, orgID)
    
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
    "quantity": 10
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

## 12. Pre-Implementation Checklist

### Environment Setup
- [x] **Razorpay Test Account** - Credentials configured in payment.go
  - Access Key: `REDACTED_TEST_KEY`
  - Secret Key: `REDACTED_TEST_SECRET`
- [x] **PostgreSQL Database** - Running and accessible
- [x] **Current Schema** - Available in db/schema.sql
- [x] **Migration Tool** - dbmate installed and ready

### Before Starting Implementation
- [ ] **Backup Database** - Create snapshot before migrations
  ```bash
  ./pgctl.sh shell -c "pg_dump livereview > backup_$(date +%Y%m%d_%H%M%S).sql"
  ```
- [ ] **Create Razorpay Plans** - Run CreatePlan() to get plan IDs
  ```go
  monthlyPlan, _ := CreatePlan("test", "monthly")
  annualPlan, _ := CreatePlan("test", "yearly")
  // Store monthlyPlan.ID and annualPlan.ID in constants
  ```
- [ ] **Update Plan Constants** - Store actual plan IDs in code
- [ ] **Test Database Connection** - Verify dbmate can connect
  ```bash
  dbmate status
  ```

### Migration Readiness
- [ ] **Review Migration SQL** - Check subscriptions, user_roles, license_log schemas
- [ ] **Understand Rollback** - Know how to use `dbmate down` if needed
- [ ] **Check user_roles Structure** - Verify composite PK (user_id, role_id, org_id) exists in db/schema.sql

**Ready to proceed?** Start with Phase 1, step 1: Create migration with `dbmate new add_subscription_tables`

---

## 13. Key Differences from Initial Design

### What Changed:
1. **Subscription Creation**: Returns Razorpay subscription object ready for checkout (not just ID)
2. **License Assignment**: Separate from subscription creation (owner assigns after purchase)
3. **User Plan Storage**: Extended user_roles table instead of separate license_assignments table
4. **Enforcement**: JWT-based (no DB lookup), with org-specific claims
5. **Incremental Approach**: Start with enforcement (Phase 1), not purchasing
6. **Migration Tool**: Using dbmate with migrate:up/migrate:down syntax

### What Stayed:
1. **First Principles**: Build from ground up ✅
2. **Available vs Applied Licenses**: Core concept maintained ✅
3. **Org Owner Control**: Only owners manage subscriptions ✅
4. **JWT Integration**: Plan enforcement at auth layer ✅
5. **No Fallbacks**: Explicit failures ✅

---

## 12. Pre-Implementation Checklist

### Environment Setup
- [x] **Razorpay Test Account** - Credentials configured in payment.go
  - Access Key: `REDACTED_TEST_KEY`
  - Secret Key: `REDACTED_TEST_SECRET`
- [x] **PostgreSQL Database** - Running and accessible
- [x] **Current Schema** - Available in db/schema.sql
- [x] **Migration Tool** - dbmate installed and ready

### Before Starting Implementation
- [ ] **Backup Database** - Create snapshot before migrations
  ```bash
  ./pgctl.sh shell -c "pg_dump livereview > backup_$(date +%Y%m%d_%H%M%S).sql"
  ```
- [ ] **Create Razorpay Plans** - Run CreatePlan() to get plan IDs
  ```go
  monthlyPlan, _ := CreatePlan("test", "monthly")
  annualPlan, _ := CreatePlan("test", "yearly")
  // Store monthlyPlan.ID and annualPlan.ID in constants
  ```
- [ ] **Update Plan Constants** - Store actual plan IDs in code
- [ ] **Test Database Connection** - Verify dbmate can connect
  ```bash
  dbmate status
  ```

### Migration Readiness
- [ ] **Review Migration SQL** - Check subscriptions, user_roles, license_log schemas
- [ ] **Understand Rollback** - Know how to use `dbmate down` if needed
- [ ] **Check user_roles Structure** - Verify composite PK (user_id, role_id, org_id) exists

**Ready to proceed?** Start with Phase 1, step 1: Create migration with `dbmate new add_subscription_tables`

---

## Summary

This plan provides a complete, incremental path to implementing Razorpay subscriptions with all prerequisites ready:

**Environment Status:**
- ✅ Razorpay test credentials configured (payment.go)
- ✅ PostgreSQL running with current schema (db/schema.sql)
- ✅ Migration tool ready (dbmate)

**Implementation Path:**

1. **Week 1**: Database migrations (dbmate) + Enforcement (JWT, middleware, limits)
2. **Week 2**: Razorpay integration (CreatePlan, subscriptions, webhooks)
3. **Week 3**: License assignment (user_roles updates, cross-org support)
4. **Week 4**: UI (checkout flow, license management)
5. **Week 5**: Lifecycle (renewals, expiry, grace periods)
6. **Week 6**: Testing & polish (integration tests, edge cases)

**First Steps:**
1. Backup database: `./pgctl.sh shell -c "pg_dump livereview > backup.sql"`
2. Create migration: `dbmate new add_subscription_tables`
3. Write migration SQL (subscriptions, user_roles extension, license_log)
4. Apply migration: `dbmate up`
5. Create Razorpay plans: Run `CreatePlan("test", "monthly")` and `CreatePlan("test", "yearly")`

Each phase builds on the previous, with clear deliverables and testing checkpoints. The system is designed for reliability, security, and maintainability from day one.
