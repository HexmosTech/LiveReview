import React, { useState, useEffect, useMemo } from 'react';
import { Link } from 'react-router-dom';
import classNames from 'classnames';
import {
    ColumnDef,
    SortingState,
    ColumnFiltersState,
    PaginationState,
    RowSelectionState,
    OnChangeFn,
    useReactTable,
    getCoreRowModel,
} from '@tanstack/react-table';
import { LuSearch } from 'react-icons/lu';
import { Button, Popover, Input, Icons } from '../UIPrimitives';
import { ClientTable } from '../DataTable/ClientTable';
import { SortIcon, SortableHeaderLabel, HeaderFilterPopover } from '../DataTable/HeaderControls';
import { Member } from '../../api/users';
import { useOrgContext } from '../../hooks/useOrgContext';
import toast from 'react-hot-toast';

const USER_LIST_COLUMN_WIDTHS_WITH_CHECKBOX_AND_ORG = ['4%', '26%', '10%', '10%', '14%', '12%', '12%', '12%'];
const USER_LIST_COLUMN_WIDTHS_WITH_CHECKBOX = ['4%', '30%', '11%', '11%', '16%', '13%', '15%'];
const USER_LIST_COLUMN_WIDTHS_WITH_ORG = ['28%', '11%', '11%', '15%', '13%', '13%', '9%'];
const USER_LIST_COLUMN_WIDTHS_PLAIN = ['32%', '12%', '12%', '17%', '15%', '12%'];

const PAGE_SIZE_OPTIONS = [10, 25, 50];

const CHECKBOX_CLASS = 'rounded border-slate-600 bg-slate-700 text-blue-600 focus:ring-blue-500 focus:ring-offset-slate-800';

// Module-level (never-recreated) header/cell renderers for the selection
// column, reading selection state via TanStack's own row-selection API
// (table.getIsAllRowsSelected()/row.getIsSelected()) instead of closing over
// component state. A column's header/cell must keep the exact same function
// identity across re-renders of the `columns` memo - if it doesn't, React
// treats it as a different component type at that position and remounts it,
// silently resetting any local state inside (e.g. the "User" column's search
// popover open/closed state). See .agents/design.md.
const SelectAllHeaderCell: NonNullable<ColumnDef<Member>['header']> = ({ table }) => (
    <input
        type="checkbox"
        checked={table.getIsAllRowsSelected()}
        onChange={table.getToggleAllRowsSelectedHandler()}
        className={CHECKBOX_CLASS}
    />
);

const SelectRowCell: NonNullable<ColumnDef<Member>['cell']> = ({ row }) => (
    <input
        type="checkbox"
        checked={row.getIsSelected()}
        onChange={row.getToggleSelectedHandler()}
        className={CHECKBOX_CLASS}
    />
);

export interface UserListProps {
    /**
     * Users to display - just the current page's rows. Sorting, searching,
     * and pagination are all server-driven (see UserManagement.tsx), so this
     * is never the org's full member list.
     */
    users: Member[];

    /**
     * Org-wide total row count and page count, as reported by the API -
     * needed for the pagination footer since `users` only holds one page.
     */
    total: number;
    totalPages: number;

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

    /** Server-driven table state, owned by the parent (see UserManagement.tsx). */
    sorting: SortingState;
    onSortingChange: OnChangeFn<SortingState>;
    columnFilters: ColumnFiltersState;
    onColumnFiltersChange: OnChangeFn<ColumnFiltersState>;
    pagination: PaginationState;
    onPaginationChange: OnChangeFn<PaginationState>;
}

export const UserList: React.FC<UserListProps> = ({
    users,
    total,
    totalPages,
    loading = false,
    error,
    canManageUsers = false,
    isSuperAdminView = false,
    onDeactivateUser,
    onTransferUser,
    onRefresh,
    sorting,
    onSortingChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
}) => {
    const { currentOrg, isFreePlan } = useOrgContext();
    const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
    const selectedCount = Object.values(rowSelection).filter(Boolean).length;

    // Clear selection when the visible page changes
    useEffect(() => {
        setRowSelection({});
    }, [users]);

    const handleDownloadSelectedCredentials = () => {
        if (selectedCount === 0) {
            toast.error('Please select at least one user first');
            return;
        }

        const selectedMembers = users.filter(u => rowSelection[String(u.id)]);
        const installUrl = window.location.origin;

        const headers = ['Email', 'Name', 'Linux/Mac Command', 'Windows Command'];
        const rows = selectedMembers.map(user => {
            const name = `${user.first_name || ''} ${user.last_name || ''}`.trim() || user.email;
            const installCmdLinux = `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${user.onboarding_api_key || ''}" LRC_API_URL="${installUrl}" bash`;
            const installCmdWindows = `$env:LRC_API_KEY="${user.onboarding_api_key || ''}"; $env:LRC_API_URL="${installUrl}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`;
            return [user.email, name, installCmdLinux, installCmdWindows];
        });

        const escapeCsv = (val: string) => `"${val.replace(/"/g, '""')}"`;
        const csvContent = [
            headers.map(escapeCsv).join(','),
            ...rows.map(row => row.map(escapeCsv).join(','))
        ].join('\n');

        const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.setAttribute('href', url);
        link.setAttribute('download', 'git-lrc-setup.csv');
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        toast.success(`git-lrc-setup.csv downloaded successfully for ${selectedCount} user(s)!`);
    };

    const columns = useMemo<ColumnDef<Member>[]>(() => {
        const cols: ColumnDef<Member>[] = [];

        if (canManageUsers) {
            cols.push({
                id: 'select',
                enableSorting: false,
                enableColumnFilter: false,
                header: SelectAllHeaderCell,
                cell: SelectRowCell,
            });
        }

        cols.push({
            id: 'user',
            accessorFn: (user) => (user.first_name && user.last_name ? `${user.first_name} ${user.last_name}` : user.email),
            header: ({ column }) => (
                <div className="flex items-center justify-between gap-2">
                    <SortableHeaderLabel label="User" onToggle={column.getToggleSortingHandler()} />
                    <div className="flex items-center gap-2">
                        <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
                        <HeaderFilterPopover icon={LuSearch} label="Search users">
                            <Input
                                placeholder="Search by name or email..."
                                value={(column.getFilterValue() as string) ?? ''}
                                onChange={(e) => column.setFilterValue(e.target.value || undefined)}
                                icon={<Icons.Search />}
                                aria-label="Search users"
                                className="text-sm"
                            />
                        </HeaderFilterPopover>
                    </div>
                </div>
            ),
            cell: ({ row }) => {
                const user = row.original;
                return (
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
                );
            },
        });

        cols.push({
            id: 'status',
            accessorFn: (user) => (user.is_active ? 1 : 0),
            enableColumnFilter: false,
            header: ({ column }) => (
                <div className="flex items-center justify-between gap-2">
                    <SortableHeaderLabel label="Status" onToggle={column.getToggleSortingHandler()} />
                    <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
                </div>
            ),
            cell: ({ row }) => (
                <span className={classNames(
                    'inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium',
                    row.original.is_active
                        ? 'bg-green-900/20 text-green-300'
                        : 'bg-red-900/20 text-red-300'
                )}>
                    {row.original.is_active ? 'Active' : 'Inactive'}
                </span>
            ),
        });

        cols.push({
            id: 'role',
            accessorKey: 'role',
            enableColumnFilter: false,
            header: ({ column }) => (
                <div className="flex items-center justify-between gap-2">
                    <SortableHeaderLabel label="Role" onToggle={column.getToggleSortingHandler()} />
                    <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
                </div>
            ),
            cell: ({ row }) => (
                <span className="text-sm text-slate-400 capitalize">{row.original.role}</span>
            ),
        });

        cols.push({
            id: 'access',
            enableSorting: false,
            enableColumnFilter: false,
            header: () => (
                <div className="flex items-center justify-center gap-1">
                    git-lrc Access
                    {isFreePlan && (
                        <Popover
                            hover={true}
                            align="center"
                            trigger={
                                <svg className="w-4 h-4 text-slate-500 hover:text-slate-300 transition-colors cursor-help cursor-pointer" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                </svg>
                            }
                        >
                            <p>
                                On the Free plan, reviews can only be triggered via the git-lrc CLI by the org owner. Dashboard triggers require Premium.{' '}
                                <a href="/#/settings-subscriptions-overview" className="text-blue-300 hover:text-blue-200 underline">
                                    Upgrade to Premium
                                </a>
                            </p>
                        </Popover>
                    )}
                </div>
            ),
            cell: ({ row }) => {
                const user = row.original;
                const hasAccess = !isFreePlan || currentOrg?.created_by_user_id === user.id;
                return (
                    <div className="flex justify-center">
                        {hasAccess ? (
                            <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-label="Has Review Access">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 13l4 4L19 7" />
                            </svg>
                        ) : (
                            <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-label="No Review Access">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                            </svg>
                        )}
                    </div>
                );
            },
        });

        if (isSuperAdminView) {
            cols.push({
                id: 'organizations',
                enableSorting: false,
                enableColumnFilter: false,
                header: () => 'Organizations',
                cell: () => <span className="text-slate-500 text-sm">N/A</span>,
            });
        }

        cols.push({
            id: 'joined',
            accessorKey: 'created_at',
            enableColumnFilter: false,
            header: ({ column }) => (
                <div className="flex items-center justify-between gap-2">
                    <SortableHeaderLabel label="Joined" onToggle={column.getToggleSortingHandler()} />
                    <SortIcon sorted={column.getIsSorted()} onToggle={column.getToggleSortingHandler()} />
                </div>
            ),
            cell: ({ row }) => (
                <span className="text-sm text-slate-400">{new Date(row.original.created_at).toLocaleDateString()}</span>
            ),
        });

        if (canManageUsers) {
            cols.push({
                id: 'actions',
                enableSorting: false,
                enableColumnFilter: false,
                header: () => <div className="text-right">Actions</div>,
                cell: ({ row }) => {
                    const user = row.original;
                    return (
                        <div className="flex items-center justify-end space-x-2 text-sm font-medium">
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
                    );
                },
            });
        }

        return cols;
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [canManageUsers, isSuperAdminView, isFreePlan, currentOrg, onTransferUser, onDeactivateUser]);

    const columnWidths = canManageUsers && isSuperAdminView
        ? USER_LIST_COLUMN_WIDTHS_WITH_CHECKBOX_AND_ORG
        : canManageUsers
            ? USER_LIST_COLUMN_WIDTHS_WITH_CHECKBOX
            : isSuperAdminView
                ? USER_LIST_COLUMN_WIDTHS_WITH_ORG
                : USER_LIST_COLUMN_WIDTHS_PLAIN;

    // Server-side: TanStack just renders whatever page of already-
    // filtered/sorted rows the API returned, instead of computing
    // filtered/sorted/paginated row models itself from a client-side array
    // (same pattern as Explore > Repositories - see .agents/design.md).
    const table = useReactTable({
        data: users,
        columns,
        getRowId: (user) => String(user.id),
        getCoreRowModel: getCoreRowModel(),
        manualSorting: true,
        manualFiltering: true,
        manualPagination: true,
        pageCount: totalPages,
        state: { sorting, columnFilters, pagination, rowSelection },
        onSortingChange,
        onColumnFiltersChange,
        onPaginationChange,
        onRowSelectionChange: setRowSelection,
    });

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
                            {total} user{total !== 1 ? 's' : ''}
                        </span>
                    </div>

                    <div className="flex items-center space-x-2">
                        {canManageUsers && (
                            <Button
                                variant="secondary"
                                size="sm"
                                onClick={handleDownloadSelectedCredentials}
                                disabled={loading}
                                className="flex items-center"
                                title="Download the git-lrc installation details for the users"
                                icon={
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                                    </svg>
                                }
                            >
                                Download Onboarding Command
                            </Button>
                        )}
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
            </div>

            {/* Bulk Actions (if users selected and can manage) */}
            {selectedCount > 0 && canManageUsers && (
                <div className="px-6 py-3 bg-blue-900/20 border-b border-slate-700">
                    <div className="flex items-center justify-between">
                        <span className="text-sm text-blue-300">
                            {selectedCount} user{selectedCount !== 1 ? 's' : ''} selected
                        </span>
                        <div className="flex items-center space-x-2">
                            <Button
                                variant="ghost"
                                size="sm"
                                onClick={() => setRowSelection({})}
                                className="text-slate-400"
                            >
                                Clear
                            </Button>
                        </div>
                    </div>
                </div>
            )}

            {/* Table */}
            <ClientTable
                table={table}
                columnWidths={columnWidths}
                loading={loading}
                loadingLabel="Loading users..."
                error={error ?? null}
                onRetry={() => onRefresh?.()}
                // Don't hide the search box when a search causes zero results.
                isEmpty={total === 0 && columnFilters.length === 0}
                empty={{
                    title: 'No users found',
                    description: isSuperAdminView
                        ? 'No users exist in any organization.'
                        : `No users found in ${currentOrg?.name || 'this organization'}.`,
                }}
                pageSizeOptions={PAGE_SIZE_OPTIONS}
                manualTotal={total}
            />
        </div>
    );
};
