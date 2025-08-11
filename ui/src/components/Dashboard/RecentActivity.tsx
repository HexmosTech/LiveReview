import React, { useEffect, useState } from 'react';
import { 
    Card, 
    Badge, 
    EmptyState, 
    Button, 
    Icons 
} from '../UIPrimitives';
import { HumanizedTimestamp } from '../HumanizedTimestamp';
import { fetchRecentActivities, ActivityEntry, formatActivity, ActivitiesResponse } from '../../api/activities';

interface RecentActivityProps {
    className?: string;
}

export const RecentActivity: React.FC<RecentActivityProps> = ({ className }) => {
    const [activities, setActivities] = useState<ActivityEntry[]>([]);
    const [pagination, setPagination] = useState({
        currentPage: 0,
        pageSize: 10,
        totalCount: 0,
        hasMore: false
    });
    const [isLoading, setIsLoading] = useState(true);
    const [isLoadingMore, setIsLoadingMore] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Load activities for a specific page
    const loadActivities = async (page: number = 0, append: boolean = false) => {
        try {
            if (!append) {
                setIsLoading(true);
            } else {
                setIsLoadingMore(true);
            }
            
            const offset = page * pagination.pageSize;
            const response: ActivitiesResponse = await fetchRecentActivities(pagination.pageSize, offset);
            
            if (append) {
                // Append to existing activities (load more)
                setActivities(prev => [...prev, ...(response.activities || [])]);
            } else {
                // Replace activities (initial load or refresh)
                setActivities(response.activities || []);
            }
            
            setPagination(prev => ({
                ...prev,
                currentPage: page,
                totalCount: response.total_count || 0,
                hasMore: response.has_more || false
            }));
            
            setError(null);
        } catch (err) {
            console.error('Error loading activities:', err);
            setError('Failed to load activities');
            if (!append) {
                setActivities([]);
            }
        } finally {
            setIsLoading(false);
            setIsLoadingMore(false);
        }
    };

    // Load more activities (next page)
    const loadMoreActivities = () => {
        if (!isLoadingMore && pagination.hasMore) {
            loadActivities(pagination.currentPage + 1, true);
        }
    };

    // Refresh activities (reload from beginning)
    const refreshActivities = () => {
        loadActivities(0, false);
    };

    // Initial load and periodic refresh
    useEffect(() => {
        loadActivities(0, false);
        
        // Refresh activities every 30 seconds (only first page)
        const interval = setInterval(() => {
            loadActivities(0, false);
        }, 30 * 1000);
        
        return () => clearInterval(interval);
    }, []);

    const renderActivityItem = (activity: ActivityEntry) => {
        const formattedActivity = formatActivity(activity);
        
        return (
            <div key={activity.id} className="flex items-center justify-between p-3 rounded-lg bg-slate-700/50 hover:bg-slate-700/70 transition-colors">
                <div className="flex items-center space-x-3 flex-1">
                    <div className="text-lg">{formattedActivity.icon}</div>
                    <div className="flex-1">
                        <p className={`text-sm font-medium ${formattedActivity.color}`}>
                            {formattedActivity.title}
                        </p>
                        <p className="text-xs text-slate-400 mt-1">
                            {formattedActivity.description}
                        </p>
                    </div>
                </div>
                <div className="flex items-center space-x-2">
                    {/* External link button if URL is available */}
                    {formattedActivity.actionUrl && (
                        <a
                            href={formattedActivity.actionUrl}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="p-1.5 rounded-md bg-slate-600 hover:bg-slate-500 transition-colors text-slate-300 hover:text-white"
                            title="Open in new tab"
                        >
                            <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                            </svg>
                        </a>
                    )}
                    <Badge variant="default" size="sm" className="bg-slate-600 text-slate-300">
                        <HumanizedTimestamp 
                            timestamp={activity.created_at}
                            className="text-slate-300"
                        />
                    </Badge>
                </div>
            </div>
        );
    };

    return (
        <Card 
            title="Recent Activity" 
            badge={(activities && activities.length > 0) ? `${activities.length}${pagination.totalCount > activities.length ? `/${pagination.totalCount}` : ''}` : undefined}
            badgeColor="bg-blue-100 text-blue-800"
            className={className}
        >
            {isLoading ? (
                <div className="flex items-center justify-center p-6">
                    <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-blue-400"></div>
                    <span className="ml-2 text-sm text-slate-400">Loading activities...</span>
                </div>
            ) : error ? (
                <div className="p-4 bg-red-900/40 rounded-lg border border-red-800/30">
                    <div className="flex items-center justify-between">
                        <p className="text-sm text-red-300">{error}</p>
                        <Button 
                            variant="ghost" 
                            size="sm"
                            onClick={refreshActivities}
                            className="text-red-300 hover:text-red-200"
                        >
                            <div className="w-4 h-4">
                                <Icons.Refresh />
                            </div>
                        </Button>
                    </div>
                </div>
            ) : (activities && activities.length > 0) ? (
                <div className="space-y-3">
                    {activities.map(renderActivityItem)}
                    
                    {/* Pagination Controls - Always show for consistency */}
                    <div className="pt-3 border-t border-slate-700">
                        <div className="flex items-center justify-between">
                            {(pagination.hasMore || pagination.currentPage > 0 || pagination.totalCount > 1) ? (
                                <div className="text-xs text-slate-400">
                                    Showing {activities.length} of {pagination.totalCount} activities
                                </div>
                            ) : (
                                <div className="text-xs text-slate-400">
                                    {pagination.totalCount === 1 ? '1 activity' : 'No activities'}
                                </div>
                            )}
                            <div className="flex items-center space-x-2">
                                {/* Refresh button - always show */}
                                <Button 
                                    variant="ghost" 
                                    size="sm"
                                    onClick={refreshActivities}
                                    disabled={isLoading}
                                    className="text-slate-400 hover:text-slate-300"
                                    title="Refresh activities"
                                >
                                    <div className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`}>
                                        <Icons.Refresh />
                                    </div>
                                </Button>
                                
                                {/* Load more button - only show when there's more data */}
                                {pagination.hasMore && (
                                    <Button 
                                        variant="ghost" 
                                        size="sm"
                                        onClick={loadMoreActivities}
                                        disabled={isLoadingMore}
                                        className="text-blue-300 hover:text-blue-200"
                                    >
                                        {isLoadingMore ? (
                                            <>
                                                <div className="animate-spin rounded-full h-3 w-3 border-b border-current mr-2"></div>
                                                Loading...
                                            </>
                                        ) : (
                                            <>
                                                Load More
                                                <div className="w-3 h-3 ml-1">
                                                    <Icons.ChevronDown />
                                                </div>
                                            </>
                                        )}
                                    </Button>
                                )}
                            </div>
                        </div>
                    </div>
                </div>
            ) : (
                <EmptyState
                    icon={<Icons.EmptyState />}
                    title="No recent activity"
                    description="Your recent activity will appear here as you trigger reviews and create connectors"
                />
            )}
        </Card>
    );
};

export default RecentActivity;
