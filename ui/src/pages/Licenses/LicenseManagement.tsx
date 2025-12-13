import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import moment from 'moment-timezone';
import { useOrgContext } from '../../hooks/useOrgContext';
import apiClient from '../../api/apiClient';
import { CancelSubscriptionModal } from '../../components/Subscriptions';

type Subscription = {
    id: number;
    razorpay_subscription_id: string;
    plan_type: string;
    quantity: number;
    assigned_seats: number;
    status: string;
    current_period_start: string;
    current_period_end: string;
    license_expires_at: string;
    created_at: string;
    cancel_at_period_end: boolean;
    short_url?: string;
};

type AssignedUser = {
    user_id: number;
    email: string;
    assigned_at: string;
};

const LicenseManagement: React.FC = () => {
    const navigate = useNavigate();
    const { currentOrgId, currentOrg } = useOrgContext();
    const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Cancel Modal State
    const [showCancelModal, setShowCancelModal] = useState(false);
    const [cancelSubscriptionId, setCancelSubscriptionId] = useState<string | null>(null);
    const [cancelExpiryDate, setCancelExpiryDate] = useState<string | undefined>(undefined);
    const [navigating, setNavigating] = useState<string | null>(null);

    useEffect(() => {
        loadSubscriptions();
    }, []);

    const loadSubscriptions = async () => {
        try {
            setLoading(true);
            setError(null);

            console.log('Loading subscriptions for current user');

            // Fetch subscriptions owned by the current user
            const response = await apiClient.get<{ subscriptions: Subscription[] }>(
                `/subscriptions`
            );

            console.log('Subscriptions response:', response);
            setSubscriptions(response.subscriptions || []);
        } catch (err: any) {
            console.error('Failed to load subscriptions:', err);
            console.error('Error details:', {
                message: err.message,
                status: err.status,
                response: err.response,
                data: err.data
            });
            setError(err.response?.data?.error || err.message || 'Failed to load subscriptions');
        } finally {
            setLoading(false);
        }
    };

    const handleCancelClick = (sub: Subscription) => {
        setCancelSubscriptionId(sub.razorpay_subscription_id);
        setCancelExpiryDate(sub.current_period_end);
        setShowCancelModal(true);
    };

    const handleCancelSuccess = () => {
        loadSubscriptions();
        setShowCancelModal(false);
        setCancelSubscriptionId(null);
    };

    const handleNavigateToAssignment = (subscriptionId: string) => {
        setNavigating(subscriptionId);
        // Small delay to ensure the UI updates before navigation
        setTimeout(() => {
            navigate(`/subscribe/subscriptions/${subscriptionId}/assign`);
        }, 50);
    };

    const formatDate = (dateString: string) => {
        // Get user's timezone
        const userTimezone = moment.tz.guess();
        // Use moment-timezone to properly format with timezone abbreviation
        return moment.tz(dateString, userTimezone).format('MMM D, YYYY, h:mm A z');
    };

    const getPlanLabel = (planType: string) => {
        const labels: Record<string, string> = {
            team_monthly: 'Team Monthly',
            team_annual: 'Team Annual',
            team_yearly: 'Team Annual',
            monthly: 'Team Monthly',
            yearly: 'Team Annual',
        };
        return labels[planType] || planType;
    };

    const getStatusBadge = (status: string, sub: Subscription) => {
        const styles: Record<string, string> = {
            active: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/40',
            created: 'bg-blue-500/10 text-blue-400 border-blue-500/40',
            authenticated: 'bg-blue-500/10 text-blue-400 border-blue-500/40',
            pending: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/40',
            halted: 'bg-orange-500/10 text-orange-400 border-orange-500/40',
            cancelled: 'bg-slate-500/10 text-slate-400 border-slate-500/40',
            expired: 'bg-red-500/10 text-red-400 border-red-500/40',
        };

        if (status === 'active' && sub.cancel_at_period_end) {
            return (
                <span className="px-2 py-1 text-xs font-semibold rounded border bg-amber-500/10 text-amber-400 border-amber-500/40">
                    PENDING EXPIRY
                </span>
            );
        }

        return (
            <span className={`px-2 py-1 text-xs font-semibold rounded border ${styles[status] || styles.pending}`}>
                {status.toUpperCase()}
            </span>
        );
    };

    if (loading) {
        return (
            <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-8 px-4">
                <div className="max-w-6xl mx-auto">
                    <div className="flex items-center justify-center py-20">
                        <div className="inline-block w-8 h-8 border-4 border-slate-600 border-t-blue-500 rounded-full animate-spin" />
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="min-h-screen bg-gradient-to-b from-slate-900 to-slate-800 py-8 px-4">
            <div className="max-w-6xl mx-auto">
                {/* Header */}
                <div className="mb-8">
                    <div className="flex items-center justify-between mb-4">
                        <div>
                            <h1 className="text-3xl font-bold text-white mb-2">Subscription Management</h1>
                            <p className="text-slate-400">
                                Manage your team subscriptions and seat assignments
                                {currentOrg && ` for ${currentOrg.name}`}
                            </p>
                        </div>
                        <button
                            onClick={() => navigate('/subscribe')}
                            className="px-6 py-3 bg-blue-600 hover:bg-blue-700 text-white font-semibold rounded-lg transition-colors shadow-lg"
                        >
                            Purchase Licenses
                        </button>
                    </div>
                </div>

                {/* Error State */}
                {error && (
                    <div className="mb-6 p-4 bg-red-500/10 border border-red-500/40 rounded-lg">
                        <p className="text-red-300">{error}</p>
                    </div>
                )}

                {/* Empty State */}
                {!error && subscriptions.length === 0 && (
                    <div className="bg-slate-800 rounded-xl border border-slate-700 p-12 text-center">
                        <div className="flex items-center justify-center w-16 h-16 mx-auto rounded-full bg-slate-700/50 mb-4">
                            <svg className="w-8 h-8 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                            </svg>
                        </div>
                        <h3 className="text-xl font-semibold text-white mb-2">No Active Subscriptions</h3>
                        <p className="text-slate-400 mb-6">
                            Get started by purchasing a Team plan to unlock unlimited reviews and team collaboration features.
                        </p>
                        <button
                            onClick={() => navigate('/subscribe')}
                            className="px-6 py-3 bg-blue-600 hover:bg-blue-700 text-white font-semibold rounded-lg transition-colors inline-flex items-center gap-2"
                        >
                            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                            </svg>
                            Purchase Team Plan
                        </button>
                    </div>
                )}

                {/* Subscriptions List */}
                {subscriptions.length > 0 && (
                    <div className="space-y-6">
                        {subscriptions.map((sub) => (
                            <div key={sub.id} className="bg-slate-800 rounded-xl border border-slate-700 overflow-hidden">
                                {/* Subscription Header */}
                                <div className="p-6 border-b border-slate-700">
                                    <div className="flex items-start justify-between mb-4">
                                        <div>
                                            <div className="flex items-center gap-3 mb-2">
                                                <h2 className="text-xl font-semibold text-white">
                                                    {getPlanLabel(sub.plan_type)}
                                                </h2>
                                                {getStatusBadge(sub.status, sub)}
                                            </div>
                                            <p className="text-sm text-slate-400">
                                                Subscription ID: <span className="font-mono text-slate-300">{sub.razorpay_subscription_id}</span>
                                            </p>
                                        </div>
                                        <button
                                            onClick={() => handleNavigateToAssignment(sub.razorpay_subscription_id)}
                                            disabled={navigating === sub.razorpay_subscription_id}
                                            className="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg transition-colors text-sm font-medium disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2 min-w-[120px] justify-center"
                                        >
                                            {navigating === sub.razorpay_subscription_id ? (
                                                <>
                                                    <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
                                                    <span>Loading...</span>
                                                </>
                                            ) : (
                                                'Assign Seats'
                                            )}
                                        </button>
                                    </div>

                                    {/* Stats Grid */}
                                    <div className="grid grid-cols-1 sm:grid-cols-4 gap-4 mt-6">
                                        <div className="bg-slate-900/60 border border-slate-700 rounded-lg p-4">
                                            <div className="text-slate-400 text-xs mb-1">Total Seats</div>
                                            <div className="text-2xl font-bold text-white">{sub.quantity}</div>
                                        </div>
                                        <div className="bg-slate-900/60 border border-slate-700 rounded-lg p-4">
                                            <div className="text-slate-400 text-xs mb-1">Assigned</div>
                                            <div className="text-2xl font-bold text-emerald-400">{sub.assigned_seats}</div>
                                        </div>
                                        <div className="bg-slate-900/60 border border-slate-700 rounded-lg p-4">
                                            <div className="text-slate-400 text-xs mb-1">Available</div>
                                            <div className="text-2xl font-bold text-blue-400">{sub.quantity - sub.assigned_seats}</div>
                                        </div>
                                        <div className="bg-slate-900/60 border border-slate-700 rounded-lg p-4">
                                            <div className="text-slate-400 text-xs mb-1">
                                                {sub.status === 'cancelled' || sub.cancel_at_period_end ? 'Expires' : 'Renewal'}
                                            </div>
                                            <div className="text-sm font-semibold text-white">{formatDate(sub.current_period_end)}</div>
                                        </div>
                                    </div>
                                </div>

                                {/* Quick Actions Footer */}
                                <div className="p-6 bg-slate-900/40 flex justify-between items-center">
                                    <div className="text-sm text-slate-400">
                                        Created {formatDate(sub.created_at)} • {sub.status === 'cancelled' || sub.cancel_at_period_end ? 'Expires' : 'Renews'} {formatDate(sub.current_period_end)}
                                    </div>

                                    <div className="flex items-center gap-3">
                                        {/* Payment Method Link */}
                                        {sub.short_url && (
                                            <a
                                                href={sub.short_url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                className={`inline-flex items-center gap-2 px-3 py-1.5 text-xs font-medium rounded transition-colors ${sub.status === 'halted'
                                                        ? 'bg-orange-600 hover:bg-orange-700 text-white'
                                                        : 'text-slate-400 hover:text-slate-300'
                                                    }`}
                                            >
                                                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
                                                </svg>
                                                {sub.status === 'halted' ? 'Update Payment' : 'Manage Payment'}
                                            </a>
                                        )}

                                        {sub.status === 'active' && !sub.cancel_at_period_end && (
                                            <button
                                                onClick={() => handleCancelClick(sub)}
                                                className="text-xs text-red-400 hover:text-red-300 transition-colors font-medium"
                                            >
                                                Cancel Subscription
                                            </button>
                                        )}
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </div>

            {cancelSubscriptionId && (
                <CancelSubscriptionModal
                    isOpen={showCancelModal}
                    onClose={() => {
                        setShowCancelModal(false);
                        setCancelSubscriptionId(null);
                    }}
                    onSuccess={handleCancelSuccess}
                    subscriptionId={cancelSubscriptionId}
                    expiryDate={cancelExpiryDate}
                />
            )}
        </div>
    );
};

export default LicenseManagement;
