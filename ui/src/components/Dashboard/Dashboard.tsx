import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
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
} from '../UIPrimitives';
import { HumanizedTimestamp } from '../HumanizedTimestamp/HumanizedTimestamp';
import RecentActivity from './RecentActivity';
import { OnboardingStepper } from './OnboardingStepper';
import { PlanBadge } from './PlanBadge';
import { handleUserLoginNotification } from '../../utils/userNotifications';
import { useAppSelector } from '../../store/configureStore';

export const Dashboard: React.FC = () => {
    const navigate = useNavigate();
    const user = useAppSelector(state => state.Auth.user);
    
    // Dashboard data state
    const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [isSyncing, setIsSyncing] = useState(false);
    const [hideStepper, setHideStepper] = useState<boolean>(() => {
        try { return localStorage.getItem('lr_hide_get_started') === '1'; } catch { return false; }
    });
    const [notificationSent, setNotificationSent] = useState(false);

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

    // Use dashboard API data exclusively - no fallbacks to Redux store
    const aiComments = dashboardData?.total_comments || 0;
    const codeReviews = dashboardData?.total_reviews || 0;
    const connectedProviders = dashboardData?.connected_providers || 0;
    const aiConnectors = dashboardData?.active_ai_connectors || 0;

    // Derive onboarding state
    const hasGitProvider = connectedProviders > 0;
    const hasAIProvider = aiConnectors > 0;
    const allSet = hasGitProvider && hasAIProvider;
    // Auto-hide thresholds: disappear when the user clearly moved past onboarding
    const autoHideStepper = connectedProviders > 1 && aiConnectors > 1 && codeReviews > 1;
    const hasRunReview = codeReviews > 0;

    // After the user has completed all steps and seen the panel once,
    // auto-hide it and persist the dismissal so it doesn't reappear.
    useEffect(() => {
        if (!hideStepper && allSet && hasRunReview) {
            let wasAutoHidden = false;
            try { wasAutoHidden = localStorage.getItem('lr_get_started_auto_hidden') === '1'; } catch {}
            if (!wasAutoHidden) {
                const timer = setTimeout(() => {
                    setHideStepper(true);
                    try {
                        localStorage.setItem('lr_hide_get_started', '1');
                        localStorage.setItem('lr_get_started_auto_hidden', '1');
                    } catch {}
                }, 3500); // give a moment to notice completion, then hide
                return () => clearTimeout(timer);
            }
        }
    }, [hideStepper, allSet, hasRunReview]);

    // Check if this is an empty state (no connections and no activity)
    const isEmpty = connectedProviders === 0 && codeReviews === 0 && aiComments === 0;

    return (
        <div className="min-h-screen">
            <main className="container mx-auto px-4 py-6">
                {/* Header with aligned content and prominent CTA */}
                <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center mb-6">
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
                    <div className="flex gap-3">
                        <Button 
                            variant="primary" 
                            icon={<Icons.Add />}
                            onClick={() => navigate('/reviews/new')}
                            className="shadow-xl hover:shadow-2xl transition-all duration-300 hover:scale-105 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600"
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
                    onClick={() => navigate('/reviews/new')}
                    className="fixed bottom-6 right-6 sm:hidden z-40 rounded-full w-14 h-14 shadow-xl hover:shadow-2xl transition-all duration-300 hover:scale-110 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-500 hover:to-blue-600"
                    aria-label="New Review"
                />

                {/* Get Started stepper – stays visible until the user dismisses it, unless thresholds auto-hide it */}
                {!hideStepper && !autoHideStepper && (
                    <OnboardingStepper
                        hasGitProvider={hasGitProvider}
                        hasAIProvider={hasAIProvider}
                        hasRunReview={hasRunReview}
                        onConnectGit={() => navigate('/git')}
                        onConfigureAI={() => navigate('/ai')}
                        onNewReview={() => navigate('/reviews/new')}
                        onDismiss={() => { setHideStepper(true); try { localStorage.setItem('lr_hide_get_started','1'); } catch {} }}
                        className="mb-6"
                    />
                )}

                {/* Main Statistics Grid - Improved density and alignment */}
                <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
                    <StatCard 
                        variant="primary"
                        title="AI Reviews" 
                        value={codeReviews} 
                        icon={
                            <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path d="M14.72,8.79l-4.29,4.3L8.78,11.44a1,1,0,1,0-1.41,1.41l2.35,2.36a1,1,0,0,0,.71.29,1,1,0,0,0,.7-.29l5-5a1,1,0,0,0,0-1.42A1,1,0,0,0,14.72,8.79ZM12,2A10,10,0,1,0,22,12,10,10,0,0,0,12,2Zm0,18a8,8,0,1,1,8-8A8,8,0,0,1,12,20Z"/>
                            </svg>
                        }
                        description="AI reviews triggered"
                        emptyNote="No reviews yet."
                        emptyCtaLabel="Create one now"
                        onEmptyCta={() => navigate('/reviews/new')}
                    />
                    <StatCard 
                        variant="primary"
                        title="AI Comments" 
                        value={aiComments} 
                        icon={
                            <svg className="w-5 h-5" fill="currentColor" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
                                <path d="M12,2A10,10,0,0,0,2,12a9.89,9.89,0,0,0,2.26,6.33l-2,2a1,1,0,0,0-.21,1.09A1,1,0,0,0,3,22h9A10,10,0,0,0,12,2Zm0,18H5.41l.93-.93a1,1,0,0,0,0-1.41A8,8,0,1,1,12,20Z"/>
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
                                                            description="Once you trigger a review, you'll see activity, comments and trends here."
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
            </main>
        </div>
    );
};
