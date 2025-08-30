import React, { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import classNames from 'classnames';
import { Button } from '../UIPrimitives';
import { Member } from '../../api/users';
import { useOrgContext } from '../../hooks/useOrgContext';

export interface UserListProps {
    /**
     * Users to display
     */
    users: Member[];
    
    /**
     * Loading state
     */
    loading?: boolean;
    
    /**
     * Error message
     */
    error?: string | null;
    
    /**
     * Whether the current user can manage users
     */
    canManageUsers?: boolean;
    
    /**
     * Whether this is a super admin view (shows all orgs)
     */
    isSuperAdminView?: boolean;
    
    /**
     * Callback when user should be deactivated
     */
    onDeactivateUser?: (user: Member) => void;
    
    /**
     * Callback when user should be transferred (super admin only)
     */
    onTransferUser?: (user: Member) => void;
    
    /**
     * Callback to refresh the user list
     */
    onRefresh?: () => void;
}

export const UserList: React.FC<UserListProps> = ({
    users,
    loading = false,
    error,
    canManageUsers = false,
    isSuperAdminView = false,
    onDeactivateUser,
    onTransferUser,
    onRefresh,
}) => {
    const { currentOrg } = useOrgContext();
    const [selectedUsers, setSelectedUsers] = useState<Set<number>>(new Set());

    // Clear selection when users change
    useEffect(() => {
        setSelectedUsers(new Set());
    }, [users]);

    const handleSelectUser = (userId: number) => {
        const newSelection = new Set(selectedUsers);
        if (newSelection.has(userId)) {
            newSelection.delete(userId);
        } else {
            newSelection.add(userId);
        }
        setSelectedUsers(newSelection);
    };

    const handleSelectAll = () => {
        if (selectedUsers.size === users.length) {
            setSelectedUsers(new Set());
        } else {
            setSelectedUsers(new Set(users.map(u => u.id)));
        }
    };

    // Loading state
    if (loading && users.length === 0) {
        return (
            <div className="bg-slate-800 rounded-lg border border-slate-700">
                <div className="p-6 text-center">
                    <div className="animate-spin w-8 h-8 border-2 border-blue-400 border-t-transparent rounded-full mx-auto mb-4"></div>
                    <p className="text-slate-400">Loading users...</p>
                </div>
            </div>
        );
    }

    // Error state
    if (error) {
        return (
            <div className="bg-slate-800 rounded-lg border border-slate-700">
                <div className="p-6 text-center">
                    <div className="w-12 h-12 bg-red-900/20 rounded-full flex items-center justify-center mx-auto mb-4">
                        <svg className="w-6 h-6 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                    </div>
                    <p className="text-red-400 mb-4">{error}</p>
                    {onRefresh && (
                        <Button
                            variant="secondary"
                            onClick={onRefresh}
                            className="text-sm"
                        >
                            Try Again
                        </Button>
                    )}
                </div>
            </div>
        );
    }

    // Empty state
    if (users.length === 0) {
        return (
            <div className="bg-slate-800 rounded-lg border border-slate-700">
                <div className="p-6 text-center">
                    <div className="w-12 h-12 bg-slate-700 rounded-full flex items-center justify-center mx-auto mb-4">
                        <svg className="w-6 h-6 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197m13.5-9a2.5 2.5 0 11-5 0 2.5 2.5 0 015 0z" />
                        </svg>
                    </div>
                    <p className="text-slate-400 mb-2">No users found</p>
                    <p className="text-slate-500 text-sm">
                        {isSuperAdminView 
                            ? 'No users exist in any organization.' 
                            : `No users found in ${currentOrg?.name || 'this organization'}.`
                        }
                    </p>
                </div>
            </div>
        );
    }

    return (
        <div className="bg-slate-800 rounded-lg border border-slate-700 overflow-hidden">
            {/* Header */}
            <div className="px-6 py-4 border-b border-slate-700 bg-slate-900/50">
                <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-4">
                        <h3 className="text-lg font-medium text-white">
                            Users {isSuperAdminView ? '(All Organizations)' : `in ${currentOrg?.name}`}
                        </h3>
                        <span className="text-sm text-slate-400">
                            {users.length} user{users.length !== 1 ? 's' : ''}
                        </span>
                    </div>
                    
                    {onRefresh && (
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={onRefresh}
                            disabled={loading}
                            icon={
                                <svg className={classNames('w-4 h-4', loading && 'animate-spin')} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                                </svg>
                            }
                        >
                            Refresh
                        </Button>
                    )}
                </div>
            </div>

            {/* Bulk Actions (if users selected and can manage) */}
            {selectedUsers.size > 0 && canManageUsers && (
                <div className="px-6 py-3 bg-blue-900/20 border-b border-slate-700">
                    <div className="flex items-center justify-between">
                        <span className="text-sm text-blue-300">
                            {selectedUsers.size} user{selectedUsers.size !== 1 ? 's' : ''} selected
                        </span>
                        <div className="flex items-center space-x-2">
                            <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => setSelectedUsers(new Set())}
                                className="text-slate-400"
                            >
                                Clear
                            </Button>
                            {/* Add bulk actions here if needed */}
                        </div>
                    </div>
                </div>
            )}

            {/* Table */}
            <div className="overflow-x-auto">
                <table className="w-full">
                    <thead className="bg-slate-900/30">
                        <tr>
                            {canManageUsers && (
                                <th className="w-8 px-6 py-3 text-left">
                                    <input
                                        type="checkbox"
                                        checked={selectedUsers.size === users.length}
                                        onChange={handleSelectAll}
                                        className="rounded border-slate-600 bg-slate-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-slate-800"
                                    />
                                </th>
                            )}
                            <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                                User
                            </th>
                            <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                                Status
                            </th>
                            <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                                Role
                            </th>
                            {isSuperAdminView && (
                                <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                                    Organizations
                                </th>
                            )}
                            <th className="px-6 py-3 text-left text-xs font-medium text-slate-400 uppercase tracking-wider">
                                Joined
                            </th>
                            {canManageUsers && (
                                <th className="px-6 py-3 text-right text-xs font-medium text-slate-400 uppercase tracking-wider">
                                    Actions
                                </th>
                            )}
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-700">
                        {users.map((user) => (
                            <tr key={user.id} className="hover:bg-slate-700/30 transition-colors">
                                {canManageUsers && (
                                    <td className="px-6 py-4">
                                        <input
                                            type="checkbox"
                                            checked={selectedUsers.has(user.id)}
                                            onChange={() => handleSelectUser(user.id)}
                                            className="rounded border-slate-600 bg-slate-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-slate-800"
                                        />
                                    </td>
                                )}
                                <td className="px-6 py-4">
                                    <div className="flex items-center">
                                        <div className="w-10 h-10 bg-slate-700 rounded-full flex items-center justify-center mr-3">
                                            <span className="text-sm font-medium text-slate-300">
                                                {(user.first_name || user.email).charAt(0).toUpperCase()}
                                            </span>
                                        </div>
                                        <div>
                                            <div className="text-sm font-medium text-white">
                                                {user.first_name && user.last_name ? `${user.first_name} ${user.last_name}` : user.email}
                                            </div>
                                            <div className="text-sm text-slate-400">
                                                {user.email}
                                            </div>
                                        </div>
                                    </div>
                                </td>
                                <td className="px-6 py-4">
                                    <span className={classNames(
                                        'inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium',
                                        user.is_active 
                                            ? 'bg-green-900/20 text-green-300' 
                                            : 'bg-red-900/20 text-red-300'
                                    )}>
                                        {user.is_active ? 'Active' : 'Inactive'}
                                    </span>
                                </td>
                                <td className="px-6 py-4 text-sm text-slate-400 capitalize">
                                    {user.role}
                                </td>
                                {isSuperAdminView && (
                                    <td className="px-6 py-4">
                                        <span className="text-slate-500 text-sm">N/A</span>
                                    </td>
                                )}
                                <td className="px-6 py-4 text-sm text-slate-400">
                                    {new Date(user.created_at).toLocaleDateString()}
                                </td>
                                {canManageUsers && (
                                    <td className="px-6 py-4 text-right text-sm font-medium">
                                        <div className="flex items-center justify-end space-x-2">
                                            <Button
                                                variant="ghost"
                                                size="sm"
                                                as={Link}
                                                to={`/settings/users/edit/${user.id}`}
                                                className="text-blue-400 hover:text-blue-300"
                                            >
                                                Edit
                                            </Button>
                                            {onTransferUser && isSuperAdminView && (
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => onTransferUser(user)}
                                                    className="text-purple-400 hover:text-purple-300"
                                                >
                                                    Transfer
                                                </Button>
                                            )}
                                            {onDeactivateUser && user.is_active && (
                                                <Button
                                                    variant="ghost"
                                                    size="sm"
                                                    onClick={() => onDeactivateUser(user)}
                                                    className="text-red-400 hover:text-red-300"
                                                >
                                                    Deactivate
                                                </Button>
                                            )}
                                        </div>
                                    </td>
                                )}
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>

            {/* Loading overlay for refresh */}
            {loading && users.length > 0 && (
                <div className="absolute inset-0 bg-slate-900/50 flex items-center justify-center">
                    <div className="bg-slate-800 rounded-lg p-4 border border-slate-700">
                        <div className="flex items-center space-x-3">
                            <div className="animate-spin w-5 h-5 border-2 border-blue-400 border-t-transparent rounded-full"></div>
                            <span className="text-slate-300">Updating...</span>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
};
