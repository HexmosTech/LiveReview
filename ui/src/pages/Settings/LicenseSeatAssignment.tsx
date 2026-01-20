import React, { useEffect, useState, useCallback } from 'react';
import toast from 'react-hot-toast';
import { useAppSelector, useAppDispatch } from '../../store/configureStore';
import { fetchLicenseStatus } from '../../store/License/slice';
import {
    SeatAssignment,
    UnassignedUser,
    getSeatAssignments,
    getUnassignedUsers,
    assignSeat,
    bulkAssignSeats,
    revokeSeat,
    bulkRevokeSeats,
} from '../../api/license';

const LicenseSeatAssignment: React.FC = () => {
    const dispatch = useAppDispatch();
    const license = useAppSelector(s => s.License);
    
    const [assignments, setAssignments] = useState<SeatAssignment[]>([]);
    const [unassignedUsers, setUnassignedUsers] = useState<UnassignedUser[]>([]);
    const [loading, setLoading] = useState(true);
    const [processing, setProcessing] = useState<number | null>(null);
    const [bulkProcessing, setBulkProcessing] = useState(false);
    const [selectedAssigned, setSelectedAssigned] = useState<Set<number>>(new Set());
    const [selectedUnassigned, setSelectedUnassigned] = useState<Set<number>>(new Set());
    const [error, setError] = useState<string | null>(null);

    // Initialize tab from URL hash
    const getInitialTab = (): 'assigned' | 'unassigned' => {
        const hash = window.location.hash;
        if (hash === '#license-assignments-unassigned') return 'unassigned';
        return 'assigned';
    };
    const [activeTab, setActiveTab] = useState<'assigned' | 'unassigned'>(getInitialTab);

    // Handle tab change with URL update
    const handleTabChange = (tab: 'assigned' | 'unassigned') => {
        setActiveTab(tab);
        const hash = tab === 'unassigned' ? '#license-assignments-unassigned' : '#license-assignments-assigned';
        window.history.replaceState(null, '', `/settings${hash}`);
    };

    // Sync tab with URL hash on hash change
    useEffect(() => {
        const handleHashChange = () => {
            const hash = window.location.hash;
            if (hash === '#license-assignments-unassigned') {
                setActiveTab('unassigned');
            } else if (hash === '#license-assignments-assigned' || hash === '#license-assignments') {
                setActiveTab('assigned');
            }
        };
        window.addEventListener('hashchange', handleHashChange);
        return () => window.removeEventListener('hashchange', handleHashChange);
    }, []);

    const loadData = useCallback(async () => {
        try {
            setLoading(true);
            setError(null);
            
            const [assignmentsRes, unassignedRes] = await Promise.all([
                getSeatAssignments(),
                getUnassignedUsers(),
            ]);
            
            setAssignments(assignmentsRes.assignments || []);
            setUnassignedUsers(unassignedRes.users || []);
            
            // Also refresh license status
            dispatch(fetchLicenseStatus());
        } catch (err: any) {
            setError(err.message || 'Failed to load data');
        } finally {
            setLoading(false);
        }
    }, [dispatch]);

    useEffect(() => {
        loadData();
    }, [loadData]);

    const handleAssign = async (userId: number) => {
        try {
            setProcessing(userId);
            await assignSeat(userId);
            toast.success('Seat assigned successfully');
            await loadData();
        } catch (err: any) {
            toast.error(err.message || 'Failed to assign seat');
        } finally {
            setProcessing(null);
        }
    };

    const handleRevoke = async (userId: number) => {
        if (!window.confirm('Are you sure you want to revoke this seat? The user will lose access.')) {
            return;
        }
        
        try {
            setProcessing(userId);
            await revokeSeat(userId);
            toast.success('Seat revoked successfully');
            await loadData();
        } catch (err: any) {
            toast.error(err.message || 'Failed to revoke seat');
        } finally {
            setProcessing(null);
        }
    };

    const handleBulkAssign = async () => {
        if (selectedUnassigned.size === 0) {
            toast.error('No users selected');
            return;
        }

        const userIds = Array.from(selectedUnassigned);
        
        // Check seat availability
        if (!license.unlimited && license.seatCount != null) {
            const available = license.seatCount - (license.assignedSeats || 0);
            if (userIds.length > available) {
                toast.error(`Only ${available} seat(s) available, but ${userIds.length} selected`);
                return;
            }
        }

        try {
            setBulkProcessing(true);
            const result = await bulkAssignSeats(userIds);
            toast.success(`${result.assigned} seat(s) assigned successfully`);
            setSelectedUnassigned(new Set());
            await loadData();
        } catch (err: any) {
            toast.error(err.message || 'Failed to assign seats');
        } finally {
            setBulkProcessing(false);
        }
    };

    const handleBulkRevoke = async () => {
        if (selectedAssigned.size === 0) {
            toast.error('No users selected');
            return;
        }

        if (!window.confirm(`Are you sure you want to revoke ${selectedAssigned.size} seat(s)?`)) {
            return;
        }

        try {
            setBulkProcessing(true);
            const result = await bulkRevokeSeats(Array.from(selectedAssigned));
            toast.success(`${result.revoked} seat(s) revoked successfully`);
            setSelectedAssigned(new Set());
            await loadData();
        } catch (err: any) {
            toast.error(err.message || 'Failed to revoke seats');
        } finally {
            setBulkProcessing(false);
        }
    };

    const toggleAssigned = (userId: number) => {
        setSelectedAssigned(prev => {
            const next = new Set(prev);
            if (next.has(userId)) {
                next.delete(userId);
            } else {
                next.add(userId);
            }
            return next;
        });
    };

    const toggleUnassigned = (userId: number) => {
        setSelectedUnassigned(prev => {
            const next = new Set(prev);
            if (next.has(userId)) {
                next.delete(userId);
            } else {
                next.add(userId);
            }
            return next;
        });
    };

    const toggleSelectAllAssigned = () => {
        if (selectedAssigned.size === assignments.length) {
            setSelectedAssigned(new Set());
        } else {
            setSelectedAssigned(new Set(assignments.map(a => a.user_id)));
        }
    };

    const toggleSelectAllUnassigned = () => {
        if (selectedUnassigned.size === unassignedUsers.length) {
            setSelectedUnassigned(new Set());
        } else {
            setSelectedUnassigned(new Set(unassignedUsers.map(u => u.id)));
        }
    };

    const totalSeats = license.seatCount || 0;
    const assignedSeats = license.assignedSeats || 0;
    const availableSeats = license.unlimited ? -1 : totalSeats - assignedSeats;

    if (loading) {
        return (
            <div className="flex items-center justify-center py-12">
                <div className="inline-block w-8 h-8 border-4 border-slate-600 border-t-blue-500 rounded-full animate-spin" />
            </div>
        );
    }

    return (
        <div className="space-y-6">
            {/* Seat Summary */}
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
                    <div className="text-slate-400 text-xs mb-1">Total Seats</div>
                    <div className="text-2xl font-bold text-white">
                        {license.unlimited ? '∞' : totalSeats}
                    </div>
                </div>
                <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
                    <div className="text-slate-400 text-xs mb-1">Assigned</div>
                    <div className="text-2xl font-bold text-emerald-400">{assignedSeats}</div>
                </div>
                <div className="bg-slate-800/60 border border-slate-700 rounded-lg p-4">
                    <div className="text-slate-400 text-xs mb-1">Available</div>
                    <div className="text-2xl font-bold text-blue-400">
                        {license.unlimited ? '∞' : availableSeats}
                    </div>
                </div>
            </div>

            {error && (
                <div className="p-4 bg-red-500/10 border border-red-500/40 rounded-lg">
                    <p className="text-red-300">{error}</p>
                </div>
            )}

            {/* Tab Navigation */}
            <div className="border-b border-slate-700">
                <div className="flex space-x-1">
                    <button
                        onClick={() => handleTabChange('assigned')}
                        className={`px-4 py-3 text-sm font-medium transition-colors ${
                            activeTab === 'assigned'
                                ? 'text-white border-b-2 border-blue-500'
                                : 'text-slate-400 hover:text-slate-300'
                        }`}
                    >
                        Assigned ({assignments.length})
                    </button>
                    <button
                        onClick={() => handleTabChange('unassigned')}
                        className={`px-4 py-3 text-sm font-medium transition-colors ${
                            activeTab === 'unassigned'
                                ? 'text-white border-b-2 border-blue-500'
                                : 'text-slate-400 hover:text-slate-300'
                        }`}
                    >
                        Unassigned ({unassignedUsers.length})
                    </button>
                </div>
            </div>

            {/* Content */}
            {activeTab === 'assigned' ? (
                <div className="bg-slate-800/60 border border-slate-700 rounded-lg overflow-hidden">
                    {/* Bulk Actions Header */}
                    {selectedAssigned.size > 0 && (
                        <div className="p-4 bg-slate-900/60 border-b border-slate-700 flex items-center justify-between">
                            <span className="text-sm text-slate-300">
                                {selectedAssigned.size} selected
                            </span>
                            <button
                                onClick={handleBulkRevoke}
                                disabled={bulkProcessing}
                                className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50"
                            >
                                {bulkProcessing ? 'Processing...' : 'Revoke Selected'}
                            </button>
                        </div>
                    )}

                    {assignments.length === 0 ? (
                        <div className="p-12 text-center">
                            <p className="text-slate-400">No seats assigned yet</p>
                            <p className="text-slate-500 text-sm mt-2">
                                Go to the "Unassigned" tab to assign seats to users
                            </p>
                        </div>
                    ) : (
                        <>
                            {/* Select All */}
                            <div className="p-4 border-b border-slate-700 bg-slate-900/40 flex items-center">
                                <input
                                    type="checkbox"
                                    checked={selectedAssigned.size === assignments.length && assignments.length > 0}
                                    onChange={toggleSelectAllAssigned}
                                    className="w-4 h-4 text-blue-600 bg-slate-700 border-slate-600 rounded focus:ring-blue-500"
                                />
                                <span className="ml-4 text-sm font-medium text-slate-300">
                                    Select All ({assignments.length})
                                </span>
                            </div>

                            {/* Assignment List */}
                            <div className="divide-y divide-slate-700">
                                {assignments.map((assignment) => (
                                    <div key={assignment.id} className="p-4 hover:bg-slate-900/40 transition-colors flex items-center gap-4">
                                        <input
                                            type="checkbox"
                                            checked={selectedAssigned.has(assignment.user_id)}
                                            onChange={() => toggleAssigned(assignment.user_id)}
                                            disabled={bulkProcessing || processing === assignment.user_id}
                                            className="w-4 h-4 text-blue-600 bg-slate-700 border-slate-600 rounded focus:ring-blue-500"
                                        />
                                        <div className="flex-1 min-w-0">
                                            <div className="flex items-center gap-3 mb-1">
                                                <h3 className="text-white font-medium truncate">{assignment.email}</h3>
                                                <span className="px-2 py-1 text-xs font-semibold rounded border bg-emerald-500/10 text-emerald-400 border-emerald-500/40">
                                                    Licensed
                                                </span>
                                            </div>
                                            {(assignment.first_name || assignment.last_name) && (
                                                <p className="text-sm text-slate-400">
                                                    {assignment.first_name} {assignment.last_name}
                                                </p>
                                            )}
                                            <p className="text-xs text-slate-500 mt-1">
                                                Assigned {new Date(assignment.assigned_at).toLocaleDateString()}
                                                {assignment.assigned_by_email && ` by ${assignment.assigned_by_email}`}
                                            </p>
                                        </div>
                                        <button
                                            onClick={() => handleRevoke(assignment.user_id)}
                                            disabled={processing === assignment.user_id || bulkProcessing}
                                            className="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 min-w-[100px]"
                                        >
                                            {processing === assignment.user_id ? 'Revoking...' : 'Revoke'}
                                        </button>
                                    </div>
                                ))}
                            </div>
                        </>
                    )}
                </div>
            ) : (
                <div className="bg-slate-800/60 border border-slate-700 rounded-lg overflow-hidden">
                    {/* Bulk Actions Header */}
                    {selectedUnassigned.size > 0 && (
                        <div className="p-4 bg-slate-900/60 border-b border-slate-700 flex items-center justify-between">
                            <span className="text-sm text-slate-300">
                                {selectedUnassigned.size} selected
                            </span>
                            <button
                                onClick={handleBulkAssign}
                                disabled={bulkProcessing || (!license.unlimited && selectedUnassigned.size > availableSeats)}
                                className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50"
                            >
                                {bulkProcessing ? 'Processing...' : 'Assign Selected'}
                            </button>
                        </div>
                    )}

                    {unassignedUsers.length === 0 ? (
                        <div className="p-12 text-center">
                            <p className="text-slate-400">All active users have seats assigned</p>
                        </div>
                    ) : (
                        <>
                            {/* Select All */}
                            <div className="p-4 border-b border-slate-700 bg-slate-900/40 flex items-center">
                                <input
                                    type="checkbox"
                                    checked={selectedUnassigned.size === unassignedUsers.length && unassignedUsers.length > 0}
                                    onChange={toggleSelectAllUnassigned}
                                    className="w-4 h-4 text-blue-600 bg-slate-700 border-slate-600 rounded focus:ring-blue-500"
                                />
                                <span className="ml-4 text-sm font-medium text-slate-300">
                                    Select All ({unassignedUsers.length})
                                </span>
                            </div>

                            {/* Unassigned Users List */}
                            <div className="divide-y divide-slate-700">
                                {unassignedUsers.map((user) => (
                                    <div key={user.id} className="p-4 hover:bg-slate-900/40 transition-colors flex items-center gap-4">
                                        <input
                                            type="checkbox"
                                            checked={selectedUnassigned.has(user.id)}
                                            onChange={() => toggleUnassigned(user.id)}
                                            disabled={bulkProcessing || processing === user.id}
                                            className="w-4 h-4 text-blue-600 bg-slate-700 border-slate-600 rounded focus:ring-blue-500"
                                        />
                                        <div className="flex-1 min-w-0">
                                            <div className="flex items-center gap-3 mb-1">
                                                <h3 className="text-white font-medium truncate">{user.email}</h3>
                                                {user.role && (
                                                    <span className="px-2 py-1 text-xs font-semibold rounded border bg-slate-700/40 text-slate-300 border-slate-600">
                                                        {user.role}
                                                    </span>
                                                )}
                                                <span className="px-2 py-1 text-xs font-semibold rounded border bg-slate-700/40 text-slate-400 border-slate-600">
                                                    No Seat
                                                </span>
                                            </div>
                                            {(user.first_name || user.last_name) && (
                                                <p className="text-sm text-slate-400">
                                                    {user.first_name} {user.last_name}
                                                </p>
                                            )}
                                        </div>
                                        <button
                                            onClick={() => handleAssign(user.id)}
                                            disabled={processing === user.id || bulkProcessing || (!license.unlimited && availableSeats <= 0)}
                                            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 min-w-[100px]"
                                        >
                                            {processing === user.id ? 'Assigning...' : 'Assign'}
                                        </button>
                                    </div>
                                ))}
                            </div>
                        </>
                    )}
                </div>
            )}

            {/* Warning for no available seats */}
            {!license.unlimited && availableSeats <= 0 && unassignedUsers.length > 0 && (
                <div className="p-4 bg-orange-500/10 border border-orange-500/40 rounded-lg">
                    <p className="text-orange-300 text-sm">
                        All seats are assigned. Revoke existing seats or upgrade your license to assign more users.
                    </p>
                </div>
            )}
        </div>
    );
};

export default LicenseSeatAssignment;
