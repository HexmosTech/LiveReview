import React from 'react';
import { useNavigate } from 'react-router-dom';
import { StatCard, Icons, EmptyState } from '../../UIPrimitives';
import { useDashboardPeriod } from './DashboardPeriod';
import { useSystemOverview, hasNoSystemOverviewData } from './SystemOverviewData';
import { ChartSkeleton } from './ChartSkeleton';

export const AverageReviewsStat: React.FC = () => {
    const navigate = useNavigate();
    const { period } = useDashboardPeriod();
    const { systemOverview, loading } = useSystemOverview();

    if (loading) {
        return <ChartSkeleton />;
    }

    if (hasNoSystemOverviewData(systemOverview)) {
        return (
            <EmptyState
                icon={<Icons.EmptyState />}
                title="No review data yet"
                description="Review averages will appear here once reviews start running."
            />
        );
    }

    const overview = systemOverview!;

    return (
        <div className="grid grid-cols-2 gap-3 h-full">
            <button type="button" onClick={() => navigate('/reports?mode=explore')} className="text-left h-full">
                <StatCard
                    title="Avg Reviews / PR"
                    value={overview.avg_reviews_per_pr[period].toFixed(1)}
                    description="Across all connected repositories"
                    icon={<Icons.Reviews />}
                    className="h-full cursor-pointer"
                />
            </button>
            <button type="button" onClick={() => navigate('/reports?mode=explore')} className="text-left h-full">
                <StatCard
                    title="Avg Reviews / Commit"
                    value={overview.avg_reviews_per_commit[period].toFixed(2)}
                    description="Pre-commit reviews"
                    icon={<Icons.Reviews />}
                    className="h-full cursor-pointer"
                />
            </button>
        </div>
    );
};
