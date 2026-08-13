import React, { useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { getDashboardData, DashboardData, refreshDashboardData } from '../../api/dashboard';
import {
    Button,
    Icons,
    Tooltip,
    Alert,
} from '../UIPrimitives';
import { HumanizedTimestamp } from '../HumanizedTimestamp/HumanizedTimestamp';
import RecentActivity from './RecentActivity';
import { FloatingOnboardingNudge } from './FloatingOnboardingNudge';
import { DashboardGrid } from './widgets/DashboardGrid';
import { PlanBadge } from './PlanBadge';
import { QuotaExhaustedBanner } from './QuotaExhaustedBanner';
import { QuotaWarningBanner } from './QuotaWarningBanner';
import { handleUserLoginNotification } from '../../utils/userNotifications';
import { getApiUrl } from '../../utils/apiUrl';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { isCloudMode } from '../../utils/deploymentMode';
import { useOrgContext } from '../../hooks/useOrgContext';
import LicenseUpgradeDialog from '../License/LicenseUpgradeDialog';
import apiClient from '../../api/apiClient';
import {
    add as addNotification,
    dismiss as dismissNotification,
    dismissPermanently as dismissNotificationPermanently,
} from '../../store/Notifications/slice';

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

const connectorNotificationId = (connectorId: number): string => `connector-${connectorId}`;
const ONBOARDING_STEPPER_ID = 'onboarding-stepper';

export const Dashboard: React.FC = () => {
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const forceShowCli = searchParams.get('showCli') === '1';
    const user = useAppSelector(state => state.Auth.user);
    const { isFreePlan } = useOrgContext();

    // Dashboard data state
    const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [isSyncing, setIsSyncing] = useState(false);
    const [notificationSent, setNotificationSent] = useState(false);
    const [showUpgradeDialog, setShowUpgradeDialog] = useState(false);
    const [billingInsight, setBillingInsight] = useState<DashboardBillingInsight | null>(null);
    const dispatch = useAppDispatch();
    const notificationItems = useAppSelector(state => state.Notifications.items);
    const hideStepper = !!notificationItems.find(n => n.id === ONBOARDING_STEPPER_ID)?.dismissed;

    const isConnectorDismissed = (connectorId: number): boolean => {
        const item = notificationItems.find(n => n.id === connectorNotificationId(connectorId));
        return !!item?.dismissed;
    };

    const dismissConnectorForSession = (connectorId: number): void => {
        dispatch(dismissNotification(connectorNotificationId(connectorId)));
    };

    const dismissConnectorPermanently = (connectorId: number): void => {
        dispatch(dismissNotificationPermanently(connectorNotificationId(connectorId)));
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

    // Register the onboarding stepper as a (session-visible, permanently
    // dismissible) notification once dashboard data has loaded — deferring
    // past the synchronous mount phase gives ToastBridge's hydrate() time to
    // apply any prior "don't show again" dismissal before this first add().
    useEffect(() => {
        if (!dashboardData) return;
        dispatch(addNotification({
            dedupeKey: ONBOARDING_STEPPER_ID,
            severity: 'info',
            message: 'Complete your onboarding checklist to get the most out of LiveReview.',
            source: 'onboarding',
            toast: false,
            persistDismiss: false,
        }));
    }, [dashboardData, dispatch]);

    useEffect(() => {

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
                
                // Hide billing insight for enterprise-selfhosted (licensed)
                if (!isCloudMode() && planCode === 'enterprise-selfhosted') {
                    setBillingInsight(null);
                    return;
                }

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
    const codeReviews = dashboardData?.total_reviews || 0;
    const aiConnectors = dashboardData?.active_ai_connectors || 0;

    // Derive onboarding state
    const hasCLI = dashboardData?.cli_installed || false;
    const hasAIProvider = aiConnectors > 0;
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

    // Get connectors that need setup attention (filter out dismissed ones)
    const connectorsNeedingSetup = (dashboardData?.connector_setup_progress || []).filter(
        c => !isConnectorDismissed(c.connector_id)
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

    // Mirror connector setup progress into the Notifications store — gives the
    // dismiss actions above an id to operate on, and surfaces setup issues in
    // the global tray even after the banner itself is dismissed from the dashboard.
    useEffect(() => {
        const progress = dashboardData?.connector_setup_progress || [];
        progress.forEach((connector) => {
            dispatch(addNotification({
                dedupeKey: connectorNotificationId(connector.connector_id),
                severity: getPhaseVariant(connector.phase),
                message: getPhaseMessage(
                    connector.phase,
                    connector.connector_name,
                    connector.provider,
                    connector.total_projects,
                    connector.connected_projects
                ),
                source: 'connector',
                toast: false,
                persistDismiss: false,
            }));
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [dashboardData?.connector_setup_progress, dispatch]);

    const handleNewReviewClick = () => {
        if (isFreePlan) {
            setShowUpgradeDialog(true);
        } else {
            navigate('/reviews/new');
        }
    };

    return (
        <div className="min-h-screen">
            <main className="container mx-auto px-4 py-8">
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
                            {isCloudMode() && (
                                <Button
                                    variant="primary"
                                    size="sm"
                                    onClick={() => navigate('/subscribe')}
                                    className="!bg-red-600 hover:!bg-red-500 flex-shrink-0"
                                >
                                    Upgrade Now
                                </Button>
                            )}
                        </div>
                    </Alert>
                )}
                {billingInsight && !billingInsight.blocked && billingInsight.usagePct >= 100 && (
                    <QuotaExhaustedBanner
                        locUsed={billingInsight.locUsed}
                        locLimit={billingInsight.locLimit}
                        usagePct={billingInsight.usagePct}
                        onUpgrade={isCloudMode() ? () => navigate('/settings-subscriptions-overview') : undefined}
                    />
                )}
                {billingInsight && !billingInsight.blocked && billingInsight.usagePct >= 90 && billingInsight.usagePct < 100 && (
                    <QuotaWarningBanner
                        locUsed={billingInsight.locUsed}
                        locLimit={billingInsight.locLimit}
                        usagePct={billingInsight.usagePct}
                        onUpgrade={isCloudMode() ? () => navigate('/settings-subscriptions-overview') : undefined}
                    />
                )}

                {/* Header with aligned content and prominent CTA */}
                <div className="flex flex-col gap-3 sm:flex-row sm:justify-between sm:items-center mb-6">
                    <p className="text-sm text-slate-400">
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
                    <div className="hidden sm:flex gap-3">
                        <Button
                            variant="primary"
                            icon={<Icons.Add />}
                            onClick={handleNewReviewClick}
                            title={isFreePlan ? "Upgrade your plan to create reviews" : "Safe review - no comments posted"}
                        >
                            New Review
                        </Button>
                    </div>
                </div>

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

                {/* Floating Action Button for mobile */}
                <Button
                    variant="primary"
                    icon={<Icons.Add />}
                    onClick={handleNewReviewClick}
                    className="fixed bottom-6 right-6 sm:hidden z-40 rounded-full w-14 h-14 shadow-lg"
                    aria-label={isFreePlan ? "Review creation locked" : "New Review (Safe - no comments posted)"}
                    title={isFreePlan ? "Upgrade your plan to create reviews" : "Safe review - no comments posted"}
                />

                {/* Floating setup-guide nudge – stays available until the user manually dismisses it */}
                {(!hideStepper || forceShowCli) && (
                    <FloatingOnboardingNudge
                        hasCLI={hasCLI}
                        hasAIProvider={hasAIProvider}
                        hasRunReview={hasRunReview}
                        installCommand={installCommand}
                        installCommandWindows={installCommandWindows}
                        onConfigureAI={() => navigate('/ai')}
                        onNewReview={() => navigate('/reviews/new')}
                        isFreePlan={isFreePlan}
                        onUpgrade={() => setShowUpgradeDialog(true)}
                        forceOpen={forceShowCli}
                        onDismiss={() => dispatch(dismissNotificationPermanently(ONBOARDING_STEPPER_ID))}
                    />
                )}

                {/* Customizable engineering dashboard — drag/resize/add/remove widgets */}
                <DashboardGrid userId={user?.id} />

                {/* Real, API-backed activity feed — kept as a fixed section below the widget grid */}
                <RecentActivity className="mt-6" />

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
