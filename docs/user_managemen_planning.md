# User Management UI Implementation Plan 📋

## Overview & Strategic Approach

Based on the successful completion of all backend APIs (Tasks 9-11), this document outlines the comprehensive UI implementation plan for multi-tenant user management. The focus is on creating a site-wide organization selector system and robust user management interfaces that leverage existing React components and patterns.

## Current Status Analysis 🔍

### ✅ Backend APIs Complete & Tested
- **Task 9**: User Management Service Layer ✅ **COMPLETE**
- **Task 10.1**: Organization-scoped user management APIs ✅ **TESTED & WORKING**
- **Task 10.2**: Super admin global management endpoints ✅ **TESTED & WORKING**  
- **Task 10.3**: Self-service profile endpoints ✅ **TESTED & WORKING**
- **Task 11**: Organization management APIs ✅ **TESTED & WORKING**

### 🎯 Frontend Implementation Needed
- Site-wide organization selector system
- User management UI with table-based interface
- Profile management interface
- Role-based permission controls within unified interface

### 📋 Available React Infrastructure
- **Redux Store**: Auth state with user/organizations already available
- **UI Components**: Comprehensive UI primitives (Button, Input, Select, Card, Badge, etc.)
- **Pagination**: Existing pagination patterns in RecentActivity component
- **Navbar**: Extensible navbar component ready for organization selector
- **Settings Page**: Structured settings page ready for new tabs

---

## PHASE 1: Site-wide Organization Selector System 🌐

### Problem Statement
Users need to select which organization they're operating in context of, and all subsequent API calls should include this organization context. This needs to be:
- **Prominently visible** in the navbar for all users
- **Persistent** across page navigation and browser sessions
- **Integrated** with permission context system from backend
- **Universal for all roles** (super admin can switch to any org, owners/members see their accessible orgs)
- **Context-driven permissions** (actions available depend on user's role within selected org)

### Solution Architecture

#### 1.1 Redux Store Enhancement
**File**: `ui/src/store/Organization/` (New Redux slice)
**Dependencies**: Integrates with existing Auth store

```typescript
// ui/src/store/Organization/orgSlice.ts
interface OrgContextState {
  selectedOrgId: number | null;
  availableOrgs: OrgInfo[]; // All orgs user can access (their own + all for super admin)
  currentUserRole: string | null; // User's role in the currently selected org
  isLoading: boolean;
  error: string | null;
}

// Actions needed:
// - setSelectedOrg(orgId: number) - Auto-fetches user's role in that org
// - fetchAvailableOrgs() - Uses existing /api/v1/organizations endpoint
// - clearOrgContext()
// - updateCurrentUserRole(role: string) - Updates role in current org context
```

**Permission Strategy**: 
- Super admin: Can access any organization, role becomes 'super_admin' in org context
- Org owner: Can access organizations they own, role is 'owner' 
- Member: Can access organizations they're members of, role is 'member'
- Role-based UI: Actions and options filtered based on currentUserRole

#### 1.2 API Client Enhancement
**File**: `ui/src/api/apiClient.ts` (Modify existing)
**Purpose**: Automatically include org context in API calls

```typescript
// Add request interceptor to include org context
apiClient.interceptors.request.use((config) => {
  const store = getStore(); // Access Redux store
  const selectedOrgId = store.getState().Organization.selectedOrgId;
  
  // Add org context to headers for API calls that need it
  if (selectedOrgId && shouldIncludeOrgContext(config.url)) {
    config.headers['X-Org-Context'] = selectedOrgId.toString();
  }
  
  return config;
});
```

**URL Pattern Analysis**:
- Include org context for: `/api/v1/orgs/`, `/api/v1/users/profile`, user management endpoints
- Skip org context for: `/api/v1/auth/`, setup endpoints, system endpoints
- Smart routing: Super admin actions automatically use appropriate API (org-scoped vs global)

#### 1.3 Organization Selector Component
**File**: `ui/src/components/OrganizationSelector/OrganizationSelector.tsx` (New component)
**Design**: Dropdown component integrated into navbar

```tsx
interface OrganizationSelectorProps {
  className?: string;
  size?: 'sm' | 'md';
}

// Features:
// - Shows current selected organization name and user's role in that org
// - Dropdown with all accessible organizations
// - Role badge showing user's role in each org (owner, member, super_admin)
// - Search/filter for users with access to many orgs  
// - Loading state while switching organizations
// - Clear visual indication of current context
// - For super admin: "All Organizations" option + individual org access
```

**Visual Design**:
- **Navbar Integration**: Positioned between logo and navigation links
- **Desktop**: Full dropdown with org names and role badges
- **Mobile**: Compact dropdown, shows just org name
- **Styling**: Consistent with existing navbar button styling
- **Accessibility**: Proper ARIA labels, keyboard navigation

#### 1.4 Organization Context Hook
**File**: `ui/src/hooks/useOrgContext.ts` (New hook)
**Purpose**: Provide easy access to org context throughout app

```typescript
export const useOrgContext = () => {
  const dispatch = useAppDispatch();
  const { selectedOrgId, availableOrgs, currentUserRole, isLoading } = useAppSelector(state => state.Organization);
  
  const switchOrganization = (orgId: number) => {
    dispatch(setSelectedOrg(orgId)); // This will also fetch user's role in that org
  };
  
  const getCurrentOrg = () => {
    return availableOrgs.find(org => org.id === selectedOrgId);
  };
  
  const hasPermission = (action: string) => {
    // Role-based permission checking for current org context
    return checkPermission(currentUserRole, action);
  };
  
  const canManageUsers = () => hasPermission('manage_users'); // owner + super_admin
  const canChangeOwnership = () => hasPermission('change_ownership'); // super_admin only
  const canCreateUsers = () => hasPermission('create_users'); // owner + super_admin
  const canDeleteUsers = () => hasPermission('delete_users'); // owner + super_admin
  
  return { 
    selectedOrgId, 
    availableOrgs, 
    currentUserRole,
    switchOrganization, 
    getCurrentOrg, 
    hasPermission,
    canManageUsers,
    canChangeOwnership,
    canCreateUsers,
    canDeleteUsers,
    isLoading 
  };
};
```

#### 1.5 Navbar Integration
**File**: `ui/src/components/Navbar/Navbar.tsx` (Modify existing)
**Changes**: Add organization selector between logo and nav links

```tsx
// Add to Navbar component, positioned after logo, before nav links
<div className="flex items-center">
  <Link to="/" className="...">
    <img src="assets/logo-horizontal.svg" ... />
  </Link>
  
  {/* NEW: Organization Selector */}
  <div className="ml-6 hidden md:block">
    <OrganizationSelector size="md" />
  </div>
</div>

// Mobile version in dropdown menu
{isOpen && (
  <div className="md:hidden ...">
    {/* NEW: Mobile org selector at top */}
    <div className="pb-3 mb-3 border-b border-slate-700/60">
      <OrganizationSelector size="sm" />
    </div>
    {/* Existing nav links */}
  </div>
)}
```

---

## PHASE 2: User Management Table Interface 👥

### Problem Statement
Need a comprehensive table-based user management interface that:
- **Displays all users** in the currently selected organization with pagination
- **Supports filtering** by role, status, email search
- **Enables CRUD operations** with role-based permissions (create, edit roles, deactivate, reset passwords)
- **Shows user statistics** (total users, active users, role distribution)
- **Unified interface** that works for all user roles with different capabilities:
  - **Members**: View-only access to user list
  - **Owners**: Full user management within their organization
  - **Super Admin**: Full user management + ability to change ownership + access to any org

### Solution Architecture

#### 2.1 User Management API Client
**File**: `ui/src/api/users.ts` (New file)
**Purpose**: Wrapper for all user management API calls

```typescript
```typescript
// API functions needed:
// Organization-scoped (primary interface for all users)
export const fetchOrgUsers = async (orgId: number, params: UserListParams): Promise<UserListResponse>
export const createOrgUser = async (orgId: number, userData: CreateUserRequest): Promise<User>
export const updateOrgUser = async (orgId: number, userId: number, updates: UpdateUserRequest): Promise<User>
export const deleteOrgUser = async (orgId: number, userId: number): Promise<void>
export const resetUserPassword = async (orgId: number, userId: number): Promise<{ temporary_password: string }>
export const changeUserRole = async (orgId: number, userId: number, newRole: string): Promise<User>

// Super admin specific actions (automatically used when currentUserRole === 'super_admin')
export const transferUserToOrg = async (userId: number, targetOrgId: number): Promise<User>
export const changeOrgOwnership = async (orgId: number, newOwnerId: number): Promise<void>
export const getUserAnalytics = async (): Promise<UserAnalytics>

// Self-service profile (available to all users)
export const fetchUserProfile = async (): Promise<UserProfile>
export const updateUserProfile = async (updates: ProfileUpdateRequest): Promise<UserProfile>
export const changePassword = async (currentPassword: string, newPassword: string): Promise<void>

// Smart API routing based on user role and action
export const performUserAction = async (action: UserAction, params: any) => {
  const userRole = getCurrentUserRole();
  
  // Route to appropriate API based on user's role and action type
  if (userRole === 'super_admin' && action.type === 'TRANSFER_USER') {
    return transferUserToOrg(params.userId, params.targetOrgId);
  } else if (action.type === 'UPDATE_USER') {
    return updateOrgUser(params.orgId, params.userId, params.updates);
  }
  // ... etc
};
```

// Interface definitions for all request/response types
interface UserListParams {
  page?: number;
  limit?: number;
  search?: string;
  role?: string;
  status?: 'active' | 'inactive';
  sort_by?: 'email' | 'created_at' | 'last_login_at';
  sort_order?: 'asc' | 'desc';
}
```

#### 2.2 User Management Table Component
**File**: `ui/src/components/UserManagement/UserTable.tsx` (New component)
**Design**: Single unified table component with role-based action filtering

```tsx
interface UserTableProps {
  orgId: number; // Always org-scoped, from org selector context
  onUserUpdate?: (user: User) => void;
  onUserDelete?: (userId: number) => void;
}

// Table Features:
// - Sortable columns (email, role, status, last login, created date)
// - Row actions filtered by user permissions:
//   * Members: View only (no action menu)
//   * Owners: Edit role, deactivate, reset password (within their org)
//   * Super Admin: All owner actions + transfer to other org + change ownership
// - Bulk selection for bulk operations (permission-filtered)
// - Responsive design (mobile-friendly)
// - Loading states with skeleton rows
// - Empty state with create user prompt (if user has permission)
```

**Action Menu Examples**:
```tsx
// For Members (read-only)
<UserRow user={user}>
  {/* No action menu, just view */}
</UserRow>

// For Owners  
<UserActionMenu>
  <MenuItem>Edit User</MenuItem>
  <MenuItem>Change Role</MenuItem>
  <MenuItem>Reset Password</MenuItem>
  <MenuItem>Deactivate User</MenuItem>
</UserActionMenu>

// For Super Admin (additional actions)
<UserActionMenu>
  <MenuItem>Edit User</MenuItem>
  <MenuItem>Change Role</MenuItem>
  <MenuItem>Transfer to Organization</MenuItem>
  <MenuItem>Make Organization Owner</MenuItem>
  <MenuItem>Reset Password</MenuItem>
  <MenuItem>Deactivate User</MenuItem>
</UserActionMenu>
```

**Table Columns**:
1. **Checkbox** (for bulk selection - only if user has management permissions)
2. **Email** (sortable, searchable)
3. **Name** (first_name + last_name)
4. **Role** (badge with color coding + edit capability based on permissions)
5. **Status** (active/inactive badge)
6. **Last Login** (humanized timestamp)
7. **Actions** (dropdown menu - content filtered by user's role in current org)

**Table Libraries**: 
- **Leverage existing patterns** from RecentActivity component for pagination
- **Use existing UI primitives** (Button, Badge, Select) for consistency
- **Manual table implementation** using existing components rather than external libraries

#### 2.3 User Management Filters
**File**: `ui/src/components/UserManagement/UserFilters.tsx` (New component)
**Purpose**: Filter and search controls above user table

```tsx
interface UserFiltersProps {
  onFilterChange: (filters: UserListParams) => void;
  currentFilters: UserListParams;
  isLoading?: boolean;
}

// Filter Controls:
// - Search input (email/name search)
// - Role filter dropdown (all, super_admin, owner, member)
// - Status filter dropdown (all, active, inactive)
// - Sort by dropdown (email, created_at, last_login_at)
// - Sort order toggle (asc/desc)
// - Clear filters button
```

#### 2.4 User Form Components
**File**: `ui/src/components/UserManagement/UserForm.tsx` (New component)
**Purpose**: Modal forms for creating and editing users with role-based field availability

```tsx
// CreateUserForm - for creating new users
interface CreateUserFormProps {
  orgId: number; // Always from current org context
  isOpen: boolean;
  onClose: () => void;
  onUserCreated: (user: User) => void;
}

// EditUserForm - for editing existing users  
interface EditUserFormProps {
  user: User;
  isOpen: boolean;
  onClose: () => void;
  onUserUpdated: (user: User) => void;
}

// Form Fields (role-based availability):
// - Email (required, validation)
// - First Name (optional)
// - Last Name (optional) 
// - Role (dropdown):
//   * Members: No role selection (read-only)
//   * Owners: owner/member roles only
//   * Super Admin: All roles including super_admin, plus "Make Owner" option
// - Transfer Organization (super admin only - dropdown of all orgs)
// - Generate temporary password checkbox
// - Password strength indicator
```

**Role-Based Form Logic**:
```tsx
const getRoleOptions = (currentUserRole: string, isEditing: boolean) => {
  if (currentUserRole === 'member') return []; // No role editing
  
  if (currentUserRole === 'owner') {
    return [
      { value: 'member', label: 'Member' },
      { value: 'owner', label: 'Owner' }
    ];
  }
  
  if (currentUserRole === 'super_admin') {
    return [
      { value: 'member', label: 'Member' },
      { value: 'owner', label: 'Owner' },
      { value: 'super_admin', label: 'Super Admin' },
      ...(isEditing ? [{ value: 'make_org_owner', label: 'Make Organization Owner' }] : [])
    ];
  }
};
```

**Form Validation**:
- Email format validation
- Password strength requirements
- Duplicate email prevention
- Role permission validation
- Required field validation

**Modal Integration**:
- Use existing modal patterns from the codebase
- Form submission with loading states
- Error handling and display
- Success notifications

#### 2.5 User Statistics Widget
**File**: `ui/src/components/UserManagement/UserStats.tsx` (New component)
**Purpose**: Display user statistics for currently selected organization

```tsx
interface UserStatsProps {
  orgId: number; // From current org context
}

// Statistics Displayed (same for all roles, but different actions available):
// - Total users in current organization
// - Active vs inactive users  
// - Role distribution (owners, members, super_admins if any)
// - Recent user activity in this org
// - Quick actions (role-dependent):
//   * Members: View-only stats
//   * Owners: "Invite Users" button
//   * Super Admin: "Invite Users" + "Manage Organizations" buttons
```

---

## PHASE 3: Settings Page Integration ⚙️

### Problem Statement
The existing Settings page needs to be enhanced with:
- **Tab-based navigation** for different settings categories
- **Users tab** for organization user management
- **Profile tab** for self-service profile management
- **Role-based visibility** (different tabs for different user roles)

### Solution Architecture

#### 3.1 Settings Page Tab System
**File**: `ui/src/pages/Settings/Settings.tsx` (Major modification)
**Design**: Convert from single-page to tabbed interface

```tsx
// Tab Configuration based on user role and permissions
const getSettingsTabs = (currentUserRole: string, canManageUsers: boolean) => {
  const tabs = [
    { id: 'general', label: 'General', icon: <Icons.Settings />, component: GeneralSettings },
    { id: 'profile', label: 'Profile', icon: <Icons.User />, component: ProfileSettings },
  ];
  
  // Add Users tab for anyone who can manage users (owners + super admins in any org they can access)
  if (canManageUsers) {
    tabs.push({
      id: 'users', 
      label: 'Users', 
      icon: <Icons.Users />, 
      component: UserManagementSettings
    });
  }
  
  // Add Organizations tab for super admins only (global org management)
  if (currentUserRole === 'super_admin') {
    tabs.push({
      id: 'organizations',
      label: 'Organizations',
      icon: <Icons.Building />,
      component: OrganizationManagementSettings
    });
  }
  
  return tabs;
};
```

**Tab Implementation Strategy**:
- **Keep existing General tab** with production URL settings
- **Add new tabs** without breaking existing functionality
- **Use URL routing** for direct tab access (`/settings/users`, `/settings/profile`)
- **Responsive design** with mobile-friendly tab navigation

#### 3.2 Profile Settings Tab
**File**: `ui/src/pages/Settings/ProfileSettings.tsx` (New component)
**Purpose**: Self-service profile management interface

```tsx
interface ProfileSettingsProps {
  className?: string;
}

// Profile Management Features:
// - View current profile information
// - Edit first name, last name, email
// - Change password with current password verification
// - View organization memberships and roles
// - Account activity log (recent logins, profile changes)
```

**Form Sections**:
1. **Basic Information**
   - First Name / Last Name (editable)
   - Email (editable with verification)
   - Member since (read-only)

2. **Security**
   - Change password form
   - Active sessions list
   - Login history

3. **Organization Membership**
   - List of organizations with roles
   - Request access to new organizations (future feature)

#### 3.3 User Management Settings Tab
**File**: `ui/src/pages/Settings/UserManagementSettings.tsx` (New component)
**Purpose**: Unified user management interface that adapts to user's role in current org

```tsx
interface UserManagementSettingsProps {
  className?: string;
}

// Component Structure (same for all roles, different capabilities):
// - Current organization display (from org selector context)
// - UserStats widget (role-appropriate actions)
// - UserFilters component (search always available)
// - UserTable component (actions filtered by role)
// - Create User button (only if canCreateUsers permission)
// - Bulk action controls (only if canManageUsers permission)

// The magic: Same UI, different permissions
const UserManagementSettings = () => {
  const { selectedOrgId, currentUserRole, canManageUsers, canCreateUsers } = useOrgContext();
  
  return (
    <div>
      <OrgContextHeader /> {/* Shows current org + role */}
      <UserStats orgId={selectedOrgId} />
      <UserFilters onFilterChange={handleFilterChange} />
      
      {canCreateUsers && (
        <CreateUserButton orgId={selectedOrgId} />
      )}
      
      <UserTable 
        orgId={selectedOrgId}
        userRole={currentUserRole} // Determines available actions
      />
    </div>
  );
};
```

**Layout Design**:
```
┌─ User Management Settings ─────────────────────────────────┐
│ 📍 Currently managing: Acme Corp (You are: Owner)          │
│ ┌─ Statistics Widget ──────────────────────────────────┐   │
│ │ Total: 25 | Active: 23 | Owners: 2 | Members: 21   │   │
│ │ [+ Invite Users] (if canCreateUsers)                │   │
│ └──────────────────────────────────────────────────────┘   │
│                                                            │
│ ┌─ Filters & Actions ──────────────────────────────────┐   │
│ │ [Search] [Role ▼] [Status ▼] [Sort ▼]                │   │
│ │                              [+ Add User] (if perms) │   │
│ └──────────────────────────────────────────────────────┘   │
│                                                            │
│ ┌─ User Table ─────────────────────────────────────────┐   │
│ │ ☐  Email        Name      Role    Status   Actions   │   │
│ │ ☐  john@co.com  John Doe  Owner   Active   [⋯]      │   │
│ │ ☐  jane@co.com  Jane Doe  Member  Active   [⋯]      │   │
│ │    (Actions menu content depends on currentUserRole) │   │
│ └──────────────────────────────────────────────────────┘   │
│                                                            │
│ ┌─ Pagination ─────────────────────────────────────────┐   │
│ │ Showing 1-10 of 25 users    [← Previous] [Next →]   │   │
│ └──────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────┘
```

#### 3.4 Organization Management Settings Tab (Super Admin Only)
**File**: `ui/src/pages/Settings/OrganizationManagementSettings.tsx` (New component)
**Purpose**: Super admin organization management interface

```tsx
// Organization Management Features:
// - List all organizations with statistics (super admin global view)
// - Create new organizations  
// - Edit organization details
// - View organization user counts and activity
// - Quick organization switching with "Manage Users" action for each org

// Note: This is the only truly "global" interface - it shows all orgs at once
// For managing users within a specific org, super admin still uses the 
// unified user management interface after selecting that org
```

---

## PHASE 4: Implementation Details & File Structure 📁

### 4.1 New File Structure
```
ui/src/
├── api/
│   ├── users.ts                     # NEW: User management API calls
│   └── organizations.ts             # NEW: Organization management API calls
├── components/
│   ├── OrganizationSelector/
│   │   ├── OrganizationSelector.tsx # NEW: Site-wide org selector
│   │   └── index.ts
│   └── UserManagement/
│       ├── UserTable.tsx            # NEW: Main user table component
│       ├── UserFilters.tsx          # NEW: Filter and search controls
│       ├── UserForm.tsx             # NEW: Create/edit user forms
│       ├── UserStats.tsx            # NEW: User statistics widget
│       ├── UserActions.tsx          # NEW: User action menu component
│       └── index.ts
├── hooks/
│   ├── useOrgContext.ts             # NEW: Organization context hook
│   ├── useUserManagement.ts         # NEW: User management operations hook
│   └── usePagination.ts             # NEW: Reusable pagination hook
├── pages/Settings/
│   ├── Settings.tsx                 # MODIFIED: Add tab system
│   ├── ProfileSettings.tsx          # NEW: Profile management tab
│   ├── UserManagementSettings.tsx   # NEW: User management tab
│   └── OrganizationManagementSettings.tsx # NEW: Org management tab
└── store/
    └── Organization/
        ├── orgSlice.ts              # NEW: Organization context Redux slice
        └── index.ts
```

### 4.2 Integration Points

#### 4.2.1 Redux Store Integration
**File**: `ui/src/store/rootReducer.ts` (Modify existing)
```typescript
import organizationReducer from './Organization/orgSlice';

const rootReducer = combineReducers({
  Auth: authReducer,
  Organization: organizationReducer, // NEW
  Connector: connectorReducer,
  Dashboard: dashboardReducer,
  Settings: settingsReducer,
  ToDo: toDoReducer,
});
```

#### 4.2.2 App Initialization
**File**: `ui/src/App.tsx` (Modify existing)
```typescript
// Add organization context initialization
useEffect(() => {
  if (isAuthenticated) {
    dispatch(fetchAvailableOrgs());
    dispatch(restoreSelectedOrg()); // From localStorage
  }
}, [isAuthenticated]);
```

#### 4.2.3 Routing Enhancement
**File**: `ui/src/App.tsx` or routing file (Modify existing)
```typescript
// Add sub-routes for settings tabs
<Route path="/settings" element={<Settings />}>
  <Route path="general" element={<GeneralSettings />} />
  <Route path="profile" element={<ProfileSettings />} />
  <Route path="users" element={<UserManagementSettings />} />
  <Route path="organizations" element={<OrganizationManagementSettings />} />
</Route>
```

### 4.3 Library Dependencies

#### 4.3.1 Existing Libraries (Reuse)
- **@reduxjs/toolkit**: Redux state management ✅ Available
- **react-router-dom**: Routing for settings tabs ✅ Available  
- **classnames**: CSS class management ✅ Available
- **date-fns** or similar: Date formatting ✅ Check if available

#### 4.3.2 New Libraries (Minimal Additions)
- **react-hook-form**: Form validation and management (recommended)
- **zod**: Schema validation for forms (optional, can use manual validation)
- **date-fns**: Date/time formatting if not already available

**Installation Strategy**: Add only if needed, prefer manual implementations using existing patterns.

### 4.4 Responsive Design Strategy

#### 4.4.1 Mobile-First Approach
- **Organization Selector**: Compact dropdown in mobile navbar
- **User Table**: Horizontal scroll with sticky first column
- **Filters**: Collapsible filter panel on mobile
- **Forms**: Full-screen modals on mobile devices

#### 4.4.2 Breakpoint Strategy
```css
/* Use existing Tailwind breakpoints */
sm: 640px   /* Mobile landscape */
md: 768px   /* Tablet */
lg: 1024px  /* Desktop */
xl: 1280px  /* Large desktop */
```

---

## PHASE 5: Implementation Timeline & Testing Strategy 🚀

### 5.1 Implementation Order (Revised)

The foundational UI work (Organization Selector, Redux state, basic table and settings tabs) is largely complete. The focus now shifts to connecting the UI to the backend permission model and then implementing core user management functionality in order of priority.

#### Stage 1: Connect UI to Backend Permissions (Immediate Priority)
1.  **API Client Enhancement**:
    -   **Task**: Modify `ui/src/api/apiClient.ts` to inject the `X-Org-Context` header into relevant API requests.
    -   **Goal**: This is the critical link that makes the frontend's organization selection meaningful to the backend. It is the highest priority as it enables all subsequent permission-scoped features.

#### Stage 2: Core User CRUD Functionality (High Priority)
1.  **User Creation & Editing Forms**:
    -   **Task**: Implement `UserForm.tsx` and the associated modals for creating new users and editing existing users' details.
    -   **Goal**: Enable administrators (Org Owners, Super Admins) to perform the most fundamental user management tasks. This delivers immediate, high-value functionality.

#### Stage 3: Secondary User Management Features (Medium Priority)
1.  **User Filters & Statistics**:
    -   **Task**: Implement the `UserFilters.tsx` and `UserStats.tsx` components.
    -   **Goal**: Enhance the user management table with powerful search, filtering, and at-a-glance overview statistics.

#### Stage 4: Self-Service and Polish (Lower Priority)
1.  **Profile Management**:
    -   **Task**: Implement the `ProfileSettings.tsx` tab for self-service profile updates and password changes.
    -   **Goal**: Empower all users to manage their own accounts.
2.  **UI Polish & Testing**:
    -   **Task**: Comprehensive UI/UX improvements, responsive design checks, accessibility enhancements, and robust error handling.
    -   **Goal**: Ensure a robust, reliable, and polished user experience across the new features.

### 5.2 Testing Strategy

#### 5.2.1 Manual Testing Approach
- **Role-based testing**: Test as super admin, org owner, and member
- **Organization switching**: Verify context switching works correctly
- **CRUD operations**: Test all user management operations
- **Permission boundaries**: Ensure users can only access appropriate functions
- **Mobile responsiveness**: Test on various screen sizes

#### 5.2.2 Integration Testing
- **API integration**: Test all API endpoints with proper error handling
- **State management**: Verify Redux state updates correctly
- **Navigation**: Test routing and tab navigation
- **Form validation**: Test all form validations and error states

### 5.3 Success Criteria

### 5.3 Success Criteria

#### 5.3.1 Functional Requirements ✅
- ✅ Site-wide organization selector working for all user roles
- ✅ Unified user table with role-based action filtering
- ✅ CRUD operations with proper permission boundaries
- ✅ Super admin can change organization ownership via org selector context
- ✅ Members have read-only access, owners have full org management, super admin has global access
- ✅ Profile management for all users
- ✅ Mobile-responsive design

#### 5.3.2 Performance Requirements
- Page load times under 2 seconds
- Table operations responsive (< 500ms)
- Organization switching smooth (< 1 second)
- No memory leaks in single-page navigation

#### 5.3.3 User Experience Requirements
- Intuitive navigation and organization selection
- Clear visual feedback for all operations
- Consistent design with existing application
- Accessible for keyboard and screen readers

---

## PHASE 6: Future Enhancements & Extensibility 🔮

### 6.1 Planned Enhancements
- **Bulk user import**: CSV/Excel user import functionality
- **User invitation system**: Email-based user invitations
- **Advanced permissions**: Granular permission management
- **Audit logging UI**: View user management audit trails
- **User groups**: Create and manage user groups within organizations

### 6.2 Integration Points for Future Features
- **SSO Integration**: Organization selector ready for SSO provider selection
- **API Keys Management**: User table extensible for API key management
- **Advanced Analytics**: Statistics widgets ready for expanded metrics
- **Multi-language Support**: Component structure ready for i18n

---

## Summary & Next Steps 📋

This comprehensive plan provides:

1. **🌐 Universal Organization Context**: Single organization selector that works for all user roles with appropriate permissions
2. **👥 Role-Based User Management**: Unified interface that adapts actions and capabilities based on user's role in current organization
3. **⚙️ Permission-Driven UI**: Same components, different capabilities - members see read-only, owners get full management, super admins get ownership controls
4. **📱 Responsive Design**: Mobile-first approach using existing UI components
5. **🔧 Intuitive Architecture**: Super admins use the same org selector → user management flow, just with additional permissions

**Key Insight**: Instead of separate "super admin" and "regular" interfaces, we use a single unified interface where the organization selector provides context and the user's role in that context determines available actions. This creates an intuitive, consistent experience where super admins simply "switch into" an organization and then have additional powers within that context.

**Immediate Next Step**: Begin with Phase 1 (Organization Context System) as it's the foundation that enables this unified, role-based approach throughout the application.
