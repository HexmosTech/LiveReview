import { useCallback, useEffect } from 'react';
import { useAppSelector, useAppDispatch } from '../store/configureStore';
import {
    loadUserOrganizations,
    loadAllOrganizations,
    switchOrganization,
    loadOrganizationMembers,
    createOrganization,
    updateOrganization,
    changeUserRole,
    setOrgSelectorOpen,
    clearError,
    initializeFromStorage
} from '../store/Organizations/reducer';
import {
    Organization,
    OrganizationMember,
    CreateOrganizationPayload,
    UpdateOrganizationPayload,
    ChangeUserRolePayload
} from '../store/Organizations/types';

/**
 * Custom hook for organization context and operations
 */
export const useOrgContext = () => {
    const dispatch = useAppDispatch();
    
    const {
        currentOrgId,
        currentOrg,
        userOrganizations,
        currentOrgMembers,
        allOrganizations,
        loading,
        error,
        orgSelectorOpen
    } = useAppSelector((state) => state.Organizations);
    
    const { user, organizations } = useAppSelector((state) => state.Auth);
    // For now, we'll determine super admin status based on organization roles
    // TODO: Update this once the backend provides the super admin flag
    const isSuperAdmin = organizations?.some(org => org.role === 'super_admin') || false;

    // Initialize organization context on mount
    useEffect(() => {
        if (!userOrganizations.length) {
            dispatch(loadUserOrganizations() as any);
        }
        dispatch(initializeFromStorage() as any);
    }, [dispatch, userOrganizations.length]);

    // Load user organizations
    const loadUserOrgs = useCallback(() => {
        dispatch(loadUserOrganizations() as any);
    }, [dispatch]);

    // Load all organizations (super admin)
    const loadAllOrgs = useCallback(() => {
        dispatch(loadAllOrganizations() as any);
    }, [dispatch]);

    // Switch to a different organization
    const switchToOrg = useCallback((orgId: number) => {
        dispatch(switchOrganization({ orgId }) as any);
    }, [dispatch]);

    // Load members for current organization
    const loadMembers = useCallback((orgId?: number) => {
        const targetOrgId = orgId || currentOrgId;
        if (targetOrgId) {
            dispatch(loadOrganizationMembers({ orgId: targetOrgId }) as any);
        }
    }, [dispatch, currentOrgId]);

    // Create new organization (super admin)
    const createOrg = useCallback((payload: CreateOrganizationPayload) => {
        return dispatch(createOrganization(payload) as any);
    }, [dispatch]);

    // Update organization
    const updateOrg = useCallback((payload: UpdateOrganizationPayload) => {
        return dispatch(updateOrganization(payload) as any);
    }, [dispatch]);

    // Change user role in organization
    const changeRole = useCallback((payload: ChangeUserRolePayload) => {
        return dispatch(changeUserRole(payload) as any);
    }, [dispatch]);

    // Toggle organization selector
    const toggleOrgSelector = useCallback(() => {
        dispatch(setOrgSelectorOpen(!orgSelectorOpen) as any);
    }, [dispatch, orgSelectorOpen]);

    const closeOrgSelector = useCallback(() => {
        dispatch(setOrgSelectorOpen(false) as any);
    }, [dispatch]);

    const openOrgSelector = useCallback(() => {
        dispatch(setOrgSelectorOpen(true) as any);
    }, [dispatch]);

    // Clear error
    const clearOrgError = useCallback(() => {
        dispatch(clearError() as any);
    }, [dispatch]);

    return {
        // State
        currentOrgId,
        currentOrg,
        userOrganizations,
        currentOrgMembers,
        allOrganizations,
        loading,
        error,
        orgSelectorOpen,
        isSuperAdmin,
        
        // Actions
        loadUserOrgs,
        loadAllOrgs,
        switchToOrg,
        loadMembers,
        createOrg,
        updateOrg,
        changeRole,
        toggleOrgSelector,
        closeOrgSelector,
        openOrgSelector,
        clearOrgError,
        
        // Computed values
        hasOrganizations: userOrganizations.length > 0,
        canCreateOrgs: isSuperAdmin,
                canManageCurrentOrg: currentOrg?.role === 'owner' || isSuperAdmin,
    };
};