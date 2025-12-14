import React, { useEffect, useState } from 'react';
import { useAppSelector } from '../store/configureStore';
import { BlockingSubscriptionModal } from './Subscriptions/BlockingSubscriptionModal';
import { isCloudMode } from '../utils/deploymentMode';

interface SubscriptionGuardProps {
    children: React.ReactNode;
}

export const SubscriptionGuard: React.FC<SubscriptionGuardProps> = ({ children }) => {
    const currentOrg = useAppSelector(state => state.Organizations.currentOrg);
    const currentUser = useAppSelector(state => state.Auth.user);
    const organizations = useAppSelector(state => state.Organizations.userOrganizations);
    const authLoading = useAppSelector(state => state.Auth.isLoading);
    const orgLoading = useAppSelector(state => state.Organizations.loading.currentOrg);

    // Determine if access is blocked
    const [isBlocked, setIsBlocked] = useState(false);

    useEffect(() => {
        // Skip check if not cloud mode
        if (!isCloudMode()) {
            setIsBlocked(false);
            return;
        }

        // Skip if loading or data missing
        if (authLoading || orgLoading || !currentOrg || !currentUser) {
            // Don't modify block state while loading to avoid flashing?
            // If we are blocked, and strictly loading (e.g. switching org), we might want to keep blocked?
            // But if we switch org, we want to reset.
            if (!currentOrg) setIsBlocked(false);
            return;
        }

        // Logic matches backend:
        // If Plan == Free AND !IsCreator AND !IsSuperAdmin -> Block

        // 1. Check Plan
        // Default to 'free' if missing in cloud mode, but be careful with 'undefined'
        const isFree = currentOrg.plan_type === 'free';

        // 2. Check Creator
        // If created_by_user_id is missing (legacy payload), default to treating the user as creator to avoid false blocks
        const isCreator = currentOrg.created_by_user_id === undefined || currentOrg.created_by_user_id === currentUser.id;

        // 3. Check Super Admin
        // Check if user has 'super_admin' role in ANY org
        const isSuperAdmin = organizations.some(o => o.role === 'super_admin');

        /*
        console.debug('[SubscriptionGuard] Checking access:', {
            org: currentOrg.name,
            plan: currentOrg.plan_type,
            user: currentUser.email,
            isCreator,
            isSuperAdmin,
            isFree
        });
        */

        if (isFree && !isCreator && !isSuperAdmin) {
            setIsBlocked(true);
        } else {
            setIsBlocked(false);
        }

    }, [currentOrg, currentUser, organizations, authLoading, orgLoading]);

    if (isBlocked && currentOrg) {
        return <BlockingSubscriptionModal orgName={currentOrg.name} />;
    }

    return <>{children}</>;
};
