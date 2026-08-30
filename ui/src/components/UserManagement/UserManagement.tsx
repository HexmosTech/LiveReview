import React, { useState, useEffect, useCallback, useRef } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { notify } from '../../utils/notify';
import { useSelector } from 'react-redux';
import { SortingState, ColumnFiltersState, PaginationState, OnChangeFn } from '@tanstack/react-table';
import { Button } from '../UIPrimitives';
import { UserList } from './UserList';
import { useOrgContext } from '../../hooks/useOrgContext';
import { fetchOrgUsers, FetchOrgUsersParams, Member, deactivateOrgUser } from '../../api/users';
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
    const { currentOrg, isSuperAdmin, canManageCurrentOrg, isFreePlan } = useOrgContext();
    const license = useSelector((state: RootState) => state.License);
    const navigate = useNavigate();
    const [users, setUsers] = useState<Member[]>([]);
    const [total, setTotal] = useState(0);
    const [totalPages, setTotalPages] = useState(1);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);
    const licenseTier = useLicenseTier();
    const hasTeamLicense = useHasLicenseFor('team');

    // Sorting/filtering/pagination are server-driven - these three are the
    // single source of truth the table's header controls (sort icons, search
    // input) read from and write to. Every change re-fetches the current
    // page from the API instead of the table recomputing rows from an
    // already-loaded client-side array (same pattern as Explore > Repositories).
    const [sorting, setSorting] = useState<SortingState>([]);
    const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([]);
    const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 10 });
    const searchDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    // Mirrors of the three state values above, read by refetchCurrent so it
    // can stay referentially stable (deps: [loadUsers] only) instead of
    // getting a new identity on every keystroke in the search box. A
    // changing refetchCurrent would ripple into handleDeactivateUser (which
    // calls it) and from there into UserList's memoized column defs -
    // remounting the "User" column's header and silently closing its search
    // popover mid-type. See .agents/design.md.
    const sortingRef = useRef(sorting);
    const columnFiltersRef = useRef(columnFilters);
    const paginationRef = useRef(pagination);
    useEffect(() => { sortingRef.current = sorting; }, [sorting]);
    useEffect(() => { columnFiltersRef.current = columnFilters; }, [columnFilters]);
    useEffect(() => { paginationRef.current = pagination; }, [pagination]);

    const canManageUsers = isSuperAdminView ? isSuperAdmin : canManageCurrentOrg;

    const buildFetchParams = (
        sortingState: SortingState,
        filtersState: ColumnFiltersState,
        paginationState: PaginationState
    ): FetchOrgUsersParams => {
        const sortEntry = sortingState[0];
        const search = filtersState.find((f) => f.id === 'user')?.value as string | undefined;
        return {
            page: paginationState.pageIndex + 1,
            perPage: paginationState.pageSize,
            search: search || undefined,
            sort: sortEntry ? (sortEntry.id as FetchOrgUsersParams['sort']) : undefined,
            order: sortEntry ? (sortEntry.desc ? 'desc' : 'asc') : undefined,
        };
    };

    // Load users
    const loadUsers = useCallback(async (params: FetchOrgUsersParams) => {
        console.log('[UserManagement] loadUsers called', {
            orgId: currentOrg?.id,
            isSuperAdminView,
            isSuperAdmin,
            params,
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
                setTotal(0);
                setTotalPages(1);
            } else if (currentOrg) {
                console.log('[UserManagement] Fetching users for org:', currentOrg.id);
                const response = await fetchOrgUsers(currentOrg.id.toString(), params);
                setUsers(response.members || []);
                setTotal(response.total || 0);
                setTotalPages(response.total_pages || 1);
            }
        } catch (err: any) {
            console.error('Failed to load users:', err);
            setError(err.message || 'Failed to load users');
        } finally {
            setLoading(false);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [currentOrg?.id, isSuperAdminView, isSuperAdmin]);

    const refetchCurrent = useCallback(() => {
        loadUsers(buildFetchParams(sortingRef.current, columnFiltersRef.current, paginationRef.current));
    }, [loadUsers]);

    // Load users on mount and when context changes
    useEffect(() => {
        loadUsers(buildFetchParams(sorting, columnFilters, pagination));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [loadUsers]);

    const handleSortingChange: OnChangeFn<SortingState> = (updater) => {
        const next = typeof updater === 'function' ? updater(sorting) : updater;
        const nextPagination = { ...pagination, pageIndex: 0 };
        setSorting(next);
        setPagination(nextPagination);
        loadUsers(buildFetchParams(next, columnFilters, nextPagination));
    };

    const handleColumnFiltersChange: OnChangeFn<ColumnFiltersState> = (updater) => {
        const next = typeof updater === 'function' ? updater(columnFilters) : updater;
        setColumnFilters(next);
        const nextPagination = { ...pagination, pageIndex: 0 };

        // Free-text search: debounce the actual request, but let the input's
        // own value (bound to column.getFilterValue()) update immediately via
        // the setColumnFilters call above, so typing itself stays responsive.
        if (searchDebounceRef.current) clearTimeout(searchDebounceRef.current);
        searchDebounceRef.current = setTimeout(() => {
            setPagination(nextPagination);
            loadUsers(buildFetchParams(sorting, next, nextPagination));
        }, 300);
    };

    const handlePaginationChange: OnChangeFn<PaginationState> = (updater) => {
        const next = typeof updater === 'function' ? updater(pagination) : updater;
        setPagination(next);
        loadUsers(buildFetchParams(sorting, columnFilters, next));
    };

    // useCallback here isn't just an optimization: UserList's column defs
    // (including each header's HeaderFilterPopover, which holds its own
    // open/closed state) are memoized on these two callbacks. A plain
    // function would get a new identity on every UserManagement re-render
    // (e.g. every keystroke in the search box), which would recompute those
    // columns and hand React a new header render-function reference for the
    // same column - React then treats it as a different component type and
    // remounts it, silently closing any open popover. See .agents/design.md.
    const handleDeactivateUser = useCallback(async (user: Member) => {
        const userName = user.first_name && user.last_name ? `${user.first_name} ${user.last_name}` : user.email;
        if (!window.confirm(`Are you sure you want to deactivate ${userName}?`)) {
            return;
        }

        try {
            if (isSuperAdminView) {
                // TODO: Implement super admin deactivation
                console.log('Super admin deactivate user:', user);
                notify.error('Super admin deactivation not yet implemented.');
            } else if (currentOrg) {
                await deactivateOrgUser(currentOrg.id.toString(), user.id.toString());
                notify.success(`User ${userName} deactivated successfully.`);
                await refetchCurrent(); // Refresh list
            }
        } catch (err: any) {
            console.error('Failed to deactivate user:', err);
            setError(err.message || 'Failed to deactivate user');
            notify.error(`Failed to deactivate user: ${err.message}`);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [isSuperAdminView, currentOrg, refetchCurrent]);

    const handleTransferUser = useCallback((user: Member) => {
        // TODO: Open transfer user modal
        console.log('Transfer user:', user);
        notify.error('User transfer not yet implemented.');
    }, []);

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
                            // Require Team license for more than allowed Community users (super admins exempt).
                            // Uses the server-reported org-wide total, not users.length - that's now just
                            // the current page's rows since listing is paginated server-side.
                            if (total >= COMMUNITY_TIER_LIMITS.MAX_USERS && !hasTeamLicense && !isSuperAdmin) {
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
                        Invite User
                    </Button>
                )}
            </div>

            {/* Free Plan Info Banner - Cloud only */}
            {isCloudMode() && !isSuperAdminView && isFreePlan && (
                <div className="bg-blue-900/20 border border-blue-500/30 rounded-lg p-4">
                    <div className="flex items-start">
                        <svg className="w-5 h-5 text-blue-400 mr-3 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                        </svg>
                        <div className="flex-1">
                            <h3 className="text-blue-200 font-semibold mb-1">Free Plan Limitations</h3>
                            <p className="text-blue-100/80 text-sm">
                                On the Free plan, only the organization creator can trigger reviews via the git-lrc CLI. Dashboard triggers require Premium.
                                Added members cannot trigger reviews. <a href="/#/settings-subscriptions-overview" className="text-blue-300 hover:text-blue-200 underline font-medium">Upgrade to Premium</a> to give all members full access.
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
                total={total}
                totalPages={totalPages}
                loading={loading}
                error={error}
                canManageUsers={canManageUsers}
                isSuperAdminView={isSuperAdminView}
                onDeactivateUser={handleDeactivateUser}
                onTransferUser={isSuperAdminView ? handleTransferUser : undefined}
                onRefresh={refetchCurrent}
                sorting={sorting}
                onSortingChange={handleSortingChange}
                columnFilters={columnFilters}
                onColumnFiltersChange={handleColumnFiltersChange}
                pagination={pagination}
                onPaginationChange={handlePaginationChange}
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