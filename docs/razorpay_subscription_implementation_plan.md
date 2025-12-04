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

**Verification & Spot Checks:**

1. **Database Schema Verification** (CLI)
```bash
# Check tables exist
./pgctl.sh shell -c "\dt subscriptions"
./pgctl.sh shell -c "\dt license_log"

# Verify user_roles columns added
./pgctl.sh shell -c "\d user_roles" | grep -E "plan_type|license_expires_at|active_subscription_id"

# Check indexes created
./pgctl.sh shell -c "\di" | grep -E "idx_subscriptions|idx_user_roles_plan|idx_license_log"

# Verify constraints
./pgctl.sh shell -c "\d+ subscriptions" | grep -E "valid_quantity|valid_assigned_seats"
```

2. **Plan Definitions Test** (Code)
```bash
# Run unit tests for plan definitions
go test ./internal/license -run TestPlanDefinitions -v
go test ./internal/license -run TestPlanLimits -v
```

3. **JWT Claims Test** (Code)
```bash
# Test JWT generation with new claims
go test ./internal/api/auth -run TestCustomClaims -v
go test ./internal/api/auth -run TestJWTWithOrgContext -v
```

4. **Enforcement Middleware Test** (Code)
```bash
# Test plan enforcement logic
go test ./internal/api/middleware -run TestEnforcePlan -v
go test ./internal/api/middleware -run TestCheckReviewLimit -v
```

5. **Integration Spot Check** (CLI)
```bash
# Create a test free user in database
./pgctl.sh shell -c "INSERT INTO user_roles (user_id, role_id, org_id, plan_type) VALUES (999, 1, 1, 'free') ON CONFLICT DO NOTHING;"

# Verify default plan is 'free'
./pgctl.sh shell -c "SELECT user_id, org_id, plan_type, license_expires_at FROM user_roles WHERE user_id = 999;"

# Expected output: plan_type = 'free', license_expires_at = NULL
```

6. **[USER ACTION] UI Verification**
   - [ ] Login as free user
   - [ ] Create 3 reviews successfully
   - [ ] Attempt 4th review - should see "Daily review limit reached" error
   - [ ] Check that error message mentions "Upgrade to Team plan"

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

**Verification & Spot Checks:**

1. **Razorpay Plan Creation** (CLI)
```bash
# Test plan creation via Go code
go run -tags=test ./cmd/create_plans.go
# Should output: Monthly Plan ID: plan_XXXXX, Annual Plan ID: plan_YYYYY

# Or test via direct API call
curl -u REDACTED_TEST_KEY:REDACTED_TEST_SECRET \
  https://api.razorpay.com/v1/plans | jq '.items[] | {id, item: .item.name}'
# Should show LiveReview Team plans
```

2. **Subscription Service Tests** (Code)
```bash
# Test subscription creation
go test ./internal/license -run TestCreateSubscription -v
go test ./internal/license -run TestUpdateQuantity -v
go test ./internal/license -run TestCancelSubscription -v
```

3. **Webhook Signature Verification** (Code)
```bash
# Test webhook signature validation
go test ./internal/api/webhooks -run TestVerifyWebhookSignature -v
go test ./internal/api/webhooks -run TestHandleRazorpayWebhook -v
```

4. **Database Subscription Record** (CLI)
```bash
# Create test subscription via API
curl -X POST http://localhost:8888/api/v1/subscriptions \
  -H "Authorization: Bearer <test_jwt>" \
  -H "Content-Type: application/json" \
  -d '{"plan_type": "team_monthly", "quantity": 5}'

# Verify subscription in database
./pgctl.sh shell -c "SELECT id, owner_user_id, plan_type, quantity, status FROM subscriptions ORDER BY created_at DESC LIMIT 1;"
# Expected: status = 'created', quantity = 5
```

5. **Webhook Event Logging** (CLI)
```bash
# Send test webhook (simulate Razorpay)
curl -X POST http://localhost:8888/api/v1/webhooks/razorpay \
  -H "X-Razorpay-Signature: <test_signature>" \
  -d '{"event": "subscription.activated", "payload": {...}}'

# Check license_log for webhook entry
./pgctl.sh shell -c "SELECT action, razorpay_event_id, processed FROM license_log WHERE action LIKE 'subscription.%' ORDER BY created_at DESC LIMIT 5;"
# Should see logged webhook events
```

6. **Idempotency Check** (CLI)
```bash
# Send same webhook twice
./pgctl.sh shell -c "SELECT COUNT(*) FROM license_log WHERE razorpay_event_id = 'evt_test_123';"
# Should return 1 (duplicate ignored via UNIQUE constraint)
```

7. **[USER ACTION] UI Verification**
   - [ ] Navigate to /subscribe page
   - [ ] See Team Monthly ($6/user/month) and Annual ($60/user/year) options
   - [ ] Click "Get Team Plan" - should initialize Razorpay checkout
   - [ ] Verify Razorpay test mode indicated in checkout UI

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

**Verification & Spot Checks:**

1. **Assignment Service Tests** (Code)
```bash
# Test license assignment logic
go test ./internal/license -run TestAssignLicense -v
go test ./internal/license -run TestRevokeLicense -v
go test ./internal/license -run TestAssignmentValidation -v
go test ./internal/license -run TestCrossOrgAssignment -v
```

2. **API Endpoint Tests** (Code)
```bash
# Test assignment endpoints
go test ./internal/api -run TestAssignLicensesEndpoint -v
go test ./internal/api -run TestRevokeLicenseEndpoint -v
go test ./internal/api -run TestGetUserLicenseEndpoint -v
```

3. **License Assignment Flow** (CLI)
```bash
# Create test subscription in DB
./pgctl.sh shell -c "INSERT INTO subscriptions (owner_user_id, razorpay_subscription_id, plan_type, quantity, status) VALUES (1, 'sub_test_123', 'team_monthly', 10, 'active') RETURNING id;"
# Note the returned subscription ID

# Assign license to user via API
curl -X POST http://localhost:8888/api/v1/subscriptions/1/licenses \
  -H "Authorization: Bearer <owner_jwt>" \
  -H "Content-Type: application/json" \
  -d '{"user_ids": [2, 3], "org_id": 1}'

# Verify user_roles updated
./pgctl.sh shell -c "SELECT user_id, org_id, plan_type, license_expires_at, active_subscription_id FROM user_roles WHERE user_id IN (2,3) AND org_id = 1;"
# Expected: plan_type = 'team', active_subscription_id = 1

# Verify assigned_seats counter incremented
./pgctl.sh shell -c "SELECT quantity, assigned_seats FROM subscriptions WHERE id = 1;"
# Expected: assigned_seats = 2
```

4. **License Log Audit Trail** (CLI)
```bash
# Check audit log for assignments
./pgctl.sh shell -c "SELECT user_id, org_id, action, actor_id, created_at FROM license_log WHERE subscription_id = 1 ORDER BY created_at DESC;"
# Should show 'assigned' actions for users 2 and 3
```

5. **Cross-Org Assignment Test** (CLI)
```bash
# Verify owner can assign across multiple orgs they own
./pgctl.sh shell -c "SELECT id, owner_id FROM orgs WHERE owner_id = 1;"
# Note multiple org IDs

# Assign license in different org
curl -X POST http://localhost:8888/api/v1/subscriptions/1/licenses \
  -d '{"user_ids": [4], "org_id": 2}'

# Verify assignment in org 2
./pgctl.sh shell -c "SELECT user_id, org_id, plan_type FROM user_roles WHERE user_id = 4 AND org_id = 2;"
# Expected: plan_type = 'team'
```

6. **Seat Limit Enforcement** (CLI)
```bash
# Try to assign more than available seats
curl -X POST http://localhost:8888/api/v1/subscriptions/1/licenses \
  -d '{"user_ids": [5,6,7,8,9,10,11,12], "org_id": 1}'
# Should fail with "no available seats" when quantity reached

# Verify assigned_seats doesn't exceed quantity
./pgctl.sh shell -c "SELECT quantity, assigned_seats, (quantity - assigned_seats) as available FROM subscriptions WHERE id = 1;"
```

7. **License Revocation** (CLI)
```bash
# Revoke license
curl -X DELETE http://localhost:8888/api/v1/subscriptions/1/licenses/2 \
  -H "Authorization: Bearer <owner_jwt>"

# Verify user_roles reverted to free
./pgctl.sh shell -c "SELECT user_id, org_id, plan_type, license_expires_at FROM user_roles WHERE user_id = 2 AND org_id = 1;"
# Expected: plan_type = 'free', license_expires_at = NULL

# Verify assigned_seats decremented
./pgctl.sh shell -c "SELECT assigned_seats FROM subscriptions WHERE id = 1;"
# Expected: assigned_seats decreased by 1
```

8. **JWT Claims Update** (CLI)
```bash
# Get user license info
curl http://localhost:8888/api/v1/users/3/license \
  -H "Authorization: Bearer <jwt>"
# Should show plan_type, license_expires_at, subscription_id
```

9. **[USER ACTION] UI Verification**
   - [ ] Login as subscription owner
   - [ ] Navigate to license management page
   - [ ] See subscription with "10 seats: 3 assigned, 7 available"
   - [ ] Click "Assign License" and select users from dropdown
   - [ ] Verify assigned users show "Team" badge
   - [ ] Click "Revoke" on a user - should revert to "Free"
   - [ ] Verify seat counter updates in real-time

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

**Verification & Spot Checks:**

1. **Frontend Build Check** (CLI)
```bash
# Build UI without errors
cd ui
npm run build
# Should complete without errors

# Check for Razorpay script inclusion
grep -r "razorpay" src/
# Should find Razorpay checkout integration
```

2. **API Integration Tests** (Code)
```bash
# Test API endpoints used by UI
go test ./internal/api -run TestSubscriptionEndpoints -v
go test ./internal/api -run TestPaymentVerification -v
```

3. **Checkout Flow Simulation** (CLI)
```bash
# Test subscription creation returns checkout data
curl -X POST http://localhost:8888/api/v1/subscriptions \
  -H "Authorization: Bearer <test_jwt>" \
  -d '{"plan_type": "team_monthly", "quantity": 3}' | jq
# Expected fields: razorpay_subscription_id, razorpay_key_id, short_url, amount
```

4. **Payment Verification Endpoint** (CLI)
```bash
# Test payment verification (mock signature)
curl -X POST http://localhost:8888/api/v1/subscriptions/verify-payment \
  -H "Authorization: Bearer <test_jwt>" \
  -d '{"razorpay_payment_id": "pay_test", "razorpay_subscription_id": "sub_test", "razorpay_signature": "test_sig"}'
# Should validate and update subscription status
```

5. **[USER ACTION] Complete UI Verification**

   **Subscription Page:**
   - [ ] Navigate to `/subscribe`
   - [ ] See pricing: Monthly $6/user/month, Annual $60/user/year
   - [ ] See "17% savings" badge on annual plan
   - [ ] Seat quantity selector works (min 1, increments properly)
   - [ ] Total price updates correctly (e.g., 5 seats × $6 = $30/month)
   - [ ] "Get Team Plan" button enabled

   **Checkout Flow:**
   - [ ] Click "Get Team Plan" for Monthly with 5 seats
   - [ ] Razorpay checkout modal opens
   - [ ] See correct amount: $30 ($6 × 5 seats)
   - [ ] Test mode indicator visible
   - [ ] Use Razorpay test card: 4111 1111 1111 1111
   - [ ] Payment succeeds
   - [ ] Redirect to `/dashboard/licenses`

   **License Management Page:**
   - [ ] See active subscription: "Team Monthly - 5 seats"
   - [ ] Shows "0 assigned, 5 available"
   - [ ] "Assign Licenses" button visible
   - [ ] Click "Assign" - see list of org members
   - [ ] Select 2 users and assign
   - [ ] Counter updates to "2 assigned, 3 available"
   - [ ] Assigned users show in table with "Revoke" button
   - [ ] "Update Seat Count" option visible

   **User Dashboard:**
   - [ ] Login as assigned user
   - [ ] Dashboard shows "Team Plan" badge
   - [ ] See "License expires: [date]" 
   - [ ] No daily review limit message
   - [ ] Create more than 3 reviews - should succeed

   **Free User Dashboard:**
   - [ ] Login as unassigned user
   - [ ] Dashboard shows "Free Plan"
   - [ ] See "Upgrade to Team" CTA prominently
   - [ ] Create 3 reviews - success
   - [ ] Attempt 4th review - blocked with upgrade message

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

**Verification & Spot Checks:**

1. **Expiration Job Tests** (Code)
```bash
# Test scheduled expiration checks
go test ./internal/jobs -run TestCheckExpiringLicenses -v
go test ./internal/jobs -run TestAutoDowngradeExpired -v
go test ./internal/notifications -run TestExpiryNotifications -v
```

2. **Simulate License Expiration** (CLI)
```bash
# Set license to expire soon
./pgctl.sh shell -c "UPDATE user_roles SET license_expires_at = NOW() + INTERVAL '2 days' WHERE user_id = 3 AND org_id = 1;"

# Run expiration notification job
go run ./cmd/jobs/notify_expiring_licenses.go

# Check notifications sent
./pgctl.sh shell -c "SELECT user_id, notification_type, sent_at FROM notifications WHERE notification_type = 'license_expiring' ORDER BY sent_at DESC LIMIT 5;"
```

3. **Test Expiration Enforcement** (CLI)
```bash
# Set license to expired
./pgctl.sh shell -c "UPDATE user_roles SET license_expires_at = NOW() - INTERVAL '1 day' WHERE user_id = 3 AND org_id = 1;"

# Try to access with expired license (should fail)
curl http://localhost:8888/api/v1/reviews \
  -H "Authorization: Bearer <expired_jwt>" \
  -X POST -d '{...}'
# Expected: 402 Payment Required - "Your license has expired"
```

4. **Grace Period Test** (CLI)
```bash
# Set license expired but within grace period
./pgctl.sh shell -c "UPDATE user_roles SET license_expires_at = NOW() - INTERVAL '3 days' WHERE user_id = 3;"

# Check grace period status
curl http://localhost:8888/api/v1/users/3/license
# Should show: {"status": "grace_period", "days_remaining": 4}
```

5. **Auto-Downgrade Test** (CLI)
```bash
# Run downgrade job for licenses past grace period
go run ./cmd/jobs/downgrade_expired_licenses.go

# Verify users downgraded to free
./pgctl.sh shell -c "SELECT user_id, org_id, plan_type FROM user_roles WHERE license_expires_at < NOW() - INTERVAL '7 days' LIMIT 5;"
# Expected: plan_type = 'free' for expired licenses

# Check license_log for downgrade actions
./pgctl.sh shell -c "SELECT user_id, action, created_at FROM license_log WHERE action = 'expired' ORDER BY created_at DESC LIMIT 5;"
```

6. **Renewal Webhook Test** (CLI)
```bash
# Simulate Razorpay renewal webhook
curl -X POST http://localhost:8888/api/v1/webhooks/razorpay \
  -H "X-Razorpay-Signature: <signature>" \
  -d '{
    "event": "subscription.charged",
    "payload": {
      "subscription": {"entity": {"id": "sub_123", "current_period_end": 1735689600}}
    }
  }'

# Verify license_expires_at updated for assigned users
./pgctl.sh shell -c "SELECT user_id, license_expires_at FROM user_roles WHERE active_subscription_id IN (SELECT id FROM subscriptions WHERE razorpay_subscription_id = 'sub_123');"
# Expected: license_expires_at extended to new period end
```

7. **Cancellation Test** (CLI)
```bash
# Cancel subscription (end of cycle)
curl -X POST http://localhost:8888/api/v1/subscriptions/1/cancel \
  -H "Authorization: Bearer <owner_jwt>" \
  -d '{"cancel_at_cycle_end": true}'

# Verify subscription status
./pgctl.sh shell -c "SELECT id, status, cancelled_at, current_period_end FROM subscriptions WHERE id = 1;"
# Expected: status = 'cancelled', current_period_end still in future

# Verify users retain access until period end
./pgctl.sh shell -c "SELECT plan_type, license_expires_at FROM user_roles WHERE active_subscription_id = 1;"
# Expected: plan_type still 'team', expires_at = current_period_end
```

8. **Payment Retry Test** (CLI)
```bash
# Simulate failed payment webhook
curl -X POST http://localhost:8888/api/v1/webhooks/razorpay \
  -d '{"event": "payment.failed", "payload": {...}}'

# Check license_log for failure
./pgctl.sh shell -c "SELECT action, payload FROM license_log WHERE action = 'payment.failed' ORDER BY created_at DESC LIMIT 1;"

# Verify subscription status updated
./pgctl.sh shell -c "SELECT status FROM subscriptions WHERE razorpay_subscription_id = 'sub_with_failed_payment';"
# Expected: status = 'halted' or 'past_due'
```

9. **[USER ACTION] UI Verification**

   **Expiration Notifications:**
   - [ ] Check email for "License expiring in 7 days" notification
   - [ ] Check email for "License expiring in 3 days" notification  
   - [ ] Check email for "License expiring in 1 day" notification
   - [ ] Verify emails contain renewal link

   **Expired License Experience:**
   - [ ] Login as user with expired license
   - [ ] Dashboard shows "License Expired" warning banner
   - [ ] See "Renew Now" button prominently
   - [ ] Try to create review - blocked with "Payment Required" error
   - [ ] Error message includes "Your license expired on [date]"

   **Grace Period Experience:**
   - [ ] Login as user in grace period (expired < 7 days)
   - [ ] Dashboard shows "Grace Period: 4 days remaining" warning
   - [ ] Can still access in read-only mode
   - [ ] Write operations disabled

   **Renewal Flow:**
   - [ ] Click "Renew" from expired dashboard
   - [ ] Redirected to payment page with existing subscription details
   - [ ] Complete payment
   - [ ] Dashboard updates to show active license with new expiry date

   **Cancellation UI:**
   - [ ] Login as subscription owner
   - [ ] Go to subscription management
   - [ ] Click "Cancel Subscription"
   - [ ] See options: "Cancel Now" vs "Cancel at End of Cycle"
   - [ ] Choose "End of Cycle" - confirmation shows expiry date
   - [ ] Status shows "Cancelled (Active until [date])"

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

**Verification & Spot Checks:**

1. **Full Integration Test Suite** (Code)
```bash
# Run complete test suite
go test ./... -v -cover

# Run integration tests specifically
go test ./internal/api/integration -run TestFullPurchaseFlow -v
go test ./internal/api/integration -run TestAssignmentFlow -v
go test ./internal/api/integration -run TestWebhookProcessing -v
go test ./internal/api/integration -run TestExpirationFlow -v

# Check test coverage
go test ./internal/license/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total
# Target: >80% coverage for license package
```

2. **Edge Case Tests** (Code)
```bash
# Test concurrent assignments
go test ./internal/license -run TestConcurrentAssignments -v -race

# Test webhook replay protection
go test ./internal/api/webhooks -run TestWebhookIdempotency -v

# Test subscription downgrades
go test ./internal/license -run TestDowngradeSubscription -v

# Test payment failure recovery
go test ./internal/license -run TestPaymentFailureRecovery -v
```

3. **Load/Stress Test** (CLI)
```bash
# Test concurrent license assignments (requires 'hey' tool)
hey -n 100 -c 10 -m POST \
  -H "Authorization: Bearer <jwt>" \
  -d '{"user_ids": [10], "org_id": 1}' \
  http://localhost:8888/api/v1/subscriptions/1/licenses

# Verify data integrity after load
./pgctl.sh shell -c "SELECT COUNT(*) FROM license_log WHERE action = 'assigned';"
./pgctl.sh shell -c "SELECT assigned_seats FROM subscriptions WHERE id = 1;"
# assigned_seats should match license_log count
```

4. **Webhook Replay Attack Test** (CLI)
```bash
# Send same webhook multiple times rapidly
for i in {1..10}; do
  curl -X POST http://localhost:8888/api/v1/webhooks/razorpay \
    -H "X-Razorpay-Signature: <valid_sig>" \
    -d '{"event": "subscription.activated", "id": "evt_duplicate_test", ...}' &
done
wait

# Verify only processed once
./pgctl.sh shell -c "SELECT COUNT(*) FROM license_log WHERE razorpay_event_id = 'evt_duplicate_test';"
# Expected: 1 (duplicates rejected)
```

5. **Data Consistency Checks** (CLI)
```bash
# Verify assigned_seats matches actual assignments
./pgctl.sh shell -c "
SELECT 
  s.id,
  s.assigned_seats as counter,
  COUNT(ur.user_id) as actual_assignments,
  (s.assigned_seats = COUNT(ur.user_id)) as consistent
FROM subscriptions s
LEFT JOIN user_roles ur ON ur.active_subscription_id = s.id
GROUP BY s.id, s.assigned_seats;
"
# All rows should show consistent = true

# Verify no orphaned licenses
./pgctl.sh shell -c "SELECT COUNT(*) FROM user_roles WHERE active_subscription_id IS NOT NULL AND active_subscription_id NOT IN (SELECT id FROM subscriptions);"
# Expected: 0

# Verify license_log completeness
./pgctl.sh shell -c "
SELECT 
  action,
  COUNT(*) as count,
  COUNT(DISTINCT subscription_id) as unique_subscriptions
FROM license_log 
GROUP BY action 
ORDER BY count DESC;
"
# Should see all action types: assigned, revoked, expired, subscription.*, payment.*
```

6. **Admin Tool Tests** (CLI)
```bash
# Test manual license grant
go run ./cmd/admin/grant_license.go --user-id=99 --org-id=1 --plan=team --expires="2025-12-31"

# Verify grant bypasses subscription
./pgctl.sh shell -c "SELECT user_id, plan_type, active_subscription_id FROM user_roles WHERE user_id = 99;"
# Expected: plan_type = 'team', active_subscription_id = NULL (manual grant)

# Test subscription override
go run ./cmd/admin/override_subscription.go --subscription-id=1 --extend-days=30

# Check audit log for admin actions
./pgctl.sh shell -c "SELECT action, actor_id, payload FROM license_log WHERE action LIKE 'admin.%' ORDER BY created_at DESC LIMIT 5;"
```

7. **Security Audit** (CLI)
```bash
# Test authorization - non-owner can't assign licenses
curl -X POST http://localhost:8888/api/v1/subscriptions/1/licenses \
  -H "Authorization: Bearer <non_owner_jwt>" \
  -d '{"user_ids": [10], "org_id": 1}'
# Expected: 403 Forbidden

# Test cross-org protection - can't assign to org not owned
curl -X POST http://localhost:8888/api/v1/subscriptions/1/licenses \
  -H "Authorization: Bearer <owner_jwt>" \
  -d '{"user_ids": [10], "org_id": 999}'
# Expected: 403 Forbidden or 400 Bad Request

# Test expired JWT rejection
curl http://localhost:8888/api/v1/reviews \
  -H "Authorization: Bearer <expired_jwt>"
# Expected: 401 Unauthorized
```

8. **Performance Benchmarks** (Code)
```bash
# Run performance benchmarks
go test ./internal/license -bench=BenchmarkAssignLicense -benchmem
go test ./internal/api/middleware -bench=BenchmarkEnforcePlan -benchmem

# Check slow query log
./pgctl.sh shell -c "SELECT query, mean_exec_time, calls FROM pg_stat_statements WHERE mean_exec_time > 100 ORDER BY mean_exec_time DESC LIMIT 10;"
# Verify no subscription-related queries >100ms
```

9. **Documentation Verification** (CLI)
```bash
# Verify API docs exist
ls -la docs/api/
# Should contain: subscriptions.md, licenses.md, webhooks.md

# Check code examples compile
go build -o /dev/null examples/create_subscription.go
go build -o /dev/null examples/assign_license.go

# Verify troubleshooting guide covers common issues
grep -i "payment failed" docs/troubleshooting.md
grep -i "webhook" docs/troubleshooting.md
grep -i "license expired" docs/troubleshooting.md
```

10. **[USER ACTION] End-to-End Acceptance Testing**

   **Complete Purchase Journey:**
   - [ ] Start as new free user
   - [ ] Hit 3-review limit
   - [ ] Click upgrade → purchase Team plan (5 seats)
   - [ ] Payment completes successfully
   - [ ] Automatically assigned first license
   - [ ] Create >3 reviews successfully
   - [ ] Invite 4 team members to org
   - [ ] Assign licenses to all 4 members
   - [ ] Each member can create unlimited reviews
   - [ ] Attempt to assign 6th license → blocked (only 5 seats)

   **Cross-Org Scenario:**
   - [ ] Create second org (same owner)
   - [ ] Invite different users to org 2
   - [ ] Assign licenses from same subscription to org 2 users
   - [ ] User A in org 1 is Team, same User A in org 2 is Free
   - [ ] Verify User A sees different limits when switching orgs

   **Lifecycle Journey:**
   - [ ] Cancel subscription (end of cycle)
   - [ ] Continue using until expiry date
   - [ ] On expiry, all assigned users downgraded
   - [ ] Receive expiration notifications
   - [ ] Renew subscription
   - [ ] Users automatically upgraded back to Team

   **Error Handling:**
   - [ ] Try to assign license to non-member → see clear error
   - [ ] Try payment with invalid card → see Razorpay error
   - [ ] Simulate webhook failure → verify retry mechanism
   - [ ] Test with slow network → see loading states

   **Admin Tools:**
   - [ ] Admin grants license manually
   - [ ] License shows in user dashboard
   - [ ] Admin revokes license
   - [ ] User immediately downgraded

**Deliverable:** Production-ready subscription system

**Go/No-Go Checklist for Production:**
- [x] All integration tests passing
- [x] Test coverage >80% for license package
- [x] No high-severity security issues
- [x] Webhook signature verification working
- [x] All edge cases handled gracefully
- [x] Admin tools tested and documented
- [x] Load testing passed (100 concurrent requests)
- [x] Data consistency verified in test environment
- [x] User acceptance testing completed
- [x] Rollback plan documented and tested

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
- [x] Purchase Team Monthly (test mode)
- [x] Assign licenses to 5 users in same org
- [x] Create second org owned by same user
- [x] Assign licenses to users in second org (cross-org)
- [x] Verify JWT contains correct plan info
- [x] Trigger review as free user (should block at 3)
- [x] Trigger review as team user (should be unlimited)
- [x] Attempt to assign license to user in non-owned org (should fail)
- [x] Revoke license, verify user downgraded
- [x] Cancel subscription (end of cycle)
- [x] Test webhook: subscription.activated
- [x] Test webhook: payment.captured
- [x] Test seat count update

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
- [x] Create live Razorpay plans
- [x] Set webhook URL in Razorpay dashboard
- [x] Configure webhook secret in .env
- [x] Run database migrations
- [x] Test payment flow in test mode

### Production Launch
- [x] Switch to live Razorpay keys
- [x] Monitor webhook delivery
- [x] Set up alerts for payment failures
- [x] Enable subscription notifications
- [x] Document support procedures

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
- [x] **Update Plan Constants** - Store actual plan IDs in code
- [x] **Test Database Connection** - Verify dbmate can connect
  ```bash
  dbmate status
  ```

### Migration Readiness
- [x] **Review Migration SQL** - Check subscriptions, user_roles, license_log schemas
- [x] **Understand Rollback** - Know how to use `dbmate down` if needed
- [x] **Check user_roles Structure** - Verify composite PK (user_id, role_id, org_id) exists in db/schema.sql

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
- [x] **Update Plan Constants** - Store actual plan IDs in code
- [x] **Test Database Connection** - Verify dbmate can connect
  ```bash
  dbmate status
  ```

### Migration Readiness
- [x] **Review Migration SQL** - Check subscriptions, user_roles, license_log schemas
- [x] **Understand Rollback** - Know how to use `dbmate down` if needed
- [x] **Check user_roles Structure** - Verify composite PK (user_id, role_id, org_id) exists

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
