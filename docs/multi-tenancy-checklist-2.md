# Phase 2 Multi-Tenancy Implementation Checklist 🚀

## Implementation Status: **PHASE 2A COMPLETE - READY FOR PHASE 2B**

**Current State Analysis:**
- ✅ Phase 1 Foundation: Basic multi-tenancy tables exist (`orgs`, `users`, `roles`, `user_roles`)
- ✅ Org ID Integration: All business tables have `org_id` columns with foreign key constraints
- ✅ Basic User Service: `CreateFirstAdminUser()`, `MigrateExistingAdminPassword()`, and `CheckSetupStatus()` implemented
- ✅ **Migration Files**: Phase 1 migration files are properly populated and applied
- ✅ **PHASE 2A COMPLETE**: JWT session management system implemented and working
- ✅ **PHASE 2A COMPLETE**: Authentication service layer with TokenService and middleware
- ✅ **PHASE 2A COMPLETE**: Auth API endpoints (login, logout, refresh, setup, me)
- ✅ **PHASE 2A COMPLETE**: Echo middleware chain for auth and org context
- ✅ **PHASE 2B PARTIALLY COMPLETE**: User management service layer exists (`internal/api/users/`)
- ✅ **PHASE 2B PARTIALLY COMPLETE**: User management handlers implemented but not registered
- ❌ User management APIs not registered in main server (Task 10 - PRIORITY)
- ❌ Organization management APIs missing (Task 11)
- ❌ Self-service profile endpoints missing (Task 10.3)
- ❌ Super admin global endpoints missing (Task 10.2)

---

## Phase 2A: Core Authentication & Session Management (Week 1) 🔐

### Task 1: Session Management Database Schema (PRIORITY)
- [x] **1.1** Create migration: `create_session_management_system.sql`
  ```bash
  dbmate new create_session_management_system
  ```
- [x] **1.2** Implement `auth_tokens` table with:
  - JWT session tokens + refresh tokens support
  - Optimized indexes for fast lookups (partial indexes on active tokens)
  - Rate limiting fields for API tokens (future)
  - Background cleanup support
  - Configurable token expiration (default: 1 month)
- [x] **1.3** Apply migration and verify
  ```bash
  dbmate up
  dbmate status
  ```

### Task 2: Authentication Service Layer
- [x] **2.1** Create `internal/api/auth/` package structure:
  ```
  internal/api/auth/
  ├── token_service.go     # JWT + session management with refresh tokens
  ├── middleware.go        # Echo middleware for different route types
  ├── context.go          # PermissionContext and builders
  ├── permissions.go      # Permission validation logic
  └── handlers.go         # Auth API handlers
  ```
- [x] **2.2** Implement `TokenService` with:
  - JWT generation and validation (access + refresh tokens)
  - Session lifecycle management (create, validate, refresh, revoke)
  - Background cleanup scheduler with periodic cleanup
  - Configurable token expiration (15min access, 30 days refresh)
  - Token refresh endpoint for seamless UI experience
- [x] **2.3** Implement Echo middleware chain:
  - `RequireAuth()` - Basic token validation
  - `BuildOrgContext()` - Organization context building
  - `ValidateOrgAccess()` - Org membership validation
  - `BuildPermissionContext()` - Permission context for handlers

### Task 3: Enhanced User Authentication APIs
- [x] **3.1** **REPLACE EXISTING AUTH SYSTEM** - Remove legacy password header auth
- [x] **3.2** Create new auth endpoints in `internal/api/`:
  ```
  POST /api/v1/auth/login        # User login with email/password (returns access + refresh tokens)
  POST /api/v1/auth/logout       # Logout and token revocation
  GET  /api/v1/auth/me          # Get current user info
  POST /api/v1/auth/refresh     # Refresh access token using refresh token
  ```
- [x] **3.3** Update setup endpoints:
  ```
  POST /api/v1/auth/setup       # Create first admin user (replaces password system completely)
  GET  /api/v1/auth/setup-status # Check if system is initialized
  ```
- [x] **3.4** **NO BACKWARD COMPATIBILITY** - Clean removal of old password-based auth

### Task 4: Route Architecture & Middleware Integration
- [x] **4.1** Implement multi-tier route structure:
  ```go
  // Public routes (no auth)
  public := v1.Group("")
  
  // Protected routes (require authentication)
  protected := v1.Group("")
  protected.Use(auth.RequireAuth(s.tokenService, s.db))
  
  // Org-scoped routes (full permission context) - READY FOR IMPLEMENTATION
  orgGroup := e.Group("/api/v1/orgs/:org_id")
  orgGroup.Use(auth.RequireAuth())
  orgGroup.Use(auth.BuildOrgContext())
  orgGroup.Use(auth.ValidateOrgAccess())
  orgGroup.Use(auth.BuildPermissionContext())
  
  // Super admin routes - READY FOR IMPLEMENTATION
  adminGroup := e.Group("/api/v1/admin")
  adminGroup.Use(auth.RequireAuth())
  adminGroup.Use(auth.RequireSuperAdmin())
  ```
- [x] **4.2** Implement "unmissable" permission patterns:
  - PermissionContext type with org filtering built-in
  - Service methods that enforce org boundaries automatically
  - Middleware framework ready for org_id filters

### Task 5: React UI Integration & Token Management
- [x] **5.1** Update React auth system (studying existing structure from `ui/src/`):
  - Update `src/api/auth.ts` for new JWT-based endpoints ✅ IMPLEMENTED
  - Implement token storage (localStorage + httpOnly cookies for refresh tokens) ✅ IMPLEMENTED
  - Add automatic token refresh logic in API client ✅ IMPLEMENTED
  - Update auth context/store (existing Redux structure in `src/store/Auth/`) ✅ IMPLEMENTED
- [x] **5.2** Minimal UI updates for immediate functionality:
  - Convert existing login to email/password (instead of admin password header) ✅ IMPLEMENTED
  - Add token refresh handling in API calls ✅ IMPLEMENTED
  - Update auth guards and protected routes ✅ IMPLEMENTED
  - Add user context (organization, role) to app state ✅ IMPLEMENTED
- [ ] **5.3** User creation with temporary passwords (**DEPENDS ON TASK 10.1 & 10.3**):
  - Add "Change Password" UI component (needs Task 10.3 - password change API)
  - Force password change on first login with temporary password
  - Integrate with existing settings pages structure

### Task 6: Basic Testing & Validation (Manual)
- [ ] **6.1** Manual testing of auth flow (**DEPENDS ON TASK 10.1 COMPLETION**):
  - User login with email/password works ✅ WORKING
  - JWT tokens are properly stored and used ✅ WORKING
  - Token refresh works automatically in UI ✅ WORKING
  - Logout clears tokens and invalidates session ✅ WORKING
- [ ] **6.2** Manual testing of middleware (**READY AFTER TASK 10.1**):
  - Protected endpoints require valid tokens (needs Task 10.1 registration)
  - Org-scoped endpoints enforce membership (needs Task 10.1 registration)
  - Super admin endpoints work correctly (needs Task 10.2)
- [ ] **6.3** Basic session functionality (**NEEDS TESTING AFTER TASK 10.1**):
  - Background token cleanup doesn't interfere with active sessions
  - Database performance is reasonable for auth operations

---

## IMMEDIATE IMPLEMENTATION ROADMAP 📋

### Current Status Summary:
- ✅ **Authentication & Middleware**: Fully functional JWT system with permission context
- ✅ **User Service Layer**: Complete user management service with all CRUD operations
- ✅ **User Handlers**: All organization-scoped user management handlers implemented
- ❌ **API Registration**: Handlers exist but are not registered in main server
- ❌ **Profile Management**: Self-service profile endpoints missing
- ❌ **Super Admin APIs**: Global user management endpoints missing
- ❌ **Organization APIs**: Organization CRUD endpoints missing

### Implementation Order (Revised Based on Current State):

**PHASE 2B-1: Register Existing User Management APIs (Week 1)** ✅ **COMPLETE**
1. **Task 10.1** - Register org-scoped user management routes in main server ✅ **WORKING**
2. Create comprehensive test suite for all registered endpoints ✅ **WORKING**
3. Validate middleware chain and permission enforcement ✅ **VALIDATED**

**Test Results Summary:**
- ✅ All CRUD operations working perfectly
- ✅ JWT authentication and authorization working
- ✅ Organization scoping enforced correctly
- ✅ User creation with temporary passwords working
- ✅ Role management and updates working
- ✅ Audit trail tracking user actions
- ⚠️ Minor: Audit log endpoint needs debugging

**PHASE 2B-2: Complete Missing APIs (Week 1-2)** ✅ **COMPLETE**
4. **Task 10.3** - Implement self-service profile management endpoints ✅ **WORKING**
5. **Task 10.2** - Implement super admin global management endpoints ✅ **WORKING**
6. **Task 11** - Implement organization management APIs ✅ **COMPLETE**

**Task 10.3 Test Results Summary:**
- ✅ GET /api/v1/users/profile - Returns profile with organizations
- ✅ PUT /api/v1/users/profile - Updates first_name, last_name successfully  
- ✅ PUT /api/v1/users/password - Password changes working correctly
- ✅ JWT authentication and authorization working perfectly
- ✅ Data persistence verified (updated_at timestamps updating)
- ✅ Organization context included in profile responses

**Task 10.2 Test Results Summary:**
- ✅ GET /api/v1/admin/users - List all users across organizations
- ✅ POST /api/v1/admin/orgs/:org_id/users - Create users in any organization
- ✅ PUT /api/v1/admin/users/:user_id/org - Transfer users between organizations
- ✅ GET /api/v1/admin/analytics/users - Comprehensive user analytics
- ✅ Super admin authentication and authorization working perfectly
- ✅ All super admin endpoints properly secured with middleware

**Task 11 Test Results Summary:**
- ✅ POST /api/v1/admin/organizations - Create organizations (super admin only)
- ✅ GET /api/v1/organizations - List user's accessible organizations
- ✅ GET /api/v1/organizations/:org_id - Get organization details
- ✅ PUT /api/v1/orgs/:org_id - Update organization details (owners)
- ✅ GET /api/v1/orgs/:org_id/members - List organization members with roles
- ✅ PUT /api/v1/orgs/:org_id/members/:user_id/role - Change user roles
- ✅ GET /api/v1/orgs/:org_id/analytics - Organization analytics
- ✅ Permission context fixes: Auto-assign creator as owner of new organizations
- ✅ All organization management endpoints working perfectly

**PHASE 2B-3: UI Integration (Week 2)** ✅ **IN PROGRESS**
7. **Task 13** - Implement organization selector system and unified UI architecture ✅ **COMPLETE**
8. **Task 14** - Create role-based user management interface ✅ **COMPLETE**
9. **Task 15** - Add profile management and settings integration ✅ **IN PROGRESS**

**PHASE 2B-4: Testing & Validation (Week 2-3)**
10. **Task 6** - Comprehensive manual testing of all functionality
11. Create automated test scripts for validation
12. Performance testing for multi-tenant operations

---

## Phase 2B: User Management System (Week 2) 👥

### Task 7: Enhanced User Management Database Schema
- [x] **7.1** Create migration: `enhance_user_management_system.sql`
  ```bash
  dbmate new enhance_user_management_system
  ```
- [x] **7.2** Enhance users table with:
  - `first_name`, `last_name` columns
  - `is_active`, `last_login_at` for status tracking
  - `created_by_user_id`, `deactivated_by_user_id` for audit trail
  - `password_reset_required` BOOLEAN for temporary passwords
- [x] **7.3** Create `user_management_audit` table:
  - Complete audit trail for all user management actions
  - Tracks who did what to which user and when
- [x] **7.4** Create performance indexes:
  - Compound indexes for org-scoped queries
  - Audit trail indexes for management reports

### Task 8: Organization Management Enhancement
- [x] **8.1** Create migration: `enhance_organization_management.sql`
  ```bash
  dbmate new enhance_organization_management
  ```
- [x] **8.2** Enhance orgs table with:
  - `settings` JSONB field for organization configuration
  - `is_active`, `subscription_plan`, `max_users` for SaaS features
  - `created_by_user_id` for tracking
- [x] **8.3** Create `user_role_history` table:
  - Audit trail for role changes within organizations
  - Track who changed which user's role and why

### Task 9: User Management Service Layer
- [x] **9.1** Create `internal/api/users/` package:
  ```
  internal/api/users/
  ├── user_service.go          # Core user management ✅ IMPLEMENTED
  ├── user_management_service.go # Admin user operations (merged into user_service.go)
  └── profile_service.go       # Self-service profile management (NOT YET IMPLEMENTED)
  ```
- [x] **9.2** Implement role-based user management:
  - `CreateUserInOrg()` - Create users with temporary passwords ✅ IMPLEMENTED
  - `UpdateUserInOrg()` - Role-based user updates with audit trail ✅ IMPLEMENTED
  - `DeactivateUserInOrg()` - Soft delete with permission checks ✅ IMPLEMENTED
  - `ChangeUserRole()` - Role changes with proper validation ✅ IMPLEMENTED (via UpdateUserInOrg)
  - `ForcePasswordReset()` - Set temporary password requirement ✅ IMPLEMENTED
- [x] **9.3** Implement permission validation:
  - Central permission checking for all user operations ✅ IMPLEMENTED (PermissionContext)
  - Organization boundary enforcement ✅ IMPLEMENTED (middleware chain)
  - Audit logging for all management actions ✅ IMPLEMENTED (user_management_audit table)

### Task 10: User Management APIs **← READY FOR TESTING**
- [x] **10.1** Organization-scoped user management endpoints (**✅ IMPLEMENTED & REGISTERED**):
  ```
  POST   /api/v1/orgs/:org_id/users           # Create user in org ✅ READY
  GET    /api/v1/orgs/:org_id/users           # List org users (paginated) ✅ READY
  GET    /api/v1/orgs/:org_id/users/:user_id  # Get user details ✅ READY
  PUT    /api/v1/orgs/:org_id/users/:user_id  # Update user ✅ READY
  DELETE /api/v1/orgs/:org_id/users/:user_id  # Deactivate user ✅ READY
  PUT    /api/v1/orgs/:org_id/users/:user_id/role # Change user role ✅ READY
  ```
  **STATUS**: All handlers implemented and registered in server with proper middleware chain
- [ ] **10.2** Super admin global management endpoints (**NOT YET IMPLEMENTED**):
  ```
  GET    /api/v1/admin/users                  # List all users across orgs
  POST   /api/v1/admin/orgs/:org_id/users     # Create user in any org  
  PUT    /api/v1/admin/users/:user_id/org     # Transfer user between orgs
  GET    /api/v1/admin/analytics/users        # User management analytics
  ```
- [x] **10.3** Self-service profile endpoints (**✅ IMPLEMENTED & TESTED**):
  ```
  GET    /api/v1/users/profile                # Get own profile ✅ WORKING
  PUT    /api/v1/users/profile                # Update own profile ✅ WORKING
  PUT    /api/v1/users/password               # Change own password ✅ WORKING
  ```
  **STATUS**: All endpoints implemented, registered, and successfully tested

### Task 11: Organization Management APIs
- [x] **11.1** Organization CRUD endpoints:
  ```
  GET    /api/v1/organizations                # List user's organizations ✅ IMPLEMENTED
  POST   /api/v1/admin/organizations          # Create organization (super admin) ✅ IMPLEMENTED
  GET    /api/v1/organizations/:org_id        # Get org details ✅ IMPLEMENTED
  PUT    /api/v1/orgs/:org_id                 # Update org (owners + super admin) ✅ IMPLEMENTED
  DELETE /api/v1/admin/organizations/:org_id  # Deactivate org (super admin) ✅ IMPLEMENTED
  ```
- [x] **11.2** Organization membership endpoints:
  ```
  GET    /api/v1/orgs/:org_id/members         # List org members with roles ✅ IMPLEMENTED
  POST   /api/v1/orgs/:org_id/members/invite  # Invite user to org (future feature)
  PUT    /api/v1/orgs/:org_id/members/:user_id/role # Change member role ✅ IMPLEMENTED
  ```

### Task 12: Data Isolation & Business Logic Updates
- [ ] **12.1** Create migration: `optimize_business_table_indexes.sql`
  ```bash
  dbmate new optimize_business_table_indexes
  ```
- [ ] **12.2** Add compound indexes for multi-tenant queries:
  - `idx_reviews_org_created` for reviews list
  - `idx_ai_comments_org_review` for comment queries  
  - `idx_integration_tokens_org_provider` for provider lookups
- [ ] **12.3** Update business logic services:
  - Ensure all queries include org_id filters
  - Update existing APIs to use PermissionContext
  - Add org validation to all business operations

---

## Phase 2C: UI & Frontend Integration (Week 3) 🎨

### Task 13: Organization Selector System (Foundation)
- [x] **13.1** Create Organization Redux Slice:
  ```
  ui/src/store/Organizations/
  ├── types.ts                 # TypeScript interfaces ✅ IMPLEMENTED
  ├── reducer.ts               # Redux slice with async thunks ✅ IMPLEMENTED  
  └── index.ts                 # Redux slice exports ✅ IMPLEMENTED
  ```
  - Selected organization ID with localStorage persistence ✅
  - Available organizations list (role-based access) ✅
  - Current user role in selected organization ✅
  - Organization switching actions and reducers ✅
- [x] **13.2** Implement Organization Context Hook:
  ```typescript
  // ui/src/hooks/useOrgContext.tsx ✅ IMPLEMENTED
  // - switchOrganization(orgId: number) ✅
  // - getCurrentOrg() ✅
  // - hasPermission(action: string) ✅
  // - canManageUsers(), canChangeOwnership(), etc. ✅
  ```
- [x] **13.3** Create Organization Selector Component:
  ```typescript
  // ui/src/components/OrganizationSelector/OrganizationSelector.tsx ✅ IMPLEMENTED
  // - Dropdown with org names + role badges ✅
  // - Search/filter for users with many orgs ✅
  // - Loading states during org switching ✅
  // - Mobile-responsive design ✅
  ```
- [x] **13.4** Integrate with Navbar:
  ```typescript
  // ui/src/components/Navbar/Navbar.tsx (modified) ✅ IMPLEMENTED
  // - Add OrganizationSelector between logo and nav links ✅
  // - Mobile version in dropdown menu
  // - Consistent styling with existing navbar buttons
  ```
- [ ] **13.5** API Client Enhancement: **← IMMEDIATE PRIORITY**
  ```typescript
  // ui/src/api/apiClient.ts (modify existing)
  // Add request interceptor to automatically include X-Org-Context header
  ```

### Task 14: User Management API Layer
- [x] **14.1** Create User Management API Client:
  ```typescript
  // ui/src/api/users.ts (new file) ✅ IMPLEMENTED
  // Organization-scoped APIs:
  // - fetchOrgUsers(orgId, params): UserListResponse ✅
  // - createOrgUser(orgId, userData): User ✅
  // - updateOrgUser(orgId, userId, updates): User ✅
  // - deleteOrgUser(orgId, userId): void ✅
  // - resetUserPassword(orgId, userId): {temporary_password}
  // - changeUserRole(orgId, userId, newRole): User
  ```
- [x] **14.2** Add Super Admin APIs:
  ```typescript
  // Super admin specific actions: ✅ IMPLEMENTED
  // - transferUserToOrg(userId, targetOrgId): User ✅
  // - changeOrgOwnership(orgId, newOwnerId): void
  // - getUserAnalytics(): UserAnalytics ✅
  ```
- [x] **14.3** Add Profile Management APIs:
  ```typescript
  // Self-service profile APIs: ✅ IMPLEMENTED
  // - fetchUserProfile(): UserProfile ✅
  // - updateUserProfile(updates): UserProfile ✅
  // - changePassword(currentPassword, newPassword): void ✅
  ```
- [x] **14.4** Define TypeScript Interfaces:
  ```typescript
  // Complete type definitions for: ✅ IMPLEMENTED
  // - UserListParams, UserListResponse, User, UserProfile ✅
  // - CreateUserRequest, UpdateUserRequest, ProfileUpdateRequest ✅
  // - UserAnalytics, UserAction types ✅
  ```

### Task 15: Unified User Management Components
- [x] **15.1** Create User Management Table:
  ```typescript
  // ui/src/components/UserManagement/UserTable.tsx ✅ IMPLEMENTED
  // - Single unified table for all user roles ✅
  // - Role-based action menu filtering: ✅
  //   * Members: Read-only view
  //   * Owners: Edit, change role, reset password, deactivate
  //   * Super Admin: All owner actions + transfer org + change ownership
  // - Sortable columns with loading states ✅
  // - Responsive design with mobile optimization ✅
  ```
- [ ] **15.2** Create User Filters Component:
  ```typescript
  // ui/src/components/UserManagement/UserFilters.tsx
  // - Search input (email/name search)
  // - Role filter dropdown (all, super_admin, owner, member)
  // - Status filter dropdown (all, active, inactive)
  // - Sort controls (email, created_at, last_login_at)
  // - Clear filters functionality
  ```
- [x] **15.1** Create User Management Table:
- [x] **15.3** Create User Forms:
  ```typescript
  // ui/src/components/UserManagement/UserForm.tsx ✅ IMPLEMENTED
  // - CreateUserForm with role-based field availability ✅
  // - EditUserForm with permission-based options ✅
  // - Role selection logic: ✅
  //   * Members: No role editing
  //   * Owners: owner/member roles only  
  //   * Super Admin: All roles + "Make Owner" option
  // - Form validation and error handling ✅
  ```
- [ ] **15.4** Create User Statistics Widget:
  ```typescript
  // ui/src/components/UserManagement/UserStats.tsx
  // - Total users in current organization
  // - Active vs inactive user counts
  // - Role distribution statistics
  // - Quick actions based on user permissions
  ```
- [ ] **15.5** Create User Action Menu:
  ```typescript
  // ui/src/components/UserManagement/UserActions.tsx
  // - Dynamic action menu based on currentUserRole
  // - Consistent styling with existing UI patterns
  // - Confirmation dialogs for destructive actions
  ```

### Task 16: Settings Page Integration
- [x] **16.1** Convert Settings to Tab System:
  ```typescript
  // ui/src/pages/Settings/Settings.tsx (major modification) ✅ IMPLEMENTED
  // - Tab configuration based on user role and permissions ✅
  // - Keep existing General tab with production URL settings ✅
  // - Add new tabs without breaking existing functionality ✅
  // - URL routing for direct tab access (/settings/users, /settings/profile)
  ```
- [ ] **16.2** Create Profile Settings Tab:
  ```typescript
  // ui/src/pages/Settings/ProfileSettings.tsx (new component)
  // Form Sections:
  // - Basic Information (first name, last name, email)
  // - Security (change password, active sessions)
  // - Organization Membership (list orgs with roles)
  ```
- [x] **16.3** Create User Management Settings Tab:
  ```typescript
  // ui/src/pages/Settings/UserManagementSettings.tsx (new component) ✅ IMPLEMENTED
  // - Current organization context display ✅
  // - UserStats widget with role-appropriate actions
  // - UserFilters component
  // - UserTable component with permission-filtered actions ✅
  // - Create User button (only if canCreateUsers) ✅
  // - Bulk action controls (only if canManageUsers)
  ```
- [x] **16.4** Create Organization Management Tab (Super Admin Only):
  ```typescript
  // ui/src/pages/Settings/OrganizationManagementSettings.tsx (new component) ✅ IMPLEMENTED
  // - List all organizations with statistics
  // - Create new organizations
  // - Quick organization switching with "Manage Users" action
  // - Note: This is the only truly "global" interface
  ```
- [x] **16.5** Add Tab Navigation Logic:
  ```typescript
  // Tab visibility based on permissions: ✅ IMPLEMENTED
  // - General tab: All users ✅
  // - Profile tab: All users  
  // - Users tab: Users who canManageUsers (owners + super admins) ✅
  // - Organizations tab: Super admins only ✅
  ```

### Task 17: Redux Store Integration
- [x] **17.1** Update Root Reducer:
  ```typescript
  // ui/src/store/rootReducer.ts (modify existing) ✅ IMPLEMENTED
  // - Add Organization reducer to combineReducers ✅
  // - Ensure proper state typing ✅
  ```
- [x] **17.2** App Initialization Updates:
  ```typescript
  // ui/src/App.tsx (modify existing) ✅ IMPLEMENTED
  // - Add organization context initialization on auth ✅
  // - Fetch available orgs and restore selected org from localStorage ✅
  // - Clear org context on logout ✅
  ```
- [ ] **17.3** Routing Enhancement:
  ```typescript
  // Add sub-routes for settings tabs:
  // - /settings/general (default)
  // - /settings/profile 
  // - /settings/users
  // - /settings/organizations
  ```

### Task 18: Responsive Design & Polish
- [ ] **18.1** Mobile Optimization:
  - Organization Selector: Compact dropdown in mobile navbar
  - User Table: Horizontal scroll with sticky first column  
  - Filters: Collapsible filter panel on mobile
  - Forms: Full-screen modals on mobile devices
- [ ] **18.2** Accessibility:
  - Proper ARIA labels for organization selector
  - Keyboard navigation support
  - Screen reader compatibility
  - Focus management in modals
- [ ] **18.3** Error Handling:
  - Organization switching errors
  - API call failures with user-friendly messages
  - Form validation errors
  - Loading states throughout the interface
- [ ] **18.4** Performance Optimization:
  - Efficient org context updates
  - Table pagination optimization
  - Debounced search functionality
  - Memoized expensive computations

---

## Next Steps to Start Implementation 🚀

**UI implementation is in progress. The foundational components for organization selection and user management are complete. The immediate priority is to connect the UI to the backend's permission model and then implement core User CRUD functionality.**

### Implementation Priority:
1.  **Task 13.5: API Client Enhancement (IMMEDIATE PRIORITY)**
    -   Modify `ui/src/api/apiClient.ts` to inject the `X-Org-Context` header into relevant API requests. This is the critical link between the frontend's organization selector and the backend's data scoping.

2.  **Task 15.3: Implement User Forms (NEXT PRIORITY)**
    -   Create the `UserForm.tsx` component for creating and editing users.
    -   Implement the modals and form logic for adding a new user and editing an existing user's details.
    -   This prioritizes core CRUD functionality over filters and other enhancements.

3.  **Task 15.2 & 15.4: Filters and Statistics (LOWER PRIORITY)**
    -   Implement the `UserFilters.tsx` and `UserStats.tsx` components after core CRUD is functional.

4.  **Task 16.2: Profile Settings Tab**
    -   Implement the self-service profile management tab.

**CURRENT STARTING POINT: Task 13.5 - API Client Enhancement**

---

## Phase 2D: External Authentication Foundation (Week 4) 🔗

### Task 17: External Provider Database Foundation (Preparation Only)
- [ ] **17.1** Create migration: `add_external_auth_foundation.sql`
  ```bash
  dbmate new add_external_auth_foundation
  ```
- [ ] **17.2** Create basic `identity_providers` table structure:
  - Prepare for multiple auth providers per organization
  - Local provider as default (foundation for Authentik later)
  - Keep simple for now, extensible for future
- [ ] **17.3** Extend users table minimally:
  - `provider_id` for future external provider linkage (nullable, defaults to local)
  - Keep current email/password auth working as local provider

### Task 18: Local Authentication Provider Framework
- [ ] **18.1** Abstract provider interface (simple foundation):
  - Local provider implementation (current email/password)
  - Interface design for future Authentik/OIDC providers
  - Provider validation and user lookup
- [ ] **18.2** Provider configuration foundation:
  - Local provider setup (basic configuration)
  - Database structure for future provider configs
  - Keep focused on local user management

### Task 19: API Token Foundation (Future Extension)
- [ ] **19.1** Prepare auth_tokens table for API tokens:
  - Add token_type field ('session' vs 'api_key')
  - Basic structure for future API token permissions
  - Keep simple - focus on session tokens for now

---

## Phase 2E: Manual Testing & Deployment (Week 5) 🧪

### Task 20: Manual Testing & Validation
- [ ] **20.1** Core functionality manual testing:
  - Login/logout flows work end-to-end
  - Token refresh works seamlessly in UI
  - User creation with temporary passwords
  - Password change functionality
- [ ] **20.2** Multi-user scenarios:
  - Super admin can manage all organizations
  - Org owners can manage their organization users
  - Members can access their profiles and change passwords
- [ ] **20.3** Database and performance:
  - Session cleanup works without issues
  - Multi-tenant queries perform reasonably
  - Auth token storage and retrieval is fast

### Task 21: Basic Documentation
- [ ] **21.1** Essential API documentation:
  - Authentication endpoints documentation
  - User management API reference
  - Setup and initial admin creation guide
- [ ] **21.2** User guide:
  - How to create first admin
  - How to manage users in organizations
  - How to change passwords and manage profiles

### Task 22: Production Readiness (Basic)
- [ ] **22.1** Security basics:
  - JWT tokens are properly secured
  - Password hashing is secure
  - Session management is secure
- [ ] **22.2** Basic deployment:
  - Works with existing Docker setup without changes
  - Database migrations apply correctly
  - Configuration is straightforward

---

## Validation Milestones 🎯

### Phase 2A Complete When:
- [x] Users can login with email/password and receive JWT tokens (access + refresh)
- [x] Session management works with automatic cleanup and token refresh
- [x] React UI automatically handles token refresh without user interruption **← COMPLETED**
- [x] Middleware enforces org boundaries on all protected endpoints
- [x] Legacy password-based auth is completely removed and replaced

### Phase 2B Complete When:
- [x] **Task 10.1**: Organization-scoped user management APIs are registered and working ✅ **COMPLETE**
- [x] **Task 10.2**: Super admins can manage users across all organizations ✅ **COMPLETE**
- [x] **Task 10.3**: Self-service profile endpoints enable users to manage their own profiles ✅ **COMPLETE**
- [x] **Task 11**: Organization owners can manage users within their organizations ✅ **COMPLETE**
- [ ] **Task 5.3**: Users are created with temporary passwords and forced to change on first login
- [ ] **Task 6**: All user management actions are validated through comprehensive testing
- [x] All user management actions are audited and logged ✅ **COMPLETE**

### Phase 2C Complete When:
- [ ] **Organization Selector System**: Site-wide org selector working for all user roles with proper persistence
- [ ] **Unified User Management**: Role-based user table with permission-filtered actions working
- [ ] **Settings Integration**: Tab-based settings with Users tab for management and Profile tab for all users
- [ ] **Permission Boundaries**: Members see read-only, owners get full org management, super admins get ownership controls
- [ ] **Mobile Responsive**: All components work properly on mobile devices
- [ ] **API Integration**: Organization context automatically included in relevant API calls

### Phase 2D Complete When:
- [ ] Foundation supports multiple authentication providers (prepared for Authentik)
- [ ] Local authentication provider works as default
- [ ] Database structure supports future external providers
- [ ] API token foundation is prepared for future development

### Phase 2E Complete When:
- [ ] Manual testing confirms all functionality works
- [ ] Basic documentation is available
- [ ] Deployment works with existing Docker setup
- [ ] System is ready for production use

---

## Risk Mitigation & Success Criteria 🛡️

### Critical Success Factors:
1. **Clean Migration**: Complete replacement of old password-based auth with JWT system
2. **Data Integrity**: No data loss during migration, proper user/org associations  
3. **Performance**: Multi-tenant queries perform well with proper indexing
4. **Security**: Permission boundaries are enforced at all layers with unmissable patterns
5. **Usability**: Minimal UI changes provide essential user management functionality
6. **Token Management**: Seamless token refresh in React UI without user interruption

### Risk Mitigation:
1. **Clean Cut**: No backward compatibility burden, fresh JWT-based implementation
2. **Incremental Deployment**: Features can be deployed and tested phase by phase
3. **Manual Testing**: Thorough manual validation at each phase milestone
4. **Foundation Focus**: Build solid foundation for local auth, prepare for future external providers
5. **Minimal UI Changes**: Leverage existing React components and patterns

---

## Next Steps to Start Implementation 🚀

**UI implementation is in progress. The foundational components for organization selection and user management are complete. The immediate priority is to connect the UI to the backend's permission model and then implement core User CRUD functionality.**

### Implementation Priority:
1.  **Task 13.5: API Client Enhancement (IMMEDIATE PRIORITY)**
    -   Modify `ui/src/api/apiClient.ts` to inject the `X-Org-Context` header into relevant API requests. This is the critical link between the frontend's organization selector and the backend's data scoping.

2.  **Task 15.3: Implement User Forms (NEXT PRIORITY)**
    -   Create the `UserForm.tsx` component for creating and editing users.
    -   Implement the modals and form logic for adding a new user and editing an existing user's details.
    -   This prioritizes core CRUD functionality over filters and other enhancements.

3.  **Task 15.2 & 15.4: Filters and Statistics (LOWER PRIORITY)**
    -   Implement the `UserFilters.tsx` and `UserStats.tsx` components after core CRUD is functional.

4.  **Task 16.2: Profile Settings Tab**
    -   Implement the self-service profile management tab.

**CURRENT STARTING POINT: Task 13.5 - API Client Enhancement**
