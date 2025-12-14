import { createSlice, createAsyncThunk, PayloadAction } from '@reduxjs/toolkit';
import {
    OrganizationState,
    Organization,
    OrganizationMember,
    SwitchOrganizationPayload,
    CreateOrganizationPayload,
    UpdateOrganizationPayload,
    ChangeUserRolePayload,
    LoadMembersPayload
} from './types';
import { organizationsApi } from '../../api/organizations';
import { logout } from '../Auth/reducer';

// Initial state
const initialState: OrganizationState = {
    currentOrgId: null,
    currentOrg: null,
    userOrganizations: [],
    currentOrgMembers: [],
    allOrganizations: [],
    loading: {
        organizations: false,
        currentOrg: false,
        members: false,
        switching: false,
        creating: false,
        updating: false,
    },
    error: null,
    orgSelectorOpen: false,
};

// Async thunks
export const loadUserOrganizations = createAsyncThunk(
    'organizations/loadUserOrganizations',
    async (_, { rejectWithValue }) => {
        try {
            const organizations = await organizationsApi.getUserOrganizations();
            return organizations;
        } catch (error: any) {
            return rejectWithValue(error.response?.data?.message || 'Failed to load organizations');
        }
    }
);

export const loadAllOrganizations = createAsyncThunk(
    'organizations/loadAllOrganizations',
    async (_, { rejectWithValue }) => {
        try {
            const organizations = await organizationsApi.getAllOrganizations();
            return organizations;
        } catch (error: any) {
            return rejectWithValue(error.response?.data?.message || 'Failed to load all organizations');
        }
    }
);

export const switchOrganization = createAsyncThunk(
    'organizations/switchOrganization',
    async ({ orgId }: SwitchOrganizationPayload, { rejectWithValue, getState }) => {
        try {
            const state = getState() as any;
            const organizations = state.Organizations.userOrganizations;
            const org = organizations.find((o: Organization) => o.id === orgId);
            
            if (!org) {
                throw new Error('Organization not found');
            }
            
            // Store in localStorage for persistence
            localStorage.setItem('currentOrgId', orgId.toString());
            
            return org;
        } catch (error: any) {
            return rejectWithValue(error.message || 'Failed to switch organization');
        }
    }
);

export const loadOrganizationMembers = createAsyncThunk(
    'organizations/loadOrganizationMembers',
    async ({ orgId }: LoadMembersPayload, { rejectWithValue }) => {
        try {
            const members = await organizationsApi.getOrganizationMembers(orgId);
            return members;
        } catch (error: any) {
            return rejectWithValue(error.response?.data?.message || 'Failed to load organization members');
        }
    }
);

export const createOrganization = createAsyncThunk(
    'organizations/createOrganization',
    async (payload: CreateOrganizationPayload, { rejectWithValue }) => {
        try {
            const organization = await organizationsApi.createOrganization(payload);
            return organization;
        } catch (error: any) {
            return rejectWithValue(error.message || 'Failed to create organization');
        }
    }
);

export const updateOrganization = createAsyncThunk(
    'organizations/updateOrganization',
    async ({ orgId, ...payload }: UpdateOrganizationPayload, { rejectWithValue }) => {
        try {
            const organization = await organizationsApi.updateOrganization(orgId, payload);
            return organization;
        } catch (error: any) {
            return rejectWithValue(error.response?.data?.message || 'Failed to update organization');
        }
    }
);

export const changeUserRole = createAsyncThunk(
    'organizations/changeUserRole',
    async ({ orgId, userId, role }: ChangeUserRolePayload, { rejectWithValue }) => {
        try {
            await organizationsApi.changeUserRole(orgId, userId, role);
            return { userId, role };
        } catch (error: any) {
            return rejectWithValue(error.response?.data?.message || 'Failed to change user role');
        }
    }
);

// Slice
const organizationsSlice = createSlice({
    name: 'organizations',
    initialState,
    reducers: {
        setOrgSelectorOpen(state, action: PayloadAction<boolean>) {
            state.orgSelectorOpen = action.payload;
        },
        clearError(state) {
            state.error = null;
        },
        initializeFromStorage(state) {
            const currentOrgId = localStorage.getItem('currentOrgId');
            if (currentOrgId) {
                state.currentOrgId = parseInt(currentOrgId, 10);
            }
        },
        setOrganizationsFromAuth(state, action: PayloadAction<Organization[]>) {
            // Set organizations directly from auth response (avoid extra API call)
            state.userOrganizations = action.payload;
            
            const orgs = action.payload;
            if (orgs && orgs.length > 0) {
                const storedOrgId = state.currentOrgId; // Use state's currentOrgId which was initialized from storage
                let orgToSelect = storedOrgId ? orgs.find(o => o.id === storedOrgId) : undefined;

                if (!orgToSelect) {
                    orgToSelect = orgs[0];
                }
                
                if (orgToSelect) {
                    state.currentOrg = orgToSelect;
                    state.currentOrgId = orgToSelect.id;
                    localStorage.setItem('currentOrgId', orgToSelect.id.toString());
                }
            } else {
                state.currentOrg = null;
                state.currentOrgId = null;
                localStorage.removeItem('currentOrgId');
            }
        },
    },
    extraReducers: (builder) => {
        builder
            .addCase(loadUserOrganizations.pending, (state) => {
                state.loading.organizations = true;
            })
            .addCase(loadUserOrganizations.fulfilled, (state, action) => {
                state.loading.organizations = false;
                state.userOrganizations = action.payload;

                const orgs = action.payload;
                if (orgs && orgs.length > 0) {
                    const storedOrgId = state.currentOrgId; // Use state's currentOrgId which was initialized from storage
                    let orgToSelect = storedOrgId ? orgs.find(o => o.id === storedOrgId) : undefined;

                    if (!orgToSelect) {
                        orgToSelect = orgs[0];
                    }
                    
                    if (orgToSelect) {
                        state.currentOrg = orgToSelect;
                        state.currentOrgId = orgToSelect.id;
                        localStorage.setItem('currentOrgId', orgToSelect.id.toString());
                    }
                } else {
                    state.currentOrg = null;
                    state.currentOrgId = null;
                    localStorage.removeItem('currentOrgId');
                }
            })
            .addCase(loadUserOrganizations.rejected, (state, action) => {
                state.loading.organizations = false;
                state.error = action.payload as string;
            })
            .addCase(loadAllOrganizations.pending, (state) => {
                state.loading.organizations = true;
            })
            .addCase(loadAllOrganizations.fulfilled, (state, action) => {
                state.loading.organizations = false;
                state.allOrganizations = action.payload;
            })
            .addCase(loadAllOrganizations.rejected, (state, action) => {
                state.loading.organizations = false;
                state.error = action.payload as string;
            })
            .addCase(switchOrganization.fulfilled, (state, action) => {
                state.currentOrg = action.payload;
                state.currentOrgId = action.payload.id;
            })
            .addCase(switchOrganization.rejected, (state, action) => {
                state.loading.switching = false;
                state.error = action.payload as string;
            })
            .addCase(loadOrganizationMembers.pending, (state) => {
                state.loading.members = true;
            })
            .addCase(loadOrganizationMembers.fulfilled, (state, action) => {
                state.loading.members = false;
                state.currentOrgMembers = action.payload;
            })
            .addCase(loadOrganizationMembers.rejected, (state, action) => {
                state.loading.members = false;
                state.error = action.payload as string;
            })
            .addCase(createOrganization.pending, (state) => {
                state.loading.creating = true;
                state.error = null;
            })
            .addCase(createOrganization.fulfilled, (state, action) => {
                state.loading.creating = false;
                state.userOrganizations.push(action.payload);
            })
            .addCase(createOrganization.rejected, (state, action) => {
                state.loading.creating = false;
                state.error = action.payload as string;
            })
            .addCase(updateOrganization.pending, (state) => {
                state.loading.updating = true;
                state.error = null;
            })
            .addCase(updateOrganization.fulfilled, (state, action) => {
                state.loading.updating = false;
                const index = state.userOrganizations.findIndex(org => org.id === action.payload.id);
                if (index !== -1) {
                    state.userOrganizations[index] = action.payload;
                }
            })
            .addCase(updateOrganization.rejected, (state, action) => {
                state.loading.updating = false;
                state.error = action.payload as string;
            })
            .addCase(changeUserRole.pending, (state) => {
                state.loading.updating = true;
                state.error = null;
            })
            .addCase(changeUserRole.fulfilled, (state, action) => {
                state.loading.updating = false;
                
                // Update member role in current org members
                const member = state.currentOrgMembers.find(m => m.user_id === action.payload.userId);
                if (member) {
                    member.role = action.payload.role;
                }
            })
            .addCase(changeUserRole.rejected, (state, action) => {
                state.loading.updating = false;
                state.error = action.payload as string;
            })
            .addCase(logout.fulfilled, (state) => {
                // Reset to initial state on logout
                Object.assign(state, initialState);
            })
            // Listen for auth login/setupAdmin actions to populate organizations immediately
            .addMatcher(
                (action) => action.type === 'auth/login/fulfilled' || action.type === 'auth/setupAdmin/fulfilled',
                (state, action: any) => {
                    // Populate organizations from auth response
                    if (action.payload?.organizations && action.payload.organizations.length > 0) {
                        state.userOrganizations = action.payload.organizations;
                        
                        const orgs = action.payload.organizations;
                        const storedOrgId = state.currentOrgId;
                        let orgToSelect = storedOrgId ? orgs.find((o: Organization) => o.id === storedOrgId) : undefined;

                        if (!orgToSelect) {
                            orgToSelect = orgs[0];
                        }
                        
                        if (orgToSelect) {
                            state.currentOrg = orgToSelect;
                            state.currentOrgId = orgToSelect.id;
                            localStorage.setItem('currentOrgId', orgToSelect.id.toString());
                        }
                    }
                }
            );
    },
});

export const { setOrgSelectorOpen, clearError, initializeFromStorage, setOrganizationsFromAuth } = organizationsSlice.actions;
export default organizationsSlice.reducer;