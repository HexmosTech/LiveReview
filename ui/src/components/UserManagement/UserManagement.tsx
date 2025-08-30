import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { Button } from '../UIPrimitives';
import { UserList } from './UserList';
import { useOrgContext } from '../../hooks/useOrgContext';
import { fetchOrgUsers, Member, deactivateOrgUser } from '../../api/users';

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
    const [users, setUsers] = useState<Member[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const canManageUsers = isSuperAdminView ? isSuperAdmin : canManageCurrentOrg;

    // Load users
    const loadUsers = async () => {
        if (isSuperAdminView && !isSuperAdmin) {
            setError('Access denied: Super admin privileges required');
            return;
        }

        if (!isSuperAdminView && !currentOrg) {
            // This can happen briefly on first load, so don't set an error
            return;
        }

        setLoading(true);
        setError(null);
        
        try {
            if (isSuperAdminView) {
                // TODO: Implement super admin user fetching
                setUsers([]);
            } else if (currentOrg) {
                const response = await fetchOrgUsers(currentOrg.id.toString());
                setUsers(response.members);
            }
        } catch (err: any) {
            console.error('Failed to load users:', err);
            setError(err.message || 'Failed to load users');
        } finally {
            setLoading(false);
        }
    };

    // Load users on mount and when context changes
    useEffect(() => {
        loadUsers();
    }, [currentOrg, isSuperAdminView, isSuperAdmin]);

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
                </div>
                
                {canManageUsers && (
                    <Button
                        variant="primary"
                        as={Link}
                        to="/settings/users/add"
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
        </div>
    );
};