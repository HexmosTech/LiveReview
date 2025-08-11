import React, { useEffect, useState } from 'react';
import { 
    Card, 
    Badge, 
    EmptyState, 
    Button, 
    Icons 
} from '../UIPrimitives';
import { HumanizedTimestamp } from '../HumanizedTimestamp';
import { fetchRecentActivities, ActivityEntry, formatActivity } from '../../api/activities';

interface RecentActivityProps {
    className?: string;
}

export const RecentActivity: React.FC<RecentActivityProps> = ({ className }) => {
    const [activities, setActivities] = useState<ActivityEntry[]>([]);
    const [isLoading, setIsLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    // Load recent activities
    useEffect(() => {
        const loadActivities = async () => {
            try {
                setIsLoading(true);
                const response = await fetchRecentActivities(10, 0); // Fetch 10 most recent
                setActivities(response.activities || []); // Ensure we always have an array
                setError(null);
            } catch (err) {
                console.error('Error loading activities:', err);
                setError('Failed to load activities');
                setActivities([]); // Set empty array on error
            } finally {
                setIsLoading(false);
            }
        };

        loadActivities();
        
        // Refresh activities every 30 seconds
        const interval = setInterval(loadActivities, 30 * 1000);
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
            badge={(activities && activities.length > 0) ? `${activities.length}` : undefined}
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
                    <p className="text-sm text-red-300">{error}</p>
                </div>
            ) : (activities && activities.length > 0) ? (
                <div className="space-y-3">
                    {activities.map(renderActivityItem)}
                    {activities.length >= 10 && (
                        <div className="pt-2 border-t border-slate-700">
                            <Button 
                                variant="ghost" 
                                size="sm"
                                className="w-full text-blue-300 hover:text-blue-200"
                            >
                                View All Activity
                            </Button>
                        </div>
                    )}
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
