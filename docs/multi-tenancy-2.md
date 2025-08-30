The results of multi tenancy phase 1 implementatin is as follows:


Phase 1 Output

Phase 1 Multi-Tenancy Implementation Complete ✅
I have successfully implemented Phase 1 of the multi-tenancy system for your LiveReview SaaS application. Here's what has been accomplished:

🗄️ Database Schema Implementation
✅ Core multi-tenancy tables created:

orgs - Organizations with ID, name, description
users - Users with email, password hash
roles - Three-tier role system (super_admin, owner, member)
user_roles - Many-to-many relationship between users, roles, and orgs
✅ Existing tables u### Performance Considerations ⚡

### Database Optimization
- **Optimized indexing** - Partial indexes on active tokens, compound indexes for common queries
- **Connection pooling** - Reuse existing PostgreSQL connections for session operations
- **Query optimization** - Session lookups use covering indexes for sub-millisecond performance
- **Background cleanup** - Automated cleanup via gocron scheduler and goroutines
- **Transaction efficiency** - Batch session operations where possible
- **Startup cleanup** - Application removes stale data on startup for optimal performance

### Session Management Performance
- **Fast token validation** - Hash lookup with partial index on active tokens only
- **Efficient updates** - Update `last_used_at` with minimal locking
- **Memory optimization** - JWT tokens carry minimal payload, full session data in database
- **Connection reuse** - Session operations use existing database pool
- **Background cleanup** - Non-blocking goroutines clean expired tokens without affecting performance
- **Startup optimization** - Initial cleanup removes stale data on application started org_id column to all business tables (ai_comments, ai_connectors, integration_tokens, reviews, webhook_registry, recent_activity, dashboard_cache)
Created proper foreign key constraints and indexes
Migrated existing data to default organization (ID: 1)
🔧 Application Layer Implementation
✅ Go Models: Added struct definitions for Org, User, Role, UserRole, and UserOrgInfo
✅ UserService: Complete service layer with:
CreateFirstAdminUser() - Creates initial admin user and organization
MigrateExistingAdminPassword() - Converts existing admin password to user-based auth
CheckSetupStatus() - Checks if system is initialized
User authentication and role management methods
🌐 API Endpoints Implementation
✅ New auth endpoints (alongside existing ones for backward compatibility):
POST /api/v1/auth/setup - Create first admin user
GET /api/v1/auth/setup/status - Check setup status
POST /api/v1/auth/login - User login
GET /api/v1/auth/me - Get current user (placeholder for sessions)
🔄 Migration & Compatibility
✅ Seamless migration: Existing admin password automatically converted to super_admin user with email admin@localhost
✅ Backward compatibility: Legacy password endpoints remain functional during transition
✅ Data preservation: All existing data assigned to default organization
✅ Verification Tests
Database migrations applied successfully
Application builds and runs without errors
Setup status endpoint returns correct state
Admin user creation and migration working properly
System correctly prevents duplicate admin users
🚀 Current System State
Setup Status: ✅ System is set up (migrated from existing password)
Admin User: admin@localhost with super_admin role in default organization
Database: All 25 migrations applied, multi-tenancy schema active
API: New auth endpoints functional alongside legacy ones
📋 Next Steps for Full Multi-Tenancy
The foundation is now solid for Phase 2 implementation:

Session Management: Implement JWT/session handling for GetCurrentUser
Organization Management: APIs for creating/managing organizations
✅ User Management: APIs for inviting users, role assignments - **COMPLETED**
Data Isolation: Update business logic to filter by org_id
CLI Updates: Update command-line tools to use new user system
UI Updates: Frontend changes for organization switching
Your multi-tenancy foundation is now ready! The system can scale from single-admin self-hosted deployments to full multi-tenant SaaS with organizations and role-based access control.

---

# Phase 2 Multi-Tenancy Implementation Plan 🚀

## Overview & Strategic Approach

Phase 2 will transform the foundation into a fully functional multi-tenant SaaS system with a unified authentication system that supports multiple user sources (admin-created accounts, external providers like Authentik), session management, and future API token capabilities.

## Core Authentication Strategy 🔐

### Problem Statement
We need a unified authentication system that handles:
1. **Admin-created users** - Direct user management by org owners/admins
2. **External providers** - Future Authentik/OIDC integration for enterprise SSO
3. **API tokens** - Developer access for programmatic API usage
4. **Session management** - Secure, scalable session handling for web users

### Architectural Approach

**Identity Provider Abstraction Layer**
Instead of building everything from scratch, we'll create an identity provider abstraction that can handle multiple authentication sources uniformly. This allows us to start with local user management and seamlessly add external providers later.

**Why this approach:**
- **Unified login flow** - Same session/JWT handling regardless of auth source
- **Future-proof** - Easy to add Authentik, Google SSO, etc. without changing core logic
- **Developer-friendly** - API tokens work alongside user sessions with same authorization
- **Minimal code maintenance** - Leverage Echo's middleware system and proven JWT libraries

### Implementation Strategy

**Location: `internal/api/auth/`** (New package for all auth-related code)
- Separates authentication concerns from main API logic
- Makes it easier to test and maintain auth flows
- Allows for clean provider abstraction

**Core Components:**
1. **Identity Provider Interface** - Abstract different auth sources
2. **Session Manager** - Unified session/JWT handling using Echo middleware
3. **Token Service** - Handles both user sessions and API tokens
4. **User Management Service** - Admin user CRUD operations

## 1. Session Management & Token Architecture 🔐

### Problem Analysis
Current challenge: Need robust session management that works for:
- Web UI users (JWT tokens)
- API developers (API keys)
- External SSO users (future)
- CLI tools (token-based auth)

### Solution Architecture

**Echo Middleware Integration**
Instead of building custom session handling, we'll leverage Echo's middleware ecosystem with proven libraries:
- **echo-jwt** for JWT validation middleware
- **golang-jwt** for token generation/validation
- **PostgreSQL** for session storage with optimized schema and indexing

**Why this approach:**
- **Battle-tested** - Echo middleware is production-proven
- **Low maintenance** - Standard JWT handling with existing libraries
- **Flexible** - Same middleware handles user sessions and API tokens
- **Simple infrastructure** - No additional Redis dependency, uses existing PostgreSQL
- **ACID compliance** - Database transactions ensure session consistency

### Database Schema Strategy - Session Management Migration

**Step 1: Create Migration for Session Management**
```bash
dbmate new create_session_management
```

**Migration File: `db/migrations/YYYYMMDDHHMMSS_create_session_management.sql`**

**-- +migrate Up**
```sql
-- Unified token table for sessions, API keys, and refresh tokens
CREATE TABLE auth_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_type VARCHAR(20) NOT NULL CHECK (token_type IN ('session', 'api_key', 'refresh')),
    token_hash VARCHAR(255) NOT NULL UNIQUE, -- Hashed token for security
    name VARCHAR(100), -- For API keys: "My App Token" 
    scopes TEXT[], -- Future: granular permissions array
    expires_at TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    ip_address INET, -- Track session origin
    user_agent TEXT, -- Track client info
    is_active BOOLEAN NOT NULL DEFAULT true
);

-- Performance optimized indexes for PostgreSQL
CREATE INDEX idx_auth_tokens_user_id ON auth_tokens(user_id);
CREATE INDEX idx_auth_tokens_hash ON auth_tokens(token_hash) WHERE is_active = true;
CREATE INDEX idx_auth_tokens_expires ON auth_tokens(expires_at) WHERE is_active = true;
CREATE INDEX idx_auth_tokens_type_user ON auth_tokens(token_type, user_id) WHERE is_active = true;
CREATE INDEX idx_auth_tokens_last_used ON auth_tokens(last_used_at) WHERE is_active = true;

-- Partial index for active sessions only (major performance boost)
CREATE INDEX idx_auth_tokens_active_sessions ON auth_tokens(user_id, last_used_at) 
WHERE token_type = 'session' AND is_active = true;

-- Composite index for cleanup operations
CREATE INDEX idx_auth_tokens_cleanup ON auth_tokens(token_type, expires_at, is_active);
```

**-- +migrate Down**
```sql
-- Drop indexes first (order matters for PostgreSQL)
DROP INDEX IF EXISTS idx_auth_tokens_cleanup;
DROP INDEX IF EXISTS idx_auth_tokens_active_sessions;
DROP INDEX IF EXISTS idx_auth_tokens_last_used;
DROP INDEX IF EXISTS idx_auth_tokens_type_user;
DROP INDEX IF EXISTS idx_auth_tokens_expires;
DROP INDEX IF EXISTS idx_auth_tokens_hash;
DROP INDEX IF EXISTS idx_auth_tokens_user_id;

-- Drop table last
DROP TABLE IF EXISTS auth_tokens;
```

**Step 2: Apply Migration**
```bash
dbmate up
```

**PostgreSQL Session Management Benefits:**
- **ACID transactions** - Consistent session state updates
- **Efficient cleanup** - Background goroutines with gocron scheduler for automated cleanup
- **Fast lookups** - Partial indexes on active tokens only
- **Connection pooling** - Reuse existing database connections
- **Atomic operations** - Session creation/validation/cleanup in single transactions
- **Application-controlled** - No external dependencies, cleanup runs within the Go application

**Session Cleanup Strategy:**
```sql
-- Cleanup expired tokens (run via background goroutine)
DELETE FROM auth_tokens 
WHERE expires_at < NOW() AND token_type IN ('session', 'refresh');

-- Cleanup old inactive sessions (keep for audit)
UPDATE auth_tokens 
SET is_active = false 
WHERE last_used_at < NOW() - INTERVAL '30 days' 
  AND token_type = 'session' 
  AND is_active = true;
```

**Go Background Cleanup Implementation:**
```go
// In token_service.go
func (s *TokenService) StartCleanupScheduler() {
    // Run cleanup on startup
    go s.cleanupExpiredTokens()
    
    // Schedule periodic cleanup using gocron
    scheduler := gocron.NewScheduler(time.UTC)
    scheduler.Every(1).Hour().Do(s.cleanupExpiredTokens)
    scheduler.Every(24).Hours().Do(s.cleanupInactiveSessions)
    scheduler.StartAsync()
}

func (s *TokenService) cleanupExpiredTokens() error {
    result, err := s.db.Exec(`
        DELETE FROM auth_tokens 
        WHERE expires_at < NOW() AND token_type IN ('session', 'refresh')
    `)
    // Log cleanup results
    return err
}
```

**Why PostgreSQL-based sessions:**
- **Consistency** - Same database for all data, no sync issues
- **Simplicity** - One service manages all token types with database transactions
- **Security** - Centralized token hashing and validation with ACID guarantees
- **Performance** - Optimized indexes and partial indexes for fast session lookups
- **Reliability** - No additional infrastructure dependencies
- **Audit trail** - All session activities logged in same database
- **Self-managing** - Background goroutines handle cleanup automatically within the application

### Implementation Location & Logic

**File: `internal/api/auth/token_service.go`**
- **Why here:** Centralizes all token operations
- **Logic:** Factory pattern for different token types (session vs API key)
- **Integration:** Uses Echo JWT middleware for validation
- **Background cleanup:** Includes gocron scheduler for automatic token cleanup

**File: `internal/api/auth/middleware.go`**
- **Why here:** Custom Echo middleware for our specific needs
- **Logic:** Token extraction → validation → user context setting
- **Benefits:** Plugs directly into Echo's middleware chain

**Background Cleanup Service Architecture:**
```go
type TokenService struct {
    db        *sql.DB
    scheduler *gocron.Scheduler
    logger    *log.Logger
}

// Startup cleanup and scheduler initialization
func (s *TokenService) Initialize() error {
    // Run initial cleanup on startup
    if err := s.cleanupExpiredTokens(); err != nil {
        s.logger.Printf("Initial token cleanup failed: %v", err)
    }
    
    // Start background scheduler
    s.StartCleanupScheduler()
    return nil
}

// Graceful shutdown
func (s *TokenService) Shutdown() error {
    s.scheduler.Stop()
    return nil
}
```

## 2. User Management System 👥

### Problem Statement
**Current Challenge:** On-premises deployments need admin-controlled user management without external dependencies like SMTP servers or invitation systems.

**Business Context:**
- **Self-hosted environments** often have restricted network access and no SMTP servers
- **Organization owners** need immediate user creation capability without waiting for email acceptance
- **Super admins** need global oversight across all organizations for compliance and support
- **Security requirement** - No self-registration, all users must be admin-created with defined roles

**User Stories:**
- "As an organization owner, I want to create users immediately with email/password so my team can start working"
- "As a super admin, I want to manage users across all organizations for compliance and support"
- "As a member, I want to view my organization's users and update my own profile"

### Strategic Solution Approach

**Direct User Creation Pattern**
Instead of invitation-based workflows, we'll implement direct user creation where admins set initial passwords.

**Why this approach:**
- **Zero external dependencies** - No SMTP, email services, or network requirements
- **Immediate productivity** - Users can login instantly after creation
- **Simple mental model** - Admins create users like traditional systems
- **On-premises friendly** - Works in air-gapped or restricted environments
- **Future extensible** - Can add invitation system later without breaking existing workflows

**Alternative Approaches Considered:**
- **Invitation system** - Rejected due to SMTP dependency and complexity
- **Self-registration** - Rejected due to security requirements
- **External auth only** - Rejected due to on-premises limitations

**Key Design Decisions:**
- **Admin-set initial passwords** - Org owners set temporary passwords, users can change them
- **Role-based user creation** - Owners create users in their orgs, super admins create anywhere
- **Immediate activation** - No email verification needed, users active on creation
- **Audit trail** - Track who created which users and when

**Role-Based User Management Matrix:**
```
Action                    | Super Admin | Org Owner | Member
--------------------------|-------------|-----------|--------
Create users in any org  |     ✅      |     ❌    |   ❌
Create users in own org  |     ✅      |     ✅    |   ❌
Modify users in any org  |     ✅      |     ❌    |   ❌
Modify users in own org  |     ✅      |     ✅    |   ❌
Deactivate users any org |     ✅      |     ❌    |   ❌
Deactivate users own org |     ✅      |     ✅    |   ❌
View users in any org    |     ✅      |     ❌    |   ❌
View users in own org    |     ✅      |     ✅    |   ✅
Update own profile       |     ✅      |     ✅    |   ✅
Change own password      |     ✅      |     ✅    |   ✅
```

### Architecture Strategy

**Three-Layer User Management**
1. **UI Layer** - Role-based dashboards with different user management interfaces
2. **API Layer** - RESTful endpoints with permission validation
3. **Service Layer** - Business logic with audit trails and validation

**Security Philosophy:**
- **Principle of least privilege** - Each role gets minimum necessary permissions
- **Organization boundaries** - Strict isolation between organizations
- **Audit everything** - All user management actions logged with who/what/when
- **Fail-safe defaults** - Deny access unless explicitly permitted

### Implementation Strategy

**UI Components Needed:**

**1. Organization Admin Dashboard** (`ui/src/pages/org-admin/users/`)
- **User list view** - Table showing org users with role badges, status, last login
- **Create user form** - Email, password, role selection, optional first/last name
- **Edit user modal** - Update user details, change role, deactivate account
- **Bulk actions** - Select multiple users for role changes or deactivation

**2. Super Admin Dashboard** (`ui/src/pages/super-admin/users/`)
- **Global user list** - All users across organizations with org column
- **Organization selector** - Filter users by organization
- **Cross-org user management** - Create users in any org, transfer between orgs
- **System analytics** - User count per org, role distribution, activity metrics

**3. Member Profile Page** (`ui/src/pages/profile/`)
- **View profile** - Display user info, role, organization membership
- **Edit profile** - Update first/last name, email (with validation)
- **Change password** - Current password verification, new password form

**API Endpoints Design:**

**Organization-Scoped User Management:**
```
POST   /api/v1/orgs/:org_id/users           # Create user in organization
GET    /api/v1/orgs/:org_id/users           # List org users (paginated)
GET    /api/v1/orgs/:org_id/users/:user_id  # Get specific user details
PUT    /api/v1/orgs/:org_id/users/:user_id  # Update user (role, profile, status)
DELETE /api/v1/orgs/:org_id/users/:user_id  # Deactivate user (soft delete)
```

**Super Admin Global Management:** ✅ **COMPLETED & TESTED**
```
GET    /api/v1/admin/users                  # List all users across orgs
POST   /api/v1/admin/orgs/:org_id/users     # Create user in any org
PUT    /api/v1/admin/users/:user_id/org     # Transfer user between orgs
GET    /api/v1/admin/analytics/users        # User management analytics
```

**Self-Service Profile Management:**
```
GET    /api/v1/users/profile                # Get own profile
PUT    /api/v1/users/profile                # Update own profile
PUT    /api/v1/users/password               # Change own password
```

**Database Schema Requirements:**

**Enhanced Users Table:**
```sql
-- Add fields for better user management (no invitation system needed)
ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_by_user_id BIGINT NULL REFERENCES users(id);
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_by_user_id BIGINT NULL REFERENCES users(id);
```

**User Management Audit Trail:**
```sql
CREATE TABLE user_management_audit (
    id BIGSERIAL PRIMARY KEY,
    target_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    performed_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL, -- 'created', 'updated', 'deactivated', 'role_changed'
    old_values JSONB,
    new_values JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

**Indexing for Performance:**
```sql
-- User management query optimization
CREATE INDEX idx_users_org_active ON users(org_id, is_active);
CREATE INDEX idx_users_created_by ON users(created_by_user_id, created_at);
CREATE INDEX idx_audit_target_time ON user_management_audit(target_user_id, created_at DESC);
CREATE INDEX idx_audit_org_action ON user_management_audit(org_id, action, created_at DESC);
```
### Database Migration Strategy

**Step 1: Create Migration for Direct User Management**
```bash
dbmate new enhance_user_management_system
```

**Migration File: `db/migrations/YYYYMMDDHHMMSS_enhance_user_management_system.sql`**

**-- +migrate Up**
```sql
-- Enhance users table for direct admin-managed user creation
ALTER TABLE users ADD COLUMN IF NOT EXISTS first_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_name VARCHAR(100);
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMP NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deactivated_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;

-- User management audit trail for compliance and debugging
CREATE TABLE user_management_audit (
    id BIGSERIAL PRIMARY KEY,
    target_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    performed_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    action VARCHAR(50) NOT NULL CHECK (action IN ('created', 'updated', 'deactivated', 'role_changed', 'password_reset')),
    old_values JSONB DEFAULT '{}',
    new_values JSONB DEFAULT '{}',
    reason TEXT, -- Optional reason for the action
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Performance indexes for user management queries
CREATE INDEX idx_users_org_active ON users(org_id, is_active);
CREATE INDEX idx_users_created_by ON users(created_by_user_id, created_at DESC);
CREATE INDEX idx_users_last_login ON users(last_login_at DESC) WHERE is_active = true;

-- Audit trail indexes
CREATE INDEX idx_audit_target_time ON user_management_audit(target_user_id, created_at DESC);
CREATE INDEX idx_audit_org_action ON user_management_audit(org_id, action, created_at DESC);
CREATE INDEX idx_audit_performed_by ON user_management_audit(performed_by_user_id, created_at DESC);
```

**-- +migrate Down**
```sql
-- Drop audit trail
DROP INDEX IF EXISTS idx_audit_performed_by;
DROP INDEX IF EXISTS idx_audit_org_action;
DROP INDEX IF EXISTS idx_audit_target_time;
DROP TABLE IF EXISTS user_management_audit;

-- Drop user management indexes
DROP INDEX IF EXISTS idx_users_last_login;
DROP INDEX IF EXISTS idx_users_created_by;
DROP INDEX IF EXISTS idx_users_org_active;

-- Remove user management columns
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_by_user_id;
ALTER TABLE users DROP COLUMN IF EXISTS deactivated_at;
ALTER TABLE users DROP COLUMN IF EXISTS created_by_user_id;
ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS is_active;
ALTER TABLE users DROP COLUMN IF EXISTS last_name;
ALTER TABLE users DROP COLUMN IF EXISTS first_name;
```

**Step 2: Apply Migration**
```bash
dbmate up
```
```

**Step 2: Create Migration for Enhanced Organization Management**
```bash
dbmate new enhance_organization_management
```

**Migration File: `db/migrations/YYYYMMDDHHMMSS_enhance_organization_management.sql`**

**-- +migrate Up**
```sql
-- Add organization management fields
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS settings JSONB DEFAULT '{}';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS created_by_user_id BIGINT NULL REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS subscription_plan VARCHAR(50) DEFAULT 'free';
ALTER TABLE orgs ADD COLUMN IF NOT EXISTS max_users INTEGER DEFAULT 10;

-- Add organization settings index for JSON queries
CREATE INDEX idx_orgs_settings ON orgs USING GIN (settings) WHERE settings IS NOT NULL;
CREATE INDEX idx_orgs_active ON orgs(is_active, created_at);
CREATE INDEX idx_orgs_plan ON orgs(subscription_plan, is_active);

-- Add audit trail for user role changes
CREATE TABLE user_role_history (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    old_role_id BIGINT NULL REFERENCES roles(id) ON DELETE SET NULL,
    new_role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    changed_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for audit trail queries
CREATE INDEX idx_user_role_history_user ON user_role_history(user_id, created_at);
CREATE INDEX idx_user_role_history_org ON user_role_history(org_id, created_at);
CREATE INDEX idx_user_role_history_changed_by ON user_role_history(changed_by_user_id, created_at);
```

**-- +migrate Down**
```sql
-- Drop audit trail
DROP INDEX IF EXISTS idx_user_role_history_changed_by;
DROP INDEX IF EXISTS idx_user_role_history_org;
DROP INDEX IF EXISTS idx_user_role_history_user;
DROP TABLE IF EXISTS user_role_history;

-- Drop organization indexes
DROP INDEX IF EXISTS idx_orgs_plan;
DROP INDEX IF EXISTS idx_orgs_active;
DROP INDEX IF EXISTS idx_orgs_settings;

-- Remove organization columns
ALTER TABLE orgs DROP COLUMN IF EXISTS max_users;
ALTER TABLE orgs DROP COLUMN IF EXISTS subscription_plan;
ALTER TABLE orgs DROP COLUMN IF EXISTS created_by_user_id;
ALTER TABLE orgs DROP COLUMN IF EXISTS is_active;
ALTER TABLE orgs DROP COLUMN IF EXISTS settings;
```

**Step 3: Create Migration for Enhanced Business Table Indexing**
```bash
dbmate new optimize_business_table_indexes
```

**Migration File: `db/migrations/YYYYMMDDHHMMSS_optimize_business_table_indexes.sql`**

**-- +migrate Up**
```sql
-- Optimize existing business tables for multi-tenant performance
-- These compound indexes support both org filtering and common query patterns

-- Reviews table optimization
CREATE INDEX IF NOT EXISTS idx_reviews_org_created ON reviews(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reviews_org_status ON reviews(org_id, status);
CREATE INDEX IF NOT EXISTS idx_reviews_org_reviewer ON reviews(org_id, reviewer_id);

-- AI Comments table optimization  
CREATE INDEX IF NOT EXISTS idx_ai_comments_org_review ON ai_comments(org_id, review_id);
CREATE INDEX IF NOT EXISTS idx_ai_comments_org_created ON ai_comments(org_id, created_at DESC);

-- AI Connectors table optimization
CREATE INDEX IF NOT EXISTS idx_ai_connectors_org_active ON ai_connectors(org_id, is_active);
CREATE INDEX IF NOT EXISTS idx_ai_connectors_org_type ON ai_connectors(org_id, connector_type);

-- Integration Tokens table optimization
CREATE INDEX IF NOT EXISTS idx_integration_tokens_org_provider ON integration_tokens(org_id, provider);
CREATE INDEX IF NOT EXISTS idx_integration_tokens_org_active ON integration_tokens(org_id, is_active);

-- Recent Activity table optimization
CREATE INDEX IF NOT EXISTS idx_recent_activity_org_created ON recent_activity(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recent_activity_org_type ON recent_activity(org_id, activity_type);

-- Dashboard Cache table optimization
CREATE INDEX IF NOT EXISTS idx_dashboard_cache_org_key ON dashboard_cache(org_id, cache_key);
CREATE INDEX IF NOT EXISTS idx_dashboard_cache_org_expires ON dashboard_cache(org_id, expires_at);

-- Webhook Registry table optimization
CREATE INDEX IF NOT EXISTS idx_webhook_registry_org_active ON webhook_registry(org_id, is_active);
CREATE INDEX IF NOT EXISTS idx_webhook_registry_org_event ON webhook_registry(org_id, event_type);
```

**-- +migrate Down**
```sql
-- Drop all the compound indexes
DROP INDEX IF EXISTS idx_webhook_registry_org_event;
DROP INDEX IF EXISTS idx_webhook_registry_org_active;
DROP INDEX IF EXISTS idx_dashboard_cache_org_expires;
DROP INDEX IF EXISTS idx_dashboard_cache_org_key;
DROP INDEX IF EXISTS idx_recent_activity_org_type;
DROP INDEX IF EXISTS idx_recent_activity_org_created;
DROP INDEX IF EXISTS idx_integration_tokens_org_active;
DROP INDEX IF EXISTS idx_integration_tokens_org_provider;
DROP INDEX IF EXISTS idx_ai_connectors_org_type;
DROP INDEX IF EXISTS idx_ai_connectors_org_active;
DROP INDEX IF EXISTS idx_ai_comments_org_created;
DROP INDEX IF EXISTS idx_ai_comments_org_review;
DROP INDEX IF EXISTS idx_reviews_org_reviewer;
DROP INDEX IF EXISTS idx_reviews_org_status;
DROP INDEX IF EXISTS idx_reviews_org_created;
```

**Step 4: Apply All Migrations**
```bash
# Apply all migrations in order
dbmate up

# Verify migration status
dbmate status
```

**Step 5: Verify Database Schema**
```bash
# Check that all tables and indexes are created properly
dbmate dump > current_schema.sql
```

### Architectural Approach

**Role-Based User Management**
Unlike typical SaaS where users self-register, we're building a role-based admin-managed system where:
- **Organization owners control user creation within their organizations**
- **Super admins have global user management capabilities across all organizations**
- **Clear permission boundaries prevent unauthorized access**

**Why this pattern:**
- **Organizational control** - Each organization controls its own users
- **Super admin oversight** - Global administrators can manage any organization
- **Security** - No open registration, controlled access at org level
- **B2B focused** - Matches enterprise customer expectations where org owners manage their teams
- **Compliance** - Clear audit trails showing who managed which users

### Implementation Strategy

**Location: `internal/api/users/`** (New package)
- **Why separate package:** User management is distinct from authentication
- **Benefits:** Clear separation of concerns, easier testing

**Service Architecture:**
```go
type UserManagementService struct {
    db          *sql.DB
    authService *auth.TokenService
    emailService *EmailService // Future: invitation emails
}
```

**Key Methods & Role-Based Logic:**
- `CreateUserInOrg(creatorUserID, orgID int64, email, password string, roleID int64)` - Org owners create users in their orgs, super admins can create in any org
- `UpdateUserInOrg(updaterUserID, targetUserID, orgID int64, updates map[string]interface{})` - Role-based user updates
- `DeactivateUserInOrg(deactivatorUserID, targetUserID, orgID int64)` - Soft delete with permission checks
- `GetOrgUsers(requesterUserID, orgID int64)` - Returns users based on requester's permissions
- `ChangeUserRole(changerUserID, targetUserID, orgID, newRoleID int64)` - Only owners can change roles in their org
- `ValidateUserManagementPermission(userID, targetOrgID int64, action string)` - Central permission validation

### UI/API Strategy

**Role-Based Dashboard Access:**
- **Super Admin Dashboard: `ui/src/pages/super-admin/`**
  - Global user management across all organizations
  - Organization management and oversight
  - System-wide user analytics and audit logs
- **Organization Dashboard: `ui/src/pages/org-admin/`**
  - User management within owned organizations only
  - Role assignment for org members
  - Org-specific user analytics
- **Member Dashboard: `ui/src/pages/member/`**
  - Read-only view of organization members
  - Own profile management only

**API Endpoints with Role-Based Access:**
```
# Organization-scoped user management (Org Owners + Super Admins)
POST /api/v1/orgs/:org_id/users          - Create user in specific org
GET  /api/v1/orgs/:org_id/users          - List org users (role-filtered results)
PUT  /api/v1/orgs/:org_id/users/:user_id - Update user in org
DELETE /api/v1/orgs/:org_id/users/:user_id - Deactivate user in org
PUT  /api/v1/orgs/:org_id/users/:user_id/role - Change user role in org

# Global user management (Super Admins only)
GET  /api/v1/admin/users                 - List all users across orgs
POST /api/v1/admin/users/bulk            - Bulk user operations
GET  /api/v1/admin/orgs/:org_id/users    - View any org's users

# Self-management (All authenticated users)
GET  /api/v1/users/profile               - Get own profile
PUT  /api/v1/users/profile               - Update own profile
PUT  /api/v1/users/password              - Change own password
```

**Why org-scoped URLs with role-based access:** 
- **Clear data isolation boundaries** - Users belong to specific organizations
- **Permission enforcement** - Middleware validates org ownership or super admin status
- **Intuitive API design** - Matches user mental model (org owners manage their org users)
- **Audit trail clarity** - Easy to track who managed users in which organization
- **Scalable permissions** - Easy to add more granular permissions within organizations

## 3. External Authentication Preparation (Authentik Integration) 🔗

### Problem Statement
Future requirement: Enterprise customers want SSO integration with their identity providers (Authentik, Active Directory, etc.) while maintaining the same user experience.

### Strategic Approach

**Identity Provider Abstraction**
We'll build an identity provider interface that treats local users and external users uniformly after authentication.

**Why abstraction layer:**
- **Seamless integration** - External users get same org roles/permissions
- **Consistent UX** - Same dashboard regardless of auth source
- **Gradual rollout** - Can enable per-organization
- **Future flexibility** - Easy to add multiple providers

### External Authentication Database Migration

**Step 1: Create Migration for External Auth Provider Support**
```bash
dbmate new add_external_auth_providers
```

**Migration File: `db/migrations/YYYYMMDDHHMMSS_add_external_auth_providers.sql`**

**-- +migrate Up**
```sql
-- Identity provider configuration table
CREATE TABLE identity_providers (
    id BIGSERIAL PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    provider_type VARCHAR(50) NOT NULL, -- 'local', 'authentik', 'ldap', 'google', etc.
    provider_name VARCHAR(100) NOT NULL, -- User-friendly name
    configuration JSONB NOT NULL DEFAULT '{}', -- Provider-specific config
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Only one default provider per org
CREATE UNIQUE INDEX idx_identity_providers_default_per_org 
ON identity_providers(org_id) 
WHERE is_default = true;

-- Indexes for provider lookups
CREATE INDEX idx_identity_providers_org_type ON identity_providers(org_id, provider_type);
CREATE INDEX idx_identity_providers_org_active ON identity_providers(org_id, is_active);

-- Add external provider fields to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider_id BIGINT NULL REFERENCES identity_providers(id) ON DELETE SET NULL;
ALTER TABLE users ADD COLUMN IF NOT EXISTS external_user_id VARCHAR(255) NULL; -- Provider's user ID
ALTER TABLE users ADD COLUMN IF NOT EXISTS provider_metadata JSONB DEFAULT '{}'; -- Provider-specific user data

-- Unique constraint for external users (one external user per provider)
CREATE UNIQUE INDEX idx_users_external_unique 
ON users(provider_id, external_user_id) 
WHERE provider_id IS NOT NULL AND external_user_id IS NOT NULL;

-- Index for external user lookups
CREATE INDEX idx_users_provider ON users(provider_id, external_user_id);

-- Insert default local provider for existing organizations
INSERT INTO identity_providers (org_id, provider_type, provider_name, is_default, is_active)
SELECT id, 'local', 'Local Authentication', true, true 
FROM orgs 
WHERE NOT EXISTS (
    SELECT 1 FROM identity_providers ip WHERE ip.org_id = orgs.id
);

-- Update existing users to use local provider
UPDATE users 
SET provider_id = (
    SELECT ip.id 
    FROM identity_providers ip 
    WHERE ip.org_id = users.org_id AND ip.provider_type = 'local'
)
WHERE provider_id IS NULL;
```

**-- +migrate Down**
```sql
-- Remove provider references from users
UPDATE users SET provider_id = NULL, external_user_id = NULL, provider_metadata = '{}';

-- Drop user indexes and constraints
DROP INDEX IF EXISTS idx_users_provider;
DROP INDEX IF EXISTS idx_users_external_unique;

-- Remove user columns
ALTER TABLE users DROP COLUMN IF EXISTS provider_metadata;
ALTER TABLE users DROP COLUMN IF EXISTS external_user_id;
ALTER TABLE users DROP COLUMN IF EXISTS provider_id;

-- Drop provider indexes
DROP INDEX IF EXISTS idx_identity_providers_org_active;
DROP INDEX IF EXISTS idx_identity_providers_org_type;
DROP INDEX IF EXISTS idx_identity_providers_default_per_org;

-- Drop providers table
DROP TABLE IF EXISTS identity_providers;
```

**Step 2: Create Migration for API Token Management**
```bash
dbmate new add_api_token_management
```

**Migration File: `db/migrations/YYYYMMDDHHMMSS_add_api_token_management.sql`**

**-- +migrate Up**
```sql
-- Extend auth_tokens table for API key management
ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS org_id BIGINT NULL REFERENCES orgs(id) ON DELETE CASCADE;
ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS permissions JSONB DEFAULT '{}'; -- API-specific permissions
ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS rate_limit_requests_per_hour INTEGER DEFAULT 1000;
ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS last_rate_limit_reset TIMESTAMP DEFAULT NOW();
ALTER TABLE auth_tokens ADD COLUMN IF NOT EXISTS requests_this_hour INTEGER DEFAULT 0;

-- Update existing tokens to have org_id from their users
UPDATE auth_tokens 
SET org_id = (
    SELECT u.org_id 
    FROM users u 
    WHERE u.id = auth_tokens.user_id
)
WHERE org_id IS NULL;

-- Add constraint after data migration
ALTER TABLE auth_tokens ALTER COLUMN org_id SET NOT NULL;

-- Indexes for API token management
CREATE INDEX idx_auth_tokens_org_type ON auth_tokens(org_id, token_type);
CREATE INDEX idx_auth_tokens_rate_limit ON auth_tokens(last_rate_limit_reset) 
WHERE token_type = 'api_key' AND is_active = true;

-- API token usage tracking table
CREATE TABLE api_token_usage (
    id BIGSERIAL PRIMARY KEY,
    token_id BIGINT NOT NULL REFERENCES auth_tokens(id) ON DELETE CASCADE,
    endpoint VARCHAR(255) NOT NULL,
    method VARCHAR(10) NOT NULL,
    status_code INTEGER NOT NULL,
    response_time_ms INTEGER,
    request_size_bytes INTEGER,
    response_size_bytes INTEGER,
    user_agent TEXT,
    ip_address INET,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for usage analytics
CREATE INDEX idx_api_token_usage_token_time ON api_token_usage(token_id, created_at DESC);
CREATE INDEX idx_api_token_usage_endpoint ON api_token_usage(endpoint, created_at DESC);
CREATE INDEX idx_api_token_usage_status ON api_token_usage(status_code, created_at DESC);

-- Partitioning setup for usage table (optional but recommended for high-volume)
-- CREATE TABLE api_token_usage_y2025m01 PARTITION OF api_token_usage 
-- FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
```

**-- +migrate Down**
```sql
-- Drop usage tracking
DROP INDEX IF EXISTS idx_api_token_usage_status;
DROP INDEX IF EXISTS idx_api_token_usage_endpoint;
DROP INDEX IF EXISTS idx_api_token_usage_token_time;
DROP TABLE IF EXISTS api_token_usage;

-- Drop API token indexes
DROP INDEX IF EXISTS idx_auth_tokens_rate_limit;
DROP INDEX IF EXISTS idx_auth_tokens_org_type;

-- Remove API token columns
ALTER TABLE auth_tokens DROP COLUMN IF EXISTS requests_this_hour;
ALTER TABLE auth_tokens DROP COLUMN IF EXISTS last_rate_limit_reset;
ALTER TABLE auth_tokens DROP COLUMN IF EXISTS rate_limit_requests_per_hour;
ALTER TABLE auth_tokens DROP COLUMN IF EXISTS permissions;
ALTER TABLE auth_tokens DROP COLUMN IF EXISTS org_id;
```

**Step 3: Apply All External Auth Migrations**
```bash
# Apply the external auth migrations
dbmate up

# Verify the schema
dbmate status
```

**Implementation Strategy:**
- `LocalProvider` - Handles database users (current system)
- `AuthentikProvider` - OIDC integration that handles all external identity sources
  - LDAP integration: LDAP → Authentik → LiveReview
  - Active Directory: AD → Authentik → LiveReview  
  - Google SSO: Google → Authentik → LiveReview
  - SAML providers: SAML → Authentik → LiveReview

**User Linking Strategy:**
When external user first logs in:
1. Create local user record with `provider_id` field
2. Admin assigns org roles (same as local users)
3. Subsequent logins: provider validates → local user lookup → same session flow

### Benefits of This Approach
- **No code duplication** - Same authorization logic for all users
- **Admin control maintained** - External users still need org assignment
- **Audit trail preserved** - All user actions logged the same way
- **Rollback capability** - Can disable external auth without data loss

## 4. API Token System for Developers 🔑

### Problem Statement
Future requirement: Third-party developers need programmatic access to LiveReview APIs without user credentials.

### Strategic Design

**Token-as-User Pattern**
API tokens will be treated as special users in the system, allowing them to inherit the same role-based access control.

**Why this pattern:**
- **Reuse authorization logic** - No separate permission system
- **Org-scoped tokens** - API keys belong to organizations
- **Role-based** - API tokens can have specific roles (read-only, full access)
- **Audit trail** - API actions logged same as user actions

### Implementation Strategy

**Token Generation Location: `internal/api/auth/api_keys.go`**
- **Why here:** Part of unified auth system
- **Logic:** Generate long-lived tokens with specific scopes
- **Storage:** Same `auth_tokens` table with `token_type='api_key'`

**API Key Management UI: `ui/src/pages/developer/`**
- **Token generation** - Name, expiry, permissions
- **Usage tracking** - Last used, request counts
- **Revocation** - Instant token deactivation

**Security Considerations:**
- **Hashed storage** - Only token hashes stored in database
- **Scope limitation** - Tokens can't exceed creating user's permissions
- **Rate limiting** - Per-token rate limits
- **Audit logging** - All API key actions logged

## 5. Echo Server Integration Strategy ⚡

### Problem Statement
Leverage Echo's middleware system effectively without reinventing the wheel.

### Integration Approach

**Middleware Chain Architecture:**
```go
// Global middleware
e.Use(middleware.Logger())
e.Use(middleware.Recover())

// Auth middleware (custom)
e.Use(auth.TokenValidation())  // Validates JWT/API keys
e.Use(auth.UserContext())      // Sets user in Echo context

// Org-scoped routes with role-based access control
orgGroup := e.Group("/api/v1/orgs/:org_id")
orgGroup.Use(auth.OrgAccess())     // Validates org membership
orgGroup.Use(auth.RoleCheck())     // Validates role permissions

// Super admin routes (global access)
adminGroup := e.Group("/api/v1/admin")
adminGroup.Use(auth.RequireSuperAdmin()) // Super admin only

// User self-management routes (all authenticated users)
userGroup := e.Group("/api/v1/users")
userGroup.Use(auth.RequireAuth())  // Any authenticated user
```

**Why this structure:**
- **Performance** - Early auth validation prevents unnecessary processing
- **Security** - Multiple validation layers
- **Flexibility** - Easy to add/remove middleware per route group
- **Echo native** - Uses Echo's context system efficiently

**Custom Middleware Location: `internal/api/middleware/`**
- **auth.go** - Token validation and user context setting
- **org.go** - Organization access control and ownership validation
- **role.go** - Role-based permission checking (super_admin, owner, member)
- **rate_limit.go** - Future rate limiting per user/org
- **audit.go** - Request logging and audit trails with user context

**Permission Validation Logic:**
```go
// In middleware/role.go
func RequireOrgOwnerOrSuperAdmin() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            user := c.Get("user").(*models.User)
            orgID := getOrgIDFromPath(c)
            
            // Super admins can access any org
            if user.IsSuperAdmin() {
                return next(c)
            }
            
            // Check if user is owner of this specific org
            if user.IsOwnerOf(orgID) {
                return next(c)
            }
            
            return echo.NewHTTPError(http.StatusForbidden, "Insufficient permissions")
        }
    }
}
```

### Context Management Strategy

**Echo Context Usage:**
```go
// Set in middleware
c.Set("user", user)
c.Set("org", org)
c.Set("role", userRole)

// Use in handlers
user := c.Get("user").(*models.User)
org := c.Get("org").(*models.Org)
```

**Benefits:**
- **Type safety** - Consistent user/org access patterns
- **Performance** - No repeated database lookups
- **Simplicity** - Handlers focus on business logic

## RBAC Enforcement Architecture - "Unmissable & Mandatory" 🔒

### Problem Statement
RBAC enforcement must be:
- **Impossible to bypass** - Architectural patterns that force permission checks
- **Simple for developers** - Clear patterns that are easy to follow
- **AI-friendly** - Consistent patterns that AI can easily replicate
- **Compile-time safe** - Type system prevents permission mistakes
- **Runtime validated** - Multiple layers of validation

### Strategic Approach: "Security by Construction"

**1. Context-Driven Architecture**
Instead of checking permissions in handlers, we build permission context into the request flow itself, while allowing for necessary public endpoints.

**Public vs Protected Endpoint Strategy:**
- **Public endpoints** - System setup, health checks, initial admin creation
- **Protected endpoints** - All business logic requiring authentication and authorization
- **Permission context** - Built for authenticated requests, skipped for public endpoints

**2. Multi-Tier Security Architecture**
```go
// Public endpoints - no authentication required
publicGroup := e.Group("/api/v1/public")
publicGroup.POST("/setup/admin", s.CreateFirstAdmin)     // Only works if no super admin exists
publicGroup.GET("/health", s.HealthCheck)               // System health
publicGroup.GET("/setup/status", s.GetSetupStatus)      // Check if system is initialized

// Authenticated endpoints - requires valid token but no org context
authGroup := e.Group("/api/v1/auth")
authGroup.Use(auth.RequireAuth())
authGroup.GET("/me", s.GetCurrentUser)
authGroup.POST("/logout", s.Logout)

// Organization-scoped endpoints - requires org membership and role validation
orgGroup := e.Group("/api/v1/orgs/:org_id")
orgGroup.Use(auth.RequireAuth())           // 1. Validate user exists
orgGroup.Use(auth.BuildOrgContext())       // 2. Build org context
orgGroup.Use(auth.ValidateOrgAccess())     // 3. Validate org access
orgGroup.Use(auth.BuildPermissionContext()) // 4. Build permission context
orgGroup.POST("/users", s.CreateUser)      // Permission context guaranteed

// Super admin endpoints - requires super admin role
adminGroup := e.Group("/api/v1/admin")
adminGroup.Use(auth.RequireAuth())
adminGroup.Use(auth.RequireSuperAdmin())
adminGroup.GET("/users", s.GetAllUsers)
```

**3. Type-Safe Permission Context (For Protected Endpoints)**
```go
// Every authenticated handler receives a PermissionContext, not raw user data
type PermissionContext struct {
    User           *models.User
    CurrentOrg     *models.Org
    Role           string
    Permissions    []Permission
    IsOrgOwner     bool
    IsSuperAdmin   bool
}

// Public handlers use different pattern - no permission context needed
func (s *Server) CreateFirstAdmin(c echo.Context) error {
    // Public endpoint logic - validates no existing super admin
    existingSuperAdmin, err := s.userService.HasSuperAdmin()
    if err != nil {
        return err
    }
    if existingSuperAdmin {
        return echo.NewHTTPError(http.StatusConflict, "Super admin already exists")
    }
    
    // Create first admin logic here
    return s.userService.CreateFirstSuperAdmin(email, password)
}

// Protected handlers use permission context
func (s *Server) CreateUser(c echo.Context) error {
    pctx := GetPermissionContext(c) // Guaranteed to be validated by middleware
    
    // Context already knows what user can do - no manual checking needed
    if !pctx.CanManageUsers() {
        return echo.NewHTTPError(http.StatusForbidden)
    }
    
    // Business logic here - permissions already enforced
}
```

**3. Middleware-Enforced Context Building (For Protected Routes)**
```go
// Public routes - no middleware chain needed
func SetupPublicRoutes(e *echo.Echo) *echo.Group {
    publicGroup := e.Group("/api/v1/public")
    // No authentication middleware - these are intentionally public
    return publicGroup
}

// Authenticated routes - basic auth validation only
func SetupAuthRoutes(e *echo.Echo) *echo.Group {
    authGroup := e.Group("/api/v1/auth")
    authGroup.Use(auth.RequireAuth()) // Only validates token exists
    return authGroup
}

// Organization-scoped routes - full permission context building
func SetupOrgRoutes(e *echo.Echo) *echo.Group {
    orgGroup := e.Group("/api/v1/orgs/:org_id")
    
    // Mandatory chain for org-scoped operations
    orgGroup.Use(auth.RequireAuth())           // 1. Validate user exists
    orgGroup.Use(auth.BuildOrgContext())       // 2. Build org context
    orgGroup.Use(auth.ValidateOrgAccess())     // 3. Validate org access
    orgGroup.Use(auth.BuildPermissionContext()) // 4. Build permission context
    
    // By the time handler runs, PermissionContext is guaranteed valid
    return orgGroup
}

// Super admin routes - requires super admin validation
func SetupSuperAdminRoutes(e *echo.Echo) *echo.Group {
    adminGroup := e.Group("/api/v1/admin")
    adminGroup.Use(auth.RequireAuth())
    adminGroup.Use(auth.RequireSuperAdmin())
    return adminGroup
}
```

**4. Service Layer with Built-in Permissions**
```go
// Services receive PermissionContext, not raw parameters
type UserService struct {
    db *sql.DB
}

func (s *UserService) CreateUser(pctx *PermissionContext, email, password string) (*models.User, error) {
    // Service validates permissions - handler cannot bypass
    if !pctx.CanCreateUsersInOrg(pctx.CurrentOrg.ID) {
        return nil, ErrInsufficientPermissions
    }
    
    // Permission check is built into the service call
    return s.createUserImpl(pctx.CurrentOrg.ID, email, password)
}
```

### Implementation Strategy: "Fail-Safe Defaults"

**File Structure for Multi-Tier Security Patterns:**
```
internal/api/
├── auth/
│   ├── context.go          # PermissionContext type and builders
│   ├── middleware.go       # Authentication middleware for different route types
│   └── permissions.go      # Permission checking logic
├── handlers/
│   ├── public_handler.go   # Public endpoints (no auth required)
│   ├── auth_handler.go     # Basic auth endpoints (token validation only)
│   ├── base_handler.go     # Base handler with permission helpers
│   ├── org_handler.go      # Org-scoped handlers (full permission context)
│   └── admin_handler.go    # Super admin handlers
└── services/
    ├── setup_service.go    # System setup and initialization (used by public endpoints)
    ├── base_service.go     # Base service with permission validation
    └── user_service.go     # Permission-aware service methods
```

**1. Public Handler Pattern (No Authentication)**
```go
// File: internal/api/handlers/public_handler.go
type PublicHandler struct {
    setupService *services.SetupService
    userService  *services.UserService
}

// Public endpoints use different validation logic
func (h *PublicHandler) CreateFirstAdmin(c echo.Context) error {
    // Validate no existing super admin exists
    hasAdmin, err := h.setupService.HasSuperAdmin()
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, "Setup check failed")
    }
    if hasAdmin {
        return echo.NewHTTPError(http.StatusConflict, "System already initialized")
    }
    
    // Parse request and create first admin
    var req CreateAdminRequest
    if err := c.Bind(&req); err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "Invalid request")
    }
    
    admin, err := h.userService.CreateFirstSuperAdmin(req.Email, req.Password, req.OrgName)
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, "Admin creation failed")
    }
    
    return c.JSON(http.StatusCreated, admin)
}

func (h *PublicHandler) GetSetupStatus(c echo.Context) error {
    status, err := h.setupService.GetSystemStatus()
    if err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, "Status check failed")
    }
    
    return c.JSON(http.StatusOK, status)
}
```

**2. Protected Handler Pattern (Permission Context Required)**
```go
// File: internal/api/handlers/base_handler.go
type BaseHandler struct {
    db *sql.DB
}

// ALL handlers must use this - no direct echo.Context access
func (h *BaseHandler) WithOrgPermissions(handler func(*PermissionContext) error) echo.HandlerFunc {
    return func(c echo.Context) error {
        pctx := GetPermissionContext(c) // Middleware guaranteed this exists
        if pctx == nil {
            panic("Permission context not found - middleware misconfigured")
        }
        return handler(pctx)
    }
}

// Example usage - compiler enforces permission context
func (s *Server) CreateUser(c echo.Context) error {
    return s.WithOrgPermissions(func(pctx *PermissionContext) error {
        // pctx is guaranteed valid - no way to bypass
        if !pctx.CanManageUsers() {
            return echo.NewHTTPError(http.StatusForbidden)
        }
        
        // Business logic with guaranteed permissions
        return s.userService.CreateUser(pctx, email, password)
    })(c)
}
```

**2. Type-Safe Permission Methods**
```go
// File: internal/api/auth/context.go
type PermissionContext struct {
    User         *models.User
    CurrentOrg   *models.Org
    Role         string
    orgID        int64  // Private - forces using CurrentOrg
}

// These methods make permissions explicit and unmissable
func (p *PermissionContext) CanManageUsers() bool {
    return p.Role == "owner" || p.User.IsSuperAdmin
}

func (p *PermissionContext) CanManageUsersInOrg(orgID int64) bool {
    if p.User.IsSuperAdmin {
        return true // Super admin can manage any org
    }
    return p.Role == "owner" && p.CurrentOrg.ID == orgID
}

func (p *PermissionContext) CanViewOrg(orgID int64) bool {
    if p.User.IsSuperAdmin {
        return true
    }
    return p.CurrentOrg.ID == orgID // User must be member of org
}

// Force org-scoped queries
func (p *PermissionContext) GetOrgID() int64 {
    return p.CurrentOrg.ID // Cannot be bypassed
}
```

**3. Mandatory Middleware Chain**
```go
// File: internal/api/auth/middleware.go

// This function sets up the ONLY way to create org routes
func SetupOrgRoutes(e *echo.Echo, services *Services) *echo.Group {
    orgGroup := e.Group("/api/v1/orgs/:org_id")
    
    // Mandatory chain - cannot be modified
    orgGroup.Use(RequireAuth())           // Extract user from JWT
    orgGroup.Use(BuildOrgContext())       // Load org from URL param
    orgGroup.Use(ValidateOrgMembership()) // Ensure user belongs to org
    orgGroup.Use(BuildPermissionContext()) // Create PermissionContext
    
    return orgGroup
}

// Middleware that builds PermissionContext
func BuildPermissionContext() echo.MiddlewareFunc {
    return func(next echo.HandlerFunc) echo.HandlerFunc {
        return func(c echo.Context) error {
            user := c.Get("user").(*models.User)
            org := c.Get("org").(*models.Org)
            
            // Get user's role in this org
            role, err := getUserRoleInOrg(user.ID, org.ID)
            if err != nil {
                return echo.NewHTTPError(http.StatusForbidden, "No access to organization")
            }
            
            pctx := &PermissionContext{
                User:       user,
                CurrentOrg: org,
                Role:       role,
            }
            
            c.Set("permission_context", pctx)
            return next(c)
        }
    }
}
```

**4. Service Layer with Built-in Validation**
```go
// File: internal/api/services/base_service.go
type BaseService struct {
    db *sql.DB
}

// ALL service methods must receive PermissionContext
func (s *BaseService) validatePermission(pctx *PermissionContext, required Permission) error {
    if !pctx.HasPermission(required) {
        return ErrInsufficientPermissions
    }
    return nil
}

// File: internal/api/services/user_service.go
func (s *UserService) CreateUser(pctx *PermissionContext, email, password string) (*models.User, error) {
    // Permission validation is built into every service call
    if err := s.validatePermission(pctx, PermissionManageUsers); err != nil {
        return nil, err
    }
    
    // All queries are automatically org-scoped through context
    return s.createUserInOrg(pctx.GetOrgID(), email, password)
}

func (s *UserService) GetUsers(pctx *PermissionContext) ([]*models.User, error) {
    if err := s.validatePermission(pctx, PermissionViewUsers); err != nil {
        return nil, err
    }
    
    // Impossible to forget org_id filter - it's built into the context
    query := "SELECT * FROM users WHERE org_id = $1"
    return s.queryUsers(query, pctx.GetOrgID())
}
```

### "Unmissable" Patterns for Developers/AI

**1. Code Generation Templates**
```go
// Template for new handlers - AI/developers copy this pattern
func (s *Server) New{{.Resource}}Handler(c echo.Context) error {
    return s.WithOrgPermissions(func(pctx *PermissionContext) error {
        // Step 1: Permission check (unmissable - compiler enforces)
        if !pctx.Can{{.Action}}() {
            return echo.NewHTTPError(http.StatusForbidden)
        }
        
        // Step 2: Service call with context (unmissable - service requires it)
        result, err := s.{{.service}}.{{.Action}}(pctx, params...)
        if err != nil {
            return err
        }
        
        return c.JSON(http.StatusOK, result)
    })
}
```

**2. Lint Rules (Enforced at CI)**
```go
// Custom linter rules that fail CI if violated
// Rule 1: No direct database access in handlers
// Rule 2: All service methods must accept PermissionContext as first param
// Rule 3: No direct user/org access - must use PermissionContext methods
// Rule 4: All org-scoped routes must use SetupOrgRoutes()
```

**3. Test Helpers (Make Testing Easy)**
```go
// File: internal/api/testing/helpers.go
func CreateTestPermissionContext(userRole string, orgID int64) *PermissionContext {
    // Creates valid context for testing
}

func TestWithPermissions(t *testing.T, role string, test func(*PermissionContext)) {
    // Helper that ensures all tests include permission context
}

// Example test - pattern is unmissable
func TestCreateUser(t *testing.T) {
    TestWithPermissions(t, "owner", func(pctx *PermissionContext) {
        user, err := userService.CreateUser(pctx, "test@example.com", "password")
        assert.NoError(t, err)
        assert.Equal(t, pctx.GetOrgID(), user.OrgID)
    })
}
```

**4. Database Query Patterns (Impossible to Forget)**
```go
// File: internal/api/services/query_builder.go
type OrgScopedQuery struct {
    orgID int64
    query strings.Builder
}

// All queries must go through this - org_id is automatic
func (s *BaseService) NewOrgQuery(pctx *PermissionContext, table string) *OrgScopedQuery {
    return &OrgScopedQuery{
        orgID: pctx.GetOrgID(),
        query: strings.Builder{},
    }.Table(table)
}

func (q *OrgScopedQuery) Select(fields string) *OrgScopedQuery {
    q.query.WriteString(fmt.Sprintf("SELECT %s FROM %s WHERE org_id = %d", fields, q.table, q.orgID))
    return q
}

// Usage - org_id filter is automatic and unmissable
func (s *UserService) GetUsers(pctx *PermissionContext) ([]*models.User, error) {
    query := s.NewOrgQuery(pctx, "users").
        Select("*").
        Build()
    
    return s.queryUsers(query) // org_id filter already included
}
```

### Database Strategy

**Consistent Foreign Keys:**
Every business table has `org_id BIGINT NOT NULL REFERENCES orgs(id)`

**Index Strategy:**
```sql
-- Compound indexes for performance
CREATE INDEX idx_reviews_org_created ON reviews(org_id, created_at);
CREATE INDEX idx_ai_comments_org_review ON ai_comments(org_id, review_id);
```

**Why compound indexes:**
- **Performance** - Fast org-scoped queries with sorting
- **Query optimization** - Single index serves multiple query patterns
- **Storage efficiency** - Fewer indexes to maintain

## Implementation Phases & Database Migration Workflow 📋

### Phase 2A: Foundation - Session Management (Week 1)
**Database Changes:**
```bash
# Step 1: Create session management migration
dbmate new create_session_management
# Edit the migration file as shown above
dbmate up

# Step 2: Verify migration applied successfully
dbmate status
```

**Implementation Tasks:**
- ✅ Database schema for unified tokens (migration created and applied)
- ✅ Echo middleware integration  
- ✅ Basic JWT handling
- ✅ Session cleanup service with gocron scheduler

**Why first:** Everything else depends on solid auth foundation

### Phase 2B: User Management (Week 2)  
**Database Changes:**
```bash
# Step 1: Create user invitation system
dbmate new create_user_invitations_system
# Edit migration file as shown above
dbmate up

# Step 2: Create enhanced org management
dbmate new enhance_organization_management  
# Edit migration file as shown above
dbmate up

# Step 3: Verify all migrations
dbmate status
```

**Implementation Tasks:**
- ✅ User invitation database schema (migration created)
- ✅ Organization management enhancements (migration created)
- ✅ User CRUD APIs with role-based permissions
- ✅ Organization owner dashboard for user management
- ✅ Super admin global user management interface
- ✅ Member self-service profile management

**Why second:** Enables testing with multiple users and proper role isolation

### Phase 2C: Data Isolation & Performance (Week 3)
**Database Changes:**
```bash
# Step 1: Optimize business table indexes
dbmate new optimize_business_table_indexes
# Edit migration file as shown above  
dbmate up

# Step 2: Verify index creation
dbmate status

# Step 3: Test query performance
# Run EXPLAIN ANALYZE on key queries to verify index usage
```

**Implementation Tasks:**
- ✅ Compound indexes for all business tables (migration created)
- ✅ Organization context middleware implementation
- ✅ Service layer updates for org-scoped queries
- ✅ Data isolation testing and verification
- ✅ Performance benchmarking with org_id filters

**Why third:** Critical security foundation before advanced features

### Phase 2D: External Auth Preparation (Week 4)
**Database Changes:**
```bash
# Step 1: Add external auth provider support
dbmate new add_external_auth_providers
# Edit migration file as shown above
dbmate up

# Step 2: Add API token management  
dbmate new add_api_token_management
# Edit migration file as shown above
dbmate up

# Step 3: Verify external auth schema
dbmate status
```

**Implementation Tasks:**
- ✅ Identity provider abstraction layer (database schema ready)
- ✅ Local provider implementation
- ✅ API token management system (database schema ready)
- ✅ Future Authentik/OIDC preparation

**Why fourth:** Can develop/test with local users while preparing for external

### Phase 2E: API Tokens & Polish (Week 5)
**Database Changes:**
```bash
# No new migrations needed - using existing auth_tokens schema
# Verify all migrations are applied
dbmate status

# Optional: Create usage analytics views
dbmate new create_analytics_views
# Add SQL views for token usage analytics
dbmate up
```

**Implementation Tasks:**
- ✅ API token generation UI and backend
- ✅ Developer dashboard for token management
- ✅ Rate limiting implementation
- ✅ Usage analytics and monitoring
- ✅ Documentation and testing

**Why last:** Builds on all previous auth infrastructure

### Database Migration Safety & Rollback Strategy

**Pre-Migration Backup:**
```bash
# Always backup before major migrations
pg_dump livereview_db > backup_before_phase2_$(date +%Y%m%d_%H%M%S).sql
```

**Migration Verification Steps:**
```bash
# 1. Check migration status
dbmate status

# 2. Verify table structure
psql livereview_db -c "\d+ auth_tokens"
psql livereview_db -c "\d+ user_invitations"  
psql livereview_db -c "\d+ identity_providers"

# 3. Check indexes are created
psql livereview_db -c "\di+ *auth_tokens*"
psql livereview_db -c "\di+ *user_invitations*"

# 4. Verify constraints
psql livereview_db -c "SELECT conname, contype FROM pg_constraint WHERE conrelid = 'auth_tokens'::regclass;"
```

**Rollback Procedure (if needed):**
```bash
# Rollback specific migrations (be careful with data loss)
dbmate rollback  # Rolls back last migration
dbmate down     # Rolls back all migrations (DESTRUCTIVE)

# Restore from backup if needed
psql livereview_db < backup_before_phase2_YYYYMMDD_HHMMSS.sql
```

**Production Migration Best Practices:**
```bash
# 1. Test migrations on staging environment first
export DATABASE_URL="postgres://user:pass@staging-host/livereview_db"
dbmate up

# 2. Apply with monitoring on production
export DATABASE_URL="postgres://user:pass@prod-host/livereview_db"  
dbmate up

# 3. Monitor application logs for errors
tail -f /var/log/livereview/app.log

# 4. Verify data integrity
psql livereview_db -c "SELECT COUNT(*) FROM auth_tokens;"
psql livereview_db -c "SELECT COUNT(*) FROM users WHERE provider_id IS NOT NULL;"
```

## Success Metrics & Validation 📊

### Security Validation
- **Data isolation testing** - Verify no cross-org data leakage
- **Permission boundary testing** - Ensure role restrictions work
- **Token security** - Validate hashing, expiry, revocation

### Performance Metrics
- **Authentication latency** - < 50ms for token validation
- **Database query optimization** - All org queries use indexes
- **Memory usage** - Efficient context management

### Developer Experience
- **API clarity** - Clear org-scoped endpoint patterns
- **Error messages** - Helpful auth/permission error responses
- **Documentation** - Complete API token setup guides

This strategic approach gives us a robust, maintainable multi-tenant authentication system that scales from simple admin-managed users to enterprise SSO integration while leveraging Echo's strengths and minimizing custom code maintenance.

## Implementation Order & Dependencies 📋

### Phase 2A: Core Authentication (Week 1)
1. ✅ Database migrations for sessions
2. ✅ Session service implementation  
3. ✅ JWT middleware
4. ✅ Update auth endpoints
5. ✅ Test authentication flow

### Phase 2B: Organization Management (Week 2)
1. ✅ Database migrations for org management
2. ✅ Org service implementation
3. ✅ Organization CRUD APIs
4. ✅ Org context middleware
5. ✅ Test org operations

### Phase 2C: User Management (Week 3)
1. ✅ Database migrations for invitations
2. ✅ User invitation system
3. ✅ User management APIs
4. ✅ Email notification system
5. ✅ Test invitation flow

### Phase 2D: Data Isolation (Week 4)
1. ✅ Update all services for org filtering
2. ✅ Update all handlers with org context
3. ✅ Implement role-based access control
4. ✅ Test data isolation
5. ✅ Performance optimization

### Phase 2E: CLI & UI Updates (Week 5)
1. ✅ Update CLI authentication
2. ✅ Add org management commands
3. ✅ Update frontend auth flow
4. ✅ Add org switcher component
5. ✅ Test end-to-end flows

## Testing Strategy 🧪

### Unit Tests
- Session service methods
- Organization service methods
- User invitation service methods
- Middleware functions
- JWT token handling

### Integration Tests
- Complete auth flow (login → org selection → API calls)
- Invitation flow (invite → accept → login)
- Organization switching
- Data isolation verification
- Role-based access control

### End-to-End Tests
- Multi-user scenarios
- Cross-organization data leakage prevention
- Permission boundary testing
- CLI command flows
- UI navigation flows

## Security Considerations 🔐

### JWT Token Management
- Short-lived access tokens (15 minutes)
- Refresh tokens with rotation
- Secure token storage
- Token revocation on logout

### Data Isolation
- Row-level security with org_id filters
- Middleware validation on every request
- Database constraints to prevent cross-org access
- Audit logging for all org operations

### Role-based Access Control
- **Principle of least privilege** - Users get minimum permissions needed for their role
- **Role inheritance** - Owners can do everything members can do, super admins can do everything
- **Organization boundaries** - Owners can only manage users in organizations they own
- **Super admin override** - Super admins can manage users in any organization with full audit logging
- **Permission validation at multiple layers** - Middleware, service layer, and database constraints

## Performance Considerations ⚡

### Database Optimization
- Proper indexing on org_id columns
- Query optimization for org-scoped data
- Connection pooling for multi-tenant load
- Periodic cleanup of expired sessions/invitations

### Caching Strategy
- User session caching
- Organization context caching
- Role permission caching
- JWT token validation caching

### Monitoring & Observability
- Org-level metrics and dashboards
- Authentication failure monitoring
- Permission denied tracking
- Performance metrics per organization

## Migration Strategy for Existing Data 📦

### Self-hosted to SaaS Transition
1. Existing single-admin deployments keep working
2. Admin can create additional orgs if needed
3. Legacy auth endpoints remain functional
4. Gradual migration to new auth system
5. Clear upgrade path documentation

### Data Integrity Verification
- Verify all existing data assigned to default org (ID: 1)
- Ensure no orphaned records
- Validate foreign key constraints
- Test rollback procedures

This comprehensive Phase 2 plan will transform your LiveReview application into a fully functional multi-tenant SaaS platform while maintaining backward compatibility and ensuring security, performance, and usability.