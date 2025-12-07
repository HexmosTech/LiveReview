# Plan: Subscription-Based License Enforcement Transition

This plan details the migration from file-based JWT licenses to flexible subscription-based enforcement for LiveReview, supporting both cloud (with usage-based plans) and self-hosted (with existing JWT file) deployments.

## Background

### Current Architecture

**Cloud Authentication Flow (JWT Replacement):**
1. User authenticates with Hexmos (external SSO via fw-parse)
2. Frontend receives fw-parse JWT in [ui/src/pages/Auth/Cloud.tsx](../ui/src/pages/Auth/Cloud.tsx)
3. Frontend calls `/api/v1/auth/ensure-cloud-user` with fw-parse JWT
4. Backend validates fw-parse JWT using `CLOUD_JWT_SECRET` in [internal/api/auth/handlers.go](../internal/api/auth/handlers.go) `EnsureCloudUser` handler
5. Backend provisions/finds user, creates org, assigns role
6. Backend generates **NEW LiveReview-native JWT** with `CreateTokenPair` from [internal/api/auth/token_service.go](../internal/api/auth/token_service.go)
7. Frontend receives LiveReview JWT and uses it for all subsequent API calls

**Key Insight:** The JWT replacement already happens at step 6. We need to extend the JWT claims generated there to include subscription information.

**Self-Hosted Flow:**
- Uses file-based JWT license validated in [internal/license](../internal/license)
- License server URL: `https://parse.apps.hexmos.com` (hardcoded in validator)
- Displays license status in UI (green banner, settings page)

### Deployment Mode Detection

The system uses `LIVEREVIEW_IS_CLOUD` environment variable (already in [.env](../.env)):
- Set to `true` for cloud deployment
- Set to `false` for self-hosted deployment
- Read in [internal/api/server.go](../internal/api/server.go) via `getEnvBool`

## Implementation Plan

### Phase 1: Extend JWT Claims for Subscription Data

**Goal:** Add subscription-related claims to LiveReview JWTs generated in cloud mode.

**Location:** [internal/api/auth/token_service.go](../internal/api/auth/token_service.go)

**Changes:**

1. **JWTClaims struct remains simple (no changes needed):**
   ```go
   type JWTClaims struct {
       jwt.RegisteredClaims
       UserID    int64  `json:"user_id"`
       Email     string `json:"email"`
       TokenHash string `json:"token_hash"`
       // No subscription claims - loaded dynamically per-request
   }
   ```
   
   **Rationale:** Since org context is passed per-request (via URL/header), and subscription 
   data varies by org, we load it dynamically in middleware. This requires **aggressive PostgreSQL 
   optimization** to avoid performance degradation.
   
   **Performance Strategy:**
   - Create composite index on `user_roles(user_id, org_id)` for O(1) lookups
   - Use connection pooling to minimize connection overhead
   - Consider query result caching for frequently accessed user-org pairs
   - Benchmark to ensure <5ms query time

2. **No changes to CreateTokenPair needed** - it continues to create simple JWTs:
2. **No changes to CreateTokenPair needed** - it continues to create simple JWTs:
   ```go
   func (ts *TokenService) CreateTokenPair(user *models.User, userAgent, ipAddress string) (*TokenPair, error) {
       // ... existing code unchanged ...
       // Tokens remain org-agnostic; subscription data loaded per-request
   }
   ```
   
   **Note:** Unlike the initial proposal, we do NOT embed org-specific subscription claims in the JWT. 
   Instead, subscription data is loaded dynamically per-request based on the org context from the URL/header.
   This approach:
   - Avoids JWT refresh when switching orgs
   - Keeps tokens smaller and simpler
   - Allows real-time subscription updates without re-authentication

3. **Remove isCloudMode helper** - not needed since JWTs don't change based on deployment mode

**Database Schema Requirement:**

Ensure `user_roles` table has these columns (add migration if missing):
```sql
ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS plan_type VARCHAR(50) DEFAULT 'free';
ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS license_expires_at TIMESTAMP;
ALTER TABLE user_roles ADD COLUMN IF NOT EXISTS active_subscription_id BIGINT;
```

**Critical Performance Optimization:**

Since subscription data is queried on EVERY API request, we must ensure blazing-fast lookups:

```sql
-- Create composite index for fast (user_id, org_id) lookups
CREATE INDEX IF NOT EXISTS idx_user_roles_user_org_plan 
ON user_roles(user_id, org_id) 
INCLUDE (plan_type, license_expires_at);

-- This covering index allows PostgreSQL to:
-- 1. Use index-only scan (no table heap access)
-- 2. Find rows in O(log n) time
-- 3. Return plan data without additional lookups
-- Expected query time: <2ms even with millions of rows
```

**Query Performance Verification:**

```sql
-- Test query performance (should use index-only scan)
EXPLAIN (ANALYZE, BUFFERS) 
SELECT plan_type, license_expires_at
FROM user_roles
WHERE user_id = 123 AND org_id = 456;

-- Expected output should show:
-- "Index Only Scan using idx_user_roles_user_org_plan"
-- "Execution Time: < 2ms"
```

### Phase 2: Implement Subscription Enforcement Middleware

**Goal:** Enforce plan limits based on JWT claims in cloud mode; use existing license validation in self-hosted mode.

**Location:** [internal/api/auth/middleware.go](../internal/api/auth/middleware.go)

**Changes:**

1. **Add EnforceSubscriptionLimits middleware:**
   ```go
   // EnforceSubscriptionLimits checks subscription validity (cloud) or license (self-hosted)
   func (am *AuthMiddleware) EnforceSubscriptionLimits() echo.MiddlewareFunc {
       return func(next echo.HandlerFunc) echo.HandlerFunc {
           return func(c echo.Context) error {
               if isCloudMode() {
                   // Cloud mode: load subscription data based on current org context
                   user, ok := c.Get("user").(*models.User)
                   if !ok {
                       return echo.NewHTTPError(http.StatusUnauthorized, "user not found in context")
                   }
                   
                   orgID, ok := c.Get("org_id").(int64)
                   if !ok {
                       // If no org context, use default/first org for the user
                       err := am.db.QueryRow(`
                           SELECT org_id FROM user_roles 
                           WHERE user_id = $1 
                           ORDER BY created_at ASC LIMIT 1
                       `, user.ID).Scan(&orgID)
                       if err != nil {
                           return echo.NewHTTPError(http.StatusForbidden, "no organization access")
                       }
                       c.Set("org_id", orgID)
                   }
                   
                   // Query user's plan in this specific org
                   var planType string
                   var licenseExpiresAt sql.NullTime
                   err := am.db.QueryRow(`
                       SELECT plan_type, license_expires_at
                       FROM user_roles
                       WHERE user_id = $1 AND org_id = $2
                   `, user.ID, orgID).Scan(&planType, &licenseExpiresAt)
                   
                   if err != nil {
                       if err == sql.ErrNoRows {
                           return echo.NewHTTPError(http.StatusForbidden, "no access to this organization")
                       }
                       return echo.NewHTTPError(http.StatusInternalServerError, "failed to check subscription")
                   }
                   
                   // Check license expiration
                   if licenseExpiresAt.Valid && time.Now().After(licenseExpiresAt.Time) {
                       return echo.NewHTTPError(http.StatusPaymentRequired, map[string]interface{}{
                           "error": "license expired",
                           "expired_at": licenseExpiresAt.Time,
                           "upgrade_required": true,
                       })
                   }
                   
                   // Set plan info in context for downstream handlers
                   c.Set("plan_type", planType)
                   if planType == "free" {
                       dailyLimit := 3
                       c.Set("daily_review_limit", &dailyLimit)
                   } else {
                       c.Set("daily_review_limit", (*int)(nil)) // unlimited
                   }
                   
               } else {
                   // Self-hosted mode: use existing license validation
                   // No changes - existing license.Service handles this
               }
               
               return next(c)
           }
       }
   }
   ```
   
   **Key Points:**
   - Queries `user_roles` for subscription data based on current org context (from URL/header)
   - **Optimized with covering index** - uses index-only scan for <2ms lookups
   - Works with existing middleware stack (`BuildOrgContext`, `BuildOrgContextFromHeader`)
   - Supports org-switching without JWT changes
   - Consider adding prepared statement caching for even better performance
   - Helper function for deployment mode check:
   ```go
   func isCloudMode() bool {
       return strings.ToLower(strings.TrimSpace(os.Getenv("LIVEREVIEW_IS_CLOUD"))) == "true"
   }
   ```

2. **Optional: Add prepared statement for query optimization:**
   ```go
   // In AuthMiddleware struct, add prepared statement
   type AuthMiddleware struct {
       tokenService *TokenService
       db           *sql.DB
       planStmt     *sql.Stmt  // Prepared statement for plan lookup
   }
   
   // Initialize prepared statement in NewAuthMiddleware
   func NewAuthMiddleware(tokenService *TokenService, db *sql.DB) *AuthMiddleware {
       stmt, err := db.Prepare(`
           SELECT plan_type, license_expires_at
           FROM user_roles
           WHERE user_id = $1 AND org_id = $2
       `)
       if err != nil {
           log.Printf("Warning: failed to prepare plan query: %v", err)
       }
       
       return &AuthMiddleware{
           tokenService: tokenService,
           db:           db,
           planStmt:     stmt,
       }
   }
   
   // Use prepared statement in middleware
   err := am.planStmt.QueryRow(user.ID, orgID).Scan(&planType, &licenseExpiresAt)
   ```
   
   **Performance Benefit:** Prepared statements reduce query parsing overhead by ~20-30%

2. **Apply middleware to protected routes** in [internal/api/server.go](../internal/api/server.go):
   ```go
   // After existing auth middleware
   protected.Use(auth.EnforceSubscriptionLimits())
   ```

### Phase 3: Usage Tracking for Free Plan ✅ **ALREADY IMPLEMENTED**

**Status:** This functionality is already complete in the codebase.

**Existing Implementation:**

1. **Plan Definitions:** [internal/license/plans.go](../internal/license/plans.go)
   - `PlanFree` configured with `MaxReviewsPerDay: 3`
   - `PlanTeam` and `PlanEnterprise` have unlimited reviews (`MaxReviewsPerDay: -1`)

2. **Enforcement Middleware:** [internal/api/middleware/plan_enforcement.go](../internal/api/middleware/plan_enforcement.go)
   - `CheckReviewLimit(db)` middleware queries the `reviews` table
   - Counts reviews created by user in current org since `CURRENT_DATE`
   - Returns `429 Too Many Requests` when limit exceeded
   - Works with plan limits from JWT claims (`claims.PlanType`, `claims.CurrentOrgID`)

**Existing Code:**

```go
// CheckReviewLimit enforces daily review limits based on plan
func CheckReviewLimit(db *sql.DB) echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            claims, ok := c.Get("claims").(*auth.JWTClaims)
            if !ok {
                return echo.NewHTTPError(http.StatusUnauthorized, "Invalid or missing authentication")
            }

            // Get plan limits
            planType := license.PlanType(claims.PlanType)
            limits := planType.GetLimits()

            // If unlimited reviews, skip the check
            if limits.MaxReviewsPerDay == -1 {
                return next(c)
            }

            // Count today's reviews for this user in this org
            var reviewCount int
            err := db.QueryRow(`
                SELECT COUNT(*) 
                FROM reviews 
                WHERE created_by_user_id = $1 
                AND org_id = $2
                AND created_at >= CURRENT_DATE
            `, claims.UserID, claims.CurrentOrgID).Scan(&reviewCount)

            if err != nil {
                return echo.NewHTTPError(http.StatusInternalServerError,
                    "Failed to check review limit")
            }

            // Check if limit exceeded
            if reviewCount >= limits.MaxReviewsPerDay {
                return echo.NewHTTPError(http.StatusTooManyRequests,
                    "Daily review limit reached. Upgrade to Team plan for unlimited reviews.")
            }

            return next(c)
        }
    }
}
```

**What This Means for Our Plan:**

- ✅ No separate `daily_usage` table needed (counts directly from `reviews` table)
- ✅ No new tracking functions needed (`CheckReviewLimit` handles everything)
- ✅ Middleware already integrated into review creation handlers
- ⚠️ **Action Required:** Ensure this middleware is applied to review creation routes in cloud mode

### Phase 4: Update EnsureCloudUser to Set Default Plan

**Goal:** Ensure newly provisioned cloud users get `plan_type='free'` set in their `user_roles`.

**Location:** [internal/api/auth/handlers.go](../internal/api/auth/handlers.go) line ~850

**Changes:**

Update the `user_roles` INSERT in `EnsureCloudUser`:

```go
// Current code (around line 850):
_, err = tx.Exec(`
    INSERT INTO user_roles (user_id, org_id, role_id, created_at, updated_at)
    VALUES ($1, $2, $3, NOW(), NOW())
`, userID, orgID, superAdminRoleID)

// Change to:
_, err = tx.Exec(`
    INSERT INTO user_roles (user_id, org_id, role_id, plan_type, created_at, updated_at)
    VALUES ($1, $2, $3, 'free', NOW(), NOW())
`, userID, orgID, superAdminRoleID)
```

### Phase 5: Conditionally Hide License UI in Frontend

**Goal:** Hide the license banner and settings page in cloud mode; show subscription management instead.

**Frontend Configuration:**

1. **Pass LIVEREVIEW_IS_CLOUD to frontend** in [internal/api/ui.go](../internal/api/ui.go):

```go
// In the UI server code that generates window.LIVEREVIEW_CONFIG
config := map[string]interface{}{
    "apiUrl": apiURL,
    "isCloud": os.Getenv("LIVEREVIEW_IS_CLOUD") == "true",
    // ... other config
}
```

2. **Update License Banner Component:**

Find the license status banner component (likely in `ui/src/components/` or `ui/src/layouts/`) and wrap it:

```tsx
// In the layout/header component
{!window.LIVEREVIEW_CONFIG?.isCloud && (
  <LicenseStatusBanner />
)}
```

3. **Update Settings Page:**

In the settings/license page component:

```tsx
// ui/src/pages/Settings/License.tsx or similar
import { Navigate } from 'react-router-dom';

const LicensePage = () => {
  const isCloud = window.LIVEREVIEW_CONFIG?.isCloud;
  
  if (isCloud) {
    // Redirect to subscription management in cloud mode
    return <Navigate to="/settings/subscription" replace />;
  }
  
  // Render license management UI for self-hosted
  return (
    <div>
      {/* Existing license UI */}
    </div>
  );
};
```

4. **Create Subscription Management Page (Cloud):**

```tsx
// ui/src/pages/Settings/Subscription.tsx (new file)
import React from 'react';

const SubscriptionPage = () => {
  const isCloud = window.LIVEREVIEW_CONFIG?.isCloud;
  
  if (!isCloud) {
    return <Navigate to="/settings/license" replace />;
  }
  
  return (
    <div>
      <h1>Subscription Management</h1>
      {/* TODO: Implement subscription UI in future phases */}
      <p>Your current plan: Free (3 reviews per day)</p>
      <p>Subscription management coming soon.</p>
    </div>
  );
};

export default SubscriptionPage;
```

5. **Update Navigation/Routing:**

```tsx
// In settings route configuration
{
  path: 'license',
  element: <LicensePage />,
},
{
  path: 'subscription',
  element: <SubscriptionPage />,
},
```

### Phase 6: Self-Hosted License Validation (Preserved)

**Goal:** Ensure self-hosted deployments continue to work with existing license file mechanism.

**No Changes Required** - The existing implementation in [internal/license](../internal/license) will continue to work:

- License validation in [internal/license/validator.go](../internal/license/validator.go)
- License service in [internal/license/service.go](../internal/license/service.go)
- License endpoints in [internal/api/license.go](../internal/api/license.go)

The middleware added in Phase 2 explicitly checks `LIVEREVIEW_IS_CLOUD` and only applies subscription logic in cloud mode.

## Database Migrations

Create migration files in order:

**Migration 1: Add subscription columns to user_roles**
```sql
-- migrations/XXXXXX_add_subscription_to_user_roles.up.sql
ALTER TABLE user_roles 
ADD COLUMN IF NOT EXISTS plan_type VARCHAR(50) DEFAULT 'free',
ADD COLUMN IF NOT EXISTS license_expires_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS active_subscription_id BIGINT;

COMMENT ON COLUMN user_roles.plan_type IS 'User plan in this org: free, team, enterprise';
COMMENT ON COLUMN user_roles.license_expires_at IS 'When the license expires for this user in this org';
COMMENT ON COLUMN user_roles.active_subscription_id IS 'Reference to subscriptions table (future)';

-- CRITICAL: Create covering index for extremely fast lookups
-- This index allows PostgreSQL to serve queries from index alone (no heap access)
CREATE INDEX IF NOT EXISTS idx_user_roles_user_org_plan 
ON user_roles(user_id, org_id) 
INCLUDE (plan_type, license_expires_at);

-- Verify index is used (should show "Index Only Scan")
-- EXPLAIN SELECT plan_type, license_expires_at 
-- FROM user_roles WHERE user_id = 1 AND org_id = 1;

COMMENT ON INDEX idx_user_roles_user_org_plan IS 'Covering index for subscription lookups - enables index-only scans for <2ms query time';
```

**Migration 2: ~~Create daily_usage table~~ NOT NEEDED - Already using `reviews` table**

> **Note:** The existing `CheckReviewLimit` middleware queries the `reviews` table directly:
> ```sql
> SELECT COUNT(*) FROM reviews 
> WHERE created_by_user_id = $1 AND org_id = $2 AND created_at >= CURRENT_DATE
> ```
> This is more efficient than a separate `daily_usage` table and requires no additional migration.

## Testing Strategy

### Unit Tests

1. **JWT Claims Testing:**
   - ~~Test `CreateTokenPair` generates correct subscription claims in cloud mode~~ - NOT APPLICABLE
   - ~~Test `CreateTokenPair` doesn't add subscription claims in self-hosted mode~~ - NOT APPLICABLE
   - Test JWT validation works unchanged
   - **Performance:** Verify JWT generation time unchanged (<10ms)

2. **Usage Tracking:** ✅ **Already Tested**
   - ✅ `CheckReviewLimit` middleware already exists and tested
   - Test with free plan user: verify 3 reviews allowed, 4th blocked
   - Test with team plan user: verify unlimited reviews

3. **Middleware:**
   - Test `EnforceSubscriptionLimits` queries subscription data correctly
   - Test `EnforceSubscriptionLimits` rejects expired licenses
   - Test self-hosted mode bypasses cloud checks
   - **Performance:** Test subscription query time <5ms (p99)
   - **Load Testing:** Verify index is used under concurrent requests

### Integration Tests

1. **Cloud Mode Flow:**
   - User provisions via `EnsureCloudUser` → verify user gets `plan_type='free'`
   - User makes API request → verify subscription data loaded from DB
   - User creates reviews → verify `CheckReviewLimit` middleware enforces 3/day limit
   - User exceeds limit → verify 429 Too Many Requests response (from existing middleware)
   - **Performance:** Monitor query performance under load (p99 <5ms)

2. **Self-Hosted Mode Flow:**
   - User logs in → verify existing license validation works
   - User operates system → verify no usage tracking applied
   - **Performance:** Verify no performance degradation

### Manual Testing

1. **Cloud Deployment:**
   - Set `LIVEREVIEW_IS_CLOUD=true`
   - Login via Hexmos → verify license UI hidden
   - Create 3 reviews → verify allowed
   - Attempt 4th review → verify blocked with upgrade message

2. **Self-Hosted Deployment:**
   - Set `LIVEREVIEW_IS_CLOUD=false`
   - Login with email/password → verify license UI shown
   - Load license file → verify validation works
   - Operate normally → verify no changes in behavior

## Rollout Plan

### Step 1: Database Preparation (Week 1)
- Apply migrations to add subscription columns
- **Create covering index on user_roles(user_id, org_id)** - critical for performance
- Verify index usage with EXPLAIN ANALYZE
- Backfill existing cloud users with `plan_type='free'`
- ~~Create daily_usage table~~ - NOT NEEDED (using `reviews` table directly)
- **Benchmark subscription lookup query** - must be <5ms at p99

### Step 2: Backend Implementation (Week 2)
- ~~Implement JWT claims extension (Phase 1)~~ - NOT NEEDED (keeping JWTs simple)
- Implement enforcement middleware with optimized queries (Phase 2)
- Add prepared statement caching for subscription lookups
- Update EnsureCloudUser (Phase 4)
- **Performance testing:** Verify subscription lookup <5ms under load
- Deploy to staging environment

### Step 3: Usage Tracking ✅ **ALREADY COMPLETE**
- ✅ `CheckReviewLimit` middleware already exists in [internal/api/middleware/plan_enforcement.go](../internal/api/middleware/plan_enforcement.go)
- ✅ Free plan limit (3 reviews/day) configured in [internal/license/plans.go](../internal/license/plans.go)
- ⚠️ **Action Required:** Verify middleware is applied to review creation routes

### Step 4: Frontend Updates (Week 4)
- Hide license UI in cloud mode (Phase 5)
- Create placeholder subscription page
- Deploy to staging
- User acceptance testing

### Step 5: Production Deployment (Week 5)
- Deploy to production cloud
- Monitor for issues
- Verify self-hosted installations unaffected

## Open Questions & Future Work

### Resolved Design Decisions

1. **Org Context Switching:**
   - **Current Implementation:** Organization context is passed via URL parameters (`:org_id` in routes) or `X-Org-Context` header
   - **Middleware:** `BuildOrgContext()` and `BuildOrgContextFromHeader()` extract org_id and set it in request context
   - **No JWT Refresh Needed:** The same JWT is reused; middleware validates user's access to the requested org on each request
   - **Plan Integration:** When enforcing subscription limits, middleware will:
     1. Extract org_id from URL/header (existing behavior)
     2. Query `user_roles` table for user's plan in that specific org
     3. Set plan claims in context for the request
     4. Enforce limits based on that org's plan
   - **Performance:** Single DB query per request to get plan info (acceptable overhead)

2. **Subscription Data Management:**
   - **Decision:** Use existing subscription purchase flow - NO schema changes needed
   - **Rationale:** The buying flow works fine as-is; changing to a new schema would be unnecessary disruption
   - **Implementation:** Store plan data in `user_roles` table columns (`plan_type`, `license_expires_at`, `active_subscription_id`)
   - **Future:** Can migrate to full `subscriptions` table from [razorpay_subscription_implementation_plan.md](razorpay_subscription_implementation_plan.md) later if needed

3. **Self-Hosted License Server:**
   - **Decision:** Keep existing validation at `https://parse.apps.hexmos.com` unchanged
   - **Rationale:** Self-hosted deployments should not be affected by cloud subscription changes
   - **Implementation:** Conditional logic in middleware checks `LIVEREVIEW_IS_CLOUD`:
     - If `false`: Use existing [internal/license](../internal/license) validation (no changes)
     - If `true`: Use new subscription-based validation from JWT claims

### Future Enhancements

1. **Subscription Purchase Flow:**
   - Payment integration (Razorpay/Stripe)
   - Plan selection UI
   - Checkout process
   - License assignment interface

2. **Advanced Features:**
   - Team plan management (assign licenses to team members)
   - Usage analytics dashboard
   - Billing history
   - Invoice generation

3. **Enterprise Features:**
   - Custom plans
   - SSO integration
   - Advanced analytics
   - Dedicated support

## Success Criteria

### Cloud Mode
- [ ] New users get `plan_type='free'` automatically
- [ ] ~~JWT contains subscription claims~~ - Subscription data loaded per-request from DB
- [ ] Covering index created on `user_roles(user_id, org_id)`
- [ ] Subscription lookup query uses index-only scan
- [ ] Query performance <5ms at p99 under production load
- [ ] Free users limited to 3 reviews per day
- [ ] License UI hidden, subscription page shown
- [ ] Clear error messages when limits reached
- [ ] No license file required

### Self-Hosted Mode
- [ ] Existing license validation works unchanged
- [ ] License UI continues to function
- [ ] No behavioral changes from user perspective
- [ ] No new environment variables required (except existing `LIVEREVIEW_IS_CLOUD=false`)

### Technical
- [ ] Zero downtime deployment
- [ ] All tests passing
- [ ] Documentation updated
- [ ] Monitoring in place for usage tracking
- [ ] **Performance monitoring:** Subscription query latency dashboards
- [ ] **Index verification:** Automated checks that covering index is used
- [ ] Error tracking for enforcement failures
- [ ] Load testing shows <5ms p99 latency for subscription checks

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        CLOUD MODE FLOW                          │
└─────────────────────────────────────────────────────────────────┘

   Hexmos SSO (fw-parse)
          │
          │ JWT with email
          ▼
   [Cloud.tsx Frontend]
          │
          │ POST /api/v1/auth/ensure-cloud-user
          │ Authorization: Bearer <fw-parse-jwt>
          ▼
   [EnsureCloudUser Handler]
          │
          ├─ Validate fw-parse JWT (CLOUD_JWT_SECRET)
          ├─ Find/Create User
          ├─ Find/Create Org
          ├─ Set plan_type='free' in user_roles
          │
          ▼
   [TokenService.CreateTokenPair]
          │
          ├─ Query user_roles for subscription data
          ├─ Generate JWT with claims:
          │  - currentOrgID
          │  - planType: "free"
          │  - dailyReviewLimit: 3
          │  - licenseExpiresAt: null
          │
          ▼
   Return to Frontend:
   {
     "tokens": { "access_token": "...", "refresh_token": "..." },
     "user": { ... },
     "organizations": [ ... ]
   }
          │
          ▼
   [Subsequent API Requests]
          │
          ├─ Authorization: Bearer <livereview-jwt>
          │
          ▼
   [RequireAuth Middleware]
          │
          ├─ Validate JWT signature (JWT_SECRET)
          ├─ Extract claims → c.Set("claims", claims)
          │
          ▼
   [EnforceSubscriptionLimits Middleware]
          │
          ├─ Check licenseExpiresAt → reject if expired
          ├─ Set plan_type in context
          │
          ▼
   [Review Creation Handler]
          │
          ├─ CheckDailyLimit(user, org, limit=3)
          ├─ Query daily_usage table
          │
          ├─ If limit reached → 402 Payment Required
          │
          ├─ Create review
          ├─ IncrementDailyUsage(user, org)
          │
          ▼
   Success Response

┌─────────────────────────────────────────────────────────────────┐
│                     SELF-HOSTED MODE FLOW                       │
└─────────────────────────────────────────────────────────────────┘

   Email/Password Login
          │
          ▼
   [Login Handler]
          │
          ├─ Validate credentials
          ├─ CreateTokenPair (no subscription claims)
          │
          ▼
   [Subsequent API Requests]
          │
          ▼
   [RequireAuth Middleware]
          │
          ▼
   [EnforceSubscriptionLimits Middleware]
          │
          ├─ Detect LIVEREVIEW_IS_CLOUD=false
          ├─ Call existing License Validator
          ├─ Check license file validity
          │
          ▼
   [Review Creation Handler]
          │
          ├─ No usage limits checked
          ├─ Create review normally
          │
          ▼
   Success Response
```

## File Manifest

Files to be modified:

1. **Backend:**
   - ~~`internal/api/auth/token_service.go` - Extend JWT claims, add subscription data~~ - NOT NEEDED
   - `internal/api/auth/middleware.go` - Add EnforceSubscriptionLimits middleware with optimized queries
   - `internal/api/auth/handlers.go` - Update EnsureCloudUser to set plan_type
   - `internal/api/server.go` - Apply new middleware to routes
   - `internal/api/ui.go` - Pass isCloud config to frontend
   - `internal/api/reviews.go` - Add usage tracking to review creation

2. **Database:**
   - `migrations/XXXXXX_add_subscription_to_user_roles.up.sql` - New migration with covering index
   - ~~`migrations/XXXXXX_create_daily_usage.up.sql`~~ - NOT NEEDED

3. **Frontend:**
   - `ui/src/pages/Auth/Cloud.tsx` - No changes needed
   - `ui/src/pages/Settings/License.tsx` - Add cloud mode redirect
   - `ui/src/pages/Settings/Subscription.tsx` - New file (placeholder)
   - `ui/src/components/LicenseStatusBanner.tsx` - Add conditional rendering
   - `ui/src/routes/settings.tsx` - Add subscription route

4. **Documentation:**
   - `docs/subscription-licence-shift.md` - This file
   - `README.md` - Update deployment instructions
   - `docs/API.md` - Document new JWT claims

Files to remain unchanged:
- `internal/license/*` - Self-hosted license validation preserved
- `internal/api/license.go` - License endpoints for self-hosted
- All existing review logic (except adding usage tracking)
