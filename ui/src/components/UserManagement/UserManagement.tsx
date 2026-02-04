import React, { useState, useEffect, useCallback } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { useSelector } from 'react-redux';
import { Button } from '../UIPrimitives';
import { UserList } from './UserList';
import { useOrgContext } from '../../hooks/useOrgContext';
import { fetchOrgUsers, Member, deactivateOrgUser } from '../../api/users';
import { isCloudMode } from '../../utils/deploymentMode';
import { RootState } from '../../store/configureStore';
import LicenseUpgradeDialog from '../License/LicenseUpgradeDialog';
import { useLicenseTier, useHasLicenseFor, COMMUNITY_TIER_LIMITS } from '../../hooks/useLicenseTier';

export interface UserManagementProps {
    /**
     * Whether this is a super admin view
     */
    isSuperAdminView?: boolean;
}

export const UserManagement: React.FC<UserManagementProps> = ({
    isSuperAdminView = false,
}) => {
    const { currentOrg, isSuperAdmin, canManageCurrentOrg } = useOrgContext();
    const license = useSelector((state: RootState) => state.License);
    const navigate = useNavigate();
    const [users, setUsers] = useState<Member[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);
    const licenseTier = useLicenseTier();
    const hasTeamLicense = useHasLicenseFor('team');

    const canManageUsers = isSuperAdminView ? isSuperAdmin : canManageCurrentOrg;

    // Load users
    const loadUsers = useCallback(async () => {
        console.log('[UserManagement] loadUsers called', { 
            orgId: currentOrg?.id, 
            isSuperAdminView, 
            isSuperAdmin 
        });
        
        if (isSuperAdminView && !isSuperAdmin) {
            setError('Access denied: Super admin privileges required');
            return;
        }

        if (!isSuperAdminView && !currentOrg) {
            // This can happen briefly on first load, so don't set an error
            console.log('[UserManagement] Skipping load - no currentOrg yet');
            return;
        }

        setLoading(true);
        setError(null);
        
        try {
            if (isSuperAdminView) {
                // TODO: Implement super admin user fetching
                setUsers([]);
            } else if (currentOrg) {
                console.log('[UserManagement] Fetching users for org:', currentOrg.id);
                const response = await fetchOrgUsers(currentOrg.id.toString());
                setUsers(response.members || []);
            }
        } catch (err: any) {
            console.error('Failed to load users:', err);
            setError(err.message || 'Failed to load users');
        } finally {
            setLoading(false);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [currentOrg?.id, isSuperAdminView, isSuperAdmin]);

    // Load users on mount and when context changes
    useEffect(() => {
        loadUsers();
    }, [loadUsers]);

    const handleDeactivateUser = async (user: Member) => {
        const userName = user.first_name && user.last_name ? `${user.first_name} ${user.last_name}` : user.email;
        if (!window.confirm(`Are you sure you want to deactivate ${userName}?`)) {
            return;
        }

        try {
            if (isSuperAdminView) {
                // TODO: Implement super admin deactivation
                console.log('Super admin deactivate user:', user);
                toast.error('Super admin deactivation not yet implemented.');
            } else if (currentOrg) {
                await deactivateOrgUser(currentOrg.id.toString(), user.id.toString());
                toast.success(`User ${userName} deactivated successfully.`);
                await loadUsers(); // Refresh list
            }
        } catch (err: any) {
            console.error('Failed to deactivate user:', err);
            setError(err.message || 'Failed to deactivate user');
            toast.error(`Failed to deactivate user: ${err.message}`);
        }
    };

    const handleTransferUser = (user: Member) => {
        // TODO: Open transfer user modal
        console.log('Transfer user:', user);
        toast.error('User transfer not yet implemented.');
    };

    return (
        <div className="space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
                <div>
                    <h2 className="text-xl font-semibold text-white">
                        {isSuperAdminView ? 'All Users (Super Admin)' : 'User Management'}
                    </h2>
                    <p className="text-slate-400 mt-1">
                        {isSuperAdminView 
                            ? 'Manage users across all organizations'
                            : `Manage users in ${currentOrg?.name || 'your organization'}`
                        }
                    </p>
                    {!isSuperAdminView && currentOrg && currentOrg.created_at && (
                        <p className="text-slate-500 text-sm mt-1">
                            Organization created on {new Date(currentOrg.created_at).toLocaleDateString()}
                            {currentOrg.creator_email && (
                                <> by {
                                    currentOrg.creator_first_name && currentOrg.creator_last_name 
                                        ? `${currentOrg.creator_first_name} ${currentOrg.creator_last_name}` 
                                        : currentOrg.creator_email
                                }</>
                            )}
                        </p>
                    )}
                </div>
                
                {canManageUsers && (
                    <Button
                        variant="primary"
                        onClick={() => {
                            // Require Team license for more than allowed Community users (super admins exempt)
                            if (users.length >= COMMUNITY_TIER_LIMITS.MAX_USERS && !hasTeamLicense && !isSuperAdmin) {
                                setShowUpgradeDialog(true);
                                return;
                            }
                            navigate('/settings/users/add');
                        }}
                        icon={
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                            </svg>
                        }
                    >
                        Add User
                    </Button>
                )}
            </div>

            {/* Free Plan Info Banner - Cloud only */}
            {isCloudMode() && !isSuperAdminView && currentOrg?.plan_type === 'free' && (
                <div className="bg-blue-900/20 border border-blue-500/30 rounded-lg p-4">
                    <div className="flex items-start">
                        <svg className="w-5 h-5 text-blue-400 mr-3 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        <div className="flex-1">
                            <h3 className="text-blue-200 font-semibold mb-1">Free Plan Limitations</h3>
                            <p className="text-blue-100/80 text-sm mb-2">
                                You can add members to your organization, but on the Free plan only the organization creator has access. 
                                Added members won't be able to access reviews or trigger new reviews.
                            </p>
                            <p className="text-blue-100/80 text-sm">
                                <Link to="/subscribe" className="text-blue-300 hover:text-blue-200 underline font-medium">
                                    Upgrade to Team Plan
                                </Link>
                                {' '}to give all team members full access with unlimited reviews.
                            </p>
                        </div>
                    </div>
                </div>
            )}

            {/* Access Control Messages */}
            {!isSuperAdminView && !canManageCurrentOrg && (
                <div className="bg-yellow-900/20 border border-yellow-600/30 rounded-lg p-4">
                    <div className="flex items-center">
                        <svg className="w-5 h-5 text-yellow-400 mr-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L3.732 16.5c-.77.833.192 2.5 1.732 2.5z" />
                        </svg>
                        <p className="text-yellow-300">
                            You can view users in this organization but don't have permission to manage them. 
                            Contact an organization owner for management capabilities.
                        </p>
                    </div>
                </div>
            )}

            {isSuperAdminView && !isSuperAdmin && (
                <div className="bg-red-900/20 border border-red-600/30 rounded-lg p-4">
                    <div className="flex items-center">
                        <svg className="w-5 h-5 text-red-400 mr-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        <p className="text-red-300">
                            Access denied. Super administrator privileges are required to view all users.
                        </p>
                    </div>
                </div>
            )}

            {/* User List */}
            <UserList
                users={users}
                loading={loading}
                error={error}
                canManageUsers={canManageUsers}
                isSuperAdminView={isSuperAdminView}
                onDeactivateUser={handleDeactivateUser}
                onTransferUser={isSuperAdminView ? handleTransferUser : undefined}
                onRefresh={loadUsers}
            />

            {/* License Upgrade Dialog */}
            <LicenseUpgradeDialog
                open={showUpgradeDialog}
                onClose={() => setShowUpgradeDialog(false)}
                requiredTier="team"
                featureName="Team Management (>3 users)"
                featureDescription="Add more team members to your organization. Community tier includes up to 3 users."
            />
        </div>
    );
};