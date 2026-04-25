import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import classNames from 'classnames';
import { getDashboardData, DashboardData, refreshDashboardData } from '../../api/dashboard';
import {
    StatCard,
    Section,
    PageHeader,
    Card,
    EmptyState,
    Button,
    Icons,
    Tooltip,
    Alert,
} from '../UIPrimitives';
import { HumanizedTimestamp } from '../HumanizedTimestamp/HumanizedTimestamp';
import RecentActivity from './RecentActivity';
import { OnboardingStepper } from './OnboardingStepper';
import { PlanBadge } from './PlanBadge';
import { QuotaExhaustedBanner } from './QuotaExhaustedBanner';
import { QuotaWarningBanner } from './QuotaWarningBanner';
import { handleUserLoginNotification } from '../../utils/userNotifications';
import { getApiUrl } from '../../utils/apiUrl';
import { useAppSelector } from '../../store/configureStore';
import { isCloudMode } from '../../utils/deploymentMode';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseUpgradeDialog from '../License/LicenseUpgradeDialog';
import apiClient from '../../api/apiClient';

type DashboardBillingStatusResponse = {
    billing: {
        current_plan_code: string;
        loc_used_month: number;
        trial_active?: boolean;
        trial_ends_at?: string | null;
        trial_eligibility?: {
            status?: 'eligible' | 'already_used' | 'reserved' | 'unknown';
            eligible?: boolean;
            reason?: string;
            consumed_at?: string | null;
        };
    };
    available_plans: Array<{
        plan_code: string;
        monthly_loc_limit: number;
        trial_days?: number;
    }>;
};

type DashboardQuotaStatusResponse = {
    envelope?: {
        usage_pct?: number;
        blocked?: boolean;
        trial_readonly?: boolean;
    };
};

type DashboardUpgradeStatusResponse = {
    request: {
        customer_state?: string;
        support_reference?: string;
        action_required?: {
            type?: string;
        };
    } | null;
};

type DashboardBillingInsight = {
    planCode: string;
    locUsed: number;
    locLimit: number;
    usagePct: number;
    blocked: boolean;
    trialReadonly: boolean;
    trialActive: boolean;
    trialEndsAt: string;
    trialEligibleForFirstPaidPurchase: boolean;
    trialEligibilityStatus: string;
    trialPolicyDays: number;
    customerState: string;
    supportReference: string;
    actionRequiredType: string;
};

const dashboardPlanLabel = (planCode: string): string => {
    const normalized = String(planCode || '').trim().toLowerCase();
    if (normalized === 'free_30k' || normalized === 'free') return 'Free 30k';
    if (normalized === 'team_32usd' || normalized === 'team') return 'Team 100k';
    if (normalized === 'loc_200k') return 'Team 200k';
    if (normalized === 'loc_400k') return 'Team 400k';
    if (normalized === 'loc_800k') return 'Team 800k';
    if (normalized === 'loc_1600k') return 'Team 1.6M';
    if (normalized === 'loc_3200k') return 'Team 3.2M';
    return planCode || 'Plan';
};

const getConnectorWarningDismissStorageKey = (userId?: number | string): string => {
    return userId ? `lr_hidden_connector_warnings_${userId}` : 'lr_hidden_connector_warnings';
};

const parseDismissedConnectorIds = (rawValue: string | null): Set<number> => {
    if (!rawValue) return new Set<number>();

    try {
        const parsed = JSON.parse(rawValue);
        if (!Array.isArray(parsed)) return new Set<number>();

        const validIds = parsed
            .map((item) => Number(item))
            .filter((item) => Number.isInteger(item) && item > 0);

        return new Set<number>(validIds);
    } catch {
        return new Set<number>();
    }
};

const loadDismissedConnectorIds = (storageKey: string): Set<number> => {
    try {
        return parseDismissedConnectorIds(localStorage.getItem(storageKey));
    } catch {
        return new Set<number>();
    }
};

const saveDismissedConnectorIds = (storageKey: string, connectorIds: Set<number>): void => {
    try {
        const sortedIds = Array.from(connectorIds).sort((a, b) => a - b);
        localStorage.setItem(storageKey, JSON.stringify(sortedIds));
    } catch {
        // no-op: keep UI functional when localStorage is unavailable
    }
};

export const Dashboard: React.FC = () => {
    const navigate = useNavigate();
    const user = useAppSelector(state => state.Auth.user);
    const { isFreePlan } = useOrgContext();

    // Dashboard data state
    const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [isSyncing, setIsSyncing] = useState(false);
    const [hideStepper, setHideStepper] = useState<boolean>(() => {
        // Scope localStorage to user ID so each user has their own onboarding state
        try {
            const key = user?.id ? `lr_hide_get_started_${user.id}` : 'lr_hide_get_started';
            return localStorage.getItem(key) === '1';
        } catch {
            return false;
        }
    });
    const [notificationSent, setNotificationSent] = useState(false);
    // Track dismissed connector progress notifications for this tab session
    const [dismissedConnectors, setDismissedConnectors] = useState<Set<number>>(new Set());
    const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);
    // Track connector warnings explicitly hidden by the user across page reloads
    const [persistedDismissedConnectors, setPersistedDismissedConnectors] = useState<Set<number>>(new Set());
    const [billingInsight, setBillingInsight] = useState<DashboardBillingInsight | null>(null);

    useEffect(() => {
        const storageKey = getConnectorWarningDismissStorageKey(user?.id);
        setPersistedDismissedConnectors(loadDismissedConnectorIds(storageKey));
    }, [user?.id]);

    const dismissConnectorForSession = (connectorId: number): void => {
        setDismissedConnectors((prev) => {
            if (prev.has(connectorId)) return prev;

            const updated = new Set(prev);
            updated.add(connectorId);
            return updated;
        });
    };

    const dismissConnectorPermanently = (connectorId: number): void => {
        dismissConnectorForSession(connectorId);

        setPersistedDismissedConnectors((prev) => {
            if (prev.has(connectorId)) return prev;

            const updated = new Set(prev);
            updated.add(connectorId);
            saveDismissedConnectorIds(getConnectorWarningDismissStorageKey(user?.id), updated);
            return updated;
        });
    };

    // Handle user notification on first dashboard load
    useEffect(() => {
        if (!notificationSent && user?.email && user?.created_at) {
            handleUserLoginNotification(
                user.email,
                '',  // first_name not available in UserInfo
                '',  // last_name not available in UserInfo
                user.created_at
            ).catch(err => {
                console.warn('[Dashboard] User notification failed:', err);
            });
            setNotificationSent(true);
        }
    }, [user, notificationSent]);

    // Load dashboard data
    useEffect(() => {
        const loadDashboardData = async () => {
            try {
                setIsLoading(true);
                setIsSyncing(true);
                // Ensure backend cache reflects latest connectors/changes
                try { await refreshDashboardData(); } catch { /* best-effort */ }
                const data = await getDashboardData();
                setDashboardData(data);
                setError(null);
            } catch (err) {
                console.error('Error loading dashboard data:', err);
                setError('Failed to load dashboard data');
            } finally {
                setIsLoading(false);
                setIsSyncing(false);
            }
        };

        loadDashboardData();

        // Refresh data every 5 minutes
        const interval = setInterval(loadDashboardData, 5 * 60 * 1000);

        // Also refresh when the tab regains focus or becomes visible (handy after New Review)
        const onFocus = () => { loadDashboardData(); };
        const onVisibility = () => { if (document.visibilityState === 'visible') loadDashboardData(); };
        window.addEventListener('focus', onFocus);
        document.addEventListener('visibilitychange', onVisibility);

        return () => {
            clearInterval(interval);
            window.removeEventListener('focus', onFocus);
            document.removeEventListener('visibilitychange', onVisibility);
        };
    }, []);

    useEffect(() => {
        if (!isCloudMode()) {
            setBillingInsight(null);
            return;
        }

        let cancelled = false;
        const loadBillingInsight = async () => {
            try {
                const [billing, quota, upgrade] = await Promise.all([
                    apiClient.get<DashboardBillingStatusResponse>('/billing/status'),
                    apiClient.get<DashboardQuotaStatusResponse>('/quota/status').catch((): null => null),
                    apiClient.get<DashboardUpgradeStatusResponse>('/billing/upgrade/request-status').catch((): null => null),
                ]);

                if (cancelled || !billing?.billing) return;

                const planCode = String(billing.billing.current_plan_code || 'free_30k').trim();
                const plan = (billing.available_plans || []).find((item) => item.plan_code === planCode);
                const locUsed = Number(billing.billing.loc_used_month || 0);
                const locLimit = Number(plan?.monthly_loc_limit || 0);
                const fallbackPct = locLimit > 0 ? Math.min(100, Math.round((locUsed * 100) / locLimit)) : 0;
                const trialPolicyDays = (billing.available_plans || []).reduce((max, item) => {
                    const days = Number(item.trial_days || 0);
                    if (days <= 0) {
                        return max;
                    }
                    return Math.max(max, days);
                }, 0);
                const trialEligibilityStatus = String(billing.billing.trial_eligibility?.status || 'unknown').trim().toLowerCase();

                setBillingInsight({
                    planCode,
                    locUsed,
                    locLimit,
                    usagePct: Math.max(0, Math.round(quota?.envelope?.usage_pct ?? fallbackPct)),
                    blocked: Boolean(quota?.envelope?.blocked),
                    trialReadonly: Boolean(quota?.envelope?.trial_readonly),
                    trialActive: Boolean(billing.billing.trial_active),
                    trialEndsAt: String(billing.billing.trial_ends_at || '').trim(),
                    trialEligibleForFirstPaidPurchase: Boolean(billing.billing.trial_eligibility?.eligible),
                    trialEligibilityStatus,
                    trialPolicyDays: trialPolicyDays > 0 ? trialPolicyDays : 7,
                    customerState: String(upgrade?.request?.customer_state || 'none').trim().toLowerCase(),
                    supportReference: String(upgrade?.request?.support_reference || '').trim(),
                    actionRequiredType: String(upgrade?.request?.action_required?.type || '').trim().toLowerCase(),
                });
            } catch {
                if (!cancelled) setBillingInsight(null);
            }
        };

        loadBillingInsight();
        const intervalId = window.setInterval(loadBillingInsight, 60000);

        return () => {
            cancelled = true;
            clearInterval(intervalId);
        };
    }, []);

    // Use dashboard API data exclusively - no fallbacks to Redux store
    const aiComments = dashboardData?.total_comments || 0;
    const codeReviews = dashboardData?.total_reviews || 0;
    const connectedProviders = dashboardData?.connected_providers || 0;
    const aiConnectors = dashboardData?.active_ai_connectors || 0;

    // Derive onboarding state
    const hasCLI = dashboardData?.cli_installed || false;
    const hasAIProvider = aiConnectors > 0;
    const allSet = hasCLI && hasAIProvider;
    const hasRunReview = codeReviews > 0;

    // Build install commands with API key
    const apiKey = dashboardData?.onboarding_api_key || '';
    // Get API URL - use the shared utility that correctly handles the UI/API port difference
    const apiUrl = getApiUrl();
    const installCommand = apiKey
        ? `curl -fsSL https://hexmos.com/lrc-install.sh | LRC_API_KEY="${apiKey}" LRC_API_URL="${apiUrl}" bash`
        : '';
    const installCommandWindows = apiKey
        ? `$env:LRC_API_KEY="${apiKey}"; $env:LRC_API_URL="${apiUrl}"; iwr -useb https://hexmos.com/lrc-install.ps1 | iex`
        : '';

    // Auto-hide feature disabled - users can manually dismiss the onboarding stepper if they want
    // by clicking "Don't show again" button

    // Check if this is an empty state (no connections and no activity)
    const isEmpty = connectedProviders === 0 && codeReviews === 0 && aiComments === 0;

    // Get connectors that need setup attention (filter out dismissed ones)
    const connectorsNeedingSetup = (dashboardData?.connector_setup_progress || []).filter(
        c => !dismissedConnectors.has(c.connector_id) && !persistedDismissedConnectors.has(c.connector_id)
    );

    // Helper to get phase variant for Alert
    const getPhaseVariant = (phase: string): 'info' | 'warning' | 'success' | 'error' => {
        switch (phase) {
            case 'discovering': return 'info';
            case 'installing': return 'warning';
            case 'ready': return 'success';
            case 'error': return 'error';
            default: return 'info';
        }
    };

    // Helper to get phase message
    const getPhaseMessage = (phase: string, connectorName: string, provider: string, totalProjects: number, connectedProjects: number): string => {
        switch (phase) {
            case 'discovering':
                return `${connectorName} (${provider}): Discovering projects...`;
            case 'installing':
                const percent = totalProjects > 0 ? Math.round((connectedProjects / totalProjects) * 100) : 0;
                return `${connectorName} (${provider}): Installing webhooks ${connectedProjects}/${totalProjects} (${percent}%)`;
            case 'ready':
                return `${connectorName} (${provider}): Ready - ${totalProjects} projects connected`;
            case 'error':
                return `${connectorName} (${provider}): Setup failed - click to retry`;
            default:
                return `${connectorName} (${provider})`;
        }
    };

    const handleNewReviewClick = () => {
        if (isFreePlan) {
            setShowUpgradeDialog(true);
        } else {
            navigate('/reviews/new');
        }
    };

    const dashboardOnFreePlan = String(billingInsight?.planCode || '').trim().toLowerCase() === 'free_30k' || String(billingInsight?.planCode || '').trim().toLowerCase() === 'free';

    return (
        <div className="min-h-screen">
            <main className="container mx-auto px-4 py-6">
                {/* Connector Setup Progress - using standard Alert component */}
                {connectorsNeedingSetup.length > 0 && (
                    <div className="mb-6 space-y-3">
                        {connectorsNeedingSetup.map((connector) => (
                            <Alert
                                key={connector.connector_id}
                                variant={getPhaseVariant(connector.phase)}
                                onClose={() => dismissConnectorForSession(connector.connector_id)}
                                className="cursor-pointer hover:opacity-90"
                            >
                                <div
                                    className="flex w-full flex-col gap-3 sm:flex-row sm:items-center sm:justify-between"
                                    onClick={() => navigate(`/git/connector/${connector.connector_id}`)}
                                >
                                    <span className="sm:pr-4">
                                        {getPhaseMessage(
                                            connector.phase,
                                            connector.connector_name,
                                            connector.provider,
                                            connector.total_projects,
                                            connector.connected_projects
                                        )}
                                    </span>
                                    <div className="flex flex-wrap items-center gap-2 sm:ml-4 sm:flex-nowrap">
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            className="text-current hover:bg-black/10 hover:opacity-90"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                dismissConnectorPermanently(connector.connector_id);
                                            }}
                                        >
                                            Don&apos;t show again
                                        </Button>
                                        <Button
                                            variant="outline"
                                            size="sm"
                                            className="!border-current !text-current hover:opacity-80"
                                            onClick={(e) => {
                                                e.stopPropagation();
                                                navigate(`/git/connector/${connector.connector_id}`);
                                            }}
                                        >
                                            View Details
                                        </Button>
                                    </div>
                                </div>
                            </Alert>
                        ))}
                    </div>
                )}

                {/* LOC Quota Warning/Blocked Banners */}
                {billingInsight && billingInsight.blocked && (
                    <Alert variant="error" className="mb-4">
                        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2">
                            <div>
                                <span className="font-semibold text-red-100">⛔ Monthly LOC Quota Exceeded</span>
                                <span className="text-sm text-red-200 ml-2">
                                    ({billingInsight.locUsed.toLocaleString()} / {billingInsight.locLimit > 0 ? billingInsight.locLimit.toLocaleString() : 'N/A'} LOC). Reviews are blocked until quota resets or you upgrade.
                                </span>
                            </div>
                            <Button
                                variant="primary"
                                size="sm"
                                onClick={() => navigate('/subscribe')}
                                className="!bg-red-600 hover:!bg-red-500 flex-shrink-0"
                            >
                                Upgrade Now
                            </Button>
                        </div>
                    </Alert>
                )}
                {billingInsight && !billingInsight.blocked && billingInsight.usagePct >= 100 && (
                    <QuotaExhaustedBanner
                        locUsed={billingInsight.locUsed}
                        locLimit={billingInsight.locLimit}
                        usagePct={billingInsight.usagePct}
                        onUpgrade={() => navigate('/settings-subscriptions-overview')}
                    />
                )}
                {billingInsight && !billingInsight.blocked && billingInsight.usagePct >= 90 && billingInsight.usagePct < 100 && (
                    <QuotaWarningBanner
                        locUsed={billingInsight.locUsed}
                        locLimit={billingInsight.locLimit}
                        usagePct={billingInsight.usagePct}
                        onUpgrade={() => navigate('/settings-subscriptions-overview')}
                    />
                )}

                {/* Header with aligned content and prominent CTA */}
                <div className="flex flex-col gap-3 sm:flex-row sm:justify-between sm:items-center mb-6">
                    <div className="mb-4 sm:mb-0">
                        <div className="flex items-center gap-2">
                            <h1 className="text-2xl font-bold text-white">Dashboard</h1>
                        </div>
                        <p className="mt-1 text-base text-slate-300">
                            Monitor your code review activity and connected services
                            {dashboardData && (
                                <span className="text-xs text-slate-400 ml-2">
                                    Last updated: <HumanizedTimestamp
                                        timestamp={dashboardData.last_updated}
                                        className="text-slate-400"
                                    />
                                </span>
                            )}
                        </p>
                    </div>
                    <div className="hidden sm:flex gap-3">
                        <Button
                            variant="primary"
                            icon={<Icons.Add />}
                            onClick={handleNewReviewClick}
                            className="shadow-xl transition-all duration-300 hover:shadow-2xl hover:scale-105 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600"
                            title={isFreePlan ? "Upgrade your plan to create reviews" : "Safe review - no comments posted"}
                        >
                            New Review
                        </Button>
                    </div>
                </div>

                {billingInsight && (
                    <div className="mb-6 bg-slate-800/55 border border-slate-700 rounded-xl p-4">
                        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4">
                            <div className="space-y-2">
                                <p className="text-sm text-slate-300">
                                    Billing status: <span className="text-white font-semibold">{dashboardPlanLabel(billingInsight.planCode)}</span>
                                    {' • '}
                                    Usage <span className="text-white font-semibold">{billingInsight.usagePct}%</span>
                                </p>
                                <div className="w-full lg:w-80 h-2 rounded-full bg-slate-700 overflow-hidden">
                                    <div
                                        className={classNames(
                                            'h-full transition-all',
                                            billingInsight.blocked || billingInsight.customerState === 'action_needed' || billingInsight.customerState === 'payment_failed'
                                                ? 'bg-red-500'
                                                : billingInsight.usagePct >= 90
                                                    ? 'bg-amber-500'
                                                    : billingInsight.usagePct >= 80
                                                        ? 'bg-amber-500'
                                                        : 'bg-emerald-500'
                                        )}
                                        style={{ width: `${Math.max(0, Math.min(100, billingInsight.usagePct))}%` }}
                                    />
                                </div>
                                <p className="text-xs text-slate-400">
                                    {billingInsight.locUsed.toLocaleString()} / {billingInsight.locLimit > 0 ? billingInsight.locLimit.toLocaleString() : 'Unlimited'} LOC this period
                                </p>
                                {billingInsight.trialActive && (
                                    <p className="text-xs text-sky-200">
                                        Trial active: ends {billingInsight.trialEndsAt ? new Date(billingInsight.trialEndsAt).toLocaleString() : 'when synchronization completes'}.
                                    </p>
                                )}
                                {!billingInsight.trialActive && dashboardOnFreePlan && (
                                    <p className={`text-xs ${billingInsight.trialEligibleForFirstPaidPurchase
                                        ? 'text-sky-200'
                                        : billingInsight.trialEligibilityStatus === 'already_used'
                                            ? 'text-slate-300'
                                            : 'text-amber-200'
                                        }`}>
                                        {billingInsight.trialEligibleForFirstPaidPurchase
                                            ? `First paid purchase includes ${billingInsight.trialPolicyDays}-day trial on any paid LOC plan.`
                                            : billingInsight.trialEligibilityStatus === 'already_used'
                                                ? 'First paid trial already used for this email. New paid purchases bill immediately.'
                                                : 'Trial eligibility is being synchronized and will be confirmed at checkout.'}
                                    </p>
                                )}
                                {(billingInsight.customerState === 'action_needed' || billingInsight.customerState === 'payment_failed' || billingInsight.blocked || billingInsight.trialReadonly) && (
                                    <p className="text-xs text-amber-200">
                                        Action needed: {billingInsight.actionRequiredType || billingInsight.customerState.replace(/_/g, ' ')}
                                        {billingInsight.supportReference ? ` • Ref ${billingInsight.supportReference}` : ''}
                                    </p>
                                )}
                            </div>
                            <div className="flex items-center gap-2">
                                <Button
                                    variant="outline"
                                    onClick={() => navigate('/settings-subscriptions-overview')}
                                    className="text-slate-200 border-slate-500"
                                >
                                    Open Billing Details
                                </Button>
                            </div>
                        </div>
                    </div>
                )}

                {/* Error state */}
                {error && (
                    <div className="mb-6 bg-red-900/40 rounded-xl p-4 border border-red-800/30">
                        <div className="flex items-center">
                            <Icons.Info />
                            <div className="ml-3">
                                <h3 className="text-lg font-medium text-red-300">Error Loading Dashboard</h3>
                                <p className="mt-1 text-red-200">{error}</p>
                            </div>
                        </div>
                    </div>
                )}

                {/* Loading state */}
                {isLoading && !dashboardData && (
                    <div className="mb-6 bg-slate-800/40 rounded-xl p-6 border border-slate-700/30">
                        <div className="flex items-center justify-center">
                            <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-400"></div>
                            <span className="ml-3 text-slate-300">Loading dashboard data...</span>
                        </div>
                    </div>
                )}

                {/* Floating Action Button for mobile */}
                <Button
                    variant="primary"
                    icon={<Icons.Add />}
                    onClick={handleNewReviewClick}
                    className="fixed bottom-6 right-6 sm:hidden z-40 rounded-full w-14 h-14 shadow-xl transition-all duration-300 hover:shadow-2xl hover:scale-110 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600"
                    aria-label={isFreePlan ? "Review creation locked" : "New Review (Safe - no comments posted)"}
                    title={isFreePlan ? "Upgrade your plan to create reviews" : "Safe review - no comments posted"}
                />

                {/* Get Started stepper – stays visible until the user manually dismisses it */}
                {!hideStepper && (
                    <OnboardingStepper
                        hasCLI={hasCLI}
                        hasAIProvider={hasAIProvider}
                        hasRunReview={hasRunReview}
                        installCommand={installCommand}
                        installCommandWindows={installCommandWindows}
                        onConfigureAI={() => navigate('/ai')}
                        onNewReview={() => navigate('/reviews/new')}
                        userId={user?.id}
                        isFreePlan={isFreePlan}
                        onUpgrade={() => setShowUpgradeDialog(true)}
                        onDismiss={() => {
                            setHideStepper(true);
                            try {
                                const key = user?.id ? `lr_hide_get_started_${user.id}` : 'lr_hide_get_started';
                                localStorage.setItem(key, '1');
                            } catch { }
                        }}
                        className="mb-6"
                    />
                )}

                {/* Main Statistics Grid - Improved density and alignment */}
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
                    <StatCard
                        variant="primary"
                        title="AI Reviews"
                        value={codeReviews}
                        icon={
                            <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path d="M14.72,8.79l-4.29,4.3L8.78,11.44a1,1,0,1,0-1.41,1.41l2.35,2.36a1,1,0,0,0,.71.29,1,1,0,0,0,.7-.29l5-5a1,1,0,0,0,0-1.42A1,1,0,0,0,14.72,8.79ZM12,2A10,10,0,1,0,22,12,10,10,0,0,0,12,2Zm0,18a8,8,0,1,1,8-8A8,8,0,0,1,12,20Z" />
                            </svg>
                        }
                        description="Reviews generated"
                        emptyNote="No reviews yet."
                        emptyCtaLabel="Start one now"
                        onEmptyCta={() => navigate('/reviews/new')}
                    />
                    <StatCard
                        variant="primary"
                        title="AI Comments"
                        value={aiComments}
                        icon={
                            <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path d="M12,2A10,10,0,0,0,2,12a9.89,9.89,0,0,0,2.26,6.33l-2,2a1,1,0,0,0-.21,1.09A1,1,0,0,0,3,22h9A10,10,0,0,0,12,2Zm0,18H5.41l.93-.93a1,1,0,0,0,0-1.41A8,8,0,1,1,12,20Z" />
                            </svg>
                        }
                        description="Comments generated"
                        emptyNote="No comments yet."
                        emptyCtaLabel="Run a review"
                        onEmptyCta={() => navigate('/reviews/new')}
                    />
                    <div className="relative group">
                        <StatCard
                            title="Git Providers"
                            value={connectedProviders}
                            icon={<Icons.Git />}
                            description="Connected services"
                            emptyNote="No Git providers connected."
                            emptyCtaLabel="Connect now"
                            onEmptyCta={() => navigate('/git')}
                            headerInfo="GitHub, GitLab or Bitbucket accounts connected to LiveReview."
                            headerInfoPosition="bottom"
                        />
                    </div>
                    <div className="relative group">
                        <StatCard
                            title="AI Providers"
                            value={aiConnectors}
                            icon={<Icons.AI />}
                            description="Connected AI backends"
                            emptyNote="No AI providers configured."
                            emptyCtaLabel="Configure now"
                            onEmptyCta={() => navigate('/ai')}
                            headerInfo="LLM backends like OpenAI or local models used to generate review comments."
                            headerInfoPosition="bottom"
                        />
                    </div>
                </div>

                {/* Main Content Grid */}
                <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                    <div className="lg:col-span-2">
                        <RecentActivity className="h-fit" />
                    </div>

                    <div className="space-y-6">
                        {isEmpty && (
                            <Card title="Get Started" subtitle="Connect a provider or configure AI to begin">
                                <div className="space-y-2">
                                    <Button variant="outline" icon={<Icons.Git />} onClick={() => navigate('/git')} className='mr-2'>
                                        Connect Git Provider
                                    </Button>
                                    <Button variant="outline" icon={<Icons.AI />} onClick={() => navigate('/ai')} className='mr-2'>
                                        Configure AI Service
                                    </Button>
                                    <Button variant="outline" icon={<Icons.Settings />} onClick={() => navigate('/settings')} className='mr-2'>
                                        Review Settings
                                    </Button>
                                </div>
                            </Card>
                        )}

                        {/* Performance Summary */}
                        <Card
                            title="This Week"
                            subtitle="Review performance summary"
                            className="h-fit"
                        >
                            <div className="space-y-3">
                                <div className="flex justify-between items-center">
                                    <span className="text-sm text-slate-300">Reviews Generated</span>
                                    <span className="text-sm font-medium text-white">{dashboardData?.performance_metrics?.reviews_this_week || 0}</span>
                                </div>
                                <div className="flex justify-between items-center">
                                    <span className="text-sm text-slate-300">Comments Made</span>
                                    <span className="text-sm font-medium text-white">{dashboardData?.performance_metrics?.comments_this_week || 0}</span>
                                </div>
                                <div className="flex justify-between items-center">
                                    <span className="text-sm text-slate-300">Avg. Response Time</span>
                                    <span className="text-sm font-medium text-white">{dashboardData?.performance_metrics?.avg_response_time_seconds?.toFixed(1) || '2.3'}s</span>
                                </div>
                                <div className="flex justify-between items-center">
                                    <span className="text-sm text-slate-300">Success Rate</span>
                                    <span className="text-sm font-medium text-white">{dashboardData?.performance_metrics?.success_rate_percentage?.toFixed(1) || '100'}%</span>
                                </div>
                                <div className="pt-2 border-t border-slate-700">
                                    <Button
                                        variant="ghost"
                                        size="sm"
                                        className="w-full text-blue-300 hover:text-blue-200"
                                    >
                                        View Analytics
                                    </Button>
                                </div>
                            </div>
                        </Card>

                        {/* Improved empty state for metrics */}
                        {isEmpty && (
                            <Card className="h-fit" title="No data yet" subtitle="Run your first review to see stats here.">
                                <EmptyState
                                    icon={<Icons.EmptyState />}
                                    title="Nothing to show yet"
                                    description="Once you run a review, you'll see activity, comments and trends here."
                                    action={
                                        <Button variant="primary" icon={<Icons.Add />} onClick={() => navigate('/reviews/new')}>
                                            New Review
                                        </Button>
                                    }
                                />
                            </Card>
                        )}
                    </div>
                </div>

                {/* Upgrade Modal */}
                <LicenseUpgradeDialog
                    open={showUpgradeDialog}
                    onClose={() => setShowUpgradeDialog(false)}
                    requiredTier="team"
                    featureName="Review Creation From Dashboard"
                    featureDescription="Unlock AI-powered code reviews by upgrading to a paid plan. Your current plan is read-only."
                />
            </main>
        </div>
    );
};
