// Organization types for Redux store
export interface Organization {
    id: number;
    name: string;
    description?: string;
    is_active: boolean;
    created_at: string;
    updated_at: string;
    settings?: any; // Or a more specific type if known
    subscription_plan?: string;
    max_users?: number;
    created_by_user_id?: number;
    member_count?: number;
    role?: string; 
}

export interface OrganizationMember {
    user_id: number;
    username: string;
    email: string;
    full_name?: string;
    role: string;
    joined_at: string;
}

export interface OrganizationState {
    // Current organization context
    currentOrgId: number | null;
    currentOrg: Organization | null;
    
    // Available organizations for current user
    userOrganizations: Organization[];
    
    // Organization members (when viewing org details)
    currentOrgMembers: OrganizationMember[];
    
    // All organizations (super admin view)
    allOrganizations: Organization[];
    
    // Loading states
    loading: {
        organizations: boolean;
        currentOrg: boolean;
        members: boolean;
        switching: boolean;
        creating: boolean;
        updating: boolean;
    };
    
    // Error handling
    error: string | null;
    
    // UI state
    orgSelectorOpen: boolean;
}

// Action payload types
export interface SwitchOrganizationPayload {
    orgId: number;
}

export interface CreateOrganizationPayload {
    name: string;
    description?: string;
}

export interface UpdateOrganizationPayload {
    orgId: number;
    name?: string;
    description?: string;
    is_active?: boolean;
}

export interface ChangeUserRolePayload {
    orgId: number;
    userId: number;
    role: string;
}

export interface LoadMembersPayload {
    orgId: number;
}