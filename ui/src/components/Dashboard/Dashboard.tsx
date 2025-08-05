import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useAppDispatch, useAppSelector } from '../../store/configureStore';
import { getDashboardData, DashboardData } from '../../api/dashboard';
import { 
    StatCard, 
    Section, 
    PageHeader, 
    Card, 
    Badge, 
    EmptyState, 
    Button, 
    Icons 
} from '../UIPrimitives';

export const Dashboard: React.FC = () => {
    const dispatch = useAppDispatch();
    const navigate = useNavigate();
    const connectors = useAppSelector((state) => state.Connector.connectors);
    
    // Dashboard data state
    const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Load dashboard data
    useEffect(() => {
        const loadDashboardData = async () => {
            try {
                setIsLoading(true);
                const data = await getDashboardData();
                setDashboardData(data);
                setError(null);
            } catch (err) {
                console.error('Error loading dashboard data:', err);
                setError('Failed to load dashboard data');
            } finally {
                setIsLoading(false);
            }
        };

        loadDashboardData();
        
        // Refresh data every 5 minutes
        const interval = setInterval(loadDashboardData, 5 * 60 * 1000);
        return () => clearInterval(interval);
    }, []);

    // Use real data if available, fallback to defaults
    const aiComments = dashboardData?.total_comments || 0;
    const codeReviews = dashboardData?.total_reviews || 0;
    const connectedProviders = dashboardData?.connected_providers || connectors.length;
    const aiConnectors = dashboardData?.active_ai_connectors || 1;
    const recentActivity = dashboardData?.recent_activity || [];
    
    // Mock service info (could be moved to dashboard data later)
    const aiService = 'Gemini';
    const apiKey = 'sk-xxxxxxx';

    // Check if this is an empty state (no connections and no activity)
    const isEmpty = connectedProviders === 0 && codeReviews === 0 && aiComments === 0;

    return (
        <div className="min-h-screen">
            <main className="container mx-auto px-4 py-6">
                {/* Header with aligned content and prominent CTA */}
                <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center mb-6">
                    <div className="mb-4 sm:mb-0">
                        <h1 className="text-2xl font-bold text-white">Dashboard</h1>
                        <p className="mt-1 text-base text-slate-300">
                            Monitor your code review activity and connected services
                            {dashboardData && (
                                <span className="text-xs text-slate-400 ml-2">
                                    Last updated: {new Date(dashboardData.last_updated).toLocaleTimeString()}
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
                        {isEmpty && (
                            <Button 
                                variant="outline" 
                                icon={<Icons.Settings />}
                                onClick={() => navigate('/git')}
                                className="border-blue-400 text-blue-300 hover:bg-blue-900/30"
                            >
                                Get Started
                            </Button>
                        )}
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

                {/* Empty State Banner */}
                {isEmpty && (
                    <div className="mb-6 bg-gradient-to-r from-blue-900/40 to-slate-800/40 rounded-xl p-6 border border-blue-800/30">
                        <div className="flex items-center">
                            <Icons.Info />
                            <div className="ml-3">
                                <h3 className="text-lg font-medium text-blue-300">Welcome to LiveReview!</h3>
                                <p className="mt-1 text-slate-300">Connect a Git provider and configure AI settings to get started with automated code reviews.</p>
                                <div className="mt-3 flex gap-3">
                                    <Button 
                                        variant="primary" 
                                        size="sm"
                                        icon={<Icons.Git />}
                                        onClick={() => navigate('/git')}
                                    >
                                        Connect Git Provider
                                    </Button>
                                    <Button 
                                        variant="outline" 
                                        size="sm"
                                        icon={<Icons.AI />}
                                        onClick={() => navigate('/ai')}
                                        className="border-blue-400 text-blue-300"
                                    >
                                        Configure AI
                                    </Button>
                                </div>
                            </div>
                        </div>
                    </div>
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
                        description="Completed code reviews"
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
                    />
                    <StatCard 
                        title="Git Providers" 
                        value={connectedProviders} 
                        icon={<Icons.Git />}
                        description="Connected services"
                    />
                    <StatCard 
                        title="AI Providers" 
                        value={aiConnectors} 
                        icon={<Icons.AI />}
                        description="Connected AI backends"
                    />
                </div>

                {/* Main Content Grid */}
                <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                    <div className="lg:col-span-2">
                        <Card 
                            title="Recent Activity" 
                            badge={recentActivity.length > 0 ? `${recentActivity.length}` : undefined}
                            badgeColor="bg-blue-100 text-blue-800"
                            className="h-fit"
                        >
                            {!isEmpty && recentActivity.length > 0 ? (
                                <div className="space-y-3">
                                    {recentActivity.map((item) => (
                                        <div key={item.id} className="flex items-center justify-between p-3 rounded-lg bg-slate-700/50">
                                            <div className="flex items-center space-x-3">
                                                <div className="w-2 h-2 bg-blue-400 rounded-full"></div>
                                                <div>
                                                    <p className="text-sm font-medium text-slate-100">{item.action}</p>
                                                    <p className="text-xs text-slate-400">{item.repository}</p>
                                                </div>
                                            </div>
                                            <Badge variant="default" size="sm" className="bg-slate-600 text-slate-300">{item.time_ago}</Badge>
                                        </div>
                                    ))}
                                    <div className="pt-2 border-t border-slate-700">
                                        <Button 
                                            variant="ghost" 
                                            size="sm"
                                            className="w-full text-blue-300 hover:text-blue-200"
                                        >
                                            View All Activity
                                        </Button>
                                    </div>
                                </div>
                            ) : (
                                <EmptyState
                                    icon={<Icons.EmptyState />}
                                    title={isEmpty ? "Ready to start reviewing" : "No recent activity"}
                                    description={isEmpty 
                                        ? "Connect your repositories to see review activity here" 
                                        : "Your recent code review activity will appear here"
                                    }
                                    action={isEmpty ? (
                                        <Button 
                                            variant="primary" 
                                            size="sm"
                                            icon={<Icons.Git />}
                                            onClick={() => navigate('/git')}
                                        >
                                            Connect Repository
                                        </Button>
                                    ) : undefined}
                                />
                            )}
                        </Card>
                    </div>

                    <div className="space-y-6">
                        <Card 
                            title="Quick Actions" 
                            subtitle="Common tasks and shortcuts"
                            className="h-fit"
                        >
                            <div className="space-y-2">
                                <Button 
                                    variant="outline" 
                                    fullWidth 
                                    className="justify-start text-sm" 
                                    icon={<Icons.Git />}
                                    onClick={() => navigate('/git')}
                                >
                                    Connect Git Provider
                                </Button>
                                <Button 
                                    variant="outline" 
                                    fullWidth 
                                    className="justify-start text-sm" 
                                    icon={<Icons.AI />}
                                    onClick={() => navigate('/ai')}
                                >
                                    Configure AI Service
                                </Button>
                                <Button 
                                    variant="outline" 
                                    fullWidth 
                                    className="justify-start text-sm" 
                                    icon={<Icons.Settings />}
                                    onClick={() => navigate('/settings')}
                                >
                                    Review Settings
                                </Button>
                            </div>
                        </Card>

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
                    </div>
                </div>
            </main>
        </div>
    );
};
